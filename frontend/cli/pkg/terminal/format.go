package terminal

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
)

func (m model) formatMessages() string {
	var formatted strings.Builder

	for i, msg := range m.messages {
		if i > 0 {
			formatted.WriteString("\n\n")
		}

		switch msg := msg.(type) {
		case *userMessage:
			formatted.WriteString(userPromptStyle.String() + msg.content)

		case *assistantTextMessage:
			formatted.WriteString(assistantBullet.String() +
				formatMessageContent(msg.content))

		case *readFileToolCall:
			formatted.WriteString("  " + toolCallBullet.String())
			formatted.WriteString(toolCallStyle.Render(fmt.Sprintf("%s(%s)", boldStyle.Render("Read"), msg.Input.Path)))

		case *createFileToolCall:
			formatted.WriteString("  " + toolCallBullet.String())
			formatted.WriteString(toolCallStyle.Render(fmt.Sprintf("%s(%s)", boldStyle.Render("Create"), msg.Input.Path)))

		case *editFileToolCall:
			formatted.WriteString("  " + toolCallBullet.String())
			formatted.WriteString(toolCallStyle.Render(fmt.Sprintf("%s(%s)", boldStyle.Render("Edit"), msg.Input.Path)))

		case *executeCommandToolCall:
			command := msg.Input.Command
			if len(command) > 50 {
				command = command[:47] + "..."
			}
			formatted.WriteString("  " + toolCallBullet.String())
			formatted.WriteString(toolCallStyle.Render(fmt.Sprintf("%s(%s)", boldStyle.Render("Execute"), command)))

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

			formatted.WriteString("  " + toolCallBullet.String())
			formatted.WriteString(toolCallStyle.Render(fmt.Sprintf("%s(pattern: %s, path: %s, exclude: %s)", boldStyle.Render("Find"), msg.Input.Pattern, pathInfo, excludeArg)))

		case *grepToolCall:
			searchInfo := msg.Input.Query
			if msg.Input.IncludePattern != "" {
				searchInfo = fmt.Sprintf("%s in %s", searchInfo, msg.Input.IncludePattern)
			}
			formatted.WriteString("  " + toolCallBullet.String())
			formatted.WriteString(toolCallStyle.Render(fmt.Sprintf("%s(%s)", boldStyle.Render("Grep"), searchInfo)))

		case *handoffToolCall:
			formatted.WriteString("  " + toolCallBullet.String())
			formatted.WriteString(toolCallStyle.Render(fmt.Sprintf("%s → %s", boldStyle.Render("Handoff"), msg.Input.RequestedAgent)))

		case *listFilesToolCall:
			pathInfo := msg.Input.Path
			if pathInfo == "" {
				pathInfo = "."
			}
			listType := "List"
			if msg.Input.Recursive {
				listType = "List -R"
			}
			formatted.WriteString("  " + toolCallBullet.String())
			formatted.WriteString(toolCallStyle.Render(fmt.Sprintf("%s(%s)", boldStyle.Render(listType), pathInfo)))

		case *codeInterpreterToolCall:
			formatted.WriteString("  " + toolCallBullet.String())
			formatted.WriteString(toolCallStyle.Render(fmt.Sprintf("%s(%s)", boldStyle.Render("Interpreter"), "Script")))
			formatted.WriteString("\n")
			formatted.WriteString(formatCodeInterpreterContent(msg.Input.Code))

		case *codeInterpreterResult:
			formatted.WriteString("  " + toolCallBullet.String())
			formatted.WriteString(toolCallStyle.Render(fmt.Sprintf("%s(%s)", boldStyle.Render("Interpreter"), "Output")))
			formatted.WriteString("\n")
			formatted.WriteString(formatCodeInterpreterContent(msg.Result.Output))

		case *errorMessage:
			formatted.WriteString(errorStyle.Render("❌ Error: ") + msg.content)
		}
	}

	if m.partialMessage != "" {
		formatted.WriteString("\n\n")
		formatted.WriteString(assistantBullet.String() +
			formatMessageContent(m.partialMessage))
	}

	// f, _ := os.CreateTemp("", "construct-cli-messages.md")
	// f.WriteString(formatted.String())
	// f.Close()

	return formatted.String()
}

func formatMessageContent(content string) string {
	md, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"), // avoid OSC background queries
		// glamour.WithWordWrap(maxWidth),
	)

	out, _ := md.Render(content)
	trimmed := trimLeadingWhitespaceWithANSI(out)
	trimmed = trimTrailingWhitespaceWithANSI(trimmed)
	return assistantTextStyle.Render(trimmed)
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
