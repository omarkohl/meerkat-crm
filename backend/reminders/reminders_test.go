package reminders_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/reminders"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var now = time.Date(2026, 3, 10, 15, 30, 0, 0, time.UTC)

type env struct {
	db   *gorm.DB
	a, b models.User
	asA  *gin.Engine
	asB  *gin.Engine
	anon *gin.Engine
}

func newEnv(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Reminder{}, &models.ReminderCompletion{}))
	e := &env{db: db}
	e.a = models.User{Username: "a", Email: "a@example.com", Password: "x"}
	e.b = models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&e.a).Error)
	require.NoError(t, db.Create(&e.b).Error)

	mount := func(userID uint) *gin.Engine {
		r := gin.New()
		r.Use(middleware.ClockMiddleware(clock.NewFake(now)))
		authenticated := r.Group("")
		if userID != 0 {
			authenticated.Use(func(c *gin.Context) { c.Set("userID", userID); c.Next() })
		}
		reminders.New(reminders.Deps{DB: db, Config: &config.Config{}}).Register(api.New(r.Group(""), r), authenticated)
		return r
	}
	e.asA, e.asB, e.anon = mount(e.a.ID), mount(e.b.ID), mount(0)
	return e
}

func (e *env) contact(t *testing.T, owner uint, name string) models.Contact {
	t.Helper()
	c := models.Contact{UserID: owner, Firstname: name}
	require.NoError(t, e.db.Create(&c).Error)
	return c
}

func (e *env) reminder(t *testing.T, owner uint, c models.Contact, message, recurrence string, at time.Time) models.Reminder {
	t.Helper()
	r := models.Reminder{UserID: owner, ContactID: &c.ID, Message: message, Recurrence: recurrence, RemindAt: at}
	require.NoError(t, e.db.Create(&r).Error)
	return r
}

func send(r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m), w.Body.String())
	return m
}

func sid(id uint) string { return strconv.Itoa(int(id)) }

func messages(body map[string]any) []string {
	var out []string
	for _, r := range body["reminders"].([]any) {
		out = append(out, r.(map[string]any)["message"].(string))
	}
	return out
}

func form(c models.Contact, extra map[string]any) map[string]any {
	m := map[string]any{"message": "call", "remind_at": "2026-03-20T15:45:00Z", "recurrence": "weekly", "by_mail": true, "contact_id": c.ID}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestCreateSetsTheOwnerAndTheContactAndDropsTheTimeOfDay(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")

	w := send(e.asA, http.MethodPost, "/contacts/"+sid(c.ID)+"/reminders", form(models.Contact{Model: gorm.Model{ID: 99999}}, map[string]any{"reoccur_from_completion": false}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Reminder created successfully", body["message"])
	r := body["reminder"].(map[string]any)
	assert.EqualValues(t, c.ID, r["contact_id"], "the contact of the URL wins over the body")
	assert.Equal(t, "2026-03-20T00:00:00Z", r["remind_at"])
	assert.Equal(t, true, r["by_mail"])
	assert.Equal(t, false, r["completed"])
	for _, key := range []string{"ID", "CreatedAt", "UpdatedAt", "DeletedAt", "recurrence", "email_sent", "last_sent", "reoccur_from_completion"} {
		assert.Contains(t, r, key)
	}

	var stored models.Reminder
	require.NoError(t, e.db.First(&stored, uint(r["ID"].(float64))).Error)
	assert.Equal(t, e.a.ID, stored.UserID)
}

func TestCreateIgnoresStateFlagsInTheBody(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")

	w := send(e.asA, http.MethodPost, "/contacts/"+sid(c.ID)+"/reminders", form(c, map[string]any{
		"completed": true, "email_sent": true, "last_sent": "2026-01-01T00:00:00Z",
	}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored models.Reminder
	require.NoError(t, e.db.First(&stored).Error)
	assert.False(t, stored.Completed)
	assert.False(t, stored.EmailSent)
	assert.Nil(t, stored.LastSent)
}

func TestCreateRefusesOtherUsersContactsAndBadInput(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	path := "/contacts/" + sid(c.ID) + "/reminders"

	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodPost, path, form(c, nil)).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodPost, "/contacts/9999/reminders", form(c, nil)).Code)
	for name, extra := range map[string]map[string]any{
		"no message":   {"message": ""},
		"long message": {"message": strings.Repeat("x", 501)},
		"bad rhythm":   {"recurrence": "daily"},
		"no date":      {"remind_at": nil},
		"no contact":   {"contact_id": nil},
	} {
		assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPost, path, form(c, extra)).Code, name)
	}
	var n int64
	e.db.Model(&models.Reminder{}).Count(&n)
	assert.Zero(t, n)
}

