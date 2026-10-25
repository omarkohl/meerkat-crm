package database

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"math"
	"meerkat/database/upstreamguard"
	"meerkat/logger"
	"regexp"
	"strconv"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// sqliteDSN appends the connection pragmas. They are set per connection (via
// the driver's _pragma parameter) because busy_timeout, foreign_keys and
// synchronous do not persist; journal_mode=WAL persists in the file.
// _txlock=immediate takes the write lock at BEGIN, where busy_timeout waits for it: a
// deferred transaction that reads and then writes fails at once with SQLITE_BUSY.
func sqliteDSN(path string, foreignKeys bool) string {
	params := []string{"_pragma=journal_mode(WAL)", "_pragma=busy_timeout(5000)", "_pragma=synchronous(NORMAL)"}
	if foreignKeys {
		params = append(params, "_pragma=foreign_keys(1)")
	}
	params = append(params, "_txlock=immediate")
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + strings.Join(params, "&")
}

// foreignKeyViolations runs PRAGMA foreign_key_check and returns the number of
// violating rows per "table -> parent". It contains no row data.
func foreignKeyViolations(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var table, parent string
		var rowid, fkid sql.NullInt64
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return nil, err
		}
		counts[table+" -> "+parent]++
	}
	return counts, rows.Err()
}

func warnForeignKeyViolations(db *sql.DB) {
	counts, err := foreignKeyViolations(db)
	if err != nil {
		logger.Warn().Err(err).Msg("Could not run foreign key check")
		return
	}
	for rel, n := range counts {
		logger.Warn().Str("relation", rel).Int("rows", n).Msg("Foreign key violations found (orphan rows); writes touching them may fail now that foreign keys are enforced")
	}
}

// InitDB initializes the database connection and runs migrations
func InitDB(dbPath string) (*gorm.DB, error) {
	// Open database connection for migrations
	// Foreign keys stay off here: migrations may recreate tables, which with
	// enforcement on would cascade deletes or fail on dependants.
	sqlDB, err := sql.Open("sqlite", sqliteDSN(dbPath, false))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Run migrations
	err = RunMigrations(sqlDB)
	if err == nil {
		// Databases created while foreign keys were off may hold orphan rows.
		warnForeignKeyViolations(sqlDB)
	}
	_ = sqlDB.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Open GORM connection
	db, err := gorm.Open(sqlite.Open(sqliteDSN(dbPath, true)), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect with GORM: %w", err)
	}

	return db, nil
}

// legacyThumbnailDrop matches a migration that removes the old base64 thumbnail column
// (contacts.photo_thumbnail, not photo_thumbnail_file).
var legacyThumbnailDrop = regexp.MustCompile(`(?i)DROP\s+COLUMN\s+"?photo_thumbnail"?\s*(;|$)`)

// checkLegacyThumbnailsExported refuses to run a pending migration that drops
// contacts.photo_thumbnail while some row still has its thumbnail only there: the data
// would be lost silently. services.ExportLegacyThumbnails moves it into files at startup,
// but it runs after the migrations, so a version that drops the column must be preceded by
// one that has run the export (see docs/development/backend.md).
func checkLegacyThumbnailsExported(db *sql.DB, fsys fs.FS, current uint) error {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return fmt.Errorf("failed to read migrations: %w", err)
	}
	drops := false
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		v, err := strconv.ParseUint(strings.SplitN(name, "_", 2)[0], 10, 32)
		if err != nil || uint(v) <= current {
			continue
		}
		body, err := fs.ReadFile(fsys, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", name, err)
		}
		if legacyThumbnailDrop.Match(body) {
			drops = true
		}
	}
	if !drops {
		return nil
	}
	var n int
	err = db.QueryRow(`SELECT COUNT(*) FROM contacts WHERE photo_thumbnail LIKE 'data:%' AND photo_thumbnail_file = ''`).Scan(&n)
	if err != nil {
		// Without the columns there is nothing to lose.
		return nil
	}
	if n > 0 {
		return fmt.Errorf("refusing to drop contacts.photo_thumbnail: %d contacts have their thumbnail only there; "+
			"start the previous Meerkat version once so that services.ExportLegacyThumbnails moves them into files, then upgrade", n)
	}
	return nil
}

// RunMigrations runs all pending database migrations
func RunMigrations(db *sql.DB) error {
	driver, err := withInstance(db, &sqliteConfig{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("failed to create migration instance: %w", err)
	}

	// Get current version
	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if err := upstreamguard.Check(db, version); err != nil {
		return err
	}

	if dirty {
		logger.Warn().Uint("version", version).Msg("Database is in dirty state, forcing version")
		if version > math.MaxInt32 {
			return fmt.Errorf("migration version %d out of range", version)
		}
		if err := m.Force(int(version)); err != nil {
			return fmt.Errorf("failed to force version: %w", err)
		}
	}

	if err := checkLegacyThumbnailsExported(db, migrationsFS, version); err != nil {
		return err
	}

	// Run migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Get final version
	version, _, err = m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get final version: %w", err)
	}

	if err == migrate.ErrNilVersion {
		logger.Info().Msg("No migrations applied (database is empty)")
	} else {
		logger.Info().Uint("version", version).Msg("Migrations applied successfully")
	}

	return nil
}
