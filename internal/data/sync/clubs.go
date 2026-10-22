package sync

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

// A club's recent games come from its understat league season payload,
// and otherwise from ESPN's yearly units, shared with other clubs and
// national teams.

// ClubPlan is what a club's recent games need fetched.
type ClubPlan struct {
	// Leagues are the understat league seasons that are due.
	Leagues []Job
	// Units are the ESPN units the club plays in, due or not —
	// MatchSyncer.SyncUnits skips what isn't.
	Units []Unit
}

// clubYears is how many calendar years of a club's games are fetched;
// comfortably more than the thirty the Games screen lists.
const clubYears = 2

// europeanCountries are the countries whose clubs play in the European
// competitions footui covers. Russia's clubs have been excluded since 2022.
var europeanCountries = map[string]bool{"England": true, "Spain": true, "Germany": true, "Italy": true, "France": true}

// PlanClub works out what a club's recent games need. staleAfter is how
// old the current understat season may get; zero makes it always due.
func PlanClub(ctx context.Context, repos storage.Repositories, club models.Team, now time.Time, staleAfter time.Duration) (ClubPlan, error) {
	var plan ClubPlan
	years := RecentYears(now, clubYears)
	history, err := repos.TeamHistory(ctx, club.ID)
	if err != nil {
		return plan, err
	}

	seenUnit := map[string]bool{}
	addUnit := func(c Coverage, year int) {
		u := Unit{Competition: c.Competition, Year: year}
		if year < c.FirstSeason || seenUnit[u.String()] {
			return
		}
		seenUnit[u.String()] = true
		plan.Units = append(plan.Units, u)
	}
	leagueSeasons := map[string][]string{}
	var leagueOrder []string
	for _, h := range history {
		cov, ok := FindCoverage(h.CompetitionID)
		if !ok || cov.Competition.International() || cov.Competition.Cup() {
			continue
		}
		label := seasonLabel(h.SeasonID, h.CompetitionID)
		start, err := strconv.Atoi(label)
		if err != nil {
			continue
		}
		spans := []int{start}
		if !cov.Competition.CalendarYear {
			spans = append(spans, start+1)
		}
		var inRange []int
		for _, y := range spans {
			if slices.Contains(years, y) {
				inRange = append(inRange, y)
			}
		}
		if len(inRange) == 0 {
			continue
		}
		if cov.Provider != ProviderUnderstat {
			for _, y := range inRange {
				addUnit(cov, y)
			}
			continue
		}
		if !leagueSeasonDue(ctx, repos, cov, h, label, now, staleAfter) {
			continue
		}
		if _, ok := leagueSeasons[cov.Competition.ID]; !ok {
			leagueOrder = append(leagueOrder, cov.Competition.ID)
		}
		leagueSeasons[cov.Competition.ID] = append(leagueSeasons[cov.Competition.ID], label)
	}
	for _, id := range leagueOrder {
		cov, _ := FindCoverage(id)
		seasons := leagueSeasons[id]
		slices.SortFunc(seasons, func(a, b string) int { return compareLabels(b, a) })
		plan.Leagues = append(plan.Leagues, Job{Competition: cov.Competition, Seasons: seasons})
	}

	for _, c := range Coverages {
		comp := c.Competition
		europe := comp.Country == "Europe" && !comp.International() && comp.Gender == club.Gender && europeanCountries[club.Country]
		cup := comp.Cup() && comp.Country == club.Country && comp.Gender == club.Gender
		if !europe && !cup {
			continue
		}
		for _, y := range years {
			addUnit(c, y)
		}
	}
	return plan, nil
}

// leagueSeasonDue reports whether an understat league season needs
// fetching for its matches.
func leagueSeasonDue(ctx context.Context, repos storage.Repositories, cov Coverage, line models.TeamSeasonStats,
	label string, now time.Time, staleAfter time.Duration) bool {

	if n, err := repos.CountSeasonMatches(ctx, line.CompetitionID, line.SeasonID); err == nil && n == 0 && line.MatchesPlayed > 0 {
		return true
	}
	if label != cov.CurrentSeason(now) {
		return false
	}
	run, err := repos.LatestSyncRun(ctx, line.CompetitionID, line.SeasonID)
	if err != nil {
		return true
	}
	return now.Sub(run.FinishedAt) > staleAfter
}

// seasonLabel is the label part of a season ID ("eng-premier-league:2025"
// is "2025").
func seasonLabel(seasonID, competitionID string) string {
	return strings.TrimPrefix(seasonID, competitionID+":")
}

func compareLabels(a, b string) int {
	x, _ := strconv.Atoi(a)
	y, _ := strconv.Atoi(b)
	return x - y
}
