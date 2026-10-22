// Package app holds the use cases of the images module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"io"
	"time"

	"meerkat/images/internal/domain"
)

type Store interface {
	Create(ctx context.Context, actor domain.UserID, s domain.Stored) (domain.Image, error)
	Get(ctx context.Context, actor domain.UserID, id domain.ImageID) (domain.Image, error)
	// DeletePendingBefore deletes the images with no owner created before t and returns the number of rows and their file names.
	DeletePendingBefore(ctx context.Context, t time.Time) (int, []string, error)
	// Files are the file names of every image row, of every user.
	Files(ctx context.Context) (map[string]bool, error)
	// SetContactImages makes ids the ordered images of the contact, keeps its main image and avatar right,
	// and returns the images. Ids may be pending or on another contact of the actor.
	SetContactImages(ctx context.Context, actor domain.UserID, contact domain.ContactID, ids []uint) ([]domain.Image, error)
	// SetMainImage makes an image of the contact its main image and cuts the avatar from the crop.
	SetMainImage(ctx context.Context, actor domain.UserID, contact domain.ContactID, image domain.ImageID, crop domain.Crop) (domain.Image, error)
}

type Files interface {
	// Save makes the stored variants of an uploaded picture. A picture that cannot be used is domain.ErrInvalidImage.
	Save(ctx context.Context, actor domain.UserID, data []byte) (domain.Stored, error)
	Open(name string) (domain.File, error)
	Remove(names ...string)
	// Sweep deletes the files under the images directory that are not in keep and were last written before t.
	Sweep(keep map[string]bool, t time.Time) int
}

type UseCases struct {
	store Store
	files Files
}

func New(store Store, files Files) *UseCases { return &UseCases{store: store, files: files} }

// Upload stores a picture as a pending image: it belongs to no note, activity or contact yet.
func (u *UseCases) Upload(ctx context.Context, actor domain.UserID, src io.Reader) (domain.Image, error) {
	data, err := io.ReadAll(io.LimitReader(src, domain.MaxUploadBytes+1))
	if err != nil {
		return domain.Image{}, err
	}
	if len(data) > domain.MaxUploadBytes {
		return domain.Image{}, domain.ErrTooLarge
	}
	stored, err := u.files.Save(ctx, actor, data)
	if err != nil {
		return domain.Image{}, err
	}
	img, err := u.store.Create(ctx, actor, stored)
	if err != nil {
		u.files.Remove(stored.File, stored.ThumbFile)
		return domain.Image{}, err
	}
	return img, nil
}

// Open returns an image of the actor and one of its files, also for trashed owners.
func (u *UseCases) Open(ctx context.Context, actor domain.UserID, id domain.ImageID, v domain.Variant) (domain.Image, domain.File, error) {
	img, err := u.store.Get(ctx, actor, id)
	if err != nil {
		return domain.Image{}, domain.File{}, err
	}
	name := img.FileOf(v)
	if !domain.SafeName(name) {
		return domain.Image{}, domain.File{}, domain.ErrUnsafeName
	}
	f, err := u.files.Open(name)
	if err != nil {
		return domain.Image{}, domain.File{}, domain.ErrFileMissing
	}
	return img, f, nil
}

// SetContactImages replaces the images of a contact by the listed ones, in that order.
func (u *UseCases) SetContactImages(ctx context.Context, actor domain.UserID, contact domain.ContactID, ids []uint) ([]domain.Image, error) {
	return u.store.SetContactImages(ctx, actor, contact, ids)
}

// SetMainImage makes an image of the contact its main image with the given face crop.
func (u *UseCases) SetMainImage(ctx context.Context, actor domain.UserID, contact domain.ContactID, image domain.ImageID, crop domain.Crop) (domain.Image, error) {
	return u.store.SetMainImage(ctx, actor, contact, image, crop)
}

type SweepResult struct{ Pending, Orphans int }

// Sweep is the safety net for every crash path: it drops pending images that nobody claimed
// within PendingTTL, then files no row refers to. The same age protects uploads in progress.
func (u *UseCases) Sweep(ctx context.Context, now time.Time) (SweepResult, error) {
	cutoff := now.Add(-domain.PendingTTL)
	rows, names, err := u.store.DeletePendingBefore(ctx, cutoff)
	if err != nil {
		return SweepResult{}, err
	}
	u.files.Remove(names...)
	keep, err := u.store.Files(ctx)
	if err != nil {
		return SweepResult{Pending: rows}, err
	}
	return SweepResult{Pending: rows, Orphans: u.files.Sweep(keep, cutoff)}, nil
}
