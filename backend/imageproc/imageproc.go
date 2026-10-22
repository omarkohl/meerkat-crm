// Package imageproc decodes uploaded pictures of any supported format, turns them upright,
// scales them and encodes the stored variants. It is the one decoder of the application.
package imageproc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"time"

	"meerkat/photofile"

	"github.com/gen2brain/heic"
	"github.com/gen2brain/webp"
	xdraw "golang.org/x/image/draw"
)

const (
	MaxLongEdge   = 2048
	ThumbLongEdge = 480

	imageQuality = 82
	thumbQuality = 75
	// MaxConcurrent bounds the pictures decoded at the same time. A 48 MP HEIC needs about 200 MB.
	MaxConcurrent = 2
)

var (
	ErrUnsupported = errors.New("unsupported image format")
	ErrInvalid     = errors.New("image cannot be read")
	ErrTooLarge    = photofile.ErrTooLarge
)

var slots = make(chan struct{}, MaxConcurrent)

// Acquire waits for one of the decoding slots. The caller calls the returned function when
// its pictures are no longer in memory.
func Acquire(ctx context.Context) (release func(), err error) {
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Source is a decoded picture with the metadata that tells how to show it.
type Source struct {
	Image image.Image
	Exif  []byte
	// Lossless is true when the original was a PNG or has transparency.
	Lossless bool
}

func sniff(data []byte) string {
	switch ct := http.DetectContentType(data[:min(len(data), 512)]); {
	case ct == "image/jpeg", ct == "image/png", ct == "image/gif", ct == "image/webp":
		return ct
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		switch string(data[8:12]) {
		case "heic", "heix", "hevc", "hevx", "mif1", "heim", "heis", "msf1":
			return "image/heic"
		}
	}
	return ""
}

// Decode reads a JPEG, PNG, GIF (first frame), WebP or HEIC picture. Pixels are not rotated.
func Decode(data []byte) (*Source, error) {
	kind := sniff(data)
	if kind == "" {
		return nil, ErrUnsupported
	}
	if err := checkSize(kind, data); err != nil {
		return nil, err
	}
	var img image.Image
	var err error
	r := bytes.NewReader(data)
	switch kind {
	case "image/jpeg":
		img, err = jpeg.Decode(r)
	case "image/png":
		img, err = png.Decode(r)
	case "image/gif":
		img, err = gif.Decode(r)
	case "image/webp":
		img, err = webp.Decode(r)
	default:
		img, err = heic.Decode(r)
	}
	if err != nil {
		return nil, errors.Join(ErrInvalid, err)
	}
	return &Source{Image: img, Exif: ExifOf(data), Lossless: kind == "image/png" || !opaque(img)}, nil
}

func checkSize(kind string, data []byte) error {
	var w, h int
	var err error
	switch kind {
	case "image/webp":
		var cfg image.Config
		cfg, err = webp.DecodeConfig(bytes.NewReader(data))
		w, h = cfg.Width, cfg.Height
	case "image/heic":
		var cfg image.Config
		cfg, err = heic.DecodeConfig(bytes.NewReader(data))
		w, h = cfg.Width, cfg.Height
	default:
		if err := photofile.CheckDimensions(data); err != nil {
			if errors.Is(err, photofile.ErrTooLarge) {
				return ErrTooLarge
			}
			return errors.Join(ErrInvalid, err)
		}
		return nil
	}
	if err != nil {
		return errors.Join(ErrInvalid, err)
	}
	if photofile.CheckSize(w, h) != nil {
		return ErrTooLarge
	}
	return nil
}

func opaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return true
}

// Fit scales img down so that its long edge is at most long. It never enlarges.
func Fit(img image.Image, long int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if max(w, h) <= long {
		return img
	}
	nw, nh := max(1, w*long/max(w, h)), max(1, h*long/max(w, h))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}

// Orient applies an EXIF orientation (1 to 8) to the pixels.
func Orient(img image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	dw, dh := w, h
	if orientation >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range h {
		for x := range w {
			var dx, dy int
			switch orientation {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			copy(dst.Pix[dy*dst.Stride+dx*4:dy*dst.Stride+dx*4+4], src.Pix[y*src.Stride+x*4:y*src.Stride+x*4+4])
		}
	}
	return dst
}

func straight(img image.Image) image.Image {
	if opaque(img) {
		return img
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

func EncodeWebP(img image.Image, lossless bool, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := webp.Encode(&buf, straight(img), webp.Options{Quality: quality, Lossless: lossless}); err != nil {
		return nil, fmt.Errorf("encode webp: %w", err)
	}
	return buf.Bytes(), nil
}

// Result is a picture ready to store: the full variant with its EXIF block, and a thumbnail.
type Result struct {
	Image, Thumb  []byte
	Width, Height int
	Mime          string
	TakenAt       *time.Time
}

// Prepare turns an upload into the stored variants. At most MaxConcurrent calls decode at the
// same time; others wait until their context ends.
func Prepare(ctx context.Context, data []byte) (*Result, error) {
	release, err := Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	src, err := Decode(data)
	if err != nil {
		return nil, err
	}
	info := ParseExif(src.Exif)
	// Scale first: rotating the small picture is much cheaper than the sensor-sized one.
	img := Orient(Fit(src.Image, MaxLongEdge), info.Orientation)
	b := img.Bounds()

	full, err := EncodeWebP(img, src.Lossless, imageQuality)
	if err != nil {
		return nil, err
	}
	if len(src.Exif) > 0 {
		withExif, err := AddExifToWebP(full, ResetOrientation(src.Exif), b.Dx(), b.Dy(), !opaque(img))
		if err != nil {
			return nil, err
		}
		full = withExif
	}
	thumb, err := EncodeWebP(Fit(img, ThumbLongEdge), false, thumbQuality)
	if err != nil {
		return nil, err
	}
	return &Result{Image: full, Thumb: thumb, Width: b.Dx(), Height: b.Dy(), Mime: "image/webp", TakenAt: info.TakenAt}, nil
}

const (
	AvatarSize      = 400
	AvatarThumbSize = 96
)

// CenteredCrop is the largest square in the middle of a w x h picture.
func CenteredCrop(w, h int) (x, y, size int) {
	size = min(w, h)
	return (w - size) / 2, (h - size) / 2, size
}

// Avatar cuts the square at (x, y, size) out of img. The photo is at most AvatarSize wide and is not
// enlarged; the thumbnail is always AvatarThumbSize square.
func Avatar(img image.Image, x, y, size int) (photo, thumb image.Image) {
	b := img.Bounds()
	square := image.Rect(b.Min.X+x, b.Min.Y+y, b.Min.X+x+size, b.Min.Y+y+size)
	scale := func(side int) image.Image {
		dst := image.NewRGBA(image.Rect(0, 0, side, side))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, square, xdraw.Src, nil)
		return dst
	}
	return scale(min(size, AvatarSize)), scale(AvatarThumbSize)
}
