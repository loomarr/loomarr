package filler_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

// #749: acquisition steers toward the eras of channels that cannot fill their breaks from their
// own era, as a ranking preference that never rejects a candidate.

func TestPlanAcquisitionFor_GapEraOutranksRepresentationQuality(t *testing.T) {
	input := []filler.AcquisitionCandidate{
		candidate("a", "modern-hd", 2015, 1080, ""),
		candidate("a", "nineties-sd", 1994, 480, ""),
	}
	gaps := []filler.EraRange{{From: 1990, To: 1999}}
	plan, err := filler.PlanAcquisitionFor(filler.AcquisitionIntent{Count: 1}, input, nil, gaps)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Selected[0].Candidate.Identity.RemoteID; got != "nineties-sd" {
		t.Fatalf("selected %q, want the gap-era item ahead of the sharper one", got)
	}
	if !strings.Contains(plan.Selected[0].Detail, "coverage gap") {
		t.Fatalf("detail = %q, want the gap named as the reason", plan.Selected[0].Detail)
	}
	if got := plan.Selected[0].Gap; got != "era:1990-1999" {
		t.Fatalf("decision gap = %q, want the gap it was selected for recorded", got)
	}
	// No gaps: the historical quality ranking is unchanged.
	plain, err := filler.PlanAcquisition(filler.AcquisitionIntent{Count: 1}, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.Selected[0].Candidate.Identity.RemoteID; got != "modern-hd" {
		t.Fatalf("unsteered selection = %q, want the HD item", got)
	}
}

func TestPlanAcquisitionFor_AGapNeverRejectsACandidate(t *testing.T) {
	input := []filler.AcquisitionCandidate{
		candidate("a", "modern", 2015, 720, ""),
		candidate("a", "undated", 0, 720, ""),
	}
	plan, err := filler.PlanAcquisitionFor(filler.AcquisitionIntent{Count: 2}, input, nil,
		[]filler.EraRange{{From: 1990, To: 1999}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Selected) != 2 || len(plan.Rejected) != 0 {
		t.Fatalf("selected %d rejected %+v, want both kept: a gap ranks, it does not filter", len(plan.Selected), plan.Rejected)
	}
	for _, d := range plan.Selected {
		if d.Gap != "" {
			t.Fatalf("%s recorded gap %q, want none: it fills no gap", d.Candidate.Identity.RemoteID, d.Gap)
		}
	}
}

// Merged windows stay one gap, and open-ended ones keep a stable key.
func TestGapKey(t *testing.T) {
	for _, tc := range []struct {
		r    filler.EraRange
		want string
	}{
		{filler.EraRange{From: 1990, To: 1999}, "era:1990-1999"},
		{filler.EraRange{From: 2005}, "era:2005-"},
		{filler.EraRange{To: 1979}, "era:-1979"},
		{filler.EraRange{}, ""},
	} {
		if got := filler.EraGapKey(tc.r); got != tc.want {
			t.Errorf("EraGapKey(%+v) = %q, want %q", tc.r, got, tc.want)
		}
	}
}

func TestCoverageGapEras_AreTheErasOfChannelsBelowExact(t *testing.T) {
	channel := func(level filler.MatchLevel, windows ...filler.EraRange) filler.ChannelCoverage {
		return filler.ChannelCoverage{Report: filler.CoverageReport{Level: level, EraWindows: windows}}
	}
	pool := filler.PoolReport{Channels: []filler.ChannelCoverage{
		channel(filler.MatchExact, filler.EraRange{From: 1980, To: 1989}),    // covered: no gap
		channel(filler.MatchAudience, filler.EraRange{From: 1990, To: 1994}), // gap
		channel(filler.MatchBumperCard, filler.EraRange{From: 1995, To: 1999}),
		channel(filler.MatchWidened), // any era: nothing to steer by
		channel(filler.MatchWidened, filler.EraRange{From: 2010, To: 2012}),
	}}
	got := filler.CoverageGapEras(pool)
	want := []filler.EraRange{{From: 1990, To: 1999}, {From: 2010, To: 2012}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gaps = %+v, want %+v", got, want)
	}
}

func TestCoverageReportsTheEraTargetItRead(t *testing.T) {
	if got := filler.Coverage(sampleCatalog(), kidsWindow(1), filler.Policy{}).EraWindows; !reflect.DeepEqual(got, []filler.EraRange{filler.Year(1992)}) {
		t.Fatalf("era windows = %+v, want the channel's 1992 target", got)
	}
	anyEra := filler.Window{ChannelID: "ch", Audience: filler.Kids, GapMs: 120_000, PodMax: 4}
	if got := filler.Coverage(sampleCatalog(), anyEra, filler.Policy{}).EraWindows; got != nil {
		t.Fatalf("era windows = %+v, want nil for an any-era channel", got)
	}
}

func TestFetch_ScheduledPassTakesGapEraItemsFirst(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		offers: []filler.DiscoveredRef{
			{ID: "modern-hd", URL: "https://archive.org/details/modern-hd", Height: 1080, ObservedYear: 2015},
			{ID: "nineties", URL: "https://archive.org/details/nineties", Height: 480, ObservedYear: 1996},
		},
	}
	fetcher := newFetcher(t, stub, limits(1, 2000)).WithCoverageGaps(func(context.Context) ([]filler.EraRange, error) {
		return []filler.EraRange{{From: 1990, To: 1999}}, nil
	})
	if _, err := fetcher.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(stub.queuedIDs) != 1 || stub.queuedIDs[0] != "nineties" {
		t.Fatalf("queued %v, want the gap-era item first", stub.queuedIDs)
	}
	if !reflect.DeepEqual(stub.queuedGaps, []string{"era:1990-1999"}) {
		t.Fatalf("queued gaps %q, want the download tagged with the gap it was for", stub.queuedGaps)
	}
}

// Steering is an optimisation: a failed coverage read must not cost the source its refresh.
func TestFetch_UnavailableGapsDegradeToTheUnsteeredPass(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		offers:  refs("a", "b"),
	}
	fetcher := newFetcher(t, stub, limits(2, 2000)).WithCoverageGaps(func(context.Context) ([]filler.EraRange, error) {
		return nil, errors.New("coverage unavailable")
	})
	res, err := fetcher.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 2 {
		t.Fatalf("queued %d, want the bounded refresh to proceed without steering", res.Queued)
	}
}
