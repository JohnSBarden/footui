package sqlite

import (
	"database/sql"
	"errors"
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// updateFixture regenerates the frozen database fixture. Run it only when
// adding a new fixture for a new schema version — never to make a failing
// upgrade test pass, which would defeat the point of having one.
var updateFixture = flag.Bool("update-fixture", false, "write testdata/footy-<schema>.db from the current schema")

func migrationNames(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func rawOpen(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

// Every schema a release could have left behind must open with the
// current build, keep its rows, and come out fully usable — including the
// search columns a later migration added and Open backfills.
func TestEveryOlderSchemaUpgradesWithItsData(t *testing.T) {
	names := migrationNames(t)
	for _, last := range names[:len(names)-1] {
		t.Run("from "+last, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "old.db")
			db := rawOpen(t, path)
			if err := migrateTo(db, last); err != nil {
				t.Fatalf("building the %s schema: %v", last, err)
			}
			// Only tables and columns present since 0001.
			for _, q := range []string{
				`INSERT INTO competition (id, name, country, tier, gender) VALUES ('esp-la-liga', 'La Liga', 'Spain', 1, 'men')`,
				`INSERT INTO season (id, competition_id, label) VALUES ('esp-la-liga:2025', 'esp-la-liga', '2025')`,
				`INSERT INTO team (id, name, country, gender) VALUES ('esp-atletico', 'Atlético Madrid', 'Spain', 'men')`,
				`INSERT INTO team_competition_season (team_id, competition_id, season_id) VALUES ('esp-atletico', 'esp-la-liga', 'esp-la-liga:2025')`,
			} {
				if _, err := db.Exec(q); err != nil {
					t.Fatalf("seeding %s schema: %v", last, err)
				}
			}
			db.Close()

			upgraded, err := Open(path)
			if err != nil {
				t.Fatalf("upgrading from %s: %v", last, err)
			}
			defer upgraded.Close()
			s := NewStore(upgraded)

			comp, err := s.GetCompetition(ctx, "esp-la-liga")
			if err != nil || comp.Name != "La Liga" {
				t.Errorf("competition lost: %+v, %v", comp, err)
			}
			// Accent-insensitive search only works once 0004's columns are
			// backfilled for rows written before it existed.
			teams, err := s.FindTeamsByName(ctx, "atletico")
			if err != nil || len(teams) != 1 {
				t.Errorf("search on an upgraded row found %v, %v", teams, err)
			}
			// And it accepts writes the new schema allows.
			if err := s.RecordSyncRun(ctx, models.SyncRun{Provider: "test", CompetitionID: "esp-la-liga",
				SeasonID: "esp-la-liga:2025", StartedAt: time.Now(), FinishedAt: time.Now(),
				Status: models.SyncStatusOK}); err != nil {
				t.Errorf("writing to the upgraded database: %v", err)
			}
		})
	}
}

// fixtureName is the frozen database for the current schema.
func fixtureName(t *testing.T) string {
	names := migrationNames(t)
	latest := strings.TrimSuffix(names[len(names)-1], ".sql")
	return filepath.Join("testdata", "footy-"+latest[:4]+".db")
}

// The fixtures in testdata are real database files as a past build wrote
// them, committed and never regenerated. Every one must keep opening with
// every later build: this is the test behind "a database from 1.0 opens
// in 1.x". A new schema version adds a new fixture alongside the old ones.
func TestFrozenFixturesOpenAndRead(t *testing.T) {
	if *updateFixture {
		writeFixture(t, fixtureName(t))
	}
	fixtures, _ := filepath.Glob(filepath.Join("testdata", "footy-*.db"))
	if len(fixtures) == 0 {
		t.Fatal("no fixtures in testdata — run go test -run Frozen -update-fixture once")
	}
	for _, fixture := range fixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			path := copyFile(t, fixture)
			db, err := Open(path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer db.Close()
			checkFixture(t, NewStore(db))
		})
	}
}

