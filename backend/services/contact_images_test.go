package services

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"meerkat/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// quadrantJPEG is red top-left, green top-right, blue bottom-left, white bottom-right.
func quadrantJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{255, 0, 0, 255}
			switch {
			case x >= w/2 && y < h/2:
				c = color.RGBA{0, 255, 0, 255}
			case x < w/2 && y >= h/2:
				c = color.RGBA{0, 0, 255, 255}
			case x >= w/2 && y >= h/2:
				c = color.RGBA{255, 255, 255, 255}
			}
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	require.NoError(t, jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}))
	return b.Bytes()
}

type contactImageEnv struct {
	db      *gorm.DB
	dir     string
	user    models.User
	other   models.User
	contact models.Contact
}

func newContactImageEnv(t *testing.T) *contactImageEnv {
	t.Helper()
	db, a, b := imageDB(t)
	c := models.Contact{UserID: a.ID, Firstname: "Ann"}
	require.NoError(t, db.Create(&c).Error)
	return &contactImageEnv{db: db, dir: t.TempDir(), user: a, other: b, contact: c}
}

func (e *contactImageEnv) pending(t *testing.T, owner uint, w, h int) models.Image {
	t.Helper()
	img, err := StoreImage(context.Background(), e.dir, owner, quadrantJPEG(t, w, h))
	require.NoError(t, err)
	require.NoError(t, e.db.Create(&img).Error)
	return img
}

func (e *contactImageEnv) reload(t *testing.T, c models.Contact) models.Contact {
	t.Helper()
	require.NoError(t, e.db.First(&c, c.ID).Error)
	return c
}

func (e *contactImageEnv) exists(name string) bool {
	_, err := os.Stat(filepath.Join(e.dir, name))
	return err == nil
}

func (e *contactImageEnv) avatar(t *testing.T, c models.Contact) (photo, thumb image.Image) {
	t.Helper()
	for _, f := range []struct {
		name string
		out  *image.Image
	}{{c.Photo, &photo}, {c.PhotoThumbnailFile, &thumb}} {
		data, err := os.ReadFile(filepath.Join(e.dir, f.name))
		require.NoError(t, err)
		*f.out, err = jpeg.Decode(bytes.NewReader(data))
		require.NoError(t, err)
	}
	return photo, thumb
}

func colourAt(img image.Image, x, y int) [3]int {
	r, g, b, _ := img.At(x, y).RGBA()
	return [3]int{int(r >> 8), int(g >> 8), int(b >> 8)}
}

func assertColour(t *testing.T, img image.Image, x, y int, want [3]int) {
	t.Helper()
	got := colourAt(img, x, y)
	for i := range got {
		assert.InDelta(t, want[i], got[i], 45, "pixel (%d,%d) is %v, want %v", x, y, got, want)
	}
}

func (e *contactImageEnv) set(t *testing.T, c models.Contact, ids ...uint) (*ImageEffects, error) {
	t.Helper()
	eff := &ImageEffects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		return SetContactImages(tx, e.dir, e.user.ID, c.ID, ids, eff)
	})
	eff.Finish(e.dir, err)
	return eff, err
}

func TestTheFirstImageOfAContactBecomesItsMainImageAndAvatar(t *testing.T) {
	e := newContactImageEnv(t)
	img := e.pending(t, e.user.ID, 1000, 600)

	_, err := e.set(t, e.contact, img.ID)
	require.NoError(t, err)

	c := e.reload(t, e.contact)
	require.NotNil(t, c.MainImageID)
	assert.Equal(t, img.ID, *c.MainImageID)
	var stored models.Image
	require.NoError(t, e.db.First(&stored, img.ID).Error)
	require.NotNil(t, stored.CropSize)
	assert.Equal(t, [3]int{200, 0, 600}, [3]int{*stored.CropX, *stored.CropY, *stored.CropSize}, "centred square")
	assert.True(t, e.exists(c.Photo) && e.exists(c.PhotoThumbnailFile))
	photo, thumb := e.avatar(t, c)
	assert.Equal(t, 400, photo.Bounds().Dx())
	assert.Equal(t, 96, thumb.Bounds().Dx())
}

