package player_test

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

// stubScreen stands in for a drill-down target.
type stubScreen struct{}

func (stubScreen) Init() tea.Cmd                           { return nil }
func (s stubScreen) Update(tea.Msg) (core.Screen, tea.Cmd) { return s, nil }
func (stubScreen) View() string                            { return "stub" }
func (stubScreen) Title() string                           { return "stub" }
