package carddav_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"

	"meerkat/carddav"
	"meerkat/models"
	"meerkat/services"

	"github.com/emersion/go-vcard"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func ramp(t *testing.T, w, h, quality int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			v := uint8(255 - x*255/(w-1))
			img.Set(x, y, color.RGBA{v, v / 2, 255 - v, 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}))
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func cardWithPhoto(t *testing.T, photo string) vcard.Card {
	t.Helper()
	src := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:uid-1\r\nFN:Al Smith\r\nN:Smith;Al;;;\r\nPHOTO;ENCODING=b;TYPE=JPEG:" + photo + "\r\nEND:VCARD\r\n"
	card, err := vcard.NewDecoder(strings.NewReader(src)).Decode()
	require.NoError(t, err)
	return card
}

// @023-US4
func TestAPhotoInACardBecomesAnImageButACopyOfItDoesNot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Image{}))
	user := models.User{Username: "u", Password: "password123!A", Email: "u@example.com"}
	require.NoError(t, db.Create(&user).Error)
	dir := t.TempDir()
	b := carddav.NewBackend(db, dir)
	b.SetPhotoImporter(services.ImportContactPhotoAt)
	ctx := carddav.ContextWithUser(context.Background(), user.ID, "u", db, dir)
	put := func(card vcard.Card) {
		t.Helper()
		_, err := b.PutAddressObject(ctx, "/carddav/addressbooks/u/contacts/uid-1.vcf", card, nil)
		require.NoError(t, err)
	}

	put(cardWithPhoto(t, ramp(t, 300, 200, 95)))
	var contact models.Contact
	require.NoError(t, db.Where("vcard_uid = ?", "uid-1").First(&contact).Error)
	require.NotEmpty(t, contact.Photo)
	require.NotNil(t, contact.MainImageID)

	put(cardWithPhoto(t, ramp(t, 150, 100, 60)))
	var count int64
	require.NoError(t, db.Model(&models.Image{}).Where("contact_id = ?", contact.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "the phone sent the same picture again")
}
