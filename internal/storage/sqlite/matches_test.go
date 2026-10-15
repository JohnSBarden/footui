package sqlite

import (
	"errors"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

func kick(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 19, 0, 0, 0, time.UTC) }

// seedInternational stores a nation's fixtures across three competitions
// and two years, with stats on some matches only.
func seedInternational(t *testing.T) *Store {
	t.Helper()
	s := newTestStore(t)
	for _, c := range []models.Competition{
		{ID: "world-cup", Name: "World Cup", ShortName: "WC", Country: "International", Gender: "men", Format: "cup", CalendarYear: true, Category: models.CategoryTournament},
		{ID: "wcq-uefa", Name: "WC Qualifying", ShortName: "WCQ", Country: "International", Gender: "men", Format: "cup", Category: models.CategoryQualifier},
		{ID: "int-friendly", Name: "Friendly", Country: "International", Gender: "men", Format: "cup", CalendarYear: true, Category: models.CategoryFriendly},
	} {
		mustNoErr(t, s.UpsertCompetition(ctx, c))
	}
	for _, se := range []string{"world-cup:2026", "wcq-uefa:2025", "int-friendly:2025", "int-friendly:2026"} {
		comp, label := se[:len(se)-5], se[len(se)-4:]
		mustNoErr(t, s.UpsertSeason(ctx, models.Season{ID: se, CompetitionID: comp, Label: label}))
	}
	for _, id := range []string{"nat-arcadia", "nat-borduria", "nat-carpania"} {
		mustNoErr(t, s.UpsertTeam(ctx, models.Team{ID: id, Name: id[4:], Gender: "men", National: true}))
	}
	mustNoErr(t, s.UpsertMatches(ctx, []models.Match{
		{ID: "m1", CompetitionID: "int-friendly", SeasonID: "int-friendly:2025", Kickoff: kick(2025, 3, 20),
			Status: models.MatchFullTime, HomeTeamID: "nat-arcadia", AwayTeamID: "nat-borduria",
			HomeScore: ptrI(2), AwayScore: ptrI(0),
			HomeStats: &models.MatchTeamStats{Possession: ptrF(61), Shots: ptrI(14)},
			AwayStats: &models.MatchTeamStats{Possession: ptrF(39), Shots: ptrI(4)},
			Source:    "test", UpdatedAt: fixtureTime},
		{ID: "m2", CompetitionID: "wcq-uefa", SeasonID: "wcq-uefa:2025", Kickoff: kick(2025, 9, 6),
			Status: models.MatchFullTime, HomeTeamID: "nat-carpania", AwayTeamID: "nat-arcadia",
			HomeScore: ptrI(1), AwayScore: ptrI(1), Round: "Group K", Source: "test", UpdatedAt: fixtureTime},
		{ID: "m3", CompetitionID: "world-cup", SeasonID: "world-cup:2026", Kickoff: kick(2026, 7, 4),
			Status: models.MatchPens, HomeTeamID: "nat-arcadia", AwayTeamID: "nat-carpania", Neutral: true,
			HomeScore: ptrI(0), AwayScore: ptrI(0), HomePens: ptrI(3), AwayPens: ptrI(4), Round: "Quarterfinal",
			HomeStats: &models.MatchTeamStats{Possession: ptrF(55), Shots: ptrI(10), ShotsOnTarget: ptrI(3)},
			Source:    "test", UpdatedAt: fixtureTime},
		{ID: "m4", CompetitionID: "int-friendly", SeasonID: "int-friendly:2026", Kickoff: kick(2026, 11, 14),
			Status: models.MatchScheduled, HomeTeamID: "nat-borduria", AwayTeamID: "nat-arcadia",
			Source: "test", UpdatedAt: fixtureTime},
	}))
	return s
}

func matchIDs(ms []models.Match) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.ID
	}
	return out
}

func TestMatchesForTeamFiltersByPresetCompetitionAndYear(t *testing.T) {
	s := seedInternational(t)
	for _, tc := range []struct {
		name   string
		filter models.MatchFilter
		years  []int
		want   string
	}{
		{"everything, newest first", models.MatchFilter{}, nil, "m4 m3 m2 m1"},
		{"competitive", models.MatchFilter{Preset: models.PresetCompetitive}, nil, "m3 m2"},
		{"tournaments", models.MatchFilter{Preset: models.PresetTournaments}, nil, "m3"},
		{"explicit", models.MatchFilter{Competitions: []string{"int-friendly"}}, nil, "m4 m1"},
		{"explicit, none", models.MatchFilter{Competitions: []string{}}, nil, ""},
		{"one year", models.MatchFilter{}, []int{2025}, "m2 m1"},
		{"preset and year", models.MatchFilter{Preset: models.PresetCompetitive}, []int{2026}, "m3"},
	} {
		got, err := s.MatchesForTeam(ctx, "nat-arcadia", tc.filter, tc.years)
		mustNoErr(t, err)
		if g := join(matchIDs(got)); g != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, g, tc.want)
		}
	}
}

