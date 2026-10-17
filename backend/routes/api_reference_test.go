package routes

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"meerkat/config"
	"meerkat/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// undocumentedOnPurpose lists route path prefixes that docs/api-reference.md does not
// list. Keep this short; every entry needs a reason.
var undocumentedOnPurpose = map[string]string{
	// The CardDAV server speaks WebDAV (PROPFIND, REPORT, ...) on a wildcard path for
	// CardDAV clients, not a REST API; it is described in the CardDAV docs.
	"/carddav/": "WebDAV server for CardDAV clients, not part of the REST API",
	// Discovery redirect for CardDAV clients, same reason.
	"/.well-known/carddav": "CardDAV discovery redirect",
}

var docRowPattern = regexp.MustCompile("^\\| `([A-Z]+)`\\s*\\| `([^`]+)`\\s*\\|")

// routeLikeRow matches any table row whose second cell starts with a path, so a
// malformed route row fails the test instead of being skipped.
var routeLikeRow = regexp.MustCompile("^\\|[^|]*\\|\\s*`?/")

var paramPattern = regexp.MustCompile(`:[A-Za-z_]+`)

// normalizePath makes route and documented paths comparable: no /api/v1 prefix, no
// duplicate slashes, parameter names ignored, and the ICS token placeholder is a parameter.
func normalizePath(p string) string {
	p = strings.Replace(p, "<token>.ics", ":file", 1)
	p = strings.TrimPrefix(p, "/api/v1")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return paramPattern.ReplaceAllString(p, ":p")
}

func documentedRoutes(t *testing.T) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "api-reference.md"))
	require.NoError(t, err)
	docs := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if !routeLikeRow.MatchString(line) {
			continue
		}
		m := docRowPattern.FindStringSubmatch(line)
		require.NotNil(t, m, "row looks like a route but is not in the form | `METHOD` | `path` |: %s", line)
		docs[m[1]+" "+normalizePath(m[2])] = true
	}
	return docs
}

// Every registered route (with every optional feature switched on) must appear in
// docs/api-reference.md, and every documented route must exist. This keeps the doc
// from drifting again.
func TestAPIReferenceMatchesRegisteredRoutes(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!", CardDAVEnabled: true, MetricsEnabled: true}
	cfg.OIDC.Enabled = true
	r := gin.New()
	RegisterRoutes(r, cfg, nil, NewModules(nil, cfg, &services.OIDCProvider{}))

	registered := map[string]bool{}
	for _, route := range r.Routes() {
		skip := false
		for prefix := range undocumentedOnPurpose {
			if strings.HasPrefix(route.Path, prefix) {
				skip = true
			}
		}
		if !skip {
			registered[route.Method+" "+normalizePath(route.Path)] = true
		}
	}
	require.NotEmpty(t, registered)

	docs := documentedRoutes(t)
	var missing, stale []string
	for k := range registered {
		if !docs[k] {
			missing = append(missing, k)
		}
	}
	for k := range docs {
		if !registered[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	assert.Empty(t, missing, "registered routes missing from docs/api-reference.md")
	assert.Empty(t, stale, "routes in docs/api-reference.md that are not registered")
}
