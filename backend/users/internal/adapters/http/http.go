// Package httpadapter exposes the users use cases as huma operations. It maps HTTP to the use
// cases and their results and errors back to the responses the API always sent
// (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/users/internal/app"
	"meerkat/users/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	tags := []string{"users"}
	reg := func(id, method, path, summary string) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary, Tags: tags}
	}
	api.RegisterOn(a, group, reg("update-language", http.MethodPatch, "/users/language", "Set the language of the user"), h.language)
	api.RegisterOn(a, group, reg("update-date-format", http.MethodPatch, "/users/date-format", "Set the date format of the user"), h.dateFormat)
	api.RegisterOn(a, group, reg("get-custom-field-names", http.MethodGet, "/users/custom-fields", "The names of the user's custom contact fields"), h.customFields)
	api.RegisterOn(a, group, reg("update-custom-field-names", http.MethodPatch, "/users/custom-fields", "Replace the names of the user's custom contact fields"), h.setCustomFields)
	api.RegisterOn(a, group, reg("get-enabled-contact-fields", http.MethodGet, "/users/enabled-contact-fields", "The extended contact fields the user shows; null when never configured"), h.enabledFields)
	api.RegisterOn(a, group, reg("update-enabled-contact-fields", http.MethodPatch, "/users/enabled-contact-fields", "Replace the extended contact fields the user shows"), h.setEnabledFields)
}

type handlers struct{ uc *app.UseCases }

type LanguageBody struct {
	Language string `json:"language"`
}

type DateFormatBody struct {
	DateFormat string `json:"date_format"`
}

type CustomFieldsBody struct {
	Names []string `json:"names" validate:"dive,max=100"`
}

type EnabledFieldsBody struct {
	Fields []string `json:"fields" validate:"dive,max=50"`
}

type LanguageInput struct{ Body LanguageBody }

type DateFormatInput struct{ Body DateFormatBody }

type CustomFieldsInput struct{ Body CustomFieldsBody }

func (i *CustomFieldsInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type EnabledFieldsInput struct{ Body EnabledFieldsBody }

func (i *EnabledFieldsInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type UserLanguageOutput struct {
	Body struct {
		Message  string `json:"message"`
		Language string `json:"language"`
	}
}

type UserDateFormatOutput struct {
	Body struct {
		Message    string `json:"message"`
		DateFormat string `json:"date_format"`
	}
}

type UserCustomFieldsOutput struct {
	Body struct {
		CustomFieldNames []string `json:"custom_field_names"`
	}
}

type UserCustomFieldsSavedOutput struct {
	Body struct {
		Message          string   `json:"message"`
		CustomFieldNames []string `json:"custom_field_names" nullable:"true"`
	}
}

type UserEnabledFieldsOutput struct {
	Body struct {
		EnabledContactFields []string `json:"enabled_contact_fields" nullable:"true"`
	}
}

type UserEnabledFieldsSavedOutput struct {
	Body struct {
		Message              string   `json:"message"`
		EnabledContactFields []string `json:"enabled_contact_fields" nullable:"true"`
	}
}

func actorOf(ctx context.Context) (domain.UserID, error) {
	id, ok := api.UserID(ctx)
	if !ok {
		return 0, apperrors.ErrUnauthorized("Authentication required")
	}
	return domain.UserID(id), nil
}

func (h *handlers) language(ctx context.Context, in *LanguageInput) (*UserLanguageOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.SetLanguage(ctx, actor, in.Body.Language); err != nil {
		return nil, toAppError(err, "language")
	}
	out := &UserLanguageOutput{}
	out.Body.Message, out.Body.Language = "Language updated successfully", in.Body.Language
	return out, nil
}

func (h *handlers) dateFormat(ctx context.Context, in *DateFormatInput) (*UserDateFormatOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.SetDateFormat(ctx, actor, in.Body.DateFormat); err != nil {
		return nil, toAppError(err, "date_format")
	}
	out := &UserDateFormatOutput{}
	out.Body.Message, out.Body.DateFormat = "Date format updated successfully", in.Body.DateFormat
	return out, nil
}

func (h *handlers) customFields(ctx context.Context, _ *struct{}) (*UserCustomFieldsOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	names, err := h.uc.CustomFieldNames(ctx, actor)
	if err != nil {
		return nil, toAppError(err, "")
	}
	out := &UserCustomFieldsOutput{}
	out.Body.CustomFieldNames = names
	return out, nil
}

func (h *handlers) setCustomFields(ctx context.Context, in *CustomFieldsInput) (*UserCustomFieldsSavedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.SetCustomFieldNames(ctx, actor, in.Body.Names); err != nil {
		return nil, toAppError(err, "")
	}
	out := &UserCustomFieldsSavedOutput{}
	out.Body.Message, out.Body.CustomFieldNames = "Custom field names updated successfully", in.Body.Names
	return out, nil
}

func (h *handlers) enabledFields(ctx context.Context, _ *struct{}) (*UserEnabledFieldsOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	fields, err := h.uc.EnabledContactFields(ctx, actor)
	if err != nil {
		return nil, toAppError(err, "")
	}
	out := &UserEnabledFieldsOutput{}
	out.Body.EnabledContactFields = fields
	return out, nil
}

func (h *handlers) setEnabledFields(ctx context.Context, in *EnabledFieldsInput) (*UserEnabledFieldsSavedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.SetEnabledContactFields(ctx, actor, in.Body.Fields); err != nil {
		return nil, toAppError(err, "")
	}
	out := &UserEnabledFieldsSavedOutput{}
	out.Body.Message, out.Body.EnabledContactFields = "Enabled contact fields updated successfully", in.Body.Fields
	return out, nil
}

// toAppError words a use-case error the way the Gin handlers did; field names the request
// field an unsupported value belongs to.
func toAppError(err error, field string) *apperrors.AppError {
	var unsupported *domain.UnsupportedError
	switch {
	case errors.As(err, &unsupported):
		return apperrors.ErrInvalidInput(field, "Unsupported "+unsupported.What+". Supported: "+strings.Join(unsupported.Supported, ", "))
	case errors.Is(err, domain.ErrUserNotFound):
		return apperrors.ErrUnauthorized("Authentication required")
	}
	var f *app.Failure
	if errors.As(err, &f) {
		if f.Stage == app.StageQuery {
			return apperrors.ErrDatabase("query user").WithError(f.Err)
		}
		return apperrors.ErrDatabase("update user").WithError(f.Err)
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
