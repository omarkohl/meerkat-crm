// Package httpadapter exposes the contactsync use cases as huma operations
// (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"time"

	"meerkat/api"
	"meerkat/contactsync/internal/app"
	"meerkat/contactsync/internal/domain"
	apperrors "meerkat/errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	tags := []string{"carddav-sync"}
	op := func(id, method, path, summary string, status int) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary, Tags: tags, DefaultStatus: status}
	}
	api.RegisterOn(a, group, op("get-carddav-connection", http.MethodGet, "/carddav/connection", "The user's CardDAV connection; 404 when none is configured", 0), h.get)
	api.RegisterOn(a, group, op("save-carddav-connection", http.MethodPut, "/carddav/connection", "Create or replace the connection; 201 when it is new", 0), h.save)
	api.RegisterOn(a, group, op("delete-carddav-connection", http.MethodDelete, "/carddav/connection", "Remove the connection and its link state; contacts stay", 0), h.delete)
	api.RegisterOn(a, group, op("discover-carddav-address-books", http.MethodPost, "/carddav/discover", "List the address books on a CardDAV server", 0), h.discover)
	api.RegisterOn(a, group, op("sync-carddav-connection", http.MethodPost, "/carddav/connection/sync", "Start a sync in the background; poll the connection for the outcome", http.StatusAccepted), h.sync)
}

type handlers struct{ uc *app.UseCases }

type ContactSyncStats struct {
	PulledCreated int `json:"pulled_created"`
	PulledUpdated int `json:"pulled_updated"`
	PulledDeleted int `json:"pulled_deleted"`
	PushedCreated int `json:"pushed_created"`
	PushedUpdated int `json:"pushed_updated"`
	PushedDeleted int `json:"pushed_deleted"`
	Conflicts     int `json:"conflicts"`
	Skipped       int `json:"skipped"`
	Errors        int `json:"errors"`
}

type CardDAVConnectionResponse struct {
	ID              uint                           `json:"id"`
	BaseURL         string                         `json:"base_url"`
	AddressBookPath string                         `json:"address_book_path"`
	AddressBookName string                         `json:"address_book_name"`
	Username        string                         `json:"username"`
	HasPassword     bool                           `json:"has_password"`
	Direction       string                         `json:"direction" doc:"two_way, pull or push"`
	SyncEnabled     bool                           `json:"sync_enabled"`
	SyncRunning     bool                           `json:"sync_running" doc:"A sync started by the user or the schedule is in flight"`
	LastSyncedAt    *time.Time                     `json:"last_synced_at" nullable:"true"`
	LastSyncStatus  string                         `json:"last_sync_status" doc:"Empty, success, partial or error"`
	LastSyncError   string                         `json:"last_sync_error"`
	LastSyncStats   api.Nullable[ContactSyncStats] `json:"last_sync_stats" doc:"Counts of the last completed run, null before the first one"`
	CreatedAt       time.Time                      `json:"created_at"`
}

type CardDAVConnectionBody struct {
	BaseURL         string `json:"base_url" validate:"required,url,max=2048"`
	AddressBookPath string `json:"address_book_path" validate:"required,max=2048"`
	AddressBookName string `json:"address_book_name" validate:"max=200"`
	Username        string `json:"username" validate:"omitempty,max=200"`
	Password        string `json:"password" validate:"omitempty,max=500" doc:"On update, empty keeps the stored password"`
	ClearPassword   bool   `json:"clear_password"`
	Direction       string `json:"direction" validate:"omitempty,oneof=two_way pull push"`
	SyncEnabled     *bool  `json:"sync_enabled"`
}

type SaveInput struct{ Body CardDAVConnectionBody }

