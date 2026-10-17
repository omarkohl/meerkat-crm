// Package domain is the vocabulary of the photos module: a contact's profile photo and
// thumbnail, and which stored names are safe to serve. It imports nothing but the standard
// library (docs/adr/0012-module-layout.md).
package domain

import (
	"encoding/base64"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"
)

type UserID uint

type ContactID uint

var (
	ErrContactNotFound       = errors.New("contact not found")
	ErrPhotoNotFound         = errors.New("photo not found")
	ErrImageMissing          = errors.New("image file missing")
	ErrThumbnailNotFound     = errors.New("thumbnail not found")
	ErrUnsafePhotoName       = errors.New("invalid photo path")
	ErrUnsafeThumbnailName   = errors.New("invalid thumbnail path")
	ErrBadLegacyThumbnail    = errors.New("invalid thumbnail format")
	ErrLegacyThumbnailDecode = errors.New("cannot decode thumbnail")
	ErrInvalidPhoto          = errors.New("invalid photo")
	ErrFileMissing           = errors.New("file does not exist or is not a regular file")
)

// Photos are the stored names of a contact's pictures. Legacy is the old base64 data URL of
// the thumbnail, kept until the thumbnail is exported to a file.
type Photos struct {
	ContactID ContactID
	Photo     string
	Thumbnail string
	Legacy    string
}

type File struct {
	io.ReadSeekCloser
	ModTime time.Time
	Size    int64
}

func SafePhotoName(name string) bool {
	return !strings.Contains(name, "..") && !filepath.IsAbs(name)
}

func SafeThumbnailName(name string) bool {
	return SafePhotoName(name) && !strings.ContainsAny(name, `/\`)
}

func LegacyImage(dataURL string) ([]byte, error) {
	if !strings.HasPrefix(dataURL, "data:") {
		return nil, ErrThumbnailNotFound
	}
	_, payload, ok := strings.Cut(dataURL, ",")
	if !ok {
		return nil, ErrBadLegacyThumbnail
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, errors.Join(ErrLegacyThumbnailDecode, err)
	}
	return data, nil
}