func TestAddingMoreImagesKeepsTheMainImageAndItsAvatar(t *testing.T) {
	e := newContactImageEnv(t)
	first, second := e.pending(t, e.user.ID, 100, 100), e.pending(t, e.user.ID, 100, 100)
	_, err := e.set(t, e.contact, first.ID)
	require.NoError(t, err)
	before := e.reload(t, e.contact)

	eff, err := e.set(t, e.contact, second.ID, first.ID)
	require.NoError(t, err)

	after := e.reload(t, e.contact)
	assert.Equal(t, first.ID, *after.MainImageID)
	assert.Equal(t, before.Photo, after.Photo, "the avatar is not made again")
	assert.Empty(t, eff.Removed)
}

func TestRemovingTheMainImageMakesTheNextOneMainAndReplacesTheAvatar(t *testing.T) {
	e := newContactImageEnv(t)
	first, second := e.pending(t, e.user.ID, 100, 100), e.pending(t, e.user.ID, 200, 100)
	_, err := e.set(t, e.contact, first.ID, second.ID)
	require.NoError(t, err)
	before := e.reload(t, e.contact)

	eff, err := e.set(t, e.contact, second.ID)
	require.NoError(t, err)

	after := e.reload(t, e.contact)
	assert.Equal(t, second.ID, *after.MainImageID)
	assert.NotEqual(t, before.Photo, after.Photo)
	assert.False(t, e.exists(before.Photo), "the old avatar goes")
	assert.False(t, e.exists(first.File), "so do the files of the removed image")
	assert.Contains(t, eff.Removed, before.Photo)
	assert.True(t, e.exists(after.Photo))
}

func TestRemovingTheLastImageClearsTheAvatar(t *testing.T) {
	e := newContactImageEnv(t)
	img := e.pending(t, e.user.ID, 100, 100)
	_, err := e.set(t, e.contact, img.ID)
	require.NoError(t, err)
	before := e.reload(t, e.contact)

	_, err = e.set(t, e.contact)
	require.NoError(t, err)

	after := e.reload(t, e.contact)
	assert.Nil(t, after.MainImageID)
	assert.Empty(t, after.Photo)
	assert.Empty(t, after.PhotoThumbnailFile)
	assert.False(t, e.exists(before.Photo))
}

func TestMovingTheMainImageToAnotherContactRepairsTheFirstOne(t *testing.T) {
	e := newContactImageEnv(t)
	target := models.Contact{UserID: e.user.ID, Firstname: "Bob"}
	require.NoError(t, e.db.Create(&target).Error)
	mine, spare := e.pending(t, e.user.ID, 100, 100), e.pending(t, e.user.ID, 100, 100)
	_, err := e.set(t, e.contact, mine.ID, spare.ID)
	require.NoError(t, err)

	_, err = e.set(t, target, mine.ID)
	require.NoError(t, err)

	source := e.reload(t, e.contact)
	require.NotNil(t, source.MainImageID)
	assert.Equal(t, spare.ID, *source.MainImageID, "the contact left behind has a main image of its own again")
	moved := e.reload(t, target)
	assert.Equal(t, mine.ID, *moved.MainImageID)
	assert.NotEmpty(t, moved.Photo)
}

func TestImagesOfOtherUsersCannotBeTakenByAContact(t *testing.T) {
	e := newContactImageEnv(t)
	theirs := e.pending(t, e.other.ID, 100, 100)
	_, err := e.set(t, e.contact, theirs.ID)
	assert.ErrorIs(t, err, ErrImageNotFound)

	foreign := models.Contact{UserID: e.other.ID, Firstname: "Eve"}
	require.NoError(t, e.db.Create(&foreign).Error)
	mine := e.pending(t, e.user.ID, 100, 100)
	_, err = e.set(t, foreign, mine.ID)
	assert.ErrorIs(t, err, ErrContactNotFound)
}