func join(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}

func TestMatchesRoundTripScoresPensAndStats(t *testing.T) {
	s := seedInternational(t)
	ms, err := s.MatchesForCompetitionSeason(ctx, "world-cup", "world-cup:2026")
	mustNoErr(t, err)
	if len(ms) != 1 {
		t.Fatalf("got %d matches", len(ms))
	}
	m := ms[0]
	if m.Status != models.MatchPens || *m.HomePens != 3 || *m.AwayPens != 4 || !m.Neutral || m.Round != "Quarterfinal" {
		t.Errorf("match %+v", m)
	}
	if !m.Kickoff.Equal(kick(2026, 7, 4)) {
		t.Errorf("kickoff %v", m.Kickoff)
	}
	if m.HomeStats == nil || *m.HomeStats.ShotsOnTarget != 3 || m.HomeStats.Corners != nil {
		t.Errorf("home stats %+v", m.HomeStats)
	}
	if m.AwayStats != nil {
		t.Error("a side with no stats row has no stats")
	}
}

func TestSummaryIsComputedFromTheFilteredMatches(t *testing.T) {
	s := seedInternational(t)
	all, err := s.TeamMatchSummary(ctx, "nat-arcadia", models.MatchFilter{}, nil)
	mustNoErr(t, err)
	// m1 W 2-0, m2 D 1-1, m3 D (pens) 0-0; m4 not played.
	if all.Played != 3 || all.Won != 1 || all.Drawn != 2 || all.GoalsFor != 3 || all.GoalsAgainst != 1 || all.Form != "WDD" {
		t.Errorf("all %+v", all)
	}
	if all.Possession == nil || *all.Possession != 58 {
		t.Errorf("possession %v, want (61+55)/2 — the match without stats doesn't count", all.Possession)
	}
	comp, _ := s.TeamMatchSummary(ctx, "nat-arcadia", models.MatchFilter{Preset: models.PresetCompetitive}, nil)
	if comp.Played != 2 || comp.Won != 0 {
		t.Errorf("competitive %+v", comp)
	}
}

func TestARescheduledMatchUpdatesInPlaceAndKeepsItsStats(t *testing.T) {
	s := seedInternational(t)
	moved := models.Match{ID: "m1", CompetitionID: "int-friendly", SeasonID: "int-friendly:2025",
		Kickoff: kick(2025, 3, 21), Status: models.MatchFullTime, HomeTeamID: "nat-arcadia", AwayTeamID: "nat-borduria",
		HomeScore: ptrI(3), AwayScore: ptrI(0), HomeStats: &models.MatchTeamStats{Corners: ptrI(7)},
		Source: "test", UpdatedAt: fixtureTime}
	mustNoErr(t, s.UpsertMatches(ctx, []models.Match{moved}))
	got, _ := s.MatchesForTeam(ctx, "nat-arcadia", models.MatchFilter{Competitions: []string{"int-friendly"}}, []int{2025})
	if len(got) != 1 {
		t.Fatalf("got %d matches, want the one, moved", len(got))
	}
	m := got[0]
	if !m.Kickoff.Equal(kick(2025, 3, 21)) || *m.HomeScore != 3 {
		t.Errorf("match not updated: %+v", m)
	}
	if *m.HomeStats.Corners != 7 || m.HomeStats.Shots == nil || *m.HomeStats.Shots != 14 {
		t.Errorf("stats %+v: new figures merge, missing ones are kept", m.HomeStats)
	}
}

