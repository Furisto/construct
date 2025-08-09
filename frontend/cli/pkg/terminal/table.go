package terminal

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	v1 "github.com/furisto/construct/api/go/v1"
)

const idWidth = 36
const createdAtWidth = 16
const updatedAtWidth = 16
const workspaceWidth = 50
const summaryWidth = 50

type SelectableTable struct {
	title         string
	headers       []string
	rows          []TableRow
	selected      int
	width         int
	height        int
	cancelled     bool
	headerStyle   lipgloss.Style
	rowStyle      lipgloss.Style
	selectedStyle lipgloss.Style
}

type TableRow struct {
	ID          string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Workspace   string
	Description string
	Task        *v1.Task
}

func (t *TableRow) Init() tea.Cmd {
	return nil
}

func (t *TableRow) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return t, nil
}

func (t *TableRow) View() string {
	return fmt.Sprintf("%s  %s  %s  %s  %s",
		t.ID,
		t.CreatedAt.Format("2006-01-02 15:04"),
		t.UpdatedAt.Format("2006-01-02 15:04"),
		truncate(t.Workspace, workspaceWidth),
		truncate(t.Description, summaryWidth),
	)
}

func NewSelectableTable(title string, headers []string, rows []TableRow) *SelectableTable {
	return &SelectableTable{
		title:     title,
		headers:   headers,
		rows:      rows,
		selected:  0,
		width:     80,
		height:    20,
		cancelled: false,
		headerStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true).
			Padding(0, 1),
		rowStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Padding(0, 1),
		selectedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Background(lipgloss.Color("236")).
			Bold(true).
			Padding(0, 1),
	}
}

func (t *SelectableTable) Init() tea.Cmd {
	return nil
}

func (t *SelectableTable) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		return t, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			t.cancelled = true
			return t, tea.Quit

		case "enter":
			return t, tea.Quit

		case "up", "k":
			if t.selected > 0 {
				t.selected--
			}

		case "down", "j":
			if t.selected < len(t.rows)-1 {
				t.selected++
			}

		case "home", "g":
			t.selected = 0

		case "end", "G":
			t.selected = len(t.rows) - 1

		case "pageup", "b":
			t.selected = max(0, t.selected-10)

		case "pagedown", "f":
			t.selected = min(len(t.rows)-1, t.selected+10)
		}
	}

	return t, nil
}

func (t *SelectableTable) View() string {
	if len(t.rows) == 0 {
		return lipgloss.Place(
			t.width,
			t.height,
			lipgloss.Center,
			lipgloss.Center,
			"No tasks found",
		)
	}

	var lines []string

	lines = append(lines, t.headerStyle.Render(t.title))
	lines = append(lines, "")

	header := t.renderHeader()
	lines = append(lines, header)
	lines = append(lines, strings.Repeat("─", t.width-4))

	for index, row := range t.rows {
		rowStr := row.View()
		if index == t.selected {
			rowStr = t.selectedStyle.Render(rowStr)
		} else {
			rowStr = t.rowStyle.Render(rowStr)
		}
		lines = append(lines, rowStr)
	}

	lines = append(lines, "")
	lines = append(lines, "Press ↑/↓ or j/k to navigate, Enter to select, Esc to cancel")

	return strings.Join(lines, "\n")
}

func (t *SelectableTable) renderHeader() string {
	header := fmt.Sprintf("%-*s  %-*s  %-*s  %-*s  %-*s",
		idWidth, "ID",
		createdAtWidth, "Created",
		updatedAtWidth, "Updated",
		workspaceWidth, "Workspace",
		summaryWidth, "Summary")

	return t.headerStyle.Render(header)
}

func (t *SelectableTable) GetSelectedTask() *v1.Task {
	if t.cancelled || t.selected >= len(t.rows) {
		return nil
	}
	return t.rows[t.selected].Task
}

func (t *SelectableTable) IsCancelled() bool {
	return t.cancelled
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
