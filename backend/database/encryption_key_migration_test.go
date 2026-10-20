package database

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestEncryptionKeyMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key.db")
	db, err := sql.Open("sqlite", sqliteDSN(path, true))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrateTo(t, db, 35)
	for _, q := range []string{
		"INSERT INTO users (id, username, email, password, totp_secret, totp_enabled, totp_last_step) VALUES (1, 'u', 'u@example.com', 'x', 'old', 1, 99)",
		"INSERT INTO recovery_codes (user_id, code_hash) VALUES (1, 'h')",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	// Up: two-factor from before ENCRYPTION_KEY is turned off; it used a key no longer read
	migrateTo(t, db, 36)
	var secret string
	var enabled bool
	var step, codes int64
	if err := db.QueryRow("SELECT totp_secret, totp_enabled, totp_last_step FROM users WHERE id = 1").Scan(&secret, &enabled, &step); err != nil {
		t.Fatal(err)
	}
	if secret != "" || enabled || step != 0 {
		t.Fatalf("two-factor after up = %q %v %d", secret, enabled, step)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM recovery_codes").Scan(&codes); err != nil || codes != 0 {
		t.Fatalf("recovery codes after up = %d (err %v)", codes, err)
	}
	if _, err := db.Exec("INSERT INTO encryption_key_check (id, value) VALUES (1, 'v2:x')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO encryption_key_check (id, value) VALUES (2, 'v2:y')"); err == nil {
		t.Fatal("encryption_key_check holds one row only")
	}

	migrateTo(t, db, 35)
	if tableExists(t, db, "encryption_key_check") {
		t.Fatal("encryption_key_check still exists after down")
	}
}
