package filler

import (
	"slices"
	"testing"
)

// Political ads and news are opt-in (#2037): a channel that chose no kinds never airs them, and a
// channel that names one does. A station's own news is station material, so it airs wherever the
// station's idents do, but only from a station within the channel's radius.
func TestFilterKinds_PoliticalAndNewsAreOptIn(t *testing.T) {
	ad := Clip{Hash: "ad", Path: "ad", Kind: Commercial, DurationMs: 30_000}
	campaign := Clip{Hash: "campaign", Path: "campaign", Kind: Political, DurationMs: 30_000}
	network := Clip{Hash: "network", Path: "network", Kind: News, GeographicScope: GeographicNational, DurationMs: 60_000}
	local := Clip{Hash: "local", Path: "local", Kind: News, GeographicScope: GeographicLocal, Station: "wxyz", DurationMs: 60_000}
	unknown := Clip{Hash: "unknown", Path: "unknown", Kind: Unclassified, Placement: PlacementBreakBody, DurationMs: 30_000}
	catalog := []Clip{ad, campaign, network, local, unknown}

	inRadius := map[string]bool{"wxyz": true}

	for _, tc := range []struct {
		name  string
		kinds []string
		local map[string]bool
		want  []string
	}{
		{name: "no kinds chosen", kinds: nil, local: inRadius, want: []string{"ad", "local", "unknown"}},
		{name: "the default ticked plus political", kinds: append(DefaultKinds(), "political"), local: inRadius, want: []string{"ad", "campaign", "local"}},
		{name: "news opted in", kinds: []string{"commercial", "news"}, local: inRadius, want: []string{"ad", "network", "local"}},
		{name: "commercials only", kinds: []string{"commercial"}, local: inRadius, want: []string{"ad"}},
		{name: "station idents without news", kinds: []string{"station_id"}, local: inRadius, want: []string{"local"}},
		// An installation with no location admits every clip geographically, so the radius check
		// has to be here: a station that is nobody's neighbour gets no exception.
		{name: "no location set", kinds: nil, local: nil, want: []string{"ad", "unknown"}},
		{name: "a station outside the radius", kinds: nil, local: map[string]bool{"kabc": true}, want: []string{"ad", "unknown"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, c := range filterKinds(catalog, tc.kinds, tc.local) {
				got = append(got, c.Hash)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("filterKinds(%v) = %v, want %v", tc.kinds, got, tc.want)
			}
		})
	}
}

func TestDefaultKinds_LeavesOutOnlyTheOptInKinds(t *testing.T) {
	got := DefaultKinds()
	for _, optIn := range []Kind{Political, News, Unclassified} {
		if slices.Contains(got, string(optIn)) {
			t.Errorf("DefaultKinds() = %v, includes %q", got, optIn)
		}
	}
	for _, k := range []Kind{Commercial, Bumper, StationID, PSA, Trailer, Interstitial} {
		if !slices.Contains(got, string(k)) {
			t.Errorf("DefaultKinds() = %v, missing %q", got, k)
		}
	}
}
