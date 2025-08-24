package terminal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
)

type Session struct {
	messageFeed *MessageFeed
	input       textarea.Model
	spinner     spinner.Model

	width  int
	height int

	apiClient   *api_client.Client
	messages    []message
	task        *v1.Task
	activeAgent *v1.Agent
	agents      []*v1.Agent

	ctx     context.Context
	Verbose bool

	state           appState
	mode            uiMode
	showHelp        bool
	waitingForAgent bool
	lastUsage       *v1.TaskUsage
	workspacePath   string
	lastCtrlC       time.Time
}

var _ tea.Model = (*Session)(nil)

func NewSession(ctx context.Context, apiClient *api_client.Client, task *v1.Task, agent *v1.Agent) *Session {
	ta := textarea.New()
	ta.Focus()
	ta.CharLimit = 32768
	ta.ShowLineNumbers = false
	ta.SetHeight(4)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.Prompt = ""
	ta.Placeholder = "Type your message..."

	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"※", "⁂", "⁕", "⁜"},
		FPS:    time.Second / 10, //nolint:gomnd
	}
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	workspacePath := getWorkspacePath(task)

	agents, err := apiClient.Agent().ListAgents(ctx, &connect.Request[v1.ListAgentsRequest]{})
	if err != nil {
		slog.Error("failed to list agents", "error", err)
	}

	return &Session{
		width:           80,
		height:          20,
		input:           ta,
		messageFeed:     NewMessageFeed(),
		spinner:         sp,
		apiClient:       apiClient,
		messages:        []message{},
		activeAgent:     agent,
		agents:          agents.Msg.Agents,
		task:            task,
		ctx:             ctx,
		state:           StateNormal,
		mode:            ModeInput,
		showHelp:        false,
		waitingForAgent: false,
		lastUsage:       task.Status.Usage,
		workspacePath:   workspacePath,
	}
}

func (m Session) Init() tea.Cmd {
	windowTitle := "construct"
	if m.workspacePath != "" {
		windowTitle = fmt.Sprintf("construct (%s)", m.workspacePath)
	}

	return tea.Batch(
		tea.SetWindowTitle(windowTitle),
	)
}

