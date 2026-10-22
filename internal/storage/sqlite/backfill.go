package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/JohnSBarden/footui/internal/textfold"
)

// backfillFoldedNames fills 0004_folded_names.sql's search columns for
// older rows; SQLite can't fold Unicode, so the migration can't. It is a
// no-op once every row is populated.
func backfillFoldedNames(db *sql.DB) error {
	for _, table := range []string{"team", "player", "competition"} {
		if err := backfillTable(db, table); err != nil {
			return err
		}
	}
	return nil
}

func backfillTable(db *sql.DB, table string) error {
	// Table names are from the fixed list above, never from user input.
	rows, err := db.Query(`SELECT id, name FROM ` + table + ` WHERE name_folded IS NULL`)
	if err != nil {
		return fmt.Errorf("sqlite: scan %s for backfill: %w", table, err)
	}

	type row struct{ id, name string }
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.name); err != nil {
			rows.Close()
			return fmt.Errorf("sqlite: scan %s row: %w", table, err)
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	// Close before writing: on a single connection an open read deadlocks
	// the update.
	rows.Close()

	if len(pending) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: begin %s backfill: %w", table, err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE ` + table + ` SET name_folded = ? WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("sqlite: prepare %s backfill: %w", table, err)
	}
	defer stmt.Close()

	for _, r := range pending {
		if _, err := stmt.Exec(textfold.Fold(r.name), r.id); err != nil {
			return fmt.Errorf("sqlite: backfill %s %s: %w", table, r.id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit %s backfill: %w", table, err)
	}
	return nil
}
