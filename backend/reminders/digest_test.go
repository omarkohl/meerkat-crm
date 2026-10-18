package reminders_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"meerkat/clock"
	"meerkat/config"
	"meerkat/i18n"
	"meerkat/models"
	"meerkat/reminders"
	"meerkat/services"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type digestEnv struct {
	db     *gorm.DB
	a, b   models.User
	cfg    *config.Config
	clk    clock.Clock
	digest func() error
}

func newDigestEnv(t *testing.T, emailEnabled bool) *digestEnv {
	t.Helper()
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Relationship{}, &models.Reminder{},
		&models.EmailOutbox{}, &models.Webhook{}, &models.WebhookDelivery{}))
	t.Cleanup(services.StubEmailDelivery(func(func()) {}))
	e := &digestEnv{db: db, clk: clock.NewFake(now)}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x", Language: "de", DateFormat: "iso"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)
	e.cfg = &config.Config{ReminderTimezone: "UTC"}
	if emailEnabled {
		e.cfg.UseResend, e.cfg.ResendAPIKey, e.cfg.ResendFromEmail = true, "key", "noreply@example.com"
	}
	e.digest = func() error { return reminders.New(reminders.Deps{DB: db, Config: e.cfg}).SendDailyDigest(e.clk) }
	return e
}

func (e *digestEnv) byMail(t *testing.T, owner uint, contact models.Contact, message string, at time.Time) models.Reminder {
	t.Helper()
	yes := true
	r := models.Reminder{UserID: owner, ContactID: &contact.ID, Message: message, Recurrence: "once", ByMail: &yes, RemindAt: at}
	require.NoError(t, e.db.Create(&r).Error)
	return r
}

func (e *digestEnv) mails(t *testing.T) []models.EmailOutbox {
	t.Helper()
	var out []models.EmailOutbox
	require.NoError(t, e.db.Order("id").Find(&out).Error)
	return out
}

func (e *digestEnv) contact(t *testing.T, owner uint, first, last, birthday string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: owner, Firstname: first, Lastname: last, Birthday: birthday}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

// @006-US2
func TestDigestMailsDueRemindersToTheirOwnerAndMarksThemSent(t *testing.T) {
	e := newDigestEnv(t, true)
	jane := e.contact(t, e.a.ID, "Jane", "Doe", "")
	due := e.byMail(t, e.a.ID, jane, "Call about the move", now.Add(-time.Hour))
	other := e.byMail(t, e.b.ID, e.contact(t, e.b.ID, "Bo", "", ""), "Other user's reminder", now.Add(-time.Hour))

	require.NoError(t, e.digest())

	mails := e.mails(t)
	require.Len(t, mails, 2)
	assert.Equal(t, "a@example.com", mails[0].ToAddr)
	assert.Equal(t, i18n.T("de", "email.reminder.subject"), mails[0].Subject)
	assert.Contains(t, mails[0].HTML, "Call about the move")
	assert.Contains(t, mails[0].HTML, "Jane Doe")
	assert.Contains(t, mails[0].HTML, "2026-03-10")
	assert.NotContains(t, mails[0].HTML, "Other user's reminder")
	assert.Equal(t, "b@example.com", mails[1].ToAddr)
	assert.NotContains(t, mails[1].HTML, "Call about the move")
	require.NotNil(t, mails[0].UserID)
	assert.Equal(t, e.a.ID, *mails[0].UserID)

	for _, id := range []uint{due.ID, other.ID} {
		var r models.Reminder
		require.NoError(t, e.db.First(&r, id).Error)
		assert.True(t, r.EmailSent)
		require.NotNil(t, r.LastSent)
		assert.True(t, now.Equal(*r.LastSent))
	}
}

func TestDigestSkipsRemindersThatAreNotDueOrNotForMail(t *testing.T) {
	e := newDigestEnv(t, true)
	c := e.contact(t, e.a.ID, "Jane", "", "")
	e.byMail(t, e.a.ID, c, "tomorrow", now.Add(24*time.Hour))
	done := e.byMail(t, e.a.ID, c, "completed", now.Add(-time.Hour))
	require.NoError(t, e.db.Model(&done).Update("completed", true).Error)
	sent := e.byMail(t, e.a.ID, c, "already mailed", now.Add(-time.Hour))
	require.NoError(t, e.db.Model(&sent).Update("email_sent", true).Error)
	no := false
	require.NoError(t, e.db.Create(&models.Reminder{UserID: e.a.ID, ContactID: &c.ID, Message: "not by mail", Recurrence: "once", ByMail: &no, RemindAt: now.Add(-time.Hour)}).Error)

	require.NoError(t, e.digest())
	assert.Empty(t, e.mails(t))
}

