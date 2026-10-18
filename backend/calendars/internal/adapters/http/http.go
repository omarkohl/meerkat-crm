// Package httpadapter exposes the calendars use cases as huma operations. It maps HTTP to the
// use cases and their results and errors back to the responses the API always sent
// (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"time"

	"meerkat/api"
	"meerkat/calendars/internal/app"
	"meerkat/calendars/internal/domain"
	apperrors "meerkat/errors"
	"meerkat/idutil"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	tags := []string{"calendars"}
	op := func(id, method, path, summary string, status int) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary, Tags: tags, DefaultStatus: status}
	}
	api.RegisterOn(a, group, op("list-calendar-subscriptions", http.MethodGet, "/calendars", "The user's calendar subscriptions, oldest first", 0), h.list)
	api.RegisterOn(a, group, op("create-calendar-subscription", http.MethodPost, "/calendars", "Subscribe to a CalDAV or iCS calendar", http.StatusCreated), h.create)
	api.RegisterOn(a, group, op("update-calendar-subscription", http.MethodPut, "/calendars/{id}", "Change a subscription; a stored password does not follow a new server address", 0), h.update)
	api.RegisterOn(a, group, op("delete-calendar-subscription", http.MethodDelete, "/calendars/{id}", "Delete a subscription; the activities it imported stay", 0), h.delete)
	api.RegisterOn(a, group, op("sync-calendar-subscription", http.MethodPost, "/calendars/{id}/sync", "Import the calendar's events now", 0), h.sync)
}

type handlers struct{ uc *app.UseCases }

type CalendarSubscriptionResponse struct {
	ID             uint       `json:"id"`
	Name           string     `json:"name"`
	URL            string     `json:"url"`
	Username       string     `json:"username"`
	HasPassword    bool       `json:"has_password"`
	SyncEnabled    bool       `json:"sync_enabled"`
	PastDays       int        `json:"past_days"`
	FutureDays     int        `json:"future_days"`
	LastSyncedAt   *time.Time `json:"last_synced_at" nullable:"true"`
	LastSyncStatus string     `json:"last_sync_status" doc:"Empty, success or error"`
	LastSyncError  string     `json:"last_sync_error"`
	CreatedAt      time.Time  `json:"created_at"`
}

type CalendarSubscriptionBody struct {
	Name          string `json:"name" validate:"required,min=1,max=100"`
	URL           string `json:"url" validate:"required,url,max=2048"`
	Username      string `json:"username" validate:"omitempty,max=200"`
	Password      string `json:"password" validate:"omitempty,max=500" doc:"On update, empty keeps the stored password"`
	ClearPassword bool   `json:"clear_password"`
	SyncEnabled   *bool  `json:"sync_enabled"`
	PastDays      *int   `json:"past_days" validate:"omitempty,min=0,max=3650"`
	FutureDays    *int   `json:"future_days" validate:"omitempty,min=0,max=3650"`
}

func (b CalendarSubscriptionBody) input() domain.Input {
	return domain.Input{
		Name: b.Name, URL: b.URL, Username: b.Username, Password: b.Password, ClearPassword: b.ClearPassword,
		SyncEnabled: b.SyncEnabled, PastDays: b.PastDays, FutureDays: b.FutureDays,
	}
}

type CreateCalendarInput struct{ Body CalendarSubscriptionBody }

