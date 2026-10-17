// Command openapi writes the OpenAPI 3.1 document of the huma operations to stdout (or to
// the file named by the first argument). `make openapi` runs it; CI fails when the
// committed document differs (docs/adr/0011-api-contract-with-huma.md).
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"meerkat/api"
	"meerkat/config"
	"meerkat/routes"

	"github.com/gin-gonic/gin"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	a := api.New(r.Group("/api/v1"), r)
	// No database: the handlers are never run, only described
	routes.RegisterAPI(a, &config.Config{}, routes.NewModules(nil, nil, nil), routes.Groups{})

	// json.MarshalIndent sorts map keys, so the output is stable.
	doc, err := json.MarshalIndent(a.OpenAPI(), "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "openapi:", err)
		os.Exit(1)
	}
	doc = append(doc, '\n')
	if len(os.Args) > 1 {
		if err := os.WriteFile(os.Args[1], doc, 0o644); err != nil { //nolint:gosec // a committed, public document
			fmt.Fprintln(os.Stderr, "openapi:", err)
			os.Exit(1)
		}
		return
	}
	_, _ = os.Stdout.Write(doc)
}
