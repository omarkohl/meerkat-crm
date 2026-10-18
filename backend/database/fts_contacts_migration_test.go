package database

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestContactIndexCoversEveryFieldAndKeepsInSyncThenGoesBack(t *testing.T) {
	db, err := sql.Open("sqlite", sqliteDSN(filepath.Join(t.TempDir(), "fts.db"), true))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	migrateTo(t, db, 33)
	mustExec("INSERT INTO users (id, username, email, password) VALUES (1, 'u', 'u@example.com', 'x')")
	mustExec(`INSERT INTO contacts (id, user_id, firstname, lastname, organization, how_we_met, note, address, addresses, circles, custom_fields) VALUES
	  (1, 1, 'Ada', 'Lovelace', 'Analytical Engines', 'at a lecture', 'prefers tea', 'Old Street 1',
	   '[{"type":"home","street":"Rosenweg 4","city":"Hamburg","region":"","postal":"20095","country":"Germany"}]',
	   '["Climbing club","Family"]', '{"Favourite bird":"Pelican"}'),
	  (2, 1, 'Bob', 'Smith', NULL, NULL, NULL, NULL, 'not json', 'null', NULL)`)

	migrateTo(t, db, 34)
	for _, m := range []string{"engines", "lecture", "tea", "old street", "rosenweg", "hamburg", "20095", "germany", "climbing", "family", "pelican", "ada", "smith"} {
		want := 1
		if ftsCount(t, db, "contacts_fts", m) != want {
			t.Errorf("contacts_fts MATCH %q: want %d hit after migration", m, want)
		}
	}
	if ftsCount(t, db, "contacts_fts", "bird") != 0 {
		t.Error("custom field names are not indexed, only their values")
	}

	mustExec("UPDATE contacts SET organization = 'Difference Engines' WHERE id = 1")
	if ftsCount(t, db, "contacts_fts", "analytical") != 0 || ftsCount(t, db, "contacts_fts", "difference") != 1 {
		t.Error("organization change not reindexed")
	}
	mustExec(`UPDATE contacts SET custom_fields = '{"Favourite bird":"Heron"}', circles = '["Chess"]', addresses = '[]', address = '' WHERE id = 1`)
	for m, want := range map[string]int{"pelican": 0, "heron": 1, "climbing": 0, "chess": 1, "hamburg": 0, "old": 0} {
		if got := ftsCount(t, db, "contacts_fts", m); got != want {
			t.Errorf("after the JSON fields changed, %q gave %d hits, want %d", m, got, want)
		}
	}
	mustExec("UPDATE contacts SET note = 'now coffee' WHERE id = 1")
	if ftsCount(t, db, "contacts_fts", "tea") != 0 || ftsCount(t, db, "contacts_fts", "coffee") != 1 {
		t.Error("note change not reindexed")
	}
	mustExec("UPDATE contacts SET photo = 'x' WHERE id = 1")
	if ftsCount(t, db, "contacts_fts", "heron") != 1 {
		t.Error("unrelated update broke the index")
	}
	mustExec("UPDATE contacts SET deleted_at = '2030-01-01' WHERE id = 1")
	mustExec("UPDATE contacts SET deleted_at = NULL WHERE id = 1")
	if ftsCount(t, db, "contacts_fts", "heron") != 1 {
		t.Error("trashing and restoring a contact lost it from the index")
	}
	mustExec("DELETE FROM contacts WHERE id = 1")
	if ftsCount(t, db, "contacts_fts", "heron") != 0 {
		t.Error("delete not removed from the index")
	}

	mustExec("INSERT INTO contacts (id, user_id, firstname, organization, emails) VALUES (3, 1, 'Cy', 'Initech', '[{\"value\":\"cy@example.org\"}]')")
	migrateTo(t, db, 33)
	var columns int
	if err := db.QueryRow("SELECT count(*) FROM pragma_table_info('contacts_fts')").Scan(&columns); err != nil || columns != 7 {
		t.Errorf("going down must restore the index of migration 29 (7 columns), got %d (%v)", columns, err)
	}
	if ftsCount(t, db, "contacts_fts", "cy") != 1 || ftsCount(t, db, "contacts_fts", "example") != 1 || ftsCount(t, db, "contacts_fts", "smith") != 1 {
		t.Error("the restored index lost contacts")
	}
	mustExec("UPDATE contacts SET firstname = 'Cyrus' WHERE id = 3")
	if ftsCount(t, db, "contacts_fts", "cyrus") != 1 {
		t.Error("the restored triggers do not index")
	}
	migrateTo(t, db, 34)
	if ftsCount(t, db, "contacts_fts", "initech") != 1 {
		t.Error("going up again did not rebuild the index from the contacts")
	}
}
