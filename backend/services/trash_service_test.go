package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"meerkat/i18n"
	"meerkat/models"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTrashTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(1)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Activity{}, &models.Note{},
		&models.Relationship{}, &models.Reminder{}, &models.ReminderCompletion{}, &models.CardDAVConnection{},
		&models.CardDAVContactLink{}, &models.Image{}))
	return db
}

func trashUser(t *testing.T, db *gorm.DB, name string) models.User {
	t.Helper()
	u := models.User{Username: name, Password: "password123!A", Email: name + "@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

// contactWithDependants creates a contact with a note, a reminder, a relationship and an
// activity link, then deletes it the way DeleteContact does (dependants soft-deleted
// just before the contact).
type trashFixture struct {
	contact  models.Contact
	note     models.Note
	reminder models.Reminder
	rel      models.Relationship
	activity models.Activity
}

func newTrashFixture(t *testing.T, db *gorm.DB, userID uint, name string) trashFixture {
	t.Helper()
	f := trashFixture{}
	f.contact = models.Contact{UserID: userID, Firstname: name}
	require.NoError(t, db.Create(&f.contact).Error)
	cid := f.contact.ID
	f.note = models.Note{UserID: userID, Content: "n", Date: time.Now(), ContactID: &cid}
	require.NoError(t, db.Create(&f.note).Error)
	f.reminder = models.Reminder{UserID: userID, Message: "r", RemindAt: time.Now(), Recurrence: "once", ContactID: &cid}
	require.NoError(t, db.Create(&f.reminder).Error)
	f.rel = models.Relationship{UserID: userID, Name: "Kid", Type: "Child", ContactID: cid}
	require.NoError(t, db.Create(&f.rel).Error)
	f.activity = models.Activity{UserID: userID, Title: "Lunch", Date: time.Now()}
	require.NoError(t, db.Create(&f.activity).Error)
	require.NoError(t, db.Model(&f.activity).Association("Contacts").Append(&f.contact))
	require.NoError(t, db.Create(&models.ReminderCompletion{UserID: userID, ContactID: cid, Message: "done", CompletedAt: time.Now()}).Error)
	return f
}

func softDeleteLikeController(t *testing.T, db *gorm.DB, f trashFixture) {
	t.Helper()
	require.NoError(t, db.Delete(&f.note).Error)
	require.NoError(t, db.Delete(&f.reminder).Error)
	require.NoError(t, db.Delete(&f.rel).Error)
	require.NoError(t, db.Delete(&f.contact).Error)
}

func count(t *testing.T, db *gorm.DB, model any, where string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Unscoped().Model(model).Where(where, args...).Count(&n).Error)
	return n
}

func TestListTrashedContactsIsScopedToUser(t *testing.T) {
	db := setupTrashTestDB(t)
	a, b := trashUser(t, db, "a"), trashUser(t, db, "b")
	fa := newTrashFixture(t, db, a.ID, "Alice")
	fb := newTrashFixture(t, db, b.ID, "Bob")
	live := models.Contact{UserID: a.ID, Firstname: "Live"}
	require.NoError(t, db.Create(&live).Error)
	softDeleteLikeController(t, db, fa)
	softDeleteLikeController(t, db, fb)

	got, err := ListTrashedContacts(db, a.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, fa.contact.ID, got[0].ID)
	assert.True(t, got[0].DeletedAt.Valid)
}

func TestListTrashedContactsNewestFirst(t *testing.T) {
	db := setupTrashTestDB(t)
	a := trashUser(t, db, "a")
	c1 := models.Contact{UserID: a.ID, Firstname: "First"}
	c2 := models.Contact{UserID: a.ID, Firstname: "Second"}
	require.NoError(t, db.Create(&c1).Error)
	require.NoError(t, db.Create(&c2).Error)
	require.NoError(t, db.Delete(&c1).Error)
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, db.Delete(&c2).Error)

	got, err := ListTrashedContacts(db, a.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "Second", got[0].Firstname)
}

