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
	gaps := filler.CoverageGaps{Eras: []filler.EraRange{{From: 1990, To: 1999}}}
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
		filler.CoverageGaps{Eras: []filler.EraRange{{From: 1990, To: 1999}}, Roles: []filler.Kind{filler.StationID}})
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
	pool.Bookends = 3
	got := filler.CoverageGapsFrom(pool, filler.PoolTargets{}, 2026)
	want := filler.CoverageGaps{Eras: []filler.EraRange{{From: 1990, To: 1999}, {From: 2010, To: 2012}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gaps = %+v, want %+v", got, want)
	}
}

// A channel can fill every break from its own era and still be short of an evening (#2035): its
// era is a gap until the exact rung reaches the target, and Short is the largest shortfall.
func TestCoverageGapsFrom_AChannelShortOfItsTargetIsAGap(t *testing.T) {
	exact := func(clips int, era filler.EraRange) filler.ChannelCoverage {
		return filler.ChannelCoverage{Report: filler.CoverageReport{
			Level: filler.MatchExact, EraWindows: []filler.EraRange{era},
			Rungs: []filler.RungCoverage{{Level: filler.MatchExact, Clips: clips}, {Level: filler.MatchWidened, Clips: clips + 40}},
		}}
	}
	targets := filler.PoolTargets{Decade: 50, PerDecade: map[int]float64{1980: 80}}
	pool := filler.PoolReport{Bookends: 2, Channels: []filler.ChannelCoverage{
		exact(30, filler.EraRange{From: 1980, To: 1989}), // 80 wanted: 50 short
		exact(60, filler.EraRange{From: 1990, To: 1999}), // the default, 50: met
		exact(90, filler.EraRange{From: 2000, To: 2019}), // two decades at the default, 100: 10 short
	}}
	got := filler.CoverageGapsFrom(pool, targets, 2026)
	want := filler.CoverageGaps{Eras: []filler.EraRange{{From: 1980, To: 1989}, {From: 2000, To: 2019}}, Short: 50}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gaps = %+v, want %+v", got, want)
	}
	if got.Met() {
		t.Fatal("a short pool reads as met")
	}
	pool.Channels[0] = exact(80, filler.EraRange{From: 1980, To: 1989})
	pool.Channels[2] = exact(100, filler.EraRange{From: 2000, To: 2019})
	if got := filler.CoverageGapsFrom(pool, targets, 2026); !got.Met() {
		t.Fatalf("a pool at its target = %+v, want met", got)
	}
}

// A role gap: live channels whose breaks have no bookend, because the catalog holds no bumper or
// station ID at all. Every pod then opens and closes on the break body alone.
func TestCoverageGapsFrom_NoBookendIsARoleGap(t *testing.T) {
	live := filler.PoolReport{Channels: []filler.ChannelCoverage{{Report: filler.CoverageReport{Level: filler.MatchExact}}}}
	want := []filler.Kind{filler.Bumper, filler.StationID}
	if got := filler.CoverageGapsFrom(live, filler.PoolTargets{}, 2026).Roles; !reflect.DeepEqual(got, want) {
		t.Fatalf("role gaps with no bookends = %v, want %v", got, want)
	}
	live.Bookends = 1
	if got := filler.CoverageGapsFrom(live, filler.PoolTargets{}, 2026).Roles; got != nil {
		t.Fatalf("role gaps with a bookend = %v, want none", got)
	}
	if got := filler.CoverageGapsFrom(filler.PoolReport{}, filler.PoolTargets{}, 2026).Roles; got != nil {
		t.Fatalf("role gaps with no live channel = %v, want none: nothing is airing", got)
	}
}

