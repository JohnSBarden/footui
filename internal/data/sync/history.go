package sync

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/storage"
)

// CareerSource lists the competition-seasons a player has stats in: ESPN,
// by the player's ESPN ID.
type CareerSource interface {
	AthleteCareer(ctx context.Context, athleteID string) ([]provider.CareerSeason, error)
}

// ErrNoCareer means footui doesn't know the player's ESPN ID, so there
// is no career to ask for. Their detail refresh records it when it can
// find them on a club's roster.
var ErrNoCareer = errors.New("player not found at the source")

// PlayerCareerJobs asks src for every season the player has stats in and
// returns those footui can fetch as league tables: one job per
// competition, most recent season first. A cup's ties and international
// matches aren't tables, and a season before a competition's coverage
// starts isn't offered.
func PlayerCareerJobs(ctx context.Context, repos storage.Repositories, src CareerSource, playerID string, now time.Time) ([]Job, error) {
	athleteID, err := repos.ProviderID(ctx, "player", playerID, DetailProvider)
	if errors.Is(err, storage.ErrNotFound) || err == nil && athleteID == "" {
		return nil, ErrNoCareer
	}
	if err != nil {
		return nil, err
	}
	career, err := src.AthleteCareer(ctx, athleteID)
	if err != nil {
		return nil, err
	}
	seasons := map[string][]string{}
	var order []string
	for _, cs := range career {
		if _, ok := tableCoverage(cs.CompetitionID); !ok {
			continue
		}
		if _, ok := seasons[cs.CompetitionID]; !ok {
			order = append(order, cs.CompetitionID)
		}
		seasons[cs.CompetitionID] = append(seasons[cs.CompetitionID], strconv.Itoa(cs.Year))
	}
	return historyJobs(order, now, func(id, year string) bool {
		return slices.Contains(seasons[id], year)
	}), nil
}

// ClubHistoryJobs is every season footui offers of each competition the
// club has a table in: there is no list of the seasons a club played, so
// a league it was relegated from is fetched for the years it wasn't
// there too, which costs requests but nothing else. also adds
// competitions the club has no table in yet: the one it was opened from.
func ClubHistoryJobs(ctx context.Context, repos storage.Repositories, clubID string, also []string, now time.Time) ([]Job, error) {
	history, err := repos.TeamHistory(ctx, clubID)
	if err != nil {
		return nil, err
	}
	order := slices.Clone(also)
	for _, h := range history {
		order = append(order, h.CompetitionID)
	}
	order = slices.DeleteFunc(order, func(id string) bool {
		_, ok := tableCoverage(id)
		return !ok
	})
	return historyJobs(order, now, func(string, string) bool { return true }), nil
}

// historyJobs turns competitions into jobs of the seasons footui offers
// that want says to fetch, ordered as the catalog orders competitions.
func historyJobs(compIDs []string, now time.Time, want func(compID, year string) bool) []Job {
	var jobs []Job
	for _, cov := range Coverages {
		if !slices.Contains(compIDs, cov.Competition.ID) {
			continue
		}
		var seasons []string
		for _, y := range cov.AvailableSeasons(now) {
			if want(cov.Competition.ID, y) {
				seasons = append(seasons, y)
			}
		}
		if len(seasons) > 0 {
			jobs = append(jobs, Job{Competition: cov.Competition, Seasons: seasons})
		}
	}
	return jobs
}

// tableCoverage is the coverage of a club competition with tables: not a
// cup, whose ties are a club's matches, and not international football.
func tableCoverage(compID string) (Coverage, bool) {
	cov, ok := FindCoverage(compID)
	if !ok || cov.Competition.International() || cov.Competition.Cup() {
		return Coverage{}, false
	}
	return cov, true
}

// finishedBy reports whether a competition's season was already over at
// a moment: a sync run then fetched its final state.
func finishedBy(compID, seasonID string, at time.Time) bool {
	cov, ok := FindCoverage(compID)
	if !ok {
		return false
	}
	year, ok := seasonYear(seasonID)
	if !ok {
		return false
	}
	current, err := strconv.Atoi(cov.CurrentSeason(at))
	return err == nil && year < current
}
