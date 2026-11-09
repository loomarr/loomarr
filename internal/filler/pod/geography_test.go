package pod_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

func geoCommercial(id string, scope pod.GeographicScope, country, station string) pod.Clip {
	return pod.Clip{Hash: id, Path: id + ".mp4", Name: id, Role: pod.Commercial,
		Era: 1992, Audience: pod.Kids, DurationMs: 30_000,
		GeographicScope: scope, Country: country, Station: station}
}

// Invented places and stations: "harbor" reaches fcc:1 and fcc:2, "lakeside" reaches fcc:3, and
// the British "moorside" reaches wd:Q1.
func localStations(country, place string) map[string]bool {
	switch {
	case country == "US" && place == "harbor":
		return map[string]bool{"fcc:1": true, "fcc:2": true}
	case country == "US" && place == "lakeside":
		return map[string]bool{"fcc:3": true}
	case country == "GB" && place == "moorside":
		return map[string]bool{"wd:Q1": true}
	}
	return nil
}

func harborPolicy() pod.Policy {
	return pod.Policy{Geography: pod.Geography{Country: "us", Place: " harbor "}, LocalStations: localStations}
}

func TestAssemble_GeographyIsNeverRelaxedOrBypassedByPin(t *testing.T) {
	catalog := []pod.Clip{
		geoCommercial("us-national", pod.GeographicNational, "US", ""),
		geoCommercial("harbor-local", pod.GeographicLocal, "US", "fcc:1"),
		geoCommercial("lakeside-local", pod.GeographicLocal, "US", "fcc:3"),
		geoCommercial("canadian", pod.GeographicNational, "CA", ""),
		geoCommercial("unknown", pod.GeographicUnknown, "", ""),
	}
	w := pod.Window{Seed: 7, Era: pod.Year(2015), Audience: pod.Kids, GapMs: 120_000, PodMax: 5,
		Pinned: []string{"canadian", "lakeside-local", "unknown"}}
	brk := pod.Assemble(catalog, w, harborPolicy(), nil)

	seen := map[string]bool{}
	for _, entry := range brk.Entries {
		seen[entry.Hash] = true
	}
	if !seen["us-national"] && !seen["harbor-local"] {
		t.Fatalf("pod has no compatible content: %+v", brk.Entries)
	}
	for _, forbidden := range []string{"canadian", "lakeside-local", "unknown"} {
		if seen[forbidden] {
			t.Errorf("geographically incompatible pinned clip %q aired", forbidden)
		}
	}
}

// Region preference (#2035): a fallback country's national ads air behind the home country's within
// an era, ahead of a home ad from a widened era, and in the order the countries are listed. Its local
// ads never air.
func TestAssemble_FallbackCountriesFillInBehindHomeWithinAnEra(t *testing.T) {
	at := func(id, country string, scope pod.GeographicScope, era int) pod.Clip {
		c := geoCommercial(id, scope, country, "")
		c.Era = era
		return c
	}
	catalog := []pod.Clip{
		at("us-1992", "US", pod.GeographicNational, 1992),
		at("au-1992", "AU", pod.GeographicNational, 1992),
		at("gb-1992", "GB", pod.GeographicNational, 1992),
		at("us-2000", "US", pod.GeographicNational, 2000), // the widened rung
		at("gb-local", "GB", pod.GeographicLocal, 1992),
		at("fr-1992", "FR", pod.GeographicNational, 1992), // not listed
	}
	policy := pod.Policy{Geography: pod.Geography{Country: "US", Fallback: "gb, AU,US"}}
	aired := func(podMax int) map[string]bool {
		w := pod.Window{Seed: 3, Era: pod.Year(1992), Audience: pod.Kids, GapMs: 300_000, PodMax: podMax}
		seen := map[string]bool{}
		for _, e := range pod.Assemble(catalog, w, policy, nil).Entries {
			seen[e.Hash] = true
		}
		return seen
	}
	if got := aired(1); !got["us-1992"] || len(got) != 1 {
		t.Fatalf("one-ad pod aired %v, want the home ad", got)
	}
	if got := aired(2); !got["us-1992"] || !got["gb-1992"] || len(got) != 2 {
		t.Fatalf("two-ad pod aired %v, want home then the first fallback country", got)
	}
	if got := aired(3); !got["au-1992"] || got["us-2000"] {
		t.Fatalf("three-ad pod aired %v, want the second fallback country before a widened home ad", got)
	}
	got := aired(10)
	if got["gb-local"] || got["fr-1992"] {
		t.Fatalf("pod aired %v: a fallback country's local ad or an unlisted country's ad", got)
	}
	if !got["us-2000"] {
		t.Fatalf("pod aired %v, want the widened home ad once the era ran out", got)
	}
}