func TestGetShowsOnlyOwnReminders(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	r := e.reminder(t, e.a.ID, c, "mine", "once", now)

	w := send(e.asA, http.MethodGet, "/reminders/"+sid(r.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "mine", decode(t, w)["message"])

	w = send(e.asB, http.MethodGet, "/reminders/"+sid(r.ID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, sid(r.ID), decode(t, w)["error"].(map[string]any)["details"].(map[string]any)["id"])
	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodGet, "/reminders/abc", nil).Code)
}

func TestListsAreScopedAndOrderedByDueDate(t *testing.T) {
	e := newEnv(t)
	alice, mallory := e.contact(t, e.a.ID, "Alice"), e.contact(t, e.b.ID, "Mallory")
	e.reminder(t, e.a.ID, alice, "later", "once", now.AddDate(0, 0, 9))
	e.reminder(t, e.a.ID, alice, "sooner", "once", now.AddDate(0, 0, 1))
	e.reminder(t, e.b.ID, mallory, "foreign", "once", now)
	other := e.contact(t, e.a.ID, "Bob")
	e.reminder(t, e.a.ID, other, "bobs", "once", now.AddDate(0, 0, 2))

	assert.Equal(t, []string{"sooner", "bobs", "later"}, messages(decode(t, send(e.asA, http.MethodGet, "/reminders", nil))))
	assert.Equal(t, []string{"later", "sooner"}, messages(decode(t, send(e.asA, http.MethodGet, "/contacts/"+sid(alice.ID)+"/reminders", nil))))
	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodGet, "/contacts/"+sid(alice.ID)+"/reminders", nil).Code)
	empty := e.contact(t, e.a.ID, "Empty")
	assert.Equal(t, []any{}, decode(t, send(e.asA, http.MethodGet, "/contacts/"+sid(empty.ID)+"/reminders", nil))["reminders"])
}

func TestUpcomingUsesTheRequestClock(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	done := e.reminder(t, e.a.ID, c, "done", "once", now.AddDate(0, 0, -5))
	require.NoError(t, e.db.Model(&done).Update("completed", true).Error)
	for i, name := range []string{"a", "b", "c"} {
		e.reminder(t, e.a.ID, c, name, "once", now.AddDate(0, 0, i+1))
	}
	e.reminder(t, e.a.ID, c, "far", "once", now.AddDate(0, 3, 0))
	e.reminder(t, e.a.ID, c, "farther", "once", now.AddDate(0, 4, 0))
	e.reminder(t, e.a.ID, c, "farthest", "once", now.AddDate(0, 5, 0))

	got := messages(decode(t, send(e.asA, http.MethodGet, "/reminders/upcoming", nil)))
	assert.Equal(t, []string{"a", "b", "c", "far", "farther"}, got, "five open reminders when fewer than six are due within 7 days")

	for i := 0; i < 3; i++ {
		e.reminder(t, e.a.ID, c, "soon"+strconv.Itoa(i), "once", now.AddDate(0, 0, 2))
	}
	got = messages(decode(t, send(e.asA, http.MethodGet, "/reminders/upcoming", nil)))
	assert.Len(t, got, 6, "more than five due within 7 days: all of them")
	assert.NotContains(t, got, "far")
}