func TestRestoreBringsBackContactAndItsDependants(t *testing.T) {
	db := setupTrashTestDB(t)
	a := trashUser(t, db, "a")
	f := newTrashFixture(t, db, a.ID, "Alice")
	// A note deleted long before the contact must stay deleted.
	old := models.Note{UserID: a.ID, Content: "old", Date: time.Now(), ContactID: &f.contact.ID}
	require.NoError(t, db.Create(&old).Error)
	require.NoError(t, db.Delete(&old).Error)
	require.NoError(t, db.Unscoped().Model(&old).UpdateColumn("deleted_at", time.Now().Add(-48*time.Hour)).Error)
	softDeleteLikeController(t, db, f)

	restored, err := RestoreTrashedContact(db, a.ID, f.contact.ID)
	require.NoError(t, err)
	assert.Equal(t, f.contact.ID, restored.ID)

	var c models.Contact
	require.NoError(t, db.First(&c, f.contact.ID).Error)
	var notes []models.Note
	require.NoError(t, db.Where("contact_id = ?", f.contact.ID).Find(&notes).Error)
	require.Len(t, notes, 1)
	assert.Equal(t, "n", notes[0].Content)
	assert.EqualValues(t, 1, count(t, db, &models.Reminder{}, "contact_id = ? AND deleted_at IS NULL", f.contact.ID))
	assert.EqualValues(t, 1, count(t, db, &models.Relationship{}, "contact_id = ? AND deleted_at IS NULL", f.contact.ID))
	assert.EqualValues(t, 1, count(t, db, &models.Note{}, "id = ? AND deleted_at IS NOT NULL", old.ID))

	var act models.Activity
	require.NoError(t, db.Preload("Contacts").First(&act, f.activity.ID).Error)
	assert.Len(t, act.Contacts, 1)
}

func TestRestoreRejectsOtherUsersAndLiveContacts(t *testing.T) {
	db := setupTrashTestDB(t)
	a, b := trashUser(t, db, "a"), trashUser(t, db, "b")
	f := newTrashFixture(t, db, a.ID, "Alice")
	softDeleteLikeController(t, db, f)
	live := models.Contact{UserID: b.ID, Firstname: "Live"}
	require.NoError(t, db.Create(&live).Error)

	_, err := RestoreTrashedContact(db, b.ID, f.contact.ID)
	assert.ErrorIs(t, err, ErrTrashedContactNotFound)
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "id = ? AND deleted_at IS NOT NULL", f.contact.ID))

	_, err = RestoreTrashedContact(db, b.ID, live.ID)
	assert.ErrorIs(t, err, ErrTrashedContactNotFound)
	_, err = RestoreTrashedContact(db, b.ID, 9999)
	assert.ErrorIs(t, err, ErrTrashedContactNotFound)
}