// Region orders within a rung, so category variety still spans it: home, the fallback country's
// different product, then home's second of the same product.
func TestAssemble_FallbackRegionKeepsVarietyAcrossTheRung(t *testing.T) {
	beer := func(id string) pod.Clip {
		c := geoCommercial(id, pod.GeographicNational, "US", "")
		c.Category = "beer"
		return c
	}
	car := geoCommercial("gb-car", pod.GeographicNational, " gb ", "")
	car.Category = "car"
	brk := pod.Assemble([]pod.Clip{beer("us-beer-1"), beer("us-beer-2"), car},
		pod.Window{Seed: 5, Era: pod.Year(1992), Audience: pod.Kids, GapMs: 90_000, PodMax: 3},
		pod.Policy{Geography: pod.Geography{Country: "US", Fallback: "GB"}}, nil)
	var cats []string
	for _, e := range brk.Entries {
		cats = append(cats, e.Category)
	}
	if want := []string{"beer", "car", "beer"}; !slices.Equal(cats, want) {
		t.Fatalf("pod categories %v, want %v", cats, want)
	}
}

// A channel with its own place in the home country still inherits the preference, and a fallback
// country's local ads stay out.
func TestGeographicallyEligible_APlaceChannelInheritsTheFallback(t *testing.T) {
	policy := pod.Policy{Geography: pod.Geography{Country: "US", Fallback: "GB"}, LocalStations: localStations}
	w := pod.Window{Seed: 2, Era: pod.Year(1992), Audience: pod.Kids, GapMs: 120_000, PodMax: 4,
		Geography: pod.Geography{Country: "US", Place: "lakeside"}}
	seen := map[string]bool{}
	for _, e := range pod.Assemble([]pod.Clip{
		geoCommercial("gb-national", pod.GeographicNational, "gb", ""),
		geoCommercial("gb-local", pod.GeographicLocal, "GB", "fcc:3"),
	}, w, policy, nil).Entries {
		seen[e.Hash] = true
	}
	if !seen["gb-national"] || seen["gb-local"] {
		t.Fatalf("a lakeside channel aired %v, want the GB national ad only", seen)
	}
}

func TestSourceGeographicallyEligible_AFallbackCountryIsNationalOnly(t *testing.T) {
	home := pod.Geography{Country: "US", Fallback: "GB"}
	cases := []struct {
		source pod.Geography
		want   bool
	}{
		{pod.Geography{Country: "gb"}, true},
		{pod.Geography{Country: "AU"}, false},
		{pod.Geography{Country: "US"}, true},
	}
	for _, tc := range cases {
		if got := pod.SourceGeographicallyEligible(tc.source, home); got != tc.want {
			t.Errorf("source %+v eligible = %v, want %v", tc.source, got, tc.want)
		}
	}
	if got := home.Normalize().RegionRank("gb"); got != 1 {
		t.Fatalf("GB ranks %d, want 1, behind home", got)
	}
	if got := (pod.Geography{Country: "US", Fallback: " gb, CA,,gb ,us"}).Normalize().Fallback; got != "GB,CA" {
		t.Fatalf("normalized fallback %q, want GB,CA", got)
	}
}

func TestAssemble_GeographyMismatchFallsBackToCard(t *testing.T) {
	brk := pod.Assemble([]pod.Clip{
		geoCommercial("lakeside-local", pod.GeographicLocal, "US", "fcc:3"),
	}, pod.Window{Seed: 1, Era: pod.Year(1992), Audience: pod.Kids, GapMs: 60_000, PodMax: 4},
		harborPolicy(), nil)
	if brk.MatchLevel != pod.MatchBumperCard || len(brk.Entries) != 1 || !brk.Entries[0].IsFallbackCard {
		t.Fatalf("mismatch pod = %+v, want one bumper card", brk)
	}
}

