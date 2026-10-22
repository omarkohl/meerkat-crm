// Package httpadapter exposes the activities use cases as huma operations. It maps HTTP to the
// use cases and their results and errors back to the responses the API always sent
// (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"meerkat/activities/internal/app"
	"meerkat/activities/internal/domain"
	"meerkat/api"
	apperrors "meerkat/errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

// Register adds the activity operations to a, on group (the authenticated group).
func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	tags := []string{"activities"}
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "list-contact-activities", Method: http.MethodGet, Path: "/contacts/{id}/activities",
		Summary: "The activities a contact took part in", Tags: tags,
	}, h.contactActivities)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "create-activity", Method: http.MethodPost, Path: "/activities",
		Summary: "Add an activity with some contacts", Tags: tags,
	}, h.create)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "list-activities", Method: http.MethodGet, Path: "/activities",
		Summary: "List activities with search, date range and paging", Tags: tags,
	}, h.list)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "get-activity", Method: http.MethodGet, Path: "/activities/{id}",
		Summary: "Get an activity with its contacts", Tags: tags,
	}, h.get)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "update-activity", Method: http.MethodPut, Path: "/activities/{id}",
		Summary: "Replace the fields of an activity, and its contacts when contact_ids is given", Tags: tags,
	}, h.update)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "delete-activity", Method: http.MethodDelete, Path: "/activities/{id}",
		Summary: "Delete an activity", Tags: tags,
	}, h.delete)
}

type handlers struct{ uc *app.UseCases }

type ActivityBody struct {
	Title       string                  `json:"title" validate:"required,min=1,max=200"`
	Description string                  `json:"description" validate:"max=2000"`
	Location    string                  `json:"location" validate:"max=300"`
	Date        time.Time               `json:"date" validate:"required"`
	EndDate     api.Optional[time.Time] `json:"end_date,omitempty" doc:"Last day of an activity that spans several days; on or after date. On update, leaving it out keeps the current one; null removes it."`
	ContactIDs  []uint                  `json:"contact_ids,omitempty" doc:"The contacts that took part. On update, leaving it out keeps the current ones; an empty list removes them."`
}

type ContactRefResponse struct {
	ID        uint     `json:"ID"`
	Firstname string   `json:"firstname"`
	Lastname  string   `json:"lastname"`
	Circles   []string `json:"circles" nullable:"true"`
}

