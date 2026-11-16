package schedule

// Property tests for relaxation (programming-design §7). These are seeded, in-package plain-Go
// loops (math/rand with a fixed source), so any failure reproduces from the printed trial index.
//
// They assert, for any policy and pool:
//
//  1. ONE GUARD AT A TIME: a guard is recorded only when the placement under the written policy
//     breaches it, so a gap breach never shortens a no-repeat window, and the reverse.
//  2. WIDEST THAT HOLDS: the relaxed gap holds and a minute more does not; the relaxed run cap
//     holds and one less does not; a relaxed no-repeat window is the cycle's runtime.
//  3. NEVER-RELAXED: audience ceiling and scope (series, seasons, dates) are untouched.

import (
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/rating"
)

// randResolvedPolicy builds a random ResolvedPolicy exercising every relaxable field, with the
// audience ceiling and scope set to non-zero sentinels so a mutation would be caught.
func randResolvedPolicy(g *rand.Rand) ResolvedPolicy {
	ceilings := []rating.Rating{"", "TV-Y", "TV-Y7", "TV-G", "TV-PG", "TV-14", "TV-MA", "PG-13", "R"}
	rp := ResolvedPolicy{
		Ceiling:  ceilings[g.Intn(len(ceilings))],
		Ordering: OrderSyndication,
		Sep: ResolvedSeparation{
			EpisodeNoRepeat: time.Duration(g.Int63n(int64(90*24*time.Hour) + 1)),
			MovieNoRepeat:   time.Duration(g.Int63n(int64(90*24*time.Hour) + 1)),
			SeriesMinGap:    time.Duration(g.Int63n(int64(6 * time.Hour))).Truncate(time.Minute),
			BlockMax:        g.Intn(4), // 0 (unbounded) ..3
		},
	}
	if g.Intn(2) == 0 {
		rp.Scope.Series = []provision.Key{"series:tvdb:90000001", "series:tvdb:90000002"}
	}
	if g.Intn(2) == 0 {
		lo := 1 + g.Intn(5)
		rp.Scope.Seasons = &Range{From: lo, To: lo + g.Intn(4)}
	}
	switch g.Intn(3) {
	case 1:
		from := 1950 + g.Intn(70)
		rp.Scope.Dates = EraDates(Range{From: from, To: from + g.Intn(20)})
	case 2:
		from := 1950 + g.Intn(60)
		rp.Scope.Dates = &DateScope{
			MovieRelease: []Range{{From: from + 1, To: from + 5}},
			SeriesAiring: []Range{{From: from + 2, To: from + 8}},
		}
	}
	return rp
}

// randPool is 1–5 shows of skewed sizes, with sometimes a few films, of mixed runtimes.
func randPool(g *rand.Rand) []Slot {
	var pool []Slot
	for s := range 1 + g.Intn(5) {
		key := fmt.Sprintf("series:tvdb:%d", 90000100+s)
		for e := range 1 + g.Intn(12) {
			pool = append(pool, progSlot(key, fmt.Sprintf("%s-%d", key, e), int64(20+g.Intn(40))*time.Minute.Milliseconds()))
		}
	}
	for f := range g.Intn(3) {
		key := fmt.Sprintf("movie:tmdb:%d", 90000900+f)
		pool = append(pool, progSlot(key, key, int64(80+g.Intn(60))*time.Minute.Milliseconds()))
	}
	return pool
}

func snapshotScope(rp ResolvedPolicy) (rating.Rating, []provision.Key, *Range, *DateScope) {
	var series []provision.Key
	if rp.Scope.Series != nil {
		series = append([]provision.Key(nil), rp.Scope.Series...)
	}
	var seasons *Range
	if rp.Scope.Seasons != nil {
		s := *rp.Scope.Seasons
		seasons = &s
	}
	var dates *DateScope
	if rp.Scope.Dates != nil {
		dates = &DateScope{
			MovieRelease:   append([]Range(nil), rp.Scope.Dates.MovieRelease...),
			SeriesPremiere: append([]Range(nil), rp.Scope.Dates.SeriesPremiere...),
			SeriesAiring:   append([]Range(nil), rp.Scope.Dates.SeriesAiring...),
		}
	}
	return rp.Ceiling, series, seasons, dates
}

