package pod

import (
	"slices"
	"testing"
)

// A network's ident airs only on a channel of that network: not on another network's, not on a
// channel with no network, and not even when pinned, because the boundary sits outside the pins
// as geography does (#4432). A clip with no network airs on every channel. Network names are
// invented, as every fixture's are.
func TestAssemble_ANetworksPieceAirsOnlyOnItsNetwork(t *testing.T) {
	ident := Clip{Hash: "lantern-ident", Path: "lantern-ident", Role: StationID, Era: 1992, Audience: Kids,
		DurationMs: 5_000, Network: "Lantern"}
	neutral := commercial("neutral-ad", 1992, Kids)
	catalog := []Clip{ident, neutral}

	for name, tc := range map[string]struct {
		network string
		pinned  bool
		airs    bool
	}{
		"its own network":         {network: "LANTERN", airs: true},
		"its own network, spelt":  {network: "The Lantern", airs: true},
		"another network":         {network: "KESTREL"},
		"another network, pinned": {network: "KESTREL", pinned: true},
		"a channel with none":     {network: ""},
	} {
		t.Run(name, func(t *testing.T) {
			w := Window{Era: Year(1992), Audience: Kids, GapMs: 120_000, PodMax: 4, Seed: 1, Network: tc.network}
			if tc.pinned {
				w.Pinned = []string{ident.ID()}
			}
			got := Assemble(catalog, w, Policy{}, nil)
			var ids []string
			for _, e := range got.Entries {
				ids = append(ids, e.Hash)
			}
			if slices.Contains(ids, ident.ID()) != tc.airs {
				t.Fatalf("pod = %v; want the ident aired=%v", ids, tc.airs)
			}
			if !slices.Contains(ids, neutral.ID()) {
				t.Fatalf("pod = %v; want the neutral ad, which airs on any network", ids)
			}
			if fit := FitFor(ident, w, Policy{}); (fit.Reason == FitNetwork) == tc.airs {
				t.Fatalf("fit = %+v; want reason network exactly when it does not air", fit)
			}
		})
	}
}
