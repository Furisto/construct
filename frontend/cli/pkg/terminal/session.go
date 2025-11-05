package terminal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
)

type modelInfo struct {
	name          string
	contextWindow int64
}

type SessionKeyBindings struct {
	Help        key.Binding
	SendMessage key.Binding
	NewLine     key.Binding
	SwitchAgent key.Binding
	ClearOrQuit key.Binding
	SuspendTask key.Binding
}

func NewSessionKeyBindings() SessionKeyBindings {
	return SessionKeyBindings{
		Help: key.NewBinding(
			key.WithKeys("ctrl+?"),
			key.WithHelp("ctrl+?", "toggle help"),
		),
		SendMessage: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "send message"),
		),
		SwitchAgent: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch agent"),
		),
		ClearOrQuit: key.NewBinding(
			key.WithKeys(tea.KeyCtrlC.String()),
			key.WithHelp("strg+c", "press one to clear input or twice to quit"),
		),
		SuspendTask: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "suspend task execution"),
		),
	}
}

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

	showHelp        bool
	waitingForAgent bool
	lastUsage       Usage
	workspacePath   string
	lastCtrlC       time.Time
	keyBindings     SessionKeyBindings

	modelInfoCache   map[string]*modelInfo
	currentModelInfo *modelInfo
}

type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheWriteTokens int64
	CacheReadTokens  int64
	Cost             float64
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
	ta.KeyMap.InsertNewline.SetEnabled(true)
	ta.KeyMap.InsertNewline.SetKeys("alt+enter")

	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: spinner.MiniDot.Frames,
		FPS:    time.Second / 6,
	}
	sp.Style = taskStatusStyle

	workspacePath := getWorkspacePath(task)

	return &Session{
		width:            80,
		height:           20,
		input:            ta,
		messageFeed:      NewMessageFeed(),
		spinner:          sp,
		apiClient:        apiClient,
		messages:         []message{},
		activeAgent:      agent,
		agents:           []*v1.Agent{},
		task:             task,
		ctx:              ctx,
		showHelp:         false,
		waitingForAgent:  false,
		lastUsage:        Usage{},
		workspacePath:    workspacePath,
		keyBindings:      NewSessionKeyBindings(),
		modelInfoCache:   make(map[string]*modelInfo),
		currentModelInfo: nil,
	}
}

func (m Session) Init() tea.Cmd {
	windowTitle := "construct"
	if m.workspacePath != "" {
		windowTitle = fmt.Sprintf("construct (%s)", m.workspacePath)
	}

	return tea.Batch(
		tea.SetWindowTitle(windowTitle),
		func() tea.Msg {
			return listAgentsCmd{}
		},
		func() tea.Msg {
			return getModelCmd{
				modelId: m.activeAgent.Spec.ModelId,
			}
		},
		m.spinner.Tick,
	)
}