func TestRelaxAndSlot_EachGuardRelaxesAloneToTheWidestThatHolds(t *testing.T) {
	g := rand.New(rand.NewSource(0xF00D))
	for trial := range 600 {
		rp := randResolvedPolicy(g)
		pool := randPool(g)
		seed := int64(trial)
		written := slotByPolicy(pool, rp, seed)
		_, applied := relaxAndSlot(pool, rp, seed)

		relaxed := rp
		kinds := map[string]AppliedRelaxation{}
		for _, a := range applied {
			if _, dup := kinds[a.Kind]; dup {
				t.Fatalf("trial %d: %s recorded twice: %+v", trial, a.Kind, applied)
			}
			kinds[a.Kind] = a
		}

		var cycle time.Duration
		for _, s := range pool {
			cycle += time.Duration(s.DurationMs) * time.Millisecond
		}
		for _, kind := range []string{"episodeNoRepeat", "movieNoRepeat"} {
			a, ok := kinds[kind]
			window := rp.Sep.EpisodeNoRepeat
			if kind == "movieNoRepeat" {
				window = rp.Sep.MovieNoRepeat
			}
			if ok != (window > cycle && poolHas(pool, kind == "episodeNoRepeat")) {
				t.Fatalf("trial %d: %s recorded=%v for window %s on a %s cycle", trial, kind, ok, window, cycle)
			}
			if ok && a.To != cycle.Truncate(time.Minute).String() && a.To != cycle.String() {
				t.Fatalf("trial %d: %s relaxed to %s, the cycle is %s", trial, kind, a.To, cycle)
			}
		}

		if a, ok := kinds["seriesMinGap"]; ok {
			if !breaches(written, rp, "gap") {
				t.Fatalf("trial %d: gap relaxed though the written policy held it: %+v", trial, a)
			}
			gap, err := time.ParseDuration(a.To)
			if err != nil || gap >= rp.Sep.SeriesMinGap {
				t.Fatalf("trial %d: gap %s → %s does not relax", trial, a.From, a.To)
			}
			relaxed.Sep.SeriesMinGap = gap
			wider := probing(relaxed, pool) // "holds" means within the probe's search
			wider.Sep.SeriesMinGap = gap + time.Minute
			if wider.Sep.SeriesMinGap < rp.Sep.SeriesMinGap && !breaches(slotByPolicy(pool, wider, seed), wider, "gap") {
				t.Fatalf("trial %d: gap relaxed to %s, but %s holds too", trial, gap, wider.Sep.SeriesMinGap)
			}
		} else if breaches(written, rp, "gap") {
			t.Fatalf("trial %d: the written gap %s is breached and not relaxed", trial, rp.Sep.SeriesMinGap)
		}

		if a, ok := kinds["blockMax"]; ok {
			run := 0
			if a.To != "unbounded" {
				run, _ = strconv.Atoi(a.To)
				if run <= rp.Sep.BlockMax {
					t.Fatalf("trial %d: run cap %s → %s does not relax", trial, a.From, a.To)
				}
				tighter := probing(relaxed, pool)
				tighter.Sep.BlockMax = run - 1
				if !breaches(slotByPolicy(pool, tighter, seed), tighter, "block") {
					t.Fatalf("trial %d: run cap relaxed to %d, but %d holds too", trial, run, run-1)
				}
			}
			relaxed.Sep.BlockMax = run
		}
		if v := checkWrapSeparation(slotByPolicy(pool, relaxed, seed), relaxed); len(v) > 0 {
			t.Fatalf("trial %d: the relaxed policy is still breached: %+v (applied %+v)", trial, v[0], applied)
		}
	}
}

// One long show among few others costs a handful of probes, not one per episode: alone it is
// unbounded at once, and beside two films it runs half its length.
func TestShortestRun_ALongShowSettlesInAFewProbes(t *testing.T) {
	var show []Slot
	for e := range 300 {
		show = append(show, progSlot("series:tvdb:90000001", fmt.Sprint("e", e), (22*time.Minute).Milliseconds()))
	}
	rp := ResolvedPolicy{Ordering: OrderSyndication, Sep: ResolvedSeparation{BlockMax: 2}}
	if run := shortestRun(show, rp, 1); run != 0 {
		t.Fatalf("one show alone ran %d, want unbounded", run)
	}
	withFilms := append(append([]Slot(nil), show...),
		progSlot("movie:tmdb:90000901", "f1", (95*time.Minute).Milliseconds()),
		progSlot("movie:tmdb:90000902", "f2", (95*time.Minute).Milliseconds()))
	if run := shortestRun(withFilms, rp, 1); run != 150 {
		t.Fatalf("300 episodes beside two films ran %d, want 150", run)
	}
}

// The placer can miss a gap and find a wider one, so the gap search does not stop at the first
// failure: four shows of 3, 11, 11 and 10 half-hours hold more than half an hour apart.
func TestWidestGap_LooksPastAGapThePlacerMisses(t *testing.T) {
	var pool []Slot
	for s, n := range []int{3, 11, 11, 10} {
		for e := range n {
			pool = append(pool, progSlot(fmt.Sprintf("series:tvdb:%d", 90000001+s), fmt.Sprint(s, "-", e), (30*time.Minute).Milliseconds()))
		}
	}
	for seed := range int64(20) {
		rp := ResolvedPolicy{Ordering: OrderSyndication, Sep: ResolvedSeparation{BlockMax: 2, SeriesMinGap: 210 * time.Minute}}
		if gap := widestGap(pool, rp, seed); gap < 45*time.Minute {
			t.Fatalf("seed %d: gap relaxed to %s; 45m holds", seed, gap)
		}
	}
}

func poolHas(pool []Slot, episodes bool) bool {
	for _, s := range pool {
		if s.Key.IsSeries() == episodes {
			return true
		}
	}
	return false
}

// TestRelaxAndSlot_NeverRelaxesAudienceOrScope is the most important safety property (§7 "never
// relaxed"): the caller's audience ceiling and scope are byte-identical after relaxation, so
// nothing aliased through the pointers (Dates/Seasons/Series) was mutated in place.
func TestRelaxAndSlot_NeverRelaxesAudienceOrScope(t *testing.T) {
	g := rand.New(rand.NewSource(0x5EED))
	for trial := range 600 {
		rp := randResolvedPolicy(g)
		wantCeil, wantSeries, wantSeasons, wantDates := snapshotScope(rp)
		_, applied := relaxAndSlot(randPool(g), rp, int64(trial))

		gotCeil, gotSeries, gotSeasons, gotDates := snapshotScope(rp)
		if gotCeil != wantCeil || !reflect.DeepEqual(gotSeries, wantSeries) ||
			!reflect.DeepEqual(gotSeasons, wantSeasons) || !reflect.DeepEqual(gotDates, wantDates) {
			t.Fatalf("trial %d: audience or scope mutated", trial)
		}
		for _, a := range applied {
			switch a.Kind {
			case "episodeNoRepeat", "movieNoRepeat", "seriesMinGap", "blockMax":
			default:
				t.Fatalf("trial %d: relaxed %q, which is not a separation guard", trial, a.Kind)
			}
		}
	}
}
