package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"time"

	"meerkat/api"
	"meerkat/clock"
	apperrors "meerkat/errors"
	"meerkat/users/internal/app"
	"meerkat/users/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

type EnableTwoFactorBody struct {
	Password string `json:"password" validate:"required"`
	Code     string `json:"code" validate:"required,max=16"`
}

// ConfirmTwoFactorBody re-authenticates a sensitive change: the password plus an
// authenticator code or a recovery code.
type ConfirmTwoFactorBody struct {
	Password     string `json:"password" validate:"required"`
	Code         string `json:"code" validate:"omitempty,max=16"`
	RecoveryCode string `json:"recovery_code" validate:"omitempty,max=32"`
}

type EnableTwoFactorInput struct{ Body EnableTwoFactorBody }

func (i *EnableTwoFactorInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type ConfirmTwoFactorInput struct{ Body ConfirmTwoFactorBody }

func (i *ConfirmTwoFactorInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type TwoFactorStatusOutput struct {
	Body struct {
		Enabled                bool `json:"enabled"`
		RecoveryCodesRemaining int  `json:"recovery_codes_remaining"`
		Available              bool `json:"available" doc:"False for accounts without a password (single sign-on only)"`
	}
}

type TwoFactorSetupOutput struct {
	Body struct {
		Secret     string `json:"secret"`
		OtpauthURI string `json:"otpauth_uri" doc:"For the QR code the authenticator app scans"`
	}
}

type TwoFactorCodesOutput struct {
	Body struct {
		RecoveryCodes []string `json:"recovery_codes" doc:"Shown this once"`
	}
}

type TwoFactorMessageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// LockedResponse is the 429 answer to too many wrong second-factor attempts. Its body keeps
// the shape the login has always answered with, which is not the AppError format.
type LockedResponse struct {
	Err          string `json:"error"`
	Message      string `json:"message"`
	RetryAfter   int    `json:"retry_after"`
	RetryAfterAt string `json:"retry_after_at"`
}

func (e *LockedResponse) GetStatus() int { return http.StatusTooManyRequests }
func (e *LockedResponse) Error() string  { return e.Err }

func lockedResponse(seconds int, at time.Time) *LockedResponse {
	return &LockedResponse{
		Err: "Account temporarily locked", Message: "Too many failed attempts. Please try again later.",
		RetryAfter: seconds, RetryAfterAt: at.Add(time.Duration(seconds) * time.Second).Format(time.RFC3339),
	}
}

// documentLockout adds the 429 answer to the OpenAPI document, which huma cannot derive from
// an error value returned by a handler.
func documentLockout(a huma.API, paths ...string) {
	schema := huma.SchemaFromType(a.OpenAPI().Components.Schemas, reflect.TypeOf(LockedResponse{}))
	for _, path := range paths {
		a.OpenAPI().Paths[path].Post.Responses["429"] = &huma.Response{
			Description: "Too many wrong attempts: the account's second factor is locked for retry_after seconds",
			Content:     map[string]*huma.MediaType{"application/json": {Schema: schema}},
		}
	}
}

// realNow is the time codes are checked against. Authenticator apps compute them from the
// real time, so FAKE_NOW must not move it or two-factor users would be locked out.
func realNow() time.Time { return clock.Real().Now() }

func twoFactorActor(ctx context.Context) (domain.UserID, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return 0, err
	}
	if api.IsAPIToken(ctx) {
		return 0, apperrors.ErrForbidden("API tokens cannot manage two-factor authentication")
	}
	return actor, nil
}

func (h *handlers) twoFactorStatus(ctx context.Context, _ *struct{}) (*TwoFactorStatusOutput, error) {
	actor, err := twoFactorActor(ctx)
	if err != nil {
		return nil, err
	}
	st, err := h.uc.TwoFactorStatus(ctx, actor)
	if err != nil {
		return nil, twoFactorError(err)
	}
	out := &TwoFactorStatusOutput{}
	out.Body.Enabled, out.Body.RecoveryCodesRemaining, out.Body.Available = st.Enabled, st.CodesLeft, st.Available
	return out, nil
}

func (h *handlers) twoFactorSetup(ctx context.Context, _ *struct{}) (*TwoFactorSetupOutput, error) {
	actor, err := twoFactorActor(ctx)
	if err != nil {
		return nil, err
	}
	setup, err := h.uc.BeginTwoFactorSetup(ctx, actor)
	if err != nil {
		return nil, twoFactorError(err)
	}
	out := &TwoFactorSetupOutput{}
	out.Body.Secret, out.Body.OtpauthURI = setup.Secret, setup.URI
	return out, nil
}

func (h *handlers) twoFactorEnable(ctx context.Context, in *EnableTwoFactorInput) (*TwoFactorCodesOutput, error) {
	actor, err := twoFactorActor(ctx)
	if err != nil {
		return nil, err
	}
	changed, err := h.uc.EnableTwoFactor(ctx, actor, in.Body.Password, in.Body.Code, api.SessionID(ctx), client(ctx), api.Now(ctx), realNow())
	if err != nil {
		return nil, twoFactorError(err)
	}
	logSignOut(ctx, actor, changed.SignOutErr)
	out := &TwoFactorCodesOutput{}
	out.Body.RecoveryCodes = changed.RecoveryCodes
	return out, nil
}

func (h *handlers) twoFactorDisable(ctx context.Context, in *ConfirmTwoFactorInput) (*TwoFactorMessageOutput, error) {
	actor, err := twoFactorActor(ctx)
	if err != nil {
		return nil, err
	}
	b := in.Body
	changed, err := h.uc.DisableTwoFactor(ctx, actor, b.Password, b.Code, b.RecoveryCode, api.SessionID(ctx), client(ctx), api.Now(ctx), realNow())
	if err != nil {
		return nil, twoFactorError(err)
	}
	logSignOut(ctx, actor, changed.SignOutErr)
	out := &TwoFactorMessageOutput{}
	out.Body.Message = "Two-factor authentication disabled"
	return out, nil
}

func (h *handlers) twoFactorRecoveryCodes(ctx context.Context, in *ConfirmTwoFactorInput) (*TwoFactorCodesOutput, error) {
	actor, err := twoFactorActor(ctx)
	if err != nil {
		return nil, err
	}
	b := in.Body
	codes, err := h.uc.RegenerateRecoveryCodes(ctx, actor, b.Password, b.Code, b.RecoveryCode, client(ctx), realNow())
	if err != nil {
		return nil, twoFactorError(err)
	}
	out := &TwoFactorCodesOutput{}
	out.Body.RecoveryCodes = codes
	return out, nil
}

func logSignOut(ctx context.Context, actor domain.UserID, err error) {
	if err != nil {
		api.Logger(ctx).Error().Err(err).Uint("user_id", uint(actor)).Msg("Failed to revoke other sessions after a two-factor change")
	}
}

func twoFactorError(err error) error {
	var locked *domain.LockedError
	var f *app.Failure
	switch {
	case errors.As(err, &locked):
		return lockedResponse(locked.RetryAfterSeconds, realNow())
	case errors.Is(err, domain.ErrUserNotFound):
		return apperrors.ErrNotFound("User")
	case errors.Is(err, domain.ErrTwoFactorEnabled):
		return apperrors.ErrConflict("Two-factor authentication is already enabled")
	case errors.Is(err, domain.ErrTwoFactorNotAvailable):
		return apperrors.ErrInvalidInput("", "Two-factor authentication is not available for single sign-on accounts")
	case errors.Is(err, domain.ErrTwoFactorNotEnabled):
		return apperrors.ErrInvalidInput("", "Two-factor authentication is not enabled")
	case errors.Is(err, domain.ErrWrongPassword):
		return apperrors.ErrInvalidInput("password", "Password is incorrect")
	case errors.Is(err, domain.ErrInvalidVerificationCode):
		return apperrors.ErrInvalidInput("code", "Invalid verification code")
	case errors.As(err, &f):
		switch f.Stage {
		case app.StageSetup:
			return apperrors.ErrInternal("Could not start two-factor setup").WithError(f.Err)
		case app.StageEnable:
			return apperrors.ErrInternal("Could not enable two-factor authentication").WithError(f.Err)
		case app.StageFactor:
			return apperrors.ErrDatabase("update").WithError(f.Err)
		case app.StageQuery:
			return apperrors.ErrDatabase("query user").WithError(f.Err)
		}
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
