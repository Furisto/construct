package terminal

import (
	"context"
	"fmt"
	"log/slog"
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

	apiClient   *api_client.Client
	messages    []message
	task        *v1.Task
	activeAgent *v1.Agent
	agents      []*v1.Agent

	eventChannel chan *v1.SubscribeResponse
	ctx          context.Context

	// UI state
	state     appState
	mode      uiMode
	showHelp  bool
	typing    bool
	lastUsage *v1.TaskUsage
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
	vp.SetContent("")

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	return &model{
		width:        80,
		height:       20,
		input:        ta,
		viewport:     vp,
		spinner:      sp,
		apiClient:    apiClient,
		messages:     []message{},
		activeAgent:  agent,
		agents:       []*v1.Agent{agent},
		task:         task,
		eventChannel: make(chan *v1.SubscribeResponse, 100),
		ctx:          ctx,
		state:        StateNormal,
		mode:         ModeInput,
		showHelp:     false,
		typing:       false,
		lastUsage:    task.Status.Usage,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		eventSubscriber(m.ctx, m.apiClient, m.eventChannel, m.task.Metadata.Id),
		eventBridge(m.eventChannel),
		tea.Every(time.Millisecond*100, func(t time.Time) tea.Msg {
			return eventBridge(m.eventChannel)()
		}),
	)
}

func eventSubscriber(ctx context.Context, client *api_client.Client, eventChannel chan<- *v1.SubscribeResponse, taskId string) tea.Cmd {
	return func() tea.Msg {
		sub, err := client.Task().Subscribe(ctx, &connect.Request[v1.SubscribeRequest]{
			Msg: &v1.SubscribeRequest{
				TaskId: taskId,
			},
		})
		slog.Info("subscribed to task", "task_id", taskId)
		if err != nil {
			slog.Error("failed to subscribe to task", "error", err)
			return nil
		}
		slog.Info("receiving messages", "task_id", taskId)
		for sub.Receive() {
			slog.Info("received message", "message", sub.Msg())
			eventChannel <- sub.Msg()
		}

		if err := sub.Err(); err != nil {
			slog.Error("failed to receive messages", "error", err)
		}

		return nil
	}
}

func eventBridge(eventChannel <-chan *v1.SubscribeResponse) tea.Cmd {
	return func() tea.Msg {
		select {
		case event := <-eventChannel:
			if event != nil && event.Message != nil {
				return event.Message
			}
		default:
		}
		return nil
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.showHelp {
			if msg.Type == tea.KeyEsc || msg.String() == "h" || msg.String() == "H" {
				m.showHelp = false
				return m, nil
			}
			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			fallthrough
		case tea.KeyEsc:
			return m, tea.Quit
		default:
			cmds = append(cmds, m.onKeyPressed(msg))
		}

	case tea.WindowSizeMsg:
		m.onWindowResize(msg)

	case *v1.Message:
		slog.Info("processing message", "message", msg)
		m.processMessage(msg)
		m.updateViewportContent()
		slog.Info("updated viewport content")
		// Continue polling for more messages
		cmds = append(cmds, eventBridge(m.eventChannel))
	}

	if !m.showHelp {
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) onKeyPressed(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyTab:
		return m.onToggleAgent()
	case tea.KeyEnter:
		if m.mode == ModeInput {
			return m.onTextInput(msg)
		}
	case tea.KeyCtrlH:
		m.showHelp = !m.showHelp
		return nil
	case tea.KeyCtrlL:
		m.messages = []message{}
		m.updateViewportContent()
		return nil
	case tea.KeyCtrlR:
		return m.onReconnect()
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
	case "h", "H":
		m.showHelp = !m.showHelp
		return nil
	}

	return nil
}

func (m *model) onTextInput(_ tea.KeyMsg) tea.Cmd {
	if m.input.Value() != "" {
		userInput := strings.TrimSpace(m.input.Value())
		m.input.Reset()

		// Add user message to display immediately
		m.messages = append(m.messages, &userMessage{
			content:   userInput,
			timestamp: time.Now(),
		})
		m.updateViewportContent()
		m.typing = true

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
			m.messages = append(m.messages, &errorMessage{
				content:   fmt.Sprintf("Error sending message: %v", err),
				timestamp: time.Now(),
			})
			m.updateViewportContent()
			m.typing = false
		}
	}

	return nil
}