func TestPurgeRemovesContactDependantsAndPhotos(t *testing.T) {
	db := setupTrashTestDB(t)
	dir := t.TempDir()
	a := trashUser(t, db, "a")
	f := newTrashFixture(t, db, a.ID, "Alice")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.jpg"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "t.jpg"), []byte("x"), 0o600))
	require.NoError(t, db.Model(&f.contact).Updates(map[string]any{"photo": "p.jpg", "photo_thumbnail_file": "t.jpg"}).Error)
	// Another contact's relationship that points at the purged contact.
	other := models.Contact{UserID: a.ID, Firstname: "Other"}
	require.NoError(t, db.Create(&other).Error)
	link := models.Relationship{UserID: a.ID, Name: "Alice", Type: "Friend", ContactID: other.ID, RelatedContactID: &f.contact.ID}
	require.NoError(t, db.Create(&link).Error)
	softDeleteLikeController(t, db, f)

	n, err := PurgeTrashedContacts(db, a.ID, []uint{f.contact.ID}, dir, time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	cid := f.contact.ID
	assert.Zero(t, count(t, db, &models.Contact{}, "id = ?", cid))
	assert.Zero(t, count(t, db, &models.Note{}, "contact_id = ?", cid))
	assert.Zero(t, count(t, db, &models.Reminder{}, "contact_id = ?", cid))
	assert.Zero(t, count(t, db, &models.Relationship{}, "contact_id = ?", cid))
	assert.Zero(t, count(t, db, &models.ReminderCompletion{}, "contact_id = ?", cid))
	var joins int64
	require.NoError(t, db.Table("activity_contacts").Where("contact_id = ?", cid).Count(&joins).Error)
	assert.Zero(t, joins)
	// The activity itself and the other contact survive; the dangling link is cleared.
	assert.EqualValues(t, 1, count(t, db, &models.Activity{}, "id = ?", f.activity.ID))
	var rel models.Relationship
	require.NoError(t, db.First(&rel, link.ID).Error)
	assert.Nil(t, rel.RelatedContactID)

	_, err = os.Stat(filepath.Join(dir, "p.jpg"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, "t.jpg"))
	assert.True(t, os.IsNotExist(err))
}

func TestPurgeKeepsCardDAVLinks(t *testing.T) {
	db := setupTrashTestDB(t)
	a := trashUser(t, db, "a")
	f := newTrashFixture(t, db, a.ID, "Alice")
	conn := models.CardDAVConnection{UserID: a.ID, BaseURL: "https://x", AddressBookPath: "/ab"}
	require.NoError(t, db.Create(&conn).Error)
	require.NoError(t, db.Create(&models.CardDAVContactLink{ConnectionID: conn.ID, UserID: a.ID, ContactID: f.contact.ID, RemoteUID: "u", RemotePath: "/ab/u.vcf"}).Error)
	softDeleteLikeController(t, db, f)

	_, err := PurgeTrashedContacts(db, a.ID, nil, "", time.Now())
	require.NoError(t, err)
	// The link is the only record that the remote object still has to be deleted.
	assert.EqualValues(t, 1, count(t, db, &models.CardDAVContactLink{}, "contact_id = ?", f.contact.ID))
	// ... and it is marked as a tombstone so the sync can tell a purge from a vanished row.
	assert.EqualValues(t, 1, count(t, db, &models.CardDAVContactLink{}, "contact_id = ? AND purged_at IS NOT NULL", f.contact.ID))
}

func TestPurgeIsScopedToUserAndTrash(t *testing.T) {
	db := setupTrashTestDB(t)
	a, b := trashUser(t, db, "a"), trashUser(t, db, "b")
	fa := newTrashFixture(t, db, a.ID, "Alice")
	softDeleteLikeController(t, db, fa)
	live := models.Contact{UserID: b.ID, Firstname: "Live"}
	require.NoError(t, db.Create(&live).Error)

	n, err := PurgeTrashedContacts(db, b.ID, []uint{fa.contact.ID, live.ID}, "", time.Now())
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "id = ?", fa.contact.ID))
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "id = ?", live.ID))

	// Emptying B's trash does not touch A's.
	n, err = PurgeTrashedContacts(db, b.ID, nil, "", time.Now())
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "id = ?", fa.contact.ID))
}

func TestPurgeAllEmptiesOnlyThatUsersTrash(t *testing.T) {
	db := setupTrashTestDB(t)
	a, b := trashUser(t, db, "a"), trashUser(t, db, "b")
	f1 := newTrashFixture(t, db, a.ID, "One")
	f2 := newTrashFixture(t, db, a.ID, "Two")
	fb := newTrashFixture(t, db, b.ID, "Bee")
	live := models.Contact{UserID: a.ID, Firstname: "Live"}
	require.NoError(t, db.Create(&live).Error)
	softDeleteLikeController(t, db, f1)
	softDeleteLikeController(t, db, f2)
	softDeleteLikeController(t, db, fb)

	n, err := PurgeTrashedContacts(db, a.ID, nil, "", time.Now())
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "user_id = ?", b.ID))
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "id = ?", live.ID))
}

func TestPurgeRollsBackOnFailure(t *testing.T) {
	db := setupTrashTestDB(t)
	dir := t.TempDir()
	a := trashUser(t, db, "a")
	f := newTrashFixture(t, db, a.ID, "Alice")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.jpg"), []byte("x"), 0o600))
	require.NoError(t, db.Model(&f.contact).Update("photo", "p.jpg").Error)
	softDeleteLikeController(t, db, f)
	// Make the last step of the transaction fail.
	require.NoError(t, db.Exec(`CREATE TRIGGER block_contact_delete BEFORE DELETE ON contacts
		BEGIN SELECT RAISE(ABORT, 'blocked'); END`).Error)

	_, err := PurgeTrashedContacts(db, a.ID, nil, dir, time.Now())
	require.Error(t, err)
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "id = ?", f.contact.ID))
	assert.EqualValues(t, 1, count(t, db, &models.Note{}, "contact_id = ?", f.contact.ID))
	var joins int64
	require.NoError(t, db.Table("activity_contacts").Where("contact_id = ?", f.contact.ID).Count(&joins).Error)
	assert.EqualValues(t, 1, joins)
	_, statErr := os.Stat(filepath.Join(dir, "p.jpg"))
	assert.NoError(t, statErr, "photo must survive a failed purge")
}

