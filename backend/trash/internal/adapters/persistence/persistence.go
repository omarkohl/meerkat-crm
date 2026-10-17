// Package persistence implements the ports of the trash module on top of GORM and the
// existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md), and the place where every query is scoped to the actor.
// Restoring and purging are done by the services, which the contact sync shares until it is a
// module itself.
package persistence

import (
	"context"
	"errors"
	"time"

	"meerkat/models"
	"meerkat/services"
	"meerkat/trash/internal/app"
	"meerkat/trash/internal/domain"

	"gorm.io/gorm"
)

type Repository struct {
	db       *gorm.DB
	photoDir string
}

func New(db *gorm.DB, photoDir string) *Repository { return &Repository{db: db, photoDir: photoDir} }

var _ app.Trash = (*Repository)(nil)

func (r *Repository) List(ctx context.Context, actor domain.UserID) ([]domain.TrashedContact, error) {
	found, err := services.ListTrashedContacts(r.db.WithContext(ctx), uint(actor))
	out := make([]domain.TrashedContact, len(found))
	for i, c := range found {
		out[i] = domain.TrashedContact{
			ID: domain.ContactID(c.ID), Firstname: c.Firstname, Lastname: c.Lastname, Nickname: c.Nickname, Email: c.Email,
			PhotoThumbnail: models.ThumbnailURL(c.ID, c.PhotoThumbnailFile), DeletedAt: c.DeletedAt.Time,
		}
	}
	return out, err
}

func (r *Repository) Restore(ctx context.Context, actor domain.UserID, id domain.ContactID) (app.Restored, error) {
	c, err := services.RestoreTrashedContact(r.db.WithContext(ctx), uint(actor), uint(id))
	if errors.Is(err, services.ErrTrashedContactNotFound) {
		return app.Restored{}, domain.ErrNotInTrash
	}
	if err != nil {
		return app.Restored{}, err
	}
	return app.Restored{ID: domain.ContactID(c.ID), Event: c}, nil
}

func (r *Repository) Purge(ctx context.Context, actor domain.UserID, ids []domain.ContactID, now time.Time) (int, error) {
	raw := make([]uint, len(ids))
	for i, id := range ids {
		raw[i] = uint(id)
	}
	return services.PurgeTrashedContacts(r.db.WithContext(ctx), uint(actor), raw, r.photoDir, now)
}
