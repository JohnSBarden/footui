package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/storage"
)

// LastGameProvider is where match detail comes from; its match and player
// IDs are recorded under it in external_ref.
const LastGameProvider = ProviderESPN

// LastGameSource supplies match detail.
type LastGameSource = provider.MatchDetailsProvider

// LastGameResult says what RefreshLastGame did.
type LastGameResult struct {
	Match models.Match
	// Fetched is false when there was nothing to fetch: no finished game,
	// or its final detail already stored.
	Fetched bool
	// Requests is how many the fetch took (1, or more to find the game).
	Requests int
	// Unavailable: the source has no such game. An empty final detail is
	// stored so it isn't asked again.
	Unavailable bool
}

// LastGameDue reports whether a club's last finished game lacks a final
// detail, without a request.
func LastGameDue(ctx context.Context, repos storage.Repositories, teamID string) (models.Match, bool) {
	recent, err := repos.RecentMatches(ctx, teamID, 1)
	if err != nil || len(recent) == 0 {
		return models.Match{}, false
	}
	d, err := repos.MatchDetail(ctx, recent[0].ID)
	if err == nil && d.Final {
		return recent[0], false
	}
	return recent[0], err == nil || errors.Is(err, storage.ErrNotFound)
}

// RefreshLastGame fetches the detail of a club's last finished game unless
// its final detail is already stored. A game another source supplied is
// first found on that day's scoreboard.
func RefreshLastGame(ctx context.Context, repos storage.Repositories, src LastGameSource, teamID string, now time.Time) (LastGameResult, error) {
	m, due := LastGameDue(ctx, repos, teamID)
	res := LastGameResult{Match: m}
	if !due {
		return res, nil
	}
	comp, err := repos.GetCompetition(ctx, m.CompetitionID)
	if err != nil {
		return res, fmt.Errorf("last game %s: %w", m.ID, err)
	}
	ref := provider.CompetitionRef{CompetitionID: comp.ID, Name: comp.Name}
	missing := func() (LastGameResult, error) {
		res.Unavailable = true
		return res, repos.PutMatchDetail(ctx, models.MatchDetail{MatchID: m.ID, Final: true, FetchedAt: now})
	}

	native, n, err := findNativeMatch(ctx, repos, src, ref, m)
	res.Requests += n
	if errors.Is(err, provider.ErrNotFound) {
		return missing()
	}
	if err != nil {
		return res, fmt.Errorf("last game %s: %w", m.ID, err)
	}
	d, err := src.MatchDetail(ctx, ref, native)
	res.Requests++
	if errors.Is(err, provider.ErrNotFound) {
		return missing()
	}
	if err != nil {
		return res, fmt.Errorf("last game %s: %w", m.ID, err)
	}
	d.MatchID = m.ID
	if err := canonicalPlayers(ctx, repos, &d); err != nil {
		return res, err
	}
	if err := repos.PutMatchDetail(ctx, d); err != nil {
		return res, err
	}
	res.Fetched = true
	return res, nil
}

// findNativeMatch returns the source's ID for m: its recorded ref, or the
// game between m's two teams on the scoreboard of its day, or the day
// either side (a late kick-off is the next day in UTC, an early one the
// day before, depending on how the source files it).
func findNativeMatch(ctx context.Context, repos storage.Repositories, src LastGameSource, ref provider.CompetitionRef,
	m models.Match) (string, int, error) {

	if id, err := repos.ProviderID(ctx, "match", m.ID, LastGameProvider); err == nil {
		return id, 0, nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return "", 0, err
	}
	home, err := repos.GetTeam(ctx, m.HomeTeamID)
	if err != nil {
		return "", 0, err
	}
	away, err := repos.GetTeam(ctx, m.AwayTeamID)
	if err != nil {
		return "", 0, err
	}
	requests := 0
	for _, offset := range []int{0, -1, 1} {
		day := m.Kickoff.UTC().AddDate(0, 0, offset)
		matches, teams, err := src.DayMatches(ctx, ref, day)
		requests++
		if err != nil {
			return "", requests, err
		}
		names := map[string]string{}
		for _, t := range teams {
			names[t.ID] = t.Name
		}
		for _, c := range matches {
			if !sameKickoffDay(c.Kickoff, m.Kickoff) {
				continue
			}
			homeOK, err := isTeam(ctx, repos, c.HomeTeamID, names[c.HomeTeamID], home)
			if err != nil {
				return "", requests, err
			}
			awayOK, err := isTeam(ctx, repos, c.AwayTeamID, names[c.AwayTeamID], away)
			if err != nil {
				return "", requests, err
			}
			if !homeOK || !awayOK {
				continue
			}
			_, local, ok := nativeID(c.ID)
			if !ok {
				continue
			}
			for _, side := range []struct {
				native string
				team   models.Team
			}{{c.HomeTeamID, home}, {c.AwayTeamID, away}} {
				if prov, id, ok := nativeID(side.native); ok {
					if err := repos.PutExternalRef(ctx, "team", side.team.ID, prov, id); err != nil {
						return "", requests, err
					}
				}
			}
			if err := repos.PutExternalRef(ctx, "match", m.ID, LastGameProvider, local); err != nil {
				return "", requests, err
			}
			return local, requests, nil
		}
	}
	return "", requests, fmt.Errorf("no %s game between %s and %s on %s: %w",
		LastGameProvider, home.Name, away.Name, m.Kickoff.UTC().Format("2 Jan 2006"), provider.ErrNotFound)
}

// sameKickoffDay allows a day either side: the scoreboard's day is the
// source's, not UTC's.
func sameKickoffDay(a, b time.Time) bool {
	d := a.Sub(b)
	return d > -36*time.Hour && d < 36*time.Hour
}

// isTeam reports whether the source's team (native ID and name) is team:
// by a recorded ref, or else by name when the source's ID isn't already
// someone else's.
func isTeam(ctx context.Context, repos storage.Repositories, nativeTeam, name string, team models.Team) (bool, error) {
	prov, local, ok := nativeID(nativeTeam)
	if !ok {
		return false, nil
	}
	id, err := repos.ExternalRef(ctx, "team", prov, local)
	switch {
	case err == nil:
		return id == team.ID, nil
	case !errors.Is(err, storage.ErrNotFound):
		return false, err
	}
	return name != "" && normalizeClub(name) == normalizeClub(team.Name), nil
}

// canonicalPlayers swaps the source's player IDs for footui's where the
// player is known, and drops the rest.
func canonicalPlayers(ctx context.Context, repos storage.Repositories, d *models.MatchDetail) error {
	seen := map[string]string{}
	resolve := func(id *string) error {
		if *id == "" {
			return nil
		}
		if c, ok := seen[*id]; ok {
			*id = c
			return nil
		}
		native := *id
		*id = ""
		if prov, local, ok := nativeID(native); ok {
			c, err := repos.ExternalRef(ctx, "player", prov, local)
			switch {
			case err == nil:
				*id = c
			case !errors.Is(err, storage.ErrNotFound):
				return err
			}
		}
		seen[native] = *id
		return nil
	}
	for i := range d.Events {
		if err := resolve(&d.Events[i].PlayerID); err != nil {
			return err
		}
		if err := resolve(&d.Events[i].OtherID); err != nil {
			return err
		}
	}
	for _, side := range []*models.MatchSide{&d.Home, &d.Away} {
		for i := range side.Lineup {
			if err := resolve(&side.Lineup[i].PlayerID); err != nil {
				return err
			}
		}
	}
	return nil
}
