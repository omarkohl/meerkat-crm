package files_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"meerkat/photos/internal/adapters/files"
	"meerkat/photos/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	bgColour     = color.RGBA{R: 200, G: 40, B: 40, A: 255}
	centreColour = color.RGBA{R: 30, G: 160, B: 60, A: 255}
)

// fillWithCentre paints the background and a centred block of a distinct colour
// covering half the short side, so resizing mistakes (shifted or wrong crop) show.
func fillWithCentre(img *image.RGBA) {
	b := img.Bounds()
	half := min(b.Dx(), b.Dy()) / 4
	cx, cy := b.Dx()/2, b.Dy()/2
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := bgColour
			if x >= cx-half && x < cx+half && y >= cy-half && y < cy+half {
				c = centreColour
			}
			img.Set(x, y, c)
		}
	}
}

func assertCentre(t *testing.T, what string, img image.Image) {
	t.Helper()
	b := img.Bounds()
	r, g, bl, _ := img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).RGBA()
	got := [3]int{int(r >> 8), int(g >> 8), int(bl >> 8)}
	want := [3]int{int(centreColour.R), int(centreColour.G), int(centreColour.B)}
	for i := range got {
		if d := got[i] - want[i]; d < -10 || d > 10 {
			t.Errorf("%s centre pixel is %v, want %v +-10", what, got, want)
			return
		}
	}
}

func pngBytes(t *testing.T, w, h int) *bytes.Reader {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillWithCentre(img)
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return bytes.NewReader(buf.Bytes())
}

func decodeJPEG(t *testing.T, path string) image.Image {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	img, err := jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	return img
}

func TestSaveCropsToASquareAndScalesThePhotoDownTo400AndTheThumbnailTo48(t *testing.T) {
	tests := []struct {
		name         string
		w, h         int
		wantW, wantH int
	}{
		{"large square is downscaled to 400", 800, 800, 400, 400},
		{"large landscape is cropped square then downscaled", 1000, 600, 400, 400},
		{"large portrait is cropped square then downscaled", 500, 900, 400, 400},
		{"small square keeps its size", 200, 200, 200, 200},
		{"small landscape is cropped to the short side", 300, 100, 100, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			photo, thumb, err := files.New(dir).Save(pngBytes(t, tt.w, tt.h))
			require.NoError(t, err)

			photoImg := decodeJPEG(t, filepath.Join(dir, photo))
			assert.Equal(t, image.Rect(0, 0, tt.wantW, tt.wantH), photoImg.Bounds())
			assertCentre(t, "photo", photoImg)

			assert.True(t, strings.HasSuffix(thumb, "_thumb.jpg"))
			thumbImg := decodeJPEG(t, filepath.Join(dir, thumb))
			assert.Equal(t, image.Rect(0, 0, 48, 48), thumbImg.Bounds())
			assertCentre(t, "thumbnail", thumbImg)
		})
	}
}

func TestSaveWritesWithPrivateModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "photos")
	photo, _, err := files.New(dir).Save(pngBytes(t, 50, 50))
	require.NoError(t, err)

	di, err := os.Stat(dir)
	require.NoError(t, err)
	fi, err := os.Stat(filepath.Join(dir, photo))
	require.NoError(t, err)
	assert.Zero(t, di.Mode().Perm()&0o007, "others have no access to the directory")
	assert.Zero(t, fi.Mode().Perm()&0o137, "the file is neither executable nor group-writable, others cannot touch it")
}

// Valid HEIC photos (brands heic and mif1) must still be accepted by the fail-closed
// header check; a truncated one is refused.
func TestSaveAcceptsHEICAndRefusesATruncatedOne(t *testing.T) {
	data, err := os.ReadFile("testdata/test8.heic")
	require.NoError(t, err)
	mif1 := append([]byte(nil), data...)
	copy(mif1[8:12], "mif1")

	for name, content := range map[string][]byte{"heic": data, "mif1": mif1} {
		t.Run(name, func(t *testing.T) {
			_, _, err := files.New(t.TempDir()).Save(bytes.NewReader(content))
			require.NoError(t, err)
		})
	}
	dir := t.TempDir()
	_, _, err = files.New(dir).Save(bytes.NewReader(data[:40]))
	assert.ErrorIs(t, err, domain.ErrInvalidPhoto)
	entries, _ := os.ReadDir(dir)
	assert.Empty(t, entries)
}

func TestOpenRefusesWhatIsNotAFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o750))
	d := files.New(dir)
	_, err := d.Open("sub")
	assert.ErrorIs(t, err, domain.ErrFileMissing)
	_, err = d.Open("nothing.jpg")
	assert.ErrorIs(t, err, domain.ErrFileMissing)
}
