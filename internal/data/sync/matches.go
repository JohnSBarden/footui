package sync

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/storage"
)

// International football is synced by the unit: one calendar year of one
// competition's matches, which a single request fetches for every nation
// at once. A nation's matches are the units of every international
// competition of its gender for the years wanted — so the first nation
// followed costs a request per competition per year, and every nation
// after it costs nothing, because the units are shared.
//
// Politeness is the point of the rules below. A unit for a year that has
// ended, fetched after it ended, never changes and is never fetched again.
// The current year's units are fetched again only when stale and a stored
// match has kicked off since (or a week has passed, to learn of fixtures
// announced since) — outside international windows, that is nothing.

// MatchSource is the slice of the provider contract international sync
// needs: the yearly match lists, the directory of nations, and tournament
// group tables.
type MatchSource interface {
	provider.MatchesProvider
	provider.NationalTeamsProvider
	provider.StandingsProvider
	provider.TeamsProvider
}

// Unit is one calendar year of one competition's matches.
type Unit struct {
	Competition models.Competition
	Year        int
}

// String is "world-cup 2026".
func (u Unit) String() string { return u.Competition.ID + " " + strconv.Itoa(u.Year) }

const (
	// failureBackoff keeps a unit that failed moments ago from being
	// retried at once.
	failureBackoff = 10 * time.Minute
	// yearGrace is how long after a year ends its last results are
	// assumed to settle: a New Year's Eve evening kick-off in the
	// Americas ends after midnight UTC.
	yearGrace = 24 * time.Hour
	// weeklyCheck is how old a current-year unit may get before it is
	// fetched again regardless, so fixtures announced after the last
	// fetch are learned of.
	weeklyCheck = 7 * 24 * time.Hour
	// inPlayWindow is how long before the last fetch a match may have
	// kicked off and still have been in progress then.
	inPlayWindow = 3 * time.Hour
)

// MatchSyncer runs international refreshes against a MatchSource and a
// storage.Repositories.
type MatchSyncer struct {
	source       MatchSource
	repos        storage.Repositories
	providerName string
	// StaleAfter is how old a current-year unit must be before it is
	// worth fetching again. Zero leaves only the kicked-off and weekly
	// rules, which is what an explicit refresh uses.
	StaleAfter time.Duration
	// MaxConsecutiveFailures works as on Syncer.
	MaxConsecutiveFailures int
	// Now is the clock, injectable for tests.
	Now func() time.Time
}

// NewMatchSyncer returns a syncer for international matches.
func NewMatchSyncer(source MatchSource, repos storage.Repositories) *MatchSyncer {
	return &MatchSyncer{source: source, repos: repos, providerName: "unknown"}
}

// WithProviderName sets the name recorded in the sync log.
func (s *MatchSyncer) WithProviderName(name string) *MatchSyncer {
	s.providerName = name
	return s
}

func (s *MatchSyncer) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// NationUnits are the units a nation of one gender plays in over the given
// calendar years: every international competition of that gender with
// matches that year. A tournament is only asked about in the year of an
// edition, or the year after one (Euro 2020 was played in 2021).
func NationUnits(gender string, years []int) []Unit {
	var out []Unit
	for _, y := range years {
		for _, c := range InternationalCoverages(gender) {
			if y < c.FirstSeason || !c.playedIn(y) {
				continue
			}
			out = append(out, Unit{Competition: c.Competition, Year: y})
		}
	}
	return out
}

// playedIn reports whether a competition can have matches in a calendar
// year.
func (c Coverage) playedIn(year int) bool {
	if c.Editions == nil {
		return true
	}
	for _, e := range c.Editions {
		if e == year || e == year-1 {
			return true
		}
	}
	return false
}

// RecentYears is this calendar year and the n-1 before it, newest first.
func RecentYears(now time.Time, n int) []int {
	if n < 1 {
		n = 1
	}
	out := make([]int, n)
	for i := range out {
		out[i] = now.Year() - i
	}
	return out
}