// The current schema must have a fixture, so the next migration is tested
// against data written by this one.
func TestCurrentSchemaHasAFixture(t *testing.T) {
	if _, err := os.Stat(fixtureName(t)); err != nil {
		t.Errorf("%s is missing: a new migration needs a fixture of the schema it creates — "+
			"run go test ./internal/storage/sqlite -run Frozen -update-fixture and commit it", fixtureName(t))
	}
}

func copyFile(t *testing.T, src string) string {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	dst := filepath.Join(t.TempDir(), filepath.Base(src))
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	return dst
}

var fixtureTime = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

// writeFixture builds the fixture through the repository, the way the
// app writes data, then compacts it into a single self-contained file.
func writeFixture(t *testing.T, path string) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "build.db")
	db, err := Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	mustNoErr(t, s.UpsertCompetition(ctx, models.Competition{ID: "eng-premier-league", Name: "Premier League", Country: "England", Tier: 1, Gender: "men"}))
	mustNoErr(t, s.UpsertSeason(ctx, models.Season{ID: "eng-premier-league:2025", CompetitionID: "eng-premier-league", Label: "2025"}))
	mustNoErr(t, s.UpsertTeam(ctx, models.Team{ID: "eng-arsenal", Name: "Arsenal", Country: "England", Gender: "men"}))
	mustNoErr(t, s.LinkTeamCompetitionSeason(ctx, "eng-arsenal", "eng-premier-league", "eng-premier-league:2025"))
	mustNoErr(t, s.UpsertTeamSeasonStats(ctx, models.TeamSeasonStats{
		TeamID: "eng-arsenal", CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		Position: ptrI(1), MatchesPlayed: 38, Wins: 26, Draws: 7, Losses: 5, GoalsFor: 84, GoalsAgainst: 29,
		GoalDiff: 55, Points: 85, XG: ptrF(77.5), XGA: ptrF(31.2),
		Extras: map[string]float64{models.StatPPDA: 10.3}, Source: "understat", UpdatedAt: fixtureTime,
	}))
	mustNoErr(t, s.UpsertPlayer(ctx, models.Player{ID: "p-saka", Name: "Bukayo Saka", Position: "M", Gender: "men"}))
	mustNoErr(t, s.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{
		PlayerID: "p-saka", TeamID: "eng-arsenal", CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		Appearances: 35, Minutes: 2900, Goals: 16, Assists: 11, XG: ptrF(14.1), Shots: ptrI(96),
		Source: "understat", UpdatedAt: fixtureTime,
	}))
	mustNoErr(t, s.InsertTrophy(ctx, models.Trophy{TeamID: "eng-arsenal", Title: "Premier League", SeasonLabel: "2003-04", TrophyType: "league_title", Source: "seed:v1"}))
	mustNoErr(t, s.UpsertTeamColors(ctx, models.TeamColors{TeamID: "eng-arsenal", PrimaryHex: "#EF0107", SecondaryHex: "#FFFFFF", Source: "seed:v1"}))
	mustNoErr(t, s.RecordSyncRun(ctx, models.SyncRun{Provider: "understat", CompetitionID: "eng-premier-league",
		SeasonID: "eng-premier-league:2025", StartedAt: fixtureTime, FinishedAt: fixtureTime, Status: models.SyncStatusOK, RowsUpserted: 2}))

	// From schema 0005: a calendar-year competition.
	mustNoErr(t, s.UpsertCompetition(ctx, models.Competition{ID: "usa-mls", Name: "MLS", Country: "USA", Tier: 1, Gender: "men", CalendarYear: true}))
	mustNoErr(t, s.UpsertSeason(ctx, models.Season{ID: "usa-mls:2026", CompetitionID: "usa-mls", Label: "2026"}))

	mustNoErr(t, os.MkdirAll(filepath.Dir(path), 0o755))
	_ = os.Remove(path)
	if _, err := db.Exec(`VACUUM INTO ?`, path); err != nil {
		t.Fatal(err)
	}
	db.Close()
	t.Logf("wrote %s", path)
}

