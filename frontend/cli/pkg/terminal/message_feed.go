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
	"github.com/furisto/construct/frontend/cli/pkg/fail"
)

type MessageFeedKeybindings struct {
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	Down         key.Binding
	Up           key.Binding
}

func NewMessageFeedKeybindings() MessageFeedKeybindings {
	return MessageFeedKeybindings{
		HalfPageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "½ page up"),
		),
		HalfPageDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "½ page down"),
		),
		Up: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "down"),
		),
	}
}

type MessageFeed struct {
	width            int
	height           int
	viewport         viewport.Model
	messages         []message
	partialMessage   string
	keyBindings      MessageFeedKeybindings
	userIsScrolledUp bool

	subtaskOutputs map[string]*SubtaskOutput
}

var _ tea.Model = (*MessageFeed)(nil)

func NewMessageFeed() *MessageFeed {
	return &MessageFeed{
		viewport:       viewport.New(0, 0),
		keyBindings:    NewMessageFeedKeybindings(),
		subtaskOutputs: make(map[string]*SubtaskOutput),
	}
}

func (m *MessageFeed) Init() tea.Cmd {
	return m.scheduleCleanupTick()
}

func (m *MessageFeed) scheduleCleanupTick() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return subtaskCleanupTickMsg{}
	})
}

func (m *MessageFeed) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, m.keyBindings.Up) || key.Matches(msg, m.keyBindings.HalfPageUp) {
			m.userIsScrolledUp = true
		}

		switch {
		case key.Matches(msg, m.keyBindings.HalfPageUp):
			m.viewport.HalfViewUp()
		case key.Matches(msg, m.keyBindings.HalfPageDown):
			m.viewport.HalfViewDown()
		case key.Matches(msg, m.keyBindings.Up):
			m.viewport.LineUp(1)
		case key.Matches(msg, m.keyBindings.Down):
			m.viewport.LineDown(1)
		}

		if linesFromBottom(m.viewport) == 0 {
			m.userIsScrolledUp = false
		}

	case tea.MouseMsg:
		u, cmd := m.viewport.Update(msg)
		m.viewport = u
		cmds = append(cmds, cmd)

	case *v1.Message:
		cmd := m.processMessage(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.updateViewportContent()

	case *Error:
		m.upsertErrorMessage(msg)
		m.updateViewportContent()

	case subtaskMessageMsg:
		m.handleSubtaskMessage(msg)
		m.updateViewportContent()

	case subtaskCompletedMsg:
		m.handleSubtaskCompleted(msg)
		m.updateViewportContent()

	case subtaskErrorMsg:
		m.handleSubtaskError(msg)
		m.updateViewportContent()

	case subtaskCleanupTickMsg:
		m.cleanupStaleSubtasks()
		m.updateViewportContent()
		cmds = append(cmds, m.scheduleCleanupTick())
	}

	return m, tea.Batch(cmds...)
}

func (m *MessageFeed) handleSubtaskMessage(msg subtaskMessageMsg) {
	output, exists := m.subtaskOutputs[msg.toolCallID]
	if !exists {
		return
	}

	if output.Status == SubtaskStatusPending {
		output.Status = SubtaskStatusRunning
	}

	for _, part := range msg.message.Spec.Content {
		var newMsg message
		timestamp := time.Now()
		if msg.message.Metadata.CreatedAt != nil {
			timestamp = msg.message.Metadata.CreatedAt.AsTime()
		}

		switch data := part.Data.(type) {
		case *v1.MessagePart_Text_:
			if msg.message.Metadata.Role == v1.MessageRole_MESSAGE_ROLE_ASSISTANT {
				newMsg = &assistantTextMessage{
					content:   data.Text.Content,
					timestamp: timestamp,
				}
			}
		case *v1.MessagePart_ToolCall:
			newMsg = m.createToolCallMessage(data.ToolCall, timestamp)
		case *v1.MessagePart_ToolResult:
			newMsg = m.createToolResultMessage(data.ToolResult, timestamp)
		}

		if newMsg != nil {
			output.AddMessage(newMsg)
		}
	}
}

