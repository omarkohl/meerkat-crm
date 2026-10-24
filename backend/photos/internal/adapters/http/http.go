// Package httpadapter exposes the photos use cases over HTTP. The upload is a huma operation;
// the two downloads send image bytes with conditional-request support and stay plain Gin
// handlers, which keep sending errors in the AppError format (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/idutil"
	"meerkat/logger"
	"meerkat/photos/internal/app"
	"meerkat/photos/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

const path = "/contacts/{id}/profile_picture"

func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "upload-profile-picture", Method: http.MethodPost, Path: path,
		Summary: "Add an uploaded JPEG, PNG, HEIC, WebP or GIF image to a contact and make it the main image, cropped to a centred square", Tags: []string{"photos"},
	}, h.upload)
	if group == nil { // cmd/openapi describes operations only; the downloads are not
		return
	}
	group.GET(strings.ReplaceAll(strings.ReplaceAll(path, "{", ":"), "}", ""), h.download)
}

type handlers struct{ uc *app.UseCases }

type UploadForm struct {
	Photo huma.FormFile `form:"photo" required:"false" doc:"The image; leaving it out changes nothing"`
}

type UploadInput struct {
	ID      uint `path:"id" minimum:"1" doc:"The contact's ID"`
	RawBody huma.MultipartFormFiles[UploadForm]
}

type PhotoUploadedOutput struct {
	Body struct {
		Message string `json:"message"`
		Photo   string `json:"photo" doc:"The stored name of the new photo, empty when the contact has none"`
	}
}

func (h *handlers) upload(ctx context.Context, in *UploadInput) (*PhotoUploadedOutput, error) {
	if os.Getenv("DEMO_MODE") == "true" {
		return nil, apperrors.ErrForbidden("Photo uploads are disabled in demo mode")
	}
	actor, ok := api.UserID(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized("Authentication required")
	}
	var src io.Reader
	if f := in.RawBody.Data().Photo; f.IsSet {
		defer f.Close()
		src = f
	}
	photos, err := h.uc.UploadPhoto(ctx, domain.UserID(actor), domain.ContactID(in.ID), src)
	if err != nil {
		return nil, toAppError(err)
	}
	out := &PhotoUploadedOutput{}
	out.Body.Message = "Profile picture saved"
	out.Body.Photo = photos.Photo
	return out, nil
}

func (h *handlers) download(c *gin.Context) {
	contactID, err := idutil.Parse(c.Param("id"))
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrValidation("Invalid contact ID"))
		return
	}
	userID, ok := c.Get("userID")
	actor, isUint := userID.(uint)
	if !ok || !isUint {
		apperrors.AbortWithError(c, apperrors.ErrUnauthorized("Authentication required"))
		return
	}
	if c.Query("thumbnail") == "true" {
		h.thumbnail(c, domain.UserID(actor), domain.ContactID(contactID))
		return
	}
	photos, file, err := h.uc.OpenPhoto(c.Request.Context(), domain.UserID(actor), domain.ContactID(contactID))
	if err != nil {
		apperrors.AbortWithError(c, toAppError(err))
		return
	}
	defer file.Close()
	logger.FromContext(c).Debug().Str("file", photos.Photo).Msg("Serving profile picture")
	http.ServeContent(c.Writer, c.Request, photos.Photo, file.ModTime, file)
}

// thumbnail sends the thumbnail with an ETag and a long private cache: its URL carries the
// file's unique name, so a new picture is a new URL.
func (h *handlers) thumbnail(c *gin.Context, actor domain.UserID, id domain.ContactID) {
	t, err := h.uc.OpenThumbnail(c.Request.Context(), actor, id)
	if err != nil {
		apperrors.AbortWithError(c, toAppError(err))
		return
	}
	if t.Name == "" {
		c.Header("Cache-Control", "private, no-cache")
		c.Data(http.StatusOK, "image/jpeg", t.Legacy)
		return
	}
	defer t.File.Close()
	// The name is unique per picture; size and mtime guard against a file replaced in place.
	c.Header("ETag", fmt.Sprintf(`"%s-%x-%x"`, strings.TrimSuffix(t.Name, ".jpg"), t.File.Size, t.File.ModTime.UnixNano()))
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.Header("Content-Type", "image/jpeg")
	http.ServeContent(c.Writer, c.Request, "", t.File.ModTime, t.File)
}

func toAppError(err error) *apperrors.AppError {
	switch {
	case errors.Is(err, domain.ErrContactNotFound):
		return apperrors.ErrNotFound("Contact")
	case errors.Is(err, domain.ErrPhotoNotFound):
		return apperrors.ErrNotFound("Photo")
	case errors.Is(err, domain.ErrImageMissing):
		return apperrors.ErrNotFound("Image")
	case errors.Is(err, domain.ErrThumbnailNotFound):
		return apperrors.ErrNotFound("Thumbnail")
	case errors.Is(err, domain.ErrUnsafePhotoName):
		return apperrors.ErrValidation("Invalid photo path")
	case errors.Is(err, domain.ErrUnsafeThumbnailName):
		return apperrors.ErrValidation("Invalid thumbnail path")
	case errors.Is(err, domain.ErrBadLegacyThumbnail):
		return apperrors.ErrInternal("Invalid thumbnail format")
	case errors.Is(err, domain.ErrTooManyImages):
		return apperrors.ErrInvalidInput("photo", "A contact can have at most 50 images")
	case errors.Is(err, domain.ErrInvalidPhoto):
		return apperrors.ErrInvalidInput("photo", "must be a valid JPEG, PNG, HEIC, WebP or GIF image of reasonable size").WithError(err)
	}
	var f *app.Failure
	if errors.As(err, &f) {
		switch f.Stage {
		case app.StageFind:
			return apperrors.ErrDatabase("find").WithError(f.Err)
		case app.StageUpdate:
			return apperrors.ErrDatabase("update").WithError(f.Err)
		case app.StageProcess:
			if errors.Is(f.Err, domain.ErrLegacyThumbnailDecode) {
				return apperrors.ErrInternal("Failed to decode thumbnail").WithError(f.Err)
			}
			return apperrors.ErrInternal("Failed to process photo").WithError(f.Err)
		}
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
