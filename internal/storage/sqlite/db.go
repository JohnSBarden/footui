// Package sqlite provides the local SQLite-backed cache/store for footui,
// plus repositories that are the only path the CLI and TUI layers use to
// reach persisted data — neither talks to the database directly.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open opens (creating if necessary) the SQLite database at path, applies
// pragmas suited to a single-process CLI/TUI workload, and runs any pending
// migrations.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("sqlite: create db dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}

	// A CLI/TUI process talks to its own local file; one connection avoids
	// SQLite's writer-lock contention entirely.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: set pragmas: %w", err)
	}

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}

	return db, nil
}