func (i *CreateCalendarInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type UpdateCalendarInput struct {
	ID   string `path:"id" doc:"The subscription's ID"`
	Body CalendarSubscriptionBody
}

func (i *UpdateCalendarInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type CalendarPath struct {
	ID string `path:"id" doc:"The subscription's ID"`
}

type CalendarListOutput struct {
	Body struct {
		Calendars []CalendarSubscriptionResponse `json:"calendars" nullable:"false"`
	}
}

type CalendarOutput struct{ Body CalendarSubscriptionResponse }

type CalendarMessageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

type CalendarSyncOutput struct {
	Body struct {
		Message string `json:"message"`
		Created int    `json:"created"`
		Updated int    `json:"updated"`
		Skipped int    `json:"skipped"`
	}
}

func actorAndID(ctx context.Context, raw string) (domain.UserID, domain.SubscriptionID, error) {
	actor, ok := api.UserID(ctx)
	if !ok {
		return 0, 0, apperrors.ErrUnauthorized("Authentication required")
	}
	id, err := idutil.Parse(raw)
	if err != nil {
		return 0, 0, apperrors.ErrInvalidInput("id", "must be a positive integer")
	}
	return domain.UserID(actor), domain.SubscriptionID(id), nil
}

func response(s domain.Subscription) CalendarSubscriptionResponse {
	return CalendarSubscriptionResponse{
		ID: uint(s.ID), Name: s.Name, URL: s.URL, Username: s.Username, HasPassword: s.HasPassword, SyncEnabled: s.SyncEnabled,
		PastDays: s.PastDays, FutureDays: s.FutureDays, LastSyncedAt: s.LastSyncedAt, LastSyncStatus: s.LastSyncStatus,
		LastSyncError: s.LastSyncError, CreatedAt: s.CreatedAt,
	}
}

func (h *handlers) list(ctx context.Context, _ *struct{}) (*CalendarListOutput, error) {
	actor, ok := api.UserID(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized("Authentication required")
	}
	subs, err := h.uc.List(ctx, domain.UserID(actor))
	if err != nil {
		return nil, toAppError(err)
	}
	out := &CalendarListOutput{}
	out.Body.Calendars = make([]CalendarSubscriptionResponse, len(subs))
	for i, s := range subs {
		out.Body.Calendars[i] = response(s)
	}
	return out, nil
}

func (h *handlers) create(ctx context.Context, in *CreateCalendarInput) (*CalendarOutput, error) {
	actor, ok := api.UserID(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized("Authentication required")
	}
	s, err := h.uc.Create(ctx, domain.UserID(actor), in.Body.input())
	if err != nil {
		return nil, toAppError(err)
	}
	return &CalendarOutput{Body: response(s)}, nil
}

func (h *handlers) update(ctx context.Context, in *UpdateCalendarInput) (*CalendarOutput, error) {
	actor, id, err := actorAndID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	s, err := h.uc.Update(ctx, actor, id, in.Body.input())
	if err != nil {
		return nil, toAppError(err)
	}
	return &CalendarOutput{Body: response(s)}, nil
}

func (h *handlers) delete(ctx context.Context, in *CalendarPath) (*CalendarMessageOutput, error) {
	actor, id, err := actorAndID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if err := h.uc.Delete(ctx, actor, id); err != nil {
		return nil, toAppError(err)
	}
	out := &CalendarMessageOutput{}
	out.Body.Message = "Calendar subscription deleted"
	return out, nil
}

func (h *handlers) sync(ctx context.Context, in *CalendarPath) (*CalendarSyncOutput, error) {
	actor, id, err := actorAndID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	stats, err := h.uc.Sync(ctx, actor, id, api.Clock(ctx))
	if err != nil {
		var syncErr *domain.SyncError
		if errors.As(err, &syncErr) {
			api.Logger(ctx).Warn().Err(err).Uint("subscription_id", uint(id)).Msg("Manual calendar sync failed")
		}
		return nil, toAppError(err)
	}
	out := &CalendarSyncOutput{}
	out.Body.Message, out.Body.Created, out.Body.Updated, out.Body.Skipped = "Calendar sync completed", stats.Created, stats.Updated, stats.Skipped
	return out, nil
}

func toAppError(err error) *apperrors.AppError {
	var syncErr *domain.SyncError
	var f *app.Failure
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apperrors.ErrNotFound("Calendar subscription")
	case errors.Is(err, domain.ErrLimit):
		return apperrors.ErrConflict("maximum of 10 calendar subscriptions per user reached")
	case errors.Is(err, domain.ErrInvalidURL):
		return apperrors.ErrInvalidInput("url", err.Error())
	case errors.Is(err, domain.ErrPasswordRequired):
		return apperrors.ErrInvalidInput("password", err.Error())
	case errors.As(err, &syncErr):
		return syncError(syncErr)
	case errors.As(err, &f):
		if f.Stage == app.StageSecret {
			return apperrors.ErrInternal("failed to store credentials").WithError(f.Err)
		}
		msg := map[app.Stage]string{
			app.StageList: "query", app.StageCount: "count", app.StageCreate: "insert", app.StageFind: "query",
			app.StageUpdate: "update", app.StageDelete: "delete",
		}[f.Stage]
		if msg != "" {
			return apperrors.ErrDatabase(msg).WithError(f.Err)
		}
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}

func syncError(e *domain.SyncError) *apperrors.AppError {
	switch e.Kind {
	case domain.SyncInvalidURL:
		return apperrors.ErrInvalidInput("url", "calendar URL is invalid")
	case domain.SyncUnauthorized:
		return apperrors.ErrExternal("Calendar", "authentication failed - check username and password")
	case domain.SyncNotFound:
		return apperrors.ErrExternal("Calendar", "calendar not found at the given URL")
	case domain.SyncPrivateAddress:
		return apperrors.ErrExternal("Calendar", "calendar URL resolves to a blocked private address")
	case domain.SyncTooLarge:
		return apperrors.ErrExternal("Calendar", "calendar response is too large")
	case domain.SyncInvalidData:
		return apperrors.ErrExternal("Calendar", "calendar returned data that could not be parsed")
	case domain.SyncUnreachable:
		return apperrors.ErrExternal("Calendar", "calendar could not be reached")
	}
	return apperrors.ErrOperationFailed("Calendar sync", e.Err.Error())
}