func TestTeamMatchYearsAndKickedOff(t *testing.T) {
	s := seedInternational(t)
	years, err := s.TeamMatchYears(ctx, "nat-arcadia")
	mustNoErr(t, err)
	if len(years) != 2 || years[0] != 2026 || years[1] != 2025 {
		t.Errorf("years %v", years)
	}
	n, err := s.MatchesKickedOff(ctx, "int-friendly", 2026, kick(2026, 11, 1), kick(2026, 11, 20))
	mustNoErr(t, err)
	if n != 1 {
		t.Errorf("kicked off %d, want the scheduled friendly", n)
	}
	if n, _ := s.MatchesKickedOff(ctx, "int-friendly", 2026, kick(2026, 11, 15), kick(2026, 11, 20)); n != 0 {
		t.Errorf("a match that kicked off before 'since' was already seen: %d", n)
	}
	if n, _ := s.MatchesKickedOff(ctx, "int-friendly", 2025, kick(2025, 1, 1), kick(2025, 12, 31)); n != 0 {
		t.Errorf("a finished match isn't waiting on anything: %d", n)
	}
}

func TestMatchXGRoundTrips(t *testing.T) {
	s := seedInternational(t)
	mustNoErr(t, s.UpsertMatches(ctx, []models.Match{{ID: "m5", CompetitionID: "int-friendly", SeasonID: "int-friendly:2026",
		Kickoff: kick(2026, 3, 1), Status: models.MatchFullTime, HomeTeamID: "nat-arcadia", AwayTeamID: "nat-borduria",
		HomeScore: ptrI(1), AwayScore: ptrI(1),
		HomeStats: &models.MatchTeamStats{XG: ptrF(1.84)}, AwayStats: &models.MatchTeamStats{XG: ptrF(0.32), Shots: ptrI(5)},
		Source: "test", UpdatedAt: fixtureTime}}))
	got, err := s.RecentMatches(ctx, "nat-borduria", 1)
	if err != nil || len(got) != 1 || got[0].ID != "m5" {
		t.Fatalf("recent %v, %v", got, err)
	}
	m := got[0]
	if m.HomeStats == nil || m.HomeStats.XG == nil || *m.HomeStats.XG != 1.84 || *m.AwayStats.XG != 0.32 || *m.AwayStats.Shots != 5 {
		t.Errorf("stats %+v / %+v", m.HomeStats, m.AwayStats)
	}
}

func TestRecentMatchesAreFinishedNewestFirstAndLimited(t *testing.T) {
	s := seedInternational(t)
	got, err := s.RecentMatches(ctx, "nat-arcadia", 2)
	mustNoErr(t, err)
	// m4 is scheduled, so it is not a recent game.
	if ids := matchIDs(got); len(ids) != 2 || ids[0] != "m3" || ids[1] != "m2" {
		t.Errorf("recent %v, want [m3 m2]", ids)
	}
	if n, err := s.CountSeasonMatches(ctx, "int-friendly", "int-friendly:2025"); err != nil || n != 1 {
		t.Errorf("season matches %d, %v", n, err)
	}
}

func TestTeamMatchFilterRoundTrips(t *testing.T) {
	s := newTestStore(t)
	f, err := s.GetTeamMatchFilter(ctx, "nat-arcadia")
	mustNoErr(t, err)
	if !f.IsAll() {
		t.Errorf("no saved filter should mean every match, got %+v", f)
	}
	for _, want := range []models.MatchFilter{
		{Preset: models.PresetTournaments},
		{Competitions: []string{"world-cup", "wcq-uefa"}},
		{Competitions: []string{}},
	} {
		mustNoErr(t, s.SetTeamMatchFilter(ctx, "nat-arcadia", want))
		got, err := s.GetTeamMatchFilter(ctx, "nat-arcadia")
		mustNoErr(t, err)
		if got.Preset != want.Preset || len(got.Competitions) != len(want.Competitions) || (want.Competitions != nil) != (got.Competitions != nil) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	}
}

func TestNationalTeamsAreKeptApartFromClubs(t *testing.T) {
	s := newTestStore(t)
	mustNoErr(t, s.UpsertTeam(ctx, models.Team{ID: "eng-arsenal", Name: "Arsenal", Country: "England", Gender: "men"}))
	mustNoErr(t, s.UpsertTeam(ctx, models.Team{ID: "nat-england", Name: "England", Gender: "men", National: true}))
	mustNoErr(t, s.UpsertTeam(ctx, models.Team{ID: "nat-england-women", Name: "England", Gender: "women", National: true}))
	men, err := s.NationalTeams(ctx, "men")
	mustNoErr(t, err)
	if len(men) != 1 || men[0].ID != "nat-england" || !men[0].National {
		t.Errorf("men's nations %+v", men)
	}
	all, _ := s.NationalTeams(ctx, "")
	if len(all) != 2 {
		t.Errorf("all nations %+v", all)
	}
	if club, _ := s.GetTeam(ctx, "eng-arsenal"); club.National {
		t.Error("a club isn't a national team")
	}
}

