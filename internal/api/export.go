package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"sort"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	catalogapi "github.com/loomarr/loomarr/internal/api/catalog"
	channelsapi "github.com/loomarr/loomarr/internal/api/channels"
	fillerapi "github.com/loomarr/loomarr/internal/api/filler"
	guideapi "github.com/loomarr/loomarr/internal/api/guide"
	"github.com/loomarr/loomarr/internal/api/httpcore"
	imagesapi "github.com/loomarr/loomarr/internal/api/images"
	libraryapi "github.com/loomarr/loomarr/internal/api/library"
	opsapi "github.com/loomarr/loomarr/internal/api/ops"
	"github.com/loomarr/loomarr/internal/api/people"
	playoutapi "github.com/loomarr/loomarr/internal/api/playout"
	proposalsapi "github.com/loomarr/loomarr/internal/api/proposals"
	setupapi "github.com/loomarr/loomarr/internal/api/setup"
	"github.com/loomarr/loomarr/internal/api/system"
)

// ExportOpenAPI builds the API's operation definitions and returns the OpenAPI
// 3.1 spec as YAML (§7.1). It needs no live store — only the registered
// operation schemas — so `make openapi` runs offline and deterministically.
func ExportOpenAPI(log *slog.Logger) ([]byte, error) {
	_, humaAPI := schemaOnlyAPI(log)
	return humaAPI.OpenAPI().YAML()
}

// schemaOnlyAPI registers every typed operation against store-less dependencies and returns the
// root Server (for its middleware) with the Huma API.
//
// Shared by ExportOpenAPI and the authorization gate so the two cannot disagree about what
// "every operation" means: a route added to one list and not the other is exactly the drift
// this centralises away. Adding a subpackage's New(...).Register call here reaches both.
func schemaOnlyAPI(log *slog.Logger) (*Server, huma.API) {
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, humaConfig())
	// The `true` passed to people, system, playout and ops is their schemaOnly flag: it makes the
	// nil-guarded registrars (auth, provisioning, users sync, events) emit their operation
	// schemas into the spec despite the bare Deps — handlers aren't invoked; only their schemas
	// are read. Without it the exported spec would omit /v1/auth/*, /v1/setup/bootstrap,
	// /v1/users/{import,sync} and orval couldn't type them.
	srv := &Server{log: log}
	srv.registerMiddleware(humaAPI)
	catalogapi.New(catalogapi.Deps{}).Register(humaAPI)
	people.New(people.Deps{Log: log}, true).Register(humaAPI)
	system.New(system.Deps{Log: log}, true).Register(humaAPI)
	channelsapi.New(channelsapi.Deps{}).Register(humaAPI)
	// Hidden ops — present for register-list parity, absent from the spec (§9.1) — plus the play-url
	// op and the status projection.
	playoutapi.New(playoutapi.Deps{Log: log}, true).Register(humaAPI)
	guideapi.New(guideapi.Deps{}).Register(humaAPI)
	proposalsapi.New(proposalsapi.Deps{}).Register(humaAPI)
	libraryapi.New(libraryapi.Deps{}).Register(humaAPI)
	fillerapi.New(fillerapi.Deps{}).Register(humaAPI)
	imagesapi.New(imagesapi.Deps{}).Register(humaAPI)
	setupapi.New(setupapi.Deps{}).Register(humaAPI)
	// The events frames are nil-guarded on the bus; schemaOnly forces the frame schemas out so the
	// generated client always has the event types even when this export ran without one. The
	// profiler routes are Hidden on both paths anyway (they exist only when a flag is set), and the
	// exporter has no Options to read the flag from.
	opsapi.New(opsapi.Deps{}, true).Register(humaAPI)
	return srv, humaAPI
}

// EventFrameTypes reports the SSE frame vocabulary as event-name → payload Go type name.
// Exported so the guard in api_test can compare it against what internal/app actually
// publishes without reaching into the package.
//
// ⚠ The failure this enables detection of is silent: huma names an SSE frame after the Go
// TYPE of its payload, so publishing a type absent from eventTypeMap emits a frame with no
// `event:` line. Every addEventListener keyed to that name stops firing, nothing errors, and
// the feature just looks broken.
func EventFrameTypes() map[string]string {
	out := make(map[string]string, len(opsapi.EventTypeMap()))
	for name, v := range opsapi.EventTypeMap() {
		out[name] = reflect.TypeOf(v).Name()
	}
	return out
}