func TestDigestIncludesRemindersDueLaterTodayInTheReminderTimezone(t *testing.T) {
	e := newDigestEnv(t, true)
	e.cfg.ReminderTimezone = "Pacific/Auckland"
	c := e.contact(t, e.a.ID, "Jane", "", "")
	e.byMail(t, e.a.ID, c, "tonight in Auckland", now.Add(10*time.Hour))
	require.NoError(t, e.digest())
	assert.Len(t, e.mails(t), 1)
}

func TestDigestSendsABirthdayOnlyMailOnTheDay(t *testing.T) {
	e := newDigestEnv(t, true)
	e.contact(t, e.a.ID, "Ada", "Lovelace", "1815-03-10")
	e.contact(t, e.b.ID, "Grace", "Hopper", "1906-04-09")

	require.NoError(t, e.digest())
	mails := e.mails(t)
	require.Len(t, mails, 1)
	assert.Equal(t, "a@example.com", mails[0].ToAddr)
	assert.Contains(t, mails[0].HTML, "Ada Lovelace")
	assert.Contains(t, mails[0].HTML, i18n.T("de", "email.reminder.today"))
}

func TestDigestWithoutAnEmailChannelKeepsRemindersForLater(t *testing.T) {
	e := newDigestEnv(t, false)
	r := e.byMail(t, e.a.ID, e.contact(t, e.a.ID, "Jane", "", ""), "later", now.Add(-time.Hour))

	require.NoError(t, e.digest())
	assert.Empty(t, e.mails(t))
	var got models.Reminder
	require.NoError(t, e.db.First(&got, r.ID).Error)
	assert.False(t, got.EmailSent)
	assert.Nil(t, got.LastSent)
}

func TestDigestMarksRemindersOfAUserWithoutAnAddressWithoutMailing(t *testing.T) {
	e := newDigestEnv(t, true)
	require.NoError(t, e.db.Model(&e.a).Update("email", "").Error)
	r := e.byMail(t, e.a.ID, e.contact(t, e.a.ID, "Jane", "", ""), "x", now.Add(-time.Hour))

	require.NoError(t, e.digest())
	assert.Empty(t, e.mails(t))
	var got models.Reminder
	require.NoError(t, e.db.First(&got, r.ID).Error)
	assert.True(t, got.EmailSent)
}

func TestDigestFiresWebhooksForRemindersAndBirthdaysEvenWithoutAnEmailChannel(t *testing.T) {
	received := make(chan string, 4)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("X-Meerkat-Event")
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	e := newDigestEnv(t, false)
	require.NoError(t, e.db.Create(&models.Webhook{UserID: e.a.ID, Name: "h", URL: target.URL, Secret: "s", IsActive: true,
		Events: []string{"reminder.triggered", "birthday.occurred"}}).Error)
	e.byMail(t, e.a.ID, e.contact(t, e.a.ID, "Ada", "L", "1815-03-10"), "ping", now.Add(-time.Hour))

	require.NoError(t, e.digest())

	got := map[string]bool{}
	require.Eventually(t, func() bool {
		for {
			select {
			case ev := <-received:
				got[ev] = true
			default:
				return got["reminder.triggered"] && got["birthday.occurred"]
			}
		}
	}, 5*time.Second, 20*time.Millisecond)
}

func TestDigestLeavesAUserWhoseMailFailedUnmarkedAndStillMailsTheOthers(t *testing.T) {
	e := newDigestEnv(t, true)
	require.NoError(t, e.db.Model(&e.a).Update("email", "a@example.com\r\nBcc: x@example.com").Error)
	failed := e.byMail(t, e.a.ID, e.contact(t, e.a.ID, "Jane", "", ""), "a's", now.Add(-time.Hour))
	ok := e.byMail(t, e.b.ID, e.contact(t, e.b.ID, "Bo", "", ""), "b's", now.Add(-time.Hour))

	require.NoError(t, e.digest())

	mails := e.mails(t)
	require.Len(t, mails, 1)
	assert.Equal(t, "b@example.com", mails[0].ToAddr)
	var gotFailed, gotOK models.Reminder
	require.NoError(t, e.db.First(&gotFailed, failed.ID).Error)
	require.NoError(t, e.db.First(&gotOK, ok.ID).Error)
	assert.False(t, gotFailed.EmailSent)
	assert.True(t, gotOK.EmailSent)
}

func TestDigestDoesNotMailTheSameReminderTwice(t *testing.T) {
	e := newDigestEnv(t, true)
	e.byMail(t, e.a.ID, e.contact(t, e.a.ID, "Jane", "", ""), "once", now.Add(-time.Hour))
	require.NoError(t, e.digest())
	require.NoError(t, e.digest())
	assert.Len(t, e.mails(t), 1)
}
