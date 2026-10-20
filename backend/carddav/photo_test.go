package carddav

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestSaveContactPhotoSizes(t *testing.T) {
	tests := []struct {
		name         string
		w, h         int
		wantW, wantH int
	}{
		{"large square is downscaled to 400", 800, 800, 400, 400},
		{"large landscape is cropped square then downscaled", 1000, 600, 400, 400},
		{"small square keeps its size", 200, 200, 200, 200},
		{"small portrait is cropped to the short side", 100, 300, 100, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, tt.w, tt.h))
			fillWithCentre(img)
			var in bytes.Buffer
			if err := png.Encode(&in, img); err != nil {
				t.Fatal(err)
			}

			dir := t.TempDir()
			photo, thumb, err := SaveContactPhoto(in.Bytes(), "image/png", dir)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, photo))
			if err != nil {
				t.Fatal(err)
			}
			out, err := jpeg.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if b := out.Bounds(); b.Dx() != tt.wantW || b.Dy() != tt.wantH {
				t.Errorf("photo is %dx%d, want %dx%d", b.Dx(), b.Dy(), tt.wantW, tt.wantH)
			}
			assertCentre(t, "photo", out)

			if !strings.HasSuffix(thumb, "_thumb.jpg") {
				t.Fatalf("thumbnail is not a file name: %.30s", thumb)
			}
			raw, err := os.ReadFile(filepath.Join(dir, thumb))
			if err != nil {
				t.Fatal(err)
			}
			th, err := jpeg.Decode(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if b := th.Bounds(); b.Dx() != 48 || b.Dy() != 48 {
				t.Errorf("thumbnail is %dx%d, want 48x48", b.Dx(), b.Dy())
			}
			assertCentre(t, "thumbnail", th)
		})
	}
}

func TestSaveContactPhotoUsesPrivateModes(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	var in bytes.Buffer
	if err := png.Encode(&in, img); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "new", "photos")
	photo, _, err := SaveContactPhoto(in.Bytes(), "image/png", dir)
	if err != nil {
		t.Fatal(err)
	}
	// Written through photofile: private modes, not the process default.
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, photo))
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm()&0o007 != 0 || fi.Mode().Perm()&0o137 != 0 {
		t.Errorf("dir %v, file %v: others must have no access, file must not be executable or group-writable", di.Mode().Perm(), fi.Mode().Perm())
	}
}

func TestSaveContactPhotoRefusesHugeDimensions(t *testing.T) {
	var small bytes.Buffer
	if err := png.Encode(&small, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	data := small.Bytes()
	binary.BigEndian.PutUint32(data[16:], 30000)
	binary.BigEndian.PutUint32(data[20:], 30000)
	binary.BigEndian.PutUint32(data[29:], crc32.ChecksumIEEE(data[12:29]))
	dir := t.TempDir()
	if _, _, err := SaveContactPhoto(data, "image/png", dir); err == nil {
		t.Fatal("expected an error for a 900 megapixel image")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("nothing may be written, found %d files", len(entries))
	}
}

// @004-US9
func TestReencodePhotoReturnsAJPEGOfTheSameImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 30, 20))
	fillWithCentre(src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}

	out, err := ReencodePhoto(buf.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("output is not a JPEG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 30 || b.Dy() != 20 {
		t.Errorf("size is %dx%d, want 30x20", b.Dx(), b.Dy())
	}
	assertCentre(t, "re-encoded", img)
}

// @004-US9
func TestReencodePhotoRefusesWhatIsNotAPhoto(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	if _, err := ReencodePhoto(svg, "image/svg+xml"); err == nil {
		t.Error("an SVG was accepted")
	}
}
