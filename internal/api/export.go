package api

import (
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// ExportOpenAPI builds the API's operation definitions and returns the OpenAPI
// 3.1 spec as YAML (§7.1). It needs no live store — only the registered
// operation schemas — so `make openapi` runs offline and deterministically.
func ExportOpenAPI(log *slog.Logger) ([]byte, error) {
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, humaConfig())
	srv := &Server{log: log} // handlers aren't invoked; only their schemas are read
	srv.registerMiddleware(humaAPI)
	srv.registerTitles(humaAPI)
	srv.registerAuth(humaAPI)
	srv.registerUsers(humaAPI)
	srv.registerChannels(humaAPI)
	srv.registerSetup(humaAPI)
	return humaAPI.OpenAPI().YAML()
}
