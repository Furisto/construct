package terminal

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	appStyle = lipgloss.NewStyle().Margin(1, 2)

	// Header styles
	headerStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Background(lipgloss.Color("235")).
			Padding(0, 1).
			MarginBottom(1)

	agentNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	taskStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("34"))

	usageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	// Input styles
	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Background(lipgloss.NoColor{}).
			Padding(0, 1)

	inputFocusedStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Background(lipgloss.NoColor{}).
				Padding(0, 1)

	// Viewport styles
	viewportStyle = lipgloss.NewStyle().
			Padding(1).
			MarginTop(1).
			MarginBottom(1)

	// Footer styles
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			MarginTop(1)

	shortcutStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	shortcutDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	// Message styles
	userMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	assistantTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	assistantToolStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39"))

	// Code block style for tool messages
	codeBlockStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	// Bullet points
	whiteBullet = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			SetString("• ")

	blueBullet = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			SetString("• ")

	// Waiting indicator
	waitingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)

	// Error style
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true)

	// Tool message styles
	toolCallStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	toolArgsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true)

	// Submit report styles
	reportStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")).
			Bold(true)

	reportContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	// Status indicator styles
	statusRunningStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("34")).
				Bold(true)

	statusWaitingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true)

	statusErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("9")).
				Bold(true)

	// Timestamp style
	timestampStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)

	// User prompt style
	userPromptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true).
			SetString("> ")

	// Separator style
	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			SetString(strings.Repeat("─", 80))

	boldStyle = lipgloss.NewStyle().
			Bold(true)

	// Help overlay styles
	helpOverlayStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("39")).
				Background(lipgloss.Color("235")).
				Padding(1, 2).
				Margin(1)

	helpTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true).
			Align(lipgloss.Center)

	helpItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
)

func Bold(s string) string {
	return boldStyle.Render(s)
}
