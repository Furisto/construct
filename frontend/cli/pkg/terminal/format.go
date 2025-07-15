package terminal

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	v1 "github.com/furisto/construct/api/go/v1"
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
			formatted.WriteString(whiteBullet.String() +
				formatMessageContent(msg.content, m.width-6))

		case *assistantToolMessage:
			toolName := getToolNameString(msg.toolName)
			formatted.WriteString(blueBullet.String() +
				toolCallStyle.Render(fmt.Sprintf("Tool: %s", toolName)) + "\n")

			if len(msg.arguments) > 0 {
				formatted.WriteString("  Arguments:\n")
				for key, value := range msg.arguments {
					formatted.WriteString(fmt.Sprintf("    %s: %s\n",
						boldStyle.Render(key),
						toolArgsStyle.Render(value)))
				}
			}

			if msg.error != "" {
				formatted.WriteString("  " + errorStyle.Render("Error: ") + msg.error + "\n")
			}
		case *submitReportMessage:
			formatted.WriteString(reportStyle.Render("📋 Task Report") + "\n")
			formatted.WriteString(reportContentStyle.Render("Summary: ") + msg.summary + "\n")

			if msg.completed {
				formatted.WriteString(reportContentStyle.Render("Status: ") + "✅ Completed\n")
			} else {
				formatted.WriteString(reportContentStyle.Render("Status: ") + "🔄 In Progress\n")
			}

			if len(msg.deliverables) > 0 {
				formatted.WriteString(reportContentStyle.Render("Deliverables:\n"))
				for _, deliverable := range msg.deliverables {
					formatted.WriteString(fmt.Sprintf("  • %s\n", deliverable))
				}
			}

			if msg.nextSteps != "" {
				formatted.WriteString(reportContentStyle.Render("Next Steps: ") + msg.nextSteps + "\n")
			}

		case *errorMessage:
			formatted.WriteString(errorStyle.Render("❌ Error: ") + msg.content)
		}
	}

	f, _ := os.CreateTemp("", "construct-cli-messages.md")
	f.WriteString(formatted.String())
	f.Close()

	return formatted.String()
}

func getToolNameString(toolName v1.ToolName) string {
	switch toolName {
	case v1.ToolName_EDIT_FILE:
		return "Edit File"
	case v1.ToolName_CREATE_FILE:
		return "Create File"
	case v1.ToolName_READ_FILE:
		return "Read File"
	case v1.ToolName_EXECUTE_COMMAND:
		return "Execute Command"
	case v1.ToolName_FIND_FILE:
		return "Find File"
	case v1.ToolName_HANDOFF:
		return "Handoff"
	case v1.ToolName_LIST_FILES:
		return "List Files"
	case v1.ToolName_CODE_INTERPRETER:
		return "Code Interpreter"
	default:
		return "Unknown Tool"
	}
}

// formatMessageContent formats the content of a message
func formatMessageContent(content string, maxWidth int) string {
	md, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"), // avoid OSC background queries
		// glamour.WithWordWrap(maxWidth),
	)

	out, _ := md.Render(content)

	// // If it's a code block, format it differently
	// if containsCodeBlock(content) {
	// 	return formatCodeBlocks(content, maxWidth)
	// }

	// Regular text formatting

	return assistantTextStyle.Render(trimLeadingWhitespaceWithANSI(out))
}

func trimLeadingWhitespaceWithANSI(s string) string {
	// This pattern matches from the start:
	// - Any combination of whitespace OR ANSI sequences
	// - Stops when it hits a character that's neither
	pattern := `^(?:\x1b\[[0-9;]*m|\s)*`
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
