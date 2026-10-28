package fillerenrichment_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerenrichment"
)

type kindCorpus struct {
	Version string `json:"version"`
	Items   []struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Role        string `json:"role"`
		Want        string `json:"want"`
	} `json:"items"`
}

// The free pass's political and news rules are scored against a labelled corpus (#2037). A wrong
// political or news label takes an ordinary ad off every channel that has not opted in, so neither
// rule may make a false call: precision must be 1. Recall is reported, and held to a floor, because
// a miss is not silent. The text model still classifies the clip, and an operator can correct it.
func TestAnalyzeDeterministic_OptInKindsScoreOnTheLabelledCorpus(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "kind-corpus-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus kindCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	type tally struct{ truePositive, falsePositive, falseNegative int }
	scores := map[string]*tally{"political": {}, "news": {}}
	for _, item := range corpus.Items {
		states := statesByAxis(fillerenrichment.AnalyzeDeterministic(fillerenrichment.Signals{
			ClipHash: item.Title, Kind: item.Role, Title: item.Title, Description: item.Description,
			ObservedAt: time.Unix(100, 0).UTC(),
		}))
		got := states[fillerenrichment.AxisKind].Value.Text
		for class, score := range scores {
			switch {
			case got == class && item.Want == class:
				score.truePositive++
			case got == class:
				score.falsePositive++
				t.Errorf("%q labelled %s, want %s", item.Title, got, item.Want)
			case item.Want == class:
				score.falseNegative++
				t.Logf("missed %s: %q read as %q", class, item.Title, got)
			}
		}
		if (got == "political" || got == "news") && states[fillerenrichment.AxisFormat].Value.Tags[0] != got {
			t.Errorf("%q kind %s, format %v: the format tag must name the same class", item.Title, got,
				states[fillerenrichment.AxisFormat].Value.Tags)
		}
	}
	for class, score := range scores {
		precision := float64(score.truePositive) / float64(max(1, score.truePositive+score.falsePositive))
		recall := float64(score.truePositive) / float64(max(1, score.truePositive+score.falseNegative))
		t.Logf("%s %s: precision %.2f, recall %.2f (%d labelled)", corpus.Version, class, precision, recall,
			score.truePositive+score.falseNegative)
		if precision < 1 {
			t.Errorf("%s precision %.2f, want 1", class, precision)
		}
		if recall < 0.9 {
			t.Errorf("%s recall %.2f, want at least 0.90", class, recall)
		}
	}
}

// A specific role outranks the wording rules: a PSA that mentions the weather stays a PSA.
func TestAnalyzeDeterministic_OptInWordingDoesNotOverrideASpecificRole(t *testing.T) {
	got := statesByAxis(fillerenrichment.AnalyzeDeterministic(fillerenrichment.Signals{
		ClipHash: "psa", Kind: "psa", Title: "Severe weather forecast safety PSA", ObservedAt: time.Unix(100, 0).UTC(),
	}))
	if kind := got[fillerenrichment.AxisKind].Value.Text; kind != "psa" {
		t.Fatalf("kind = %q, want psa", kind)
	}
}
