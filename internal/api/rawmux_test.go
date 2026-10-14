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

	// ⚠ A guard that matches nothing passes forever. This used to assert a MINIMUM COUNT, which
	// was right while raw registrations were plentiful and became wrong the moment the migration
	// finished — a count cannot tell "the regex broke" from "there is nothing left to find", and
	// it started failing on success.
	//
	// The SPA catch-all is the one registration that must always exist (it serves the embedded
	// frontend and is not an API route), so its presence is what proves the regex still matches.
	// That stays true no matter how few raw routes remain.
	var sawSPA bool
	for _, p := range found {
		if p == "/" {
			sawSPA = true
		}
	}
	if !sawSPA {
		t.Fatalf(`the SPA catch-all mux.Handle("/", …) was not found among %v — the regex stopped `+
			"matching, so this guard is no longer guarding anything", found)
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
	// It is empty now (see TestTheRawV1AllowlistStaysEmpty), so this half is dormant — and it
	// stays because the first person to add an entry needs it working, not written afterwards.
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

// knownRawV1Routes is EMPTY, and TestTheRawV1AllowlistStaysEmpty keeps it that way.
//
// It began as a work list — the /v1 routes still owed a migration, each with why and what
// replaced it — and shrank to nothing as they landed: the backup pair, three clip byte routes
// and the channel-icon serve, then the SSE stream and the multipart upload, then the two SSO
// redirects. Every entry cost the protections listed at the top of this file.
//
// ⚠ **Do not add an entry to make a new route pass.** The map survives only so the mechanism is
// obvious if a genuine exception ever turns up; adding one silently re-opens the escape hatch
// this whole migration closed. A route that streams bytes, redirects, or takes multipart still
// belongs on the Huma API — that is what rawOp (rawop.go) is for, and every route listed above
// proved it works for exactly those shapes.
var knownRawV1Routes = map[string]string{}

// TestTheRawV1AllowlistStaysEmpty is the ratchet.
//
// TestNoV1RouteEscapesToTheRawMux passes either by a route being migrated OR by someone adding
// it to the allowlist — which is the right shape while a migration is in flight, and the wrong
// one once it is finished. Every /v1 route is a Huma operation now, so the honest assertion is
// that the exemption list is empty, and re-opening the escape hatch has to be a deliberate,
// visible edit to this test rather than one quiet map entry.
func TestTheRawV1AllowlistStaysEmpty(t *testing.T) {
	if len(knownRawV1Routes) == 0 {
		return
	}
	var listed []string
	for pattern, why := range knownRawV1Routes {
		listed = append(listed, pattern+" — "+why)
	}
	sort.Strings(listed)
	t.Errorf("knownRawV1Routes is meant to be empty, and has %d entries:\n  %s\n\n"+
		"Every /v1 route is a Huma operation. A route that streams bytes, redirects or takes "+
		"multipart is not an exception — rawOp (rawop.go) covers all three, and the backup, clip, "+
		"icon, SSE and SSO routes each proved it. If an exception is genuinely warranted, delete "+
		"this test in the same commit so the decision is reviewed rather than absorbed.",
		len(knownRawV1Routes), strings.Join(listed, "\n  "))
}

// TestTheSPACatchAllIsTheOnlyRawMuxRoute states the end condition of the migration.
//
// TestNoV1RouteEscapesToTheRawMux only constrains `/v1`, which was the right scope while the ops
// probes, Prometheus, the profiler and the API reference still sat at bare paths. They are under
// /v1 now (with hidden aliases, see ops.go), so the honest assertion is stronger: the embedded
// SPA is the ONLY thing registered directly on the mux, because it is the only registration that
// is not an API route.
//
// A new raw route outside /v1 would slip past the other guard entirely. This is what makes
// adding one a deliberate act rather than an oversight.
func TestTheSPACatchAllIsTheOnlyRawMuxRoute(t *testing.T) {
	var extra []string
	for _, p := range rawMuxRoutes(t) {
		if p != "/" {
			extra = append(extra, p)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("raw mux registrations besides the SPA catch-all:\n  %s\n\n"+
			"Every route is a Huma operation, including the ones that serve bytes, redirect, "+
			"stream or wrap a third-party handler — rawOp (rawop.go) covers all of them, and the "+
			"ops probes, Prometheus exposition and Go profiler each proved it. Registering here "+
			"instead skips the role middleware (which fails CLOSED), the CSRF check and the spec.",
			strings.Join(extra, "\n  "))
	}
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