// checkFixture reads back everything writeFixture stored.
func checkFixture(t *testing.T, s *Store) {
	t.Helper()
	standings, err := s.Standings(ctx, "eng-premier-league", "eng-premier-league:2025")
	if err != nil || len(standings) != 1 {
		t.Fatalf("standings: %v, %v", standings, err)
	}
	st := standings[0]
	if st.Points != 85 || st.XG == nil || *st.XG != 77.5 || st.PossessionAvg != nil {
		t.Errorf("team stats %+v", st)
	}
	extras, err := s.TeamSeasonStatExtras(ctx, "eng-arsenal", "eng-premier-league", "eng-premier-league:2025")
	if err != nil || extras[models.StatPPDA] != 10.3 {
		t.Errorf("extras %v, %v", extras, err)
	}
	roster, err := s.RosterForTeamSeason(ctx, "eng-arsenal", "eng-premier-league", "eng-premier-league:2025")
	if err != nil || len(roster) != 1 || roster[0].Goals != 16 || *roster[0].Shots != 96 {
		t.Errorf("roster %+v, %v", roster, err)
	}
	if players, _ := s.FindPlayersByName(ctx, "saka"); len(players) != 1 {
		t.Errorf("player search found %v", players)
	}
	if trophies, _ := s.TeamTrophies(ctx, "eng-arsenal"); len(trophies) != 1 {
		t.Errorf("trophies %v", trophies)
	}
	if c, err := s.GetTeamColors(ctx, "eng-arsenal"); err != nil || c.PrimaryHex != "#EF0107" {
		t.Errorf("colors %+v, %v", c, err)
	}
	run, err := s.LatestSyncRun(ctx, "eng-premier-league", "eng-premier-league:2025")
	if err != nil || !run.Succeeded() || !run.FinishedAt.Equal(fixtureTime) {
		t.Errorf("sync log %+v, %v", run, err)
	}

	// Competitions from before 0005 upgrade to split-year seasons.
	pl, _ := s.GetCompetition(ctx, "eng-premier-league")
	if pl.CalendarYear {
		t.Error("a pre-0005 competition must default to split-year seasons")
	}
	// Fixtures from 0005 on carry a calendar-year competition.
	if mls, err := s.GetCompetition(ctx, "usa-mls"); err == nil {
		seasons, _ := s.ListSeasons(ctx, "usa-mls")
		if !mls.CalendarYear || len(seasons) != 1 || !seasons[0].CalendarYear {
			t.Errorf("calendar-year competition lost its flag: %+v %+v", mls, seasons)
		}
	}
}

// A database migrated by a newer footy must be refused, not written to.
func TestANewerSchemaIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES ('9999_from_the_future.sql')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, err = Open(path)
	if !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("got %v, want ErrNewerSchema", err)
	}
	if !strings.Contains(err.Error(), "9999_from_the_future.sql") {
		t.Errorf("the error should name the unknown migration: %v", err)
	}
}

func TestOpenCreatesDirectoriesAndPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "footy.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	mustNoErr(t, NewStore(db).UpsertCompetition(ctx, models.Competition{ID: "c", Name: "C", Country: "X", Tier: 1, Gender: "men"}))
	var mode string
	mustNoErr(t, db.QueryRow(`PRAGMA journal_mode`).Scan(&mode))
	if mode != "wal" {
		t.Errorf("journal mode %q, want wal", mode)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := NewStore(db).GetCompetition(ctx, "c"); err != nil {
		t.Errorf("data lost across reopen: %v", err)
	}
}

func TestOpenRejectsAFileThatIsntADatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	mustNoErr(t, os.WriteFile(path, []byte("definitely not sqlite, just some text that is long enough"), 0o644))
	if db, err := Open(path); err == nil {
		db.Close()
		t.Error("opening a non-database file should fail, not overwrite it")
	}
}
