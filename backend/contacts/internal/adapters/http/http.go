// Package httpadapter exposes the contacts use cases as huma operations. It maps HTTP to the
// use cases and their results and errors back to the responses the API always sent
// (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"meerkat/api"
	"meerkat/contacts/internal/app"
	"meerkat/contacts/internal/domain"
	apperrors "meerkat/errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

// Register adds the contact operations to a, on group (the authenticated group).
func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "list-contacts", Method: http.MethodGet, Path: "/contacts",
		Summary: "List contacts with search, circle filter, sorting and paging", Tags: []string{"contacts"},
	}, h.list)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "random-contacts", Method: http.MethodGet, Path: "/contacts/random",
		Summary: "Five random contacts", Tags: []string{"contacts"},
	}, h.random)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "upcoming-birthdays", Method: http.MethodGet, Path: "/contacts/birthdays",
		Summary: "Upcoming birthdays of contacts and relatives", Tags: []string{"contacts"},
	}, h.birthdays)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "get-contact", Method: http.MethodGet, Path: "/contacts/{id}",
		Summary: "Get a contact, optionally only some fields", Tags: []string{"contacts"},
	}, h.get)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "create-contact", Method: http.MethodPost, Path: "/contacts", DefaultStatus: http.StatusCreated,
		Summary: "Create a contact", Tags: []string{"contacts"},
	}, h.create)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "update-contact", Method: http.MethodPut, Path: "/contacts/{id}",
		Summary: "Replace the editable fields of a contact", Tags: []string{"contacts"},
	}, h.update)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "archive-contact", Method: http.MethodPost, Path: "/contacts/{id}/archive",
		Summary: "Archive a contact and delete its reminders", Tags: []string{"contacts"},
	}, h.archive)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "unarchive-contact", Method: http.MethodPost, Path: "/contacts/{id}/unarchive",
		Summary: "Restore an archived contact", Tags: []string{"contacts"},
	}, h.unarchive)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "delete-contact", Method: http.MethodDelete, Path: "/contacts/{id}",
		Summary: "Move a contact and its notes, reminders and relationships to the trash", Tags: []string{"contacts"},
	}, h.delete)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "list-circles", Method: http.MethodGet, Path: "/contacts/circles",
		Summary: "The distinct circle names of all contacts", Tags: []string{"contacts"},
	}, h.circles)
}

type handlers struct{ uc *app.UseCases }

// contactPath is the {id} of /contacts/{id}/....
type contactPath struct {
	ID uint `path:"id" minimum:"1" doc:"The contact's ID"`
}

// ContactOutput is the response of the operations that return the changed contact.
type ContactOutput struct {
	Body ContactResponse
}

// GetInput is the request of GET /contacts/{id}.
type GetInput struct {
	ID     uint   `path:"id" minimum:"1" doc:"The contact's ID"`
	Fields string `query:"fields" doc:"Comma-separated field names; only these are filled. Unknown names are ignored."`
}

// Values are strings on purpose: one that does not parse falls back to its default, as before.
type ListInput struct {
	Page            string `query:"page" doc:"Page number, from 1"`
	Limit           string `query:"limit" doc:"Page size, 1 to 100, default 25"`
	Search          string `query:"search" doc:"Full-text search over names, nicknames, e-mail addresses and phone numbers"`
	SearchAll       string `query:"search_all" doc:"true: search every text field, not only names, e-mail addresses and phone numbers; name matches come first"`
	Circle          string `query:"circle" doc:"Only contacts in this circle"`
	Sort            string `query:"sort" doc:"firstname, lastname, id or random; with search and no sort the best match comes first"`
	Order           string `query:"order" doc:"asc or desc, default desc"`
	IncludeArchived string `query:"include_archived" doc:"true: archived contacts too, after the others when searching"`
	Archived        string `query:"archived" doc:"true: only archived contacts"`
	Fields          string `query:"fields" doc:"Comma-separated field names; only these are filled. photo_thumbnail adds the thumbnail URL"`
	Includes        string `query:"includes" doc:"Comma-separated: notes, activities, relationships, reminders"`
}

