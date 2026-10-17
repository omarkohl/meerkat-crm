package httpadapter

import (
	"context"
	"time"
)

type CurrentUserOutput struct {
	Body struct {
		ID                   uint      `json:"id"`
		Username             string    `json:"username"`
		Email                string    `json:"email"`
		Language             string    `json:"language"`
		DateFormat           string    `json:"date_format"`
		IsAdmin              bool      `json:"is_admin"`
		CreatedAt            time.Time `json:"created_at"`
		UpdatedAt            time.Time `json:"updated_at"`
		TwoFactorEnabled     bool      `json:"two_factor_enabled" doc:"Always false here; GET /users/me/2fa says whether two-factor is on"`
		CustomFieldNames     []string  `json:"custom_field_names" nullable:"true"`
		EnabledContactFields []string  `json:"enabled_contact_fields" nullable:"true"`
	}
}

func (h *handlers) currentUser(ctx context.Context, _ *struct{}) (*CurrentUserOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	p, err := h.uc.Profile(ctx, actor)
	if err != nil {
		return nil, accountError(err)
	}
	out := &CurrentUserOutput{}
	b := &out.Body
	b.ID, b.Username, b.Email, b.Language, b.DateFormat, b.IsAdmin = uint(p.ID), p.Username, p.Email, p.Language, p.DateFormat, p.IsAdmin
	b.CreatedAt, b.UpdatedAt = p.CreatedAt, p.UpdatedAt
	b.CustomFieldNames, b.EnabledContactFields = p.CustomFieldNames, p.EnabledContactFields
	return out, nil
}
