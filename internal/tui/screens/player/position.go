package player

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/playerstats"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// enrichHistory attaches what the page judges a position by: each
// season's detail, a keeper's estimated xGA, and the numbers unusual for
// the player's group. It returns that group, and whether any season has
// detail to show.
func enrichHistory(deps core.Deps, p models.Player, stats []models.PlayerSeasonStats, history map[string][]SeasonRow) (playerstats.Group, bool, error) {
	details, err := deps.Repos.PlayerSeasonDetails(deps.Ctx, p.ID)
	if err != nil {
		return 0, false, err
	}
	byKey := make(map[string]*models.PlayerSeasonDetail, len(details))
	espnPos := ""
	for i := range details {
		d := &details[i]
		byKey[d.TeamID+"|"+d.CompetitionID+"|"+d.SeasonID] = d
		if espnPos == "" {
			espnPos = d.Position // newest season first
		}
	}
	group := playerstats.GroupOf(espnPos, p.Position, stats)

	clubSeasons := map[string]map[string]models.TeamSeasonStats{}
	clubSeason := func(team, comp, season string) (models.TeamSeasonStats, bool, error) {
		seasons, ok := clubSeasons[team]
		if !ok {
			rows, err := deps.Repos.TeamHistory(deps.Ctx, team)
			if err != nil {
				return models.TeamSeasonStats{}, false, err
			}
			seasons = map[string]models.TeamSeasonStats{}
			for _, r := range rows {
				seasons[r.CompetitionID+"|"+r.SeasonID] = r
			}
			clubSeasons[team] = seasons
		}
		ts, ok := seasons[comp+"|"+season]
		return ts, ok, nil
	}

	hasDetail := false
	for comp, rows := range history {
		for i := range rows {
			r := &rows[i]
			s := r.Stats
			r.Detail = byKey[s.TeamID+"|"+s.CompetitionID+"|"+s.SeasonID]
			hasDetail = hasDetail || r.Detail != nil
			if group == playerstats.GK && r.Detail != nil {
				ts, ok, err := clubSeason(s.TeamID, s.CompetitionID, s.SeasonID)
				if err != nil {
					return 0, false, err
				}
				if xga, est := playerstats.KeeperExpected(minutesOf(*r), ts); ok && est {
					r.XGA = &xga
				}
			}
			standouts, err := seasonStandouts(deps, s, group)
			if err != nil {
				return 0, false, err
			}
			r.Standouts = standouts
		}
		history[comp] = rows
	}
	return group, hasDetail, nil
}

// seasonStandouts compares one season with everyone else's in the same
// competition-season.
func seasonStandouts(deps core.Deps, s models.PlayerSeasonStats, g playerstats.Group) ([]playerstats.Standout, error) {
	if s.Minutes < 900 {
		return nil, nil // can't stand out; spare the queries
	}
	lines, err := deps.Repos.SeasonPlayerLines(deps.Ctx, s.CompetitionID, s.SeasonID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(lines))
	byPlayer := map[string][]models.PlayerSeasonStats{}
	for _, l := range lines {
		if _, seen := byPlayer[l.PlayerID]; !seen {
			ids = append(ids, l.PlayerID)
		}
		byPlayer[l.PlayerID] = append(byPlayer[l.PlayerID], l)
	}
	players, err := deps.Repos.GetPlayers(deps.Ctx, ids)
	if err != nil {
		return nil, err
	}
	groups := make(map[string]playerstats.Group, len(players))
	for id, p := range players {
		groups[id] = playerstats.GroupOf("", p.Position, byPlayer[id])
	}
	groups[s.PlayerID] = g
	return playerstats.Standouts(s, g, lines, groups), nil
}

// minutesOf is the season's minutes, the detail's when it has them.
func minutesOf(r SeasonRow) int {
	if r.Detail != nil && r.Detail.Minutes != nil && *r.Detail.Minutes > 0 {
		return *r.Detail.Minutes
	}
	return r.Stats.Minutes
}

func appsOf(r SeasonRow) int {
	if r.Detail != nil && r.Detail.Appearances != nil && *r.Detail.Appearances > 0 {
		return *r.Detail.Appearances
	}
	return r.Stats.Appearances
}

// per90 is a count as a rate per 90 minutes, or "—" when it wasn't
// recorded or the minutes are too few to mean anything.
func per90(n *int, minutes int) string {
	if n == nil || minutes < 90 {
		return "—"
	}
	return strconv.FormatFloat(float64(*n)*90/float64(minutes), 'f', 2, 64)
}