// #2030 decision 16: a channel's own location replaces the household's, another country included,
// so a channel of British shows can opt into British commercials in a US household. The
// household's place stays home (#2059 decision 8): a British channel airs British station clips
// only near a British place of its own.
func TestAssemble_AChannelMayOptIntoAnotherCountry(t *testing.T) {
	catalog := []pod.Clip{
		geoCommercial("us-national", pod.GeographicNational, "US", ""),
		geoCommercial("harbor-local", pod.GeographicLocal, "US", "fcc:1"),
		geoCommercial("gb-national", pod.GeographicNational, "GB", ""),
		geoCommercial("moorside-local", pod.GeographicLocal, "GB", "wd:Q1"),
	}
	policy := harborPolicy()
	for _, tc := range []struct {
		channel     pod.Geography
		want, never []string
	}{
		{pod.Geography{Country: "GB"}, []string{"gb-national"}, []string{"us-national", "harbor-local", "moorside-local"}},
		{pod.Geography{Country: "GB", Place: "moorside"}, []string{"gb-national", "moorside-local"}, []string{"us-national", "harbor-local"}},
		{pod.Geography{}, []string{"us-national", "harbor-local"}, []string{"gb-national", "moorside-local"}},
	} {
		seen := map[string]bool{}
		for seed := range int64(40) {
			w := pod.Window{Seed: seed, Era: pod.Year(1992), Audience: pod.Kids, GapMs: 60_000, PodMax: 2,
				Geography: tc.channel}
			for _, e := range pod.Assemble(catalog, w, policy, nil).Entries {
				seen[e.Hash] = true
			}
		}
		for _, id := range tc.want {
			if !seen[id] {
				t.Errorf("channel %+v: %s never aired; aired %v", tc.channel, id, seen)
			}
		}
		for _, id := range tc.never {
			if seen[id] {
				t.Errorf("channel %+v: %s aired", tc.channel, id)
			}
		}
	}
}

// #2062: a local clip airs only near its station; without a place, a station, or a resolver it
// airs nowhere, while a national clip needs only the country.
func TestGeographicallyEligible_LocalMeansNearTheStation(t *testing.T) {
	harbor := pod.Geography{Country: "US", Place: "harbor"}.Resolve(localStations)
	countryOnly := pod.Geography{Country: "US"}.Resolve(localStations)
	unresolved := pod.Geography{Country: "US", Place: "harbor"}.Resolve(nil)
	for _, c := range []struct {
		name string
		clip pod.Clip
		area pod.Area
		want bool
	}{
		{"near station", geoCommercial("a", pod.GeographicLocal, "US", "fcc:2"), harbor, true},
		{"far station", geoCommercial("b", pod.GeographicLocal, "US", "fcc:3"), harbor, false},
		{"local without station", geoCommercial("c", pod.GeographicLocal, "US", ""), harbor, false},
		{"local, no place", geoCommercial("d", pod.GeographicLocal, "US", "fcc:1"), countryOnly, false},
		{"local, no resolver", geoCommercial("e", pod.GeographicLocal, "US", "fcc:1"), unresolved, false},
		{"national, no place", geoCommercial("f", pod.GeographicNational, "US", "fcc:3"), countryOnly, true},
		{"foreign national", geoCommercial("g", pod.GeographicNational, "CA", ""), harbor, false},
	} {
		if got := pod.GeographicallyEligible(c.clip, c.area); got != c.want {
			t.Errorf("%s: eligible = %v, want %v", c.name, got, c.want)
		}
	}
}

