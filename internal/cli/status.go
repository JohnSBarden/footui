package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

func newStatusCmd(g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show what's in the local cache and how fresh it is",
		Long: "status reports which competitions are cached, how many seasons and teams each " +
			"holds, and when it was last refreshed.\n\n" +
			"footui is built around a durable local cache, so knowing what's actually in it — " +
			"and which competitions are running stale — shouldn't require opening the interface " +
			"and looking.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, store, err := g.openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			summary, err := store.CacheSummary(cmdContext(cmd))
			if err != nil {
				return err
			}

			printStatus(cmd.OutOrStdout(), summary, g.dbPathOrDefault(), time.Now())
			return nil
		},
	}
}

func printStatus(out io.Writer, summary []models.CompetitionCache, dbPath string, now time.Time) {
	fmt.Fprintf(out, "Database: %s", dbPath)
	if size, ok := fileSize(dbPath); ok {
		fmt.Fprintf(out, " (%s)", humanBytes(size))
	}
	fmt.Fprintln(out)

	if len(summary) == 0 {
		fmt.Fprintln(out, "\nNothing cached yet. Run `footui refresh --all` to populate it.")
		return
	}

	fmt.Fprintf(out, "\n%-22s %-16s %6s %9s  %s\n",
		"COMPETITION", "SEASONS", "TEAMS", "PLAYERS", "LAST REFRESH")

	var totalPlayers int
	stale := 0
	for _, e := range summary {
		seasons := strconv.Itoa(e.Seasons)
		if e.Seasons > 1 && e.EarliestSeason != "" {
			// A count alone doesn't say which seasons; the range does.
			seasons = fmt.Sprintf("%s..%s",
				components.SeasonLabelFor(e.Competition.CalendarYear, e.EarliestSeason), components.SeasonLabelFor(e.Competition.CalendarYear, e.LatestSeason))
		} else if e.Seasons == 1 && e.LatestSeason != "" {
			seasons = components.SeasonLabelFor(e.Competition.CalendarYear, e.LatestSeason)
		}

		fmt.Fprintf(out, "%-22s %-16s %6d %9d  %s\n",
			e.Competition.ID, seasons, e.Teams, e.PlayerSeasons, refreshNote(e.LastSync, now))

		totalPlayers += e.PlayerSeasons
		if e.LastSync != nil && !e.LastSync.Succeeded() {
			stale++
		}
	}

	fmt.Fprintf(out, "\n%s, %d player-season rows\n", plural(len(summary), "competition"), totalPlayers)
	if stale > 0 {
		fmt.Fprintf(out, "%s last failed to refresh — see the note above.\n",
			plural(stale, "competition"))
	}
}

// refreshNote describes a competition's last refresh. A failure is
// reported as a failure rather than as age: "we tried and it broke" is
// what the user needs to act on.
func refreshNote(run *models.SyncRun, now time.Time) string {
	if run == nil {
		return "never"
	}
	ago := components.Freshness(run.FinishedAt, now)
	if !run.Succeeded() {
		return "FAILED " + ago
	}
	return ago
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func fileSize(path string) (int64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return info.Size(), true
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < 3 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
