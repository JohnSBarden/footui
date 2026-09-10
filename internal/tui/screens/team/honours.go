package team

import (
	"sort"
	"strconv"
	"strings"

	"github.com/JohnSBarden/footui/internal/trophies"
)

// honourLine is one aggregated row of the honours panel: a title, how many
// times it was won, and the seasons.
type honourLine struct {
	Title   string
	Type    string
	Seasons []string
}

// Honours groups the club's trophy rows into display lines. Rows are
// stored one per win (the schema's grain), but an honours list reads as
// "Premier League ×3", so they're aggregated here rather than in SQL —
// the grouping is a presentation decision, not a storage one.
func (m *Model) Honours() []honourLine {
	if len(m.honours) == 0 {
		return nil
	}

	byTitle := map[string]*honourLine{}
	var order []string
	for _, t := range m.honours {
		line, ok := byTitle[t.Title]
		if !ok {
			line = &honourLine{Title: t.Title, Type: t.TrophyType}
			byTitle[t.Title] = line
			order = append(order, t.Title)
		}
		if t.SeasonLabel != "" {
			line.Seasons = append(line.Seasons, t.SeasonLabel)
		}
	}

	lines := make([]honourLine, 0, len(order))
	for _, title := range order {
		line := byTitle[title]
		// Most recent first, matching how the seasons tables read.
		sort.Sort(sort.Reverse(sort.StringSlice(line.Seasons)))
		lines = append(lines, *line)
	}

	// Continental honours first — they're the ones a reader looks for.
	sort.SliceStable(lines, func(i, j int) bool {
		return honourRank(lines[i].Type) < honourRank(lines[j].Type)
	})
	return lines
}

func honourRank(trophyType string) int {
	switch trophyType {
	case "continental_cup":
		return 0
	case "league_title":
		return 1
	case "domestic_cup":
		return 2
	default:
		return 3
	}
}

// honoursPanel renders the club's honours, or nothing at all when the
// curated dataset doesn't cover this club. An empty panel would imply the
// club has won nothing, which is a different and probably false claim.
func (m *Model) honoursPanel() string {
	lines := m.Honours()
	if len(lines) == 0 {
		return ""
	}
	th := m.theme

	var b strings.Builder
	b.WriteString(th.PanelTitle.Render("Honours"))
	// The league-title lists start at a fixed season, so the panel says so
	// rather than letting a partial count read as an all-time total.
	b.WriteString(th.Muted.Render("  league titles from " + trophies.LeagueTitlesFrom + " only"))
	b.WriteString("\n")

	for _, line := range lines {
		label := line.Title
		if n := len(line.Seasons); n > 1 {
			label += " ×" + strconv.Itoa(n)
		}
		b.WriteString("  ")
		b.WriteString(th.Accent.Render(label))
		if len(line.Seasons) > 0 {
			b.WriteString(th.Muted.Render("  " + m.seasonList(line.Seasons)))
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// maxSeasonsShown caps how many years one honour line lists before
// summarizing; a club with eighteen league titles would otherwise wrap
// across the whole terminal.
const maxSeasonsShown = 8

func (m *Model) seasonList(seasons []string) string {
	if len(seasons) <= maxSeasonsShown {
		return strings.Join(seasons, ", ")
	}
	shown := strings.Join(seasons[:maxSeasonsShown], ", ")
	return shown + ", +" + strconv.Itoa(len(seasons)-maxSeasonsShown) + " more"
}