func TestUpdateReplacesTheEditableFields(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.contact(t, e.a.ID, "Alice"), e.contact(t, e.a.ID, "Bob")
	r := e.reminder(t, e.a.ID, alice, "old", "once", now)

	w := send(e.asA, http.MethodPut, "/reminders/"+sid(r.ID), form(bob, map[string]any{"message": "new", "recurrence": "monthly"}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Reminder updated successfully", body["message"])
	assert.Equal(t, "new", body["reminder"].(map[string]any)["message"])

	var stored models.Reminder
	require.NoError(t, e.db.First(&stored, r.ID).Error)
	assert.Equal(t, "new", stored.Message)
	assert.Equal(t, "monthly", stored.Recurrence)
	assert.Equal(t, bob.ID, *stored.ContactID)
	assert.Equal(t, 20, stored.RemindAt.Day())
	assert.Zero(t, stored.RemindAt.Hour())
}

func TestUpdateWithoutFlagsKeepsTheStoredOnes(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	yes, no := true, false
	r := models.Reminder{UserID: e.a.ID, ContactID: &c.ID, Message: "x", Recurrence: "weekly", RemindAt: now, ByMail: &yes, ReoccurFromCompletion: &no}
	require.NoError(t, e.db.Create(&r).Error)

	body := form(c, nil)
	delete(body, "by_mail")
	w := send(e.asA, http.MethodPut, "/reminders/"+sid(r.ID), body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored models.Reminder
	require.NoError(t, e.db.First(&stored, r.ID).Error)
	assert.True(t, *stored.ByMail)
	assert.False(t, *stored.ReoccurFromCompletion)
}

func TestUpdateRefusesOtherUsersRemindersContactsAndBadInput(t *testing.T) {
	e := newEnv(t)
	alice := e.contact(t, e.a.ID, "Alice")
	foreign := e.contact(t, e.b.ID, "Mallory")
	r := e.reminder(t, e.a.ID, alice, "mine", "once", now)
	path := "/reminders/" + sid(r.ID)

	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodPut, path, form(foreign, map[string]any{"message": "hacked"})).Code)
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodPut, path, form(foreign, nil)).Code)
	assert.Equal(t, http.StatusBadRequest, send(e.asA, http.MethodPut, path, form(alice, map[string]any{"message": ""})).Code)
	var stored models.Reminder
	require.NoError(t, e.db.First(&stored, r.ID).Error)
	assert.Equal(t, "mine", stored.Message)
	assert.Equal(t, alice.ID, *stored.ContactID)
}

// @006-US1
func TestCompletingARecurringReminderRecordsItAndSchedulesTheNext(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	r := e.reminder(t, e.a.ID, c, "call", "weekly", now.AddDate(0, 0, -2))

	w := send(e.asA, http.MethodPost, "/reminders/"+sid(r.ID)+"/complete", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := decode(t, w)
	assert.Equal(t, "Reminder completed successfully", body["message"])
	assert.Equal(t, "2026-03-17T00:00:00Z", body["reminder"].(map[string]any)["remind_at"], "a week from the fake 'today'")
	assert.Equal(t, false, body["reminder"].(map[string]any)["completed"])

	var done models.ReminderCompletion
	require.NoError(t, e.db.First(&done).Error)
	assert.Equal(t, e.a.ID, done.UserID)
	assert.Equal(t, c.ID, done.ContactID)
	assert.Equal(t, "call", done.Message)
	assert.True(t, done.CompletedAt.Equal(now), "stamped with the request clock")
	var stored models.Reminder
	require.NoError(t, e.db.First(&stored, r.ID).Error)
	assert.True(t, stored.LastSent.Equal(now))
}

func TestCompletingAOnceReminderDeletesIt(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	r := e.reminder(t, e.a.ID, c, "call", "once", now)

	w := send(e.asA, http.MethodPost, "/reminders/"+sid(r.ID)+"/complete", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Reminder completed and deleted"}`, w.Body.String())
	var live, records int64
	e.db.Model(&models.Reminder{}).Where("id = ?", r.ID).Count(&live)
	e.db.Model(&models.ReminderCompletion{}).Count(&records)
	assert.Zero(t, live)
	assert.EqualValues(t, 1, records)
}

func TestSkippingDoesNotRecordACompletion(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	r := e.reminder(t, e.a.ID, c, "call", "monthly", now.AddDate(0, 0, -2))

	w := send(e.asA, http.MethodPost, "/reminders/"+sid(r.ID)+"/complete?skip=true", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "Reminder skipped successfully", decode(t, w)["message"])
	var records int64
	e.db.Model(&models.ReminderCompletion{}).Count(&records)
	assert.Zero(t, records)

	once := e.reminder(t, e.a.ID, c, "once", "once", now)
	w = send(e.asA, http.MethodPost, "/reminders/"+sid(once.ID)+"/complete?skip=true", nil)
	assert.Equal(t, "Reminder skipped and deleted", decode(t, w)["message"])
}

func TestCompleteRefusesOtherUsers(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	r := e.reminder(t, e.a.ID, c, "call", "weekly", now)
	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodPost, "/reminders/"+sid(r.ID)+"/complete", nil).Code)
	var records int64
	e.db.Model(&models.ReminderCompletion{}).Count(&records)
	assert.Zero(t, records)
}

