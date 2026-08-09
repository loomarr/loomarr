package api_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"log/slog"

	"github.com/mantonx/loomarr/internal/api"
	"gopkg.in/yaml.v3"
)

// The spec declares NOTHING nullable (V53b set huma.DefaultArrayNullable = false, taking the
// count of `- "null"` type-unions from 109 to 0). That makes a very simple invariant available:
//
//	*no response body may contain a JSON null.*
//
// ⚠ This guard exists because the flag alone would make the spec LIE. Huma says so in its own
// doc comment — "any `nil` slice will still encode as `null` in JSON" — so flipping it changes
// what the document CLAIMS without changing what the wire SENDS. A nil slice in any handler
// silently reintroduces the `null | []` ambiguity that V53b removed, and every generated client
// (orval's types, the zod schemas, the future MSW mocks) would be wrong in a way nothing else
// checks.
//
// ⚠ An EMPTY STORE is the point, not an accident. `nil` slices are what a repository returns
// when it finds no rows, so a fresh database is precisely the state that produces them — a
// seeded fixture would hide the whole class by never taking the empty branch.
//
// The endpoint list is DERIVED from the exported spec rather than hand-written: a hand-kept
// roster is the drift class this repo already tracks in three places, and a new list endpoint
// that nobody added would be exactly the one that regresses.
func TestResponses_ContainNoJSONNull(t *testing.T) {
	specBytes, err := api.ExportOpenAPI(slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(specBytes, &spec); err != nil {
		t.Fatal(err)
	}

	// GETs with no path parameters: they are reachable against an empty store without inventing
	// ids, and they are where list-shaped responses live.
	var paths []string
	for p, ops := range spec.Paths {
		if _, ok := ops["get"]; !ok {
			continue
		}
		if strings.Contains(p, "{") {
			continue
		}
		paths = append(paths, p)
	}
	if len(paths) == 0 {
		t.Fatal("derived zero GET paths from the spec — the derivation broke, not the API")
	}

	srv, _ := newServer(t)
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			resp := do(t, srv, http.MethodGet, p, adminToken, "")
			defer func() { _ = resp.Body.Close() }()

			// Only success bodies make a claim about the response schema. A 4xx/5xx is a
			// problem+json document and is not what this guard is about.
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				t.Skipf("status %d — not a schema-bearing response", resp.StatusCode)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if len(body) == 0 {
				return
			}

			var decoded any
			if err := json.Unmarshal(body, &decoded); err != nil {
				return // not JSON (a stream, a file) — nothing to assert
			}
			for _, where := range findNulls(decoded, "") {
				t.Errorf("null at %s — the spec declares nothing nullable, so this is a nil slice "+
					"reaching the wire; initialise it (e.g. make([]T, 0)) at the point it is built", where)
			}
		})
	}
}

// findNulls walks a decoded JSON document and reports the path of every null it finds.
func findNulls(v any, path string) []string {
	var out []string
	switch t := v.(type) {
	case nil:
		return []string{orRoot(path)}
	case map[string]any:
		for k, child := range t {
			out = append(out, findNulls(child, path+"."+k)...)
		}
	case []any:
		for i, child := range t {
			out = append(out, findNulls(child, fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return out
}

func orRoot(p string) string {
	if p == "" {
		return "(root)"
	}
	return p
}
