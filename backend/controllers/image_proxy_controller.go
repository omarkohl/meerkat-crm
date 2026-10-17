package controllers

import (
	apperrors "meerkat/errors"
	"meerkat/httputil"
	"meerkat/logger"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ProxyImage fetches an image from a URL for the browser, which cannot do it itself (CORS).
func ProxyImage(c *gin.Context) {
	imageURL := c.Query("url")
	if imageURL == "" {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("url"))
		return
	}

	body, contentType, err := httputil.FetchImageFromURL(imageURL)
	if err != nil {
		logger.FromContext(c).Warn().Err(err).Str("url", imageURL).Msg("Failed to fetch image from URL")
		apperrors.AbortWithError(c, apperrors.ErrExternal("image proxy", "Failed to fetch image").WithError(err))
		return
	}

	c.Data(http.StatusOK, contentType, body)
}