func (m *Session) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.showHelp {
			if msg.Type == tea.KeyEsc || msg.String() == "ctrl+?" {
				m.showHelp = false
				return m, nil
			}
			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			return m, m.handleCtrlC()
		case tea.KeyEsc:
			m.handleEsc()
		default:
			cmds = append(cmds, m.onKeyEvent(msg))
		}

	case tea.WindowSizeMsg:
		m.onWindowResize(msg)

	case *v1.TaskEvent:
		m.processTaskEvent(msg)
	}

	if !m.showHelp {
		switch k := msg.(type) {
		case tea.KeyMsg:
			// Ignore Alt/ESC-prefixed key messages which are usually terminal
			// responses (e.g. OSC colour queries). These have k.Alt == true.
			// We forward only genuine user keyboard input (Alt not pressed).
			if !k.Alt {
				m.input, cmd = m.input.Update(msg)
				cmds = append(cmds, cmd)
			}
		case tea.MouseMsg:
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	messageFeed, cmd := m.messageFeed.Update(msg)
	m.messageFeed = messageFeed.(*MessageFeed)
	cmds = append(cmds, cmd)

	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Session) onKeyEvent(msg tea.KeyMsg) tea.Cmd {
	// Reset Ctrl+C timer on any other key press
	m.lastCtrlC = time.Time{}

	switch msg.Type {
	case tea.KeyTab:
		return m.onToggleAgent()
	case tea.KeyEnter:
		if m.mode == ModeInput {
			return m.onMessageSend(msg)
		}
	case tea.KeyF1:
		m.mode = ModeInput
		m.input.Focus()
		return nil
	case tea.KeyF2:
		m.mode = ModeScroll
		m.input.Blur()
		return nil
	}

	switch msg.String() {

	case "ctrl+?":
		m.showHelp = !m.showHelp
		return nil
	}

	return nil
}

func (m *Session) handleCtrlC() tea.Cmd {
	now := time.Now()

	// If Ctrl+C was pressed recently (within 1 second), quit the app
	if !m.lastCtrlC.IsZero() && now.Sub(m.lastCtrlC) < time.Second {
		return tea.Quit
	}

	// First Ctrl+C: clear the input and record the time
	m.input.Reset()
	m.lastCtrlC = now

	return nil
}

func (m *Session) handleEsc() {
	_, err := m.apiClient.Task().SuspendTask(m.ctx, &connect.Request[v1.SuspendTaskRequest]{
		Msg: &v1.SuspendTaskRequest{
			TaskId: m.task.Metadata.Id,
		},
	})

	if err != nil {
		slog.Error("failed to suspend task", "error", err)
	}
}

func (m *Session) onMessageSend(_ tea.KeyMsg) tea.Cmd {
	if m.input.Value() != "" {
		userInput := strings.TrimSpace(m.input.Value())
		m.input.Reset()

		m.waitingForAgent = true

		return m.sendMessage(userInput)
	}

	return nil
}

func (m *Session) sendMessage(userInput string) tea.Cmd {
	return func() tea.Msg {
		_, err := m.apiClient.Message().CreateMessage(context.Background(), &connect.Request[v1.CreateMessageRequest]{
			Msg: &v1.CreateMessageRequest{
				TaskId: m.task.Metadata.Id,
				Content: []*v1.MessagePart{
					{
						Data: &v1.MessagePart_Text_{
							Text: &v1.MessagePart_Text{
								Content: userInput,
							},
						},
					},
				},
			},
		})

		if err != nil {
			slog.Error("failed to send message", "error", err)
			m.waitingForAgent = false

			return &errorMessage{
				content:   fmt.Sprintf("Error sending message: %v", err),
				timestamp: time.Now(),
			}
		}
		return nil
	}
}

func (m *Session) processTaskEvent(msg *v1.TaskEvent) {
	if msg.TaskId == m.task.Metadata.Id {
		resp, err := m.apiClient.Task().GetTask(m.ctx, &connect.Request[v1.GetTaskRequest]{
			Msg: &v1.GetTaskRequest{
				Id: msg.TaskId,
			},
		})
		if err != nil {
			slog.Error("failed to get task", "error", err)
			return
		}

		m.task = resp.Msg.Task
		m.lastUsage = resp.Msg.Task.Status.Usage
	}
}

func (m *Session) onToggleAgent() tea.Cmd {
	if len(m.agents) <= 1 {
		return nil
	}

	currentIdx := -1
	for i, agent := range m.agents {
		if agent.Metadata.Id == m.activeAgent.Metadata.Id {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		currentIdx = 0
	} else {
		currentIdx = (currentIdx + 1) % len(m.agents)
	}

	m.activeAgent = m.agents[currentIdx]
	return nil
}

func (m *Session) onWindowResize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height

	appWidth := msg.Width - appStyle.GetHorizontalFrameSize()

	headerHeight := lipgloss.Height(m.headerView())
	inputHeight := lipgloss.Height(m.inputView())
	messageFeedHeight := msg.Height - headerHeight - inputHeight - appStyle.GetVerticalFrameSize()
	m.messageFeed.SetSize(appWidth, messageFeedHeight)

	m.input.SetWidth(appWidth)
}

func (m *Session) View() string {
	if m.showHelp {
		return m.renderHelp()
	}

	result := appStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		m.headerView(),
		m.messageFeed.View(),
		m.inputView(),
	))

	return result
}

