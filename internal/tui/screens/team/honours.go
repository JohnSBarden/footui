package team

import (
	"sort"
	"strconv"
	"strings"

	"github.com/JohnSBarden/footui/internal/trophies"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

// honourLine is one aggregated row of the honours panel.
type honourLine struct {
	Title   string
	Type    string
	Seasons []string
}

// Honours aggregates one-row-per-win trophies into display lines.
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
		sort.Sort(sort.Reverse(sort.StringSlice(line.Seasons)))
		lines = append(lines, *line)
	}

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

// honoursPanel renders nothing for an uncovered club: an empty panel would
// falsely imply it has won nothing.
func (m *Model) honoursPanel() string {
	lines := m.Honours()
	if len(lines) == 0 {
		return ""
	}
	th := m.theme

	var b strings.Builder
	b.WriteString(components.Label(th, "Honours"))
	// League-title data is partial; don't let it read as an all-time count.
	b.WriteString(th.Muted.Render("  league titles from " + trophies.LeagueTitlesFrom + " only"))
	b.WriteString("\n")

	for _, line := range lines {
		label := line.Title
		if n := len(line.Seasons); n > 1 {
			label += " ×" + strconv.Itoa(n)
		}
		b.WriteString("  ")
		b.WriteString(th.Draw.Render(diamonds(max(len(line.Seasons), 1))) + " ")
		b.WriteString(th.Accent.Render(label))
		if len(line.Seasons) > 0 {
			b.WriteString(th.Muted.Render("  " + m.seasonList(line.Seasons)))
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// maxDiamonds is the longest ◆ tally before switching to ◆×n.
const maxDiamonds = 10

func diamonds(n int) string {
	if n > maxDiamonds {
		return "◆×" + strconv.Itoa(n)
	}
	return strings.Repeat("◆", n)
}

// maxSeasonsShown caps the years listed per honour line.
const maxSeasonsShown = 8

func (m *Model) seasonList(seasons []string) string {
	if len(seasons) <= maxSeasonsShown {
		return strings.Join(seasons, ", ")
	}
	shown := strings.Join(seasons[:maxSeasonsShown], ", ")
	return shown + ", +" + strconv.Itoa(len(seasons)-maxSeasonsShown) + " more"
}
