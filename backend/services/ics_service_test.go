package services

import (
	"strings"
	"testing"
	"time"

	"meerkat/i18n"
	"meerkat/models"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupICSTestDB(t testing.TB) *gorm.DB {
	t.Helper()
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Reminder{}))
	return db
}

func icsUser(t testing.TB, db *gorm.DB, name string) models.User {
	t.Helper()
	u := models.User{Username: name, Password: "password123!A", Email: name + "@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func TestEscapeICSText(t *testing.T) {
	assert.Equal(t, `a\\b\;c\,d\ne`, escapeICSText("a\\b;c,d\r\ne"))
	assert.Equal(t, `one\ntwo`, escapeICSText("one\ntwo"))
	assert.Equal(t, "plain", escapeICSText("plain\x00\x07"))
}

func TestFoldICSLine(t *testing.T) {
	short := "SUMMARY:short"
	assert.Equal(t, short+"\r\n", foldICSLine(short))

	long := "SUMMARY:" + strings.Repeat("x", 200)
	folded := foldICSLine(long)
	lines := strings.Split(strings.TrimSuffix(folded, "\r\n"), "\r\n")
	require.Greater(t, len(lines), 1)
	for i, l := range lines {
		assert.LessOrEqual(t, len(l), 75, "line %d", i)
		if i > 0 {
			assert.True(t, strings.HasPrefix(l, " "), "continuation must start with a space")
		}
	}
	// Unfolding restores the original
	assert.Equal(t, long, strings.ReplaceAll(strings.TrimSuffix(folded, "\r\n"), "\r\n ", ""))
}

func TestFoldICSLineKeepsMultibyteCharactersWhole(t *testing.T) {
	long := "SUMMARY:" + strings.Repeat("é", 100) + strings.Repeat("日", 50)
	folded := foldICSLine(long)
	lines := strings.Split(strings.TrimSuffix(folded, "\r\n"), "\r\n")
	for _, l := range lines {
		assert.LessOrEqual(t, len(l), 75)
		assert.True(t, strings.ToValidUTF8(l, "") == l, "no split rune")
	}
	assert.Equal(t, long, strings.ReplaceAll(strings.TrimSuffix(folded, "\r\n"), "\r\n ", ""))
}

// Hardening found by FuzzICSText: a run of 75 or more UTF-8 continuation bytes made the fold
// loop find no rune start to cut at and spin forever. Current callers escape their input first
// (which replaces invalid bytes), so this was not reachable from outside.
func TestFoldICSLineTerminatesOnInvalidUTF8(t *testing.T) {
	in := strings.Repeat("\x80", 200)
	done := make(chan string, 1)
	go func() { done <- foldICSLine(in) }()
	select {
	case out := <-done:
		assert.Equal(t, in, strings.ReplaceAll(strings.TrimSuffix(out, "\r\n"), "\r\n ", ""))
		for _, l := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
			assert.LessOrEqual(t, len(l), icsLineLimit)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("foldICSLine did not terminate")
	}
}

func TestBuildICSFeedBirthdays(t *testing.T) {
	db := setupICSTestDB(t)
	u := icsUser(t, db, "a")
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Ada", Lastname: "Lovelace", Birthday: "1815-12-10"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Bob", Nickname: "Bobby", Birthday: "--03-05"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Nobody"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Archived", Birthday: "1990-01-01", Archived: true}).Error)

	out, err := BuildICSFeed(db, u.ID, "en")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(out, "BEGIN:VCALENDAR\r\n"))
	assert.True(t, strings.HasSuffix(out, "END:VCALENDAR\r\n"))
	assert.NotContains(t, strings.ReplaceAll(out, "\r\n", ""), "\n")
	assert.Contains(t, out, "VERSION:2.0\r\n")
	assert.Contains(t, out, "PRODID:")
	assert.Contains(t, out, "DTSTART;VALUE=DATE:18151210\r\n")
	assert.Contains(t, out, "DTSTART;VALUE=DATE:19720305\r\n") // yearless -> leap base year
	assert.Equal(t, 2, strings.Count(out, "RRULE:FREQ=YEARLY\r\n"))
	assert.Contains(t, out, "SUMMARY:Ada Lovelace's birthday\r\n")
	assert.Contains(t, out, "SUMMARY:Bobby's birthday\r\n")
	assert.NotContains(t, out, "Nobody")
	assert.NotContains(t, out, "Archived")
	assert.Equal(t, 2, strings.Count(out, "BEGIN:VEVENT"))
	assert.Contains(t, out, "TRANSP:TRANSPARENT")
}