func TestPurgeExpiredTrashUsesCutoffAcrossUsers(t *testing.T) {
	db := setupTrashTestDB(t)
	a, b := trashUser(t, db, "a"), trashUser(t, db, "b")
	oldA := newTrashFixture(t, db, a.ID, "OldA")
	oldB := newTrashFixture(t, db, b.ID, "OldB")
	recent := newTrashFixture(t, db, a.ID, "Recent")
	live := models.Contact{UserID: a.ID, Firstname: "Live"}
	require.NoError(t, db.Create(&live).Error)
	for _, f := range []trashFixture{oldA, oldB, recent} {
		softDeleteLikeController(t, db, f)
	}
	now := time.Now()
	setDeleted := func(id uint, at time.Time) {
		require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ?", id).UpdateColumn("deleted_at", at).Error)
	}
	setDeleted(oldA.contact.ID, now.AddDate(0, 0, -31))
	setDeleted(oldB.contact.ID, now.AddDate(0, 0, -45))
	setDeleted(recent.contact.ID, now.AddDate(0, 0, -29))

	n, err := PurgeExpiredTrash(db, 30, "", now)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Zero(t, count(t, db, &models.Contact{}, "id IN ?", []uint{oldA.contact.ID, oldB.contact.ID}))
	assert.Zero(t, count(t, db, &models.Note{}, "contact_id IN ?", []uint{oldA.contact.ID, oldB.contact.ID}))
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "id = ?", recent.contact.ID))
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "id = ?", live.ID))
}

func TestPurgeExpiredTrashIgnoresInvalidRetention(t *testing.T) {
	db := setupTrashTestDB(t)
	a := trashUser(t, db, "a")
	f := newTrashFixture(t, db, a.ID, "Alice")
	softDeleteLikeController(t, db, f)
	n, err := PurgeExpiredTrash(db, 0, "", time.Now().AddDate(1, 0, 0))
	require.NoError(t, err)
	assert.Zero(t, n, "a retention below one day must never purge")
}

func TestTrashedContactsStayOutOfBirthdaysAndICSFeed(t *testing.T) {
	db := setupTrashTestDB(t)
	require.NoError(t, i18n.Init())
	a := trashUser(t, db, "a")
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	gone := models.Contact{UserID: a.ID, Firstname: "Gone", Birthday: "1990-05-20"}
	kept := models.Contact{UserID: a.ID, Firstname: "Kept", Birthday: "1990-05-21"}
	require.NoError(t, db.Create(&gone).Error)
	require.NoError(t, db.Create(&kept).Error)
	// A relationship birthday of a trashed contact (soft-deleted with it).
	rel := models.Relationship{UserID: a.ID, Name: "Kid", Type: "Child", ContactID: gone.ID, Birthday: "2015-05-22"}
	require.NoError(t, db.Create(&rel).Error)
	require.NoError(t, db.Delete(&rel).Error)
	require.NoError(t, db.Delete(&gone).Error)

	birthdays, err := GetUpcomingBirthdays(db, a.ID, now)
	require.NoError(t, err)
	require.Len(t, birthdays, 1)
	assert.Equal(t, "Kept", birthdays[0].Name)

	ics, err := BuildICSFeed(db, a.ID, "en")
	require.NoError(t, err)
	assert.Contains(t, ics, "Kept")
	assert.NotContains(t, ics, "Gone")
}

// A restore that commits between listing the trash and purging it must win:
// the live contact keeps its dependants and photo files.
func TestPurgeSkipsContactRestoredAfterListing(t *testing.T) {
	db := setupTrashTestDB(t)
	dir := t.TempDir()
	a := trashUser(t, db, "a")
	f := newTrashFixture(t, db, a.ID, "Alice")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.jpg"), []byte("x"), 0o600))
	require.NoError(t, db.Model(&f.contact).Update("photo", "p.jpg").Error)
	softDeleteLikeController(t, db, f)

	stale, err := ListTrashedContacts(db, a.ID)
	require.NoError(t, err)
	require.Len(t, stale, 1)
	_, err = RestoreTrashedContact(db, a.ID, f.contact.ID)
	require.NoError(t, err)

	n, err := purgeContacts(db, stale, dir, time.Now())
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "id = ? AND deleted_at IS NULL", f.contact.ID))
	assert.EqualValues(t, 1, count(t, db, &models.Note{}, "contact_id = ? AND deleted_at IS NULL", f.contact.ID))
	assert.EqualValues(t, 1, count(t, db, &models.Reminder{}, "contact_id = ? AND deleted_at IS NULL", f.contact.ID))
	assert.EqualValues(t, 1, count(t, db, &models.Relationship{}, "contact_id = ? AND deleted_at IS NULL", f.contact.ID))
	var joins int64
	require.NoError(t, db.Table("activity_contacts").Where("contact_id = ?", f.contact.ID).Count(&joins).Error)
	assert.EqualValues(t, 1, joins)
	_, statErr := os.Stat(filepath.Join(dir, "p.jpg"))
	assert.NoError(t, statErr)
}

