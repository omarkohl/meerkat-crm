package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"os"
	"reflect"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/middleware"
	"meerkat/users/internal/app"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

// registerPublic adds the operations anyone can call, behind the strict auth rate limit.
func registerPublic(a huma.API, public gin.IRoutes, h *handlers) {
	limit := middleware.AuthRateLimitMiddleware()
	op := func(id, path, summary string) huma.Operation {
		return huma.Operation{OperationID: id, Method: http.MethodPost, Path: path, Summary: summary, Tags: []string{"auth"}}
	}
	api.RegisterOn(a, public, op("check-password-strength", "/check-password-strength", "Rate a password without registering"), h.passwordStrength, limit)
	api.RegisterOn(a, public, op("request-password-reset", "/password-reset/request", "Mail reset instructions to the account with this email; the answer is the same for unknown emails"), h.requestReset, limit)
	api.RegisterOn(a, public, op("confirm-password-reset", "/password-reset/confirm", "Set a new password with a reset token and end every session"), h.confirmReset, limit)
	documentPublicErrors(a)
}

// RateLimitedResponse is the 429 of the rate limit middleware, which is not in the AppError
// format.
type RateLimitedResponse struct {
	Err     string `json:"error"`
	Message string `json:"message"`
}

// documentPublicErrors adds what the middleware and the demo mode answer, which huma cannot
// derive from the handlers.
func documentPublicErrors(a huma.API) {
	limited := huma.SchemaFromType(a.OpenAPI().Components.Schemas, reflect.TypeOf(RateLimitedResponse{}))
	paths := a.OpenAPI().Paths
	for _, path := range []string{"/check-password-strength", "/password-reset/request", "/password-reset/confirm"} {
		responses := paths[path].Post.Responses
		responses["429"] = &huma.Response{
			Description: "Too many requests from this address",
			Content:     map[string]*huma.MediaType{"application/json": {Schema: limited}},
		}
		if path != "/check-password-strength" {
			forbidden := *responses["default"]
			forbidden.Description = "Password changes are disabled in demo mode"
			responses["403"] = &forbidden
		}
	}
}

// PasswordBody has no required tag: a missing password is answered as a missing field.
type PasswordBody struct {
	Password string `json:"password"`
}

type PasswordStrengthInput struct{ Body PasswordBody }

type PasswordStrengthOutput struct {
	Body struct {
		IsValid     bool    `json:"is_valid"`
		Entropy     float64 `json:"entropy"`
		Score       int     `json:"score" doc:"0 (very weak) to 4 (very strong)"`
		Feedback    string  `json:"feedback"`
		MinEntropy  float64 `json:"min_entropy"`
		CharSetSize int     `json:"char_set_size"`
		Length      int     `json:"length"`
	}
}

type ResetRequestBody struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetRequestInput struct{ Body ResetRequestBody }

func (i *ResetRequestInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type ResetConfirmBody struct {
	Token    string `json:"token" validate:"required,min=16"`
	Password string `json:"password" validate:"required,min=8,strong_password"`
}

type ResetConfirmInput struct{ Body ResetConfirmBody }

func (i *ResetConfirmInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type PasswordResetOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func demoRefusal() error {
	if os.Getenv("DEMO_MODE") == "true" {
		return apperrors.ErrForbidden("Password changes are disabled in demo mode")
	}
	return nil
}

func (h *handlers) passwordStrength(_ context.Context, in *PasswordStrengthInput) (*PasswordStrengthOutput, error) {
	if in.Body.Password == "" {
		return nil, apperrors.ErrMissingField("password")
	}
	s := h.uc.PasswordStrength(in.Body.Password)
	out := &PasswordStrengthOutput{}
	out.Body.IsValid, out.Body.Entropy, out.Body.Score, out.Body.Feedback = s.IsValid, s.Entropy, s.Score, s.Feedback
	out.Body.MinEntropy, out.Body.CharSetSize, out.Body.Length = s.MinEntropy, s.CharSetSize, s.Length
	return out, nil
}

func (h *handlers) requestReset(ctx context.Context, in *ResetRequestInput) (*PasswordResetOutput, error) {
	if err := demoRefusal(); err != nil {
		return nil, err
	}
	if err := h.uc.RequestPasswordReset(ctx, in.Body.Email, api.Now(ctx)); err != nil {
		return nil, toAppError(err, "")
	}
	out := &PasswordResetOutput{}
	out.Body.Message = "If an account exists, password reset instructions were sent"
	return out, nil
}

func (h *handlers) confirmReset(ctx context.Context, in *ResetConfirmInput) (*PasswordResetOutput, error) {
	if err := demoRefusal(); err != nil {
		return nil, err
	}
	err := h.uc.ConfirmPasswordReset(ctx, in.Body.Token, in.Body.Password, client(ctx), api.Now(ctx))
	if err != nil {
		var rejected *app.ResetTokenRejected
		if errors.As(err, &rejected) && rejected.ClearErr != nil {
			api.Logger(ctx).Error().Err(rejected.ClearErr).Msg("Failed to clear expired reset token")
		}
		return nil, toAppError(err, "")
	}
	out := &PasswordResetOutput{}
	out.Body.Message = "Password reset successful"
	return out, nil
}
