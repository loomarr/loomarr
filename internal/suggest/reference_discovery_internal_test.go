package suggest

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
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
	want := []string{"Full House", "Family Matters", "Step by Step", "Camp Wilder", "Boy Meets World", "Hangin' with Mr. Cooper", "Sister, Sister", "Sabrina the Teenage Witch", "Clueless", "Teen Angel", "You Wish"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("titles=%v, want directly named members before model hints", got)
	}
}

func TestReferenceCandidatePoolKeepsLibraryAndStrongExternalMembersVisible(t *testing.T) {
	got := prioritizeReferenceCandidatePool([]catalog.Candidate{
		{MediaType: provision.Series, TMDBID: 1, Name: "Obscure external", VoteCount: 5},
		{MediaType: provision.Series, TMDBID: 2, Name: "Owned classic", InLibrary: true},
		{MediaType: provision.Series, TMDBID: 3, Name: "Defining external", VoteCount: 5000},
	})
	want := []string{"Owned classic", "Defining external", "Obscure external"}
	if names := []string{got[0].Name, got[1].Name, got[2].Name}; !reflect.DeepEqual(names, want) {
		t.Fatalf("candidate order=%v, want library first then stronger external members", names)
	}
}

func TestNamedSourceContractInvalidatesPriorSuccessfulProposal(t *testing.T) {
	// Frozen reference-source-v3 identity, before broader roster inspection and
	// source media-type preservation changed the named-source result.
	const prior = "da78510e8dea8ac714a496c924d9289b0ef1f560a4fadcc682258384bb970286"
	if IntentHash(Intent{Description: "TGIF"}) == prior {
		t.Fatal("source contract reused the prior proposal cache identity")
	}
}