func (m *Session) headerView() string {
	// Build agent section
	agentName := "Unknown"
	if m.activeAgent != nil {
		agentName = m.activeAgent.Spec.Name
	}

	taskStatus := "Unknown"
	if m.task != nil {
		switch m.task.Status.Phase {
		case v1.TaskPhase_TASK_PHASE_AWAITING:
			taskStatus = "Idle"
		case v1.TaskPhase_TASK_PHASE_RUNNING:
			taskStatus = "Thinking"
		case v1.TaskPhase_TASK_PHASE_SUSPENDED:
			taskStatus = "Suspended"
		}
	}

	statusText := ""
	if m.task.Status.Phase == v1.TaskPhase_TASK_PHASE_RUNNING {
		statusText = m.spinner.View() + taskStatusStyle.Render(taskStatus)
	} else {
		statusText = taskStatusStyle.Render(taskStatus)
	}

	modelName, contextWindowSize, err := m.getAgentModelInfo(m.activeAgent)
	if err != nil {
		slog.Error("failed to get model info", "error", err)
	}

	agentSection := lipgloss.JoinHorizontal(lipgloss.Left,
		agentDiamondStyle.Render("» "),
		agentNameStyle.Render(agentName),
	)

	if modelName != "" {
		agentSection = lipgloss.JoinHorizontal(lipgloss.Left,
			agentSection,
			bulletSeparatorStyle.Render(" • "),
			agentModelStyle.Render(abbreviateModelName(modelName)),
		)
	}

	left := lipgloss.JoinHorizontal(lipgloss.Left,
		agentSection,
		bulletSeparatorStyle.Render(" • "),
		statusText,
	)

	// usage section
	usageText := ""
	if m.lastUsage != nil {
		tokenDisplay := fmt.Sprintf("Tokens: %d↑ %d↓", m.lastUsage.InputTokens, m.lastUsage.OutputTokens)

		if m.lastUsage.CacheReadTokens > 0 || m.lastUsage.CacheWriteTokens > 0 {
			tokenDisplay += fmt.Sprintf(" (Cache: %d↑ %d↓)", m.lastUsage.CacheReadTokens, m.lastUsage.CacheWriteTokens)
		}

		contextUsage := m.calculateContextUsage(contextWindowSize)
		if contextUsage >= 0 {
			tokenDisplay += fmt.Sprintf(" | Context: %d%%", contextUsage)
		}

		usageText = usageStyle.Render(fmt.Sprintf("%s | Cost: $%.2f", tokenDisplay, m.lastUsage.Cost))
	}

	headerContent := lipgloss.JoinHorizontal(lipgloss.Left,
		left,
		strings.Repeat(" ", Max(0, m.width-lipgloss.Width(left)-lipgloss.Width(usageText)-4)),
		usageText,
	)

	return headerStyle.Render(headerContent)
}

func (m *Session) inputView() string {
	return inputStyle.Render(m.input.View())
}

func (m *Session) getAgentModelInfo(agent *v1.Agent) (string, int64, error) {
	if agent.Spec.ModelId == "" {
		return "", 0, fmt.Errorf("agent %s has no model", agent.Metadata.Id)
	}

	resp, err := m.apiClient.Model().GetModel(m.ctx, &connect.Request[v1.GetModelRequest]{
		Msg: &v1.GetModelRequest{
			Id: agent.Spec.ModelId,
		},
	})
	if err != nil {
		return "", 0, fmt.Errorf("failed to retrieve model %s: %w", agent.Spec.ModelId, err)
	}

	return resp.Msg.Model.Spec.Name, resp.Msg.Model.Spec.ContextWindow, nil
}

func (m *Session) calculateContextUsage(contextWindowSize int64) int {
	if m.lastUsage == nil || m.activeAgent == nil {
		return -1
	}

	if contextWindowSize <= 0 {
		slog.Error("invalid context window size", "contextWindowSize", contextWindowSize)
		return -1
	}

	totalTokens := m.lastUsage.InputTokens + m.lastUsage.OutputTokens + m.lastUsage.CacheReadTokens + m.lastUsage.CacheWriteTokens
	percentage := int((float64(totalTokens) / float64(contextWindowSize)) * 100)
	if percentage > 100 {
		percentage = 100
	}

	return percentage
}

func getWorkspacePath(task *v1.Task) string {
	if task.Spec.Workspace != "" {
		return abbreviatePath(task.Spec.Workspace)
	}

	return ""
}

func abbreviatePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}

	return path
}

func abbreviateModelName(name string) string {
	if len(name) > 12 {
		return name[:12] + "..."
	}
	return name
}
