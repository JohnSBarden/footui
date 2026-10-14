package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

type refreshFlags struct {
	competitionID string
	team          string
	all           bool
	season        string
	years         int
	dryRun        bool
	maxAge        time.Duration
}

func newRefreshCmd(g *globalFlags) *cobra.Command {
	f := &refreshFlags{}

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Fetch and cache the latest stats from the data provider",
		Long: "refresh pulls team, standings, and player data from the provider that covers each competition (understat.com or ESPN) " +
			"and stores it in the local SQLite cache.\n\n" +
			"By default it refreshes the current season of one competition. Use --all for every " +
			"club competition footy covers, and --years to go back through previous seasons. " +
			"--team england fetches a national team's matches (this year and last by default); " +
			"they come from yearly match lists shared by every nation, so a second nation costs " +
			"nothing, and a year that has ended is never fetched twice. A failure " +
			"in one competition-season does not abort the rest of the run; the summary at the end " +
			"lists exactly what failed.\n\n" +
			"See `footy competitions` for valid --competition values.",
		// A failed fetch or an unknown competition is a runtime problem,
		// not a usage error — printing the whole usage block on top of it
		// buries the actual message.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.team != "" {
				if !cmd.Flags().Changed("years") {
					f.years = 2 // this year and last
				}
				return f.runTeam(cmd, g)
			}
			comps, err := f.resolveCompetitions()
			if err != nil {
				return err
			}
			jobs := f.plan(comps, time.Now())

			out := cmd.OutOrStdout()
			if f.dryRun {
				printPlan(out, jobs, g.dbPathOrDefault())
				return nil
			}

			db, store, err := g.openStore()
			if err != nil {
				return err
			}
			defer db.Close()

			fmt.Fprintf(out, "Refreshing %d competition-season(s)…\n\n", sync.Units(jobs))
			// Each unit and each retry is printed as it happens rather
			// than batched into the report: a full refresh is mostly
			// network wait, and silence for minutes — or for three
			// 15-second timeouts in a row — is indistinguishable from a
			// hang.
			noted := false
			results := runRefresh(cmdContext(cmd), store, jobs, f.maxAge, refreshEvents{
				unit: func(u sync.UnitResult) { printUnit(out, u) },
				retry: func(attempt, of int, err error) {
					fmt.Fprintf(out, "  retry attempt %d of %d failed, trying again: %v\n", attempt, of, err)
				},
				note: func(text string) {
					// The notes sit in their own block below the units.
					if !noted {
						fmt.Fprintln(out)
						noted = true
					}
					fmt.Fprintf(out, "  %s\n", text)
				},
			})

			return report(out, results, g.dbPathOrDefault())
		},
	}

	cmd.Flags().StringVar(&f.competitionID, "competition", "eng-premier-league",
		"competition ID to refresh (see `footy competitions`)")
	cmd.Flags().BoolVar(&f.all, "all", false,
		"refresh every club competition footy covers instead of just --competition")
	cmd.Flags().StringVar(&f.season, "season", "",
		"a single season start year to refresh, e.g. 2025 for 2025-26 (default: the current season)")
	cmd.Flags().IntVar(&f.years, "years", 1,
		"how many recent seasons to refresh, counting back from the current one")
	cmd.Flags().StringVar(&f.team, "team", "",
		"a national team whose matches to fetch, e.g. england or \"england women\" (default --years: this year and last)")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false,
		"print what would be fetched without making any network or database changes")
	cmd.Flags().DurationVar(&f.maxAge, "max-age", 0,
		"skip competition-seasons refreshed successfully within this duration, e.g. 24h "+
			"(default: refresh everything)")

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

// plan pairs each competition with the seasons to fetch for it. An
// explicit --season pins one season and overrides --years, since asking
// for a specific season and a count of recent seasons at once has no
// coherent meaning. Otherwise each competition counts back from its own
// current season — a calendar-year league's differs from a European
// one's — and stops at the first season its provider has.
func (f *refreshFlags) plan(comps []models.Competition, now time.Time) []sync.Job {
	jobs := make([]sync.Job, 0, len(comps))
	for _, c := range comps {
		var seasons []string
		if f.season != "" {
			seasons = []string{f.season}
		} else if cov, ok := sync.FindCoverage(c.ID); ok {
			seasons = cov.RecentSeasons(now, f.years)
		}
		if len(seasons) > 0 {
			jobs = append(jobs, sync.Job{Competition: c, Seasons: seasons})
		}
	}
	return jobs
}

func printPlan(out io.Writer, jobs []sync.Job, dbPath string) {
	fmt.Fprintf(out, "Dry run — nothing will be fetched or written.\n\n")
	fmt.Fprintf(out, "Would refresh %d competition-season(s) into %s:\n\n", sync.Units(jobs), dbPath)
	for _, j := range jobs {
		fmt.Fprintf(out, "  %-28s %s\n", j.Competition.ID, strings.Join(displaySeasons(j.Competition, j.Seasons), ", "))
	}
}

// displaySeasons renders stored start-year labels the way a user thinks of
// them ("2025-26"), matching what the TUI shows.
func displaySeasons(c models.Competition, seasons []string) []string {
	out := make([]string, 0, len(seasons))
	for _, s := range seasons {
		out = append(out, components.SeasonLabelFor(c.CalendarYear, s))
	}
	return out
}

