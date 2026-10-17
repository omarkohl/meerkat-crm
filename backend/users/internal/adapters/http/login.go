package httpadapter

import (
	"context"
	"errors"
	"net/http"

	"meerkat/api"
	"meerkat/clock"
	apperrors "meerkat/errors"
	"meerkat/middleware"
	"meerkat/users/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

func registerLogin(a huma.API, public gin.IRoutes, h *handlers) {
	api.RegisterOn(a, public, huma.Operation{
		OperationID: "login", Method: http.MethodPost, Path: "/login", Tags: []string{"auth"},
		Summary: "Sign in with a username or email and the password, and a code when two-factor is on; the token comes in the auth_token cookie",
	}, h.login, middleware.AuthRateLimitMiddleware())
	api.RegisterOn(a, public, huma.Operation{
		OperationID: "logout", Method: http.MethodPost, Path: "/logout", Tags: []string{"auth"},
		Summary: "End the session named by the auth_token cookie, if any, and clear the cookie",
	}, h.logout)
	documentLoginLockout(a)
}

// documentLoginLockout adds the 429 of /login. The address rate limit answers with the same
// error and message keys, without the retry fields.
func documentLoginLockout(a huma.API) {
	documentLockout(a, "/login")
	a.OpenAPI().Paths["/login"].Post.Responses["429"].Description = "The identifier or the account's second factor is locked for retry_after seconds, or this address sent too many requests"
}

// LoginBody leaves every field optional: the missing ones are named by the use case.
type LoginBody struct {
	Identifier   string `json:"identifier,omitempty" doc:"Username or email"`
	Email        string `json:"email,omitempty" doc:"Legacy name of identifier"`
	Password     string `json:"password,omitempty"`
	TOTPCode     string `json:"totp_code,omitempty" doc:"Second step for accounts with two-factor: an authenticator code"`
	RecoveryCode string `json:"recovery_code,omitempty" doc:"Second step: a recovery code, in place of the code"`
}

type LoginInput struct{ Body LoginBody }

type LoginOutput struct {
	Body struct {
		Language     *string `json:"language,omitempty"`
		DateFormat   *string `json:"date_format,omitempty"`
		TOTPRequired bool    `json:"totp_required,omitempty" doc:"True when the password was right and the second step is still due"`
	}
}

type LogoutInput struct {
	Token string `cookie:"auth_token"`
}

func (h *handlers) login(ctx context.Context, in *LoginInput) (*LoginOutput, error) {
	b := in.Body
	identifier := b.Identifier
	if identifier == "" {
		identifier = b.Email
	}
	creds := domain.Credentials{Identifier: identifier, Password: b.Password, TOTPCode: b.TOTPCode, RecoveryCode: b.RecoveryCode}
	done, err := h.uc.Login(ctx, creds, client(ctx), api.Now(ctx), clock.Real().Now())
	if err != nil {
		return nil, loginError(ctx, err)
	}
	out := &LoginOutput{}
	if done.SecondFactorRequired {
		out.Body.TOTPRequired = true
		return out, nil
	}
	h.cookie.set(ctx, done.Token, h.cookie.MaxAge)
	out.Body.Language, out.Body.DateFormat = &done.Language, &done.DateFormat
	return out, nil
}

func (h *handlers) logout(ctx context.Context, in *LogoutInput) (*SessionMessageOutput, error) {
	h.uc.Logout(ctx, in.Token, client(ctx), api.Now(ctx))
	h.cookie.clear(ctx)
	out := &SessionMessageOutput{}
	out.Body.Message = "Logged out successfully"
	return out, nil
}

// loginError words a login failure; the lock on the typed identifier answers in its own
// words and counts from the request clock.
func loginError(ctx context.Context, err error) error {
	var locked *domain.LockedError
	var missing *domain.MissingFieldError
	switch {
	case errors.As(err, &locked) && locked.FromLogin:
		return lockedResponse(loginLockedMessage, locked.RetryAfterSeconds, api.Now(ctx))
	case errors.As(err, &missing):
		return apperrors.ErrMissingField(missing.Field)
	case errors.Is(err, domain.ErrInvalidCredentials):
		return apperrors.ErrInvalidCredentials()
	case errors.Is(err, domain.ErrInvalidSecondFactor):
		return apperrors.ErrInvalidTOTPCode()
	}
	return accountError(err)
}
