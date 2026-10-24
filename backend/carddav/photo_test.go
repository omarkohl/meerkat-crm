package carddav

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
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
