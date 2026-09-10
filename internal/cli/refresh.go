package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/colors"
	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider/understat"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/trophies"
)

type refreshFlags struct {
	competitionID string
	all           bool
	season        string
	years         int
	dryRun        bool
}

func newRefreshCmd(g *globalFlags) *cobra.Command {
	f := &refreshFlags{}

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Fetch and cache the latest stats from the data provider",
		Long: "refresh pulls team, standings, and player data from the understat.com provider " +
			"and stores it in the local SQLite cache.\n\n" +
			"By default it refreshes the current season of one competition. Use --all for every " +
			"competition footy covers, and --years to go back through previous seasons. A failure " +
			"in one competition-season does not abort the rest of the run; the summary at the end " +
			"lists exactly what failed.\n\n" +
			"See `footy competitions` for valid --competition values.",
		// A failed fetch or an unknown competition is a runtime problem,
		// not a usage error — printing the whole usage block on top of it
		// buries the actual message.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			comps, err := f.resolveCompetitions()
			if err != nil {
				return err
			}
			seasons := f.resolveSeasons(time.Now())

			out := cmd.OutOrStdout()
			if f.dryRun {
				printPlan(out, comps, seasons, g.dbPathOrDefault())
				return nil
			}

			db, store, err := g.openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			syncer := sync.New(understat.New(), store)

			fmt.Fprintf(out, "Refreshing %d competition-season(s)…\n\n", len(comps)*len(seasons))
			// Print each unit as it lands rather than batching the whole
			// report to the end: a full refresh is mostly network wait,
			// and silence for minutes is indistinguishable from a hang.
			results := syncer.SyncMany(cmdContext(cmd), comps, seasons, func(u sync.UnitResult) {
				printUnit(out, u)
			})

			// Curated colors and honours are re-seeded after every refresh
			// so a newly cached competition picks up its clubs' branding
			// and trophies with no extra step. Neither may fail the
			// refresh: the stats are the point, these are decoration.
			if colorReport, err := colors.Seed(cmdContext(cmd), store); err != nil {
				fmt.Fprintf(out, "\n  note: could not apply club colors: %v\n", err)
			} else if colorReport.Applied > 0 {
				fmt.Fprintf(out, "\n  colors applied to %d of %d cached teams\n",
					colorReport.Applied, colorReport.Applied+len(colorReport.Uncolored))
			}

			if trophyReport, err := trophies.Seed(cmdContext(cmd), store); err != nil {
				fmt.Fprintf(out, "  note: could not apply club honours: %v\n", err)
			} else if trophyReport.Teams > 0 {
				fmt.Fprintf(out, "  honours applied to %d teams (%d trophies)\n",
					trophyReport.Teams, trophyReport.Trophies)
			}

			return report(out, results, g.dbPathOrDefault())
		},
	}

	cmd.Flags().StringVar(&f.competitionID, "competition", "eng-premier-league",
		"competition ID to refresh (see `footy competitions`)")
	cmd.Flags().BoolVar(&f.all, "all", false,
		"refresh every competition footy covers instead of just --competition")
	cmd.Flags().StringVar(&f.season, "season", "",
		"a single season start year to refresh, e.g. 2025 for 2025-26 (default: the current season)")
	cmd.Flags().IntVar(&f.years, "years", 1,
		"how many recent seasons to refresh, counting back from the current one")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false,
		"print what would be fetched without making any network or database changes")

	return cmd
}

// resolveCompetitions turns the competition flags into the list to sync.
func (f *refreshFlags) resolveCompetitions() ([]models.Competition, error) {
	if f.all {
		return sync.Scope, nil
	}

	comp, ok := sync.FindCompetition(f.competitionID)
	if !ok {
		ids := make([]string, 0, len(sync.Scope))
		for _, c := range sync.Scope {
			ids = append(ids, c.ID)
		}
		return nil, fmt.Errorf("unknown --competition %q; supported: %s",
			f.competitionID, strings.Join(ids, ", "))
	}
	return []models.Competition{comp}, nil
}

// resolveSeasons turns the season flags into season start years. An
// explicit --season pins one season and overrides --years, since asking
// for a specific season and a count of recent seasons at once has no
// coherent meaning.
func (f *refreshFlags) resolveSeasons(now time.Time) []string {
	if f.season != "" {
		return []string{f.season}
	}
	return sync.RecentSeasonYears(now, f.years)
}

func printPlan(out io.Writer, comps []models.Competition, seasons []string, dbPath string) {
	fmt.Fprintf(out, "Dry run — nothing will be fetched or written.\n\n")
	fmt.Fprintf(out, "Would refresh %d competition-season(s) into %s:\n\n", len(comps)*len(seasons), dbPath)
	for _, c := range comps {
		fmt.Fprintf(out, "  %-24s %s\n", c.ID, strings.Join(displaySeasons(seasons), ", "))
	}
}

// displaySeasons renders stored start-year labels the way a user thinks of
// them ("2025-26"), matching what the TUI shows.
func displaySeasons(seasons []string) []string {
	out := make([]string, 0, len(seasons))
	for _, s := range seasons {
		out = append(out, seasonDisplay(s))
	}
	return out
}

// report prints a per-unit result table and a summary, returning an error
// only if every unit failed — a partial success still leaves the cache
// more populated than it was, and exiting non-zero would misreport that to
// a cron job.
func printUnit(out io.Writer, r sync.UnitResult) {
	label := fmt.Sprintf("%-24s %-8s", r.Competition.ID, seasonDisplay(r.Season.Label))
	if r.Failed() {
		fmt.Fprintf(out, "  FAIL  %s  %v\n", label, r.Err)
		return
	}
	fmt.Fprintf(out, "  ok    %s  %3d teams, %4d players\n", label, r.Result.Teams, r.Result.PlayerStats)
}

func report(out io.Writer, results []sync.UnitResult, dbPath string) error {
	s := sync.Summarize(results)
	fmt.Fprintf(out, "\n%d/%d succeeded — %d teams, %d team-season rows, %d players, %d player-season rows\n",
		s.Succeeded, s.Units, s.Totals.Teams, s.Totals.TeamStats, s.Totals.Players, s.Totals.PlayerStats)
	fmt.Fprintf(out, "Cached at %s\n", dbPath)

	if s.Succeeded == 0 && s.Failed > 0 {
		return fmt.Errorf("every competition-season failed (%d of %d)", s.Failed, s.Units)
	}
	if s.Failed > 0 {
		fmt.Fprintf(out, "\n%d competition-season(s) failed; the rest were cached.\n", s.Failed)
	}
	return nil
}
