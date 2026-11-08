package settings

import (
	"strings"
	"testing"
)

// maxSummary keeps the line under a field to what reads at a glance beside the control.
const maxSummary = 140

// The line under every field is one plain sentence; anything more goes in Detail, behind the info
// icon (#2741). Neither carries a spec reference: a household never reads design.md.
func TestRegistry_HelpIsOneSentenceWithDetailBehindIt(t *testing.T) {
	for _, s := range NewRegistry().All() {
		if s.MigrationOnly {
			continue
		}
		doc := s.Doc
		switch {
		case doc == "":
			t.Errorf("%s: no summary", s.Key)
			continue
		case len(doc) > maxSummary:
			t.Errorf("%s: summary is %d characters, want at most %d: %q", s.Key, len(doc), maxSummary, doc)
		case !strings.HasSuffix(doc, "."):
			t.Errorf("%s: summary is not a sentence ending in a full stop: %q", s.Key, doc)
		}
		if body := strings.TrimSuffix(doc, "."); strings.ContainsAny(body, ".?!;") {
			t.Errorf("%s: summary is more than one sentence: %q", s.Key, doc)
		}
		if strings.Contains(doc, "§") || strings.Contains(s.Detail, "§") {
			t.Errorf("%s: help cites a spec section", s.Key)
		}
		if s.Detail == doc {
			t.Errorf("%s: detail repeats the summary", s.Key)
		}
	}
}

// The generated reference keeps every fact: both halves appear in the row.
func TestConfigDocs_RowCarriesSummaryAndDetail(t *testing.T) {
	got := row(Setting{
		Key: "test.key", EnvVar: "TEST_KEY", Owner: OwnerAdvanced, Kind: KindString,
		Doc: "What it does.", Detail: "Why and when.",
	})
	if !strings.Contains(got, "What it does. Why and when.") {
		t.Errorf("row = %q, want summary then detail", got)
	}
}
