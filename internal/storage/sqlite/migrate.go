package sqlite

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies any migration files that haven't yet been recorded in the
// schema_migrations table, in filename order. Migrations are plain SQL and
// run inside a transaction each.
//
// A database that records a migration this build doesn't have was written
// by a newer footy. Migrate refuses it rather than carrying on: an older
// binary writing rows into a schema it doesn't understand is how a user's
// cache gets quietly corrupted after a downgrade.
func Migrate(db *sql.DB) error {
	return migrateTo(db, "")
}

// ErrNewerSchema is returned when the database was migrated by a newer
// build than this one.
var ErrNewerSchema = errors.New("the database was created by a newer version of footy")

// migrateTo applies migrations up to and including the one whose filename
// starts with upTo, or all of them when upTo is empty. Tests use it to
// build a database as an older release would have left it.
func migrateTo(db *sql.DB, upTo string) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	if err := checkNotNewer(db, names); err != nil {
		return err
	}

	for _, name := range names {
		if upTo != "" && name > upTo && !strings.HasPrefix(name, upTo) {
			break
		}
		var applied int
		if err := db.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, name).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}

		contents, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for migration %s: %w", name, err)
		}

		if _, err := tx.Exec(string(contents)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}

	return nil
}

// checkNotNewer fails if the database records a migration that isn't in
// known.
func checkNotNewer(db *sql.DB, known []string) error {
	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	have := make(map[string]bool, len(known))
	for _, k := range known {
		have[k] = true
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("read schema_migrations: %w", err)
		}
		if !have[v] {
			return fmt.Errorf("%w (it has migration %s, which this build doesn't) — upgrade footy, or point --db at another file", ErrNewerSchema, v)
		}
	}
	return rows.Err()
}
