package terminal

import (
	tea "github.com/charmbracelet/bubbletea"
)

type agentChangedMsg struct {
	Agent string
}

type pricingMsg struct {
	Context      int64
	InputTokens  int64
	OutputTokens int64
	Cost         float64
}

type Status struct {
	tea.Model
	Info statusInfo

	width int
}

type statusInfo struct {
	Agent        string
	Workspace    string
	Context      int64
	InputTokens  int64
	OutputTokens int64
	Cost         float64
}

func (m *Status) Init() tea.Cmd {
	return nil
}

func (s *Status) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
	case agentChangedMsg:
		s.Info.Agent = msg.Agent
	case pricingMsg:
		s.Info.Context = msg.Context
		s.Info.InputTokens = msg.InputTokens
		s.Info.OutputTokens = msg.OutputTokens
		s.Info.Cost = msg.Cost
	}

	return s, nil
}

func (m *Status) View() string {
	return ""
}