// UnitDue reports whether a unit needs fetching, and why. See the package
// comment above for the rules.
func (s *MatchSyncer) UnitDue(ctx context.Context, u Unit) (string, bool) {
	return UnitDue(ctx, s.repos, u, s.now(), s.StaleAfter)
}

// UnitDue is MatchSyncer.UnitDue for callers without a syncer: the stale
// check on open asks it of the units a favourite nation needs.
func UnitDue(ctx context.Context, repos storage.Repositories, u Unit, now time.Time, staleAfter time.Duration) (string, bool) {
	run, err := repos.LatestYearRun(ctx, u.Competition.ID, u.Year)
	if err != nil {
		return "not fetched yet", true
	}
	if !run.Succeeded() {
		if now.Sub(run.FinishedAt) < failureBackoff {
			return "", false
		}
		return "last attempt failed", true
	}
	settled := time.Date(u.Year+1, time.January, 1, 0, 0, 0, 0, time.UTC).Add(yearGrace)
	switch {
	case !run.FinishedAt.Before(settled):
		// Fetched after the year was over: it is complete, for good.
		return "", false
	case !now.Before(settled):
		return "the year has ended since it was fetched", true
	}
	age := now.Sub(run.FinishedAt)
	if staleAfter > 0 && age < staleAfter {
		return "", false
	}
	if age >= weeklyCheck {
		return "fetched over a week ago", true
	}
	n, err := repos.MatchesKickedOff(ctx, u.Competition.ID, u.Year, run.FinishedAt.Add(-inPlayWindow), now)
	if err == nil && n > 0 {
		return "matches played since", true
	}
	return "", false
}

// SyncUnits fetches every unit that is due, leaving the rest alone, and
// reports each as it finishes. As with clubs, one unit failing doesn't
// stop the others, three in a row gives up on the rest, and being
// rate-limited stops at once.
func (s *MatchSyncer) SyncUnits(ctx context.Context, units []Unit, progress Progress) []UnitResult {
	results := make([]UnitResult, 0, len(units))
	record := func(u UnitResult) {
		results = append(results, u)
		if progress != nil {
			progress(u)
		}
	}
	limit := s.MaxConsecutiveFailures
	if limit == 0 {
		limit = DefaultMaxConsecutiveFailures
	}
	failures := 0
	var stop error
	for _, u := range units {
		base := UnitResult{Competition: u.Competition, Year: u.Year, Season: yearSeason(u.Year)}
		if err := ctx.Err(); err != nil {
			base.Err = err
			record(base)
			return results
		}
		if stop != nil {
			base.Skipped, base.Err = true, stop
			record(base)
			continue
		}
		if _, due := s.UnitDue(ctx, u); !due {
			base.Fresh = true
			if run, err := s.repos.LatestYearRun(ctx, u.Competition.ID, u.Year); err == nil {
				base.CachedAt = run.FinishedAt
			}
			record(base)
			continue
		}
		base.Result, base.Err = s.syncUnit(ctx, u)
		record(base)
		switch {
		case base.Err == nil:
			failures = 0
		case errors.Is(base.Err, provider.ErrRateLimited):
			stop = errSkippedRateLimited
		default:
			failures++
			if limit > 0 && failures >= limit {
				stop = fmt.Errorf("skipped after %d consecutive failures", limit)
			}
		}
	}
	return results
}

// yearSeason is a season-shaped label for a calendar year, for display.
func yearSeason(year int) models.Season {
	return models.Season{Label: strconv.Itoa(year), CalendarYear: true}
}

