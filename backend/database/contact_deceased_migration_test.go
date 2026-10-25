package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactDeceasedMigration(t *testing.T) {
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "deceased.db"), true))
	require.NoError(t, err)
	defer db.Close()
	migrateTo(t, db, 39)

	for _, q := range []string{
		`INSERT INTO users (id, username, email, password, custom_field_names) VALUES (1, 'u', 'u@example.com', 'x', '["Deceased","Pet"]')`,
		`INSERT INTO contacts (id, user_id, firstname, custom_fields) VALUES (1, 1, 'Yes', '{"Deceased":"yes","Pet":"cat"}')`,
		`INSERT INTO contacts (id, user_id, firstname, custom_fields) VALUES (2, 1, 'Full', '{"Deceased":"2020-01-15"}')`,
		`INSERT INTO contacts (id, user_id, firstname, custom_fields) VALUES (3, 1, 'NoYear', '{"Deceased":"--03-04"}')`,
		`INSERT INTO contacts (id, user_id, firstname, custom_fields) VALUES (4, 1, 'No', '{"Deceased":"no"}')`,
		`INSERT INTO contacts (id, user_id, firstname, custom_fields) VALUES (5, 1, 'Empty', '{"Deceased":""}')`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}

	migrateTo(t, db, 41)

	type row struct {
		deceased bool
		date     string
		custom   string
	}
	read := func(id int) row {
		var r row
		require.NoError(t, db.QueryRow("SELECT is_deceased, deceased_date, custom_fields FROM contacts WHERE id = ?", id).Scan(&r.deceased, &r.date, &r.custom))
		return r
	}
	assert.Equal(t, row{true, "", `{"Pet":"cat"}`}, read(1))
	assert.Equal(t, row{true, "2020-01-15", `{}`}, read(2))
	assert.Equal(t, row{true, "", `{}`}, read(3))
	assert.Equal(t, row{false, "", `{"Deceased":"no"}`}, read(4))
	assert.Equal(t, row{false, "", `{"Deceased":""}`}, read(5))
	var names string
	require.NoError(t, db.QueryRow("SELECT custom_field_names FROM users WHERE id = 1").Scan(&names))
	assert.Equal(t, `["Pet"]`, names)

	migrateTo(t, db, 39)
	assert.False(t, columnExists(t, db, "contacts", "is_deceased"))
	assert.Equal(t, `{"Pet":"cat","Deceased":"yes"}`, readCustom(t, db, 1))
	assert.Equal(t, `{"Deceased":"2020-01-15"}`, readCustom(t, db, 2))
	assert.Equal(t, `{"Deceased":"yes"}`, readCustom(t, db, 3))
	require.NoError(t, db.QueryRow("SELECT custom_field_names FROM users WHERE id = 1").Scan(&names))
	assert.Equal(t, `["Pet","Deceased"]`, names)
}

func readCustom(t *testing.T, db *sql.DB, id int) string {
	t.Helper()
	var s string
	require.NoError(t, db.QueryRow("SELECT custom_fields FROM contacts WHERE id = ?", id).Scan(&s))
	return s
}