func (m *MessageFeed) handleSubtaskCompleted(msg subtaskCompletedMsg) {
	delete(m.subtaskOutputs, msg.toolCallID)
}

func (m *MessageFeed) handleSubtaskError(msg subtaskErrorMsg) {
	output, exists := m.subtaskOutputs[msg.toolCallID]
	if !exists {
		return
	}
	output.Status = SubtaskStatusError
	output.Error = msg.err
	output.LastActivity = time.Now()
}

func (m *MessageFeed) cleanupStaleSubtasks() {
	for toolCallID, output := range m.subtaskOutputs {
		if output.IsStale() {
			delete(m.subtaskOutputs, toolCallID)
		}
	}
}

func (m *MessageFeed) CleanupAllSubtasks() {
	m.subtaskOutputs = make(map[string]*SubtaskOutput)
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
	var rerender bool
	if m.width != width {
		rerender = true
	}
	m.width = width
	m.height = height

	m.viewport.Width = width
	m.viewport.Height = height

	if rerender {
		m.updateViewportContent()
	}

	return nil
}

func (m *MessageFeed) renderInitialMessage() string {
	separator := separatorStyle.Render()

	welcomeLines := []string{
		separator,
		"Welcome! Type your message below.",
		"Press Ctrl + ? for help at any time.",
		"Press Ctrl + C to clear the input area.",
		"Press Ctrl + C twice to exit.",
		"Press Esc to stop the agent execution.",
		separator,
		"",
	}

	return strings.Join(welcomeLines, "\n")
}

func (m *MessageFeed) updateViewportContent() {
	formatted := m.formatMessages(m.messages, m.partialMessage, m.viewport.Width)
	m.viewport.SetContent(formatted)

	shouldScroll := !m.userIsScrolledUp || lastMessageIsUserMessage(m.messages, m.partialMessage)

	if shouldScroll {
		m.viewport.GotoBottom()
		if lastMessageIsUserMessage(m.messages, m.partialMessage) {
			m.userIsScrolledUp = false
		}
	}
}

func linesFromBottom(vp viewport.Model) int {
	if vp.TotalLineCount() <= vp.Height {
		return 0
	}
	return vp.TotalLineCount() - vp.YOffset - vp.Height
}

func lastMessageIsUserMessage(messages []message, partialMessage string) bool {
	if len(messages) == 0 {
		return false
	}

	if partialMessage != "" {
		return false
	}

	lastMessage := messages[len(messages)-1]
	_, ok := lastMessage.(*userTextMessage)
	return ok
}

func (m *MessageFeed) upsertErrorMessage(errMsg *Error) {
	if errMsg == nil {
		return
	}

	if len(m.messages) > 0 {
		if lastMsg, ok := m.messages[len(m.messages)-1].(*Error); ok {
			lastMsg.Error = errMsg.Error
			return
		}
	}

	m.messages = append(m.messages, errMsg)
}

func (m *MessageFeed) processMessage(msg *v1.Message) tea.Cmd {
	var cmd tea.Cmd

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
			toolMsg := m.createToolCallMessage(data.ToolCall, msg.Metadata.CreatedAt.AsTime())
			if toolMsg != nil {
				m.messages = append(m.messages, toolMsg)
			}
		case *v1.MessagePart_ToolResult:
			resultMsg, resultCmd := m.createToolResultMessageWithCmd(data.ToolResult, msg.Metadata.CreatedAt.AsTime())
			if resultMsg != nil {
				m.messages = append(m.messages, resultMsg)
			}
			if resultCmd != nil {
				cmd = resultCmd
			}
		}
	}

	return cmd
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
	case *v1.ToolCall_SpawnTask:
		msg := &spawnTaskToolCall{
			ID:        toolCall.Id,
			Input:     toolInput.SpawnTask,
			timestamp: timestamp,
		}
		now := time.Now()
		m.subtaskOutputs[toolCall.Id] = &SubtaskOutput{
			ToolCallID:   toolCall.Id,
			AgentName:    toolInput.SpawnTask.Agent,
			Prompt:       toolInput.SpawnTask.Prompt,
			Messages:     []message{},
			Status:       SubtaskStatusPending,
			CreatedAt:    now,
			LastActivity: now,
		}
		return msg
	}

	return nil
}

