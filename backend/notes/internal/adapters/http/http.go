// Package httpadapter exposes the notes use cases as huma operations. It maps HTTP to the
// use cases and their results and errors back to the responses the API always sent
// (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/notes/internal/app"
	"meerkat/notes/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

// Register adds the note operations to a, on group (the authenticated group).
func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	tags := []string{"notes"}
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "list-contact-notes", Method: http.MethodGet, Path: "/contacts/{id}/notes",
		Summary: "The notes about a contact", Tags: tags,
	}, h.contactNotes)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "create-contact-note", Method: http.MethodPost, Path: "/contacts/{id}/notes",
		Summary: "Add a note about a contact", Tags: tags,
	}, h.createForContact)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "list-unassigned-notes", Method: http.MethodGet, Path: "/notes",
		Summary: "Notes without a contact, with search, date range and paging", Tags: tags,
	}, h.list)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "create-note", Method: http.MethodPost, Path: "/notes",
		Summary: "Add a note, about a contact or free-floating", Tags: tags,
	}, h.create)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "get-note", Method: http.MethodGet, Path: "/notes/{id}",
		Summary: "Get a note", Tags: tags,
	}, h.get)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "update-note", Method: http.MethodPut, Path: "/notes/{id}",
		Summary: "Replace the content and date of a note, and move it to another contact", Tags: tags,
	}, h.update)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "delete-note", Method: http.MethodDelete, Path: "/notes/{id}",
		Summary: "Delete a note", Tags: tags,
	}, h.delete)
}

type handlers struct{ uc *app.UseCases }

type NoteBody struct {
	Content   string    `json:"content" validate:"max=5000" doc:"May be empty when the note has images"`
	Date      time.Time `json:"date" validate:"required"`
	ContactID *uint     `json:"contact_id,omitempty" validate:"omitempty,gt=0" doc:"Ignored when the note is added under /contacts/{id}/notes. On update, leaving it out keeps the current contact."`
	ImageIDs  []uint    `json:"image_ids,omitempty" validate:"max=20,dive,gt=0" doc:"Uploaded images (POST /images), in order, at most 20. On update, leaving it out keeps the current ones; an empty list removes them."`
}

type NoteResponse struct {
	ID        uint           `json:"ID"`
	CreatedAt time.Time      `json:"CreatedAt"`
	UpdatedAt time.Time      `json:"UpdatedAt"`
	DeletedAt *time.Time     `json:"DeletedAt" nullable:"true"`
	Content   string         `json:"content"`
	Date      time.Time      `json:"date"`
	ContactID *uint          `json:"contact_id" nullable:"true"`
	Images    []api.ImageRef `json:"images" nullable:"false"`
}

type contactPath struct {
	ID uint `path:"id" minimum:"1" doc:"The contact's ID"`
}

type notePath struct {
	ID uint `path:"id" minimum:"1" doc:"The note's ID"`
}

type CreateForContactInput struct {
	ID   uint `path:"id" minimum:"1" doc:"The contact's ID"`
	Body NoteBody
}

func (i *CreateForContactInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type CreateInput struct{ Body NoteBody }

func (i *CreateInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type UpdateInput struct {
	ID   uint `path:"id" minimum:"1" doc:"The note's ID"`
	Body NoteBody
}

func (i *UpdateInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

// ListInput values are strings on purpose: one that does not parse falls back to its default,
// as before.
type ListInput struct {
	Page     string `query:"page" doc:"Page number, from 1"`
	Limit    string `query:"limit" doc:"Page size, 1 to 100, default 25"`
	Search   string `query:"search" doc:"Full-text search over the content"`
	FromDate string `query:"fromDate" doc:"First day, YYYY-MM-DD; ignored when not a date"`
	ToDate   string `query:"toDate" doc:"Last day, inclusive, YYYY-MM-DD; ignored when not a date"`
}

type NoteOutput struct{ Body NoteResponse }

type NoteSavedOutput struct {
	Body struct {
		Message string       `json:"message"`
		Note    NoteResponse `json:"note"`
	}
}

type NotesOutput struct {
	Body struct {
		Notes []NoteResponse `json:"notes"`
	}
}

type NoteListOutput struct {
	Body struct {
		Notes []NoteResponse `json:"notes"`
		Total int64          `json:"total"`
		Page  int            `json:"page"`
		Limit int            `json:"limit"`
	}
}

type NoteMessageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func (h *handlers) contactNotes(ctx context.Context, in *contactPath) (*NotesOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	notes, err := h.uc.NotesOfContact(ctx, actor, domain.ContactID(in.ID))
	if err != nil {
		return nil, toAppError(err, 0, in.ID)
	}
	out := &NotesOutput{}
	out.Body.Notes = responses(notes)
	return out, nil
}

func (h *handlers) createForContact(ctx context.Context, in *CreateForContactInput) (*NoteSavedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	contact := domain.ContactID(in.ID)
	n, err := h.uc.CreateNote(ctx, actor, in.Body.Content, in.Body.Date, &contact, in.Body.ImageIDs, api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, 0, in.ID)
	}
	return saved("Note created successfully", n), nil
}

func (h *handlers) create(ctx context.Context, in *CreateInput) (*NoteSavedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	n, err := h.uc.CreateNote(ctx, actor, in.Body.Content, in.Body.Date, contactOf(in.Body.ContactID), in.Body.ImageIDs, api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, 0, 0)
	}
	return saved("Note created successfully", n), nil
}

func (h *handlers) get(ctx context.Context, in *notePath) (*NoteOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	n, err := h.uc.GetNote(ctx, actor, domain.NoteID(in.ID))
	if err != nil {
		return nil, toAppError(err, in.ID, 0)
	}
	return &NoteOutput{Body: NewNoteResponse(n)}, nil
}

func (h *handlers) list(ctx context.Context, in *ListInput) (*NoteListOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	notes, total, page, limit, err := h.uc.ListUnassigned(ctx, actor, app.Filter{
		Search: in.Search, From: day(in.FromDate), To: day(in.ToDate),
		Page: atoiOr0(in.Page), Limit: atoiOr0(in.Limit),
	})
	if err != nil {
		return nil, toAppError(err, 0, 0)
	}
	out := &NoteListOutput{}
	out.Body.Notes = responses(notes)
	out.Body.Total, out.Body.Page, out.Body.Limit = total, page, limit
	return out, nil
}

func (h *handlers) update(ctx context.Context, in *UpdateInput) (*NoteSavedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	n, err := h.uc.UpdateNote(ctx, actor, domain.NoteID(in.ID), in.Body.Content, in.Body.Date, contactOf(in.Body.ContactID), in.Body.ImageIDs, api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, in.ID, 0)
	}
	return saved("Note updated successfully", n), nil
}