// syncUnit fetches one unit and stores it, logging the attempt either way.
func (s *MatchSyncer) syncUnit(ctx context.Context, u Unit) (Result, error) {
	started := s.now()
	res, err := s.fetchUnit(ctx, u)
	if ctx.Err() == nil {
		run := models.SyncRun{Provider: s.providerName, CompetitionID: u.Competition.ID, Year: u.Year,
			StartedAt: started, FinishedAt: s.now(), Status: models.SyncStatusOK, RowsUpserted: res.Matches}
		if err != nil {
			run.Status, run.Error = failureStatus(err), err.Error()
		}
		_ = s.repos.RecordSyncRun(ctx, run)
	}
	return res, err
}

func (s *MatchSyncer) fetchUnit(ctx context.Context, u Unit) (Result, error) {
	var res Result
	comp := u.Competition
	matches, teams, fetchErr := s.source.CompetitionMatches(ctx,
		provider.CompetitionRef{CompetitionID: comp.ID, Name: comp.Name}, u.Year)
	if fetchErr != nil && !errors.Is(fetchErr, provider.ErrIncomplete) {
		return res, fmt.Errorf("sync: fetch matches: %w", fetchErr)
	}
	if len(matches) == 0 {
		// Nothing played that year: worth logging (so it isn't asked
		// again), not worth a competition row with nothing in it.
		return res, fetchErr
	}

	rows, teamIDs, err := s.resolveTeams(ctx, comp, teams)
	if err != nil {
		return res, err
	}
	if err := s.repos.UpsertCompetition(ctx, comp); err != nil {
		return res, err
	}
	for _, t := range rows {
		if err := s.repos.UpsertTeam(ctx, t); err != nil {
			return res, fmt.Errorf("sync: upsert team %s: %w", t.ID, err)
		}
		res.Teams++
	}

	seasons := map[string]bool{}
	links := map[[2]string]bool{}
	for i := range matches {
		m := &matches[i]
		m.CompetitionID = comp.ID
		if mapped, ok := teamIDs[m.HomeTeamID]; ok {
			m.HomeTeamID = mapped
		}
		if mapped, ok := teamIDs[m.AwayTeamID]; ok {
			m.AwayTeamID = mapped
		}
		if m.ID, err = canonicalMatchID(ctx, s.repos, m.ID); err != nil {
			return res, fmt.Errorf("sync: resolve match: %w", err)
		}
		if !seasons[m.SeasonID] {
			seasons[m.SeasonID] = true
			label := seasonLabel(m.SeasonID, comp.ID)
			if err := s.repos.UpsertSeason(ctx, NewSeason(comp.ID, label)); err != nil {
				return res, err
			}
		}
		if !comp.International() {
			// A club's season lines come from its league tables. A cup
			// tie or a European qualifier would otherwise give it an
			// empty line among its seasons.
			continue
		}
		for _, team := range []string{m.HomeTeamID, m.AwayTeamID} {
			key := [2]string{team, m.SeasonID}
			if !links[key] {
				links[key] = true
				if err := s.repos.LinkTeamCompetitionSeason(ctx, team, comp.ID, m.SeasonID); err != nil {
					return res, fmt.Errorf("sync: link team %s: %w", team, err)
				}
			}
		}
	}
	if err := s.repos.UpsertMatches(ctx, matches); err != nil {
		return res, err
	}
	res.Matches = len(matches)
	return res, fetchErr
}

// resolveTeams maps a unit's teams to canonical ones: nations among
// nations, clubs among clubs, never one for the other.
func (s *MatchSyncer) resolveTeams(ctx context.Context, comp models.Competition, teams []models.Team) ([]models.Team, map[string]string, error) {
	if comp.International() {
		rows, ids, err := nations(ctx, s.repos, teams)
		if err != nil {
			return nil, nil, fmt.Errorf("sync: resolve nations: %w", err)
		}
		return rows, ids, nil
	}
	r, err := newResolver(ctx, s.repos, comp)
	if err != nil {
		return nil, nil, fmt.Errorf("sync: resolve clubs: %w", err)
	}
	rows, err := r.teams(ctx, teams)
	if err != nil {
		return nil, nil, fmt.Errorf("sync: resolve clubs: %w", err)
	}
	return rows, r.teamOf, nil
}

