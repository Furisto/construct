package terminal

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	appStyle = lipgloss.NewStyle().
			Margin(1, 2)
		// MarginBackground(lipgloss.Color("21"))

	headerStyle = lipgloss.NewStyle().
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1).
			MarginBottom(1)
		// MarginBackground(lipgloss.Color("3")).
		// Background(lipgloss.Color("208"))

	inputStyle = lipgloss.NewStyle().
			MarginTop(1)
		// MarginBackground(lipgloss.Color("3")).
		// Background(lipgloss.Color("208"))

	// messageFeedStyle = lipgloss.NewStyle().
	// 	MarginBackground(lipgloss.Color("3")).
	// 	Background(lipgloss.Color("212"))

	agentNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)

	agentDiamondStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("250")).
				Bold(true)

	agentModelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true)

	bulletSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	taskStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("34"))

	usageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	// Bullet points
	toolCallBullet = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			SetString("◆ ")

	// Error style
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Bold(true)

	// Tool message styles
	toolCallStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			PaddingLeft(1)

	userMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("245")).
				MarginBottom(1).
				Padding(0, 1)

	assistantMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("254")).
				PaddingLeft(1)

	// Separator style
	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			SetString(strings.Repeat("─", 80))

	boldStyle = lipgloss.NewStyle().
			Bold(true)

	// Code interpreter styles
	codeInterpreterStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Padding(0, 1).
				BorderLeft(true).
				BorderStyle(lipgloss.ThickBorder()).
				BorderForeground(lipgloss.Color("34"))

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
