package api_test

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ⚠ TWO PARALLEL REGISTER LISTS. `api.go` mounts the live routes; `export.go` builds the
// OpenAPI spec. A route added to one and not the other is invisible in the direction it was
// missed — live but undocumented (so orval cannot type it and the frontend hand-writes a
// request), or documented but unmounted (so the generated client calls a 404).
//
// This bit during V16: `GET /v1/playout/sessions` was registered in api.go, served correctly,
// and simply absent from the spec — `make openapi` produced no diff and `openapi-verify` stayed
// green, because the exporter never saw the route. The export.go docstring records an earlier
// instance of the same thing (/v1/auth/* omitted, "orval couldn't type them"), patched
// case-by-case.
//
// Parsing the source is deliberate. The alternative — diffing the exported spec against the
// live mux — cannot distinguish "absent because it drifted" from "absent because it is
// nil-guarded", which several routes legitimately are.
func TestRegisterListsMatchBetweenRouterAndExporter(t *testing.T) {
	// ⚠ **The delimiter after `humaAPI` is `[,)]`, not `)`, and that is a bug fix.** The pattern
	// used to demand exactly `(humaAPI)`, so the first registrar taking a second argument —
	// `srv.registerOps(humaAPI, opts.Pprof)` — was invisible to this guard. It was missing from
	// export.go, its routes served traffic, and the spec did not list them: the exact
	// live-but-undocumented failure recorded above, walking straight through the check meant to
	// catch it, because the check was matching a call SHAPE rather than a call.
	call := regexp.MustCompile(`srv\.(register[A-Za-z]+)\(humaAPI[,)]`)

	read := func(path string) []string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, m := range call.FindAllStringSubmatch(string(b), -1) {
			out = append(out, m[1])
		}
		sort.Strings(out)
		return out
	}

	live := read("api.go")
	exported := read("export.go")

	if len(live) == 0 || len(exported) == 0 {
		t.Fatalf("found %d live and %d exported register calls — the regex stopped matching, "+
			"so this guard is no longer guarding anything", len(live), len(exported))
	}

	missing := diff(live, exported)
	extra := diff(exported, live)

	if len(missing) > 0 {
		t.Errorf("registered in api.go but NOT in export.go: %s\n"+
			"These routes serve traffic and are absent from api/openapi.yaml, so orval cannot "+
			"type them and openapi-verify stays green while the spec is incomplete.",
			strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		t.Errorf("registered in export.go but NOT in api.go: %s\n"+
			"These are in the spec and the generated client, but nothing mounts them — a "+
			"client call would 404.", strings.Join(extra, ", "))
	}
}

// diff returns the elements of a that are not in b.
func diff(a, b []string) []string {
	inB := make(map[string]bool, len(b))
	for _, s := range b {
		inB[s] = true
	}
	var out []string
	for _, s := range a {
		if !inB[s] {
			out = append(out, s)
		}
	}
	return out
}
