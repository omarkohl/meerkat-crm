// Package domain is the vocabulary of the images module: a stored picture, its variants and
// the limits. It imports nothing but the standard library (docs/adr/0012-module-layout.md).
package domain

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"
)

type UserID uint

type ImageID uint

const (
	MaxUploadBytes = 25 << 20
	MaxPerNote     = 20
	MaxPerActivity = 20
	MaxPerContact  = 50
	// PendingTTL is how long an upload that no owner claimed is kept.
	PendingTTL = 24 * time.Hour
)

var (
	ErrNotFound     = errors.New("image not found")
	ErrTooLarge     = errors.New("image file too large")
	ErrInvalidImage = errors.New("invalid image")
	ErrFileMissing  = errors.New("image file missing")
	ErrUnsafeName   = errors.New("invalid image path")
)

type Variant string

const (
	VariantImage Variant = "image"
	VariantThumb Variant = "thumb"
)

func ParseVariant(s string) (Variant, bool) {
	v := Variant(s)
	return v, v == VariantImage || v == VariantThumb
}

// Stored is a processed picture on disk. File names are relative to the photo directory.
type Stored struct {
	File, ThumbFile, Mime string
	Width, Height         int
	Bytes                 int64
	TakenAt               *time.Time
}

// Crop is the square face crop of a contact's image, in pixels of the full image.
type Crop struct{ X, Y, Size int }

type Image struct {
	ID ImageID
	Stored
	Crop *Crop
}

func Extension(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	}
	return ".webp"
}

func (i Image) FileOf(v Variant) string {
	if v == VariantThumb {
		return i.ThumbFile
	}
	return i.File
}

type File struct {
	io.ReadSeekCloser
	ModTime time.Time
	Size    int64
}

func SafeName(name string) bool {
	return strings.HasPrefix(name, "images/") && !strings.Contains(name, "..") && !filepath.IsAbs(name)
}
