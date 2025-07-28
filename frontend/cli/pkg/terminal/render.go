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
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
)

type model struct {
	viewport viewport.Model
	input    textarea.Model
	spinner  spinner.Model

	width  int
	height int

	apiClient      *api_client.Client
	messages       []message
	partialMessage string
	task           *v1.Task
	activeAgent    *v1.Agent
	agents         []*v1.Agent

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

func NewModel(ctx context.Context, apiClient *api_client.Client, task *v1.Task, agent *v1.Agent) *model {
	ta := textarea.New()
	ta.Focus()
	ta.CharLimit = 32768
	ta.ShowLineNumbers = false
	ta.SetHeight(4)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.Prompt = ""
	ta.Placeholder = "Type your message..."

	vp := viewport.New(80, 20)

	// Set initial welcome message
	welcomeMessage := renderWelcomeMessage()
	vp.SetContent(welcomeMessage)

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

	return &model{
		width:           80,
		height:          20,
		input:           ta,
		viewport:        vp,
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

func (m model) Init() tea.Cmd {
	windowTitle := "construct"
	if m.workspacePath != "" {
		windowTitle = fmt.Sprintf("construct (%s)", m.workspacePath)
	}

	return tea.Batch(
		tea.EnterAltScreen,
		tea.SetWindowTitle(windowTitle),
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			return m, tea.Quit
		default:
			cmds = append(cmds, m.onKeyPressed(msg))
		}

	case tea.WindowSizeMsg:
		m.onWindowResize(msg)

	case *v1.Message:
		// slog.Info("processing message", "message", msg)
		m.processMessage(msg)
		m.updateViewportContent()
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

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) onKeyPressed(msg tea.KeyMsg) tea.Cmd {
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
	case "k", "up":
		if m.mode == ModeScroll {
			m.viewport.LineUp(1)
		}
	case "j", "down":
		if m.mode == ModeScroll {
			m.viewport.LineDown(1)
		}
	case "b", "pageup":
		m.viewport.HalfViewUp()
	case "f", "pagedown":
		m.viewport.HalfViewDown()
	case "home":
		m.viewport.GotoTop()
	case "end":
		m.viewport.GotoBottom()
	case "ctrl+?":
		m.showHelp = !m.showHelp
		return nil
	}

	return nil
}

func (m *model) handleCtrlC() tea.Cmd {
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

func (m *model) onMessageSend(_ tea.KeyMsg) tea.Cmd {
	if m.input.Value() != "" {
		userInput := strings.TrimSpace(m.input.Value())
		m.input.Reset()

		m.messages = append(m.messages, &userMessage{
			content:   userInput,
			timestamp: time.Now(),
		})
		m.updateViewportContent()
		m.waitingForAgent = true

		return m.sendMessage(userInput)
	}

	return nil
}

func (m *model) sendMessage(userInput string) tea.Cmd {
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
			m.updateViewportContent()
			m.waitingForAgent = false

			return &errorMessage{
				content:   fmt.Sprintf("Error sending message: %v", err),
				timestamp: time.Now(),
			}
		}
		return nil
	}
}

func (m *model) processMessage(msg *v1.Message) {
	if msg.Metadata.Role == v1.MessageRole_MESSAGE_ROLE_ASSISTANT || msg.Metadata.Role == v1.MessageRole_MESSAGE_ROLE_SYSTEM {
		m.waitingForAgent = false

		for _, part := range msg.Spec.Content {
			switch data := part.Data.(type) {
			case *v1.MessagePart_Text_:
				if msg.Status.ContentState == v1.ContentStatus_CONTENT_STATUS_PARTIAL {
					m.partialMessage += data.Text.Content
				} else if msg.Status.ContentState == v1.ContentStatus_CONTENT_STATUS_COMPLETE {
					m.messages = append(m.messages, &assistantTextMessage{
						content:   data.Text.Content,
						timestamp: msg.Metadata.CreatedAt.AsTime(),
					})
					m.partialMessage = ""
				}
			case *v1.MessagePart_ToolCall:
				m.messages = append(m.messages, m.createToolCallMessage(data.ToolCall, msg.Metadata.CreatedAt.AsTime()))
			case *v1.MessagePart_ToolResult:
				m.messages = append(m.messages, m.createToolResultMessage(data.ToolResult, msg.Metadata.CreatedAt.AsTime()))
			}
		}
	}

	if msg.Status != nil && msg.Status.Usage != nil {
		m.lastUsage = &v1.TaskUsage{
			InputTokens:      msg.Status.Usage.InputTokens,
			OutputTokens:     msg.Status.Usage.OutputTokens,
			CacheWriteTokens: msg.Status.Usage.CacheWriteTokens,
			CacheReadTokens:  msg.Status.Usage.CacheReadTokens,
			Cost:             msg.Status.Usage.Cost,
		}
	}
}

func (m *model) onToggleAgent() tea.Cmd {
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

func (m *model) onWindowResize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height

	// Update component sizes. Subtract 6 (2 margin, 2 border, 2 padding) so
	// the textarea including its own border fits perfectly inside the
	// outer appStyle margins.
	m.input.SetWidth(Max(5, Min(m.width-6, 115)))
	m.viewport.Width = Max(5, Min(m.width-6, 115))
}

func (m *model) View() string {
	if m.showHelp {
		return m.renderHelp()
	}

	header := m.renderHeader()

	// Calculate dimensions
	headerHeight := lipgloss.Height(header)
	inputHeight := 5 // Fixed height for input area (4 lines + border)

	m.input.SetWidth(Max(5, Min(m.width-6, 115)))
	textInput := m.input.View()

	m.viewport.Width = Max(5, Min(m.width-6, 115))
	m.viewport.Height = Max(5, m.height-headerHeight-inputHeight-4)
	viewport := viewportStyle.Render(m.viewport.View())

	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		header,
		viewport,
		textInput,
	))
}

