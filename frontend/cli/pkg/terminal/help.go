package terminal

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Help struct {
	tea.Model
}

func (m *Session) renderHelp() string {
	helpContent := []string{
		helpTitleStyle.Render("KEYBOARD SHORTCUTS"),
		"",
		helpItemStyle.Render("General:"),
		helpItemStyle.Render("  H, h          - Toggle this help"),
		helpItemStyle.Render("  Ctrl+C        - Quit application"),
		helpItemStyle.Render("  Ctrl+L        - Clear conversation"),
		helpItemStyle.Render("  Ctrl+R        - Reconnect to task"),
		helpItemStyle.Render("  Tab           - Switch agent"),
		"",
		helpItemStyle.Render("Input Mode (F1):"),
		helpItemStyle.Render("  Enter         - Send message"),
		helpItemStyle.Render("  F2            - Switch to scroll mode"),
		"",
		helpItemStyle.Render("Scroll Mode (F2):"),
		helpItemStyle.Render("  ↑↓, k/j       - Line up/down"),
		helpItemStyle.Render("  PgUp/PgDn     - Page up/down"),
		helpItemStyle.Render("  Home/End      - Go to top/bottom"),
		helpItemStyle.Render("  F1            - Switch to input mode"),
		"",
		helpItemStyle.Render("Press H or Esc to close this help."),
	}

	return helpOverlayStyle.Render(strings.Join(helpContent, "\n"))
}