type ActivityResponse struct {
	ID          uint                 `json:"ID"`
	CreatedAt   time.Time            `json:"CreatedAt"`
	UpdatedAt   time.Time            `json:"UpdatedAt"`
	DeletedAt   *time.Time           `json:"DeletedAt" nullable:"true"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	Location    string               `json:"location"`
	Date        time.Time            `json:"date"`
	EndDate     *time.Time           `json:"end_date" nullable:"true"`
	Contacts    []ContactRefResponse `json:"contacts,omitempty"`
}

type contactPath struct {
	ID uint `path:"id" minimum:"1" doc:"The contact's ID"`
}

type activityPath struct {
	ID uint `path:"id" minimum:"1" doc:"The activity's ID"`
}

type CreateInput struct{ Body ActivityBody }

func (i *CreateInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return i.Body.validate(prefix)
}

type UpdateInput struct {
	ID   uint `path:"id" minimum:"1" doc:"The activity's ID"`
	Body ActivityBody
}

func (i *UpdateInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return i.Body.validate(prefix)
}

func (b *ActivityBody) validate(prefix *huma.PathBuffer) []error {
	errs := api.ValidateBody(prefix, b)
	if b.draft().EndsBeforeStart() {
		errs = append(errs, &huma.ErrorDetail{Message: "end_date must not be before date", Location: prefix.With("body") + ".end_date"})
	}
	return errs
}

// ListInput values are strings on purpose: one that does not parse falls back to its default,
// as before.
type ListInput struct {
	Page     string `query:"page" doc:"Page number, from 1"`
	Limit    string `query:"limit" doc:"Page size, 1 to 100, default 25"`
	Search   string `query:"search" doc:"Full-text search over title, description, location and contact names"`
	FromDate string `query:"fromDate" doc:"First day, YYYY-MM-DD; ignored when not a date"`
	ToDate   string `query:"toDate" doc:"Last day, inclusive, YYYY-MM-DD; ignored when not a date"`
	Include  string `query:"include" doc:"contacts: add the contacts of each activity"`
}

type ActivityOutput struct{ Body ActivityResponse }

type ActivityCreatedOutput struct {
	Body struct {
		Message  string           `json:"message"`
		Activity ActivityResponse `json:"activity"`
	}
}

type ContactActivitiesOutput struct {
	Body struct {
		Activities []ActivityResponse `json:"activities"`
	}
}

type ActivityListOutput struct {
	Body struct {
		Activities []ActivityResponse `json:"activities"`
		Total      int64              `json:"total"`
		Page       int                `json:"page"`
		Limit      int                `json:"limit"`
	}
}

type ActivityMessageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func (h *handlers) contactActivities(ctx context.Context, in *contactPath) (*ContactActivitiesOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	list, err := h.uc.ActivitiesOfContact(ctx, actor, domain.ContactID(in.ID))
	if err != nil {
		return nil, toAppError(err, 0, in.ID)
	}
	out := &ContactActivitiesOutput{}
	out.Body.Activities = responses(list)
	return out, nil
}

func (h *handlers) create(ctx context.Context, in *CreateInput) (*ActivityCreatedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	a, err := h.uc.CreateActivity(ctx, actor, in.Body.draft(), contactIDs(in.Body.ContactIDs), api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, 0, 0)
	}
	out := &ActivityCreatedOutput{}
	out.Body.Message = "Activity created successfully"
	out.Body.Activity = NewActivityResponse(a)
	return out, nil
}

func (h *handlers) get(ctx context.Context, in *activityPath) (*ActivityOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	a, err := h.uc.GetActivity(ctx, actor, domain.ActivityID(in.ID))
	if err != nil {
		return nil, toAppError(err, in.ID, 0)
	}
	return &ActivityOutput{Body: NewActivityResponse(a)}, nil
}

func (h *handlers) list(ctx context.Context, in *ListInput) (*ActivityListOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	list, total, page, limit, err := h.uc.ListActivities(ctx, actor, app.Filter{
		Search: in.Search, From: day(in.FromDate), To: day(in.ToDate),
		Page: atoiOr0(in.Page), Limit: atoiOr0(in.Limit), WithContacts: in.Include == "contacts",
	})
	if err != nil {
		return nil, toAppError(err, 0, 0)
	}
	out := &ActivityListOutput{}
	out.Body.Activities = responses(list)
	out.Body.Total, out.Body.Page, out.Body.Limit = total, page, limit
	return out, nil
}

func (h *handlers) update(ctx context.Context, in *UpdateInput) (*ActivityOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	a, err := h.uc.UpdateActivity(ctx, actor, domain.ActivityID(in.ID), in.Body.draft(), !in.Body.EndDate.Set, contactIDs(in.Body.ContactIDs), api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, in.ID, 0)
	}
	return &ActivityOutput{Body: NewActivityResponse(a)}, nil
}

func (h *handlers) delete(ctx context.Context, in *activityPath) (*ActivityMessageOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.DeleteActivity(ctx, actor, domain.ActivityID(in.ID), api.Now(ctx)); err != nil {
		return nil, toAppError(err, in.ID, 0)
	}
	out := &ActivityMessageOutput{}
	out.Body.Message = "Activity deleted"
	return out, nil
}

func (b ActivityBody) draft() domain.Activity {
	return domain.Activity{Title: b.Title, Description: b.Description, Location: b.Location, Date: b.Date, EndDate: b.EndDate.Value}
}

func contactIDs(ids []uint) []domain.ContactID {
	if ids == nil {
		return nil
	}
	out := make([]domain.ContactID, len(ids))
	for i, id := range ids {
		out[i] = domain.ContactID(id)
	}
	return out
}

// NewActivityResponse maps an activity to the JSON shape of the API, also used for webhooks.
func NewActivityResponse(a domain.Activity) ActivityResponse {
	r := ActivityResponse{
		ID: uint(a.ID), CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, DeletedAt: a.DeletedAt,
		Title: a.Title, Description: a.Description, Location: a.Location, Date: a.Date, EndDate: a.EndDate,
	}
	for _, c := range a.Contacts {
		r.Contacts = append(r.Contacts, ContactRefResponse{ID: uint(c.ID), Firstname: c.Firstname, Lastname: c.Lastname, Circles: c.Circles})
	}
	return r
}

func responses(list []domain.Activity) []ActivityResponse {
	out := make([]ActivityResponse, len(list))
	for i, a := range list {
		out[i] = NewActivityResponse(a)
	}
	return out
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

// toAppError words a use-case error the way the Gin handlers did. activityID and contactID,
// when not 0, are reported in the details of a not-found error.
func toAppError(err error, activityID, contactID uint) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apperrors.ErrNotFound("Activity").WithDetails("id", strconv.FormatUint(uint64(activityID), 10))
	case errors.Is(err, domain.ErrContactNotFound):
		return apperrors.ErrNotFound("Contact").WithDetails("id", strconv.FormatUint(uint64(contactID), 10))
	case errors.Is(err, domain.ErrContactsNotFound):
		return apperrors.ErrNotFound("One or more contacts")
	case errors.Is(err, domain.ErrEndBeforeStart):
		return apperrors.ErrInvalidInput("end_date", "must not be before date")
	}
	var f *app.Failure
	if errors.As(err, &f) {
		switch f.Stage {
		case app.StageRetrieve:
			return apperrors.ErrDatabase("Failed to retrieve activity").WithError(f.Err)
		case app.StageContact:
			return apperrors.ErrDatabase("Failed to retrieve contact").WithError(f.Err)
		case app.StageForContact:
			return apperrors.ErrDatabase("Failed to retrieve activities for contact").WithError(f.Err)
		case app.StageList:
			return apperrors.ErrDatabase("Failed to retrieve activities").WithError(f.Err)
		case app.StageCreate:
			return apperrors.ErrDatabase("Failed to save activity").WithError(f.Err)
		case app.StageUpdate:
			return apperrors.ErrDatabase("Failed to save activity").WithError(f.Err)
		case app.StageDelete:
			return apperrors.ErrDatabase("Failed to delete activity").WithError(f.Err)
		}
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
