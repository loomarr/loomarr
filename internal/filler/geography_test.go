package filler_test

import (
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