func (m *model) processMessage(msg *v1.Message) {
	if msg.Metadata.Role == v1.MessageRole_MESSAGE_ROLE_ASSISTANT {
		m.typing = false

		for _, part := range msg.Spec.Content {
			switch data := part.Data.(type) {
			case *v1.MessagePart_Text_:
				m.messages = append(m.messages, &assistantTextMessage{
					content:   data.Text.Content,
					timestamp: msg.Metadata.CreatedAt.AsTime(),
				})
			case *v1.MessagePart_ToolResult_:
				m.messages = append(m.messages, &assistantToolMessage{
					toolName:  data.ToolResult.ToolName,
					arguments: data.ToolResult.Arguments,
					result:    data.ToolResult.Result,
					error:     data.ToolResult.Error,
					timestamp: msg.Metadata.CreatedAt.AsTime(),
				})
			case *v1.MessagePart_SubmitReport_:
				m.messages = append(m.messages, &submitReportMessage{
					summary:      data.SubmitReport.Summary,
					completed:    data.SubmitReport.Completed,
					deliverables: data.SubmitReport.Deliverables,
					nextSteps:    data.SubmitReport.NextSteps,
					timestamp:    msg.Metadata.CreatedAt.AsTime(),
				})
			}
		}
	}

	// Update usage info if available
	if msg.Status != nil && msg.Status.Usage != nil {
		// Convert MessageUsage to TaskUsage for display
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

func (m *model) onReconnect() tea.Cmd {
	return tea.Batch(
		eventSubscriber(m.ctx, m.apiClient, m.eventChannel, m.task.Metadata.Id),
		eventBridge(m.eventChannel),
	)
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
	agentName := "Unknown"
	if m.activeAgent != nil {
		agentName = m.activeAgent.Spec.Name
	}

	taskStatus := "Unknown"
	if m.task != nil {
		switch m.task.Status.Phase {
		case v1.TaskPhase_TASK_PHASE_AWAITING:
			taskStatus = "Awaiting"
		case v1.TaskPhase_TASK_PHASE_RUNNING:
			taskStatus = "Running"
		case v1.TaskPhase_TASK_PHASE_SUSPENDED:
			taskStatus = "Suspended"
		}
	}

	statusText := ""
	if m.typing {
		statusText = m.spinner.View() + " Agent is thinking..."
	} else {
		statusText = taskStatusStyle.Render(taskStatus)
	}

	usageText := ""
	if m.lastUsage != nil {
		usageText = usageStyle.Render(fmt.Sprintf("Tokens: %d/%d | Cost: $%.4f",
			m.lastUsage.InputTokens, m.lastUsage.OutputTokens, m.lastUsage.Cost))
	}

	// Extract model info from agent if available
	modelInfo := ""
	if m.activeAgent != nil && m.activeAgent.Spec.ModelId != "" {
		// Extract meaningful model name from model ID
		modelName := m.extractModelName(m.activeAgent.Spec.ModelId)
		if modelName != "" {
			modelInfo = agentModelStyle.Render(modelName)
		}
	}

	// Build agent section with diamond symbol
	agentSection := lipgloss.JoinHorizontal(lipgloss.Left,
		agentDiamondStyle.Render("» "),
		agentNameStyle.Render(agentName),
	)

	// Add model info if available
	if modelInfo != "" {
		agentSection = lipgloss.JoinHorizontal(lipgloss.Left,
			agentSection,
			bulletSeparatorStyle.Render(" • "),
			modelInfo,
		)
	}

	left := lipgloss.JoinHorizontal(lipgloss.Left,
		agentSection,
		bulletSeparatorStyle.Render(" • "),
		statusText,
	)

	headerContent := lipgloss.JoinHorizontal(lipgloss.Left,
		left,
		strings.Repeat(" ", Max(0, m.width-lipgloss.Width(left)-lipgloss.Width(usageText)-4)),
		usageText,
	)

	return headerStyle.Render(headerContent)
}

func (m *model) updateViewportContent() {
	m.viewport.SetContent(m.formatMessages())
	m.viewport.GotoBottom()
}

func (m *model) extractModelName(modelId string) string {
	// Map common model IDs to display names
	// This is a simplified approach - could be enhanced with actual model resolution
	switch {
	case strings.Contains(strings.ToLower(modelId), "gpt-4"):
		return "GPT-4"
	case strings.Contains(strings.ToLower(modelId), "gpt-3.5"):
		return "GPT-3.5"
	case strings.Contains(strings.ToLower(modelId), "claude"):
		return "Claude"
	case strings.Contains(strings.ToLower(modelId), "sonnet"):
		return "Claude Sonnet"
	case strings.Contains(strings.ToLower(modelId), "haiku"):
		return "Claude Haiku"
	case strings.Contains(strings.ToLower(modelId), "opus"):
		return "Claude Opus"
	default:
		// If no recognizable pattern, show first 8 chars of model ID
		if len(modelId) > 8 {
			return modelId[:8] + "..."
		}
		return modelId
	}
}
