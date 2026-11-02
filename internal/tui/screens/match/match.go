// Package match shows one game in full: its timeline, every team stat,
// both lineups and the commentary.
package match

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// Sections of the screen, in tab order.
const (
	TabTimeline = iota
	TabStats
	TabLineups
	TabCommentary
)

var tabNames = []string{"Timeline", "Stats", "Lineups", "Commentary"}

// Model is the Match screen.
type Model struct {
	deps   core.Deps
	match  models.Match
	teamID string // the side the screen was opened from, "" for neither
	detail models.MatchDetail
	have   bool // a detail is stored
	teams  map[string]models.Team
	comps  map[string]models.Competition

	tab    int
	scroll int

	width, height int
	loaded        bool
	err           error
}

// New constructs the Match screen for m, seen from teamID's side.
func New(deps core.Deps, m models.Match, teamID string) *Model {
	return &Model{deps: deps, match: m, teamID: teamID, width: 100, height: 30}
}

// Title implements core.Screen.
func (m *Model) Title() string { return "Match" }

// Keys implements core.Screen.
func (m *Model) Keys() core.KeyMap {
	return core.KeyMap{
		Short: []key.Binding{keys.MatchTabs, keys.Move, keys.Opponent},
		Full: [][]key.Binding{
			{keys.MatchTabs, keys.Opponent},
			{keys.Up, keys.Down, keys.PageUp, keys.PageDown},
			{keys.Home, keys.End},
		},
	}
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd { return m.load() }

// Tab is the section on show.
func (m *Model) Tab() int { return m.tab }

// RefreshRequest implements core.RefreshRequester: the club's recent
// games, which end with its last game in full.
func (m *Model) RefreshRequest() (core.RefreshRequest, bool) {
	if m.teamID == "" {
		return core.RefreshRequest{}, false
	}
	t := m.teams[m.teamID]
	return core.RefreshRequest{TeamID: m.teamID, Name: t.Name}, true
}

type loadedMsg struct {
	detail models.MatchDetail
	have   bool
	teams  map[string]models.Team
	comps  map[string]models.Competition
}

func (m *Model) load() tea.Cmd {
	deps, match := m.deps, m.match
	return func() tea.Msg {
		teams, comps, err := core.MatchNames(deps, []models.Match{match})
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		d, err := deps.Repos.MatchDetail(deps.Ctx, match.ID)
		switch {
		case errors.Is(err, storage.ErrNotFound):
			return loadedMsg{teams: teams, comps: comps}
		case err != nil:
			return core.ErrMsg{Err: err}
		}
		return loadedMsg{detail: d, have: true, teams: teams, comps: comps}
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
		m.detail, m.have, m.teams, m.comps = msg.detail, msg.have, msg.teams, msg.comps
		m.loaded = true
	case core.ErrMsg:
		m.err = msg.Err
		m.loaded = true
	case core.ClickMsg:
		if i, ok := msg.ClickedIndex("tab"); ok {
			m.setTab(i)
		}
	case tea.KeyMsg:
		return m, m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) setTab(i int) {
	m.tab = (i%len(tabNames) + len(tabNames)) % len(tabNames)
	m.scroll = 0
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	page := max(m.bodyHeight()-1, 1)
	switch s := msg.String(); s {
	case "tab", "]":
		m.setTab(m.tab + 1)
	case "shift+tab", "[":
		m.setTab(m.tab - 1)
	case "o":
		return m.openOpponent()
	case "1", "2", "3", "4":
		m.setTab(int(s[0] - '1'))
	case "up", "k":
		m.scrollBy(-1)
	case "down", "j":
		m.scrollBy(1)
	case "pgup":
		m.scrollBy(-page)
	case "pgdown":
		m.scrollBy(page)
	case "home", "g":
		m.scroll = 0
	case "end", "G":
		m.scrollBy(1 << 20)
	}
	return nil
}

// openOpponent opens the side the screen wasn't opened from; with no
// side, the away team.
func (m *Model) openOpponent() tea.Cmd {
	if m.deps.Nav.Team == nil {
		return nil
	}
	opp := m.match.AwayTeamID
	if m.teamID != "" {
		opp = m.match.Opponent(m.teamID)
	}
	team, ok := m.teams[opp]
	if !ok {
		team = models.Team{ID: opp, Name: opp}
	}
	return core.Push(m.deps.Nav.Team(m.deps, team, m.match.CompetitionID, m.match.SeasonID))
}

func (m *Model) scrollBy(delta int) {
	maxScroll := max(len(m.body())-m.bodyHeight(), 0)
	m.scroll = min(max(m.scroll+delta, 0), maxScroll)
}

// --- view ---

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	switch {
	case m.err != nil:
		return th.Error.Render("Could not load this game: " + m.err.Error())
	case !m.loaded:
		return th.Muted.Render("Loading…")
	}
	head := m.header()
	if !m.available() {
		return head + "\n\n" + m.emptyState()
	}
	body := m.body()
	h := m.bodyHeight()
	start := min(m.scroll, max(len(body)-h, 0))
	end := min(start+h, len(body))
	view := strings.Join(body[start:end], "\n")
	if hidden := len(body) - end; hidden > 0 {
		view += "\n" + th.Faint.Render(fmt.Sprintf("  ↓ %d more", hidden))
	}
	return head + "\n\n" + m.tabs() + "\n\n" + view
}

func (m *Model) available() bool {
	return m.have && !m.detail.Empty()
}

func (m *Model) emptyState() string {
	th := m.deps.Theme
	if m.have {
		return th.Empty.Render("No timeline, stats or lineups are available for this game.")
	}
	return th.Empty.Render("This game hasn't been fetched in full yet.") +
		th.Muted.Render("  Press ") + th.Accent.Render("r") + th.Muted.Render(" to fetch it.")
}

func (m *Model) tabs() string {
	tabs := make([]components.Tab, len(tabNames))
	for i, n := range tabNames {
		tabs[i] = components.Tab{Label: fmt.Sprintf("%d %s", i+1, n)}
	}
	return components.Tabs(m.deps.Theme, tabs, m.tab)
}

// bodyHeight is what's left for the section under the header and tabs.
func (m *Model) bodyHeight() int {
	used := components.Lines(m.header()) + 1 + 1 + 1 + 1 // gap, tabs, gap, "more"
	return max(m.height-used, 3)
}

func (m *Model) name(id string) string {
	if t, ok := m.teams[id]; ok {
		return t.Name
	}
	return id
}

// side is the screen's side as the detail names it.
func (m *Model) side() string {
	switch m.teamID {
	case m.match.HomeTeamID:
		return models.SideHome
	case m.match.AwayTeamID:
		return models.SideAway
	}
	return ""
}

func (m *Model) header() string {
	th := m.deps.Theme
	g := m.match
	score := th.Title.Render(m.name(g.HomeTeamID) + "  " + components.Score(g) + "  " + m.name(g.AwayTeamID))

	var info []string
	if c, ok := m.comps[g.CompetitionID]; ok {
		info = append(info, c.Name)
	}
	if g.Round != "" {
		info = append(info, g.Round)
	}
	info = append(info, g.Kickoff.Local().Format("Mon 2 Jan 2006"))
	if m.detail.Venue != "" {
		info = append(info, m.detail.Venue)
	}
	if m.detail.Attendance > 0 {
		info = append(info, commas(m.detail.Attendance))
	}
	lines := []string{score, th.Subtitle.Render(strings.Join(info, " · "))}
	if !m.available() {
		return strings.Join(lines, "\n")
	}

	home, away := components.Scorers(m.detail.Events, models.SideHome), components.Scorers(m.detail.Events, models.SideAway)
	if home != "" || away != "" {
		half := max((m.width-3)/2, 10)
		lines = append(lines, th.Muted.Render(padLeft(components.Truncate(home, half), half)+" "+
			components.GlyphGoal+" "+components.Truncate(away, half)))
	}
	if tl := components.Timeline(th, m.detail.Events, m.side(), min(m.width, 100)); tl != "" {
		lines = append(lines, tl)
	}
	return strings.Join(lines, "\n")
}

// body is the open section's lines.
func (m *Model) body() []string {
	switch m.tab {
	case TabStats:
		return m.statsLines()
	case TabLineups:
		return m.lineupLines()
	case TabCommentary:
		return m.commentaryLines()
	}
	return m.timelineLines()
}

// --- timeline ---

func (m *Model) timelineLines() []string {
	th := m.deps.Theme
	width := min(m.width, 100)
	const mid = 8
	half := max((width-mid)/2, 12)
	var out []string
	homeGoals, awayGoals := 0, 0
	for _, e := range m.detail.Events {
		switch e.Kind {
		case models.EventHalfTime, models.EventFullTime:
			label := "Half time"
			if e.Kind == models.EventFullTime {
				label = "Full time"
			}
			text := fmt.Sprintf(" %s %d–%d ", label, homeGoals, awayGoals)
			rule := max((half*2+mid-lipgloss.Width(text))/2, 2)
			out = append(out, th.Faint.Render(strings.Repeat("─", rule))+th.Muted.Render(text)+
				th.Faint.Render(strings.Repeat("─", rule)))
			continue
		}
		if e.IsGoal() && e.Period != 5 {
			if e.Side == models.SideHome {
				homeGoals++
			} else {
				awayGoals++
			}
		}
		text := m.eventText(e, homeGoals, awayGoals)
		clock := th.Muted.Render(center(e.Clock(), mid))
		if e.Side == models.SideHome {
			out = append(out, padLeft(components.Truncate(text, half), half)+clock)
		} else {
			out = append(out, strings.Repeat(" ", half)+clock+components.Truncate(text, half))
		}
	}
	if len(out) == 0 {
		out = append(out, th.Empty.Render("No goals, cards or substitutions were recorded."))
	}
	return out
}

// eventText reads the same either side: glyph first.
func (m *Model) eventText(e models.MatchEvent, home, away int) string {
	th := m.deps.Theme
	name := th.TableCell.Render(components.ShortName(e.Player))
	g := components.EventGlyph(th, e)
	switch e.Kind {
	case models.EventGoal, models.EventPenaltyGoal, models.EventOwnGoal:
		s := g + " " + th.TableCell.Bold(true).Render(components.ShortName(e.Player))
		switch {
		case e.Kind == models.EventPenaltyGoal:
			s += th.Muted.Render(" (pen)")
		case e.Kind == models.EventOwnGoal:
			s += th.Muted.Render(" (og)")
		case e.Other != "":
			s += th.Muted.Render(" (" + components.ShortName(e.Other) + ")")
		}
		if e.Period != 5 {
			s += "  " + th.Accent.Bold(true).Render(fmt.Sprintf("%d–%d", home, away))
		}
		return s
	case models.EventPenaltyMiss:
		return g + " " + name + th.Muted.Render(" penalty missed")
	case models.EventSecondYellow:
		return g + " " + name + th.Muted.Render(" second yellow")
	case models.EventRed:
		return g + " " + name + th.Muted.Render(" sent off")
	case models.EventSub:
		s := g + " " + name
		if e.Other != "" {
			s += th.Faint.Render(" for " + components.ShortName(e.Other))
		}
		return s
	}
	return g + " " + name
}

// --- stats ---

func count(f func(*models.MatchDetailStats) *int) func(*models.MatchDetailStats) (float64, string, bool) {
	return func(s *models.MatchDetailStats) (float64, string, bool) {
		if v := f(s); v != nil {
			return float64(*v), fmt.Sprint(*v), true
		}
		return 0, "", false
	}
}

// ofTotal shows "377/461", compared on the first.
func ofTotal(part, total func(*models.MatchDetailStats) *int) func(*models.MatchDetailStats) (float64, string, bool) {
	return func(s *models.MatchDetailStats) (float64, string, bool) {
		p, t := part(s), total(s)
		if p == nil || t == nil {
			return 0, "", false
		}
		return float64(*p), fmt.Sprintf("%d/%d", *p, *t), true
	}
}

// share shows part/total as a percentage, compared on it.
func share(part, total func(*models.MatchDetailStats) *int) func(*models.MatchDetailStats) (float64, string, bool) {
	return func(s *models.MatchDetailStats) (float64, string, bool) {
		p, t := part(s), total(s)
		if p == nil || t == nil || *t == 0 {
			return 0, "", false
		}
		pct := float64(*p) / float64(*t) * 100
		return pct, fmt.Sprintf("%.0f%%", pct), true
	}
}

var statRows = []struct {
	label string
	val   func(*models.MatchDetailStats) (float64, string, bool)
}{
	{"Possession", func(s *models.MatchDetailStats) (float64, string, bool) {
		if s.Possession == nil {
			return 0, "", false
		}
		return *s.Possession, fmt.Sprintf("%.0f%%", *s.Possession), true
	}},
	{"Shots", count(func(s *models.MatchDetailStats) *int { return s.Shots })},
	{"On target", count(func(s *models.MatchDetailStats) *int { return s.ShotsOnTarget })},
	{"Blocked", count(func(s *models.MatchDetailStats) *int { return s.BlockedShots })},
	{"Passes", count(func(s *models.MatchDetailStats) *int { return s.Passes })},
	{"Pass accuracy", share(func(s *models.MatchDetailStats) *int { return s.AccuratePasses }, func(s *models.MatchDetailStats) *int { return s.Passes })},
	{"Crosses", ofTotal(func(s *models.MatchDetailStats) *int { return s.AccurateCrosses }, func(s *models.MatchDetailStats) *int { return s.Crosses })},
	{"Long balls", ofTotal(func(s *models.MatchDetailStats) *int { return s.AccurateLongBalls }, func(s *models.MatchDetailStats) *int { return s.LongBalls })},
	{"Tackles won", ofTotal(func(s *models.MatchDetailStats) *int { return s.TacklesWon }, func(s *models.MatchDetailStats) *int { return s.Tackles })},
	{"Interceptions", count(func(s *models.MatchDetailStats) *int { return s.Interceptions })},
	{"Clearances", count(func(s *models.MatchDetailStats) *int { return s.Clearances })},
	{"Saves", count(func(s *models.MatchDetailStats) *int { return s.Saves })},
	{"Corners", count(func(s *models.MatchDetailStats) *int { return s.Corners })},
	{"Offsides", count(func(s *models.MatchDetailStats) *int { return s.Offsides })},
	{"Fouls", count(func(s *models.MatchDetailStats) *int { return s.Fouls })},
	{"Yellow cards", count(func(s *models.MatchDetailStats) *int { return s.YellowCards })},
	{"Red cards", count(func(s *models.MatchDetailStats) *int { return s.RedCards })},
}

func (m *Model) statsLines() []string {
	th := m.deps.Theme
	const valWidth, labelWidth = 9, 15
	bar := max(min(m.width-2*valWidth-labelWidth-4, 40), 8)
	home, away := m.detail.Home.Stats, m.detail.Away.Stats
	out := []string{th.Muted.Render(padLeft(components.Truncate(m.name(m.match.HomeTeamID), valWidth+bar/2), valWidth+bar/2) +
		strings.Repeat(" ", bar-bar/2+1) + components.Truncate(m.name(m.match.AwayTeamID), valWidth+bar/2))}
	for _, r := range statRows {
		hv, hs, hok := r.val(&home)
		av, as, aok := r.val(&away)
		if !hok && !aok {
			continue
		}
		if !hok {
			hs = "—"
		}
		if !aok {
			as = "—"
		}
		line := th.TableCell.Render(padLeft(hs, valWidth)) + " " +
			lipgloss.NewStyle().Width(bar).Render(components.SplitBar(th, hv, av, bar)) + " " +
			th.TableCell.Render(padRight(as, valWidth)) + " " + th.Muted.Render(r.label)
		out = append(out, line)
	}
	if len(out) == 1 {
		return []string{th.Empty.Render("No team stats were recorded for this game.")}
	}
	return out
}

// --- lineups ---

func (m *Model) lineupLines() []string {
	th := m.deps.Theme
	col := max((min(m.width, 110)-3)/2, 24)
	left := m.sideLineup(m.detail.Home, m.name(m.match.HomeTeamID), col)
	right := m.sideLineup(m.detail.Away, m.name(m.match.AwayTeamID), col)
	if len(left) == 0 && len(right) == 0 {
		return []string{th.Empty.Render("No lineups were recorded for this game.")}
	}
	n := max(len(left), len(right))
	out := make([]string, n)
	for i := range n {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = lipgloss.NewStyle().Width(col).MaxWidth(col).Render(l) + "   " + r
	}
	return out
}

func (m *Model) sideLineup(s models.MatchSide, team string, width int) []string {
	th := m.deps.Theme
	if len(s.Lineup) == 0 {
		return nil
	}
	title := th.PanelTitle.Render(components.Truncate(team, width-10))
	if s.Formation != "" {
		title += th.Muted.Render("  " + s.Formation)
	}
	out := []string{title}
	var used, unused []models.LineupPlayer
	for _, p := range s.Lineup {
		switch {
		case p.Starter:
			out = append(out, m.playerLine(p, width))
		case p.On > 0:
			used = append(used, p)
		default:
			unused = append(unused, p)
		}
	}
	if len(used)+len(unused) > 0 {
		out = append(out, "", components.Label(th, "Subs"))
	}
	for _, p := range used {
		out = append(out, m.playerLine(p, width))
	}
	for _, p := range unused {
		out = append(out, th.Faint.Render(fmt.Sprintf("%3s ", p.Shirt)+components.Truncate(p.Name, width-4)))
	}
	return out
}

func (m *Model) playerLine(p models.LineupPlayer, width int) string {
	th := m.deps.Theme
	var marks []string
	for range p.Goals {
		marks = append(marks, th.Positive.Bold(true).Render(components.GlyphGoal))
	}
	for range p.OwnGoals {
		marks = append(marks, th.Negative.Render(components.GlyphGoal+"og"))
	}
	if p.Assists > 0 {
		marks = append(marks, th.Accent.Render(strings.Repeat("A", min(p.Assists, 3))))
	}
	switch {
	case p.Red:
		marks = append(marks, th.Negative.Bold(true).Render(components.GlyphCard))
	case p.Yellow:
		marks = append(marks, th.Draw.Render(components.GlyphCard))
	}
	if p.On > 0 {
		marks = append(marks, th.Positive.Render(fmt.Sprintf("▲%d'", p.On)))
	}
	if p.Off > 0 {
		marks = append(marks, th.Muted.Render(fmt.Sprintf("▼%d'", p.Off)))
	}
	tail := strings.Join(marks, " ")
	room := max(width-4-lipgloss.Width(tail)-1, 6)
	return th.Muted.Render(fmt.Sprintf("%3s ", p.Shirt)) + th.TableCell.Render(components.Truncate(p.Name, room)) + " " + tail
}

// --- commentary ---

func (m *Model) commentaryLines() []string {
	th := m.deps.Theme
	const clockWidth = 7
	width := max(min(m.width, 110)-clockWidth, 20)
	var out []string
	for _, c := range m.detail.Commentary {
		style := th.TableCell
		switch kind := strings.ToLower(c.Kind); {
		case c.Kind == "":
			style = th.Muted
		case strings.HasPrefix(kind, "goal") || kind == "own goal" || kind == "penalty - scored":
			style = th.Positive.Bold(true)
		case kind == "yellow card":
			style = th.Draw
		case kind == "red card":
			style = th.Negative.Bold(true)
		}
		for i, line := range wrap(c.Text, width) {
			clock := ""
			if i == 0 {
				clock = c.Clock
			}
			out = append(out, th.Muted.Render(padRight(clock, clockWidth))+style.Render(line))
		}
	}
	if len(out) == 0 {
		return []string{th.Empty.Render("No commentary was recorded for this game.")}
	}
	return out
}

// --- text helpers ---

func wrap(s string, width int) []string {
	var lines []string
	var line strings.Builder
	for _, w := range strings.Fields(s) {
		if line.Len() > 0 && lipgloss.Width(line.String())+1+lipgloss.Width(w) > width {
			lines = append(lines, line.String())
			line.Reset()
		}
		if line.Len() > 0 {
			line.WriteByte(' ')
		}
		line.WriteString(w)
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return lines
}

func padLeft(s string, width int) string {
	return strings.Repeat(" ", max(width-lipgloss.Width(s), 0)) + s
}

func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}

func center(s string, width int) string {
	gap := max(width-lipgloss.Width(s), 0)
	return strings.Repeat(" ", gap/2) + s + strings.Repeat(" ", gap-gap/2)
}

func commas(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
