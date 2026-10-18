// Package httpadapter exposes the webhooks use cases as huma operations. It maps HTTP to the
// use cases and their results and errors back to the responses the API always sent
// (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/idutil"
	"meerkat/webhooks/internal/app"
	"meerkat/webhooks/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	tags := []string{"webhooks"}
	reg := func(id, method, path, summary string) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary, Tags: tags}
	}
	create := reg("create-webhook", http.MethodPost, "/webhooks", "Create a webhook; the secret is in this response only")
	create.DefaultStatus = http.StatusCreated
	api.RegisterOn(a, group, reg("list-webhooks", http.MethodGet, "/webhooks", "The user's webhooks, newest first"), h.list)
	api.RegisterOn(a, group, create, h.create)
	api.RegisterOn(a, group, reg("get-webhook", http.MethodGet, "/webhooks/{id}", "One webhook"), h.show)
	api.RegisterOn(a, group, reg("update-webhook", http.MethodPut, "/webhooks/{id}", "Replace a webhook's name, URL, events and state; the secret stays"), h.update)
	api.RegisterOn(a, group, reg("delete-webhook", http.MethodDelete, "/webhooks/{id}", "Delete a webhook"), h.delete)
	api.RegisterOn(a, group, reg("test-webhook", http.MethodPost, "/webhooks/{id}/test", "Send a test payload to the webhook now"), h.test)
	api.RegisterOn(a, group, reg("list-webhook-deliveries", http.MethodGet, "/webhooks/{id}/deliveries", "The 50 latest deliveries of a webhook"), h.deliveries)
}

type handlers struct{ uc *app.UseCases }