func TestSetMainImageCutsTheAvatarFromTheCrop(t *testing.T) {
	e := newContactImageEnv(t)
	img := e.pending(t, e.user.ID, 1000, 600)
	_, err := e.set(t, e.contact, img.ID)
	require.NoError(t, err)
	before := e.reload(t, e.contact)

	eff := &ImageEffects{}
	err = e.db.Transaction(func(tx *gorm.DB) error {
		return SetMainImage(tx, e.dir, e.user.ID, e.contact.ID, img.ID, ImageCropInput{X: 500, Y: 0, Size: 500}, eff)
	})
	eff.Finish(e.dir, err)
	require.NoError(t, err)

	after := e.reload(t, e.contact)
	photo, _ := e.avatar(t, after)
	assertColour(t, photo, 10, 10, [3]int{0, 255, 0})
	assertColour(t, photo, 10, 390, [3]int{255, 255, 255})
	assert.False(t, e.exists(before.Photo))
	var stored models.Image
	require.NoError(t, e.db.First(&stored, img.ID).Error)
	assert.Equal(t, [3]int{500, 0, 500}, [3]int{*stored.CropX, *stored.CropY, *stored.CropSize})
}

func TestSetMainImageRefusesWhatItCannotUse(t *testing.T) {
	e := newContactImageEnv(t)
	img := e.pending(t, e.user.ID, 1000, 600)
	_, err := e.set(t, e.contact, img.ID)
	require.NoError(t, err)
	spare := e.pending(t, e.user.ID, 100, 100)
	theirs := e.pending(t, e.other.ID, 100, 100)

	try := func(id uint, crop ImageCropInput) error {
		return e.db.Transaction(func(tx *gorm.DB) error {
			return SetMainImage(tx, e.dir, e.user.ID, e.contact.ID, id, crop, &ImageEffects{})
		})
	}
	good := ImageCropInput{X: 0, Y: 0, Size: 600}
	assert.ErrorIs(t, try(spare.ID, good), ErrImageNotFound, "not on this contact")
	assert.ErrorIs(t, try(theirs.ID, good), ErrImageNotFound)
	for _, bad := range []ImageCropInput{{X: 500, Y: 0, Size: 600}, {X: -1, Y: 0, Size: 10}, {X: 0, Y: 0, Size: 0}, {X: 0, Y: 100, Size: 600}} {
		assert.ErrorIs(t, try(img.ID, bad), ErrInvalidCrop, "%+v", bad)
	}
}

func TestAddContactImageMakesTheNewImageMainAndPutsItLastOrFirst(t *testing.T) {
	e := newContactImageEnv(t)
	add := func(front bool) models.Image {
		eff := &ImageEffects{}
		var img models.Image
		err := e.db.Transaction(func(tx *gorm.DB) error {
			var err error
			img, err = AddContactImage(context.Background(), tx, e.dir, e.user.ID, e.contact.ID, quadrantJPEG(t, 100, 100), front, eff)
			return err
		})
		eff.Finish(e.dir, err)
		require.NoError(t, err)
		return img
	}
	first := add(false)
	before := e.reload(t, e.contact)
	second := add(false)
	c := e.reload(t, e.contact)
	assert.Equal(t, second.ID, *c.MainImageID)
	assert.False(t, e.exists(before.Photo), "the avatar of the first image goes")
	got, err := ImagesFor(e.db, e.user.ID, ImageOfContact, []uint{c.ID})
	require.NoError(t, err)
	assert.Equal(t, []uint{first.ID, second.ID}, []uint{got[c.ID][0].ID, got[c.ID][1].ID})

	third := add(true)
	got, _ = ImagesFor(e.db, e.user.ID, ImageOfContact, []uint{c.ID})
	assert.Equal(t, []uint{third.ID, first.ID, second.ID}, []uint{got[c.ID][0].ID, got[c.ID][1].ID, got[c.ID][2].ID})
	assert.Equal(t, third.ID, *e.reload(t, e.contact).MainImageID)
}

