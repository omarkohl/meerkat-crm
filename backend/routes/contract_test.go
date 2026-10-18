package routes

import (
	"sort"
	"strings"
	"testing"

	"meerkat/api"
	"meerkat/config"
	"meerkat/services"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// notJSON lists the /api/v1 routes that are not huma operations because they do not speak
// JSON: they send a file, an image or a calendar, or redirect. Every entry needs a reason. A
// new JSON route must be a huma operation (docs/adr/0011-api-contract-with-huma.md).
var notJSON = map[string]string{
	"GET /api/v1/admin/backups/:name":          "sends a backup archive",
	"GET /api/v1/contacts/:id/profile_picture": "sends an image (range requests, ETag)",
	"GET /api/v1/export":                       "sends a CSV file",
	"GET /api/v1/export/vcf":                   "sends a vCard file",
	"GET /api/v1/ics/:file":                    "sends a calendar; its token in the path is the credential",
	"GET /api/v1/proxy/image":                  "sends a fetched image",
	"GET /api/v1/auth/oidc/login":              "redirects to the identity provider",
	"GET /api/v1/auth/oidc/callback":           "redirects back to the app after sign-in",
}

func TestEveryJSONRouteIsAnOperationOfTheContract(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!", CardDAVEnabled: true}
	cfg.OIDC.Enabled = true
	modules := NewModules(nil, cfg, &services.OIDCProvider{})

	described := gin.New()
	spec := api.New(described.Group("/api/v1"), described)
	RegisterAPI(spec, cfg, modules, Groups{})
	operations := map[string]bool{}
	for path, item := range spec.OpenAPI().Paths {
		route := "/api/v1" + strings.NewReplacer("{", ":", "}", "").Replace(path)
		for method, op := range map[string]*huma.Operation{"GET": item.Get, "POST": item.Post, "PUT": item.Put, "PATCH": item.Patch, "DELETE": item.Delete} {
			if op != nil {
				operations[method+" "+route] = true
			}
		}
	}
	require.NotEmpty(t, operations)

	r := gin.New()
	RegisterRoutes(r, cfg, nil, modules)
	var stray []string
	seen := map[string]bool{}
	for _, route := range r.Routes() {
		if !strings.HasPrefix(route.Path, "/api/v1/") {
			continue
		}
		key := route.Method + " " + route.Path
		seen[key] = true
		if !operations[key] && notJSON[key] == "" {
			stray = append(stray, key)
		}
	}
	sort.Strings(stray)
	assert.Empty(t, stray, "these /api/v1 routes are not huma operations: convert them, or list them in notJSON with the reason")

	for key := range notJSON {
		assert.True(t, seen[key], "notJSON lists a route that does not exist: %s", key)
	}
	for key := range operations {
		assert.True(t, seen[key], "the spec describes an operation that is not routed: %s", key)
	}
}
