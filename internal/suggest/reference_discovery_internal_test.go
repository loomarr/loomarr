package suggest

import (
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
)

func TestNamedBlockLabelPreservesAmbiguityAndNetworkRoles(t *testing.T) {
	for _, tc := range []struct{ description, refinement, want string }{
		{"QFUN", "", "QFUN"},
		{"QFUN, with Grand Oak Hotel, Crumb Street Bakery, and Bowling Alley Blues. Keep the sitcoms that actually aired in ZBN's 1990s Friday-night block; play episodes from the 1990s in episode order.", "", "QFUN"},
		{"QFUN: Grand Oak Hotel and Crumb Street Bakery", "", "QFUN"},
		{"QFUN; include Crumb Street Bakery", "", "QFUN"},
		{"QFUN with Crumb Street Bakery and Bowling Alley Blues.", "", "QFUN"},
		{"Friday-night block", "", ""},
		{"ZBN, network shows", "", ""},
		{"QFUN, with Grand Oak Hotel", "Moonbeam block", ""},

		{"a QFUN block", "", "QFUN"},
		{"Make me a channel like ZBN's QFUN Friday-night block from the 1990s.", "", "QFUN"},
		{"QFUN block", "Moonbeam block", ""},
		{"ZBN network shows, not QFUN", "", ""},
		{"ZBN network", "", ""},
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
	intent := Intent{Description: "QFUN, with Grand Oak Hotel, Crumb Street Bakery, and Bowling Alley Blues."}
	titles := []string{"Camp Thistle", "Kid Meets Kite", "Hangin' with Mr. Hobb", "Twice, Twice", "Penny the Pocket Witch", "Pinwheel Summer", "Halo Kid", "Wishbone Way", "Grand Oak Hotel", "Crumb Street Bakery", "Bowling Alley Blues"}
	got := prioritizedReferenceTitles(intent, titles, []string{"Camp Thistle", "Kid Meets Kite"})
	want := []string{"Grand Oak Hotel", "Crumb Street Bakery", "Bowling Alley Blues", "Camp Thistle", "Kid Meets Kite", "Hangin' with Mr. Hobb", "Twice, Twice", "Penny the Pocket Witch", "Pinwheel Summer", "Halo Kid", "Wishbone Way"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("titles=%v, want directly named members before model hints", got)
	}
}

func TestReferenceCandidatePoolKeepsLibraryAndStrongExternalMembersVisible(t *testing.T) {
	got := prioritizeReferenceCandidatePool([]catalog.Candidate{
		{MediaType: provision.Series, TMDBID: 90000001, Name: "Obscure external", VoteCount: 5},
		{MediaType: provision.Series, TMDBID: 90000002, Name: "Owned classic", InLibrary: true},
		{MediaType: provision.Series, TMDBID: 90000003, Name: "Defining external", VoteCount: 5000},
	})
	want := []string{"Owned classic", "Defining external", "Obscure external"}
	if names := []string{got[0].Name, got[1].Name, got[2].Name}; !reflect.DeepEqual(names, want) {
		t.Fatalf("candidate order=%v, want library first then stronger external members", names)
	}
}

func TestNamedSourceContractInvalidatesPriorSuccessfulProposal(t *testing.T) {
	// Frozen reference-source-v3 identity, before broader roster inspection and
	// source media-type preservation changed the named-source result.
	const prior = "07aca99b45d7b6ea7fbe07bebda7f5aea73623230ac25ec0c33b7a31f4066607"
	if IntentHash(Intent{Description: "QFUN"}) == prior {
		t.Fatal("source contract reused the prior proposal cache identity")
	}
}
