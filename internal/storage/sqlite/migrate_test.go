package sqlite

import (
	"testing"
)

func TestOpenAppliesMigrations(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	tables := []string{
		"competition", "season", "team", "team_competition_season",
		"team_season_stats", "team_season_stat_extra", "player",
		"player_season_stats", "trophy", "team_colors", "external_ref",
		"sync_log", "schema_migrations",
	}
	for _, table := range tables {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist: %v", table, err)
		}
	}

	var version string
	if err := db.QueryRow(`SELECT version FROM schema_migrations WHERE version = '0001_init.sql'`).Scan(&version); err != nil {
		t.Errorf("expected 0001_init.sql to be recorded as applied: %v", err)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate call should be a no-op, got error: %v", err)
	}
}

func TestTeamCompetitionSeasonEnforcesNotBlended(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("exec %q: %v", query, err)
		}
	}

	exec(`INSERT INTO team (id, name, gender) VALUES ('eng-manchester-united', 'Manchester United', 'men')`)
	exec(`INSERT INTO competition (id, name, country, tier, gender) VALUES
		('eng-premier-league', 'Premier League', 'England', 1, 'men'),
		('eur-champions-league', 'UEFA Champions League', 'Europe', 1, 'men')`)
	exec(`INSERT INTO season (id, competition_id, label) VALUES
		('eng-premier-league:2023-24', 'eng-premier-league', '2023-24'),
		('eur-champions-league:2023-24', 'eur-champions-league', '2023-24')`)
	exec(`INSERT INTO team_competition_season (team_id, competition_id, season_id) VALUES
		('eng-manchester-united', 'eng-premier-league', 'eng-premier-league:2023-24'),
		('eng-manchester-united', 'eur-champions-league', 'eur-champions-league:2023-24')`)
	exec(`INSERT INTO team_season_stats (team_id, competition_id, season_id, points, xg, source, updated_at) VALUES
		('eng-manchester-united', 'eng-premier-league', 'eng-premier-league:2023-24', 60, 55.1, 'fbref', datetime('now')),
		('eng-manchester-united', 'eur-champions-league', 'eur-champions-league:2023-24', 10, 8.4, 'fbref', datetime('now'))`)

	rows, err := db.Query(`SELECT competition_id, points, xg FROM team_season_stats WHERE team_id = 'eng-manchester-united' ORDER BY competition_id`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	got := map[string][2]float64{}
	for rows.Next() {
		var comp string
		var points, xg float64
		if err := rows.Scan(&comp, &points, &xg); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[comp] = [2]float64{points, xg}
	}

	if got["eng-premier-league"] == got["eur-champions-league"] {
		t.Fatalf("expected distinct per-competition stats, got identical rows: %v", got)
	}
	if got["eng-premier-league"][0] != 60 || got["eur-champions-league"][0] != 10 {
		t.Fatalf("stats blended across competitions, got: %v", got)
	}
}