// canonicalMatchID maps a provider's match ID to a canonical one, recorded
// in external_ref like a team's, so a match fetched again — rescheduled,
// or now with a result — updates the same row.
func canonicalMatchID(ctx context.Context, repos storage.Repositories, native string) (string, error) {
	prov, local, ok := nativeID(native)
	if !ok {
		return native, nil
	}
	if id, err := repos.ExternalRef(ctx, "match", prov, local); err == nil {
		return id, nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return "", err
	}
	id := prov + "-" + local
	return id, repos.PutExternalRef(ctx, "match", id, prov, local)
}

// SyncNations fetches the units a gender's nations play in over the given
// years.
func (s *MatchSyncer) SyncNations(ctx context.Context, gender string, years []int, progress Progress) []UnitResult {
	return s.SyncUnits(ctx, NationUnits(gender, years), progress)
}

// SeedNationalTeams stores the directory of one gender's national teams,
// so every nation can be found in search before any of its matches are
// fetched. It returns how many there are.
func (s *MatchSyncer) SeedNationalTeams(ctx context.Context, gender string) (int, error) {
	teams, err := s.source.NationalTeams(ctx, gender)
	if err != nil {
		return 0, err
	}
	rows, _, err := nations(ctx, s.repos, teams)
	if err != nil {
		return 0, err
	}
	for _, t := range rows {
		if err := s.repos.UpsertTeam(ctx, t); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

// SyncEdition refreshes one season of an international competition as the
// catalog and the League screen ask for it: a tournament edition, or a
// year of a qualifier or nations league. Its matches come from the shared
// units; its group tables, where it has them, from one standings request
// each.
//
// A tournament edition is fetched from its named year, and the year after
// only when that year held no final — Euro 2020, played in 2021; AFCON
// 2025, which ended in January.
func (s *MatchSyncer) SyncEdition(ctx context.Context, comp models.Competition, season string, progress Progress) []UnitResult {
	year, err := strconv.Atoi(season)
	if err != nil {
		return []UnitResult{{Competition: comp, Season: NewSeason(comp.ID, season), Err: fmt.Errorf("sync: bad season %q", season)}}
	}
	cov, _ := FindCoverage(comp.ID)
	results := s.SyncUnits(ctx, []Unit{{Competition: comp, Year: year}}, progress)
	fetched := anyFetched(results)
	if cov.Editions != nil && year+1 <= s.now().Year() && !s.hasFinal(ctx, comp, NewSeason(comp.ID, season).ID) {
		more := s.SyncUnits(ctx, []Unit{{Competition: comp, Year: year + 1}}, progress)
		fetched = fetched || anyFetched(more)
		results = append(results, more...)
	}
	if !comp.HasTable() || stopped(results) {
		return results
	}

	editions := []string{season}
	if cov.Editions == nil {
		// A calendar year of a qualifier holds matches of whichever
		// editions were being played that year.
		if editions, err = s.editionsPlayedIn(ctx, comp, year); err != nil {
			return append(results, UnitResult{Competition: comp, Season: NewSeason(comp.ID, season), Err: err})
		}
	}
	for _, ed := range editions {
		se := NewSeason(comp.ID, ed)
		se.CalendarYear = comp.CalendarYear
		if !fetched {
			if run, err := s.repos.LatestSyncRun(ctx, comp.ID, se.ID); err == nil && run.Succeeded() {
				u := UnitResult{Competition: comp, Season: se, Fresh: true, CachedAt: run.FinishedAt}
				results = append(results, u)
				if progress != nil {
					progress(u)
				}
				continue
			}
		}
		u := UnitResult{Competition: comp, Season: se}
		u.Result, u.Err = s.syncTable(ctx, comp, se)
		results = append(results, u)
		if progress != nil {
			progress(u)
		}
		if errors.Is(u.Err, provider.ErrRateLimited) {
			break
		}
	}
	return results
}

func anyFetched(results []UnitResult) bool {
	for _, r := range results {
		if !r.Fresh && !r.Skipped && r.Err == nil {
			return true
		}
	}
	return false
}

func stopped(results []UnitResult) bool {
	for _, r := range results {
		if errors.Is(r.Err, provider.ErrRateLimited) || errors.Is(r.Err, context.Canceled) {
			return true
		}
	}
	return false
}

func (s *MatchSyncer) hasFinal(ctx context.Context, comp models.Competition, seasonID string) bool {
	matches, err := s.repos.MatchesForCompetitionSeason(ctx, comp.ID, seasonID)
	if err != nil {
		return false
	}
	for _, m := range matches {
		if m.Round == "Final" {
			return true
		}
	}
	return false
}

// editionsPlayedIn is the editions of a competition with a match in a
// calendar year, newest first.
func (s *MatchSyncer) editionsPlayedIn(ctx context.Context, comp models.Competition, year int) ([]string, error) {
	seasons, err := s.repos.ListSeasons(ctx, comp.ID)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, se := range seasons {
		matches, err := s.repos.MatchesForCompetitionSeason(ctx, comp.ID, se.ID)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			if m.Kickoff.Year() == year {
				out = append(out, se.Label)
				break
			}
		}
	}
	return out, nil
}

// syncTable fetches and stores one edition's group tables, logging the
// attempt as the edition's. An edition with no table (a knockout-only
// finals) is a success with nothing in it, so it isn't asked again.
func (s *MatchSyncer) syncTable(ctx context.Context, comp models.Competition, season models.Season) (Result, error) {
	started := s.now()
	res, err := s.fetchTable(ctx, comp, season)
	if ctx.Err() == nil {
		run := models.SyncRun{Provider: s.providerName, CompetitionID: comp.ID, SeasonID: season.ID,
			StartedAt: started, FinishedAt: s.now(), Status: models.SyncStatusOK, RowsUpserted: res.TeamStats}
		if err != nil {
			run.Status, run.Error = failureStatus(err), err.Error()
		}
		_ = s.repos.RecordSyncRun(ctx, run)
	}
	return res, err
}

func (s *MatchSyncer) fetchTable(ctx context.Context, comp models.Competition, season models.Season) (Result, error) {
	var res Result
	ref := provider.CompetitionRef{CompetitionID: comp.ID, Name: comp.Name}
	sref := provider.SeasonRef{SeasonID: season.ID, Label: season.Label}
	standings, err := s.source.Standings(ctx, ref, sref)
	if errors.Is(err, provider.ErrNoTable) {
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("sync: fetch standings: %w", err)
	}
	teams, err := s.source.Teams(ctx, ref, sref)
	if err != nil {
		return res, fmt.Errorf("sync: fetch teams: %w", err)
	}
	rows, ids, err := nations(ctx, s.repos, teams)
	if err != nil {
		return res, fmt.Errorf("sync: resolve nations: %w", err)
	}
	if err := s.repos.UpsertCompetition(ctx, comp); err != nil {
		return res, err
	}
	if err := s.repos.UpsertSeason(ctx, season); err != nil {
		return res, err
	}
	for _, t := range rows {
		if err := s.repos.UpsertTeam(ctx, t); err != nil {
			return res, err
		}
		if err := s.repos.LinkTeamCompetitionSeason(ctx, t.ID, comp.ID, season.ID); err != nil {
			return res, err
		}
		res.Teams++
	}
	for _, st := range standings {
		if mapped, ok := ids[st.TeamID]; ok {
			st.TeamID = mapped
		}
		if err := s.repos.UpsertTeamSeasonStats(ctx, st); err != nil {
			return res, err
		}
		res.TeamStats++
	}
	return res, nil
}
