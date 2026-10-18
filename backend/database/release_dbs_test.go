package database

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"meerkat/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each file in testdata/releases is a database created by a released version (see
// scripts/seed-release-db.sh), so these tests run what an upgrade does to real old databases.
func TestEveryReleaseDatabaseMigratesToTheNewestVersionAndStillWorks(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "releases", "*.db"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "no release databases: run scripts/seed-release-db.sh <tag>")

	for _, file := range files {
		tag := strings.TrimSuffix(filepath.Base(file), ".db")
		t.Run(tag, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "meerkat.db")
			copyFile(t, file, path)
			from := schemaVersion(t, path)

			db, err := InitDB(path)
			require.NoError(t, err, "upgrading from %s", tag)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })

			var version uint
			var dirty bool
			require.NoError(t, sqlDB.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty))
			assert.False(t, dirty)
			assert.Equal(t, newestMigration(t), version)

			var user models.User
			require.NoError(t, db.Where("username = ?", "release_user").First(&user).Error)
			assertSeededContacts(t, sqlDB, from)
			var notes int64
			require.NoError(t, db.Model(&models.Note{}).Where("user_id = ?", user.ID).Count(&notes).Error)
			assert.EqualValues(t, 1, notes)
			var reminders int64
			require.NoError(t, db.Model(&models.Reminder{}).Where("user_id = ?", user.ID).Count(&reminders).Error)
			assert.EqualValues(t, 1, reminders)
			var contacts []models.Contact
			require.NoError(t, db.Where("user_id = ?", user.ID).Order("id").Find(&contacts).Error)

			created := models.Contact{UserID: user.ID, Firstname: "Added", Lastname: "After"}
			require.NoError(t, db.Create(&created).Error)
			note := models.Note{UserID: user.ID, ContactID: &created.ID, Content: "new note"}
			require.NoError(t, db.Create(&note).Error)
			require.NoError(t, db.Model(&contacts[0]).Update("firstname", "Augusta").Error)
			require.NoError(t, db.Delete(&created).Error)

			rows, err := sqlDB.Query("PRAGMA foreign_key_check")
			require.NoError(t, err)
			defer rows.Close()
			assert.False(t, rows.Next(), "no foreign key violations after the upgrade")
		})
	}
}

func newestMigration(t *testing.T) uint {
	t.Helper()
	entries, err := migrationsFS.ReadDir("migrations")
	require.NoError(t, err)
	var newest uint
	for _, e := range entries {
		var n uint
		if _, err := fmt.Sscanf(e.Name(), "%d_", &n); err == nil && n > newest {
			newest = n
		}
	}
	return newest
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	in, err := os.Open(from)
	require.NoError(t, err)
	defer in.Close()
	out, err := os.Create(to)
	require.NoError(t, err)
	defer out.Close()
	_, err = io.Copy(out, in)
	require.NoError(t, err)
}

func schemaVersion(t *testing.T, path string) uint {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer db.Close()
	var v uint
	require.NoError(t, db.QueryRow("SELECT version FROM schema_migrations").Scan(&v))
	return v
}

// assertSeededContacts checks every value the seeder wrote, as the newest schema holds it. What a
// data migration had to move or fill depends on the version the database came from.
func assertSeededContacts(t *testing.T, db *sql.DB, from uint) {
	t.Helper()
	type contact struct {
		last, nick, email, phone, birthday, address, met, uid, etag, title, role, urls, note, extra string
	}
	read := func(first string) contact {
		var c contact
		require.NoError(t, db.QueryRow(`SELECT lastname, COALESCE(nickname,''), COALESCE(email,''), COALESCE(phone,''), COALESCE(birthday,''),
			COALESCE(address,''), COALESCE(how_we_met,''), COALESCE(vcard_uid,''), COALESCE(etag,''), COALESCE(job_title,''), COALESCE(role,''),
			COALESCE(urls,''), COALESCE(note,''), COALESCE(vcard_extra,'') FROM contacts WHERE firstname = ?`, first).Scan(
			&c.last, &c.nick, &c.email, &c.phone, &c.birthday, &c.address, &c.met, &c.uid, &c.etag, &c.title, &c.role, &c.urls, &c.note, &c.extra))
		return c
	}

	ada := read("Ada")
	assert.Equal(t, "Lovelace", ada.last)
	assert.Equal(t, "Countess", ada.nick)
	assert.Equal(t, "ada@example.com", ada.email)
	assert.Equal(t, "+44 20 7946 0000", ada.phone)
	assert.Equal(t, "1815-12-10", ada.birthday, "the birthday format migration")
	assert.Equal(t, "London", ada.address)
	assert.Equal(t, "Maths club", ada.met)
	assert.NotEmpty(t, ada.uid, "a contact without a vCard UID gets one")
	assert.NotEmpty(t, ada.etag)
	if from < 9 {
		assert.Regexp(t, `^e-\d+-\d+$`, ada.etag, "the etag backfill")
	}
	if from >= 11 {
		assert.Equal(t, "uid-ada", ada.uid, "an existing UID is kept")
	}
	if from >= 8 {
		assert.Equal(t, "Countess", ada.title, "TITLE moved out of vcard_extra")
		assert.Equal(t, "Mathematician", ada.role)
		assert.Contains(t, ada.urls, "https://example.com/ada")
		assert.Equal(t, "Wrote the first program", ada.note, "NOTE moved out of vcard_extra")
		assert.NotContains(t, ada.extra, "NOTE")
		assert.NotContains(t, ada.extra, "TITLE")
	}

	grace := read("Grace")
	assert.Equal(t, "Hopper", grace.last)
	if from >= 8 {
		assert.Equal(t, "uid-grace", grace.uid)
		assert.Equal(t, "etag-grace", grace.etag)
	}
}
