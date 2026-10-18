// Package persistence implements the ports of the tokens module on top of GORM and the
// existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md), and the place where every query is scoped to the actor.
package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"meerkat/logger"
	"meerkat/models"
	"meerkat/services"
	"meerkat/tokens/internal/app"
	"meerkat/tokens/internal/domain"

	"gorm.io/gorm"
)

type Tokens struct{ db *gorm.DB }

func NewTokens(db *gorm.DB) *Tokens { return &Tokens{db: db} }

var _ app.Tokens = (*Tokens)(nil)

func apiToken(t models.ApiToken) domain.APIToken {
	return domain.APIToken{ID: domain.TokenID(t.ID), Name: t.Name, CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, RevokedAt: t.RevokedAt}
}

func (r *Tokens) List(ctx context.Context, actor domain.UserID) ([]domain.APIToken, error) {
	var rows []models.ApiToken
	if err := r.db.WithContext(ctx).Where("user_id = ?", uint(actor)).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.APIToken, len(rows))
	for i, t := range rows {
		out[i] = apiToken(t)
	}
	return out, nil
}

func (r *Tokens) Create(ctx context.Context, actor domain.UserID, name, hash string) (domain.APIToken, error) {
	row := models.ApiToken{UserID: uint(actor), Name: name, TokenHash: hash}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return domain.APIToken{}, err
	}
	return apiToken(row), nil
}

func (r *Tokens) Revoke(ctx context.Context, actor domain.UserID, id domain.TokenID, now time.Time) (domain.APIToken, error) {
	db := r.db.WithContext(ctx)
	var row models.ApiToken
	if err := db.Where("id = ? AND user_id = ?", uint(id), uint(actor)).First(&row).Error; err != nil {
		return domain.APIToken{}, domain.ErrTokenNotFound
	}
	if err := db.Model(&row).Update("revoked_at", now).Error; err != nil {
		return domain.APIToken{}, err
	}
	return apiToken(row), nil
}

type Feeds struct{ db *gorm.DB }

func NewFeeds(db *gorm.DB) *Feeds { return &Feeds{db: db} }

var _ app.Feeds = (*Feeds)(nil)

func feed(f models.IcsFeed) domain.Feed {
	return domain.Feed{ID: domain.FeedID(f.ID), Owner: domain.UserID(f.UserID), TokenHash: f.TokenHash, CreatedAt: f.CreatedAt, LastFetchedAt: f.LastFetchedAt}
}

func (r *Feeds) Find(ctx context.Context, actor domain.UserID) (*domain.Feed, error) {
	var row models.IcsFeed
	err := r.db.WithContext(ctx).Where("user_id = ?", uint(actor)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f := feed(row)
	return &f, nil
}

// Replace retries once: two concurrent requests can both pass the delete and then collide on
// the unique user_id, and the later one should win.
func (r *Feeds) Replace(ctx context.Context, actor domain.UserID, hash string) (domain.Feed, error) {
	var row models.IcsFeed
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		row = models.IcsFeed{UserID: uint(actor), TokenHash: hash}
		err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("user_id = ?", uint(actor)).Delete(&models.IcsFeed{}).Error; err != nil {
				return err
			}
			return tx.Create(&row).Error
		})
		if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed") {
			break
		}
	}
	if err != nil {
		return domain.Feed{}, err
	}
	return feed(row), nil
}

func (r *Feeds) Delete(ctx context.Context, actor domain.UserID) error {
	return r.db.WithContext(ctx).Where("user_id = ?", uint(actor)).Delete(&models.IcsFeed{}).Error
}

// ByHash looks past users that were soft-deleted: the feed row outlives its owner.
func (r *Feeds) ByHash(ctx context.Context, hash string) (domain.Feed, error) {
	db := r.db.WithContext(ctx)
	var row models.IcsFeed
	if err := db.Where("token_hash = ?", hash).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Feed{}, domain.ErrFeedNotFound
		}
		return domain.Feed{}, err
	}
	var owner models.User
	if err := db.Select("id", "language").First(&owner, row.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Feed{}, domain.ErrFeedNotFound
		}
		return domain.Feed{}, err
	}
	f := feed(row)
	f.Language = owner.Language
	return f, nil
}

func (r *Feeds) Render(ctx context.Context, owner domain.UserID, language string) (string, error) {
	return services.BuildICSFeed(r.db.WithContext(ctx), uint(owner), language)
}

// RecordFetch puts the throttle in the UPDATE, so concurrent fetches write at most once.
func (r *Feeds) RecordFetch(ctx context.Context, id domain.FeedID, at time.Time) {
	now := at.UTC()
	err := r.db.WithContext(ctx).Model(&models.IcsFeed{}).
		Where("id = ? AND (last_fetched_at IS NULL OR last_fetched_at < ?)", uint(id), now.Add(-domain.FetchThrottle)).
		UpdateColumn("last_fetched_at", now).Error
	if err != nil {
		logger.Logger.Warn().Err(err).Uint("ics_feed_id", uint(id)).Msg("Failed to update ics feed last_fetched_at")
	}
}

type Audit struct{ db *gorm.DB }

func NewAudit(db *gorm.DB) *Audit { return &Audit{db: db} }

var _ app.Audit = (*Audit)(nil)

func (a *Audit) Record(ctx context.Context, actor domain.UserID, event, detail string, client domain.Client) {
	services.WriteUserAudit(a.db.WithContext(ctx), uint(actor), event, detail, client.IP, client.UserAgent)
}