// Restored and deleted again after the listing: the new deletion is a fresh
// one, so a purge decided on the old listing must not take it.
func TestPurgeSkipsContactDeletedAgainAfterListing(t *testing.T) {
	db := setupTrashTestDB(t)
	a := trashUser(t, db, "a")
	f := newTrashFixture(t, db, a.ID, "Alice")
	softDeleteLikeController(t, db, f)
	stale, err := ListTrashedContacts(db, a.ID)
	require.NoError(t, err)
	_, err = RestoreTrashedContact(db, a.ID, f.contact.ID)
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	require.NoError(t, models.TrashContact(db, a.ID, f.contact.ID))

	n, err := purgeContacts(db, stale, "", time.Now())
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "id = ?", f.contact.ID))
}

func TestTrashContactSoftDeletesDependantsAndKeepsActivityLinks(t *testing.T) {
	db := setupTrashTestDB(t)
	a, b := trashUser(t, db, "a"), trashUser(t, db, "b")
	f := newTrashFixture(t, db, a.ID, "Alice")
	other := newTrashFixture(t, db, b.ID, "Bob")

	require.NoError(t, models.TrashContact(db, a.ID, f.contact.ID))
	require.NoError(t, models.TrashContact(db, a.ID, other.contact.ID)) // another user's: no-op

	cid := f.contact.ID
	for _, m := range []any{&models.Contact{}, &models.Note{}, &models.Reminder{}, &models.Relationship{}} {
		col := "contact_id = ?"
		if _, ok := m.(*models.Contact); ok {
			col = "id = ?"
		}
		assert.EqualValues(t, 1, count(t, db, m, col+" AND deleted_at IS NOT NULL", cid))
	}
	assert.EqualValues(t, 1, count(t, db, &models.Contact{}, "id = ? AND deleted_at IS NULL", other.contact.ID))
	assert.EqualValues(t, 1, count(t, db, &models.Reminder{}, "contact_id = ? AND deleted_at IS NULL", other.contact.ID))

	_, err := RestoreTrashedContact(db, a.ID, cid)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count(t, db, &models.Reminder{}, "contact_id = ? AND deleted_at IS NULL", cid))
}

func TestPurgeRemovesTheImagesOfTheContactAndOfItsNotesButKeepsOthers(t *testing.T) {
	db := setupTrashTestDB(t)
	dir := t.TempDir()
	a := trashUser(t, db, "a")
	f := newTrashFixture(t, db, a.ID, "Alice")
	keeper := newTrashFixture(t, db, a.ID, "Keeper")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "images", "u1"), 0o750))
	mk := func(name string, update map[string]any) models.Image {
		img := models.Image{UserID: a.ID, File: "images/u1/" + name + "_image.webp", ThumbFile: "images/u1/" + name + "_thumb.webp", Mime: "image/webp", Width: 1, Height: 1, Bytes: 1}
		require.NoError(t, db.Create(&img).Error)
		require.NoError(t, db.Model(&img).Updates(update).Error)
		for _, n := range []string{img.File, img.ThumbFile} {
			require.NoError(t, os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600))
		}
		return img
	}
	ofContact := mk("c", map[string]any{"contact_id": f.contact.ID})
	ofNote := mk("n", map[string]any{"note_id": f.note.ID})
	kept := mk("k", map[string]any{"contact_id": keeper.contact.ID})
	softDeleteLikeController(t, db, f)

	n, err := PurgeTrashedContacts(db, a.ID, []uint{f.contact.ID}, dir, time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	assert.EqualValues(t, 1, count(t, db, &models.Image{}, "1 = 1"))
	for _, img := range []models.Image{ofContact, ofNote} {
		assert.NoFileExists(t, filepath.Join(dir, img.File))
		assert.NoFileExists(t, filepath.Join(dir, img.ThumbFile))
	}
	assert.FileExists(t, filepath.Join(dir, kept.File))
}
