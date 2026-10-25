// Package upstreamguard recognises a database that upstream Meerkat migrated. It imports no SQL
// driver, so the migrate command can use it next to its own.
package upstreamguard

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func Check(db *sql.DB, version uint) error {
	if version < 24 {
		return nil
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'ics_feeds'").Scan(&n); err != nil {
		return fmt.Errorf("failed to inspect the database: %w", err)
	}
	if n > 0 {
		return nil
	}
	return fmt.Errorf("database is at migration version %d but has no ics_feeds table: upstream Meerkat may have migrated it, "+
		"and fork migrations from 24 on differ from upstream's; nothing was changed (see docs/development/fork.md, \"Coming from upstream\")", version)
}

func CheckCurrent(db *sql.DB) error {
	var version uint
	err := db.QueryRow("SELECT version FROM schema_migrations LIMIT 1").Scan(&version)
	if errors.Is(err, sql.ErrNoRows) || (err != nil && strings.Contains(err.Error(), "no such table")) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read the migration version: %w", err)
	}
	return Check(db, version)
}
