package models

import (
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"
)

// Image is a stored picture of a contact, an activity or a note. With no owner it is pending:
// uploaded, not yet saved with anything. Rows are deleted for good (no soft delete).
type Image struct {
	ID         uint  `gorm:"primarykey"`
	UserID     uint  `gorm:"not null;index"`
	ContactID  *uint `gorm:"index"`
	ActivityID *uint `gorm:"index"`
	NoteID     *uint `gorm:"index"`
	Position   int   `gorm:"not null;default:0"`
	// File and ThumbFile are relative to the photo directory.
	File      string `gorm:"not null"`
	ThumbFile string `gorm:"not null"`
	Mime      string `gorm:"not null"`
	Width     int    `gorm:"not null"`
	Height    int    `gorm:"not null"`
	Bytes     int64  `gorm:"not null"`
	TakenAt   *time.Time
	// Crop is the square face crop in pixels of File; contacts only.
	CropX, CropY, CropSize *int
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// ImageCrop is a square in pixels of the full image.
type ImageCrop struct {
	X    int `json:"x"`
	Y    int `json:"y"`
	Size int `json:"size"`
}

// ImageRef is how an image appears in API responses.
type ImageRef struct {
	ID       uint       `json:"id"`
	Width    int        `json:"width"`
	Height   int        `json:"height"`
	Mime     string     `json:"mime"`
	Bytes    int64      `json:"bytes"`
	TakenAt  *time.Time `json:"taken_at"`
	Crop     *ImageCrop `json:"crop"`
	URL      string     `json:"url"`
	ThumbURL string     `json:"thumb_url"`
}

// ImageVersion is the unique part of the file name. It is in the URL, so the URL of an image
// never serves other bytes and can be cached for good.
func ImageVersion(file string) string {
	base := path.Base(file)
	return strings.TrimSuffix(strings.TrimSuffix(base, path.Ext(base)), "_image")
}

// ImageURL is the download URL of one variant ("image" or "thumb") of an image.
func ImageURL(id uint, file, variant string) string {
	return fmt.Sprintf("/api/v1/images/%d/%s?v=%s", id, variant, url.QueryEscape(ImageVersion(file)))
}

func (i Image) Ref() ImageRef {
	ref := ImageRef{
		ID: i.ID, Width: i.Width, Height: i.Height, Mime: i.Mime, Bytes: i.Bytes, TakenAt: i.TakenAt,
		URL:      ImageURL(i.ID, i.File, "image"),
		ThumbURL: ImageURL(i.ID, i.File, "thumb"),
	}
	if i.CropX != nil && i.CropY != nil && i.CropSize != nil {
		ref.Crop = &ImageCrop{X: *i.CropX, Y: *i.CropY, Size: *i.CropSize}
	}
	return ref
}
