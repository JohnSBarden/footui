package sync

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/storage"
)

// DetailSource is where a player's season detail comes from: ESPN, which
// has it for its own leagues and understat's.
type DetailSource interface {
	HasDetail(competitionID string) bool
	AthleteSeason(ctx context.Context, competitionID string, year int, athleteID string, keeper bool) (models.PlayerSeasonDetail, error)
	Squad(ctx context.Context, competitionID string, year int, teamID string) ([]provider.SquadMember, error)
}

// DetailResult counts what a player refresh did with each season line.
type DetailResult struct {
	Fetched   int // fetched and stored
	Fresh     int // already current, left alone
	Unmatched int // the source has no such player for that club
	Missing   int // the source kept no stats for that season
	Failed    int
}

// DetailProvider is the provider name player detail IDs are recorded
// under in external_ref.
const DetailProvider = ProviderESPN

// RefreshPlayerDetail brings one player's season detail up to date, a
// request per season line that needs it. A past season is fetched once
// and kept: it no longer changes. The current season is fetched again
// once its copy is older than maxAge; zero means always (a refresh the
// user asked for).
//
// The player's ESPN ID is the one recorded when they were synced from an
// ESPN competition, or else found by matching them on their club's ESPN
// roster for the season, and recorded. A player that can't be matched is
// counted, not an error: their page just goes without the detail.
func RefreshPlayerDetail(ctx context.Context, repos storage.Repositories, src DetailSource, playerID string, now time.Time, maxAge time.Duration) (DetailResult, error) {
	var res DetailResult
	player, err := repos.GetPlayer(ctx, playerID)
	if err != nil {
		return res, err
	}
	lines, err := repos.PlayerHistory(ctx, playerID)
	if err != nil {
		return res, err
	}
	existing, err := repos.PlayerSeasonDetails(ctx, playerID)
	if err != nil {
		return res, err
	}
	have := make(map[string]models.PlayerSeasonDetail, len(existing))
	for _, d := range existing {
		have[detailKey(d.TeamID, d.CompetitionID, d.SeasonID)] = d
	}

	athleteID, err := repos.ProviderID(ctx, "player", playerID, DetailProvider)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return res, err
	}
	var lastErr error
	for _, line := range lines {
		if !src.HasDetail(line.CompetitionID) {
			continue
		}
		year, ok := seasonYear(line.SeasonID)
		if !ok {
			continue
		}
		prev, had := have[detailKey(line.TeamID, line.CompetitionID, line.SeasonID)]
		if had && (!isCurrent(line.CompetitionID, year, now) || (maxAge > 0 && now.Sub(prev.FetchedAt) < maxAge)) {
			res.Fresh++
			continue
		}

		position := prev.Position
		if athleteID == "" {
			member, found, err := findOnSquad(ctx, repos, src, player, line, year)
			if err != nil {
				if stop(err) {
					return res, err
				}
				res.Failed++
				lastErr = err
				continue
			}
			if !found {
				res.Unmatched++
				continue
			}
			athleteID, position = member.ID, member.Position
			if err := repos.PutExternalRef(ctx, "player", playerID, DetailProvider, athleteID); err != nil {
				return res, err
			}
		}

		keeper := position == "G" || isKeeperCode(player.Position)
		d, err := src.AthleteSeason(ctx, line.CompetitionID, year, athleteID, keeper)
		if errors.Is(err, provider.ErrNotFound) {
			// Not every season is kept (ESPN has no Champions League
			// stats for Messi): the page goes without that season's
			// detail, as for a player the source doesn't know.
			res.Missing++
			continue
		}
		if err != nil {
			if stop(err) {
				return res, err
			}
			res.Failed++
			lastErr = err
			continue
		}
		d.PlayerID, d.TeamID, d.CompetitionID, d.SeasonID = playerID, line.TeamID, line.CompetitionID, line.SeasonID
		d.Position, d.FetchedAt = position, now
		if err := repos.UpsertPlayerSeasonDetail(ctx, d); err != nil {
			return res, err
		}
		res.Fetched++
	}
	if res.Fetched == 0 && res.Failed > 0 {
		return res, lastErr
	}
	return res, nil
}

// findOnSquad looks the player up on their club's roster for the season.
func findOnSquad(ctx context.Context, repos storage.Repositories, src DetailSource, player models.Player, line models.PlayerSeasonStats, year int) (provider.SquadMember, bool, error) {
	team, err := repos.ProviderID(ctx, "team", line.TeamID, DetailProvider)
	if errors.Is(err, storage.ErrNotFound) {
		return provider.SquadMember{}, false, nil
	}
	if err != nil {
		return provider.SquadMember{}, false, err
	}
	squad, err := src.Squad(ctx, line.CompetitionID, year, team)
	if err != nil {
		return provider.SquadMember{}, false, fmt.Errorf("roster for %s %d: %w", line.TeamID, year, err)
	}
	m, ok := matchSquadMember(player, squad)
	return m, ok, nil
}

// matchSquadMember is matchPlayer's rule from the other side: the roster
// member with the player's name, or failing that the one member whose
// name contains it or is contained by it ("Gabriel" = "Gabriel
// Magalhães" only if no other Gabriel is on the roster).
func matchSquadMember(p models.Player, squad []provider.SquadMember) (provider.SquadMember, bool) {
	name := normalizePerson(p.Name)
	var exact, loose []provider.SquadMember
	for _, m := range squad {
		mn := normalizePerson(m.Name)
		switch {
		case mn == name:
			exact = append(exact, m)
		case subset(mn, name) || subset(name, mn):
			loose = append(loose, m)
		}
	}
	if len(exact) == 1 {
		return exact[0], true
	}
	if len(exact) == 0 && len(loose) == 1 {
		return loose[0], true
	}
	return provider.SquadMember{}, false
}

// stop reports errors that end the whole refresh rather than one season.
func stop(err error) bool {
	return errors.Is(err, provider.ErrRateLimited) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func detailKey(team, comp, season string) string { return team + "|" + comp + "|" + season }

// seasonYear is the start year in a season ID ("eng-premier-league:2025").
func seasonYear(seasonID string) (int, bool) {
	i := strings.LastIndexByte(seasonID, ':')
	if i < 0 {
		return 0, false
	}
	y, err := strconv.Atoi(seasonID[i+1:])
	return y, err == nil
}

// isCurrent reports whether year is the competition's season in play.
func isCurrent(competitionID string, year int, now time.Time) bool {
	cov, ok := FindCoverage(competitionID)
	if !ok {
		return false
	}
	return cov.CurrentSeason(now) == strconv.Itoa(year)
}

func isKeeperCode(code string) bool {
	for _, f := range strings.Fields(code) {
		if f == "GK" {
			return true
		}
	}
	return false
}
