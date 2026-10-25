package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"meerkat/database/upstreamguard"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func upstreamDB(t *testing.T, dirty bool) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "u.db"), false))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	migrateTo(t, db, 23)
	for _, name := range []string{"000024_add_contact_deceased.up.sql", "000025_backfill_contact_deceased.up.sql"} {
		body, err := os.ReadFile(filepath.Join("testdata", "upstream", name))
		require.NoError(t, err)
		_, err = db.Exec(string(body))
		require.NoError(t, err)
	}
	_, err = db.Exec("UPDATE schema_migrations SET version = 25, dirty = ?", dirty)
	require.NoError(t, err)
	return db
}

func dbSnapshot(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var out []string
	rows, err := db.Query("SELECT name || ':' || COALESCE(sql, '') FROM sqlite_master ORDER BY name")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		out = append(out, s)
	}
	var v int
	var d bool
	require.NoError(t, db.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&v, &d))
	return append(out, fmt.Sprintf("version %d dirty %v", v, d))
}

func TestRunMigrationsRefusesAnUpstreamDatabase(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		t.Run(fmt.Sprintf("dirty=%v", dirty), func(t *testing.T) {
			db := upstreamDB(t, dirty)
			before := dbSnapshot(t, db)

			err := RunMigrations(db)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "version 25")
			assert.Contains(t, err.Error(), "upstream")
			assert.Contains(t, err.Error(), "docs/development/fork.md")
			assert.Equal(t, before, dbSnapshot(t, db))
		})
	}
}

func TestRunMigrationsAcceptsForkDatabases(t *testing.T) {
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "f.db"), false))
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, RunMigrations(db), "fresh database")
	require.NoError(t, RunMigrations(db), "database at the latest fork version")
}

func TestRunMigrationsAcceptsAForkDatabaseBeyond24(t *testing.T) {
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "f.db"), false))
	require.NoError(t, err)
	defer db.Close()
	migrateTo(t, db, 39)

	require.NoError(t, RunMigrations(db))
}

func TestCheckCurrentRefusesAnUpstreamDatabase(t *testing.T) {
	require.Error(t, upstreamguard.CheckCurrent(upstreamDB(t, false)))

	empty, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "e.db"), false))
	require.NoError(t, err)
	defer empty.Close()
	require.NoError(t, upstreamguard.CheckCurrent(empty))
}
