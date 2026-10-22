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

// ErrNoCareer means footui doesn't know the player's ESPN ID yet; a detail
// refresh records it when it finds them on a roster.
var ErrNoCareer = errors.New("player not found at the source")

// PlayerCareerJobs returns the player's career seasons footui can fetch as
// league tables, one job per competition, most recent first.
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
// club has a table in, plus also. No source lists a club's seasons, so
// years it wasn't in a league are fetched too.
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

// historyJobs builds jobs of the seasons want selects, in Coverages order.
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

// tableCoverage is the coverage of a club competition with tables.
func tableCoverage(compID string) (Coverage, bool) {
	cov, ok := FindCoverage(compID)
	if !ok || cov.Competition.International() || cov.Competition.Cup() {
		return Coverage{}, false
	}
	return cov, true
}

// finishedBy reports whether a season was already over at time at.
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
