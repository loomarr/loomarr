package filler_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

func geoCommercial(id string, scope filler.GeographicScope, country, station string) filler.Clip {
	return filler.Clip{Hash: id, Path: id + ".mp4", Name: id, Kind: filler.Commercial,
		Era: 1992, Audience: filler.Kids, DurationMs: 30_000,
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

func harborPolicy() filler.Policy {
	return filler.Policy{Geography: filler.Geography{Country: "us", Place: " harbor "}, LocalStations: localStations}
}

func TestAssemble_GeographyIsNeverRelaxedOrBypassedByPin(t *testing.T) {
	catalog := []filler.Clip{
		geoCommercial("us-national", filler.GeographicNational, "US", ""),
		geoCommercial("harbor-local", filler.GeographicLocal, "US", "fcc:1"),
		geoCommercial("lakeside-local", filler.GeographicLocal, "US", "fcc:3"),
		geoCommercial("canadian", filler.GeographicNational, "CA", ""),
		geoCommercial("unknown", filler.GeographicUnknown, "", ""),
	}
	w := filler.Window{Seed: 7, Era: filler.Year(2015), Audience: filler.Kids, GapMs: 120_000, PodMax: 5,
		Pinned: []string{"canadian", "lakeside-local", "unknown"}}
	pod := filler.Assemble(catalog, w, harborPolicy(), nil)

	seen := map[string]bool{}
	for _, entry := range pod.Entries {
		seen[entry.Hash] = true
	}
	if !seen["us-national"] && !seen["harbor-local"] {
		t.Fatalf("pod has no compatible content: %+v", pod.Entries)
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
	at := func(id, country string, scope filler.GeographicScope, era int) filler.Clip {
		c := geoCommercial(id, scope, country, "")
		c.Era = era
		return c
	}
	catalog := []filler.Clip{
		at("us-1992", "US", filler.GeographicNational, 1992),
		at("au-1992", "AU", filler.GeographicNational, 1992),
		at("gb-1992", "GB", filler.GeographicNational, 1992),
		at("us-2000", "US", filler.GeographicNational, 2000), // the widened rung
		at("gb-local", "GB", filler.GeographicLocal, 1992),
		at("fr-1992", "FR", filler.GeographicNational, 1992), // not listed
	}
	policy := filler.Policy{Geography: filler.Geography{Country: "US", Fallback: "gb, AU,US"}}
	aired := func(podMax int) map[string]bool {
		w := filler.Window{Seed: 3, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 300_000, PodMax: podMax}
		seen := map[string]bool{}
		for _, e := range filler.Assemble(catalog, w, policy, nil).Entries {
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
	beer := func(id string) filler.Clip {
		c := geoCommercial(id, filler.GeographicNational, "US", "")
		c.Category = "beer"
		return c
	}
	car := geoCommercial("gb-car", filler.GeographicNational, " gb ", "")
	car.Category = "car"
	pod := filler.Assemble([]filler.Clip{beer("us-beer-1"), beer("us-beer-2"), car},
		filler.Window{Seed: 5, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 90_000, PodMax: 3},
		filler.Policy{Geography: filler.Geography{Country: "US", Fallback: "GB"}}, nil)
	var cats []string
	for _, e := range pod.Entries {
		cats = append(cats, e.Category)
	}
	if want := []string{"beer", "car", "beer"}; !slices.Equal(cats, want) {
		t.Fatalf("pod categories %v, want %v", cats, want)
	}
}

// A channel with its own place in the home country still inherits the preference, and a fallback
// country's local ads stay out.
func TestGeographicallyEligible_APlaceChannelInheritsTheFallback(t *testing.T) {
	policy := filler.Policy{Geography: filler.Geography{Country: "US", Fallback: "GB"}, LocalStations: localStations}
	w := filler.Window{Seed: 2, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 120_000, PodMax: 4,
		Geography: filler.Geography{Country: "US", Place: "lakeside"}}
	seen := map[string]bool{}
	for _, e := range filler.Assemble([]filler.Clip{
		geoCommercial("gb-national", filler.GeographicNational, "gb", ""),
		geoCommercial("gb-local", filler.GeographicLocal, "GB", "fcc:3"),
	}, w, policy, nil).Entries {
		seen[e.Hash] = true
	}
	if !seen["gb-national"] || seen["gb-local"] {
		t.Fatalf("a lakeside channel aired %v, want the GB national ad only", seen)
	}
}

func TestSourceGeographicallyEligible_AFallbackCountryIsNationalOnly(t *testing.T) {
	home := filler.Geography{Country: "US", Fallback: "GB"}
	cases := []struct {
		source filler.Geography
		want   bool
	}{
		{filler.Geography{Country: "gb"}, true},
		{filler.Geography{Country: "AU"}, false},
		{filler.Geography{Country: "US"}, true},
	}
	for _, tc := range cases {
		if got := filler.SourceGeographicallyEligible(tc.source, home); got != tc.want {
			t.Errorf("source %+v eligible = %v, want %v", tc.source, got, tc.want)
		}
	}
	if got := home.Normalize().RegionRank("gb"); got != 1 {
		t.Fatalf("GB ranks %d, want 1, behind home", got)
	}
	if got := (filler.Geography{Country: "US", Fallback: " gb, CA,,gb ,us"}).Normalize().Fallback; got != "GB,CA" {
		t.Fatalf("normalized fallback %q, want GB,CA", got)
	}
}

func TestFetch_AsksHomeSourcesBeforeFallbackCountries(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{
			{ID: "au", Kind: "archive", URI: "au", Enabled: true, Region: 2},
			{ID: "us", Kind: "archive", URI: "us", Enabled: true},
			{ID: "gb", Kind: "archive", URI: "gb", Enabled: true, Region: 1},
		},
		offers: refs("a"),
	}
	if _, err := newFetcher(t, stub, limits(1, 2000)).Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"us", "gb", "au"}; !slices.Equal(stub.listed, want) {
		t.Fatalf("listed %v, want %v", stub.listed, want)
	}
}

func TestAssemble_GeographyMismatchFallsBackToCard(t *testing.T) {
	pod := filler.Assemble([]filler.Clip{
		geoCommercial("lakeside-local", filler.GeographicLocal, "US", "fcc:3"),
	}, filler.Window{Seed: 1, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 60_000, PodMax: 4},
		harborPolicy(), nil)
	if pod.MatchLevel != filler.MatchBumperCard || len(pod.Entries) != 1 || !pod.Entries[0].IsFallbackCard {
		t.Fatalf("mismatch pod = %+v, want one bumper card", pod)
	}
}

// #2030 decision 16: a channel's own location replaces the household's, another country included,
// so a channel of British shows can opt into British commercials in a US household. The
// household's place stays home (#2059 decision 8): a British channel airs British station clips
// only near a British place of its own.
func TestAssemble_AChannelMayOptIntoAnotherCountry(t *testing.T) {
	catalog := []filler.Clip{
		geoCommercial("us-national", filler.GeographicNational, "US", ""),
		geoCommercial("harbor-local", filler.GeographicLocal, "US", "fcc:1"),
		geoCommercial("gb-national", filler.GeographicNational, "GB", ""),
		geoCommercial("moorside-local", filler.GeographicLocal, "GB", "wd:Q1"),
	}
	policy := harborPolicy()
	for _, tc := range []struct {
		channel     filler.Geography
		want, never []string
	}{
		{filler.Geography{Country: "GB"}, []string{"gb-national"}, []string{"us-national", "harbor-local", "moorside-local"}},
		{filler.Geography{Country: "GB", Place: "moorside"}, []string{"gb-national", "moorside-local"}, []string{"us-national", "harbor-local"}},
		{filler.Geography{}, []string{"us-national", "harbor-local"}, []string{"gb-national", "moorside-local"}},
	} {
		seen := map[string]bool{}
		for seed := range int64(40) {
			w := filler.Window{Seed: seed, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 60_000, PodMax: 2,
				Geography: tc.channel}
			for _, e := range filler.Assemble(catalog, w, policy, nil).Entries {
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
	harbor := filler.Geography{Country: "US", Place: "harbor"}.Resolve(localStations)
	countryOnly := filler.Geography{Country: "US"}.Resolve(localStations)
	unresolved := filler.Geography{Country: "US", Place: "harbor"}.Resolve(nil)
	for _, c := range []struct {
		name string
		clip filler.Clip
		area filler.Area
		want bool
	}{
		{"near station", geoCommercial("a", filler.GeographicLocal, "US", "fcc:2"), harbor, true},
		{"far station", geoCommercial("b", filler.GeographicLocal, "US", "fcc:3"), harbor, false},
		{"local without station", geoCommercial("c", filler.GeographicLocal, "US", ""), harbor, false},
		{"local, no place", geoCommercial("d", filler.GeographicLocal, "US", "fcc:1"), countryOnly, false},
		{"local, no resolver", geoCommercial("e", filler.GeographicLocal, "US", "fcc:1"), unresolved, false},
		{"national, no place", geoCommercial("f", filler.GeographicNational, "US", "fcc:3"), countryOnly, true},
		{"foreign national", geoCommercial("g", filler.GeographicNational, "CA", ""), harbor, false},
	} {
		if got := filler.GeographicallyEligible(c.clip, c.area); got != c.want {
			t.Errorf("%s: eligible = %v, want %v", c.name, got, c.want)
		}
	}
}

// A channel in another country still inherits the household's region preference (#2035): it is
// the household's alone. The channel's own country leaves the list, and home is not added to it.
func TestGeographicallyEligible_AnotherCountryChannelInheritsTheFallback(t *testing.T) {
	policy := filler.Policy{Geography: filler.Geography{Country: "US", Fallback: "GB,CA"}}
	w := filler.Window{Seed: 2, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 120_000, PodMax: 4,
		Geography: filler.Geography{Country: "GB"}}
	seen := map[string]bool{}
	for _, e := range filler.Assemble([]filler.Clip{
		geoCommercial("gb-national", filler.GeographicNational, "GB", ""),
		geoCommercial("ca-national", filler.GeographicNational, "CA", ""),
		geoCommercial("us-national", filler.GeographicNational, "US", ""),
	}, w, policy, nil).Entries {
		seen[e.Hash] = true
	}
	if !seen["gb-national"] || !seen["ca-national"] || seen["us-national"] {
		t.Fatalf("a GB channel aired %v, want the GB and CA national ads only", seen)
	}
}

func TestGeographicallyEligible_EmptyInstallationPreservesLegacyPool(t *testing.T) {
	if !filler.GeographicallyEligible(geoCommercial("unknown", filler.GeographicUnknown, "", ""), filler.Area{}) {
		t.Fatal("empty installation geography unexpectedly excluded a legacy clip")
	}
}

func TestSourceGeographyHardBoundary(t *testing.T) {
	target := filler.Geography{Country: "US", Place: "harbor"}
	cases := []struct {
		name   string
		source filler.Geography
		want   bool
	}{
		{"US-wide", filler.Geography{Country: "us"}, true},
		{"Canadian", filler.Geography{Country: "CA"}, false},
		{"unknown", filler.Geography{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := filler.SourceGeographicallyEligible(tc.source, target); got != tc.want {
				t.Fatalf("SourceGeographicallyEligible() = %v, want %v", got, tc.want)
			}
		})
	}
	if !filler.SourceGeographicallyEligible(filler.Geography{}, filler.Geography{}) {
		t.Fatal("unconfigured installation must preserve legacy source eligibility")
	}
}

// #2059 decision 8: a channel may pick its own place, and a channel that names only the household's
// country keeps the household's place.
func TestAssemble_ChannelPlaceOverridesOrInherits(t *testing.T) {
	catalog := []filler.Clip{geoCommercial("lakeside-local", filler.GeographicLocal, "US", "fcc:3")}
	for _, c := range []struct {
		name    string
		channel filler.Geography
		want    bool
	}{
		{"own place", filler.Geography{Country: "US", Place: "lakeside"}, true},
		{"country only inherits harbor", filler.Geography{Country: "US"}, false},
		{"unset inherits harbor", filler.Geography{}, false},
	} {
		pod := filler.Assemble(catalog, filler.Window{Seed: 1, Era: filler.Year(1992), Audience: filler.Kids,
			GapMs: 30_000, PodMax: 1, Geography: c.channel}, harborPolicy(), nil)
		aired := len(pod.Entries) == 1 && pod.Entries[0].Hash == "lakeside-local"
		if aired != c.want {
			t.Errorf("%s: lakeside clip aired = %v, want %v (%+v)", c.name, aired, c.want, pod.Entries)
		}
	}
}

// A break holds at most one station segment, last before the programme returns; local ads are
// ordinary ads and a local station ID never opens a break (#2065).
func TestBreakHoldsOneStationSegmentLast(t *testing.T) {
	segment := func(id string, kind filler.Kind) filler.Clip {
		c := geoCommercial(id, filler.GeographicLocal, "US", "fcc:1")
		c.Kind, c.DurationMs = kind, 15_000
		return c
	}
	bumper := geoCommercial("bumper-us", filler.GeographicNational, "US", "")
	bumper.Kind, bumper.DurationMs = filler.Bumper, 5_000
	catalog := []filler.Clip{
		segment("segment-news", filler.Interstitial), segment("segment-weather", filler.Interstitial),
		segment("segment-promo", filler.Interstitial), bumper,
		geoCommercial("local-ad", filler.GeographicLocal, "US", "fcc:2"),
		geoCommercial("ad-a", filler.GeographicNational, "US", ""), geoCommercial("ad-b", filler.GeographicNational, "US", ""),
		geoCommercial("ad-c", filler.GeographicNational, "US", ""),
	}
	// The only bookend is a local station ID: it may close a break, never open one.
	for seed := int64(1); seed <= 20; seed++ {
		w := filler.Window{Seed: seed, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 240_000, PodMax: 8}
		idOnly := append([]filler.Clip{segment("segment-id", filler.StationID)}, catalog[4:]...)
		if pod := filler.Assemble(idOnly, w, harborPolicy(), nil); pod.Entries[0].Hash == "segment-id" {
			t.Fatalf("seed %d: a local station ID opened the break", seed)
		}
	}
	sawSegment, sawLocalAd := false, false
	for seed := int64(1); seed <= 40; seed++ {
		w := filler.Window{Seed: seed, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 240_000, PodMax: 8}
		pod := filler.Assemble(catalog, w, harborPolicy(), nil)
		segments := []int{}
		for i, e := range pod.Entries {
			if strings.HasPrefix(e.Hash, "segment-") {
				segments = append(segments, i)
			}
			sawLocalAd = sawLocalAd || e.Hash == "local-ad"
		}
		if len(segments) > 1 {
			t.Fatalf("seed %d: %d station segments in one break: %+v", seed, len(segments), pod.Entries)
		}
		if len(segments) == 0 {
			continue
		}
		sawSegment = true
		at, n := segments[0], len(pod.Entries)
		if at == 0 && n > 1 {
			t.Fatalf("seed %d: a station segment opened the break", seed)
		}
		// Last, or second to last before the return bumper.
		last := at == n-1 || (at == n-2 && pod.Entries[n-1].Hash == "bumper-us")
		if !last {
			t.Fatalf("seed %d: station segment at %d of %d: %+v", seed, at, n, pod.Entries)
		}
	}
	if !sawSegment || !sawLocalAd {
		t.Fatalf("segment seen %v, local ad seen %v; the catalog should air both", sawSegment, sawLocalAd)
	}
}

// A clip stays while any current place can air it: the installation's area or a channel's own
// place. A foreign national clip, or a local one no place is near, has no area (#2064).
func TestUsableAreasKeepClipsAnyPlaceCanAir(t *testing.T) {
	lakesideLocal := geoCommercial("lakeside-local", filler.GeographicLocal, "US", "fcc:3")
	canadian := geoCommercial("canadian", filler.GeographicNational, "CA", "")
	harborLocal := geoCommercial("harbor-local", filler.GeographicLocal, "US", "fcc:1")

	installationOnly := harborPolicy().UsableAreas(nil)
	if !filler.UsableIn(harborLocal, installationOnly) {
		t.Error("the installation's own station is unusable")
	}
	if filler.UsableIn(lakesideLocal, installationOnly) || filler.UsableIn(canadian, installationOnly) {
		t.Error("a clip no place can air is still usable")
	}
	withLakeside := harborPolicy().UsableAreas([]filler.Geography{{Country: "US", Place: "lakeside"}})
	if !filler.UsableIn(lakesideLocal, withLakeside) {
		t.Error("a channel's own place did not keep its station's clip")
	}
	// A channel in another country airs that country's national clips (#2030 decision 16), so it
	// keeps them; without a place there it keeps no local clip of its own.
	withCanada := harborPolicy().UsableAreas([]filler.Geography{{Country: "CA"}})
	if !filler.UsableIn(canadian, withCanada) {
		t.Error("a Canadian channel did not keep a Canadian national clip")
	}
	if filler.UsableIn(lakesideLocal, withCanada) {
		t.Error("a Canadian channel kept a US station's clip")
	}
	// A region fallback country's national clips air at home, so they stay (#2035).
	withFallback := harborPolicy()
	withFallback.Geography.Fallback = "CA"
	if !filler.UsableIn(canadian, withFallback.UsableAreas(nil)) {
		t.Error("a national clip from the region fallback was out of bounds")
	}
	if !filler.UsableIn(canadian, filler.Policy{}.UsableAreas(nil)) {
		t.Error("with no installation country, a clip was out of bounds")
	}
}
