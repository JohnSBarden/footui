package league_test

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

// stubScreen stands in for a real drill-down target, so navigation can be
// asserted without the League tests depending on another screen package.
type stubScreen struct{}

func (stubScreen) Init() tea.Cmd                           { return nil }
func (s stubScreen) Update(tea.Msg) (core.Screen, tea.Cmd) { return s, nil }
func (stubScreen) View() string                            { return "stub" }
func (stubScreen) Title() string                           { return "stub" }

func enterKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }

func (stubScreen) Keys() core.KeyMap { return core.KeyMap{} }
