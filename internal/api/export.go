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
	srv.registerSuggestions(humaAPI)
	srv.registerSearch(humaAPI)
	srv.registerFiller(humaAPI)
	srv.registerSystemLLM(humaAPI)
	srv.registerSettings(humaAPI)
	// registerProvisioning is nil-guarded (like registerAuth): with no provisioner
	// wired here, /v1/setup/bootstrap + /v1/users/import stay out of the exported
	// spec, matching how /v1/auth/* is handled. Documented via §11.
	srv.registerProvisioning(humaAPI)
	return humaAPI.OpenAPI().YAML()
}
