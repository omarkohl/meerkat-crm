// Package persistence implements the ports of the webhooks module on top of GORM and the
// existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md), and the place where every query is scoped to the actor.
// Delivering a payload is done by the services, which every module's events share.
package persistence

import (
	"context"
	"errors"

	"meerkat/clock"
	"meerkat/config"
	"meerkat/models"
	"meerkat/services"
	"meerkat/webhooks/internal/app"
	"meerkat/webhooks/internal/domain"

	"gorm.io/gorm"
)

type Repository struct {
	db  *gorm.DB
	cfg config.Config
}

func New(db *gorm.DB, cfg config.Config) *Repository { return &Repository{db: db, cfg: cfg} }

var (
	_ app.Webhooks = (*Repository)(nil)
	_ app.Sender   = (*Repository)(nil)
)

func webhook(w models.Webhook) domain.Webhook {
	return domain.Webhook{ID: domain.WebhookID(w.ID), Name: w.Name, URL: w.URL, Events: w.Events, Active: w.IsActive, CreatedAt: w.CreatedAt}
}

func (r *Repository) find(ctx context.Context, actor domain.UserID, id domain.WebhookID) (models.Webhook, error) {
	var w models.Webhook
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", uint(id), uint(actor)).First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return w, domain.ErrNotFound
	}
	return w, err
}

func (r *Repository) List(ctx context.Context, actor domain.UserID) ([]domain.Webhook, error) {
	var rows []models.Webhook
	if err := r.db.WithContext(ctx).Where("user_id = ?", uint(actor)).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Webhook, len(rows))
	for i, w := range rows {
		out[i] = webhook(w)
	}
	return out, nil
}

func (r *Repository) Count(ctx context.Context, actor domain.UserID) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Webhook{}).Where("user_id = ? AND deleted_at IS NULL", uint(actor)).Count(&n).Error
	return int(n), err
}

func (r *Repository) Create(ctx context.Context, actor domain.UserID, in domain.Input, secret string) (domain.Webhook, error) {
	row := models.Webhook{UserID: uint(actor), Name: in.Name, URL: in.URL, Events: in.Events, Secret: secret, IsActive: in.Active}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return domain.Webhook{}, err
	}
	return webhook(row), nil
}

func (r *Repository) Find(ctx context.Context, actor domain.UserID, id domain.WebhookID) (domain.Webhook, error) {
	w, err := r.find(ctx, actor, id)
	return webhook(w), err
}

func (r *Repository) Update(ctx context.Context, actor domain.UserID, id domain.WebhookID, in domain.Input) (domain.Webhook, error) {
	w, err := r.find(ctx, actor, id)
	if err != nil {
		return domain.Webhook{}, err
	}
	w.Name, w.URL, w.Events, w.IsActive = in.Name, in.URL, in.Events, in.Active
	if err := r.db.WithContext(ctx).Save(&w).Error; err != nil {
		return domain.Webhook{}, err
	}
	return webhook(w), nil
}

func (r *Repository) Delete(ctx context.Context, actor domain.UserID, id domain.WebhookID) error {
	w, err := r.find(ctx, actor, id)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Delete(&w).Error
}

func (r *Repository) Deliveries(ctx context.Context, actor domain.UserID, id domain.WebhookID) ([]domain.Delivery, error) {
	w, err := r.find(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	var rows []models.WebhookDelivery
	if err := r.db.WithContext(ctx).Where("webhook_id = ?", w.ID).Order("created_at DESC").Limit(50).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Delivery, len(rows))
	for i, d := range rows {
		out[i] = delivery(d)
	}
	return out, nil
}

func delivery(d models.WebhookDelivery) domain.Delivery {
	return domain.Delivery{
		ID: d.ID, WebhookID: domain.WebhookID(d.WebhookID), EventType: d.EventType, StatusCode: d.StatusCode, Error: d.Error,
		Attempts: d.Attempts, Status: d.Status, NextRetryAt: d.NextRetryAt, CreatedAt: d.CreatedAt,
	}
}

func (r *Repository) SendTest(ctx context.Context, actor domain.UserID, id domain.WebhookID, clk clock.Clock) (domain.Delivery, error) {
	w, err := r.find(ctx, actor, id)
	if err != nil {
		return domain.Delivery{}, err
	}
	return delivery(services.TestWebhookDelivery(r.db.WithContext(ctx), r.cfg, w, clk)), nil
}