func TestBuildICSFeedSkipsDeceasedContactsBirthdays(t *testing.T) {
	db := setupICSTestDB(t)
	u := icsUser(t, db, "a")
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Alive", Birthday: "1990-01-01"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Departed", Birthday: "1980-02-02", IsDeceased: true}).Error)

	out, err := BuildICSFeed(db, u.ID, "en")
	require.NoError(t, err)

	assert.Contains(t, out, "Alive's birthday")
	assert.NotContains(t, out, "Departed")
}

func TestBuildICSFeedUIDsAreStableAndStampFollowsUpdate(t *testing.T) {
	db := setupICSTestDB(t)
	u := icsUser(t, db, "a")
	c := models.Contact{UserID: u.ID, Firstname: "Ada", Birthday: "1815-12-10"}
	require.NoError(t, db.Create(&c).Error)

	first, err := BuildICSFeed(db, u.ID, "en")
	require.NoError(t, err)
	second, err := BuildICSFeed(db, u.ID, "en")
	require.NoError(t, err)
	assert.Equal(t, first, second, "output must be deterministic so ETags work")
	assert.Contains(t, first, "UID:contact-birthday-1@meerkat-crm\r\n")
	assert.Regexp(t, `DTSTAMP:\d{8}T\d{6}Z\r\n`, first)
}

func TestBuildICSFeedSkipsInvalidBirthdays(t *testing.T) {
	db := setupICSTestDB(t)
	u := icsUser(t, db, "a")
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Bad", Birthday: "2020-02-31"}).Error)
	out, err := BuildICSFeed(db, u.ID, "en")
	require.NoError(t, err)
	assert.NotContains(t, out, "BEGIN:VEVENT")
}

func TestBuildICSFeedReminders(t *testing.T) {
	db := setupICSTestDB(t)
	u := icsUser(t, db, "a")
	c := models.Contact{UserID: u.ID, Firstname: "Ada", Lastname: "Lovelace"}
	require.NoError(t, db.Create(&c).Error)

	at := time.Date(2030, 4, 8, 0, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"once":       "",
		"weekly":     "RRULE:FREQ=WEEKLY\r\n",
		"monthly":    "RRULE:FREQ=MONTHLY\r\n",
		"quarterly":  "RRULE:FREQ=MONTHLY;INTERVAL=3\r\n",
		"six-months": "RRULE:FREQ=MONTHLY;INTERVAL=6\r\n",
		"yearly":     "RRULE:FREQ=YEARLY\r\n",
	}
	for rec, want := range cases {
		db.Exec("DELETE FROM reminders")
		r := models.Reminder{UserID: u.ID, ContactID: &c.ID, Message: "Call; her, now\nsoon", RemindAt: at, Recurrence: rec}
		require.NoError(t, db.Create(&r).Error)
		out, err := BuildICSFeed(db, u.ID, "en")
		require.NoError(t, err)
		assert.Contains(t, out, "DTSTART;VALUE=DATE:20300408\r\n", rec)
		assert.Contains(t, out, `SUMMARY:Call\; her\, now\nsoon`+"\r\n", rec)
		assert.Contains(t, out, "DESCRIPTION:Contact: Ada Lovelace\r\n", rec)
		assert.Contains(t, out, "UID:reminder-", rec)
		if want == "" {
			assert.NotContains(t, out, "RRULE", rec)
		} else {
			assert.Contains(t, out, want, rec)
		}
	}
}

func TestBuildICSFeedSkipsCompletedReminders(t *testing.T) {
	db := setupICSTestDB(t)
	u := icsUser(t, db, "a")
	c := models.Contact{UserID: u.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&c).Error)
	require.NoError(t, db.Create(&models.Reminder{UserID: u.ID, ContactID: &c.ID, Message: "done", RemindAt: time.Now(), Recurrence: "once", Completed: true}).Error)
	out, err := BuildICSFeed(db, u.ID, "en")
	require.NoError(t, err)
	assert.NotContains(t, out, "done")
}

