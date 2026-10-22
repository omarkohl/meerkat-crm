// Package httpadapter exposes the images use cases over HTTP. The upload is a huma operation;
// the download sends image bytes with conditional-request support and stays a plain Gin
// handler, which keeps sending errors in the AppError format (docs/adr/0012-module-layout.md).
package httpadapter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/idutil"
	"meerkat/images/internal/app"
	"meerkat/images/internal/domain"
	"meerkat/middleware"
	"meerkat/models"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

func Register(a huma.API, group gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "upload-image", Method: http.MethodPost, Path: "/images", DefaultStatus: http.StatusCreated,
		MaxBodyBytes: middleware.MaxUploadBodySize,
		Summary:      "Upload a JPEG, PNG, HEIC, WebP or GIF image; it stays pending until a note, activity or contact claims it",
		Tags:         []string{"images"},
	}, h.upload, middleware.ImageUploadRateLimitMiddleware())
	if group == nil { // cmd/openapi describes operations only; the download is not
		return
	}
	group.GET("/images/:id/:variant", h.download)
}

type handlers struct{ uc *app.UseCases }

type UploadForm struct {
	File huma.FormFile `form:"file" required:"true" doc:"The image, at most 25 MB"`
}

type UploadInput struct {
	RawBody huma.MultipartFormFiles[UploadForm]
}

type Crop struct {
	X    int `json:"x"`
	Y    int `json:"y"`
	Size int `json:"size"`
}

type ImageBody struct {
	ID       uint       `json:"id"`
	Width    int        `json:"width"`
	Height   int        `json:"height"`
	Mime     string     `json:"mime" example:"image/webp"`
	Bytes    int64      `json:"bytes"`
	TakenAt  *time.Time `json:"taken_at" doc:"Capture time from the picture's metadata, if any"`
	Crop     *Crop      `json:"crop" doc:"Square face crop in pixels of the full image; contacts only"`
	URL      string     `json:"url" doc:"The full image"`
	ThumbURL string     `json:"thumb_url" doc:"A small version, 480 px on the long edge"`
}

type UploadOutput struct{ Body ImageBody }

func body(img domain.Image) ImageBody {
	out := ImageBody{
		ID: uint(img.ID), Width: img.Width, Height: img.Height, Mime: img.Mime, Bytes: img.Bytes, TakenAt: img.TakenAt,
		URL:      models.ImageURL(uint(img.ID), img.File, string(domain.VariantImage)),
		ThumbURL: models.ImageURL(uint(img.ID), img.File, string(domain.VariantThumb)),
	}
	if img.Crop != nil {
		out.Crop = &Crop{X: img.Crop.X, Y: img.Crop.Y, Size: img.Crop.Size}
	}
	return out
}

func (h *handlers) upload(ctx context.Context, in *UploadInput) (*UploadOutput, error) {
	if os.Getenv("DEMO_MODE") == "true" {
		return nil, apperrors.ErrForbidden("Image uploads are disabled in demo mode")
	}
	actor, ok := api.UserID(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized("Authentication required")
	}
	f := in.RawBody.Data().File
	defer f.Close()
	img, err := h.uc.Upload(ctx, domain.UserID(actor), f)
	if err != nil {
		return nil, toAppError(err)
	}
	return &UploadOutput{Body: body(img)}, nil
}

func (h *handlers) download(c *gin.Context) {
	id, err := idutil.Parse(c.Param("id"))
	variant, validVariant := domain.ParseVariant(c.Param("variant"))
	if err != nil || !validVariant {
		apperrors.AbortWithError(c, apperrors.ErrValidation("Invalid image ID or variant"))
		return
	}
	userID, ok := c.Get("userID")
	actor, isUint := userID.(uint)
	if !ok || !isUint {
		apperrors.AbortWithError(c, apperrors.ErrUnauthorized("Authentication required"))
		return
	}
	img, file, err := h.uc.Open(c.Request.Context(), domain.UserID(actor), domain.ImageID(id), variant)
	if err != nil {
		apperrors.AbortWithError(c, toAppError(err))
		return
	}
	defer file.Close()
	// The name is unique per image; size and mtime guard against a file replaced in place.
	name := img.FileOf(variant)
	c.Header("ETag", fmt.Sprintf(`"%s-%s-%x-%x"`, models.ImageVersion(img.File), variant, file.Size, file.ModTime.UnixNano()))
	c.Header("Cache-Control", "private, max-age=31536000, immutable")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Type", img.Mime)
	if c.Query("download") == "1" {
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="image-%d%s"`, id, domain.Extension(img.Mime)))
	}
	http.ServeContent(c.Writer, c.Request, name, file.ModTime, file)
}

func toAppError(err error) *apperrors.AppError {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return apperrors.ErrNotFound("Image")
	case errors.Is(err, domain.ErrFileMissing):
		return apperrors.ErrNotFound("Image file")
	case errors.Is(err, domain.ErrUnsafeName):
		return apperrors.ErrValidation("Invalid image path")
	case errors.Is(err, domain.ErrTooLarge):
		return apperrors.ErrInvalidInput("file", "must be at most 25 MB").WithError(err)
	case errors.Is(err, domain.ErrInvalidImage):
		return apperrors.ErrInvalidInput("file", "must be a valid JPEG, PNG, HEIC, WebP or GIF image of reasonable size").WithError(err)
	}
	return apperrors.ErrInternal("Failed to process image").WithError(err)
}