// A bookend role is observed the way intake will classify the file: the explicit role token in
// its title (KindFromName). A sharper commercial does not fill the gap; the titled ident does.
func TestPlanAcquisitionFor_RoleGapTakesTitledBookendsFirst(t *testing.T) {
	ident := candidate("a", "ident", 0, 480, "")
	ident.Title = "Local station ID loop"
	spot := candidate("a", "spot", 0, 1080, "")
	spot.Title = "Soda commercial"
	plan, err := filler.PlanAcquisitionFor(filler.AcquisitionIntent{Count: 1}, []filler.AcquisitionCandidate{spot, ident}, nil,
		filler.CoverageGaps{Roles: []filler.Kind{filler.Bumper, filler.StationID}})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Selected[0]; got.Candidate.Identity.RemoteID != "ident" || got.Gap != "role:station_id" {
		t.Fatalf("selected %q gap %q, want the ident for role:station_id", got.Candidate.Identity.RemoteID, got.Gap)
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
	fetcher := newFetcher(t, stub, limits(1, 2000)).WithCoverageGaps(func(context.Context) (filler.CoverageGaps, error) {
		return filler.CoverageGaps{Eras: []filler.EraRange{{From: 1990, To: 1999}}}, nil
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

// searchingStub also searches inside a source, as the archive enumerator does.
type searchingStub struct {
	*fetchStub
	hits     map[string][]filler.DiscoveredRef
	searched []string
}

func (s *searchingStub) Search(_ context.Context, source filler.FetchSource, query string, _ int) ([]filler.DiscoveredRef, error) {
	s.searched = append(s.searched, source.URI+"?"+query)
	return s.hits[query], nil
}

// #2059 decision 9: a short pass searches each archive source for the in-radius call signs and takes
// a station hit ahead of a better-looking listed item; a pass at target searches nothing.
func TestFetch_AShortPassSearchesSourcesForNearbyStations(t *testing.T) {
	stub := &searchingStub{
		fetchStub: &fetchStub{
			sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
			offers:  []filler.DiscoveredRef{{ID: "listed-hd", URL: "https://archive.org/details/listed-hd", Height: 1080}},
		},
		hits: map[string][]filler.DiscoveredRef{
			"WAAA": {{ID: "waaa-news", URL: "https://archive.org/details/waaa-news", Title: "WAAA news update 1991", Height: 360}},
		},
	}
	gaps := filler.CoverageGaps{Eras: []filler.EraRange{{From: 1990, To: 1999}}, Short: 5, Stations: []string{"WAAA", "WBBB"}}
	fetcher := filler.NewFetcher(fetchStoreWithRemoteStates{
		fetchStub:        stub.fetchStub,
		listRemoteStates: func(context.Context) (map[string]filler.ExistingRemoteState, error) { return nil, nil },
	}, stub, stub.fetchStub, limits(1, 2000), discardLog()).
		WithCoverageGaps(func(context.Context) (filler.CoverageGaps, error) { return gaps, nil })
	if _, err := fetcher.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stub.searched, []string{"coll?WAAA", "coll?WBBB"}) &&
		!reflect.DeepEqual(stub.searched, []string{"coll?WBBB", "coll?WAAA"}) {
		t.Fatalf("searched %v, want each nearby call sign inside the source", stub.searched)
	}
	if len(stub.queuedIDs) != 1 || stub.queuedIDs[0] != "waaa-news" {
		t.Fatalf("queued %v, want the station hit ahead of the listed item", stub.queuedIDs)
	}
	if !reflect.DeepEqual(stub.queuedGaps, []string{"station:WAAA"}) {
		t.Fatalf("queued gaps %q, want the download tagged with the station search that found it", stub.queuedGaps)
	}
	// Local material is a flavour, not a quota: nearby stations alone never make a target unmet.
	if !(filler.CoverageGaps{Stations: []string{"WAAA"}}).Met() {
		t.Fatal("station searches made a met target unmet")
	}
}

type openSearchStub struct {
	hits     map[string][]filler.DiscoveredRef
	searched []string
}

func (o *openSearchStub) OpenSearch(_ context.Context, query string, _ int) ([]filler.DiscoveredRef, error) {
	o.searched = append(o.searched, query)
	return o.hits[query], nil
}

func (o *openSearchStub) Gate(context.Context) (filler.ListingGate, error) {
	return filler.ListingGate{Languages: "en", Station: func(title string) (string, bool) {
		if strings.Contains(title, "WAAA") {
			return "WAAA", true
		}
		return "", false
	}}, nil
}

// Open search (#2059 decisions 10 and 11) asks the whole provider only once the sources have left
// the pass short, and downloads only the hits its listing gate admits, under no registered source.
func TestFetch_OpenSearchRunsOnlyWhenTheSourcesLeaveThePassShort(t *testing.T) {
	open := &openSearchStub{hits: map[string][]filler.DiscoveredRef{"WAAA": {
		{ID: "waaa-1984", URL: "https://archive.org/details/waaa-1984", Title: "WAAA-TV station ID 1984"},
		{ID: "waaa-undated", URL: "https://archive.org/details/waaa-undated", Title: "WAAA-TV station ID"},
		{ID: "wzzz-1984", URL: "https://archive.org/details/wzzz-1984", Title: "WZZZ-TV station ID 1984"},
	}}}
	gaps := filler.CoverageGaps{Eras: []filler.EraRange{{From: 1980, To: 1989}}, Short: 5, Stations: []string{"WAAA"}}
	stub := &fetchStub{sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}}}
	fetcher := newFetcher(t, stub, limits(10, 2000)).
		WithCoverageGaps(func(context.Context) (filler.CoverageGaps, error) { return gaps, nil }).
		WithOpenSearch(open)
	if _, err := fetcher.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(open.searched, []string{"WAAA"}) {
		t.Fatalf("searched %v, want the nearby call sign across the provider", open.searched)
	}
	if !reflect.DeepEqual(stub.queuedIDs, []string{"waaa-1984"}) || stub.sourceID != filler.OpenSearchSourceID ||
		!reflect.DeepEqual(stub.queuedGaps, []string{"station:WAAA"}) {
		t.Fatalf("queued %v from %q for %q, want only the dated nearby hit, under open search", stub.queuedIDs,
			stub.sourceID, stub.queuedGaps)
	}

	for name, c := range map[string]struct {
		offers []filler.DiscoveredRef
		gaps   filler.CoverageGaps
	}{
		"the sources fill the shortfall": {refs("a", "b"), filler.CoverageGaps{Eras: gaps.Eras, Short: 2, Stations: gaps.Stations}},
		"no era is short":                {nil, filler.CoverageGaps{Short: 5, Stations: gaps.Stations}},
		"no station is near":             {nil, filler.CoverageGaps{Eras: gaps.Eras, Short: 5}},
	} {
		open.searched = nil
		stub := &fetchStub{sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}}, offers: c.offers}
		fetcher := newFetcher(t, stub, limits(10, 2000)).
			WithCoverageGaps(func(context.Context) (filler.CoverageGaps, error) { return c.gaps, nil }).
			WithOpenSearch(open)
		if _, err := fetcher.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		if len(open.searched) != 0 {
			t.Errorf("%s: open search asked for %v", name, open.searched)
		}
	}
}

