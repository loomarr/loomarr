package api_test

import (
	"bufio"
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
