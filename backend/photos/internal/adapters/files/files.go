// Package files keeps the picture files of the photos module on disk and turns uploaded
// images into a square photo and a thumbnail.
package files

import (
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"

	"meerkat/imageproc"
	"meerkat/photofile"
	"meerkat/photos/internal/app"
	"meerkat/photos/internal/domain"

	"github.com/google/uuid"
	"golang.org/x/image/draw"
)

const (
	maxPhotoSize  = 400
	thumbnailSize = 48
)

type Directory struct{ dir string }

func New(dir string) *Directory { return &Directory{dir: dir} }

var _ app.Files = (*Directory)(nil)

func (d *Directory) Open(name string) (domain.File, error) {
	file, err := os.Open(filepath.Join(d.dir, name)) //nolint:gosec // name checked by the domain, directory from config
	if err != nil {
		return domain.File{}, domain.ErrFileMissing
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return domain.File{}, domain.ErrFileMissing
	}
	return domain.File{ReadSeekCloser: file, ModTime: info.ModTime(), Size: info.Size()}, nil
}

func (d *Directory) Remove(names ...string) { photofile.Remove(d.dir, names...) }

func (d *Directory) Save(src io.Reader) (photo, thumbnail string, err error) {
	if err := photofile.EnsureDir(d.dir); err != nil {
		return "", "", fmt.Errorf("create upload directory: %w", err)
	}
	data, err := io.ReadAll(src)
	if err != nil {
		return "", "", err
	}
	img, err := decode(data)
	if err != nil {
		return "", "", err
	}
	img = cropToSquare(img)

	base := uuid.New().String()
	photo, thumbnail = base+"_photo.jpg", base+"_thumb.jpg"
	photoImg := img
	if b := img.Bounds(); b.Dx() > maxPhotoSize || b.Dy() > maxPhotoSize {
		photoImg = resizeTo(img, maxPhotoSize)
	}
	photoPath := filepath.Join(d.dir, photo)
	if err := photofile.SaveJPEG(photoPath, photoImg); err != nil {
		return "", "", err
	}
	if err := photofile.SaveJPEG(filepath.Join(d.dir, thumbnail), resizeTo(img, thumbnailSize)); err != nil {
		_ = os.Remove(photoPath)
		return "", "", err
	}
	return photo, thumbnail, nil
}

func decode(data []byte) (image.Image, error) {
	src, err := imageproc.Decode(data)
	if err != nil {
		return nil, invalid(err)
	}
	return src.Image, nil
}

func invalid(err error) error { return errors.Join(domain.ErrInvalidPhoto, err) }

func cropToSquare(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == h {
		return img
	}
	size := min(w, h)
	offsetX, offsetY := (w-size)/2, (h-size)/2
	cropped := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			cropped.Set(x, y, img.At(b.Min.X+offsetX+x, b.Min.Y+offsetY+y))
		}
	}
	return cropped
}

// resizeTo scales img to exactly size x size pixels. x/image/draw offers no Lanczos kernel;
// CatmullRom is the closest high-quality (bicubic) choice.
func resizeTo(img image.Image, size int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
	return dst
}
