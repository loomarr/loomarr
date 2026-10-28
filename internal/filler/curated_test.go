package filler_test

import (
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

// The shipped list parses and every entry is in the form adding it by hand would store, so an
// install that already has one is not offered a duplicate.
func TestCuratedSources_ShipInTheirCanonicalForm(t *testing.T) {
	list, err := filler.CuratedSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Sources) == 0 {
		t.Fatal("no curated sources")
	}
	kinds := map[string]bool{}
	for _, s := range list.Sources {
		kinds[s.Kind] = true
		switch s.Kind {
		case "archive":
			if strings.ContainsAny(s.Target, "/?# ") {
				t.Errorf("archive target %q is not a bare identifier", s.Target)
			}
		case "youtube":
			if !strings.HasPrefix(s.Target, "https://www.youtube.com/channel/UC") || !strings.HasSuffix(s.Target, "/videos") {
				t.Errorf("youtube target %q is not a canonical channel URL", s.Target)
			}
		}
	}
	if !kinds["archive"] || !kinds["youtube"] {
		t.Fatalf("curated kinds %v, want archive collections and YouTube channels", kinds)
	}
}