// report prints a per-unit result table and a summary, returning an error
// only if every unit failed — a partial success still leaves the cache
// more populated than it was, and exiting non-zero would misreport that to
// a cron job.
func printUnit(out io.Writer, r sync.UnitResult) {
	label := fmt.Sprintf("%-24s %-8s", r.Competition.ID, components.SeasonName(r.Season))
	switch {
	case r.Fresh:
		fmt.Fprintf(out, "  fresh %s  cached %s\n", label,
			components.Freshness(r.CachedAt, time.Now()))
	case r.Skipped:
		fmt.Fprintf(out, "  skip  %s  %v\n", label, r.Err)
	case r.Failed():
		fmt.Fprintf(out, "  FAIL  %s  %v\n", label, r.Err)
	case r.Year != 0:
		fmt.Fprintf(out, "  ok    %s  %4d matches\n", label, r.Result.Matches)
	case r.Competition.International():
		fmt.Fprintf(out, "  ok    %s  %3d teams in the table\n", label, r.Result.TeamStats)
	default:
		fmt.Fprintf(out, "  ok    %s  %3d teams, %4d players\n", label, r.Result.Teams, r.Result.PlayerStats)
	}
}

// runTeam is `footy refresh --team`: the yearly match lists the nation's
// competitions publish, shared with every other nation.
func (f *refreshFlags) runTeam(cmd *cobra.Command, g *globalFlags) error {
	out := cmd.OutOrStdout()
	ctx := cmdContext(cmd)
	years := sync.RecentYears(time.Now(), f.years)
	if f.season != "" {
		y, err := strconv.Atoi(f.season)
		if err != nil {
			return fmt.Errorf("--season %q isn't a year", f.season)
		}
		years = []int{y}
	}

	if f.dryRun {
		// Nothing is fetched, so the nation isn't looked up either: its
		// gender is all that decides what would be.
		name, gender := parseNationQuery(f.team)
		units := sync.NationUnits(gender, years)
		fmt.Fprintf(out, "Dry run — nothing will be fetched or written.\n\n")
		fmt.Fprintf(out, "%s's matches would come from %d competition-year(s), shared with every %s's national team:\n\n",
			name, len(units), gender)
		for _, u := range units {
			fmt.Fprintf(out, "  %-28s %d\n", u.Competition.ID, u.Year)
		}
		fmt.Fprintln(out, "\nA year already fetched after it ended is never fetched again, so a real run is usually far fewer.")
		return nil
	}

	db, store, err := g.openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	ev := refreshEvents{
		unit: func(u sync.UnitResult) { printUnit(out, u) },
		retry: func(attempt, of int, err error) {
			fmt.Fprintf(out, "  retry attempt %d of %d failed, trying again: %v\n", attempt, of, err)
		},
	}
	team, err := resolveNation(ctx, store, f.team, func() error {
		fmt.Fprintln(out, "Fetching the list of national teams first…")
		_, err := seedDirectory(ctx, store, ev)
		return err
	})
	if err != nil {
		return err
	}
	units := sync.NationUnits(team.Gender, years)
	fmt.Fprintf(out, "Refreshing %s's matches: %d competition-year(s), shared with every %s's national team…\n\n",
		team.Label(), len(units), team.Gender)
	results := runNations(ctx, store, team.Gender, years, f.maxAge, ev)
	return report(out, results, g.dbPathOrDefault())
}

func report(out io.Writer, results []sync.UnitResult, dbPath string) error {
	s := sync.Summarize(results)

	// The headline counts every outcome, so a run that fetched nothing
	// because everything was already fresh doesn't read as "0 succeeded",
	// which looks like failure when it is the flag working.
	parts := []string{fmt.Sprintf("%d refreshed", s.Succeeded)}
	if s.Fresh > 0 {
		parts = append(parts, fmt.Sprintf("%d already fresh", s.Fresh))
	}
	if s.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", s.Failed))
	}
	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", s.Skipped))
	}

	fmt.Fprintf(out, "\n%s of %d\n", strings.Join(parts, ", "), s.Units)
	if s.Succeeded > 0 && (s.Totals.Players > 0 || s.Totals.Matches == 0) {
		fmt.Fprintf(out, "%d teams, %d team-season rows, %d players, %d player-season rows\n",
			s.Totals.Teams, s.Totals.TeamStats, s.Totals.Players, s.Totals.PlayerStats)
	}
	if s.Totals.Matches > 0 {
		fmt.Fprintf(out, "%d matches\n", s.Totals.Matches)
	}
	fmt.Fprintf(out, "Cached at %s\n", dbPath)

	if s.Skipped > 0 {
		fmt.Fprintf(out, "\nGave up after repeated failures — the provider looks unreachable.\n")
	}
	if s.Failed > 0 && s.Succeeded > 0 {
		fmt.Fprintf(out, "\n%d competition-season(s) failed; the rest were cached.\n", s.Failed)
	}

	// Nothing fetched *and* nothing already fresh means the run achieved
	// nothing, which a cron job should hear about.
	if s.Succeeded == 0 && s.Fresh == 0 && (s.Failed > 0 || s.Skipped > 0) {
		return fmt.Errorf("nothing was refreshed (%d failed, %d skipped of %d)", s.Failed, s.Skipped, s.Units)
	}
	return nil
}
