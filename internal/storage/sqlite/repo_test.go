package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

var ctx = context.Background()

func ptrF(v float64) *float64 { return &v }
func ptrI(v int) *int         { return &v }

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// --- competitions and seasons ---

func TestListCompetitionsIsOrderedAndDeterministic(t *testing.T) {
	s := newTestStore(t)
	for _, c := range []models.Competition{
		{ID: "usa-nwsl", Name: "NWSL", Country: "USA", Tier: 1, Gender: "women"},
		{ID: "usa-mls", Name: "MLS", Country: "USA", Tier: 1, Gender: "men"},
		{ID: "eng-championship", Name: "Championship", Country: "England", Tier: 2, Gender: "men"},
		{ID: "eng-premier-league", Name: "Premier League", Country: "England", Tier: 1, Gender: "men"},
	} {
		mustNoErr(t, s.UpsertCompetition(ctx, c))
	}
	got, err := s.ListCompetitions(ctx)
	mustNoErr(t, err)
	want := []string{"eng-premier-league", "eng-championship", "usa-mls", "usa-nwsl"}
	for i, c := range got {
		if c.ID != want[i] {
			t.Fatalf("order %v, want %v (country, tier, then name)", ids(got), want)
		}
	}
}

func ids(cs []models.Competition) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func TestUpsertCompetitionUpdatesInPlace(t *testing.T) {
	s := newTestStore(t)
	mustNoErr(t, s.UpsertCompetition(ctx, models.Competition{ID: "x", Name: "Old", Country: "England", Tier: 1, Gender: "men"}))
	mustNoErr(t, s.UpsertCompetition(ctx, models.Competition{ID: "x", Name: "New", Country: "England", Tier: 1, Gender: "men"}))
	all, err := s.ListCompetitions(ctx)
	mustNoErr(t, err)
	if len(all) != 1 || all[0].Name != "New" {
		t.Errorf("got %+v, want one row renamed", all)
	}
}

func TestGetCompetitionsAndSeasonsResolveBatches(t *testing.T) {
	s := newTestStore(t)
	a := seedCompetitionSeason(t, s, "a", "2024", "t1", "One")
	b := seedCompetitionSeason(t, s, "b", "2025", "t2", "Two")

	comps, err := s.GetCompetitions(ctx, []string{"a", "b", "missing"})
	mustNoErr(t, err)
	if len(comps) != 2 || comps["a"].ID != "a" {
		t.Errorf("competitions %v", comps)
	}
	seasons, err := s.GetSeasons(ctx, []string{a, b, "missing:1999"})
	mustNoErr(t, err)
	if len(seasons) != 2 || seasons[b].Label != "2025" {
		t.Errorf("seasons %v", seasons)
	}
	if got, err := s.GetSeasons(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("no IDs should be an empty map, got %v, %v", got, err)
	}
}

func TestSeasonDatesRoundTripAndStayNilWhenAbsent(t *testing.T) {
	s := newTestStore(t)
	mustNoErr(t, s.UpsertCompetition(ctx, models.Competition{ID: "c", Name: "C", Country: "X", Tier: 1, Gender: "men"}))
	start := time.Date(2025, 8, 15, 0, 0, 0, 0, time.UTC)
	mustNoErr(t, s.UpsertSeason(ctx, models.Season{ID: "c:2025", CompetitionID: "c", Label: "2025", StartDate: &start}))

	got, err := s.ListSeasons(ctx, "c")
	mustNoErr(t, err)
	if got[0].StartDate == nil || !got[0].StartDate.Equal(start) {
		t.Errorf("start date %v", got[0].StartDate)
	}
	if got[0].EndDate != nil {
		t.Errorf("end date should stay nil, got %v", got[0].EndDate)
	}
}

// --- teams and players ---

func TestListTeamsAndUpsertTeamIdempotent(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 2; i++ {
		mustNoErr(t, s.UpsertTeam(ctx, models.Team{ID: "eng-b", Name: "B", Gender: "men"}))
		mustNoErr(t, s.UpsertTeam(ctx, models.Team{ID: "eng-a", Name: "A", Gender: "men"}))
	}
	teams, err := s.ListTeams(ctx)
	mustNoErr(t, err)
	if len(teams) != 2 || teams[0].ID != "eng-a" {
		t.Errorf("teams %+v, want two ordered by id", teams)
	}
}