type WebhookResponse struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Events    []string  `json:"events" nullable:"false"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type WebhookCreatedResponse struct {
	WebhookResponse
	Secret string `json:"secret" doc:"Signs every payload; it cannot be read again"`
}

type DeliveryResponse struct {
	ID          uint       `json:"id"`
	WebhookID   uint       `json:"webhook_id"`
	EventType   string     `json:"event_type"`
	StatusCode  *int       `json:"status_code" nullable:"true"`
	Error       *string    `json:"error" nullable:"true"`
	Attempts    int        `json:"attempts"`
	Status      string     `json:"status" doc:"pending, delivered or dead"`
	NextRetryAt *time.Time `json:"next_retry_at" nullable:"true"`
	CreatedAt   time.Time  `json:"created_at"`
}

type WebhookBody struct {
	Name     string   `json:"name" validate:"required,min=1,max=200"`
	URL      string   `json:"url" validate:"required,http_url"`
	Events   []string `json:"events" nullable:"false" validate:"required,min=1,dive,oneof=contact.created contact.updated contact.deleted contact.restored note.created note.updated note.deleted activity.created activity.updated activity.deleted reminder.triggered birthday.occurred"`
	IsActive bool     `json:"is_active"`
}

func (b WebhookBody) input() domain.Input {
	return domain.Input{Name: b.Name, URL: b.URL, Events: b.Events, Active: b.IsActive}
}

type CreateWebhookInput struct{ Body WebhookBody }

func (i *CreateWebhookInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type UpdateWebhookInput struct {
	ID   string `path:"id" doc:"The webhook's ID"`
	Body WebhookBody
}

func (i *UpdateWebhookInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

// WebhookPath takes the ID as text so that one that is not a number is a 400 of the API's
// own wording.
type WebhookPath struct {
	ID string `path:"id" doc:"The webhook's ID"`
}

type WebhookListOutput struct {
	Body struct {
		Webhooks []WebhookResponse `json:"webhooks" nullable:"false"`
	}
}

type WebhookOutput struct{ Body WebhookResponse }

type WebhookCreatedOutput struct{ Body WebhookCreatedResponse }

type WebhookMessageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

type WebhookTestOutput struct {
	Body struct {
		Delivery DeliveryResponse `json:"delivery"`
	}
}

type DeliveryListOutput struct {
	Body struct {
		Deliveries []DeliveryResponse `json:"deliveries" nullable:"false"`
	}
}

func actorOf(ctx context.Context) (domain.UserID, error) {
	id, ok := api.UserID(ctx)
	if !ok {
		return 0, apperrors.ErrUnauthorized("Authentication required")
	}
	return domain.UserID(id), nil
}

func webhookID(raw string) (domain.WebhookID, error) {
	id, err := idutil.Parse(raw)
	if err != nil {
		return 0, apperrors.ErrInvalidInput("id", "must be a positive integer")
	}
	return domain.WebhookID(id), nil
}

func response(w domain.Webhook) WebhookResponse {
	return WebhookResponse{ID: uint(w.ID), Name: w.Name, URL: w.URL, Events: w.Events, IsActive: w.Active, CreatedAt: w.CreatedAt}
}

func deliveryResponse(d domain.Delivery) DeliveryResponse {
	return DeliveryResponse{
		ID: d.ID, WebhookID: uint(d.WebhookID), EventType: d.EventType, StatusCode: d.StatusCode, Error: d.Error,
		Attempts: d.Attempts, Status: d.Status, NextRetryAt: d.NextRetryAt, CreatedAt: d.CreatedAt,
	}
}

func (h *handlers) list(ctx context.Context, _ *struct{}) (*WebhookListOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	list, err := h.uc.ListWebhooks(ctx, actor)
	if err != nil {
		return nil, toAppError(err)
	}
	out := &WebhookListOutput{}
	out.Body.Webhooks = make([]WebhookResponse, len(list))
	for i, w := range list {
		out.Body.Webhooks[i] = response(w)
	}
	return out, nil
}

func (h *handlers) create(ctx context.Context, in *CreateWebhookInput) (*WebhookCreatedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	w, secret, err := h.uc.CreateWebhook(ctx, actor, in.Body.input())
	if err != nil {
		return nil, toAppError(err)
	}
	return &WebhookCreatedOutput{Body: WebhookCreatedResponse{WebhookResponse: response(w), Secret: secret}}, nil
}

// actorAndID resolves the actor and the webhook ID of a request.
func actorAndID(ctx context.Context, raw string) (domain.UserID, domain.WebhookID, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return 0, 0, err
	}
	id, err := webhookID(raw)
	return actor, id, err
}

func (h *handlers) show(ctx context.Context, in *WebhookPath) (*WebhookOutput, error) {
	actor, id, err := actorAndID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	w, err := h.uc.ShowWebhook(ctx, actor, id)
	if err != nil {
		return nil, toAppError(err)
	}
	return &WebhookOutput{Body: response(w)}, nil
}

func (h *handlers) update(ctx context.Context, in *UpdateWebhookInput) (*WebhookOutput, error) {
	actor, id, err := actorAndID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	w, err := h.uc.UpdateWebhook(ctx, actor, id, in.Body.input())
	if err != nil {
		return nil, toAppError(err)
	}
	return &WebhookOutput{Body: response(w)}, nil
}

func (h *handlers) delete(ctx context.Context, in *WebhookPath) (*WebhookMessageOutput, error) {
	actor, id, err := actorAndID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if err := h.uc.DeleteWebhook(ctx, actor, id); err != nil {
		return nil, toAppError(err)
	}
	out := &WebhookMessageOutput{}
	out.Body.Message = "Webhook deleted"
	return out, nil
}

func (h *handlers) test(ctx context.Context, in *WebhookPath) (*WebhookTestOutput, error) {
	actor, id, err := actorAndID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	d, err := h.uc.TestWebhook(ctx, actor, id, api.Clock(ctx))
	if err != nil {
		return nil, toAppError(err)
	}
	out := &WebhookTestOutput{}
	out.Body.Delivery = deliveryResponse(d)
	return out, nil
}

func (h *handlers) deliveries(ctx context.Context, in *WebhookPath) (*DeliveryListOutput, error) {
	actor, id, err := actorAndID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	list, err := h.uc.WebhookDeliveries(ctx, actor, id)
	if err != nil {
		return nil, toAppError(err)
	}
	out := &DeliveryListOutput{}
	out.Body.Deliveries = make([]DeliveryResponse, len(list))
	for i, d := range list {
		out.Body.Deliveries[i] = deliveryResponse(d)
	}
	return out, nil
}

// toAppError words a use-case error the way the Gin handlers did.
func toAppError(err error) *apperrors.AppError {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apperrors.ErrNotFound("Webhook")
	case errors.Is(err, domain.ErrLimit):
		return apperrors.ErrConflict(fmt.Sprintf("maximum of %d webhooks per user reached", domain.MaxPerUser))
	}
	var f *app.Failure
	if errors.As(err, &f) {
		if f.Stage == app.StageGenerate {
			return apperrors.ErrInternal("secret generation failed").WithError(f.Err)
		}
		msg := map[app.Stage]string{
			app.StageList: "query", app.StageCount: "count", app.StageCreate: "insert", app.StageFind: "query",
			app.StageUpdate: "update", app.StageDelete: "delete", app.StageDeliveries: "query",
		}[f.Stage]
		if msg != "" {
			return apperrors.ErrDatabase(msg).WithError(f.Err)
		}
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
