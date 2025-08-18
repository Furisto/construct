package terminal

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

func formatMessages(messages []message, partialMessage string, width int) string {
	renderedMessages := []string{}
	for _, msg := range messages {

		switch msg := msg.(type) {
		case *userTextMessage:
			renderedMessages = append(renderedMessages, renderUserMessage(msg, width))

		case *assistantTextMessage:
			renderedMessages = append(renderedMessages, renderAssistantMessage(msg, width))

		case *readFileToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Read", msg.Input.Path, width))

		case *createFileToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Create", msg.Input.Path, width))

		case *editFileToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Edit", msg.Input.Path, width))

		case *executeCommandToolCall:
			command := msg.Input.Command
			if len(command) > 50 {
				command = command[:47] + "..."
			}
			renderedMessages = append(renderedMessages, renderToolCallMessage("Execute", command, width))

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
				renderToolCallMessage("Find", fmt.Sprintf("%s(pattern: %s, path: %s, exclude: %s)", boldStyle.Render("Find"), msg.Input.Pattern, pathInfo, excludeArg), width))

		case *grepToolCall:
			searchInfo := msg.Input.Query
			if msg.Input.IncludePattern != "" {
				searchInfo = fmt.Sprintf("%s in %s", searchInfo, msg.Input.IncludePattern)
			}
			renderedMessages = append(renderedMessages, renderToolCallMessage("Grep", searchInfo, width))

		case *handoffToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Handoff", msg.Input.RequestedAgent, width))

		case *listFilesToolCall:
			pathInfo := msg.Input.Path
			if pathInfo == "" {
				pathInfo = "."
			}
			listType := "List"
			if msg.Input.Recursive {
				listType = "List -R"
			}
			renderedMessages = append(renderedMessages, renderToolCallMessage(listType, pathInfo, width))

		case *codeInterpreterToolCall:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Interpreter", "Script", width))
			renderedMessages = append(renderedMessages, formatCodeInterpreterContent(msg.Input.Code))

		case *codeInterpreterResult:
			renderedMessages = append(renderedMessages, renderToolCallMessage("Interpreter", "Output", width))
			renderedMessages = append(renderedMessages, formatCodeInterpreterContent(msg.Result.Output))

		case *errorMessage:
			renderedMessages = append(renderedMessages, errorStyle.Render("❌ Error: ")+msg.content)
		}
	}

	if partialMessage != "" {
		renderedMessages = append(renderedMessages, renderAssistantMessage(&assistantTextMessage{content: partialMessage}, width))
	}

	var marginMessages []string
	for _, msg := range renderedMessages {
		marginMessages = append(marginMessages, lipgloss.JoinVertical(
			lipgloss.Left,
			msg,
			"",
		))
	}

	return lipgloss.JoinVertical(
		lipgloss.Top,
		marginMessages...,
	)

	// f, _ := os.CreateTemp("", "construct-cli-messages.md")
	// f.WriteString(formatted.String())
	// f.Close()

	// return formatted.String()
}

func renderUserMessage(msg *userTextMessage, width int) string {
	// markdown := formatAsMarkdown(msg.content, width)
	return userMessageStyle.Width(width - 1).Render(msg.content)
}

func renderAssistantMessage(msg *assistantTextMessage, width int) string {
	markdown := formatAsMarkdown(msg.content, width)
	return assistantMessageStyle.Width(width - 1).Render(markdown)
}

func renderToolCallMessage(tool, input string, width int) string {
	return "  " + assistantBullet.String() + toolCallStyle.Width(width-1).Render(fmt.Sprintf("%s(%s)", boldStyle.Render(tool), input))
}

func formatAsMarkdown(content string, width int) string {
	md, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"), // avoid OSC background queries
		glamour.WithWordWrap(width),
	)

	out, _ := md.Render(content)
	trimmed := trimLeadingWhitespaceWithANSI(out)
	return trimTrailingWhitespaceWithANSI(trimmed)
}

func trimLeadingWhitespaceWithANSI(s string) string {
	// This pattern matches from the start:
	// - Any combination of whitespace OR ANSI sequences
	// - Stops when it hits a character that's neither
	pattern := `^(?:\x1b\[[0-9;]*m|\s)*`
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(s, "")
}

func trimTrailingWhitespaceWithANSI(s string) string {
	// This pattern matches from the end:
	// - Any combination of whitespace OR ANSI sequences
	// - Stops when it hits a character that's neither
	pattern := `(?:\x1b\[[0-9;]*m|\s)*$`
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(s, "")
}

func containsCodeBlock(content string) bool {
	return strings.Contains(content, "```")
}

// func formatCodeBlocks(content string, maxWidth int) string {
// 	if !containsCodeBlock(content) {
// 		return assistantTextStyle.Render(content)
// 	}

// 	// Split the content by code block markers
// 	parts := strings.Split(content, "```")
// 	var formatted strings.Builder

// 	// Process each part
// 	for i, part := range parts {
// 		if i == 0 {
// 			// First part is regular text (might be empty)
// 			if part != "" {
// 				formatted.WriteString(assistantTextStyle.Render(part))
// 				formatted.WriteString("\n")
// 			}
// 		} else if i%2 == 1 {
// 			// Odd indexed parts are code blocks
// 			// Extract language if specified
// 			lang := ""
// 			codeContent := part
// 			if idx := strings.Index(part, "\n"); idx > 0 {
// 				lang = part[:idx]
// 				codeContent = part[idx+1:]
// 			}

// 			// Add language indicator if present
// 			if lang != "" {
// 				formatted.WriteString(lipgloss.NewStyle().
// 					Foreground(lipgloss.Color("241")).
// 					Render(fmt.Sprintf("(%s)\n", lang)))
// 			}

// 			// Format the code block
// 			formatted.WriteString(codeBlockStyle.Render(codeContent))
// 			formatted.WriteString("\n")
// 		} else {
// 			// Even indexed parts (after the first) are regular text
// 			if part != "" {
// 				formatted.WriteString(assistantTextStyle.Render(part))
// 				formatted.WriteString("\n")
// 			}
// 		}
// 	}

// 	return formatted.String()
// }

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func addIndentationToLines(content, indentation string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" { // Only indent non-empty lines
			lines[i] = indentation + line
		}
	}
	return strings.Join(lines, "\n")
}

func formatCodeInterpreterContent(code string) string {
	// Process the code through markdown rendering
	md, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
	)

	rendered, _ := md.Render(fmt.Sprintf("```\n%s\n```", code))
	trimmed := trimLeadingWhitespaceWithANSI(rendered)
	trimmed = trimTrailingWhitespaceWithANSI(trimmed)

	// Apply the code interpreter style to each line
	lines := strings.Split(trimmed, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = codeInterpreterStyle.Render(line)
		}
	}

	// Add consistent indentation
	return addIndentationToLines(strings.Join(lines, "\n"), "    ")
}
