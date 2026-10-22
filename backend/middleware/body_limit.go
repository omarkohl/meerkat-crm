package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Default body size limits
const (
	DefaultMaxBodySize = 10 << 20 // 10 MB
	MaxJSONBodySize    = 1 << 20  // 1 MB
	// MaxUploadBodySize is the limit of the image upload route: a 25 MB file plus its multipart framing.
	MaxUploadBodySize = 26 << 20
	ImageUploadRoute  = "/api/v1/images"
)

// BodySizeLimitMiddleware limits the size of request bodies to prevent DoS attacks.
func BodySizeLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Wrap the request body with a size limiter
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

func JSONBodySizeLimitMiddleware() gin.HandlerFunc {
	return BodySizeLimitMiddleware(MaxJSONBodySize)
}

// DefaultBodySizeLimitMiddleware applies DefaultMaxBodySize, except on the image upload route.
func DefaultBodySizeLimitMiddleware() gin.HandlerFunc {
	def, upload := BodySizeLimitMiddleware(DefaultMaxBodySize), BodySizeLimitMiddleware(MaxUploadBodySize)
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodPost && c.FullPath() == ImageUploadRoute {
			upload(c)
			return
		}
		def(c)
	}
}