func (i *SaveInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type ConnectionOutput struct {
	Status int
	Body   CardDAVConnectionResponse
}

type DiscoverBody struct {
	BaseURL  string `json:"base_url" validate:"required,url,max=2048"`
	Username string `json:"username" validate:"omitempty,max=200"`
	Password string `json:"password" validate:"omitempty,max=500" doc:"Empty reuses the stored password, for the stored server only"`
}

type DiscoverInput struct{ Body DiscoverBody }

func (i *DiscoverInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type AddressBook struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type DiscoverOutput struct {
	Body struct {
		AddressBooks []AddressBook `json:"address_books" nullable:"false"`
	}
}

type MessageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func actorOf(ctx context.Context) (domain.UserID, error) {
	id, ok := api.UserID(ctx)
	if !ok {
		return 0, apperrors.ErrUnauthorized("Authentication required")
	}
	return domain.UserID(id), nil
}

func response(c domain.Connection) CardDAVConnectionResponse {
	r := CardDAVConnectionResponse{
		ID: c.ID, BaseURL: c.BaseURL, AddressBookPath: c.AddressBookPath, AddressBookName: c.AddressBookName, Username: c.Username,
		HasPassword: c.HasPassword, Direction: c.Direction, SyncEnabled: c.SyncEnabled, SyncRunning: c.SyncRunning,
		LastSyncedAt: c.LastSyncedAt, LastSyncStatus: c.LastSyncStatus, LastSyncError: c.LastSyncError, CreatedAt: c.CreatedAt,
	}
	if s := c.LastSyncStats; s != nil {
		st := ContactSyncStats(*s)
		r.LastSyncStats.Value = &st
	}
	return r
}

func (h *handlers) get(ctx context.Context, _ *struct{}) (*ConnectionOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	c, err := h.uc.Get(ctx, actor)
	if err != nil {
		return nil, toAppError(err)
	}
	return &ConnectionOutput{Status: http.StatusOK, Body: response(c)}, nil
}

func (h *handlers) save(ctx context.Context, in *SaveInput) (*ConnectionOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	b := in.Body
	saved, err := h.uc.Save(ctx, actor, domain.Input{
		BaseURL: b.BaseURL, AddressBookPath: b.AddressBookPath, AddressBookName: b.AddressBookName, Username: b.Username,
		Password: b.Password, ClearPassword: b.ClearPassword, Direction: b.Direction, SyncEnabled: b.SyncEnabled,
	})
	if err != nil {
		return nil, toAppError(err)
	}
	status := http.StatusOK
	if saved.IsNew {
		status = http.StatusCreated
	}
	return &ConnectionOutput{Status: status, Body: response(saved.Connection)}, nil
}

func (h *handlers) delete(ctx context.Context, _ *struct{}) (*MessageOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.Delete(ctx, actor); err != nil {
		return nil, toAppError(err)
	}
	out := &MessageOutput{}
	out.Body.Message = "CardDAV connection deleted"
	return out, nil
}

func (h *handlers) discover(ctx context.Context, in *DiscoverInput) (*DiscoverOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	books, err := h.uc.Discover(ctx, actor, in.Body.BaseURL, in.Body.Username, in.Body.Password)
	if err != nil {
		var syncErr *domain.SyncError
		if errors.As(err, &syncErr) {
			api.Logger(ctx).Warn().Err(err).Msg("CardDAV discovery failed")
		}
		return nil, toAppError(err)
	}
	out := &DiscoverOutput{}
	out.Body.AddressBooks = make([]AddressBook, len(books))
	for i, b := range books {
		out.Body.AddressBooks[i] = AddressBook(b)
	}
	return out, nil
}

func (h *handlers) sync(ctx context.Context, _ *struct{}) (*MessageOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.StartSync(ctx, actor, api.Clock(ctx)); err != nil {
		return nil, toAppError(err)
	}
	out := &MessageOutput{}
	out.Body.Message = "Contact sync started"
	return out, nil
}

func toAppError(err error) *apperrors.AppError {
	var syncErr *domain.SyncError
	var f *app.Failure
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apperrors.ErrNotFound("CardDAV connection")
	case errors.Is(err, domain.ErrInvalidBaseURL):
		return apperrors.ErrInvalidInput("base_url", err.Error())
	case errors.Is(err, domain.ErrRelativePath):
		return apperrors.ErrInvalidInput("address_book_path", err.Error())
	case errors.Is(err, domain.ErrPasswordRequired):
		return apperrors.ErrInvalidInput("password", err.Error())
	case errors.Is(err, domain.ErrSyncRunning):
		return apperrors.ErrConflict(err.Error())
	case errors.As(err, &syncErr):
		return syncError(syncErr.Kind)
	case errors.As(err, &f):
		if f.Stage == app.StageSecret {
			return apperrors.ErrInternal("failed to store credentials").WithError(f.Err)
		}
		msg := map[app.Stage]string{app.StageFind: "query", app.StageSave: "save", app.StageDelete: "delete"}[f.Stage]
		if msg != "" {
			return apperrors.ErrDatabase(msg).WithError(f.Err)
		}
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}

func syncError(kind domain.SyncKind) *apperrors.AppError {
	messages := map[domain.SyncKind]string{
		domain.SyncUnauthorized:   "authentication failed - check username and password",
		domain.SyncForbidden:      "the server refused the request - check permissions",
		domain.SyncNotFound:       "address book not found at the given URL",
		domain.SyncPrivateAddress: "URL resolves to a blocked private address",
		domain.SyncTooLarge:       "server response is too large",
		domain.SyncTooMany:        "address book has too many contacts to sync",
		domain.SyncNoAddressBooks: "no address books found on the server",
		domain.SyncInvalidData:    "server returned data that could not be parsed",
		domain.SyncUnreachable:    "server could not be reached",
		domain.SyncServerError:    "server answered with an error",
	}
	if kind == domain.SyncInvalidURL {
		return apperrors.ErrInvalidInput("base_url", "CardDAV URL is invalid")
	}
	if msg, ok := messages[kind]; ok {
		return apperrors.ErrExternal("CardDAV", msg)
	}
	return apperrors.ErrOperationFailed("Contact sync", "an unexpected error occurred")
}