func TestGetPlayerAndPlayers(t *testing.T) {
	s := newTestStore(t)
	dob := time.Date(2001, 9, 5, 0, 0, 0, 0, time.UTC)
	mustNoErr(t, s.UpsertPlayer(ctx, models.Player{ID: "p1", Name: "Bukayo Saka", DateOfBirth: &dob, Position: "M", Gender: "men"}))
	mustNoErr(t, s.UpsertPlayer(ctx, models.Player{ID: "p2", Name: "Declan Rice", Gender: "men"}))

	p, err := s.GetPlayer(ctx, "p1")
	mustNoErr(t, err)
	if p.Name != "Bukayo Saka" || p.DateOfBirth == nil || !p.DateOfBirth.Equal(dob) {
		t.Errorf("player %+v", p)
	}
	if _, err := s.GetPlayer(ctx, "nobody"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("unknown player: %v, want ErrNotFound", err)
	}
	batch, err := s.GetPlayers(ctx, []string{"p1", "p2", "p3"})
	mustNoErr(t, err)
	if len(batch) != 2 {
		t.Errorf("batch %v", batch)
	}
}

// --- histories ---

// A club or player's history spans competitions; "most recent first" must
// mean by season, not by competition ID that happens to prefix the season
// ID.
func TestHistoriesAreMostRecentFirstAcrossCompetitions(t *testing.T) {
	s := newTestStore(t)
	// Premier League 2025 and La Liga 2019: "esp-…" sorts after "eng-…",
	// so ordering by season_id would put the older La Liga season first.
	pl := seedCompetitionSeason(t, s, "eng-premier-league", "2025", "club", "Club")
	ll := seedCompetitionSeason(t, s, "esp-la-liga", "2019", "club", "Club")
	for _, season := range []struct{ comp, id string }{{"eng-premier-league", pl}, {"esp-la-liga", ll}} {
		mustNoErr(t, s.UpsertTeamSeasonStats(ctx, models.TeamSeasonStats{
			TeamID: "club", CompetitionID: season.comp, SeasonID: season.id, Source: "test",
		}))
		mustNoErr(t, s.UpsertPlayer(ctx, models.Player{ID: "p", Name: "P", Gender: "men"}))
		mustNoErr(t, s.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{
			PlayerID: "p", TeamID: "club", CompetitionID: season.comp, SeasonID: season.id, Source: "test",
		}))
	}

	team, err := s.TeamHistory(ctx, "club")
	mustNoErr(t, err)
	if len(team) != 2 || team[0].SeasonID != pl {
		t.Errorf("team history %v, want 2025 first", seasonIDs(team))
	}
	player, err := s.PlayerHistory(ctx, "p")
	mustNoErr(t, err)
	if len(player) != 2 || player[0].SeasonID != pl {
		t.Errorf("player history starts with %s, want %s", player[0].SeasonID, pl)
	}
}

func seasonIDs(rows []models.TeamSeasonStats) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.SeasonID
	}
	return out
}

func TestTeamSeasonStatsRoundTripKeepsNullsNull(t *testing.T) {
	s := newTestStore(t)
	season := seedCompetitionSeason(t, s, "c", "2025", "club", "Club")
	in := models.TeamSeasonStats{
		TeamID: "club", CompetitionID: "c", SeasonID: season,
		Position: ptrI(3), MatchesPlayed: 10, Wins: 6, Draws: 2, Losses: 2,
		GoalsFor: 20, GoalsAgainst: 9, GoalDiff: 11, Points: 20,
		XG: ptrF(18.25), Source: "test",
	}
	mustNoErr(t, s.UpsertTeamSeasonStats(ctx, in))
	// Upserting again with new numbers replaces, not duplicates.
	in.Points = 23
	mustNoErr(t, s.UpsertTeamSeasonStats(ctx, in))

	rows, err := s.Standings(ctx, "c", season)
	mustNoErr(t, err)
	if len(rows) != 1 {
		t.Fatalf("%d rows after two upserts, want 1", len(rows))
	}
	got := rows[0]
	if got.Points != 23 || *got.Position != 3 || *got.XG != 18.25 {
		t.Errorf("round trip: %+v", got)
	}
	if got.XGA != nil || got.PossessionAvg != nil || got.Corners != nil {
		t.Error("stats the source never supplied must come back nil, not zero")
	}
}

