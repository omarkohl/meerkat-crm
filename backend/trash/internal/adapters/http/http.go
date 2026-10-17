// Package httpadapter exposes the trash use cases as huma operations. It maps HTTP to the
// use cases and their results and errors back to the responses the API always sent
// (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"time"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/idutil"
	"meerkat/trash/internal/app"
	"meerkat/trash/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	tags := []string{"trash"}
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "list-trash", Method: http.MethodGet, Path: "/trash/contacts",
		Summary: "The deleted contacts, newest deletion first", Tags: tags,
	}, h.list)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "empty-trash", Method: http.MethodDelete, Path: "/trash/contacts",
		Summary: "Delete every contact in the trash for good", Tags: tags,
	}, h.empty)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "restore-contact", Method: http.MethodPost, Path: "/trash/contacts/{id}/restore",
		Summary: "Restore a deleted contact with its notes, reminders and relationships", Tags: tags,
	}, h.restore)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "purge-contact", Method: http.MethodDelete, Path: "/trash/contacts/{id}",
		Summary: "Delete one contact in the trash for good", Tags: tags,
	}, h.purge)
}

type handlers struct{ uc *app.UseCases }

type TrashedContactResponse struct {
	ID             uint      `json:"id"`
	Firstname      string    `json:"firstname"`
	Lastname       string    `json:"lastname"`
	Nickname       string    `json:"nickname"`
	Email          string    `json:"email"`
	PhotoThumbnail string    `json:"photo_thumbnail"`
	DeletedAt      time.Time `json:"deleted_at"`
	PurgeAt        time.Time `json:"purge_at" doc:"When the automatic purge removes it"`
}

// ContactPath takes the ID as text: one that is not a number is a contact that is not in the
// trash, as it always was.
type ContactPath struct {
	ID string `path:"id" doc:"The contact's ID"`
}

type TrashListOutput struct {
	Body struct {
		Contacts      []TrashedContactResponse `json:"contacts"`
		RetentionDays int                      `json:"retention_days"`
	}
}

type TrashRestoredOutput struct {
	Body struct {
		Message string `json:"message"`
		ID      uint   `json:"id"`
	}
}

type TrashMessageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

type TrashEmptiedOutput struct {
	Body struct {
		Message string `json:"message"`
		Purged  int    `json:"purged"`
	}
}

func actorOf(ctx context.Context) (domain.UserID, error) {
	id, ok := api.UserID(ctx)
	if !ok {
		return 0, apperrors.ErrUnauthorized("Authentication required")
	}
	return domain.UserID(id), nil
}

func (h *handlers) list(ctx context.Context, _ *struct{}) (*TrashListOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	l, err := h.uc.ListTrash(ctx, actor)
	if err != nil {
		return nil, toAppError(err, "")
	}
	out := &TrashListOutput{}
	out.Body.RetentionDays = l.RetentionDays
	out.Body.Contacts = make([]TrashedContactResponse, len(l.Contacts))
	for i, c := range l.Contacts {
		out.Body.Contacts[i] = TrashedContactResponse{
			ID: uint(c.ID), Firstname: c.Firstname, Lastname: c.Lastname, Nickname: c.Nickname, Email: c.Email,
			PhotoThumbnail: c.PhotoThumbnail, DeletedAt: c.DeletedAt, PurgeAt: h.uc.PurgeAt(c),
		}
	}
	return out, nil
}

func (h *handlers) restore(ctx context.Context, in *ContactPath) (*TrashRestoredOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	id, err := idutil.Parse(in.ID)
	if err != nil {
		return nil, toAppError(domain.ErrNotInTrash, in.ID)
	}
	restored, err := h.uc.RestoreContact(ctx, actor, domain.ContactID(id), api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, in.ID)
	}
	out := &TrashRestoredOutput{}
	out.Body.Message, out.Body.ID = "Contact restored", uint(restored)
	return out, nil
}

func (h *handlers) purge(ctx context.Context, in *ContactPath) (*TrashMessageOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	id, err := idutil.Parse(in.ID)
	if err != nil {
		return nil, toAppError(domain.ErrNotInTrash, in.ID)
	}
	if err := h.uc.PurgeContact(ctx, actor, domain.ContactID(id), api.Now(ctx)); err != nil {
		return nil, toAppError(err, in.ID)
	}
	out := &TrashMessageOutput{}
	out.Body.Message = "Contact deleted permanently"
	return out, nil
}

func (h *handlers) empty(ctx context.Context, _ *struct{}) (*TrashEmptiedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	n, err := h.uc.EmptyTrash(ctx, actor, api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, "")
	}
	out := &TrashEmptiedOutput{}
	out.Body.Message, out.Body.Purged = "Trash emptied", n
	return out, nil
}

// toAppError words a use-case error the way the Gin handlers did.
func toAppError(err error, id string) *apperrors.AppError {
	if errors.Is(err, domain.ErrNotInTrash) {
		return apperrors.ErrNotFound("Contact").WithDetails("id", id)
	}
	var f *app.Failure
	if errors.As(err, &f) {
		msg := map[app.Stage]string{
			app.StageList:    "Failed to retrieve trash",
			app.StageRestore: "Failed to restore contact",
			app.StagePurge:   "Failed to delete contact permanently",
			app.StageEmpty:   "Failed to empty trash",
		}[f.Stage]
		if msg != "" {
			return apperrors.ErrDatabase(msg).WithError(f.Err)
		}
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
