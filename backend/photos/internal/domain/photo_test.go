package domain_test

import (
	"testing"

	"meerkat/photos/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestNamesThatLeaveTheDirectoryAreUnsafe(t *testing.T) {
	for _, name := range []string{"../secret.txt", "/etc/passwd", "a/../b.jpg"} {
		assert.False(t, domain.SafePhotoName(name), name)
	}
	assert.True(t, domain.SafePhotoName("abc_photo.jpg"))
	assert.True(t, domain.SafePhotoName("sub/abc.jpg"), "a photo may sit in a sub directory")
}

func TestAThumbnailIsAPlainFileName(t *testing.T) {
	for _, name := range []string{"sub/x.jpg", `..\secret.txt`, "../x", "/abs.jpg"} {
		assert.False(t, domain.SafeThumbnailName(name), name)
	}
	assert.True(t, domain.SafeThumbnailName("abc_thumb.jpg"))
}

func TestLegacyThumbnailsAreDecodedFromTheirDataURL(t *testing.T) {
	data, err := domain.LegacyImage("data:image/jpeg;base64,anBlZ2J5dGVz")
	assert.NoError(t, err)
	assert.Equal(t, "jpegbytes", string(data))

	_, err = domain.LegacyImage("")
	assert.ErrorIs(t, err, domain.ErrThumbnailNotFound)
	_, err = domain.LegacyImage("data:nothing")
	assert.ErrorIs(t, err, domain.ErrBadLegacyThumbnail)
	_, err = domain.LegacyImage("data:image/jpeg;base64,!!!")
	assert.ErrorIs(t, err, domain.ErrLegacyThumbnailDecode)
}
