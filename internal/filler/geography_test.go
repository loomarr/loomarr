package filler_test

import (
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

func geoCommercial(id string, scope filler.GeographicScope, country, market string) filler.Clip {
	return filler.Clip{Hash: id, Path: id + ".mp4", Name: id, Kind: filler.Commercial,
		Era: 1992, Audience: filler.Kids, DurationMs: 30_000,
		GeographicScope: scope, Country: country, Market: market}
}

func TestAssemble_GeographyIsNeverRelaxedOrBypassedByPin(t *testing.T) {
	catalog := []filler.Clip{
		geoCommercial("us-national", filler.GeographicNational, "US", ""),
		geoCommercial("ny-local", filler.GeographicLocal, "US", "New York"),
		geoCommercial("ca-local", filler.GeographicLocal, "US", "California"),
		geoCommercial("canadian", filler.GeographicNational, "CA", ""),
		geoCommercial("unknown", filler.GeographicUnknown, "", ""),
	}
	w := filler.Window{Seed: 7, Era: filler.Year(2015), Audience: filler.Kids, GapMs: 120_000, PodMax: 5,
		Pinned: []string{"canadian", "ca-local", "unknown"}}
	policy := filler.Policy{Geography: filler.Geography{Country: "us", Market: " New   York "}}
	pod := filler.Assemble(catalog, w, policy, nil)

	seen := map[string]bool{}
	for _, entry := range pod.Entries {
		seen[entry.Hash] = true
	}
	if !seen["us-national"] && !seen["ny-local"] {
		t.Fatalf("pod has no compatible content: %+v", pod.Entries)
	}
	for _, forbidden := range []string{"canadian", "ca-local", "unknown"} {
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

// A channel narrowed to a market in the home country still inherits the preference, and a fallback
// country's local ads stay out.
func TestGeographicallyEligible_AMarketChannelInheritsTheFallback(t *testing.T) {
	policy := filler.Policy{Geography: filler.Geography{Country: "US", Fallback: "GB"}}
	w := filler.Window{Seed: 2, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 120_000, PodMax: 4,
		Geography: filler.Geography{Country: "US", Market: "Seattle"}}
	seen := map[string]bool{}
	for _, e := range filler.Assemble([]filler.Clip{
		geoCommercial("gb-national", filler.GeographicNational, "gb", ""),
		geoCommercial("gb-local", filler.GeographicLocal, "GB", "London"),
	}, w, policy, nil).Entries {
		seen[e.Hash] = true
	}
	if !seen["gb-national"] || seen["gb-local"] {
		t.Fatalf("a Seattle channel aired %v, want the GB national ad only", seen)
	}
}

func TestSourceGeographicallyEligible_AFallbackCountryIsNationalOnly(t *testing.T) {
	home := filler.Geography{Country: "US", Fallback: "GB"}
	cases := []struct {
		source filler.Geography
		want   bool
	}{
		{filler.Geography{Country: "gb"}, true},
		{filler.Geography{Country: "GB", Market: "London"}, false},
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
		geoCommercial("ca-local", filler.GeographicLocal, "US", "California"),
	}, filler.Window{Seed: 1, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 60_000, PodMax: 4},
		filler.Policy{Geography: filler.Geography{Country: "US", Market: "New York"}}, nil)
	if pod.MatchLevel != filler.MatchBumperCard || len(pod.Entries) != 1 || !pod.Entries[0].IsFallbackCard {
		t.Fatalf("mismatch pod = %+v, want one bumper card", pod)
	}
}

func TestGeographicallyEligible_EmptyInstallationPreservesLegacyPool(t *testing.T) {
	if !filler.GeographicallyEligible(geoCommercial("unknown", filler.GeographicUnknown, "", ""), filler.Geography{}) {
		t.Fatal("empty installation geography unexpectedly excluded a legacy clip")
	}
}

func TestSourceGeographyHardBoundary(t *testing.T) {
	target := filler.Geography{Country: "US", Market: "New York"}
	cases := []struct {
		name   string
		source filler.Geography
		want   bool
	}{
		{"US-wide", filler.Geography{Country: "us"}, true},
		{"New York local", filler.Geography{Country: "US", Market: "New York"}, true},
		{"California local", filler.Geography{Country: "US", Market: "California"}, false},
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

func TestAssemble_ChannelCannotEscapeInstallationCountry(t *testing.T) {
	pod := filler.Assemble(
		[]filler.Clip{geoCommercial("canadian", filler.GeographicNational, "CA", "")},
		filler.Window{
			Seed: 1, Era: filler.Year(1992), Audience: filler.Kids, GapMs: 30_000, PodMax: 1,
			Geography: filler.Geography{Country: "CA", Market: "Toronto"},
		},
		filler.Policy{Geography: filler.Geography{Country: "US", Market: "New York"}}, nil,
	)
	if pod.MatchLevel != filler.MatchBumperCard {
		t.Fatalf("cross-country channel match = %s, want bumper card", pod.MatchLevel)
	}
}
