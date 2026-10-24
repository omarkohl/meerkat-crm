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
	// AddPhoto stores data as a new image of the contact and makes it the main image, which also
	// cuts a new avatar from it. An image that cannot be used is domain.ErrInvalidPhoto.
	AddPhoto(ctx context.Context, actor domain.UserID, id domain.ContactID, data []byte) (domain.Photos, error)
}

type Files interface {
	Open(name string) (domain.File, error)
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
	files    Files
}

func New(contacts Contacts, files Files) *UseCases {
	return &UseCases{contacts: contacts, files: files}
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

// UploadPhoto adds src to the images of the actor's contact and makes it the main one. A nil
// src only checks the contact. The upload is converted before the transaction; only the avatar
// is cut inside it.
func (u *UseCases) UploadPhoto(ctx context.Context, actor domain.UserID, id domain.ContactID, src io.Reader) (domain.Photos, error) {
	current, err := u.contacts.Photos(ctx, actor, id, false)
	if err != nil {
		return domain.Photos{}, find(err)
	}
	if src == nil {
		return current, nil
	}
	data, err := io.ReadAll(src)
	if err != nil {
		return domain.Photos{}, &Failure{Stage: StageProcess, Err: err}
	}
	photos, err := u.contacts.AddPhoto(ctx, actor, id, data)
	if errors.Is(err, domain.ErrInvalidPhoto) || errors.Is(err, domain.ErrTooManyImages) {
		return domain.Photos{}, err
	}
	if err != nil {
		return domain.Photos{}, &Failure{Stage: StageUpdate, Err: err}
	}
	return photos, nil
}
