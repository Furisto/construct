package terminal

import (
	"fmt"
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
	return &MessageFeed{
		// renderCache: make(map[string]renderCacheItem),
	}
}

func (m *MessageFeed) Init() tea.Cmd {
	return nil
}

func (m *MessageFeed) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, messageKeys.PageUp) || key.Matches(msg, messageKeys.PageDown) {
			u, cmd := m.viewport.Update(msg)
			m.viewport = u
			cmds = append(cmds, cmd)
		}
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

	result := lipgloss.NewStyle().Width(m.width).Render(lipgloss.JoinVertical(
		lipgloss.Top,
		m.viewport.View(),
	))

	return result
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

func formatMessages(messages []message, partialMessage string, width int) string {
	renderedMessages := []string{}
	for i, msg := range messages {
		switch msg := msg.(type) {
		case *userTextMessage:
			renderedMessages = append(renderedMessages, renderUserMessage(msg, width, addBottomMargin(i, messages)))

		case *assistantTextMessage:
			renderedMessages = append(renderedMessages, renderAssistantMessage(msg, width, addBottomMargin(i, messages)))

		case *readFileToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Read", msg.Input.Path, width, addBottomMargin(i, messages)))

		case *createFileToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Create", msg.Input.Path, width, addBottomMargin(i, messages)))

		case *editFileToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Edit", msg.Input.Path, width, addBottomMargin(i, messages)))

		case *executeCommandToolCall:
			command := msg.Input.Command
			if len(command) > 50 {
				command = command[:47] + "..."
			}
			renderedMessages = append(renderedMessages, renderToolCallMessage("Execute", command, width, addBottomMargin(i, messages)))

		case *findFileToolCall:
			pathInfo := msg.Input.Path
			if pathInfo == "" {
				pathInfo = "."
			}

			if len(pathInfo) > 50 {
				start := Max(0, len(pathInfo)-50)
				pathInfo = pathInfo[start:] + "..."
			}

			excludeArg := msg.Input.ExcludePattern
			if len(excludeArg) > 50 {
				excludeArg = excludeArg[:47] + "..."
			}
			if excludeArg == "" {
				excludeArg = "none"
			}

			renderedMessages = append(renderedMessages,
				renderToolCallMessage("Find", fmt.Sprintf("%s(pattern: %s, path: %s, exclude: %s)", boldStyle.Render("Find"), msg.Input.Pattern, pathInfo, excludeArg), width, addBottomMargin(i, messages)))

		case *grepToolCall:
			searchInfo := msg.Input.Query
			if msg.Input.IncludePattern != "" {
				searchInfo = fmt.Sprintf("%s in %s", searchInfo, msg.Input.IncludePattern)
			}
			renderedMessages = append(renderedMessages, renderToolCallMessage("Grep", searchInfo, width, addBottomMargin(i, messages)))

		case *handoffToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Handoff", msg.Input.RequestedAgent, width, addBottomMargin(i, messages)))

		case *listFilesToolCall:
			pathInfo := msg.Input.Path
			if pathInfo == "" {
				pathInfo = "."
			}
			listType := "List"
			if msg.Input.Recursive {
				listType = "List -R"
			}
			renderedMessages = append(renderedMessages, renderToolCallMessage(listType, pathInfo, width, addBottomMargin(i, messages)))

		case *codeInterpreterToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Interpreter", "Script", width, addBottomMargin(i, messages)))
			renderedMessages = append(renderedMessages, formatCodeInterpreterContent(msg.Input.Code))

		case *codeInterpreterResult:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Interpreter", "Output", width, addBottomMargin(i, messages)))
			renderedMessages = append(renderedMessages, formatCodeInterpreterContent(msg.Result.Output))

		case *errorMessage:
			renderedMessages = append(renderedMessages, errorStyle.Render("❌ Error: ")+msg.content)
		}
	}

	if partialMessage != "" {
		renderedMessages = append(renderedMessages, renderAssistantMessage(&assistantTextMessage{content: partialMessage}, width, false))
	}

	return lipgloss.JoinVertical(
		lipgloss.Top,
		renderedMessages...,
	)
}
