package services

import (
	"testing"
	"time"

	"meerkat/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func imageDB(t *testing.T) (*gorm.DB, models.User, models.User) {
	t.Helper()
	db := setupTestDB()
	require.NoError(t, db.AutoMigrate(&models.Image{}))
	a := models.User{Username: "a", Email: "a@example.com", Password: "x"}
	b := models.User{Username: "b", Email: "b@example.com", Password: "x"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	return db, a, b
}

func newImage(t *testing.T, db *gorm.DB, user uint, name string) models.Image {
	t.Helper()
	img := models.Image{UserID: user, File: "images/u1/" + name + "_image.webp", ThumbFile: "images/u1/" + name + "_thumb.webp", Mime: "image/webp", Width: 10, Height: 20, Bytes: 5}
	require.NoError(t, db.Create(&img).Error)
	return img
}

func newNote(t *testing.T, db *gorm.DB, user uint) models.Note {
	t.Helper()
	n := models.Note{UserID: user, Content: "n", Date: time.Now()}
	require.NoError(t, db.Create(&n).Error)
	return n
}

func idsOf(images ...models.Image) []uint {
	out := make([]uint, len(images))
	for i, img := range images {
		out[i] = img.ID
	}
	return out
}

func TestSetOwnerImagesClaimsOrdersAndDropsImages(t *testing.T) {
	db, a, _ := imageDB(t)
	note := newNote(t, db, a.ID)
	owner := ImageOwner{Kind: ImageOfNote, ID: note.ID}
	first, second, third := newImage(t, db, a.ID, "1"), newImage(t, db, a.ID, "2"), newImage(t, db, a.ID, "3")

	removed, err := SetOwnerImages(db, a.ID, owner, idsOf(second, first, third))
	require.NoError(t, err)
	assert.Empty(t, removed)
	got, err := ImagesFor(db, a.ID, ImageOfNote, []uint{note.ID})
	require.NoError(t, err)
	require.Len(t, got[note.ID], 3)
	assert.Equal(t, idsOf(second, first, third), []uint{got[note.ID][0].ID, got[note.ID][1].ID, got[note.ID][2].ID})

	removed, err = SetOwnerImages(db, a.ID, owner, idsOf(third))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{first.File, first.ThumbFile, second.File, second.ThumbFile}, removed)
	var n int64
	db.Model(&models.Image{}).Count(&n)
	assert.EqualValues(t, 1, n)

	removed, err = SetOwnerImages(db, a.ID, owner, []uint{})
	require.NoError(t, err)
	assert.Len(t, removed, 2)
	got, err = ImagesFor(db, a.ID, ImageOfNote, []uint{note.ID})
	require.NoError(t, err)
	assert.Empty(t, got[note.ID])
}

func TestSetOwnerImagesRefusesWhatIsNotTheActorsOrIsTakenOrTooMany(t *testing.T) {
	db, a, b := imageDB(t)
	note, other := newNote(t, db, a.ID), newNote(t, db, a.ID)
	mine, theirs := newImage(t, db, a.ID, "mine"), newImage(t, db, b.ID, "theirs")

	_, err := SetOwnerImages(db, a.ID, ImageOwner{Kind: ImageOfNote, ID: note.ID}, []uint{theirs.ID})
	assert.ErrorIs(t, err, ErrImageNotFound, "another user's image")
	_, err = SetOwnerImages(db, a.ID, ImageOwner{Kind: ImageOfNote, ID: note.ID}, []uint{9999})
	assert.ErrorIs(t, err, ErrImageNotFound)

	_, err = SetOwnerImages(db, a.ID, ImageOwner{Kind: ImageOfNote, ID: other.ID}, []uint{mine.ID})
	require.NoError(t, err)
	_, err = SetOwnerImages(db, a.ID, ImageOwner{Kind: ImageOfNote, ID: note.ID}, []uint{mine.ID})
	assert.ErrorIs(t, err, ErrImageNotFound, "an image of another note cannot be taken")

	var ids []uint
	for i := range MaxImagesPerNote + 1 {
		ids = append(ids, newImage(t, db, a.ID, "x"+string(rune('a'+i))).ID)
	}
	_, err = SetOwnerImages(db, a.ID, ImageOwner{Kind: ImageOfNote, ID: note.ID}, ids)
	assert.ErrorIs(t, err, ErrTooManyImages)
	_, err = SetOwnerImages(db, a.ID, ImageOwner{Kind: ImageOfNote, ID: note.ID}, []uint{ids[0], ids[0]})
	assert.ErrorIs(t, err, ErrImageNotFound, "the same image twice")
}

func TestImagesFromAnotherUserNeverShowUp(t *testing.T) {
	db, a, b := imageDB(t)
	note := newNote(t, db, a.ID)
	img := newImage(t, db, a.ID, "x")
	_, err := SetOwnerImages(db, a.ID, ImageOwner{Kind: ImageOfNote, ID: note.ID}, []uint{img.ID})
	require.NoError(t, err)
	got, err := ImagesFor(db, b.ID, ImageOfNote, []uint{note.ID})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestDeleteOwnerImagesReturnsTheFilesOfTheRowsItDeleted(t *testing.T) {
	db, a, _ := imageDB(t)
	note, keep := newNote(t, db, a.ID), newNote(t, db, a.ID)
	one, two := newImage(t, db, a.ID, "1"), newImage(t, db, a.ID, "2")
	_, err := SetOwnerImages(db, a.ID, ImageOwner{Kind: ImageOfNote, ID: note.ID}, []uint{one.ID})
	require.NoError(t, err)
	_, err = SetOwnerImages(db, a.ID, ImageOwner{Kind: ImageOfNote, ID: keep.ID}, []uint{two.ID})
	require.NoError(t, err)

	files, err := DeleteOwnerImages(db, a.ID, ImageOwner{Kind: ImageOfNote, ID: note.ID})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{one.File, one.ThumbFile}, files)
	var n int64
	db.Model(&models.Image{}).Count(&n)
	assert.EqualValues(t, 1, n)
}