// --- sync log ---

func TestLatestSyncRun(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.LatestSyncRun(ctx, "c", "c:2025"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("never synced: %v, want ErrNotFound", err)
	}

	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	record := func(at time.Time, status, msg string) {
		mustNoErr(t, s.RecordSyncRun(ctx, models.SyncRun{
			Provider: "test", CompetitionID: "c", SeasonID: "c:2025",
			StartedAt: at.Add(-time.Second), FinishedAt: at, Status: status, Error: msg,
		}))
	}
	record(base, models.SyncStatusOK, "")
	record(base.Add(time.Hour), models.SyncStatusFailed, "timed out")

	run, err := s.LatestSyncRun(ctx, "c", "c:2025")
	mustNoErr(t, err)
	if run.Succeeded() || run.Error != "timed out" || !run.FinishedAt.Equal(base.Add(time.Hour)) {
		t.Errorf("latest %+v, want the later failure", run)
	}

	// A run in another timezone is stored in UTC, so ordering still holds.
	record(base.Add(2*time.Hour).In(time.FixedZone("NYC", -4*3600)), models.SyncStatusOK, "")
	run, _ = s.LatestSyncRun(ctx, "c", "c:2025")
	if !run.Succeeded() {
		t.Error("a later run recorded with a non-UTC clock should still be latest")
	}
}

// Two attempts finishing in the same second — a refresh and an immediate
// retry — must still come back in the order they were recorded.
func TestLatestSyncRunBreaksSameSecondTiesByInsertionOrder(t *testing.T) {
	s := newTestStore(t)
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	for _, status := range []string{models.SyncStatusFailed, models.SyncStatusOK} {
		mustNoErr(t, s.RecordSyncRun(ctx, models.SyncRun{
			Provider: "test", CompetitionID: "c", SeasonID: "c:2025",
			StartedAt: at, FinishedAt: at, Status: status,
		}))
	}
	run, err := s.LatestSyncRun(ctx, "c", "c:2025")
	mustNoErr(t, err)
	if !run.Succeeded() {
		t.Errorf("got %q, want the later-recorded success", run.Status)
	}
}

// --- cache summary ---

func TestCacheSummaryCountsDontMultiplyAcrossJoins(t *testing.T) {
	s := newTestStore(t)
	for _, year := range []string{"2023", "2024", "2025"} {
		season := seedCompetitionSeason(t, s, "c", year, "t1", "One")
		seedCompetitionSeason(t, s, "c", year, "t2", "Two")
		mustNoErr(t, s.UpsertPlayer(ctx, models.Player{ID: "p", Name: "P", Gender: "men"}))
		mustNoErr(t, s.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{
			PlayerID: "p", TeamID: "t1", CompetitionID: "c", SeasonID: season, Source: "test",
		}))
	}
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	mustNoErr(t, s.RecordSyncRun(ctx, models.SyncRun{
		Provider: "test", CompetitionID: "c", SeasonID: "c:2025",
		StartedAt: at, FinishedAt: at, Status: models.SyncStatusOK,
	}))
	// A competition with nothing synced yet.
	mustNoErr(t, s.UpsertCompetition(ctx, models.Competition{ID: "empty", Name: "Empty", Country: "Z", Tier: 1, Gender: "men"}))

	summary, err := s.CacheSummary(ctx)
	mustNoErr(t, err)
	if len(summary) != 2 {
		t.Fatalf("got %d competitions", len(summary))
	}
	c := summary[0]
	if c.Seasons != 3 || c.Teams != 2 || c.PlayerSeasons != 3 {
		t.Errorf("counts seasons=%d teams=%d players=%d, want 3/2/3", c.Seasons, c.Teams, c.PlayerSeasons)
	}
	if c.EarliestSeason != "2023" || c.LatestSeason != "2025" {
		t.Errorf("range %s..%s", c.EarliestSeason, c.LatestSeason)
	}
	if c.LastSync == nil || !c.LastSync.FinishedAt.Equal(at) {
		t.Errorf("last sync %+v", c.LastSync)
	}
	if e := summary[1]; e.Seasons != 0 || e.LastSync != nil || e.LatestSeason != "" {
		t.Errorf("empty competition %+v", e)
	}
}