func sumPtr(a, b *int) *int {
	if a == nil || b == nil {
		return nil
	}
	s := *a + *b
	return &s
}

func pct(p *float64) string {
	if p == nil {
		return "—"
	}
	return strconv.Itoa(int(*p*100+0.5)) + "%"
}

func ratio(num, den int) string {
	if den <= 0 {
		return "—"
	}
	return strconv.Itoa(int(float64(num)*100/float64(den)+0.5)) + "%"
}

// savePct is saves over shots on target: ESPN's shots-faced count can't be
// trusted, but every shot on target was either saved or went in.
func savePct(d *models.PlayerSeasonDetail) string {
	if d == nil || d.Saves == nil || d.GoalsConceded == nil {
		return "—"
	}
	return ratio(*d.Saves, *d.Saves+*d.GoalsConceded)
}

func countOr(n *int) string {
	if n == nil {
		return "—"
	}
	return strconv.Itoa(*n)
}

// positionCards are the selected season's tiles for the player's group.
func (m *Model) positionCards(r SeasonRow) []components.Stat {
	d, s := r.Detail, r.Stats
	mins := minutesOf(r)
	notRecorded := func(st components.Stat, n *int) components.Stat {
		if n == nil {
			st.Value, st.Hint = "—", "not recorded"
		}
		return st
	}
	cleanSheets := components.Stat{Label: "Clean sheets", Value: countOr(d.CleanSheets),
		Hint: fmt.Sprintf("in %d apps", appsOf(r))}
	passing := components.Stat{Label: "Passing", Value: pct(d.PassPct), Hint: "completed"}
	if d.PassPct == nil {
		passing.Hint = "not recorded"
	}
	expected := func(count int, x *float64, noun string) string {
		if x == nil {
			return per90Hint(count, mins)
		}
		return "from " + components.Float(x, 1) + " " + noun
	}

	switch m.group {
	case playerstats.GK:
		if d.CleanSheets != nil && appsOf(r) > 0 {
			cleanSheets.Hint += " · " + ratio(*d.CleanSheets, appsOf(r))
		}
		stats := []components.Stat{
			cleanSheets,
			notRecorded(components.Stat{Label: "Saves", Value: countOr(d.Saves), Hint: savePct(d) + " of shots on target"}, d.Saves),
			notRecorded(components.Stat{Label: "Conceded", Value: countOr(d.GoalsConceded), Hint: per90(d.GoalsConceded, mins) + " per 90"}, d.GoalsConceded),
		}
		if r.XGA != nil && d.GoalsConceded != nil {
			diff := *r.XGA - float64(*d.GoalsConceded)
			style := m.deltaStyle(diff)
			hint := "prevented · xGA " + components.Float(r.XGA, 1) + "*"
			if diff < 0 {
				hint = "extra · xGA " + components.Float(r.XGA, 1) + "*"
			}
			stats = append(stats, components.Stat{Label: "vs expected", Value: components.SignedFloat(diff, 1), Hint: hint, Style: &style})
		}
		return stats
	case playerstats.DEF:
		return []components.Stat{
			cleanSheets,
			notRecorded(components.Stat{Label: "Tackles", Value: per90(d.Tackles, mins), Hint: "per 90 · " + countOr(d.Tackles)}, d.Tackles),
			notRecorded(components.Stat{Label: "Interceptions", Value: per90(d.Interceptions, mins), Hint: "per 90 · " + countOr(d.Interceptions)}, d.Interceptions),
			notRecorded(components.Stat{Label: "Clearances", Value: per90(d.Clearances, mins), Hint: "per 90 · " + countOr(d.Clearances)}, d.Clearances),
			passing,
		}
	case playerstats.MID:
		won := sumPtr(d.Tackles, d.Interceptions)
		return []components.Stat{
			{Label: "Goals", Value: strconv.Itoa(s.Goals), Hint: expected(s.Goals, s.XG, "xG")},
			{Label: "Assists", Value: strconv.Itoa(s.Assists), Hint: expected(s.Assists, s.XA, "xA")},
			notRecorded(components.Stat{Label: "Key passes", Value: per90(s.KeyPasses, s.Minutes), Hint: "per 90 · " + countOr(s.KeyPasses)}, s.KeyPasses),
			notRecorded(components.Stat{Label: "Tackles + int.", Value: per90(won, mins), Hint: "per 90 · " + countOr(won)}, won),
			passing,
		}
	}
	onTarget := components.Stat{Label: "On target", Value: "—", Hint: "not recorded"}
	if d.ShotsOnTarget != nil && s.Shots != nil && *s.Shots > 0 {
		onTarget.Value, onTarget.Hint = ratio(*d.ShotsOnTarget, *s.Shots), fmt.Sprintf("%d of %d", *d.ShotsOnTarget, *s.Shots)
	}
	return []components.Stat{
		{Label: "Goals", Value: strconv.Itoa(s.Goals), Hint: expected(s.Goals, s.XG, "xG")},
		{Label: "Assists", Value: strconv.Itoa(s.Assists), Hint: expected(s.Assists, s.XA, "xA")},
		notRecorded(components.Stat{Label: "Shots", Value: per90(s.Shots, s.Minutes), Hint: "per 90 · " + countOr(s.Shots)}, s.Shots),
		onTarget,
		{Label: "Minutes", Value: strconv.Itoa(s.Minutes), Hint: strconv.Itoa(s.Appearances) + " apps"},
	}
}

