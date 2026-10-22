package imageproc_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
	"time"

	"meerkat/imageproc"

	"github.com/gen2brain/webp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tiff builds little-endian EXIF data with an orientation and optionally DateTimeOriginal.
func tiff(orientation uint16, taken string) []byte {
	le := binary.LittleEndian
	entry := func(tag, typ uint16, count, value uint32) []byte {
		e := le.AppendUint16(nil, tag)
		e = le.AppendUint16(e, typ)
		e = le.AppendUint32(e, count)
		return le.AppendUint32(e, value)
	}
	b := []byte("II*\x00")
	b = le.AppendUint32(b, 8)
	if taken == "" {
		b = le.AppendUint16(b, 1)
		b = append(b, entry(0x0112, 3, 1, uint32(orientation))...)
		return le.AppendUint32(b, 0)
	}
	b = le.AppendUint16(b, 2)
	b = append(b, entry(0x0112, 3, 1, uint32(orientation))...)
	b = append(b, entry(0x8769, 4, 1, 38)...)
	b = le.AppendUint32(b, 0)
	b = le.AppendUint16(b, 1)
	b = append(b, entry(0x9003, 2, 20, 56)...)
	b = le.AppendUint32(b, 0)
	return append(b, taken+"\x00"...)
}

func withEXIF(t *testing.T, jpg, exif []byte) []byte {
	t.Helper()
	payload := append([]byte("Exif\x00\x00"), exif...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	return append(append(append([]byte{}, jpg[:2]...), append(seg, payload...)...), jpg[2:]...)
}

// quadrants is red top-left, green top-right, blue bottom-left, white bottom-right.
func quadrants(w, h int) *image.RGBA {
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
	return img
}

func jpegOf(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}))
	return b.Bytes()
}

func pngOf(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, img))
	return b.Bytes()
}

func near(t *testing.T, img image.Image, x, y int, want color.RGBA) {
	t.Helper()
	r, g, b, _ := img.At(x, y).RGBA()
	got := [3]int{int(r >> 8), int(g >> 8), int(b >> 8)}
	for i, w := range [3]uint8{want.R, want.G, want.B} {
		if d := got[i] - int(w); d < -40 || d > 40 {
			t.Fatalf("pixel (%d,%d) is %v, want %v", x, y, got, want)
		}
	}
}

func decodeWebP(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := webp.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	return img
}

func TestAPhoneJPEGWithOrientation6IsStoredUprightWithItsDate(t *testing.T) {
	// 80x40 landscape that a phone holds rotated: orientation 6 means "rotate 90 clockwise to view"
	src := withEXIF(t, jpegOf(t, quadrants(80, 40)), tiff(6, "2026:10:20 18:03:00"))

	res, err := imageproc.Prepare(context.Background(), src)
	require.NoError(t, err)

	assert.Equal(t, 40, res.Width)
	assert.Equal(t, 80, res.Height)
	assert.Equal(t, "image/webp", res.Mime)
	require.NotNil(t, res.TakenAt)
	assert.Equal(t, time.Date(2026, 10, 20, 18, 3, 0, 0, time.UTC), *res.TakenAt)

	img := decodeWebP(t, res.Image)
	assert.Equal(t, image.Rect(0, 0, 40, 80), img.Bounds())
	// rotated clockwise: the old top-left (red) is now top-right, the old bottom-left (blue) top-left
	near(t, img, 35, 5, color.RGBA{255, 0, 0, 255})
	near(t, img, 5, 5, color.RGBA{0, 0, 255, 255})

	// the stored EXIF block keeps the date and says "upright"
	exif := imageproc.ExifOfWebP(res.Image)
	require.NotEmpty(t, exif)
	info := imageproc.ParseExif(exif)
	assert.Equal(t, 1, info.Orientation)
	require.NotNil(t, info.TakenAt)
	assert.Equal(t, *res.TakenAt, *info.TakenAt)

	assert.Empty(t, imageproc.ExifOfWebP(res.Thumb), "thumbnails carry no metadata")
}