type ContactListItem struct {
	ContactResponse
	PhotoThumbnail string `json:"photo_thumbnail"`
}

type ContactListOutput struct {
	Body struct {
		Contacts []ContactListItem `json:"contacts"`
		Total    int64             `json:"total"`
		Page     int               `json:"page"`
		Limit    int               `json:"limit"`
	}
}

type RandomOutput struct {
	Body struct {
		Contacts []ContactListItem `json:"contacts"`
	}
}

type BirthdayResponse struct {
	Type                  string `json:"type" enum:"contact,relationship"`
	Name                  string `json:"name"`
	Birthday              string `json:"birthday"`
	PhotoThumbnail        string `json:"photo_thumbnail,omitempty"`
	ContactID             uint   `json:"contact_id"`
	RelationshipType      string `json:"relationship_type,omitempty"`
	AssociatedContactName string `json:"associated_contact_name,omitempty"`
}

type BirthdaysOutput struct {
	Body struct {
		Birthdays []BirthdayResponse `json:"birthdays" nullable:"true"`
	}
}

// ContactCreatedOutput is the response of POST /contacts.
type ContactCreatedOutput struct {
	Body struct {
		Message string          `json:"message"`
		Contact ContactResponse `json:"contact"`
	}
}

// ContactMessageOutput is the response of the operations that only confirm.
type ContactMessageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// CirclesOutput is the response of GET /contacts/circles: a JSON array of names.
type CirclesOutput struct {
	Body []string
}

func (h *handlers) list(ctx context.Context, in *ListInput) (*ContactListOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	page, err := h.uc.ListContacts(ctx, actor, app.ListQuery{
		Search: in.Search, SearchAll: in.SearchAll == "true", Circle: in.Circle, Sort: in.Sort, Order: in.Order,
		IncludeArchived: in.IncludeArchived == "true", ArchivedOnly: in.Archived == "true",
		Page: atoiOr0(in.Page), Limit: atoiOr0(in.Limit),
		Fields: splitList(in.Fields), Includes: splitList(in.Includes),
	})
	if err != nil {
		return nil, toAppError(err, 0)
	}
	out := &ContactListOutput{}
	out.Body.Contacts = listItems(page.Contacts)
	out.Body.Total, out.Body.Page, out.Body.Limit = page.Total, page.Page, page.Limit
	return out, nil
}

func (h *handlers) random(ctx context.Context, _ *struct{}) (*RandomOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	contacts, err := h.uc.RandomContacts(ctx, actor)
	if err != nil {
		return nil, toAppError(err, 0)
	}
	out := &RandomOutput{}
	out.Body.Contacts = listItems(contacts)
	return out, nil
}

func (h *handlers) birthdays(ctx context.Context, _ *struct{}) (*BirthdaysOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	found, err := h.uc.UpcomingBirthdays(ctx, actor, api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, 0)
	}
	out := &BirthdaysOutput{}
	for _, b := range found {
		out.Body.Birthdays = append(out.Body.Birthdays, BirthdayResponse{
			Type: b.Type, Name: b.Name, Birthday: b.Birthday, PhotoThumbnail: b.PhotoThumbnail,
			ContactID: uint(b.ContactID), RelationshipType: b.RelationshipType,
			AssociatedContactName: b.AssociatedContactName,
		})
	}
	return out, nil
}

func listItems(contacts []domain.Contact) []ContactListItem {
	items := make([]ContactListItem, len(contacts))
	for i, c := range contacts {
		items[i] = ContactListItem{ContactResponse: NewContactResponse(c), PhotoThumbnail: c.PhotoThumbnail}
	}
	return items
}

