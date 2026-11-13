package api_test

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
)

// unboundedBodyLedger lists the request-body arrays that predate the ceiling rule. It only shrinks:
// a new unbounded array fails below, and an entry that gains its maxItems must leave the ledger.
const unboundedBodyLedger = "testdata/unbounded-body-arrays.txt"

// Every collection a caller supplies carries its ceiling in its schema, so no request can fan one
// out into an unbounded run of store reads or provider calls. Query and path parameters have no
// exceptions; request-body arrays are held to the ledger.
func TestEveryCallerSuppliedArrayDeclaresMaxItems(t *testing.T) {
	found, err := api.UnboundedInputArrays()
	if err != nil {
		t.Fatal(err)
	}
	ledger := readLedger(t)
	current := map[string]bool{}
	for _, entry := range found {
		current[entry] = true
		switch {
		case !strings.Contains(entry, ": body"):
			t.Errorf("%s has no maxItems. Give the parameter a ceiling (a maxItems tag on its field).", entry)
		case !ledger[entry]:
			t.Errorf("%s has no maxItems. Give the field a ceiling (a maxItems tag); the ledger in %s "+
				"holds only arrays that predate this rule.", entry, unboundedBodyLedger)
		}
	}
	for entry := range ledger {
		if !current[entry] {
			t.Errorf("%s is bounded or gone: delete its line from %s.", entry, unboundedBodyLedger)
		}
	}
}

// The shared channel types carry their ceilings on the wire: a draft past a real limit is refused
// at the schema, one entry under it is not, and an empty list still clears (a request an empty
// household editor sends).
func TestSharedChannelArraysRefusePastTheirCeilings(t *testing.T) {
	srv := newChannelsHarness(t).Server
	list := func(item string, n int) string {
		return `[` + strings.TrimSuffix(strings.Repeat(item+",", n), ",") + `]`
	}
	for _, tc := range []struct {
		name, body, refusal string // refusal is the schema message expected, empty when the list fits
	}{
		{"lineup at the ceiling", `{"lineup":` + list(`{"key":"movie:tmdb:603","name":"A"}`, 1000) + `}`, ""},
		{"lineup past the ceiling", `{"lineup":` + list(`{"key":"movie:tmdb:603","name":"A"}`, 1001) + `}`, "array length <= 1000"},
		{"rules past the ceiling", `{"policy":{"rules":` + list(`{"id":"r"}`, 33) + `}}`, "array length <= 32"},
		{"filler pins past the ceiling", `{"policy":{"filler":{"pinned":` + list(`"h"`, 1001) + `}}}`, "array length <= 1000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, tc.body)
			defer func() { _ = resp.Body.Close() }()
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if tc.refusal == "" && strings.Contains(string(raw), "array length") {
				t.Fatalf("a list at its ceiling was refused by the schema: %s", raw)
			}
			if tc.refusal != "" && (resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(raw), tc.refusal)) {
				t.Fatalf("status %d body %.300s, want a 422 saying %q", resp.StatusCode, raw, tc.refusal)
			}
		})
	}
}

func readLedger(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open(unboundedBodyLedger)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	entries := map[string]bool{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" && !strings.HasPrefix(line, "#") {
			entries[line] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}