func TestDeleteRemovesOwnReminder(t *testing.T) {
	e := newEnv(t)
	c := e.contact(t, e.a.ID, "Alice")
	r := e.reminder(t, e.a.ID, c, "call", "once", now)

	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodDelete, "/reminders/"+sid(r.ID), nil).Code)
	w := send(e.asA, http.MethodDelete, "/reminders/"+sid(r.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"message":"Reminder deleted"}`, w.Body.String())
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodDelete, "/reminders/"+sid(r.ID), nil).Code)
}

func TestCompletionsAreListedNewestFirstAndDeletedByTheirOwner(t *testing.T) {
	e := newEnv(t)
	alice := e.contact(t, e.a.ID, "Alice")
	older := models.ReminderCompletion{UserID: e.a.ID, ContactID: alice.ID, Message: "older", CompletedAt: now.AddDate(0, 0, -2)}
	newer := models.ReminderCompletion{UserID: e.a.ID, ContactID: alice.ID, Message: "newer", CompletedAt: now}
	foreign := models.ReminderCompletion{UserID: e.b.ID, ContactID: alice.ID, Message: "foreign", CompletedAt: now}
	require.NoError(t, e.db.Create(&[]*models.ReminderCompletion{&older, &newer, &foreign}).Error)

	w := send(e.asA, http.MethodGet, "/contacts/"+sid(alice.ID)+"/reminder-completions", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got []string
	for _, c := range decode(t, w)["completions"].([]any) {
		got = append(got, c.(map[string]any)["message"].(string))
	}
	assert.Equal(t, []string{"newer", "older"}, got)
	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodGet, "/contacts/"+sid(alice.ID)+"/reminder-completions", nil).Code)

	path := "/reminder-completions/" + sid(newer.ID)
	assert.Equal(t, http.StatusNotFound, send(e.asB, http.MethodDelete, path, nil).Code)
	w = send(e.asA, http.MethodDelete, path, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"message":"Reminder completion deleted"}`, w.Body.String())
	assert.Equal(t, http.StatusNotFound, send(e.asA, http.MethodDelete, path, nil).Code)
}

func TestWithoutAnAuthenticatedUserEverythingRefuses(t *testing.T) {
	e := newEnv(t)
	body := form(models.Contact{Model: gorm.Model{ID: 1}}, nil)
	for _, call := range []struct{ method, path string }{
		{http.MethodGet, "/reminders"}, {http.MethodGet, "/reminders/upcoming"}, {http.MethodGet, "/reminders/1"},
		{http.MethodPut, "/reminders/1"}, {http.MethodDelete, "/reminders/1"}, {http.MethodPost, "/reminders/1/complete"},
		{http.MethodGet, "/contacts/1/reminders"}, {http.MethodPost, "/contacts/1/reminders"},
		{http.MethodGet, "/contacts/1/reminder-completions"}, {http.MethodDelete, "/reminder-completions/1"},
	} {
		assert.Equal(t, http.StatusUnauthorized, send(e.anon, call.method, call.path, body).Code, call.method+" "+call.path)
	}
}