// A channel in another country still inherits the household's region preference (#2035): it is
// the household's alone. The channel's own country leaves the list, and home is not added to it.
func TestGeographicallyEligible_AnotherCountryChannelInheritsTheFallback(t *testing.T) {
	policy := pod.Policy{Geography: pod.Geography{Country: "US", Fallback: "GB,CA"}}
	w := pod.Window{Seed: 2, Era: pod.Year(1992), Audience: pod.Kids, GapMs: 120_000, PodMax: 4,
		Geography: pod.Geography{Country: "GB"}}
	seen := map[string]bool{}
	for _, e := range pod.Assemble([]pod.Clip{
		geoCommercial("gb-national", pod.GeographicNational, "GB", ""),
		geoCommercial("ca-national", pod.GeographicNational, "CA", ""),
		geoCommercial("us-national", pod.GeographicNational, "US", ""),
	}, w, policy, nil).Entries {
		seen[e.Hash] = true
	}
	if !seen["gb-national"] || !seen["ca-national"] || seen["us-national"] {
		t.Fatalf("a GB channel aired %v, want the GB and CA national ads only", seen)
	}
}

func TestGeographicallyEligible_EmptyInstallationPreservesLegacyPool(t *testing.T) {
	if !pod.GeographicallyEligible(geoCommercial("unknown", pod.GeographicUnknown, "", ""), pod.Area{}) {
		t.Fatal("empty installation geography unexpectedly excluded a legacy clip")
	}
}

func TestSourceGeographyHardBoundary(t *testing.T) {
	target := pod.Geography{Country: "US", Place: "harbor"}
	cases := []struct {
		name   string
		source pod.Geography
		want   bool
	}{
		{"US-wide", pod.Geography{Country: "us"}, true},
		{"Canadian", pod.Geography{Country: "CA"}, false},
		{"unknown", pod.Geography{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pod.SourceGeographicallyEligible(tc.source, target); got != tc.want {
				t.Fatalf("SourceGeographicallyEligible() = %v, want %v", got, tc.want)
			}
		})
	}
	if !pod.SourceGeographicallyEligible(pod.Geography{}, pod.Geography{}) {
		t.Fatal("unconfigured installation must preserve legacy source eligibility")
	}
}

// #2059 decision 8: a channel may pick its own place, and a channel that names only the household's
// country keeps the household's place.
func TestAssemble_ChannelPlaceOverridesOrInherits(t *testing.T) {
	catalog := []pod.Clip{geoCommercial("lakeside-local", pod.GeographicLocal, "US", "fcc:3")}
	for _, c := range []struct {
		name    string
		channel pod.Geography
		want    bool
	}{
		{"own place", pod.Geography{Country: "US", Place: "lakeside"}, true},
		{"country only inherits harbor", pod.Geography{Country: "US"}, false},
		{"unset inherits harbor", pod.Geography{}, false},
	} {
		brk := pod.Assemble(catalog, pod.Window{Seed: 1, Era: pod.Year(1992), Audience: pod.Kids,
			GapMs: 30_000, PodMax: 1, Geography: c.channel}, harborPolicy(), nil)
		aired := len(brk.Entries) == 1 && brk.Entries[0].Hash == "lakeside-local"
		if aired != c.want {
			t.Errorf("%s: lakeside clip aired = %v, want %v (%+v)", c.name, aired, c.want, brk.Entries)
		}
	}
}

