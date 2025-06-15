package terminal

import "github.com/charmbracelet/lipgloss"

var (
	infoSymbolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33")).
			Bold(true).
			SetString("ⓘ")

	errorSymbolStyle = lipgloss.NewStyle().
				SetString("❌")

	smallErrorSymbolStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true).
				SetString("✗")

	warningSymbolStyle = lipgloss.NewStyle().
				SetString("⚠️")

	successSymbolStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10")).
				Bold(true).
				SetString("✔")

	attentionSymbolStyle = lipgloss.NewStyle().
				SetString("❗")

	questionSymbolStyle = lipgloss.NewStyle().
				SetString("❓")

	actionSymbolStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				SetString("▶")

	linkSymbolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("75")).
			SetString("→")

	docsStyle = lipgloss.NewStyle().
			SetString("📚")

	communitySymbolStyle = lipgloss.NewStyle().
				SetString("💬")

	bugSymbolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			SetString("⚑")
)

var (

	// InfoSymbol (ⓘ)
	InfoSymbol = infoSymbolStyle.String()

	// WarningSymbol (⚠️)
	WarningSymbol = warningSymbolStyle.String()

	// ErrorSymbol (❌)
	ErrorSymbol = errorSymbolStyle.String()

	// SmallErrorSymbol (✗)
	SmallErrorSymbol = smallErrorSymbolStyle.String()

	// SuccessSymbol (✔)
	SuccessSymbol = successSymbolStyle.String()

	// AttentionSymbol (❗)
	AttentionSymbol = attentionSymbolStyle.String()

	// QuestionSymbol (❓)
	QuestionSymbol = questionSymbolStyle.String()

	// ActionSymbol (▶)
	ActionSymbol = actionSymbolStyle.String()

	// LinkSymbol (→)
	LinkSymbol = linkSymbolStyle.String()

	// DocsSymbol (📚)
	DocsSymbol = docsStyle.String()

	// CommunitySymbol (💬)
	CommunitySymbol = communitySymbolStyle.String()

	// BugSymbol (⚑)
	BugSymbol = bugSymbolStyle.String()
)
