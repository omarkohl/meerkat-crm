package httpadapter

import (
	"context"
	"net/http"
	"os"
	"time"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/users/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

// Cookie is how the login cookie is set; the values come from the configuration.
type Cookie struct {
	Domain   string
	Secure   bool
	MaxAge   int
	Issuable bool // a signing key is configured, so a new session can start
}

const authCookie = "auth_token"

func (c Cookie) set(ctx context.Context, token string, maxAge int) {
	api.SetCookie(ctx, &http.Cookie{ //nolint:gosec // Secure follows COOKIE_SECURE, which is off for plain-HTTP installs
		Name: authCookie, Value: token, MaxAge: maxAge, Path: "/", Domain: c.Domain,
		Secure: c.Secure, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

func (c Cookie) clear(ctx context.Context) { c.set(ctx, "", -1) }

type SessionResponse struct {
	ID         string    `json:"id"`
	UserAgent  string    `json:"user_agent"`
	IP         string    `json:"ip"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	Current    bool      `json:"current" doc:"The session of this request"`
}

type SessionPath struct {
	ID string `path:"id" doc:"The session's ID"`
}

type ChangePasswordBody struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8,strong_password"`
}

type ChangePasswordInput struct{ Body ChangePasswordBody }

func (i *ChangePasswordInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

type SessionListOutput struct {
	Body struct {
		Sessions []SessionResponse `json:"sessions"`
	}
}

type SessionMessageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// refuseAPIToken answers 403: sessions belong to browser logins, and a token must not be
// able to sign the owner out.
func refuseAPIToken(ctx context.Context) error {
	if api.IsAPIToken(ctx) {
		return apperrors.ErrForbidden("API tokens cannot manage sessions")
	}
	return nil
}

func client(ctx context.Context) domain.Client {
	return domain.Client{IP: api.ClientIP(ctx), UserAgent: api.UserAgent(ctx)}
}

func (h *handlers) sessionsActor(ctx context.Context) (domain.UserID, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return 0, err
	}
	return actor, refuseAPIToken(ctx)
}

func (h *handlers) sessions(ctx context.Context, _ *struct{}) (*SessionListOutput, error) {
	actor, err := h.sessionsActor(ctx)
	if err != nil {
		return nil, err
	}
	list, err := h.uc.ListSessions(ctx, actor, api.SessionID(ctx), api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, "")
	}
	out := &SessionListOutput{}
	out.Body.Sessions = make([]SessionResponse, len(list))
	for i, s := range list {
		out.Body.Sessions[i] = SessionResponse{
			ID: s.ID, UserAgent: s.UserAgent, IP: s.IP, LastSeenAt: s.LastSeenAt, ExpiresAt: s.ExpiresAt,
			CreatedAt: s.CreatedAt, Current: s.Current,
		}
	}
	return out, nil
}

func (h *handlers) revoke(ctx context.Context, in *SessionPath) (*SessionMessageOutput, error) {
	actor, err := h.sessionsActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.RevokeSession(ctx, actor, in.ID, client(ctx), api.Now(ctx)); err != nil {
		return nil, toAppError(err, "")
	}
	if in.ID == api.SessionID(ctx) {
		h.cookie.clear(ctx)
	}
	out := &SessionMessageOutput{}
	out.Body.Message = "Session revoked"
	return out, nil
}

func (h *handlers) signOutEverywhere(ctx context.Context, _ *struct{}) (*SessionMessageOutput, error) {
	actor, err := h.sessionsActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.SignOutEverywhere(ctx, actor, client(ctx), api.Now(ctx)); err != nil {
		return nil, toAppError(err, "")
	}
	h.cookie.clear(ctx)
	out := &SessionMessageOutput{}
	out.Body.Message = "Signed out everywhere"
	return out, nil
}

func (h *handlers) changePassword(ctx context.Context, in *ChangePasswordInput) (*SessionMessageOutput, error) {
	if os.Getenv("DEMO_MODE") == "true" {
		return nil, apperrors.ErrForbidden("Password changes are disabled in demo mode")
	}
	actor, err := sessionActorOf(ctx)
	if err != nil {
		return nil, err
	}
	startSession := h.cookie.Issuable && !api.IsAPIToken(ctx)
	changed, err := h.uc.ChangePassword(ctx, actor, in.Body.CurrentPassword, in.Body.NewPassword, startSession, client(ctx), api.Now(ctx))
	if err != nil {
		return nil, toAppError(err, "")
	}
	if changed.SessionErr != nil {
		api.Logger(ctx).Error().Err(changed.SessionErr).Msg("Failed to start a new session after password change")
	} else if changed.Token != "" {
		h.cookie.set(ctx, changed.Token, h.cookie.MaxAge)
	}
	out := &SessionMessageOutput{}
	out.Body.Message = "Password updated successfully"
	return out, nil
}
