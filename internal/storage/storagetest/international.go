package storagetest

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

// International competitions screen tests seed against: a tournament, a
// qualifier and friendlies, men's.
var (
	WorldCup     = models.Competition{ID: "world-cup", Name: "FIFA World Cup", ShortName: "WC", Country: "International", Gender: "men", Format: "cup", CalendarYear: true, Category: models.CategoryTournament}
	Qualifying   = models.Competition{ID: "wcq-uefa", Name: "World Cup Qualifying – UEFA", ShortName: "WCQ", Country: "International", Gender: "men", Format: "cup", Category: models.CategoryQualifier}
	Friendlies   = models.Competition{ID: "int-friendly", Name: "International Friendlies", ShortName: "FRI", Country: "International", Gender: "men", Format: "cup", CalendarYear: true, Category: models.CategoryFriendly}
	nationsByIDs = map[string]string{"nat-arcadia": "Arcadia", "nat-borduria": "Borduria", "nat-carpania": "Carpania", "nat-dravonia": "Dravonia"}
)

func ip(n int) *int                             { return &n }
func fp(f float64) *float64                     { return &f }
func kick(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 19, 0, 0, 0, time.UTC) }

// SampleMatches is Arcadia's fictional 2025 and 2026: friendlies, a
// qualifying campaign and a World Cup with a group stage, a shoot-out
// exit, and one fixture still to play. Arcadia's record across all of it
// is W4 D2 L1; competitive only, W2 D2 L1.
func SampleMatches() []models.Match {
	m := func(id, comp string, edition int, at time.Time, status, round, home, away string, hs, as int) models.Match {
		return models.Match{ID: id, CompetitionID: comp, SeasonID: comp + ":" + strconv.Itoa(edition), Kickoff: at,
			Status: status, Round: round, HomeTeamID: home, AwayTeamID: away, HomeScore: ip(hs), AwayScore: ip(as),
			Source: "test", UpdatedAt: Fixed}
	}
	ms := []models.Match{
		m("m01", "int-friendly", 2025, kick(2025, 3, 21), models.MatchFullTime, "", "nat-arcadia", "nat-borduria", 3, 0),
		m("m02", "wcq-uefa", 2025, kick(2025, 6, 7), models.MatchFullTime, "Group K", "nat-carpania", "nat-arcadia", 0, 2),
		m("m03", "wcq-uefa", 2025, kick(2025, 9, 9), models.MatchFullTime, "Group K", "nat-arcadia", "nat-carpania", 1, 1),
		m("m04", "int-friendly", 2026, kick(2026, 3, 27), models.MatchFullTime, "", "nat-arcadia", "nat-dravonia", 2, 1),
		m("m05", "world-cup", 2026, kick(2026, 6, 17), models.MatchFullTime, "Group L", "nat-arcadia", "nat-borduria", 4, 2),
		m("m06", "world-cup", 2026, kick(2026, 6, 23), models.MatchFullTime, "Group L", "nat-dravonia", "nat-arcadia", 2, 1),
		m("m07", "world-cup", 2026, kick(2026, 7, 4), models.MatchPens, "Quarterfinal", "nat-arcadia", "nat-carpania", 1, 1),
		{ID: "m08", CompetitionID: "int-friendly", SeasonID: "int-friendly:2026", Kickoff: kick(2026, 11, 14),
			Status: models.MatchScheduled, HomeTeamID: "nat-borduria", AwayTeamID: "nat-arcadia", Source: "test", UpdatedAt: Fixed},
	}
	ms[6].HomePens, ms[6].AwayPens = ip(3), ip(4)
	ms[6].Neutral = true
	ms[4].HomeStats = &models.MatchTeamStats{Possession: fp(61), Shots: ip(18), ShotsOnTarget: ip(7), Corners: ip(6), Fouls: ip(9)}
	ms[4].AwayStats = &models.MatchTeamStats{Possession: fp(39), Shots: ip(6), ShotsOnTarget: ip(2), Corners: ip(2), Fouls: ip(14)}
	ms[6].HomeStats = &models.MatchTeamStats{Possession: fp(55), Shots: ip(12)}
	return ms
}

// SeedInternational stores the sample competitions, the four fictional
// nations, their matches, and a World Cup 2026 group table (Groups K and
// L, two teams each).
func SeedInternational(t *testing.T, repos storage.Repositories) {
	t.Helper()
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("storagetest: seed international: %v", err)
		}
	}
	for _, c := range []models.Competition{WorldCup, Qualifying, Friendlies} {
		must(repos.UpsertCompetition(ctx, c))
	}
	for _, s := range []string{"world-cup:2026", "wcq-uefa:2025", "int-friendly:2025", "int-friendly:2026"} {
		comp, label := s[:len(s)-5], s[len(s)-4:]
		must(repos.UpsertSeason(ctx, models.Season{ID: s, CompetitionID: comp, Label: label}))
	}
	for id, name := range nationsByIDs {
		must(repos.UpsertTeam(ctx, models.Team{ID: id, Name: name, Gender: "men", National: true}))
	}
	must(repos.UpsertMatches(ctx, SampleMatches()))
	for i, row := range []struct {
		team, group string
		pos, pts    int
	}{
		{"nat-arcadia", "Group L", 1, 6}, {"nat-dravonia", "Group L", 2, 4},
		{"nat-carpania", "Group K", 1, 7}, {"nat-borduria", "Group K", 2, 1},
	} {
		must(repos.LinkTeamCompetitionSeason(ctx, row.team, "world-cup", "world-cup:2026"))
		must(repos.UpsertTeamSeasonStats(ctx, models.TeamSeasonStats{TeamID: row.team, CompetitionID: "world-cup",
			SeasonID: "world-cup:2026", GroupLabel: row.group, Position: ip(row.pos), MatchesPlayed: 3,
			Points: row.pts, GoalsFor: 6 - i, Source: "test", UpdatedAt: Fixed}))
	}
}
