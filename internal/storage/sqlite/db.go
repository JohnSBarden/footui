// Package sqlite is footui's SQLite-backed store.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open opens or creates the database at path, sets pragmas, and runs
// pending migrations.
func Open(path string) (*sql.DB, error) {
	return OpenNotify(path, nil)
}

// OpenNotify is Open, calling upgrading first when an existing database
// has migrations to apply, since those can take a while on a big one. A
// new database's are quick and go unannounced.
func OpenNotify(path string, upgrading func(pending int)) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("sqlite: create db dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}

	// One connection avoids SQLite writer-lock contention.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: set pragmas: %w", err)
	}

	if err := migrateTo(db, "", upgrading); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}

	// SQL can't fold Unicode, so search columns are backfilled here.
	if err := backfillFoldedNames(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
