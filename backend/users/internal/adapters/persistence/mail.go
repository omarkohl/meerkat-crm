package persistence

import (
	"context"

	"meerkat/api"
	"meerkat/config"
	"meerkat/services"
	"meerkat/users/internal/app"
	"meerkat/users/internal/domain"

	"gorm.io/gorm"
)

// Mail queues the reset mail in the e-mail outbox through the service the other mail flows use.
type Mail struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewMail(db *gorm.DB, cfg *config.Config) *Mail { return &Mail{db: db, cfg: cfg} }

var _ app.ResetMail = (*Mail)(nil)

func (m *Mail) Send(ctx context.Context, to domain.ResetTarget, token string) error {
	return services.SendPasswordResetEmail(m.db.WithContext(ctx), uint(to.ID), to.Email, token, to.Language, m.cfg, api.Clock(ctx))
}