func TestEveryOrientationEndsUpright(t *testing.T) {
	// the same picture stored under each orientation must look the same after Prepare
	cases := map[int]func(*image.RGBA) image.Image{}
	for o := 1; o <= 8; o++ {
		cases[o] = nil
	}
	for o := range cases {
		stored := transformForTest(quadrants(60, 40), o)
		src := withEXIF(t, jpegOf(t, stored), tiff(uint16(o), ""))
		res, err := imageproc.Prepare(context.Background(), src)
		require.NoError(t, err, "orientation %d", o)
		img := decodeWebP(t, res.Image)
		assert.Equal(t, image.Rect(0, 0, 60, 40), img.Bounds(), "orientation %d", o)
		near(t, img, 5, 5, color.RGBA{255, 0, 0, 255})
		near(t, img, 55, 5, color.RGBA{0, 255, 0, 255})
		near(t, img, 5, 35, color.RGBA{0, 0, 255, 255})
		near(t, img, 55, 35, color.RGBA{255, 255, 255, 255})
	}
}

// transformForTest is the inverse of the EXIF orientation: what a camera stores so that
// applying the orientation gives src.
func transformForTest(src *image.RGBA, o int) image.Image {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	swap := o >= 5
	dw, dh := w, h
	if swap {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch o {
			case 1:
				dx, dy = x, y
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = y, w-1-x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = h-1-y, x
			}
			dst.Set(dx, dy, src.At(x, y))
		}
	}
	return dst
}

func TestLargeImagesAreScaledToTheCapAndSmallOnesAreNotEnlarged(t *testing.T) {
	big, err := imageproc.Prepare(context.Background(), jpegOf(t, quadrants(3000, 1500)))
	require.NoError(t, err)
	assert.Equal(t, 2048, big.Width)
	assert.Equal(t, 1024, big.Height)
	assert.Equal(t, image.Rect(0, 0, 480, 240), decodeWebP(t, big.Thumb).Bounds())

	small, err := imageproc.Prepare(context.Background(), jpegOf(t, quadrants(100, 60)))
	require.NoError(t, err)
	assert.Equal(t, 100, small.Width)
	assert.Equal(t, image.Rect(0, 0, 100, 60), decodeWebP(t, small.Thumb).Bounds())
}

func TestPNGsAreStoredLosslesslyAndKeepTransparency(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	img.Set(3, 3, color.NRGBA{10, 200, 30, 255})
	img.Set(4, 4, color.NRGBA{200, 10, 30, 0})
	res, err := imageproc.Prepare(context.Background(), pngOf(t, img))
	require.NoError(t, err)
	assert.Contains(t, string(res.Image), "VP8L", "lossless coding")
	out := decodeWebP(t, res.Image)
	_, _, _, a := out.At(4, 4).RGBA()
	assert.Zero(t, a)
	_, _, _, a = out.At(3, 3).RGBA()
	assert.Equal(t, uint32(0xffff), a)
}

func TestWebPAndGIFAndHEICAreAccepted(t *testing.T) {
	var wb bytes.Buffer
	require.NoError(t, webp.Encode(&wb, quadrants(30, 30), webp.Options{Quality: 80}))
	var gb bytes.Buffer
	require.NoError(t, gif.Encode(&gb, quadrants(30, 30), nil))
	heic, err := os.ReadFile("testdata/test8.heic")
	require.NoError(t, err)

	for name, data := range map[string][]byte{"webp": wb.Bytes(), "gif": gb.Bytes(), "heic": heic} {
		res, err := imageproc.Prepare(context.Background(), data)
		require.NoError(t, err, name)
		assert.Positive(t, res.Width, name)
		assert.NotEmpty(t, res.Thumb, name)
	}
}

func TestRefusedInput(t *testing.T) {
	_, err := imageproc.Prepare(context.Background(), []byte("hello world, this is text"))
	assert.ErrorIs(t, err, imageproc.ErrUnsupported)

	heic, err := os.ReadFile("testdata/test8.heic")
	require.NoError(t, err)
	_, err = imageproc.Prepare(context.Background(), heic[:40])
	assert.ErrorIs(t, err, imageproc.ErrInvalid)

	_, err = imageproc.Prepare(context.Background(), jpegOf(t, quadrants(40, 40))[:100])
	assert.ErrorIs(t, err, imageproc.ErrInvalid)
}