func (h *handlers) delete(ctx context.Context, in *notePath) (*NoteMessageOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.DeleteNote(ctx, actor, domain.NoteID(in.ID), api.Now(ctx)); err != nil {
		return nil, toAppError(err, in.ID, 0)
	}
	out := &NoteMessageOutput{}
	out.Body.Message = "Note deleted"
	return out, nil
}

func NewNoteResponse(n domain.Note) NoteResponse {
	r := NoteResponse{
		ID: uint(n.ID), CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt, DeletedAt: n.DeletedAt,
		Content: n.Content, Date: n.Date, Images: make([]api.ImageRef, len(n.Images)),
	}
	for i, img := range n.Images {
		r.Images[i] = api.ImageRef{
			ID: img.ID, Width: img.Width, Height: img.Height, Mime: img.Mime, Bytes: img.Bytes, TakenAt: img.TakenAt,
			URL: img.URL, ThumbURL: img.ThumbURL,
		}
	}
	if n.ContactID != nil {
		id := uint(*n.ContactID)
		r.ContactID = &id
	}
	return r
}

func responses(notes []domain.Note) []NoteResponse {
	out := make([]NoteResponse, len(notes))
	for i, n := range notes {
		out[i] = NewNoteResponse(n)
	}
	return out
}

func saved(message string, n domain.Note) *NoteSavedOutput {
	out := &NoteSavedOutput{}
	out.Body.Message = message
	out.Body.Note = NewNoteResponse(n)
	return out
}

func contactOf(id *uint) *domain.ContactID {
	if id == nil {
		return nil
	}
	c := domain.ContactID(*id)
	return &c
}

func day(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

func atoiOr0(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func actorOf(ctx context.Context) (domain.UserID, error) {
	id, ok := api.UserID(ctx)
	if !ok {
		return 0, apperrors.ErrUnauthorized("Authentication required")
	}
	return domain.UserID(id), nil
}

// toAppError words a use-case error the way the Gin handlers did. noteID and contactID, when
// not 0, are reported in the details of a not-found error.
func toAppError(err error, noteID, contactID uint) error {
	if errors.Is(err, domain.ErrNotFound) {
		return apperrors.ErrNotFound("Note").WithDetails("id", strconv.FormatUint(uint64(noteID), 10))
	}
	switch {
	case errors.Is(err, domain.ErrEmpty):
		return apperrors.ErrInvalidInput("content", "A note needs text or an image")
	case errors.Is(err, domain.ErrImageNotFound):
		return apperrors.ErrNotFound("Image")
	case errors.Is(err, domain.ErrTooManyImages):
		return apperrors.ErrInvalidInput("image_ids", "A note can have at most 20 images")
	}
	if errors.Is(err, domain.ErrContactNotFound) {
		e := apperrors.ErrNotFound("Contact")
		if contactID != 0 {
			e = e.WithDetails("id", strconv.FormatUint(uint64(contactID), 10))
		}
		return e
	}
	var f *app.Failure
	if errors.As(err, &f) {
		switch f.Stage {
		case app.StageRetrieve:
			return apperrors.ErrDatabase("Failed to retrieve note").WithError(f.Err)
		case app.StageContact:
			return apperrors.ErrDatabase("Failed to retrieve contact").WithError(f.Err)
		case app.StageList:
			return apperrors.ErrDatabase("Failed to retrieve unassigned notes").WithError(f.Err)
		case app.StageCreate:
			return apperrors.ErrDatabase("Failed to save note").WithError(f.Err)
		case app.StageUpdate:
			return apperrors.ErrDatabase("Failed to update note").WithError(f.Err)
		case app.StageDelete:
			return apperrors.ErrDatabase("Failed to delete note").WithError(f.Err)
		}
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