func (m *model) renderHeader() string {
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

func (m *model) updateViewportContent() {
	formatted := m.formatMessages()
	m.viewport.SetContent(formatted)

	if m.Verbose {
		f, _ := os.CreateTemp("", "construct-cli-messages.md")
		f.WriteString(formatted)
		f.Close()
	}
	m.viewport.GotoBottom()
}

func (m *model) getAgentModelInfo(agent *v1.Agent) (string, int64, error) {
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

func (m *model) calculateContextUsage(contextWindowSize int64) int {
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

func renderWelcomeMessage() string {
	separator := separatorStyle.Render()

	welcomeLines := []string{
		separator,
		"Welcome! Type your message below.",
		"Press Ctrl + ? for help at any time.",
		"Press Ctrl + C to exit.",
		separator,
		"",
	}

	return strings.Join(welcomeLines, "\n")
}

func (m *model) createToolCallMessage(toolCall *v1.ToolCall, timestamp time.Time) message {
	switch toolInput := toolCall.Input.(type) {
	case *v1.ToolCall_EditFile:
		return &editFileToolCall{
			ID:        toolCall.Id,
			Input:     toolInput.EditFile,
			timestamp: timestamp,
		}
	case *v1.ToolCall_CreateFile:
		return &createFileToolCall{
			ID:        toolCall.Id,
			Input:     toolInput.CreateFile,
			timestamp: timestamp,
		}
	case *v1.ToolCall_ExecuteCommand:
		return &executeCommandToolCall{
			ID:        toolCall.Id,
			Input:     toolInput.ExecuteCommand,
			timestamp: timestamp,
		}
	case *v1.ToolCall_FindFile:
		return &findFileToolCall{
			ID:        toolCall.Id,
			Input:     toolInput.FindFile,
			timestamp: timestamp,
		}
	case *v1.ToolCall_Grep:
		return &grepToolCall{
			ID:        toolCall.Id,
			Input:     toolInput.Grep,
			timestamp: timestamp,
		}
	case *v1.ToolCall_Handoff:
		return &handoffToolCall{
			ID:        toolCall.Id,
			Input:     toolInput.Handoff,
			timestamp: timestamp,
		}
	case *v1.ToolCall_AskUser:
		return &askUserToolCall{
			ID:        toolCall.Id,
			Input:     toolInput.AskUser,
			timestamp: timestamp,
		}
	case *v1.ToolCall_ListFiles:
		return &listFilesToolCall{
			ID:        toolCall.Id,
			Input:     toolInput.ListFiles,
			timestamp: timestamp,
		}
	case *v1.ToolCall_ReadFile:
		return &readFileToolCall{
			ID:        toolCall.Id,
			Input:     toolInput.ReadFile,
			timestamp: timestamp,
		}
	case *v1.ToolCall_SubmitReport:
		return &submitReportToolCall{
			ID:        toolCall.Id,
			Input:     toolInput.SubmitReport,
			timestamp: timestamp,
		}
	case *v1.ToolCall_CodeInterpreter:
		if m.Verbose {
			return &codeInterpreterToolCall{
				ID:        toolCall.Id,
				Input:     toolInput.CodeInterpreter,
				timestamp: timestamp,
			}
		}
	}

	return nil
}

func (m *model) createToolResultMessage(toolResult *v1.ToolResult, timestamp time.Time) message {
	switch toolOutput := toolResult.Result.(type) {
	case *v1.ToolResult_CreateFile:
		return &createFileResult{
			ID:        toolResult.Id,
			Result:    toolOutput.CreateFile,
			timestamp: timestamp,
		}
	case *v1.ToolResult_EditFile:
		return &editFileResult{
			ID:        toolResult.Id,
			Result:    toolOutput.EditFile,
			timestamp: timestamp,
		}
	case *v1.ToolResult_ExecuteCommand:
		return &executeCommandResult{
			ID:        toolResult.Id,
			Result:    toolOutput.ExecuteCommand,
			timestamp: timestamp,
		}
	case *v1.ToolResult_FindFile:
		return &findFileResult{
			ID:        toolResult.Id,
			Result:    toolOutput.FindFile,
			timestamp: timestamp,
		}
	case *v1.ToolResult_Grep:
		return &grepResult{
			ID:        toolResult.Id,
			Result:    toolOutput.Grep,
			timestamp: timestamp,
		}
	case *v1.ToolResult_ListFiles:
		return &listFilesResult{
			ID:        toolResult.Id,
			Result:    toolOutput.ListFiles,
			timestamp: timestamp,
		}
	case *v1.ToolResult_ReadFile:
		return &readFileResult{
			ID:        toolResult.Id,
			Result:    toolOutput.ReadFile,
			timestamp: timestamp,
		}
	case *v1.ToolResult_SubmitReport:
		return &submitReportResult{
			ID:        toolResult.Id,
			Result:    toolOutput.SubmitReport,
			timestamp: timestamp,
		}
	case *v1.ToolResult_CodeInterpreter:
		if m.Verbose {
			return &codeInterpreterResult{
				ID:        toolResult.Id,
				Result:    toolOutput.CodeInterpreter,
				timestamp: timestamp,
			}
		}
	}

	return nil
}