func TestAWaitingCallerGivesUpWithItsContext(t *testing.T) {
	release := make([]func(), 0, imageproc.MaxConcurrent)
	for range imageproc.MaxConcurrent {
		r, err := imageproc.Acquire(context.Background())
		require.NoError(t, err)
		release = append(release, r)
	}
	defer func() {
		for _, r := range release {
			r()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := imageproc.Prepare(ctx, jpegOf(t, quadrants(10, 10)))
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestExifIsFoundInPNGAndWebPContainers(t *testing.T) {
	exif := tiff(1, "2025:01:02 03:04:05")

	var wb bytes.Buffer
	require.NoError(t, webp.Encode(&wb, quadrants(16, 16), webp.Options{Quality: 80}))
	withChunk, err := imageproc.AddExifToWebP(wb.Bytes(), exif, 16, 16, false)
	require.NoError(t, err)
	assert.Equal(t, exif, imageproc.ExifOfWebP(withChunk))
	decodeWebP(t, withChunk)

	var lossless bytes.Buffer
	require.NoError(t, webp.Encode(&lossless, quadrants(16, 16), webp.Options{Lossless: true}))
	withChunk, err = imageproc.AddExifToWebP(lossless.Bytes(), exif, 16, 16, false)
	require.NoError(t, err)
	assert.Equal(t, exif, imageproc.ExifOfWebP(withChunk))
	decodeWebP(t, withChunk)

	var p bytes.Buffer
	require.NoError(t, png.Encode(&p, quadrants(8, 8)))
	data := p.Bytes()
	chunk := make([]byte, 0, len(exif)+12)
	chunk = binary.BigEndian.AppendUint32(chunk, uint32(len(exif)))
	chunk = append(chunk, "eXIf"...)
	chunk = append(chunk, exif...)
	chunk = binary.BigEndian.AppendUint32(chunk, 0)
	withExif := append(append(append([]byte{}, data[:33]...), chunk...), data[33:]...)
	assert.Equal(t, exif, imageproc.ExifOf(withExif))
}

func TestHEICExifIsReadFromTheContainer(t *testing.T) {
	exif := tiff(6, "2024:05:06 07:08:09")
	assert.Equal(t, exif, imageproc.ExifOf(syntheticHEIC(exif)))
	assert.Nil(t, imageproc.ExifOf(syntheticHEIC(nil)))
}

func TestAHEICExifLocationOutsideTheFileIsIgnored(t *testing.T) {
	ftyp := box("ftyp", []byte("heic"), []byte{0, 0, 0, 0})
	iinf := box("iinf", []byte{0, 0, 0, 0, 0, 1}, box("infe", []byte{2, 0, 0, 0, 0, 2, 0, 0}, []byte("Exif"), []byte{0}))
	for _, extent := range [][2]uint64{{^uint64(0) - 7, 16}, {16, ^uint64(0) - 7}} {
		// iloc version 1 with 8-byte offsets: the sum of offset and length wraps around.
		loc := []byte{1, 0, 0, 0, 0x88, 0x00}
		loc = binary.BigEndian.AppendUint16(loc, 1)
		loc = binary.BigEndian.AppendUint16(loc, 2)
		loc = binary.BigEndian.AppendUint16(loc, 0)
		loc = binary.BigEndian.AppendUint16(loc, 0)
		loc = binary.BigEndian.AppendUint16(loc, 1)
		loc = binary.BigEndian.AppendUint64(loc, extent[0])
		loc = binary.BigEndian.AppendUint64(loc, extent[1])
		data := append(ftyp, box("meta", []byte{0, 0, 0, 0}, iinf, box("iloc", loc))...)
		assert.NotPanics(t, func() { assert.Nil(t, imageproc.ExifOf(data)) })
	}
}

func box(typ string, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	return append(append(binary.BigEndian.AppendUint32(nil, uint32(8+len(body))), typ...), body...)
}

// syntheticHEIC is a container with the boxes that locate the Exif item and no image data.
func syntheticHEIC(exif []byte) []byte {
	infe := func(id uint16, typ string) []byte {
		p := []byte{2, 0, 0, 0}
		p = binary.BigEndian.AppendUint16(p, id)
		p = append(p, 0, 0)
		return box("infe", p, []byte(typ), []byte{0})
	}
	items := [][]byte{infe(1, "hvc1")}
	if exif != nil {
		items = append(items, infe(2, "Exif"))
	}
	iinf := box("iinf", []byte{0, 0, 0, 0, 0, byte(len(items))}, bytes.Join(items, nil))

	ftyp := box("ftyp", []byte("heic"), []byte{0, 0, 0, 0}, []byte("mif1heic"))
	mdatPayload := append(binary.BigEndian.AppendUint32(nil, 0), exif...)
	build := func(offset uint32) []byte {
		// iloc version 0: offset_size 4, length_size 4, base_offset_size 0
		loc := []byte{0, 0, 0, 0, 0x44, 0x00}
		if exif == nil {
			loc = binary.BigEndian.AppendUint16(loc, 0)
		} else {
			loc = binary.BigEndian.AppendUint16(loc, 1)
			loc = binary.BigEndian.AppendUint16(loc, 2)
			loc = binary.BigEndian.AppendUint16(loc, 0)
			loc = binary.BigEndian.AppendUint16(loc, 1)
			loc = binary.BigEndian.AppendUint32(loc, offset)
			loc = binary.BigEndian.AppendUint32(loc, uint32(len(mdatPayload)))
		}
		meta := box("meta", []byte{0, 0, 0, 0}, iinf, box("iloc", loc))
		return append(append(append([]byte{}, ftyp...), meta...), box("mdat", mdatPayload)...)
	}
	first := build(0)
	return build(uint32(len(first) - len(mdatPayload)))
}

func TestAvatarIsTheCropSquareScaledToTheAvatarSizes(t *testing.T) {
	src := quadrants(1000, 600) // the 600x600 square at x=400 shows the middle of all four colours
	photo, thumb := imageproc.Avatar(src, 0, 0, 600)
	assert.Equal(t, image.Rect(0, 0, 400, 400), photo.Bounds())
	assert.Equal(t, image.Rect(0, 0, 96, 96), thumb.Bounds())
	near(t, photo, 10, 10, color.RGBA{255, 0, 0, 255})
	near(t, photo, 390, 390, color.RGBA{255, 255, 255, 255})

	right, _ := imageproc.Avatar(src, 500, 0, 500)
	near(t, right, 10, 10, color.RGBA{0, 255, 0, 255})
	near(t, right, 10, 390, color.RGBA{255, 255, 255, 255})
}

func TestASmallCropIsNotEnlargedBeyondItsOwnSizeButThePhotoStaysSquare(t *testing.T) {
	photo, thumb := imageproc.Avatar(quadrants(100, 100), 0, 0, 100)
	assert.Equal(t, image.Rect(0, 0, 100, 100), photo.Bounds())
	assert.Equal(t, image.Rect(0, 0, 96, 96), thumb.Bounds())
}

func TestCenteredCropIsTheLargestSquareInTheMiddle(t *testing.T) {
	x, y, size := imageproc.CenteredCrop(1000, 600)
	assert.Equal(t, [3]int{200, 0, 600}, [3]int{x, y, size})
	x, y, size = imageproc.CenteredCrop(300, 900)
	assert.Equal(t, [3]int{0, 300, 300}, [3]int{x, y, size})
}

func ramp(w, h int, reversed bool) image.Image {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			v := uint8(x * 255 / (w - 1))
			if reversed {
				v = 255 - v
			}
			img.SetGray(x, y, color.Gray{v})
		}
	}
	return img
}

// @023-US4
func TestDHashFollowsTheBrightnessDirectionAndIgnoresTheSize(t *testing.T) {
	assert.Equal(t, uint64(0), imageproc.DHash(ramp(90, 90, false), 0, 0, 90))
	assert.Equal(t, ^uint64(0), imageproc.DHash(ramp(90, 90, true), 0, 0, 90))
	assert.Equal(t, imageproc.DHash(ramp(400, 400, true), 0, 0, 400), imageproc.DHash(ramp(60, 60, true), 0, 0, 60))
}

// @023-US4
func TestDHashLooksOnlyAtTheGivenSquare(t *testing.T) {
	wide := ramp(300, 100, true)
	assert.Equal(t, ^uint64(0), imageproc.DHash(wide, 100, 0, 100))
}