func TestAContactTakesAtMostFiftyImages(t *testing.T) {
	e := newContactImageEnv(t)
	var ids []uint
	for range MaxImagesPerContact + 1 {
		img := models.Image{UserID: e.user.ID, File: "images/u1/x_image.webp", ThumbFile: "images/u1/x_thumb.webp", Mime: "image/webp", Width: 1, Height: 1}
		require.NoError(t, e.db.Create(&img).Error)
		ids = append(ids, img.ID)
	}
	_, err := e.set(t, e.contact, ids...)
	assert.ErrorIs(t, err, ErrTooManyImages)
}

func TestARolledBackChangeRemovesTheAvatarFilesItMade(t *testing.T) {
	e := newContactImageEnv(t)
	img := e.pending(t, e.user.ID, 100, 100)
	eff := &ImageEffects{}
	err := e.db.Transaction(func(tx *gorm.DB) error {
		if err := SetContactImages(tx, e.dir, e.user.ID, e.contact.ID, []uint{img.ID}, eff); err != nil {
			return err
		}
		return assert.AnError
	})
	require.Error(t, err)
	require.NotEmpty(t, eff.Created)
	eff.Finish(e.dir, err)
	for _, name := range eff.Created {
		assert.False(t, e.exists(name), name)
	}
	assert.True(t, e.exists(img.File), "the uploaded image itself is the sweep's business")
}

func TestExistingPhotosBecomeImagesWithoutTouchingTheAvatar(t *testing.T) {
	e := newContactImageEnv(t)
	photo := "legacy_photo.jpg"
	require.NoError(t, os.WriteFile(filepath.Join(e.dir, photo), quadrantJPEG(t, 400, 400), 0o600))
	require.NoError(t, e.db.Model(&e.contact).Updates(map[string]any{"photo": photo, "photo_thumbnail_file": "legacy_thumb.jpg"}).Error)
	trashed := models.Contact{UserID: e.user.ID, Firstname: "Gone", Photo: "missing_photo.jpg"}
	require.NoError(t, e.db.Create(&trashed).Error)

	n, err := MigrateContactPhotos(context.Background(), e.db, e.dir)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the contact whose file is gone is skipped, not fatal")

	c := e.reload(t, e.contact)
	require.NotNil(t, c.MainImageID)
	assert.Equal(t, photo, c.Photo, "the avatar, and so the vCard, stays as it was")
	assert.Equal(t, "legacy_thumb.jpg", c.PhotoThumbnailFile)
	var img models.Image
	require.NoError(t, e.db.First(&img, *c.MainImageID).Error)
	assert.Equal(t, [3]int{0, 0, 400}, [3]int{*img.CropX, *img.CropY, *img.CropSize})
	assert.Equal(t, c.ID, *img.ContactID)

	n, err = MigrateContactPhotos(context.Background(), e.db, e.dir)
	require.NoError(t, err)
	assert.Zero(t, n, "a second run changes nothing")
}

func TestAPhotoChangedDuringTheMigrationIsLeftAlone(t *testing.T) {
	e := newContactImageEnv(t)
	photo := "legacy_photo.jpg"
	require.NoError(t, os.WriteFile(filepath.Join(e.dir, photo), quadrantJPEG(t, 400, 400), 0o600))
	require.NoError(t, e.db.Model(&e.contact).Update("photo", photo).Error)
	stale := e.reload(t, e.contact)
	chosen := e.pending(t, e.user.ID, 100, 100)
	require.NoError(t, e.db.Model(&chosen).Update("contact_id", e.contact.ID).Error)
	require.NoError(t, e.db.Model(&e.contact).Update("main_image_id", chosen.ID).Error)

	require.Error(t, migrateContactPhoto(context.Background(), e.db, e.dir, stale))

	assert.Equal(t, chosen.ID, *e.reload(t, e.contact).MainImageID, "the user's choice stays")
	var images int64
	e.db.Model(&models.Image{}).Where("contact_id = ?", e.contact.ID).Count(&images)
	assert.EqualValues(t, 1, images, "no image is added")
}
