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
	got := prioritizedReferenceTitles(Intent{}, []string{"Alpha", "Beta", "Gamma"}, []string{"Invented", "Gamma", "Gamma"})
	if !reflect.DeepEqual(got, []string{"Gamma", "Alpha", "Beta"}) {
		t.Fatalf("titles=%v", got)
	}
}

func TestReferencePrefetchPrioritizesDirectlyNamedSourceMembers(t *testing.T) {
	intent := Intent{Description: "TGIF, with Full House, Family Matters, and Step by Step."}
	titles := []string{"Camp Wilder", "Boy Meets World", "Hangin' with Mr. Cooper", "Sister, Sister", "Sabrina the Teenage Witch", "Clueless", "Teen Angel", "You Wish", "Full House", "Family Matters", "Step by Step"}
	got := prioritizedReferenceTitles(intent, titles, []string{"Camp Wilder", "Boy Meets World"})
	want := []string{"Full House", "Family Matters", "Step by Step", "Camp Wilder", "Boy Meets World", "Hangin' with Mr. Cooper", "Sister, Sister", "Sabrina the Teenage Witch"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("titles=%v, want directly named members before model hints", got)
	}
}

func TestNamedSourceContractInvalidatesPriorSuccessfulProposal(t *testing.T) {
	// Frozen reference-source-v1 identity, before owned-identity resolution and
	// direct-member prefetch priority changed the named-source result.
	const prior = "1badcd576e97648da87451ad031a746ee3cc10dc0765387bb9953b6617567704"
	if IntentHash(Intent{Description: "TGIF"}) == prior {
		t.Fatal("source contract reused the prior proposal cache identity")
	}
}
