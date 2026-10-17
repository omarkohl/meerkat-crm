package httpadapter

import (
	"context"
	"errors"
	"net/http"

	"meerkat/api"
	"meerkat/logger"
	"meerkat/middleware"
	"meerkat/services"
	"meerkat/users/internal/app"
	"meerkat/users/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

type OIDCSettings struct {
	Enabled     bool
	ProviderURL string
	Routes      bool
}

const (
	oidcCallbackPath = "/api/v1/auth/oidc/callback"
	oidcCookieAge    = 600
	afterLogin       = "/"
	deniedURL        = "/login?error=oidc_denied"
	noAccountURL     = "/login?error=oidc_no_account"
	failedURL        = "/login?error=oidc_error"
)

type OIDCConfigOutput struct {
	Body struct {
		Enabled      bool   `json:"enabled" doc:"Whether single sign-on is configured"`
		ProviderName string `json:"provider_name,omitempty" doc:"A display name for the provider, present when enabled"`
	}
}

func registerOIDC(a huma.API, public gin.IRoutes, h *handlers, s OIDCSettings) {
	api.RegisterOn(a, public, huma.Operation{
		OperationID: "get-oidc-config", Method: http.MethodGet, Path: "/auth/oidc/config",
		Summary: "Single sign-on status", Tags: []string{"auth"},
	}, func(context.Context, *struct{}) (*OIDCConfigOutput, error) {
		out := &OIDCConfigOutput{}
		out.Body.Enabled = s.Enabled
		if s.Enabled {
			out.Body.ProviderName = services.ProviderName(s.ProviderURL)
		}
		return out, nil
	})
	if !s.Routes || public == nil {
		return
	}
	public.GET("/auth/oidc/login", middleware.AuthRateLimitMiddleware(), h.oidcLogin)
	public.GET("/auth/oidc/callback", middleware.AuthRateLimitMiddleware(), h.oidcCallback)
}

func (h *handlers) setOIDCCookie(c *gin.Context, name, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, maxAge, oidcCallbackPath, h.cookie.Domain, h.cookie.Secure, true)
}

func (h *handlers) oidcLogin(c *gin.Context) {
	start, err := h.uc.StartOIDC()
	if err != nil {
		var f *app.Failure
		message := "failed to generate state"
		if errors.As(err, &f) && f.Stage == app.StageNonce {
			message = "failed to generate nonce"
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": message})
		return
	}
	h.setOIDCCookie(c, "oidc_state", start.State, oidcCookieAge)
	h.setOIDCCookie(c, "oidc_nonce", start.Nonce, oidcCookieAge)
	c.Redirect(http.StatusFound, start.URL)
}

func (h *handlers) oidcCallback(c *gin.Context) {
	log := logger.FromContext(c)
	cb := domain.OIDCCallback{ProviderError: c.Query("error"), State: c.Query("state"), Code: c.Query("code")}
	// A provider error ends the attempt before the cookies are looked at, and leaves them.
	if cb.ProviderError == "" {
		cb.StateCookie, _ = c.Cookie("oidc_state")
		cb.NonceCookie, _ = c.Cookie("oidc_nonce")
		h.setOIDCCookie(c, "oidc_state", "", -1)
		h.setOIDCCookie(c, "oidc_nonce", "", -1)
	}

	done, err := h.uc.CompleteOIDC(c.Request.Context(), cb, domain.Client{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}, middleware.RequestNow(c))
	var failure *domain.OIDCFailure
	switch {
	case errors.Is(err, domain.ErrOIDCDenied):
		c.Redirect(http.StatusFound, deniedURL)
	case errors.Is(err, domain.ErrOIDCNoAccount):
		c.Redirect(http.StatusFound, noAccountURL)
	case errors.As(err, &failure):
		if failure.Warn {
			log.Warn().Msg("OIDC callback: " + failure.Reason)
		} else {
			log.Error().Err(failure.Err).Msg("OIDC: " + failure.Reason)
		}
		c.Redirect(http.StatusFound, failedURL)
	default:
		if done.EnrichErr != nil {
			log.Warn().Err(done.EnrichErr).Msg("OIDC: userinfo lookup failed, continuing with id_token claims only")
		}
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(authCookie, done.Token, h.cookie.MaxAge, "/", h.cookie.Domain, h.cookie.Secure, true)
		c.Redirect(http.StatusFound, afterLogin)
	}
}
