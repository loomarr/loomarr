package filler

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
)

// A clip open search found belongs to no registered source, so it names the search rather than
// reading as an anonymous import (#2067).
func TestFillerSourceLabel_NamesOpenSearch(t *testing.T) {
	if got := fillerSourceLabel(acquisition.OpenSearchSourceID, nil); got != "Search for nearby stations" {
		t.Fatalf("open-search label = %q", got)
	}
	if got := fillerSourceLabel("archive:gone", nil); got != "Imported source" {
		t.Fatalf("unregistered source label = %q", got)
	}
}
