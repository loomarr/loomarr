package api_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ⚠ **THE RAW-MUX ESCAPE HATCH.** Every `/v1` route must be a Huma operation. A route registered
// straight onto the ServeMux instead skips things nothing else will notice are missing:
//
//   - the authorization middleware, which reads the required role off the operation and FAILS
//     CLOSED for one that declared nothing (routeauth.go). A raw handler's guard is whatever its
//     body remembers to call — and two of them, backupHandler and eventsHandler, had already
//     drifted to opposite answers on what a nil authorizer means.
//   - the CSRF check on cookie-authenticated mutations, which is why the icon upload still
//     hand-rolls its own.
//   - api/openapi.yaml, so the frontend hand-writes a client the spec cannot check.
//   - TestRegisterListsMatchBetweenRouterAndExporter, which matches only `srv.register*(humaAPI)`
//     and is therefore structurally blind to exactly these routes. This test covers its blind
//     spot; that one caught a live-but-undocumented route, and this is the same failure a step
//     earlier.
//
// Source-parsing rather than route-walking, for the same reason as the export-parity guard: the
// live mux cannot tell "raw on purpose" from "raw by accident", and several routes are legitimately
// nil-guarded so they do not always appear.
func TestNoV1RouteEscapesToTheRawMux(t *testing.T) {
	found := rawMuxRoutes(t)

	// ⚠ A guard that matches nothing passes forever. If the registration style changes, this
	// test must fail loudly rather than quietly approve an empty set — the same vacuous-guard
	// trap the export-parity test protects against.
	if len(found) < 5 {
		t.Fatalf("only %d raw mux registrations found (%v) — the regex stopped matching, so this "+
			"guard is no longer guarding anything", len(found), found)
	}

	var escaped []string
	for _, pattern := range found {
		if !strings.HasPrefix(routePath(pattern), "/v1/") {
			continue // ops probes, /docs, the SPA catch-all — not the versioned API surface
		}
		if _, known := knownRawV1Routes[pattern]; !known {
			escaped = append(escaped, pattern)
		}
	}
	sort.Strings(escaped)

	if len(escaped) > 0 {
		t.Errorf("these /v1 routes are registered on the bare mux, not as Huma operations:\n  %s\n\n"+
			"They bypass the role middleware (which fails CLOSED, so a raw route is not merely "+
			"undocumented — its authorization is whatever its handler body remembers), the CSRF "+
			"check, and api/openapi.yaml. Register them with rawOp (rawop.go), which keeps the "+
			"(w, r) signature for ServeContent/streaming while mounting on the Huma API. If one "+
			"genuinely cannot move yet, add it to knownRawV1Routes with the reason and the plan.",
			strings.Join(escaped, "\n  "))
	}

	// The allowlist is a migration TODO, not a permanent exemption: an entry whose route no longer
	// exists means that migration LANDED and the entry should go. Without this half, the list rots
	// into a set of stale names that quietly re-permits a future route matching an old pattern.
	inSource := make(map[string]bool, len(found))
	for _, p := range found {
		inSource[p] = true
	}
	var stale []string
	for pattern := range knownRawV1Routes {
		if !inSource[pattern] {
			stale = append(stale, pattern)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("knownRawV1Routes lists routes that are no longer registered on the mux:\n  %s\n\n"+
			"They have been migrated — delete these entries so the allowlist keeps shrinking to empty.",
			strings.Join(stale, "\n  "))
	}
}

// knownRawV1Routes are the /v1 routes still on the bare mux, each with why and what replaces it.
//
// ⚠ **This shrinks to empty; it is a work list, not a set of exemptions.** Every entry costs the
// protections listed above, and the test fails on a stale entry so the list cannot outlive the
// work it describes.
var knownRawV1Routes = map[string]string{
	"GET /v1/auth/sso/start":    "302 + Set-Cookie; becomes a rawOp with RolePublic",
	"GET /v1/auth/sso/callback": "302 + Set-Cookie; becomes a rawOp with RolePublic",
}

// muxRegistration matches `mux.HandleFunc("GET /path", …)` and `mux.Handle("/path", …)`.
var muxRegistration = regexp.MustCompile(`mux\.(?:HandleFunc|Handle)\(\s*"([^"]*)"`)

// rawMuxRoutes returns every route pattern registered directly on the ServeMux, across the whole
// package — not just api.go. Scanning every file is the point: registerSSO mounts its two routes
// from ssoroutes.go, so a guard reading only api.go would miss them, which is how the
// export-parity guard came to have a blind spot in the first place.
func rawMuxRoutes(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range muxRegistration.FindAllStringSubmatch(string(b), -1) {
			out = append(out, m[1])
		}
	}
	sort.Strings(out)
	return out
}

// routePath strips the optional leading method from a Go 1.22 mux pattern.
func routePath(pattern string) string {
	if i := strings.LastIndex(pattern, " "); i >= 0 {
		return pattern[i+1:]
	}
	return pattern
}
