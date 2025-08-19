package terminal

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
)

func renderUserMessage(msg *userTextMessage, width int, margin bool) string {
	msgWidth := width - userMessageStyle.GetHorizontalBorderSize()
	markdown := formatAsMarkdown(msg.content, msgWidth)
	style := userMessageStyle.Width(msgWidth)
	if margin {
		style = style.MarginBottom(1)
	}
	return style.Render(markdown)
}

func renderAssistantMessage(msg *assistantTextMessage, width int, margin bool) string {
	msgWidth := width - assistantMessageStyle.GetHorizontalBorderSize()
	markdown := formatAsMarkdown(msg.content, msgWidth)
	style := assistantMessageStyle.Width(msgWidth)
	if margin {
		style = style.MarginBottom(1)
	}
	return style.Render(markdown)
}

func renderToolCallMessage(tool, input string, width int, margin bool) string {
	style := toolCallStyle.Width(width - toolCallStyle.GetHorizontalBorderSize())
	if margin {
		style = style.MarginBottom(1)
	}
	return style.Render("◆ " + fmt.Sprintf("%s(%s)", boldStyle.Render(tool), input))
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

func addBottomMargin(idx int, messages []message) bool {
	return idx == 0 || idx != len(messages)-1
}