// callout names the selected season's numbers unusual for the position,
// a line each.
func (m *Model) callout(r SeasonRow) string {
	if len(r.Standouts) == 0 {
		return ""
	}
	th := m.deps.Theme
	comp, _ := m.CurrentCompetition()
	head := th.Accent.Bold(true).Render("▲ unusual for a " + m.group.Singular())
	var lines []string
	for _, st := range r.Standouts {
		value := strconv.Itoa(int(st.Value))
		if st.Stat == "xG" || st.Stat == "xA" {
			value = strconv.FormatFloat(st.Value, 'f', 1, 64)
		}
		lines = append(lines, head+"  "+th.TableCell.Bold(true).Render(value+" "+st.Stat)+
			th.Muted.Render(fmt.Sprintf(" · %.2f per 90 · top %d%% of %s %s", st.Per90, st.TopPct, comp.Name, m.group.Plural())))
	}
	return strings.Join(lines, "\n")
}

// cleanSheetChart is a keeper's or defender's clean sheets by season,
// oldest first, for the band.
func (m *Model) cleanSheetChart() []string {
	rows := m.CurrentRows() // newest first
	type point struct {
		season   string
		cs, apps int
	}
	var pts []point
	scale := 0
	for i := len(rows) - 1; i >= 0; i-- {
		d := rows[i].Detail
		if d == nil || d.CleanSheets == nil {
			continue
		}
		pts = append(pts, point{components.SeasonName(rows[i].Season), *d.CleanSheets, appsOf(rows[i])})
		scale = max(scale, *d.CleanSheets)
	}
	if len(pts) > chartSeasons {
		pts = pts[len(pts)-chartSeasons:]
	}
	if len(pts) == 0 {
		return nil
	}
	out := []string{"CLEAN SHEETS"}
	for _, p := range pts {
		bar := components.Bar(float64(p.cs), float64(max(scale, 1)), chartBarWidth)
		out = append(out, fmt.Sprintf("%-7s %s %d in %d", p.season,
			bar+strings.Repeat(" ", chartBarWidth-lipgloss.Width(bar)), p.cs, p.apps))
	}
	return padLines(out)
}

// padLines pads every line to the widest, so a right-aligned block keeps
// its columns straight.
func padLines(out []string) []string {
	width := 0
	for _, l := range out {
		width = max(width, lipgloss.Width(l))
	}
	for i, l := range out {
		out[i] = l + strings.Repeat(" ", width-lipgloss.Width(l))
	}
	return out
}