// After a purge (#2035), an undersized pool refills unattended, without a pull or an approval: each
// pass fetches what the pool is short, however many sources are due, and the pass after the target
// is met fetches nothing.
func TestFetch_AnUndersizedPoolRefillsToItsTargetThenStops(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{
			{ID: "s1", Kind: "archive", URI: "one", Enabled: true},
			{ID: "s2", Kind: "archive", URI: "two", Enabled: true},
		},
		offers: refs("a", "b", "c", "d", "e"),
	}
	short := 3
	fetcher := newFetcher(t, stub, limits(2, 2000)).WithCoverageGaps(func(context.Context) (filler.CoverageGaps, error) {
		return filler.CoverageGaps{Eras: []filler.EraRange{{From: 1980, To: 1989}}, Short: short}, nil
	})
	res, err := fetcher.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 3 {
		t.Fatalf("pass = %+v queued %v, want the three the pool is short", res, stub.queuedIDs)
	}
	// The source capped to the last one still over-lists as far as an uncapped one.
	if want := []int{8, 8}; !reflect.DeepEqual(stub.listLimits, want) {
		t.Fatalf("listed %v, want %v: the shortfall caps what is taken, not what is looked at", stub.listLimits, want)
	}

	stub.queuedIDs = nil
	reads := 0
	fetcher = newFetcher(t, stub, limits(2, 2000)).WithCoverageGaps(func(context.Context) (filler.CoverageGaps, error) {
		reads++
		return filler.CoverageGaps{}, nil
	})
	for range 2 {
		res, err = fetcher.Run(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if res.Queued != 0 || res.SourcesPolled != 0 || res.StoppedBy != "target" {
			t.Fatalf("pass at target = %+v, want no source asked", res)
		}
	}
	if reads != 1 {
		t.Fatalf("two passes at target read the pool %d times, want once until the recheck", reads)
	}
	// The operator's click is not held to the target.
	if res, err := fetcher.RunSource(t.Context(), "s1"); err != nil || res.Queued == 0 {
		t.Fatalf("a manual check at target = %+v, %v; want it to fetch", res, err)
	}
}

// Steering is an optimisation: a failed coverage read must not cost the source its refresh.
func TestFetch_UnavailableGapsDegradeToTheUnsteeredPass(t *testing.T) {
	stub := &fetchStub{
		sources: []filler.FetchSource{{ID: "s1", Kind: "archive", URI: "coll", Enabled: true}},
		offers:  refs("a", "b"),
	}
	fetcher := newFetcher(t, stub, limits(2, 2000)).WithCoverageGaps(func(context.Context) (filler.CoverageGaps, error) {
		return filler.CoverageGaps{}, errors.New("coverage unavailable")
	})
	res, err := fetcher.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 2 {
		t.Fatalf("queued %d, want the bounded refresh to proceed without steering", res.Queued)
	}
}
