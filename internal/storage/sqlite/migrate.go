package sqlite

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/trace"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies unrecorded migration files in filename order, one
// transaction each. It refuses a database migrated by a newer build, so a
// downgrade can't corrupt the cache.
func Migrate(db *sql.DB) error {
	return migrateTo(db, "", nil)
}

// ErrNewerSchema means the database was migrated by a newer build.
var ErrNewerSchema = errors.New("the database was created by a newer version of footui")

// migrateTo applies migrations up to and including upTo (all when
// empty); tests use it to build older schemas. upgrading, if set, hears
// how many are pending before they run on a database that has some
// already.
func migrateTo(db *sql.DB, upTo string, upgrading func(pending int)) error {
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

	var pending []string
	seen := false
	for _, name := range names {
		if upTo != "" && name > upTo && !strings.HasPrefix(name, upTo) {
			break
		}
		var applied int
		if err := db.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, name).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied > 0 {
			seen = true
			continue
		}
		pending = append(pending, name)
	}
	if len(pending) > 0 && seen && upgrading != nil {
		upgrading(len(pending))
	}

	for _, name := range pending {
		contents, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		start := time.Now()
		trace.Printf("migration %s: applying", name)
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
		trace.Printf("migration %s: done in %s", name, time.Since(start).Round(time.Millisecond))
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
			return fmt.Errorf("%w (it has migration %s, which this build doesn't) — upgrade footui, or point --db at another file", ErrNewerSchema, v)
		}
	}
	return rows.Err()
}
