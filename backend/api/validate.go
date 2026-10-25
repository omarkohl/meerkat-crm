package api

import (
	"meerkat/middleware"

	"github.com/danielgtaylor/huma/v2"
)

// ValidateBody runs the project's validator (go-playground `validate` tags, including the seven
// custom ones: phone, birthday, deceased_date, strong_password, unique_circles, no_at_sign, safeurl) on
// v, the Body field of an operation's input. Call it from the Resolve method of the input
// struct, so huma reports the errors in the AppError format:
//
//	func (i *In) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
//		return api.ValidateBody(prefix, &i.Body)
//	}
func ValidateBody(prefix *huma.PathBuffer, v any) []error {
	base := prefix.With("body")
	var errs []error
	for _, ve := range middleware.ValidateStructJSON(v) {
		errs = append(errs, &huma.ErrorDetail{
			Message:  ve.Message,
			Location: base + "." + ve.Field,
		})
	}
	return errs
}
