// Package httpadapter serves the two export downloads. They send a file, not JSON, so they are
// plain Gin handlers (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	apperrors "meerkat/errors"
	"meerkat/exports/internal/app"
	"meerkat/exports/internal/domain"
	"meerkat/idutil"
	"meerkat/logger"
	"meerkat/middleware"

	"github.com/gin-gonic/gin"
)

func Register(group gin.IRoutes, uc *app.UseCases) {
	if group == nil {
		return
	}
	h := &handlers{uc: uc}
	group.GET("/export", h.csv)
	group.GET("/export/vcf", h.vcf)
	group.GET("/contacts/:id/vcf", h.contactVCF)
}

type handlers struct{ uc *app.UseCases }

func (h *handlers) csv(c *gin.Context) {
	h.serve(c, h.uc.ExportCSV, "Data export completed successfully")
}

func (h *handlers) vcf(c *gin.Context) {
	h.serve(c, h.uc.ExportVCF, "VCF export completed successfully")
}

func (h *handlers) contactVCF(c *gin.Context) {
	id := c.Param("id")
	contactID, err := idutil.Parse(id)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact").WithDetails("id", id))
		return
	}
	h.serve(c, func(ctx context.Context, actor domain.UserID, _ time.Time, client domain.Client) (app.Document, error) {
		return h.uc.ExportContactVCF(ctx, actor, contactID, client)
	}, "Single contact VCF export completed successfully")
}

type export func(ctx context.Context, actor domain.UserID, now time.Time, client domain.Client) (app.Document, error)

func (h *handlers) serve(c *gin.Context, run export, done string) {
	v, _ := c.Get("userID")
	actor, ok := v.(uint)
	if !ok {
		apperrors.AbortWithError(c, apperrors.ErrUnauthorized("Authentication required"))
		return
	}
	doc, err := run(c.Request.Context(), domain.UserID(actor), middleware.RequestNow(c), domain.Client{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()})
	if err != nil {
		logger.FromContext(c).Error().Err(err).Msg("Export failed")
		apperrors.AbortWithError(c, toAppError(err))
		return
	}
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", doc.Filename))
	c.Header("Content-Length", fmt.Sprintf("%d", len(doc.Body)))
	c.Data(http.StatusOK, doc.ContentType, doc.Body)
	log := logger.FromContext(c)
	for _, id := range doc.Skipped {
		log.Error().Uint("contact_id", id).Msg("Failed to encode contact as vCard")
	}
	log.Info().Int("contacts", doc.Contacts).Int("activities", doc.Activities).Int("notes", doc.Notes).Int("reminders", doc.Reminders).Msg(done)
}

func toAppError(err error) *apperrors.AppError {
	if errors.Is(err, domain.ErrContactNotFound) {
		return apperrors.ErrNotFound("Contact")
	}
	var fetch *app.FetchError
	if errors.As(err, &fetch) {
		return apperrors.ErrInternal("Failed to fetch " + fetch.What)
	}
	var gen *app.GenerateError
	if errors.As(err, &gen) {
		return apperrors.ErrInternal("Failed to generate export")
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
