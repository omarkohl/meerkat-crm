// Package app holds the use cases of the photos module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"
	"io"

	"meerkat/photos/internal/domain"
)

type Contacts interface {
	Photos(ctx context.Context, actor domain.UserID, id domain.ContactID, trashed bool) (domain.Photos, error)
}

type Tx interface {
	Photos(ctx context.Context, actor domain.UserID, id domain.ContactID, trashed bool) (domain.Photos, error)
	// Replace points the contact at new pictures and drops the legacy database thumbnail.
	Replace(ctx context.Context, actor domain.UserID, id domain.ContactID, photo, thumbnail string) error
}

type Store interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

type Files interface {
	Open(name string) (domain.File, error)
	// Save makes a photo and a thumbnail from an uploaded image and returns their names.
	// An image that cannot be used is domain.ErrInvalidPhoto.
	Save(src io.Reader) (photo, thumbnail string, err error)
	Remove(names ...string)
}

type Stage string

const (
	StageFind    Stage = "find"
	StageProcess Stage = "process"
	StageUpdate  Stage = "update"
)

// Failure wraps an infrastructure error with the stage it happened in. Everything the user
// can fix or see as "not found" is a domain error.
type Failure struct {
	Stage Stage
	Err   error
}

func (f *Failure) Error() string { return string(f.Stage) + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

type UseCases struct {
	contacts Contacts
	store    Store
	files    Files
}

func New(contacts Contacts, store Store, files Files) *UseCases {
	return &UseCases{contacts: contacts, store: store, files: files}
}

func find(err error) error {
	var f *Failure
	if errors.Is(err, domain.ErrContactNotFound) || errors.As(err, &f) {
		return err
	}
	return &Failure{Stage: StageFind, Err: err}
}

func (u *UseCases) OpenPhoto(ctx context.Context, actor domain.UserID, id domain.ContactID) (domain.Photos, domain.File, error) {
	p, err := u.contacts.Photos(ctx, actor, id, false)
	if err != nil {
		return domain.Photos{}, domain.File{}, find(err)
	}
	if p.Photo == "" {
		return domain.Photos{}, domain.File{}, domain.ErrPhotoNotFound
	}
	if !domain.SafePhotoName(p.Photo) {
		return domain.Photos{}, domain.File{}, domain.ErrUnsafePhotoName
	}
	f, err := u.files.Open(p.Photo)
	if err != nil {
		return domain.Photos{}, domain.File{}, domain.ErrImageMissing
	}
	return p, f, nil
}

// Thumbnail is a contact's thumbnail: a file, or for a contact not exported yet the image
// decoded from the legacy database copy.
type Thumbnail struct {
	Name   string
	File   domain.File
	Legacy []byte
}

// OpenThumbnail includes trashed contacts: the trash lists avatars.
func (u *UseCases) OpenThumbnail(ctx context.Context, actor domain.UserID, id domain.ContactID) (Thumbnail, error) {
	p, err := u.contacts.Photos(ctx, actor, id, true)
	if err != nil {
		return Thumbnail{}, find(err)
	}
	if p.Thumbnail == "" {
		data, err := domain.LegacyImage(p.Legacy)
		if errors.Is(err, domain.ErrLegacyThumbnailDecode) {
			return Thumbnail{}, &Failure{Stage: StageProcess, Err: err}
		}
		return Thumbnail{Legacy: data}, err
	}
	if !domain.SafeThumbnailName(p.Thumbnail) {
		return Thumbnail{}, domain.ErrUnsafeThumbnailName
	}
	f, err := u.files.Open(p.Thumbnail)
	if err != nil {
		return Thumbnail{}, domain.ErrThumbnailNotFound
	}
	return Thumbnail{Name: p.Thumbnail, File: f}, nil
}

// UploadPhoto replaces the actor's contact's picture with one made from src. A nil src
// only checks the contact. The image is processed before the transaction, which only swaps
// the names, so the database is not locked while pictures are written.
func (u *UseCases) UploadPhoto(ctx context.Context, actor domain.UserID, id domain.ContactID, src io.Reader) (domain.Photos, error) {
	current, err := u.contacts.Photos(ctx, actor, id, false)
	if err != nil {
		return domain.Photos{}, find(err)
	}
	if src == nil {
		return current, nil
	}
	photo, thumbnail, err := u.files.Save(src)
	if errors.Is(err, domain.ErrInvalidPhoto) {
		return domain.Photos{}, err
	}
	if err != nil {
		return domain.Photos{}, &Failure{Stage: StageProcess, Err: err}
	}
	err = u.store.InTx(ctx, func(tx Tx) error {
		var err error
		current, err = tx.Photos(ctx, actor, id, false)
		if err != nil {
			return find(err)
		}
		if err := tx.Replace(ctx, actor, id, photo, thumbnail); err != nil {
			return &Failure{Stage: StageUpdate, Err: err}
		}
		return nil
	})
	if err != nil {
		u.files.Remove(photo, thumbnail)
		return domain.Photos{}, err
	}
	u.files.Remove(current.Photo, current.Thumbnail)
	return domain.Photos{ContactID: id, Photo: photo, Thumbnail: thumbnail}, nil
}