func (m *MessageFeed) createToolResultMessage(toolResult *v1.ToolResult, timestamp time.Time) message {
	msg, _ := m.createToolResultMessageWithCmd(toolResult, timestamp)
	return msg
}

func (m *MessageFeed) createToolResultMessageWithCmd(toolResult *v1.ToolResult, timestamp time.Time) (message, tea.Cmd) {
	switch toolOutput := toolResult.Result.(type) {
	case *v1.ToolResult_CreateFile:
		return &createFileResult{
			ID:        toolResult.Id,
			Result:    toolOutput.CreateFile,
			timestamp: timestamp,
		}, nil
	case *v1.ToolResult_EditFile:
		return &editFileResult{
			ID:        toolResult.Id,
			Result:    toolOutput.EditFile,
			timestamp: timestamp,
		}, nil
	case *v1.ToolResult_ExecuteCommand:
		return &executeCommandResult{
			ID:        toolResult.Id,
			Result:    toolOutput.ExecuteCommand,
			timestamp: timestamp,
		}, nil
	case *v1.ToolResult_FindFile:
		return &findFileResult{
			ID:        toolResult.Id,
			Result:    toolOutput.FindFile,
			timestamp: timestamp,
		}, nil
	case *v1.ToolResult_Grep:
		return &grepResult{
			ID:        toolResult.Id,
			Result:    toolOutput.Grep,
			timestamp: timestamp,
		}, nil
	case *v1.ToolResult_ListFiles:
		return &listFilesResult{
			ID:        toolResult.Id,
			Result:    toolOutput.ListFiles,
			timestamp: timestamp,
		}, nil
	case *v1.ToolResult_ReadFile:
		return &readFileResult{
			ID:        toolResult.Id,
			Result:    toolOutput.ReadFile,
			timestamp: timestamp,
		}, nil
	case *v1.ToolResult_SubmitReport:
		return &submitReportResult{
			ID:        toolResult.Id,
			Result:    toolOutput.SubmitReport,
			timestamp: timestamp,
		}, nil
	case *v1.ToolResult_SpawnTask:
		if output, exists := m.subtaskOutputs[toolResult.Id]; exists {
			output.TaskID = toolOutput.SpawnTask.TaskId
			output.LastActivity = time.Now()
		}
		msg := &spawnTaskResult{
			ID:        toolResult.Id,
			Result:    toolOutput.SpawnTask,
			timestamp: timestamp,
		}
		cmd := func() tea.Msg {
			return subtaskSubscribeCmd{
				toolCallID: toolResult.Id,
				taskID:     toolOutput.SpawnTask.TaskId,
			}
		}
		return msg, cmd
	}

	return nil, nil
}

