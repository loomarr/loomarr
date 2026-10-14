package filler_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

// This sweep pins what a household member notices about a break: it uses its time. It crosses
// break length × bumper set × pool size × clip mix × seed and asserts, for every pod:
//
//   - NEVER OVER THE SLOT — the scheduler cuts a slot of exactly the break length, so a pod one
//     millisecond longer loses its tail.
//   - NEVER THE CARD WHEN AN AD FITS — bumpers are reserved before any ad is placed, so bumpers
//     that left less room than the shortest ad used to turn the whole break into the 5 s card
//     (and, with no playable file, switch the channel's breaks off).
//   - NO TAIL A CLIP COULD FILL — the time left after the last clip is under what the smallest
//     unused whole clip needs; a 2 minute break of 30 s ads and 6 s bumpers used to end 18 s early.

var (
	sweepBreaksMs = []int64{30_000, 60_000, 120_000, 180_000}
	sweepPoolSize = []int{0, 1, 3, 8, 20}
	// Bookend sets: each entry is the list of bookend durations in the catalog.
	sweepBookends = map[string][]int64{
		"none":      nil,
		"two-5s":    {5_000, 5_000},
		"two-6s":    {6_000, 6_000},
		"two-12s":   {12_000, 12_000},
		"one-25s":   {25_000},
		"three-20s": {20_000, 20_000, 20_000},
		"ident-mix": {4_000, 6_000, 9_000, 15_000},
	}
	sweepMixes = map[string][]int64{
		"all-30s": {30_000},
		"all-45s": {45_000},
		"all-60s": {60_000},
		"short":   {5_000, 10_000},
		"mixed":   {10_000, 15_000, 20_000, 30_000, 45_000},
	}
)

func sweepCatalog(mix []int64, pool int, bookends []int64) []filler.Clip {
	var cat []filler.Clip
	for i := 0; i < pool; i++ {
		cat = append(cat, filler.Clip{
			Hash: "ad" + strconv.Itoa(i), Path: "ad" + strconv.Itoa(i), Name: "ad" + strconv.Itoa(i),
			Kind: filler.Commercial, Era: 1992, Audience: filler.General,
			// Four categories so category variety binds without being impossible.
			Category:   []string{"cars", "food", "toys", "tech"}[i%4],
			DurationMs: mix[i%len(mix)],
		})
	}
	for i, d := range bookends {
		kind := filler.Bumper
		if i%2 == 1 {
			kind = filler.StationID
		}
		cat = append(cat, filler.Clip{
			Hash: "bk" + strconv.Itoa(i), Path: "bk" + strconv.Itoa(i), Name: "bk" + strconv.Itoa(i),
			Kind: kind, Era: 1992, Audience: filler.General, DurationMs: d,
		})
	}
	return cat
}

// sweepUnused is the catalog clips a break could still have used: nothing in the pod, whole.
func sweepUnused(cat []filler.Clip, pod filler.Pod) []filler.Clip {
	in := map[string]bool{}
	for _, e := range pod.Entries {
		in[e.Hash] = true
	}
	var out []filler.Clip
	for _, c := range cat {
		if !in[c.Hash] {
			out = append(out, c)
		}
	}
	return out
}

func sweepCheck(t *testing.T, label string, cat []filler.Clip, gapMs int64, pod filler.Pod) {
	t.Helper()
	var sum int64
	ads := 0
	for _, e := range pod.Entries {
		sum += e.DurationMs
		if e.Kind == filler.Commercial {
			ads++
		}
	}
	if sum != pod.TotalMs {
		t.Errorf("%s: TotalMs %d != sum of entries %d", label, pod.TotalMs, sum)
	}
	if pod.TotalMs > gapMs {
		t.Errorf("%s: pod %dms is over the %dms slot", label, pod.TotalMs, gapMs)
	}

	adFits := false
	for _, c := range cat {
		if c.Kind == filler.Commercial && c.DurationMs <= gapMs {
			adFits = true
		}
	}
	if adFits && ads == 0 {
		t.Errorf("%s: an ad fits the %dms slot but the pod has none: %v", label, gapMs, podShape(pod))
	}
	if !adFits || ads == 0 {
		return // the card (or nothing to air) is the only honest answer; no tail to judge
	}

	tail := gapMs - pod.TotalMs
	for _, c := range sweepUnused(cat, pod) {
		if c.DurationMs <= tail {
			t.Errorf("%s: %dms tail left while unused %q (%dms) fits: %v", label, tail, c.Name, c.DurationMs, podShape(pod))
			return
		}
	}
}

func podShape(p filler.Pod) string {
	s := ""
	for _, e := range p.Entries {
		k := "ad"
		switch {
		case e.IsFallbackCard:
			k = "card"
		case e.Kind == filler.Bumper:
			k = "bumper"
		case e.Kind == filler.StationID:
			k = "ident"
		}
		s += fmt.Sprintf("%s:%ds ", k, e.DurationMs/1000)
	}
	return fmt.Sprintf("[%s= %ds]", s, p.TotalMs/1000)
}

func TestSweep_BreaksFillTheirTime(t *testing.T) {
	for _, gap := range sweepBreaksMs {
		for bkName, bookends := range sweepBookends {
			for mixName, mix := range sweepMixes {
				for _, pool := range sweepPoolSize {
					cat := sweepCatalog(mix, pool, bookends)
					for seed := int64(0); seed < 12; seed++ {
						w := filler.Window{
							ChannelID: "ch", Seed: seed, Era: filler.Year(1992), Audience: filler.General,
							// Count is not the limit under test: the adapter sizes it from time.
							GapMs: gap, PodMax: 64,
						}
						pod := filler.Assemble(cat, w, filler.Policy{}, map[string]bool{})
						label := fmt.Sprintf("gap=%ds bookends=%s mix=%s pool=%d seed=%d", gap/1000, bkName, mixName, pool, seed)
						sweepCheck(t, label, cat, gap, pod)
					}
				}
			}
		}
	}
}

// The adapter path: the configured clips-per-break (4) must never be the reason a fitting clip is
// left out of a break that has time for it.
func TestSweep_AdapterCountIsNotTheLimit(t *testing.T) {
	for _, gap := range []int64{60_000, 120_000, 180_000} {
		for mixName, mix := range sweepMixes {
			for _, pool := range []int{3, 8, 20} {
				cat := sweepCatalog(mix, pool, []int64{6_000, 6_000, 9_000})
				adapter := filler.NewPodAdapter(stubCatalog{clips: cat}, nil, func() filler.Policy {
					return filler.Policy{PodMax: 4, BreakDurationMs: gap}
				}, discardLogger())
				for seed := int64(0); seed < 8; seed++ {
					pod, err := adapter.Preview(context.Background(), "ch", seed, filler.Selection{Audience: filler.General, Era: filler.Year(1992)})
					if err != nil {
						t.Fatal(err)
					}
					label := fmt.Sprintf("adapter gap=%ds mix=%s pool=%d seed=%d", gap/1000, mixName, pool, seed)
					sweepCheck(t, label, cat, gap, pod)
				}
			}
		}
	}
}
