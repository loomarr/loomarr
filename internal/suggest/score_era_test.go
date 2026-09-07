package suggest

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
)

func TestScore_NamedSeriesEraIsUnassessedAndOverallRenormalizes(t *testing.T) {
	series := ProposalItem{MediaType: provision.Series, TVDBID: 42, Name: "Continuing Show", InLibrary: true}
	missing := ProposalItem{MediaType: provision.Series, TVDBID: 7, Name: "Missing", InLibrary: false}
	key, err := series.Key()
	if err != nil {
		t.Fatal(err)
	}
	missingKey, err := missing.Key()
	if err != nil {
		t.Fatal(err)
	}
	intent := Intent{
		Description:    "named continuing-show lineup",
		membershipKeys: map[provision.Key]bool{key: true, missingKey: true},
	}
	scores := score(intent, []ProposalItem{series}, []ProposalItem{missing})
	if scores.EraBalance != nil {
		t.Fatalf("EraBalance = %v, want unassessed nil", *scores.EraBalance)
	}
	if want := (0.5 + 0.35*0.5) / 0.85; math.Abs(scores.Overall-want) > 1e-15 {
		t.Fatalf("Overall = %v, want %v", scores.Overall, want)
	}
	encoded, err := json.Marshal(scores)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || !containsJSONNull(encoded, "eraBalance") {
		t.Fatalf("scores JSON = %s, want eraBalance:null", encoded)
	}
}

func containsJSONNull(encoded []byte, key string) bool {
	var object map[string]any
	if json.Unmarshal(encoded, &object) != nil {
		return false
	}
	value, ok := object[key]
	return ok && value == nil
}