// positionTable is the season-by-season columns for a keeper, defender or
// midfielder with detail: the base columns, the optional groups dropped
// right to left on a narrow terminal, and each row's cells in that order.
// ok is false for a forward or a player without detail, who keep the
// standard table.
func (m *Model) positionTable(rows []SeasonRow) (base []components.Column, extra [][]components.Column, cells [][]components.Cell, ok bool) {
	if !m.hasDetail || m.group == playerstats.FWD {
		return nil, nil, nil, false
	}
	th := m.deps.Theme
	base = []components.Column{
		{Title: "Season", Width: 8},
		{Title: "Team", Width: wantTeamCol},
		{Title: "Apps", Width: 5, Right: true},
		{Title: "Mins", Width: 5, Right: true},
	}
	col := func(title string, w int) components.Column {
		return components.Column{Title: title, Width: w, Right: true}
	}
	for _, r := range rows {
		s, d := r.Stats, r.Detail
		if d == nil {
			d = &models.PlayerSeasonDetail{}
		}
		mins := minutesOf(r)
		row := []components.Cell{
			components.Text(components.SeasonName(r.Season)),
			components.Text(r.Team.Name),
			components.Text(strconv.Itoa(s.Appearances)),
			components.Text(strconv.Itoa(s.Minutes)),
		}
		switch m.group {
		case playerstats.GK:
			pm := components.Text("—")
			if r.XGA != nil && d.GoalsConceded != nil {
				diff := *r.XGA - float64(*d.GoalsConceded)
				pm = components.Styled(components.SignedFloat(diff, 1), m.deltaStyle(diff))
			}
			row = append(row,
				components.Styled(countOr(d.CleanSheets), th.Accent.Bold(true)), components.Text(countOr(d.GoalsConceded)),
				components.Text(countOr(d.Saves)), components.Text(savePct(r.Detail)),
				components.Text(components.Float(r.XGA, 1)), pm)
		case playerstats.DEF:
			row = append(row,
				components.Styled(countOr(d.CleanSheets), th.Accent.Bold(true)), components.Text(countOr(d.GoalsConceded)),
				components.Text(per90(d.Tackles, mins)), components.Text(per90(d.Interceptions, mins)), components.Text(per90(d.Clearances, mins)),
				components.Text(pct(d.PassPct)),
				components.Styled(strconv.Itoa(s.Goals), toneOr(th, s.Goals, s.XG, th.TableCell)), components.Text(components.Float(s.XG, 1)))
		case playerstats.MID:
			row = append(row,
				components.Styled(strconv.Itoa(s.Goals), toneOr(th, s.Goals, s.XG, th.Accent.Bold(true))),
				components.Styled(strconv.Itoa(s.Assists), toneOr(th, s.Assists, s.XA, th.TableCell)),
				components.Text(components.Float(s.XG, 1)), components.Text(components.Float(s.XA, 1)),
				components.Text(per90(sumPtr(d.Tackles, d.Interceptions), mins)), components.Text(pct(d.PassPct)),
				components.Text(components.Int(s.Shots)), components.Text(components.Int(s.KeyPasses)))
		}
		cells = append(cells, row)
	}
	switch m.group {
	case playerstats.GK:
		base = append(base, col("CS", 4), col("GA", 4))
		extra = [][]components.Column{{col("Saves", 6), col("Sv%", 5)}, {col("xGA*", 6), col("±", 6)}}
	case playerstats.DEF:
		base = append(base, col("CS", 4), col("GA", 4))
		extra = [][]components.Column{{col("Tkl/90", 7), col("Int/90", 7), col("Clr/90", 7)}, {col("Pass", 5)}, {col("G", 4), col("xG", 6)}}
	case playerstats.MID:
		base = append(base, col("G", 4), col("A", 4))
		extra = [][]components.Column{{col("xG", 6), col("xA", 6)}, {col("T+I/90", 7), col("Pass", 5)}, {col("Sh", 4), col("KP", 4)}}
	}
	return base, extra, cells, true
}

// footnotes explain the table's marks, only those it uses.
func (m *Model) footnotes(rows []SeasonRow) []string {
	th := m.deps.Theme
	var notes []string
	estimated, unrecorded := false, false
	for _, r := range rows {
		estimated = estimated || r.XGA != nil
		if r.Detail != nil && r.Detail.Tackles == nil && (m.group == playerstats.DEF || m.group == playerstats.MID) {
			unrecorded = true
		}
	}
	if estimated && m.group == playerstats.GK {
		notes = append(notes, th.Muted.Render("* the club's xGA, scaled by their share of minutes"))
	}
	if unrecorded {
		notes = append(notes, th.Muted.Render("— no tackles, interceptions or clearances recorded that season"))
	}
	return notes
}

// detailStatus says whether the player's detail is being refreshed, or
// how old it is.
func (m *Model) detailStatus() string {
	th := m.deps.Theme
	if m.refreshing {
		return th.Status.Render("🔄 refreshing…")
	}
	if !m.hasDetail {
		return ""
	}
	var newest time.Time
	for _, rows := range m.history {
		for _, r := range rows {
			if r.Detail != nil && r.Detail.FetchedAt.After(newest) {
				newest = r.Detail.FetchedAt
			}
		}
	}
	return th.Muted.Render("✓ updated " + components.Freshness(newest, time.Now()))
}