func TestYearRunsAreSeparateFromSeasonRuns(t *testing.T) {
	s := newTestStore(t)
	at := func(h int) time.Time { return fixtureTime.Add(time.Duration(h) * time.Hour) }
	mustNoErr(t, s.RecordSyncRun(ctx, models.SyncRun{Provider: "espn", CompetitionID: "world-cup", SeasonID: "world-cup:2026",
		StartedAt: at(0), FinishedAt: at(0), Status: models.SyncStatusOK}))
	mustNoErr(t, s.RecordSyncRun(ctx, models.SyncRun{Provider: "espn", CompetitionID: "world-cup", Year: 2026,
		StartedAt: at(1), FinishedAt: at(1), Status: models.SyncStatusFailed, Error: "boom"}))
	mustNoErr(t, s.RecordSyncRun(ctx, models.SyncRun{Provider: "espn", CompetitionID: "world-cup", Year: 2026,
		StartedAt: at(2), FinishedAt: at(2), Status: models.SyncStatusOK, RowsUpserted: 104}))
	mustNoErr(t, s.RecordSyncRun(ctx, models.SyncRun{Provider: "espn", CompetitionID: "int-friendly", Year: 2025,
		StartedAt: at(3), FinishedAt: at(3), Status: models.SyncStatusRateLimited}))

	season, err := s.LatestSyncRun(ctx, "world-cup", "world-cup:2026")
	mustNoErr(t, err)
	if season.Year != 0 || !season.FinishedAt.Equal(at(0)) {
		t.Errorf("a year's fetch mustn't pass for the season's: %+v", season)
	}
	year, err := s.LatestYearRun(ctx, "world-cup", 2026)
	mustNoErr(t, err)
	if !year.Succeeded() || year.RowsUpserted != 104 || year.Year != 2026 {
		t.Errorf("year run %+v", year)
	}
	if _, err := s.LatestYearRun(ctx, "world-cup", 2022); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("never fetched: %v", err)
	}
	runs, err := s.LatestYearRuns(ctx)
	mustNoErr(t, err)
	if len(runs) != 2 || runs[1].CompetitionID != "world-cup" || !runs[1].Succeeded() {
		t.Errorf("latest year runs %+v", runs)
	}
	limited, err := s.LastRateLimited(ctx)
	mustNoErr(t, err)
	if !limited.Equal(at(3)) {
		t.Errorf("last rate limited %v", limited)
	}
}

func TestGroupLabelsOrderAStandingsTable(t *testing.T) {
	s := newTestStore(t)
	mustNoErr(t, s.UpsertCompetition(ctx, models.Competition{ID: "world-cup", Name: "World Cup", Country: "International", Gender: "men", Category: models.CategoryTournament, ShortName: "WC"}))
	mustNoErr(t, s.UpsertSeason(ctx, models.Season{ID: "world-cup:2026", CompetitionID: "world-cup", Label: "2026"}))
	for i, row := range []struct {
		team, group string
		pos         int
	}{{"nat-b2", "Group B", 2}, {"nat-a1", "Group A", 1}, {"nat-b1", "Group B", 1}, {"nat-a2", "Group A", 2}} {
		mustNoErr(t, s.UpsertTeam(ctx, models.Team{ID: row.team, Name: row.team, Gender: "men", National: true}))
		mustNoErr(t, s.LinkTeamCompetitionSeason(ctx, row.team, "world-cup", "world-cup:2026"))
		mustNoErr(t, s.UpsertTeamSeasonStats(ctx, models.TeamSeasonStats{TeamID: row.team, CompetitionID: "world-cup",
			SeasonID: "world-cup:2026", GroupLabel: row.group, Position: ptrI(row.pos), Points: 9 - i, Source: "test", UpdatedAt: fixtureTime}))
	}
	rows, err := s.Standings(ctx, "world-cup", "world-cup:2026")
	mustNoErr(t, err)
	var got []string
	for _, r := range rows {
		got = append(got, r.GroupLabel+"/"+r.TeamID)
	}
	if join(got) != "Group A/nat-a1 Group A/nat-a2 Group B/nat-b1 Group B/nat-b2" {
		t.Errorf("standings order %v", got)
	}
	c, _ := s.GetCompetition(ctx, "world-cup")
	if c.Category != models.CategoryTournament || c.ShortName != "WC" || !c.International() {
		t.Errorf("competition %+v", c)
	}
}
