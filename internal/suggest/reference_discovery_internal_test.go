package suggest

import (
	"reflect"
	"testing"
)

func TestNamedBlockLabelPreservesAmbiguityAndNetworkRoles(t *testing.T) {
	for _, tc := range []struct{ description, refinement, want string }{
		{"TGIF", "", "TGIF"},
		{"TGIF, with Full House, Family Matters, and Step by Step. Keep the sitcoms that actually aired in ABC's 1990s Friday-night block; play episodes from the 1990s in episode order.", "", "TGIF"},
		{"TGIF: Full House and Family Matters", "", "TGIF"},
		{"TGIF; include Family Matters", "", "TGIF"},
		{"Friday-night block", "", ""},
		{"ABC, network shows", "", ""},
		{"TGIF, with Full House", "Toonami block", ""},

		{"a TGIF block", "", "TGIF"},
		{"Make me a channel like ABC's TGIF Friday-night block from the 1990s.", "", "TGIF"},
		{"TGIF block", "Toonami block", ""},
		{"ABC network shows, not TGIF", "", ""},
		{"ABC network", "", ""},
		{"1990s comedy collection", "", ""},
		{"Friday Club block", "", "Friday Club"},
	} {
		t.Run(tc.description+tc.refinement, func(t *testing.T) {
			if got := namedBlockLabel(Intent{Description: tc.description, RefineText: tc.refinement}); got != tc.want {
				t.Fatalf("label=%q want %q", got, tc.want)
			}
		})
	}
}

func TestReferenceHypothesesOnlyReorderSourceMembers(t *testing.T) {
	got := prioritizedReferenceTitles([]string{"Alpha", "Beta", "Gamma"}, []string{"Invented", "Gamma", "Gamma"})
	if !reflect.DeepEqual(got, []string{"Gamma", "Alpha", "Beta"}) {
		t.Fatalf("titles=%v", got)
	}
}

func TestNamedSourceContractInvalidatesPriorSuccessfulProposal(t *testing.T) {
	// Frozen v6/v6 identity for TGIF before automatic source discovery.
	const prior = "a087119655986fb9016920113442e61448d783eb9ae5db7d34f0e6178b128a21"
	if IntentHash(Intent{Description: "TGIF"}) == prior {
		t.Fatal("source contract reused the prior proposal cache identity")
	}
}