// A break holds at most one station segment, last before the programme returns; local ads are
// ordinary ads and a local station ID never opens a break (#2065).
func TestBreakHoldsOneStationSegmentLast(t *testing.T) {
	segment := func(id string, kind pod.Role) pod.Clip {
		c := geoCommercial(id, pod.GeographicLocal, "US", "fcc:1")
		c.Role, c.DurationMs = kind, 15_000
		return c
	}
	bumper := geoCommercial("bumper-us", pod.GeographicNational, "US", "")
	bumper.Role, bumper.DurationMs = pod.Bumper, 5_000
	catalog := []pod.Clip{
		segment("segment-news", pod.Interstitial), segment("segment-weather", pod.Interstitial),
		segment("segment-promo", pod.Interstitial), bumper,
		geoCommercial("local-ad", pod.GeographicLocal, "US", "fcc:2"),
		geoCommercial("ad-a", pod.GeographicNational, "US", ""), geoCommercial("ad-b", pod.GeographicNational, "US", ""),
		geoCommercial("ad-c", pod.GeographicNational, "US", ""),
	}
	// The only bookend is a local station ID: it may close a break, never open one.
	for seed := int64(1); seed <= 20; seed++ {
		w := pod.Window{Seed: seed, Era: pod.Year(1992), Audience: pod.Kids, GapMs: 240_000, PodMax: 8}
		idOnly := append([]pod.Clip{segment("segment-id", pod.StationID)}, catalog[4:]...)
		if brk := pod.Assemble(idOnly, w, harborPolicy(), nil); brk.Entries[0].Hash == "segment-id" {
			t.Fatalf("seed %d: a local station ID opened the break", seed)
		}
	}
	sawSegment, sawLocalAd := false, false
	for seed := int64(1); seed <= 40; seed++ {
		w := pod.Window{Seed: seed, Era: pod.Year(1992), Audience: pod.Kids, GapMs: 240_000, PodMax: 8}
		brk := pod.Assemble(catalog, w, harborPolicy(), nil)
		segments := []int{}
		for i, e := range brk.Entries {
			if strings.HasPrefix(e.Hash, "segment-") {
				segments = append(segments, i)
			}
			sawLocalAd = sawLocalAd || e.Hash == "local-ad"
		}
		if len(segments) > 1 {
			t.Fatalf("seed %d: %d station segments in one break: %+v", seed, len(segments), brk.Entries)
		}
		if len(segments) == 0 {
			continue
		}
		sawSegment = true
		at, n := segments[0], len(brk.Entries)
		if at == 0 && n > 1 {
			t.Fatalf("seed %d: a station segment opened the break", seed)
		}
		// Last, or second to last before the return bumper.
		last := at == n-1 || (at == n-2 && brk.Entries[n-1].Hash == "bumper-us")
		if !last {
			t.Fatalf("seed %d: station segment at %d of %d: %+v", seed, at, n, brk.Entries)
		}
	}
	if !sawSegment || !sawLocalAd {
		t.Fatalf("segment seen %v, local ad seen %v; the catalog should air both", sawSegment, sawLocalAd)
	}
}

// A clip stays while any current place can air it: the installation's area or a channel's own
// place. A foreign national clip, or a local one no place is near, has no area (#2064).
func TestUsableAreasKeepClipsAnyPlaceCanAir(t *testing.T) {
	lakesideLocal := geoCommercial("lakeside-local", pod.GeographicLocal, "US", "fcc:3")
	canadian := geoCommercial("canadian", pod.GeographicNational, "CA", "")
	harborLocal := geoCommercial("harbor-local", pod.GeographicLocal, "US", "fcc:1")

	installationOnly := harborPolicy().UsableAreas(nil)
	if !pod.UsableIn(harborLocal, installationOnly) {
		t.Error("the installation's own station is unusable")
	}
	if pod.UsableIn(lakesideLocal, installationOnly) || pod.UsableIn(canadian, installationOnly) {
		t.Error("a clip no place can air is still usable")
	}
	withLakeside := harborPolicy().UsableAreas([]pod.Geography{{Country: "US", Place: "lakeside"}})
	if !pod.UsableIn(lakesideLocal, withLakeside) {
		t.Error("a channel's own place did not keep its station's clip")
	}
	// A channel in another country airs that country's national clips (#2030 decision 16), so it
	// keeps them; without a place there it keeps no local clip of its own.
	withCanada := harborPolicy().UsableAreas([]pod.Geography{{Country: "CA"}})
	if !pod.UsableIn(canadian, withCanada) {
		t.Error("a Canadian channel did not keep a Canadian national clip")
	}
	if pod.UsableIn(lakesideLocal, withCanada) {
		t.Error("a Canadian channel kept a US station's clip")
	}
	// A region fallback country's national clips air at home, so they stay (#2035).
	withFallback := harborPolicy()
	withFallback.Geography.Fallback = "CA"
	if !pod.UsableIn(canadian, withFallback.UsableAreas(nil)) {
		t.Error("a national clip from the region fallback was out of bounds")
	}
	if !pod.UsableIn(canadian, pod.Policy{}.UsableAreas(nil)) {
		t.Error("with no installation country, a clip was out of bounds")
	}
}
