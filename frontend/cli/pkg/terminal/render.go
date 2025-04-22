package terminal

import (
	"context"
	"log/slog"
	"slices"

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
	activeAgent string
	agents      []string

	eventChannel chan *v1.SubscribeResponse
	ctx          context.Context
}

func NewModel(ctx context.Context, apiClient *api_client.Client, task *v1.Task, agent *v1.Agent) *model {
	ta := textarea.New()
	ta.Focus()
	ta.CharLimit = 32768
	// ti.Prompt = fmt.Sprintf("[%s] > ", agent.Metadata.Name)
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	// ta.SetWidth(80)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.Prompt = ""

	vp := viewport.New(80, 20)
	vp.SetContent("")

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return &model{
		width:        80,
		height:       20,
		input:        ta,
		viewport:     vp,
		spinner:      sp,
		apiClient:    apiClient,
		messages:     []message{},
		activeAgent:  agent.Metadata.Name,
		task:         task,
		eventChannel: make(chan *v1.SubscribeResponse, 100),
		ctx:          ctx,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		eventSubscriber(m.ctx, m.apiClient, m.eventChannel, m.task.Id),
		eventBridge(m.eventChannel),
	)
}

func eventSubscriber(ctx context.Context, client *api_client.Client, eventChannel chan<- *v1.SubscribeResponse, taskId string) tea.Cmd {
	return func() tea.Msg {
		sub, err := client.Message().Subscribe(ctx, &connect.Request[v1.SubscribeRequest]{
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
		msg := <-eventChannel
		switch msg.GetEvent().(type) {
		case *v1.SubscribeResponse_MessageEvent:
			slog.Info("received message event", "message", msg.GetMessageEvent())
			return msg.GetMessageEvent()
		default:
			slog.Error("unknown event", "event", msg.GetEvent())
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
		m.messages = append(m.messages, &userMessage{content: msg.Content.GetText()})
		m.updateViewportContent()
		slog.Info("updated viewport content")
	}

	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) onKeyPressed(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyTab:
		return m.onToogleAgent()
	case tea.KeyEnter:
		return m.onTextInput(msg)
	}

	switch msg.String() {
	case "k", "up":
		m.viewport.LineUp(1)
	case "j", "down":
		m.viewport.LineDown(1)
	case "b", "pageup":
		m.viewport.HalfViewUp()
	case "f", "pagedown":
		m.viewport.HalfViewDown()
	case "home":
		m.viewport.GotoTop()
	case "end":
		m.viewport.GotoBottom()
	}

	return nil
}

func (m *model) onTextInput(msg tea.KeyMsg) tea.Cmd {
	if m.input.Value() != "" {
		userInput := m.input.Value()
		m.input.Reset()

		_, err := m.apiClient.Message().CreateMessage(context.Background(), &connect.Request[v1.CreateMessageRequest]{
			Msg: &v1.CreateMessageRequest{
				TaskId:  m.task.Id,
				Content: userInput,
			},
		})
		if err != nil {
		}
	}

	return nil
}

func (m *model) onToogleAgent() tea.Cmd {
	if len(m.agents) == 0 {
		return nil
	}
	idx := slices.Index(m.agents, m.activeAgent)
	if idx == -1 {
		idx = 0
	} else {
		idx = (idx + 1) % len(m.agents)
	}

	m.activeAgent = m.agents[idx]
	return nil
}

func (m *model) onWindowResize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height
}

func (m *model) View() string {
	m.input.SetWidth(m.width - 6)
	if len(m.input.Value()) > m.width-6 {
		m.input.SetHeight(2)
	}
	textInput := inputStyle.Render(m.input.View())

	footer := footerStyle.Render("\nTab: switch agent| PgUp/PgDown: scroll | Ctrl+C: quit")

	m.viewport.Width = Max(5, m.width-6)
	m.viewport.Height = Max(5, m.height-4-lipgloss.Height(m.input.View()) - lipgloss.Height(footer))
	viewport := viewportStyle.Render(m.viewport.View())
	slog.Info("viewport", "viewport", viewport)
	
	return appStyle.Render(lipgloss.JoinVertical(lipgloss.Center,
		viewport,
		textInput),
		footer,
	)
}

func (m *model) updateViewportContent() {
	m.viewport.SetContent(m.formatMessages())
	m.viewport.GotoBottom()
}
