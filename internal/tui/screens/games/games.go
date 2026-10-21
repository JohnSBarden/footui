// Package games lists a club's last games in every competition, newest
// first: its league, its cups, Europe. Each can be expanded into a box
// score, and its opponent opened.
package games

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// Limit is how many games the screen lists.
const Limit = 30

// Model is the Games screen.
type Model struct {
	deps  core.Deps
	team  models.Team
	games []models.Match
	teams map[string]models.Team
	comps map[string]models.Competition
	// cursor is the highlighted game; expanded is the ID of the game whose
	// box score is open, or "".
	cursor   int
	expanded string

	width, height int
	loaded        bool
	err           error
}

// New constructs the Games screen for a club.
func New(deps core.Deps, team models.Team) *Model {
	return &Model{deps: deps, team: team, width: 100, height: 30}
}

// Title implements core.Screen.
func (m *Model) Title() string { return "Games" }

// Keys implements core.Screen.
func (m *Model) Keys() core.KeyMap {
	return core.KeyMap{
		Short: []key.Binding{keys.Move, keys.BoxScore, keys.Opponent},
		Full: [][]key.Binding{
			{keys.Up, keys.Down, keys.PageUp, keys.PageDown},
			{keys.Home, keys.End},
			{keys.BoxScore, keys.Opponent},
		},
	}
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd { return m.load() }

// Games returns the listed games, newest first.
func (m *Model) Games() []models.Match { return m.games }

// Expanded is the ID of the game whose box score is open, or "".
func (m *Model) Expanded() string { return m.expanded }

// FavoriteTarget implements core.Favoritable: the club.
func (m *Model) FavoriteTarget() (core.FavoriteTarget, bool) {
	return core.FavoriteTarget{Kind: models.FavoriteTeam, ID: m.team.ID, Name: m.team.Name}, true
}

// RefreshRequest implements core.RefreshRequester: the club's recent
// games.
func (m *Model) RefreshRequest() (core.RefreshRequest, bool) {
	return core.RefreshRequest{TeamID: m.team.ID, Name: m.team.Name}, true
}

type loadedMsg struct {
	games []models.Match
	teams map[string]models.Team
	comps map[string]models.Competition
}

func (m *Model) load() tea.Cmd {
	deps, teamID := m.deps, m.team.ID
	return func() tea.Msg {
		games, err := deps.Repos.RecentMatches(deps.Ctx, teamID, Limit)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		teams, comps, err := core.MatchNames(deps, games)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		return loadedMsg{games: games, teams: teams, comps: comps}
	}
}

// Update implements core.Screen.
func (m *Model) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case core.ThemeChangedMsg:
		m.deps.Theme = msg.Theme
	case core.RefreshedMsg:
		return m, m.load()
	case loadedMsg:
		// A reload keeps the highlighted game where it can.
		var selected string
		if m.cursor < len(m.games) {
			selected = m.games[m.cursor].ID
		}
		m.games, m.teams, m.comps = msg.games, msg.teams, msg.comps
		m.loaded = true
		m.cursor = 0
		for i, g := range m.games {
			if g.ID == selected {
				m.cursor = i
			}
		}
	case core.ErrMsg:
		m.err = msg.Err
		m.loaded = true
	case tea.KeyMsg:
		return m, m.handleKey(msg)
	case core.ClickMsg:
		// A click selects a game; clicking the selected one opens or
		// closes its box score, as enter does.
		row, ok := msg.ClickedIndex("row")
		if !ok {
			return m, nil
		}
		if i, ok := m.gameAt(row); ok {
			if i == m.cursor {
				return m, m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			}
			m.cursor = i
		}
	}
	return m, nil
}

// gameAt is the game on a row of the table, which has an open game's box
// score rows under it; false for a box score row.
func (m *Model) gameAt(row int) (int, bool) {
	r := 0
	for i, g := range m.games {
		if r == row {
			return i, true
		}
		r++
		if g.ID == m.expanded {
			r += len(components.BoxScore(m.deps.Theme, g, m.team.ID, 4))
		}
	}
	return 0, false
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup":
		m.move(-10)
	case "pgdown":
		m.move(10)
	case "home", "g":
		m.move(-len(m.games))
	case "end", "G":
		m.move(len(m.games))
	case "enter":
		if g, ok := m.selected(); ok {
			if m.expanded == g.ID {
				m.expanded = ""
			} else {
				m.expanded = g.ID
			}
		}
	case "o":
		g, ok := m.selected()
		if !ok || m.deps.Nav.Team == nil {
			return nil
		}
		opp := g.Opponent(m.team.ID)
		team, ok := m.teams[opp]
		if !ok {
			team = models.Team{ID: opp, Name: opp}
		}
		return core.Push(m.deps.Nav.Team(m.deps, team, g.CompetitionID, g.SeasonID))
	}
	return nil
}

func (m *Model) move(delta int) {
	if len(m.games) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+delta, 0), len(m.games)-1)
}

func (m *Model) selected() (models.Match, bool) {
	if m.cursor < 0 || m.cursor >= len(m.games) {
		return models.Match{}, false
	}
	return m.games[m.cursor], true
}

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	title := th.Title.Render(m.deps.Star(models.FavoriteTeam, m.team.ID, m.team.Name))
	header := lipgloss.JoinHorizontal(lipgloss.Center, title, th.Subtitle.Render("  last games, every competition"))
	switch {
	case m.err != nil:
		return header + "\n\n" + th.Error.Render("Could not load games: "+m.err.Error())
	case !m.loaded:
		return header + "\n\n" + th.Muted.Render("Loading…")
	case len(m.games) == 0:
		return header + "\n\n" + th.Empty.Render("No games cached for "+m.team.Name+".") +
			th.Muted.Render("  Press ") + th.Accent.Render("r") + th.Muted.Render(" to fetch them.")
	}
	summary := models.SummarizeMatches(m.team.ID, m.games)
	record := th.Muted.Render(fmt.Sprintf("%d games · %dW %dD %dL · goals %d–%d · form %s",
		summary.Played, summary.Won, summary.Drawn, summary.Lost, summary.GoalsFor, summary.GoalsAgainst, summary.Form))
	return header + "\n" + record + "\n\n" + m.table()
}

func (m *Model) table() string {
	th := m.deps.Theme
	lastWidth := max(m.width-(1+1+10+1+6+1+3+1+24+1+15+1)-4, components.MatchXGWidth)
	cols := append(components.MatchColumns(24), components.Column{Title: "", Width: lastWidth})
	var rows [][]components.Cell
	cursor := -1
	for i, g := range m.games {
		if i == m.cursor {
			cursor = len(rows)
		}
		last := g.Round
		if xg := components.MatchXGBar(th, g, m.team.ID); xg != "" {
			last = xg
		}
		rows = append(rows, components.MatchCells(th, g, m.team.ID, m.comp(g.CompetitionID), m.name(g.Opponent(m.team.ID)), last))
		if g.ID == m.expanded {
			rows = append(rows, components.BoxScore(th, g, m.team.ID, 4)...)
		}
	}
	return components.Table(components.TableOpts{
		Columns: cols, Rows: rows, Cursor: cursor, MaxRows: max(m.height-8, 5), Theme: th,
		Zone: "row",
	})
}

func (m *Model) comp(id string) models.Competition {
	if c, ok := m.comps[id]; ok {
		return c
	}
	return models.Competition{ID: id, Name: id}
}

func (m *Model) name(id string) string {
	if t, ok := m.teams[id]; ok {
		return t.Name
	}
	return id
}
