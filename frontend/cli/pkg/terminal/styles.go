package terminal

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// UI styles
var (
	// Base styles
	appStyle = lipgloss.NewStyle().Margin(1,2)

	// Input field styles
	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Background(lipgloss.NoColor{}).
			MaxWidth(120).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

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

	// User prompt style
	userPromptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Bold(true).
			SetString("> ")

	// Separator style
	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			SetString(strings.Repeat("─", 80))
)
