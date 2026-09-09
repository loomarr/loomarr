package suggest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
)

func TestScore_NamedMembershipAndRetiredComposite(t *testing.T) {
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
	scores := score(intent, []ProposalItem{series}, []ProposalItem{missing}, ValidatedDateMeaning{})
	if scores.EraBalance != nil {
		t.Fatalf("EraBalance = %v, want unassessed nil", *scores.EraBalance)
	}
	if scores.ThemeFit == nil || *scores.ThemeFit != 1 || scores.Theme.Basis != "named_membership" || scores.AvailabilityRatio != 0.5 {
		t.Fatalf("named membership/presence = %+v", scores)
	}
	encoded, err := json.Marshal(scores)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"overall"`) || !containsJSONNull(encoded, "eraBalance") {
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

func TestHistoricalScoresDoNotAcquireNewAssessmentMeaning(t *testing.T) {
	var proposal Proposal
	if err := json.Unmarshal([]byte(`{"scores":{"themeFit":1,"eraBalance":1,"overall":1,"availabilityRatio":0.5}}`), &proposal); err != nil {
		t.Fatal(err)
	}
	if proposal.Scores.ThemeFit != nil || proposal.Scores.EraBalance != nil || proposal.Scores.Theme.Status != "unassessed" || proposal.Scores.AvailabilityRatio != 0.5 {
		t.Fatalf("historical percentages relabeled as evidence: %+v", proposal.Scores)
	}
	blob, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(blob, &object); err != nil {
		t.Fatal(err)
	}
	var scores map[string]any
	if err := json.Unmarshal(object["scores"], &scores); err != nil {
		t.Fatal(err)
	}
	if _, present := scores["overall"]; present {
		t.Fatal("retired composite is still public")
	}
	if err := json.Unmarshal([]byte(`{"version":1,"themeFit":0.25,"eraBalance":null,"theme":{"status":"partial"}}`), &proposal.Scores); err != nil {
		t.Fatal(err)
	}
	if proposal.Scores.ThemeFit == nil || *proposal.Scores.ThemeFit != 0.25 || proposal.Scores.Theme.Status != "partial" {
		t.Fatal("current assessment lost on decode")
	}
}