func atoiOr0(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func (h *handlers) get(ctx context.Context, in *GetInput) (*ContactOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	c, err := h.uc.ShowContact(ctx, actor, domain.ContactID(in.ID), splitList(in.Fields))
	if err != nil {
		return nil, toAppError(err, in.ID)
	}
	return &ContactOutput{Body: NewContactResponse(c)}, nil
}

func (h *handlers) create(ctx context.Context, in *CreateInput) (*ContactCreatedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	c, err := h.uc.CreateContact(ctx, actor, in.Body.draft(), api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, 0)
	}
	out := &ContactCreatedOutput{}
	out.Body.Message = "Contact created successfully"
	out.Body.Contact = NewContactResponse(c)
	return out, nil
}

func (h *handlers) update(ctx context.Context, in *UpdateInput) (*ContactOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	c, err := h.uc.UpdateContact(ctx, actor, domain.ContactID(in.ID), in.Body.draft(), api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, in.ID)
	}
	return &ContactOutput{Body: NewContactResponse(c)}, nil
}

func (h *handlers) archive(ctx context.Context, in *contactPath) (*ContactOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	c, err := h.uc.ArchiveContact(ctx, actor, domain.ContactID(in.ID))
	if err != nil {
		return nil, toAppError(err, in.ID)
	}
	return &ContactOutput{Body: NewContactResponse(c)}, nil
}

func (h *handlers) unarchive(ctx context.Context, in *contactPath) (*ContactOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	c, err := h.uc.UnarchiveContact(ctx, actor, domain.ContactID(in.ID))
	if err != nil {
		return nil, toAppError(err, in.ID)
	}
	return &ContactOutput{Body: NewContactResponse(c)}, nil
}

func (h *handlers) delete(ctx context.Context, in *contactPath) (*ContactMessageOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.DeleteContact(ctx, actor, domain.ContactID(in.ID), api.Now(ctx)); err != nil {
		return nil, toAppError(err, in.ID)
	}
	out := &ContactMessageOutput{}
	out.Body.Message = "Contact deleted"
	return out, nil
}

func (h *handlers) circles(ctx context.Context, _ *struct{}) (*CirclesOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	circles, err := h.uc.ListCircles(ctx, actor)
	if err != nil {
		return nil, toAppError(err, 0)
	}
	return &CirclesOutput{Body: circles}, nil
}

func actorOf(ctx context.Context) (domain.UserID, error) {
	id, ok := api.UserID(ctx)
	if !ok {
		return 0, apperrors.ErrUnauthorized("Authentication required")
	}
	return domain.UserID(id), nil
}

// toAppError words a use-case error the way the Gin handlers did.
func toAppError(err error, id uint) error {
	if errors.Is(err, domain.ErrNotFound) {
		return apperrors.ErrNotFound("Contact").WithDetails("id", strconv.FormatUint(uint64(id), 10))
	}
	if errors.Is(err, domain.ErrDeceasedInFuture) {
		return apperrors.ErrValidation("deceased_date must be a valid past date (YYYY-MM-DD) on or after the birthday")
	}
	var f *app.Failure
	if errors.As(err, &f) {
		switch f.Stage {
		case app.StageRetrieve:
			return apperrors.ErrDatabase("Failed to retrieve contact").WithError(f.Err)
		case app.StageCreate:
			return apperrors.ErrDatabase("Failed to save contact").WithError(f.Err)
		case app.StageUpdate:
			return apperrors.ErrDatabase("Failed to update contact").WithError(f.Err)
		case app.StageArchive:
			return apperrors.ErrDatabase("Failed to archive contact").WithError(f.Err)
		case app.StageUnarchive:
			return apperrors.ErrDatabase("Failed to unarchive contact").WithError(f.Err)
		case app.StageDelete:
			return apperrors.ErrDatabase("Failed to delete contact and associated data").WithError(f.Err)
		case app.StageList:
			return apperrors.ErrDatabase("Failed to retrieve contacts").WithError(f.Err)
		case app.StageBirthdays:
			return apperrors.ErrDatabase("Failed to retrieve upcoming birthdays").WithError(f.Err)
		case app.StageCircles:
			return apperrors.ErrDatabase("Failed to retrieve circles").WithError(f.Err)
		}
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
