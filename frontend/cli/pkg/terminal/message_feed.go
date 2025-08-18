package terminal

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	v1 "github.com/furisto/construct/api/go/v1"
)

type MessageFeedKeyMap struct {
	PageUp   key.Binding
	PageDown key.Binding
}

var (
	messageKeys = MessageFeedKeyMap{
		PageUp:   key.NewBinding(key.WithKeys("ctrl+p")),
		PageDown: key.NewBinding(key.WithKeys("ctrl+n")),
	}
)

type MessageFeed struct {
	width          int
	height         int
	viewport       viewport.Model
	messages       []message
	partialMessage string
}

var _ tea.Model = (*MessageFeed)(nil)

func NewMessageFeed() *MessageFeed {
	return &MessageFeed{}
}

func (m *MessageFeed) Init() tea.Cmd {
	return nil
}

func (m *MessageFeed) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	// case tea.KeyMsg:
	// 	if key.Matches(msg, messageKeys.PageUp) || key.Matches(msg, messageKeys.PageDown) {
	// 		u, cmd := m.viewport.Update(msg)
	// 		m.viewport = u
	// 		cmds = append(cmds, cmd)
	// 	}
	case *v1.Message:
		m.processMessage(msg)
		m.updateViewportContent()
	}

	u, cmd := m.viewport.Update(msg)
	m.viewport = u
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *MessageFeed) View() string {
	if len(m.messages) == 0 {
		return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(
			lipgloss.JoinVertical(
				lipgloss.Top,
				m.renderInitialMessage(),
			),
		)
	}

	return lipgloss.NewStyle().Width(m.width).Render(lipgloss.JoinVertical(
		lipgloss.Top,
		m.viewport.View(),
	))
}

func (m *MessageFeed) SetSize(width, height int) tea.Cmd {
	m.width = width
	m.height = height

	m.viewport.Width = width
	m.viewport.Height = height

	return nil
}

func (m *MessageFeed) renderInitialMessage() string {
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

func (m *MessageFeed) updateViewportContent() {
	formatted := formatMessages(m.messages, m.partialMessage, m.viewport.Width)
	m.viewport.SetContent(formatted)

	m.viewport.GotoBottom()
}

func (m *MessageFeed) processMessage(msg *v1.Message) {
	for _, part := range msg.Spec.Content {
		switch data := part.Data.(type) {
		case *v1.MessagePart_Text_:
			if msg.Status.ContentState == v1.ContentStatus_CONTENT_STATUS_PARTIAL {
				m.partialMessage += data.Text.Content
			} else {
				if msg.Metadata.Role == v1.MessageRole_MESSAGE_ROLE_ASSISTANT {
					m.messages = append(m.messages, &assistantTextMessage{
						content:   data.Text.Content,
						timestamp: msg.Metadata.CreatedAt.AsTime(),
					})
				} else {
					m.messages = append(m.messages, &userTextMessage{
						content:   data.Text.Content,
						timestamp: msg.Metadata.CreatedAt.AsTime(),
					})
				}
				m.partialMessage = ""
			}
		case *v1.MessagePart_ToolCall:
			m.messages = append(m.messages, m.createToolCallMessage(data.ToolCall, msg.Metadata.CreatedAt.AsTime()))
		case *v1.MessagePart_ToolResult:
			m.messages = append(m.messages, m.createToolResultMessage(data.ToolResult, msg.Metadata.CreatedAt.AsTime()))
		case *v1.MessagePart_Error_:
			m.messages = append(m.messages, &errorMessage{
				content:   data.Error.Message,
				timestamp: msg.Metadata.CreatedAt.AsTime(),
			})
		}
	}
}

func (m *MessageFeed) createToolCallMessage(toolCall *v1.ToolCall, timestamp time.Time) message {
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
		// case *v1.ToolCall_CodeInterpreter:
		// 	if m.Verbose {
		// 		return &codeInterpreterToolCall{
		// 			ID:        toolCall.Id,
		// 			Input:     toolInput.CodeInterpreter,
		// 			timestamp: timestamp,
		// 		}
		// 	}
	}

	return nil
}

func (m *MessageFeed) createToolResultMessage(toolResult *v1.ToolResult, timestamp time.Time) message {
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
		// case *v1.ToolResult_CodeInterpreter:
		// 	if m.Verbose {
		// 		return &codeInterpreterResult{
		// 			ID:        toolResult.Id,
		// 			Result:    toolOutput.CodeInterpreter,
		// 			timestamp: timestamp,
		// 		}
		// 	}
	}

	return nil
}