func TestBuildICSFeedIsScopedToUser(t *testing.T) {
	db := setupICSTestDB(t)
	a := icsUser(t, db, "a")
	b := icsUser(t, db, "b")
	ca := models.Contact{UserID: a.ID, Firstname: "Alice", Birthday: "1990-01-02"}
	cb := models.Contact{UserID: b.ID, Firstname: "Bertrand", Birthday: "1991-03-04"}
	require.NoError(t, db.Create(&ca).Error)
	require.NoError(t, db.Create(&cb).Error)
	require.NoError(t, db.Create(&models.Reminder{UserID: a.ID, ContactID: &ca.ID, Message: "secret-a", RemindAt: time.Now(), Recurrence: "weekly"}).Error)
	require.NoError(t, db.Create(&models.Reminder{UserID: b.ID, ContactID: &cb.ID, Message: "secret-b", RemindAt: time.Now(), Recurrence: "weekly"}).Error)

	out, err := BuildICSFeed(db, a.ID, "en")
	require.NoError(t, err)
	assert.Contains(t, out, "Alice")
	assert.Contains(t, out, "secret-a")
	assert.NotContains(t, out, "Bertrand")
	assert.NotContains(t, out, "secret-b")
}

func TestBuildICSFeedLocalisedAndFolded(t *testing.T) {
	db := setupICSTestDB(t)
	u := icsUser(t, db, "a")
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: strings.Repeat("Längster", 12), Birthday: "1990-01-02"}).Error)
	out, err := BuildICSFeed(db, u.ID, "de")
	require.NoError(t, err)
	assert.Contains(t, strings.ReplaceAll(out, "\r\n ", ""), "Geburtstag")
	for _, l := range strings.Split(out, "\r\n") {
		assert.LessOrEqual(t, len(l), 75)
	}
}

func TestICSRecurrenceRulesKeepShortMonthsAndLeapDays(t *testing.T) {
	d := func(y, m, day int) time.Time { return time.Date(y, time.Month(m), day, 0, 0, 0, 0, time.UTC) }
	assert.Equal(t, "FREQ=MONTHLY", icsRecurrenceRule("monthly", d(2030, 1, 28)))
	assert.Equal(t, "FREQ=MONTHLY;BYMONTHDAY=28,29;BYSETPOS=-1", icsRecurrenceRule("monthly", d(2030, 1, 29)))
	assert.Equal(t, "FREQ=MONTHLY;BYMONTHDAY=28,29,30;BYSETPOS=-1", icsRecurrenceRule("monthly", d(2030, 1, 30)))
	assert.Equal(t, "FREQ=MONTHLY;BYMONTHDAY=28,29,30,31;BYSETPOS=-1", icsRecurrenceRule("monthly", d(2030, 1, 31)))
	assert.Equal(t, "FREQ=MONTHLY;INTERVAL=3;BYMONTHDAY=28,29,30,31;BYSETPOS=-1", icsRecurrenceRule("quarterly", d(2030, 1, 31)))
	assert.Equal(t, "FREQ=MONTHLY;INTERVAL=6;BYMONTHDAY=28,29,30;BYSETPOS=-1", icsRecurrenceRule("six-months", d(2030, 8, 30)))
	assert.Equal(t, "FREQ=YEARLY", icsRecurrenceRule("yearly", d(2030, 3, 31)))
	assert.Equal(t, "FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=-1", icsRecurrenceRule("yearly", d(2028, 2, 29)))
	assert.Equal(t, "FREQ=WEEKLY", icsRecurrenceRule("weekly", d(2030, 1, 31)))
	assert.Equal(t, "", icsRecurrenceRule("once", d(2030, 1, 31)))
}

func TestBuildICSFeedLeapDayBirthdays(t *testing.T) {
	db := setupICSTestDB(t)
	u := icsUser(t, db, "a")
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Leap", Birthday: "--02-29"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Leapling", Birthday: "1996-02-29"}).Error)
	out, err := BuildICSFeed(db, u.ID, "en")
	require.NoError(t, err)
	assert.Contains(t, out, "DTSTART;VALUE=DATE:19720229\r\n")
	assert.Contains(t, out, "DTSTART;VALUE=DATE:19960229\r\n")
	assert.Equal(t, 2, strings.Count(out, "RRULE:FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=-1\r\n"))
}

func TestBuildICSFeedReminderDateIsUTCDate(t *testing.T) {
	db := setupICSTestDB(t)
	u := icsUser(t, db, "a")
	c := models.Contact{UserID: u.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&c).Error)
	// Midnight at UTC+2 is the previous day in UTC; the stored instant decides the date.
	at := time.Date(2030, 4, 8, 0, 0, 0, 0, time.FixedZone("x", 2*3600))
	require.NoError(t, db.Create(&models.Reminder{UserID: u.ID, ContactID: &c.ID, Message: "m", RemindAt: at, Recurrence: "once"}).Error)
	out, err := BuildICSFeed(db, u.ID, "en")
	require.NoError(t, err)
	assert.Contains(t, out, "DTSTART;VALUE=DATE:20300407\r\n")
}
