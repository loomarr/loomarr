package proposals

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIntentNeverSerialisesRequestContext(t *testing.T) {
	raw, err := json.Marshal(Intent{Description: "90s action", DiscoveryScopeID: "channel-1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "channel-1") || strings.Contains(strings.ToLower(string(raw)), "scope") {
		t.Fatalf("persisted intent carries request context: %s", raw)
	}
}

func TestChannelIntentDropsRunInputs(t *testing.T) {
	got := Intent{
		Description: "90s action", ChannelName: "Fixed", RefineText: "more",
		CurrentLineup: []LineupContext{{Name: "A"}}, Adjacent: []AdjacentContext{{Name: "B"}},
	}.ChannelIntent()
	if got.Description != "90s action" {
		t.Fatalf("description = %q, want kept", got.Description)
	}
	if got.ChannelName != "" || got.RefineText != "" || got.CurrentLineup != nil || got.Adjacent != nil {
		t.Fatalf("run inputs survived: %+v", got)
	}
}

func TestScoresDecodeAppliesAssessmentContract(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantStatus string
		wantFit    bool
	}{
		{"historical percentages are unassessed", `{"themeFit":0.9,"availabilityRatio":1}`, "unassessed", false},
		{"versioned scores keep their evidence", `{"version":1,"themeFit":0.5,"theme":{"status":"supported"}}`, "supported", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var s Scores
			if err := json.Unmarshal([]byte(tc.raw), &s); err != nil {
				t.Fatal(err)
			}
			if s.Theme.Status != tc.wantStatus || (s.ThemeFit != nil) != tc.wantFit {
				t.Fatalf("scores = %+v, want theme status %q fit present %v", s, tc.wantStatus, tc.wantFit)
			}
		})
	}
}

func TestProposalItemKeyRequiresGroundedIdentity(t *testing.T) {
	if _, err := (ProposalItem{MediaType: "movie", Name: "No ID"}).Key(); err == nil {
		t.Fatal("an item with no external id produced a key")
	}
	key, err := (ProposalItem{MediaType: "movie", TMDBID: 603, Name: "The Matrix"}).Key()
	if err != nil || key == "" {
		t.Fatalf("key = %q, err = %v", key, err)
	}
}