// --- trophies and colors ---

func TestTrophiesReplaceBySourceAndOrderNewestFirst(t *testing.T) {
	s := newTestStore(t)
	mustNoErr(t, s.UpsertTeam(ctx, models.Team{ID: "club", Name: "Club", Gender: "men"}))
	insert := func(title, season, source string) {
		mustNoErr(t, s.InsertTrophy(ctx, models.Trophy{
			TeamID: "club", Title: title, SeasonLabel: season, TrophyType: "league_title", Source: source,
		}))
	}
	insert("League", "2001-02", "seed:v1")
	insert("League", "2023-24", "seed:v1")
	insert("Cup", "2023-24", "manual")

	got, err := s.TeamTrophies(ctx, "club")
	mustNoErr(t, err)
	if len(got) != 3 || got[0].SeasonLabel != "2023-24" || got[0].Title != "Cup" {
		t.Errorf("order %+v, want newest season first, then title", got)
	}
	if got[0].CompetitionID != "" {
		t.Error("an unset competition should read back empty")
	}

	mustNoErr(t, s.DeleteTrophiesBySource(ctx, "club", "seed:v1"))
	got, _ = s.TeamTrophies(ctx, "club")
	if len(got) != 1 || got[0].Source != "manual" {
		t.Errorf("after deleting seed rows: %+v, want only the manual one", got)
	}
}

func TestTeamColorsUpsertAndNotFound(t *testing.T) {
	s := newTestStore(t)
	mustNoErr(t, s.UpsertTeam(ctx, models.Team{ID: "club", Name: "Club", Gender: "men"}))
	if _, err := s.GetTeamColors(ctx, "club"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("uncurated: %v, want ErrNotFound", err)
	}
	mustNoErr(t, s.UpsertTeamColors(ctx, models.TeamColors{TeamID: "club", PrimaryHex: "#111111", SecondaryHex: "#222222", Source: "a"}))
	mustNoErr(t, s.UpsertTeamColors(ctx, models.TeamColors{TeamID: "club", PrimaryHex: "#333333", Source: "b"}))
	c, err := s.GetTeamColors(ctx, "club")
	mustNoErr(t, err)
	if c.PrimaryHex != "#333333" || c.SecondaryHex != "" || c.Source != "b" {
		t.Errorf("colors %+v, want the second write wholesale", c)
	}
}

// The schema allows NULL in several descriptive columns. footy writes
// empty strings there, but a row from another writer, an older build, or
// a future provider may hold NULL, and reading it must not fail the query.
func TestNullDescriptiveColumnsReadAsEmpty(t *testing.T) {
	s := newTestStore(t)
	for _, q := range []string{
		`INSERT INTO team (id, name, name_folded, short_name, country, gender, venue) VALUES ('t', 'Team', 'team', NULL, NULL, 'men', NULL)`,
		`INSERT INTO player (id, name, name_folded, nationality, position, gender) VALUES ('p', 'Player', 'player', NULL, NULL, 'men')`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if team, err := s.GetTeam(ctx, "t"); err != nil || team.Country != "" {
		t.Errorf("team: %+v, %v", team, err)
	}
	if teams, err := s.ListTeams(ctx); err != nil || len(teams) != 1 {
		t.Errorf("list teams: %v, %v", teams, err)
	}
	if p, err := s.GetPlayer(ctx, "p"); err != nil || p.Position != "" {
		t.Errorf("player: %+v, %v", p, err)
	}
	if found, err := s.FindPlayersByName(ctx, "play"); err != nil || len(found) != 1 {
		t.Errorf("player search: %v, %v", found, err)
	}
}

// --- integrity ---

func TestForeignKeysAreEnforced(t *testing.T) {
	s := newTestStore(t)
	err := s.UpsertTeamSeasonStats(ctx, models.TeamSeasonStats{
		TeamID: "ghost", CompetitionID: "nowhere", SeasonID: "nowhere:2025", Source: "test",
	})
	if err == nil {
		t.Error("stats for a team/season that doesn't exist must be rejected")
	}
}

func TestCancelledContextIsAnError(t *testing.T) {
	s := newTestStore(t)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.ListCompetitions(cancelled); err == nil {
		t.Error("a cancelled context should fail the query")
	}
}