func (m *MessageFeed) formatMessages(messages []message, partialMessage string, width int) string {
	renderedMessages := []string{}
	for i, msg := range messages {
		switch msg := msg.(type) {
		case *userTextMessage:
			renderedMessages = append(renderedMessages, renderUserMessage(msg, width, addBottomMargin(i, messages)))

		case *assistantTextMessage:
			renderedMessages = append(renderedMessages, renderAssistantMessage(msg, width, addBottomMargin(i, messages)))

		case *readFileToolCall:
			var readFileInput string
			if msg.Input.StartLine != 0 && msg.Input.EndLine != 0 {
				readFileInput = fmt.Sprintf("%s L%d-%d", msg.Input.Path, msg.Input.StartLine, msg.Input.EndLine)
			} else {
				readFileInput = msg.Input.Path
			}
			renderedMessages = append(renderedMessages, renderToolCallMessage("Read", readFileInput, width, addBottomMargin(i, messages)))

		case *createFileToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Create", msg.Input.Path, width, addBottomMargin(i, messages)))

		case *editFileToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Edit", msg.Input.Path, width, addBottomMargin(i, messages)))

		case *executeCommandToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Execute", msg.Input.Command, width, addBottomMargin(i, messages)))

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
				renderToolCallMessage("Find", fmt.Sprintf("pattern: %s, path: %s, exclude: %s", msg.Input.Pattern, pathInfo, excludeArg), width, addBottomMargin(i, messages)))

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

		case *spawnTaskToolCall:
			promptPreview := msg.Input.Prompt
			if len(promptPreview) > 40 {
				promptPreview = promptPreview[:37] + "..."
			}
			renderedMessages = append(renderedMessages, renderToolCallMessage("Spawn", fmt.Sprintf("%s: %q", msg.Input.Agent, promptPreview), width, false))

			if output, exists := m.subtaskOutputs[msg.ID]; exists {
				subtaskOutput := m.renderSubtaskOutput(output, width)
				if subtaskOutput != "" {
					renderedMessages = append(renderedMessages, subtaskOutput)
				}
			}

			if addBottomMargin(i, messages) {
				renderedMessages = append(renderedMessages, "")
			}

		case *Error:
			var message string
			if msg != nil && msg.Error != nil {
				if err, ok := msg.Error.(*fail.UserFacingError); ok {
					message = err.UserMessage
				} else {
					message = msg.Error.Error()
				}

				renderedMessages = append(renderedMessages, errorStyle.Render("❌ Error: ")+message)
			}
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

func (m *MessageFeed) renderSubtaskOutput(output *SubtaskOutput, width int) string {
	if output == nil {
		return ""
	}

	var lines []string

	promptPreview := output.Prompt
	if len(promptPreview) > 50 {
		promptPreview = promptPreview[:47] + "..."
	}
	header := subtaskHeaderStyle.Render(fmt.Sprintf("╭─ %s: %q", output.AgentName, promptPreview))
	lines = append(lines, header)

	switch output.Status {
	case SubtaskStatusPending:
		lines = append(lines, subtaskContentStyle.Render("│ Pending..."))
	case SubtaskStatusError:
		errMsg := "subscription failed"
		if output.Error != nil {
			errMsg = output.Error.Error()
		}
		lines = append(lines, subtaskContentStyle.Render(fmt.Sprintf("│ ❌ Error: %s", errMsg)))
	case SubtaskStatusRunning, SubtaskStatusCompleted:
		if len(output.Messages) == 0 {
			lines = append(lines, subtaskContentStyle.Render("│ Waiting for output..."))
		} else {
			for _, msg := range output.Messages {
				msgLine := m.formatSubtaskMessage(msg, width-8)
				lines = append(lines, subtaskContentStyle.Render("│ "+msgLine))
			}
		}
	}

	lines = append(lines, subtaskContentStyle.Render("╰─"))

	return strings.Join(lines, "\n")
}

func (m *MessageFeed) formatSubtaskMessage(msg message, width int) string {
	switch msg := msg.(type) {
	case *assistantTextMessage:
		content := msg.content
		content = strings.ReplaceAll(content, "\n", " ")
		if len(content) > width {
			content = content[:width-3] + "..."
		}
		return content

	case *readFileToolCall:
		return fmt.Sprintf("◆ Read(%s)", truncatePath(msg.Input.Path, width-10))

	case *createFileToolCall:
		return fmt.Sprintf("◆ Create(%s)", truncatePath(msg.Input.Path, width-12))

	case *editFileToolCall:
		return fmt.Sprintf("◆ Edit(%s)", truncatePath(msg.Input.Path, width-10))

	case *executeCommandToolCall:
		cmd := msg.Input.Command
		if len(cmd) > 30 {
			cmd = cmd[:27] + "..."
		}
		return fmt.Sprintf("◆ Execute(%s)", cmd)

	case *findFileToolCall:
		return fmt.Sprintf("◆ Find(%s)", msg.Input.Pattern)

	case *grepToolCall:
		query := msg.Input.Query
		if len(query) > 20 {
			query = query[:17] + "..."
		}
		return fmt.Sprintf("◆ Grep(%s)", query)

	case *listFilesToolCall:
		return fmt.Sprintf("◆ List(%s)", truncatePath(msg.Input.Path, width-10))

	case *handoffToolCall:
		return fmt.Sprintf("◆ Handoff(%s)", msg.Input.RequestedAgent)

	default:
		return "◆ ..."
	}
}

func truncatePath(path string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 20
	}
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}
