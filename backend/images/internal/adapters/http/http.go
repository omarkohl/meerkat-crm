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
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "set-contact-images", Method: http.MethodPut, Path: "/contacts/{id}/images",
		Summary: "Set the images of a contact and their order; the first one becomes the main image when there is none",
		Tags:    []string{"images"},
	}, h.setContactImages)
	api.RegisterOn(a, group, huma.Operation{
		OperationID: "set-main-image", Method: http.MethodPut, Path: "/contacts/{id}/main_image",
		Summary: "Make an image of the contact its main image; the avatar is cut from the crop",
		Tags:    []string{"images"},
	}, h.setMainImage)
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

type UploadOutput struct{ Body api.ImageRef }

func body(img domain.Image) api.ImageRef {
	out := api.ImageRef{
		ID: uint(img.ID), Width: img.Width, Height: img.Height, Mime: img.Mime, Bytes: img.Bytes, TakenAt: img.TakenAt,
		URL:      models.ImageURL(uint(img.ID), img.File, string(domain.VariantImage)),
		ThumbURL: models.ImageURL(uint(img.ID), img.File, string(domain.VariantThumb)),
	}
	if img.Crop != nil {
		out.Crop = api.Nullable[api.ImageCrop]{Value: &api.ImageCrop{X: img.Crop.X, Y: img.Crop.Y, Size: img.Crop.Size}}
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

type ContactImagesInput struct {
	ID   uint `path:"id" minimum:"1" doc:"The contact's ID"`
	Body struct {
		ImageIDs []uint `json:"image_ids" nullable:"false" doc:"Uploaded images (POST /images) or images of another contact of yours, in order, at most 50. Images left out are deleted."`
	}
}

type ContactImagesOutput struct{ Body []api.ImageRef }

func (h *handlers) setContactImages(ctx context.Context, in *ContactImagesInput) (*ContactImagesOutput, error) {
	actor, ok := api.UserID(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized("Authentication required")
	}
	list, err := h.uc.SetContactImages(ctx, domain.UserID(actor), domain.ContactID(in.ID), in.Body.ImageIDs)
	if err != nil {
		return nil, toAppError(err)
	}
	out := &ContactImagesOutput{Body: make([]api.ImageRef, len(list))}
	for i, img := range list {
		out.Body[i] = body(img)
	}
	return out, nil
}

type MainImageInput struct {
	ID   uint `path:"id" minimum:"1" doc:"The contact's ID"`
	Body struct {
		ImageID uint `json:"image_id" minimum:"1" doc:"An image of this contact"`
		Crop    struct {
			X    int `json:"x" minimum:"0"`
			Y    int `json:"y" minimum:"0"`
			Size int `json:"size" minimum:"1"`
		} `json:"crop" doc:"The face square, in pixels of the image"`
	}
}

type MainImageOutput struct {
	Body struct {
		MainImageID uint         `json:"main_image_id"`
		Image       api.ImageRef `json:"image"`
	}
}

func (h *handlers) setMainImage(ctx context.Context, in *MainImageInput) (*MainImageOutput, error) {
	actor, ok := api.UserID(ctx)
	if !ok {
		return nil, apperrors.ErrUnauthorized("Authentication required")
	}
	c := in.Body.Crop
	img, err := h.uc.SetMainImage(ctx, domain.UserID(actor), domain.ContactID(in.ID), domain.ImageID(in.Body.ImageID), domain.Crop{X: c.X, Y: c.Y, Size: c.Size})
	if err != nil {
		return nil, toAppError(err)
	}
	out := &MainImageOutput{}
	out.Body.MainImageID, out.Body.Image = uint(img.ID), body(img)
	return out, nil
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
	case errors.Is(err, domain.ErrContactNotFound):
		return apperrors.ErrNotFound("Contact")
	case errors.Is(err, domain.ErrTooManyImages):
		return apperrors.ErrInvalidInput("image_ids", "A contact can have at most 50 images")
	case errors.Is(err, domain.ErrInvalidCrop):
		return apperrors.ErrInvalidInput("crop", "must be a square inside the image")
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
