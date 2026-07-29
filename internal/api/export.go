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
	// schemaOnly makes the nil-guarded register* funcs (auth, provisioning, users
	// sync) emit their operation schemas into the spec despite the bare Server —
	// handlers aren't invoked; only their schemas are read. Without it the exported
	// spec would omit /v1/auth/*, /v1/setup/bootstrap, /v1/users/{import,sync} and
	// orval couldn't type them (the FE↔BE seam that 13.0 closes).
	srv := &Server{log: log, schemaOnly: true}
	srv.registerMiddleware(humaAPI)
	srv.registerTitles(humaAPI)
	srv.registerAuth(humaAPI)
	srv.registerUsers(humaAPI)
	srv.registerPasswords(humaAPI)
	srv.registerChannels(humaAPI)
	srv.registerGuide(humaAPI)
	srv.registerProgramming(humaAPI)
	srv.registerSetup(humaAPI)
	srv.registerSuggestions(humaAPI)
	srv.registerSearch(humaAPI)
	srv.registerFiller(humaAPI)
	srv.registerFillerSources(humaAPI)
	srv.registerJobs(humaAPI)
	srv.registerSystemLLM(humaAPI)
	srv.registerSystemDatabase(humaAPI)
	srv.registerSystemBackups(humaAPI)
	srv.registerSystemRestart(humaAPI)
	srv.registerSettings(humaAPI)
	srv.registerHelp(humaAPI)
	srv.registerDashboard(humaAPI)
	// registerProvisioning is nil-guarded (like registerAuth): with no provisioner
	// wired here, /v1/setup/bootstrap + /v1/users/import stay out of the exported
	// spec, matching how /v1/auth/* is handled. Documented via §11.
	srv.registerProvisioning(humaAPI)
	return humaAPI.OpenAPI().YAML()
}
