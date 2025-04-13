package terminal

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"connectrpc.com/connect"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	api_client "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
)

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		func() tea.Msg {
			return tea.WindowSizeMsg{
				Width:  80,
				Height: 24,
			}
		},
		eventSubscriber(m.ctx, m.apiClient, m.eventChannel),
		eventBridge(m.eventChannel),
	)
}

func eventSubscriber(ctx context.Context, client *api_client.Client, eventChannel chan<- *v1.SubscribeResponse) tea.Cmd {
	return func() tea.Msg {
		sub, err := client.Message().Subscribe(ctx, &connect.Request[v1.SubscribeRequest]{})
		if err != nil {
			slog.Error("failed to subscribe to messages", "error", err)
			return nil
		}
		for sub.Receive() {
			eventChannel <- sub.Msg()
		}

		return nil
	}
}

func eventBridge(eventChannel <-chan *v1.SubscribeResponse) tea.Cmd {
	return func() tea.Msg {
		msg := <-eventChannel
		switch msg.GetEvent().(type) {
		case *v1.SubscribeResponse_MessageEvent:
			return msg.GetMessageEvent()
		}

		return nil
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC | tea.KeyEsc:
			return m, tea.Quit
		default:
			cmds = append(cmds, m.handleKeyEvents(msg))
		}
	case tea.WindowSizeMsg:
		cmds = append(cmds, m.handleWindowResizeEvent(msg))

	case *v1.Message:
		m.messages = append(m.messages, &userMessage{content: msg.Content.GetText()})
	}

	m.textInput, cmd = m.textInput.Update(msg)
	cmds = append(cmds, cmd)

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) handleKeyEvents(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyTab:
		return m.handleToggleAgentsEvent()
	case tea.KeyEnter:
		return m.handleInputEvents(msg)
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

func (m model) handleInputEvents(msg tea.KeyMsg) tea.Cmd {
	if m.textInput.Value() != "" {
		userInput := m.textInput.Value()
		m.textInput.Reset()

		m.apiClient.Message().CreateMessage(context.Background(), &connect.Request[v1.CreateMessageRequest]{
			Msg: &v1.CreateMessageRequest{
				Content: userInput,
			},
		})
	}

	return nil
}

func (m model) handleToggleAgentsEvent() tea.Cmd {
	idx := slices.Index(m.agents, m.activeAgent)
	if idx == -1 {
		idx = 0
	}

	if idx == len(m.agents)-1 {
		idx = 0
	} else {
		idx++
	}

	m.activeAgent = m.agents[idx]
	return nil
}

func (m model) handleWindowResizeEvent(msg tea.WindowSizeMsg) tea.Cmd {
	m.width = msg.Width
	m.height = msg.Height

	// Adjust viewport and input field sizes
	m.viewport.Width = msg.Width - 4   // Account for padding
	m.viewport.Height = msg.Height - 6 // Leave room for input
	m.textInput.Width = msg.Width - 4

	// Update the separator width
	separatorStyle = separatorStyle.Width(msg.Width - 4)

	// Reformat content for new dimensions
	m.updateViewportContent()
	return nil
}

func (m model) View() string {
	var sb strings.Builder

	if m.height == 0 {
		return "Loading..."
	}
	viewportHeight := m.height - 6

	if m.viewport.Height != viewportHeight {
		m.viewport.Height = viewportHeight
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Render("Construct")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	sb.WriteString(m.viewport.View())
	sb.WriteString("\n")

	// if m.waiting {
	// 	sb.WriteString(waitingStyle.Render("Construct is thinking..."))
	// 	sb.WriteString("\n")
	// }

	separator := separatorStyle.Width(m.width - 4).String()
	sb.WriteString(separator)
	sb.WriteString("\n")

	inputLabel := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(fmt.Sprintf("[%s] ", m.activeAgent))
	sb.WriteString(inputLabel)
	sb.WriteString(inputStyle.Render(m.textInput.View()))

	footer := "\nTab: enter plan mode | PgUp/PgDown: scroll | Ctrl+C: quit"
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(footer))

	return appStyle.Render(sb.String())
}

func (m *model) updateViewportContent() {
	m.viewport.SetContent(m.formatMessages())
	m.viewport.GotoBottom()
}