func (m *Session) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		cmds = append(cmds, m.onKeyEvent(msg))
		if m.showHelp {
			if msg.Type == tea.KeyEsc || msg.String() == "ctrl+?" {
				m.showHelp = false
				return m, nil
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.onWindowResize(msg)

	case *v1.TaskEvent:
		cmds = append(cmds, m.processTaskEvent(msg))

	// Handle API commands
	case suspendTaskCmd:
		cmds = append(cmds, m.executeSuspendTask())
	case sendMessageCmd:
		cmds = append(cmds, m.executeSendMessage(msg.content))
	case getTaskCmd:
		cmds = append(cmds, m.executeGetTask(msg.taskId))
	case getModelCmd:
		cmds = append(cmds, m.executeGetModel(msg.modelId))
	case listAgentsCmd:
		cmds = append(cmds, m.executeListAgents())
	case taskUpdatedMsg:
		// task was already updated, just trigger re-render
	}

	if !m.showHelp {
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	m.updateUsage(msg)

	messageFeed, cmd := m.messageFeed.Update(msg)
	m.messageFeed = messageFeed.(*MessageFeed)
	cmds = append(cmds, cmd)

	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Session) updateUsage(msg tea.Msg) {
	if msg, ok := msg.(*v1.Message); ok {
		if msg.Metadata.Role == v1.MessageRole_MESSAGE_ROLE_ASSISTANT && msg.Status != nil && msg.Status.Usage != nil {
			m.lastUsage.InputTokens = msg.Status.Usage.InputTokens
			m.lastUsage.OutputTokens = msg.Status.Usage.OutputTokens
			m.lastUsage.CacheWriteTokens = msg.Status.Usage.CacheWriteTokens
			m.lastUsage.CacheReadTokens = msg.Status.Usage.CacheReadTokens
		}
	}
}

func (m *Session) onKeyEvent(msg tea.KeyMsg) tea.Cmd {
	defer func() {
		// if the key is not quit, reset the last Ctrl+C time
		if !key.Matches(msg, m.keyBindings.ClearOrQuit) {
			m.lastCtrlC = time.Time{}
		}
	}()

	switch {
	case key.Matches(msg, m.keyBindings.Help):
		m.showHelp = !m.showHelp
		return nil
	case key.Matches(msg, m.keyBindings.SendMessage):
		return m.handleMessageSend()
	case key.Matches(msg, m.keyBindings.SwitchAgent):
		return m.handleSwitchAgent()
	case key.Matches(msg, m.keyBindings.SuspendTask):
		return m.handleSuspendTask()
	case key.Matches(msg, m.keyBindings.ClearOrQuit):
		return m.handleClearOrQuit()
	}

	return nil
}

func (m *Session) handleMessageSend() tea.Cmd {
	if m.input.Value() != "" {
		userInput := strings.TrimSpace(m.input.Value())
		m.input.Reset()

		m.waitingForAgent = true
		return func() tea.Msg {
			return sendMessageCmd{content: userInput}
		}
	}

	return nil
}

func (m *Session) handleSwitchAgent() tea.Cmd {
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

	// Fetch model info for the new agent
	return func() tea.Msg {
		return getModelCmd{modelId: m.activeAgent.Spec.ModelId}
	}
}

func (m *Session) handleClearOrQuit() tea.Cmd {
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

func (m *Session) handleSuspendTask() tea.Cmd {
	return func() tea.Msg {
		return suspendTaskCmd{}
	}
}

func (m *Session) processTaskEvent(msg *v1.TaskEvent) tea.Cmd {
	if msg.TaskId == m.task.Metadata.Id {
		return func() tea.Msg {
			return getTaskCmd{taskId: msg.TaskId}
		}
	}
	return nil
}

func (m *Session) executeSuspendTask() tea.Cmd {
	return func() tea.Msg {
		_, err := m.apiClient.Task().SuspendTask(m.ctx, &connect.Request[v1.SuspendTaskRequest]{
			Msg: &v1.SuspendTaskRequest{
				TaskId: m.task.Metadata.Id,
			},
		})

		return handleAPIError(err)
	}
}

func (m *Session) executeSendMessage(userInput string) tea.Cmd {
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

		return handleAPIError(err)
	}
}

func (m *Session) executeGetTask(taskId string) tea.Cmd {
	return func() tea.Msg {
		resp, err := m.apiClient.Task().GetTask(m.ctx, &connect.Request[v1.GetTaskRequest]{
			Msg: &v1.GetTaskRequest{
				Id: taskId,
			},
		})
		if err != nil {
			return handleAPIError(err)
		}

		m.task = resp.Msg.Task
		m.lastUsage.Cost = resp.Msg.Task.Status.Usage.Cost

		return taskUpdatedMsg{}
	}
}

func (m *Session) executeGetModel(modelId string) tea.Cmd {
	return func() tea.Msg {
		// Check cache first
		if cached, exists := m.modelInfoCache[modelId]; exists {
			m.currentModelInfo = cached
			return nil
		}

		resp, err := m.apiClient.Model().GetModel(m.ctx, &connect.Request[v1.GetModelRequest]{
			Msg: &v1.GetModelRequest{
				Id: modelId,
			},
		})
		if err != nil {
			return handleAPIError(err)
		}

		// Cache the result
		info := &modelInfo{
			name:          resp.Msg.Model.Spec.Name,
			contextWindow: resp.Msg.Model.Spec.ContextWindow,
		}
		m.modelInfoCache[modelId] = info
		m.currentModelInfo = info
		return nil
	}
}

func (m *Session) executeListAgents() tea.Cmd {
	return func() tea.Msg {
		agents, err := m.apiClient.Agent().ListAgents(m.ctx, &connect.Request[v1.ListAgentsRequest]{})
		if err != nil {
			return handleAPIError(err)
		}

		m.agents = agents.Msg.Agents
		return nil
	}
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

	var statusText string
	if m.task != nil {
		switch m.task.Status.Phase {
		case v1.TaskPhase_TASK_PHASE_RUNNING:
			statusText = m.spinner.View() + " " + taskStatusStyle.Render("Thinking")
		case v1.TaskPhase_TASK_PHASE_SUSPENDED:
			statusText = taskStatusStyle.Render("Suspended")
		}
	}

	agentSection := lipgloss.JoinHorizontal(lipgloss.Left,
		agentDiamondStyle.Render("» "),
		agentNameStyle.Render(agentName),
	)

	// Use cached model info
	if m.currentModelInfo != nil && m.currentModelInfo.name != "" {
		agentSection = lipgloss.JoinHorizontal(lipgloss.Left,
			agentSection,
			bulletSeparatorStyle.Render(" • "),
			agentModelStyle.Render(abbreviateModelName(m.currentModelInfo.name)),
		)
	}

	left := lipgloss.JoinHorizontal(lipgloss.Left,
		agentSection,
		bulletSeparatorStyle.Render(" • "),
		statusText,
	)

	// usage section
	usageText := ""
	tokenDisplay := fmt.Sprintf("Tokens: %d↑ %d↓", m.lastUsage.InputTokens, m.lastUsage.OutputTokens)

	if m.lastUsage.CacheReadTokens > 0 || m.lastUsage.CacheWriteTokens > 0 {
		tokenDisplay += fmt.Sprintf(" (Cache: %d↑ %d↓)", m.lastUsage.CacheReadTokens, m.lastUsage.CacheWriteTokens)
	}

	contextUsage := m.calculateContextUsage()
	if contextUsage >= 0 {
		tokenDisplay += fmt.Sprintf(" | Context: %d%%", contextUsage)
	}

	usageText = usageStyle.Render(fmt.Sprintf("%s | Cost: $%.2f", tokenDisplay, m.lastUsage.Cost))

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

func (m *Session) calculateContextUsage() int {
	if m.activeAgent == nil || m.currentModelInfo == nil {
		return -1
	}

	if m.currentModelInfo.contextWindow <= 0 {
		slog.Error("invalid context window size", "contextWindowSize", m.currentModelInfo.contextWindow)
		return -1
	}

	totalTokens := m.lastUsage.InputTokens + m.lastUsage.OutputTokens + m.lastUsage.CacheReadTokens + m.lastUsage.CacheWriteTokens
	percentage := int((float64(totalTokens) / float64(m.currentModelInfo.contextWindow)) * 100)
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

func handleAPIError(err error) *Error {
	if err == nil {
		return nil
	}

	cause := fail.HandleError(nil, err)
	if cause == nil {
		return nil
	}

	return NewError(cause)
}
