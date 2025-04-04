package terminal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) formatMessages() string {
	var formatted strings.Builder

	for i, msg := range m.messages {
		// Add separator between different conversations
		if i > 0 && m.messages[i-1].Type() == MessageTypeAssistantTool && msg.Type() == MessageTypeUser {
			formatted.WriteString("\n")
		}

		switch msg := msg.(type) {
		case *userMessage:
			formatted.WriteString(userPromptStyle.String() + msg.content + "\n\n")
		case *assistantTextMessage:
			formatted.WriteString(whiteBullet.String() +
				formatMessageContent(msg.content, m.width-6, false) + "\n\n")
			// case assistantToolMessage:
			// 	formatted.WriteString(blueBullet.String() +
			// 		formatMessageContent(msg.content, m.width-6, true) + "\n\n")
			// case assistantTypingMessage:
			// 	if containsCodeBlock(msg.content) {
			// 		// Treat as a tool message if it contains code blocks
			// 		formatted.WriteString(blueBullet.String() +
			// 			formatMessageContent(msg.content, m.width-6, true) + "█\n\n")
			// 	} else {
			// 		// Otherwise treat as a text message
			// 		formatted.WriteString(whiteBullet.String() +
			// 			formatMessageContent(msg.content, m.width-6, false) + "█\n\n")
			// 	}
		}
	}

	return formatted.String()
}

// formatMessageContent formats the content of a message
func formatMessageContent(content string, maxWidth int, isCode bool) string {
	// If it's a code block, format it differently
	if containsCodeBlock(content) {
		return formatCodeBlocks(content, maxWidth)
	}

	// Regular text formatting
	return assistantTextStyle.Render(content)
}

// containsCodeBlock checks if the content contains a code block
func containsCodeBlock(content string) bool {
	return strings.Contains(content, "```")
}

// formatCodeBlocks formats code blocks in a message
func formatCodeBlocks(content string, maxWidth int) string {
	// If not a code block, return as is
	if !containsCodeBlock(content) {
		return assistantTextStyle.Render(content)
	}

	// Split the content by code block markers
	parts := strings.Split(content, "```")
	var formatted strings.Builder

	// Process each part
	for i, part := range parts {
		if i == 0 {
			// First part is regular text (might be empty)
			if part != "" {
				formatted.WriteString(assistantTextStyle.Render(part))
				formatted.WriteString("\n")
			}
		} else if i%2 == 1 {
			// Odd indexed parts are code blocks
			// Extract language if specified
			lang := ""
			codeContent := part
			if idx := strings.Index(part, "\n"); idx > 0 {
				lang = part[:idx]
				codeContent = part[idx+1:]
			}

			// Add language indicator if present
			if lang != "" {
				formatted.WriteString(lipgloss.NewStyle().
					Foreground(lipgloss.Color("241")).
					Render(fmt.Sprintf("(%s)\n", lang)))
			}

			// Format the code block
			formatted.WriteString(codeBlockStyle.Render(codeContent))
			formatted.WriteString("\n")
		} else {
			// Even indexed parts (after the first) are regular text
			if part != "" {
				formatted.WriteString(assistantTextStyle.Render(part))
				formatted.WriteString("\n")
			}
		}
	}

	return formatted.String()
}
