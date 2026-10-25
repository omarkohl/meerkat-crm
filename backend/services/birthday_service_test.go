package services

import (
	"testing"
	"time"

	"meerkat/models"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func birthdayDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Relationship{}))
	return db
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }

func TestDaysUntilBirthday(t *testing.T) {
	now := day(2026, time.March, 10)
	cases := map[string]int{
		"1990-03-10": 0,
		"1990-03-11": 1,
		"--03-09":    364,
		"--12-31":    296,
		"":           999,
		"bogus":      999,
		"1990-13-40": 999,
	}
	for in, want := range cases {
		assert.Equal(t, want, DaysUntilBirthday(in, now), in)
	}
	// 29 February in a non-leap year is celebrated on 1 March.
	assert.Equal(t, 1, DaysUntilBirthday("--02-29", day(2026, time.February, 28)))
}

func TestUpcomingBirthdaysScopesAndFilters(t *testing.T) {
	db := birthdayDB(t)
	me, other := models.User{Username: "me", Email: "me@e.x", Password: "x"}, models.User{Username: "other", Email: "o@e.x", Password: "x"}
	require.NoError(t, db.Create(&me).Error)
	require.NoError(t, db.Create(&other).Error)

	mk := func(u uint, first, bday string, archived bool) models.Contact {
		c := models.Contact{UserID: u, Firstname: first, Birthday: bday, Archived: archived}
		require.NoError(t, db.Create(&c).Error)
		return c
	}
	today := mk(me.ID, "Today", "1990-03-10", false)
	mk(me.ID, "Yesterday", "1990-03-09", false) // passed, not until next year
	mk(me.ID, "Archived", "1990-03-12", true)
	mk(me.ID, "NoBirthday", "", false)
	mk(other.ID, "Foreign", "1990-03-11", false)
	next := mk(me.ID, "NextMonth", "--04-01", false)
	deleted := mk(me.ID, "Deleted", "1990-03-15", false)
	require.NoError(t, db.Delete(&deleted).Error)

	got, err := GetUpcomingBirthdays(db, me.ID, day(2026, time.March, 10))
	require.NoError(t, err)
	var names []string
	for _, b := range got {
		names = append(names, b.Name)
	}
	assert.Equal(t, []string{"Today", "NextMonth"}, names)
	assert.Equal(t, today.ID, got[0].ContactID)
	assert.Equal(t, next.ID, got[1].ContactID)
}

func TestUpcomingBirthdaysDecemberLooksIntoJanuary(t *testing.T) {
	db := birthdayDB(t)
	u := models.User{Username: "me", Email: "me@e.x", Password: "x"}
	require.NoError(t, db.Create(&u).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Jan", Birthday: "1980-01-05"}).Error)
	got, err := GetUpcomingBirthdays(db, u.ID, day(2026, time.December, 20))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 16, DaysUntilBirthday(got[0].Birthday, day(2026, time.December, 20)))
}

func TestUpcomingBirthdaysLimit(t *testing.T) {
	db := birthdayDB(t)
	u := models.User{Username: "me", Email: "me@e.x", Password: "x"}
	require.NoError(t, db.Create(&u).Error)
	// Eight birthdays later this month (more than two weeks away) and one within two weeks.
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Soon", Birthday: "1980-03-12"}).Error)
	for i := 0; i < 8; i++ {
		require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Later", Birthday: "1980-03-28"}).Error)
	}
	got, err := GetUpcomingBirthdays(db, u.ID, day(2026, time.March, 10))
	require.NoError(t, err)
	assert.Len(t, got, 5, "at most five unless more fall within two weeks")
	assert.Equal(t, "Soon", got[0].Name)
}

func TestUpcomingBirthdaysSkipDeceasedContactsAndTheirRelationships(t *testing.T) {
	db := birthdayDB(t)
	u := models.User{Username: "me", Email: "me@e.x", Password: "x"}
	require.NoError(t, db.Create(&u).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Alive", Birthday: "1990-03-10"}).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: u.ID, Firstname: "Deceased", Birthday: "1990-03-10", IsDeceased: true}).Error)
	aliveParent := models.Contact{UserID: u.ID, Firstname: "AliveParent"}
	deceasedParent := models.Contact{UserID: u.ID, Firstname: "DeceasedParent", IsDeceased: true}
	require.NoError(t, db.Create(&aliveParent).Error)
	require.NoError(t, db.Create(&deceasedParent).Error)
	require.NoError(t, db.Create(&models.Relationship{UserID: u.ID, ContactID: aliveParent.ID, Name: "ChildOfAlive", Type: "Child", Birthday: "2015-03-11"}).Error)
	require.NoError(t, db.Create(&models.Relationship{UserID: u.ID, ContactID: deceasedParent.ID, Name: "ChildOfDeceased", Type: "Child", Birthday: "2015-03-11"}).Error)

	got, err := GetUpcomingBirthdays(db, u.ID, day(2026, time.March, 10))
	require.NoError(t, err)
	var names []string
	for _, b := range got {
		names = append(names, b.Name)
	}
	assert.Equal(t, []string{"Alive", "ChildOfAlive"}, names)
}

func TestUpcomingBirthdaysIncludesRelationshipsWithoutContact(t *testing.T) {
	db := birthdayDB(t)
	u := models.User{Username: "me", Email: "me@e.x", Password: "x"}
	require.NoError(t, db.Create(&u).Error)
	parent := models.Contact{UserID: u.ID, Firstname: "Pat", Lastname: "Parent"}
	archivedParent := models.Contact{UserID: u.ID, Firstname: "Old", Archived: true}
	require.NoError(t, db.Create(&parent).Error)
	require.NoError(t, db.Create(&archivedParent).Error)
	require.NoError(t, db.Create(&models.Relationship{UserID: u.ID, ContactID: parent.ID, Name: "Kid", Type: "Child", Birthday: "2015-03-11"}).Error)
	require.NoError(t, db.Create(&models.Relationship{UserID: u.ID, ContactID: archivedParent.ID, Name: "Hidden", Type: "Child", Birthday: "2015-03-11"}).Error)
	linked := models.Contact{UserID: u.ID, Firstname: "Linked"}
	require.NoError(t, db.Create(&linked).Error)
	require.NoError(t, db.Create(&models.Relationship{UserID: u.ID, ContactID: parent.ID, RelatedContactID: &linked.ID, Name: "Linked", Type: "Friend", Birthday: "2015-03-11"}).Error)

	got, err := GetUpcomingBirthdays(db, u.ID, day(2026, time.March, 10))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Kid", got[0].Name)
	assert.Equal(t, "relationship", got[0].Type)
	assert.Equal(t, "Pat Parent", got[0].AssociatedContactName)
}