// UnboundedInputArrays reports every array a caller supplies, as a query or path parameter or
// anywhere in a JSON request body, that declares no maxItems. A caller-sized collection fans out
// into store reads and provider calls, so each one carries its ceiling in its schema.
//
// Returns "METHOD /path (operation-id): where" so a failure names the field.
func UnboundedInputArrays() ([]string, error) {
	_, humaAPI := schemaOnlyAPI(slog.New(slog.DiscardHandler))
	spec := humaAPI.OpenAPI()
	if spec == nil || spec.Paths == nil || spec.Components == nil {
		return nil, fmt.Errorf("api: no operations registered")
	}
	registry := spec.Components.Schemas
	var found []string
	var walk func(route, where string, s *huma.Schema, seen map[string]bool)
	walk = func(route, where string, s *huma.Schema, seen map[string]bool) {
		if s == nil {
			return
		}
		if s.Ref != "" {
			// seen holds the refs on this path only, so a recursive schema ends, and a schema shared
			// by two fields is reported under both.
			if seen[s.Ref] {
				return
			}
			seen[s.Ref] = true
			walk(route, where, registry.SchemaFromRef(s.Ref), seen)
			delete(seen, s.Ref)
			return
		}
		if s.Type == huma.TypeArray && s.MaxItems == nil {
			found = append(found, route+": "+where)
		}
		walk(route, where+"[]", s.Items, seen)
		for name, p := range s.Properties {
			walk(route, where+"."+name, p, seen)
		}
		for _, group := range [][]*huma.Schema{s.OneOf, s.AnyOf, s.AllOf} {
			for _, sub := range group {
				walk(route, where, sub, seen)
			}
		}
	}
	for path, item := range spec.Paths {
		for method, op := range map[string]*huma.Operation{
			http.MethodGet: item.Get, http.MethodPost: item.Post, http.MethodPut: item.Put,
			http.MethodPatch: item.Patch, http.MethodDelete: item.Delete,
		} {
			if op == nil {
				continue
			}
			route := fmt.Sprintf("%s %s (%s)", method, path, op.OperationID)
			for _, p := range op.Parameters {
				walk(route, p.In+" "+p.Name, p.Schema, map[string]bool{})
			}
			if op.RequestBody != nil {
				if media := op.RequestBody.Content["application/json"]; media != nil {
					walk(route, "body", media.Schema, map[string]bool{})
				}
			}
		}
	}
	sort.Strings(found)
	return found, nil
}

// OperationsMissingARole reports every registered operation that declares no required role
// (§11). Exported so the test in api_test can call it without reaching into the package.
//
// ⚠ Walks the REAL registry rather than grepping source: a regex over `huma.Register` calls
// would miss the conditionally-registered ones (dev-login, users sync) and would keep
// passing if a registration moved somewhere the pattern no longer matched. What is mounted
// is the only thing worth asserting on.
//
// Returns "METHOD /path (operation-id)" so a failure names the route rather than an index.
func OperationsMissingARole() ([]string, error) {
	_, humaAPI := schemaOnlyAPI(slog.New(slog.DiscardHandler))
	spec := humaAPI.OpenAPI()
	if spec == nil || spec.Paths == nil {
		return nil, fmt.Errorf("api: no operations registered")
	}
	var missing []string
	for path, item := range spec.Paths {
		for method, op := range map[string]*huma.Operation{
			http.MethodGet: item.Get, http.MethodPost: item.Post, http.MethodPut: item.Put,
			http.MethodPatch: item.Patch, http.MethodDelete: item.Delete,
		} {
			if op == nil {
				continue
			}
			if _, ok := op.Metadata[httpcore.RoleMetaKey]; !ok {
				missing = append(missing, fmt.Sprintf("%s %s (%s)", method, path, op.OperationID))
			}
		}
	}
	sort.Strings(missing) // deterministic: map iteration order must not reorder a failure
	return missing, nil
}
