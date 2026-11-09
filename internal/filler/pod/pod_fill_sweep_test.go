package pod_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/pod"
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

func sweepCatalog(mix []int64, pool int, bookends []int64) []pod.Clip {
	var cat []pod.Clip
	for i := 0; i < pool; i++ {
		cat = append(cat, pod.Clip{
			Hash: "ad" + strconv.Itoa(i), Path: "ad" + strconv.Itoa(i), Name: "ad" + strconv.Itoa(i),
			Role: pod.Commercial, Era: 1992, Audience: pod.General,
			// Four categories so category variety binds without being impossible.
			Category:   []string{"cars", "food", "toys", "tech"}[i%4],
			DurationMs: mix[i%len(mix)],
		})
	}
	for i, d := range bookends {
		kind := pod.Bumper
		if i%2 == 1 {
			kind = pod.StationID
		}
		cat = append(cat, pod.Clip{
			Hash: "bk" + strconv.Itoa(i), Path: "bk" + strconv.Itoa(i), Name: "bk" + strconv.Itoa(i),
			Role: kind, Era: 1992, Audience: pod.General, DurationMs: d,
		})
	}
	return cat
}

// sweepUnused is the catalog clips a break could still have used: nothing in the pod, whole.
func sweepUnused(cat []pod.Clip, brk pod.Pod) []pod.Clip {
	in := map[string]bool{}
	for _, e := range brk.Entries {
		in[e.Hash] = true
	}
	var out []pod.Clip
	for _, c := range cat {
		if !in[c.Hash] {
			out = append(out, c)
		}
	}
	return out
}

func sweepCheck(t *testing.T, label string, cat []pod.Clip, gapMs int64, brk pod.Pod) {
	t.Helper()
	var sum int64
	ads := 0
	for _, e := range brk.Entries {
		sum += e.DurationMs
		if e.Role == pod.Commercial {
			ads++
		}
	}
	if sum != brk.TotalMs {
		t.Errorf("%s: TotalMs %d != sum of entries %d", label, brk.TotalMs, sum)
	}
	if brk.TotalMs > gapMs {
		t.Errorf("%s: pod %dms is over the %dms slot", label, brk.TotalMs, gapMs)
	}

	adFits := false
	for _, c := range cat {
		if c.Role == pod.Commercial && c.DurationMs <= gapMs {
			adFits = true
		}
	}
	if adFits && ads == 0 {
		t.Errorf("%s: an ad fits the %dms slot but the pod has none: %v", label, gapMs, podShape(brk))
	}
	if !adFits || ads == 0 {
		return // the card (or nothing to air) is the only honest answer; no tail to judge
	}

	tail := gapMs - brk.TotalMs
	for _, c := range sweepUnused(cat, brk) {
		if c.DurationMs <= tail {
			t.Errorf("%s: %dms tail left while unused %q (%dms) fits: %v", label, tail, c.Name, c.DurationMs, podShape(brk))
			return
		}
	}
}

func podShape(p pod.Pod) string {
	s := ""
	for _, e := range p.Entries {
		k := "ad"
		switch {
		case e.IsFallbackCard:
			k = "card"
		case e.Role == pod.Bumper:
			k = "bumper"
		case e.Role == pod.StationID:
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
						w := pod.Window{
							ChannelID: "ch", Seed: seed, Era: pod.Year(1992), Audience: pod.General,
							// Count is not the limit under test: the adapter sizes it from time.
							GapMs: gap, PodMax: 64,
						}
						brk := pod.Assemble(cat, w, pod.Policy{}, map[string]bool{})
						label := fmt.Sprintf("gap=%ds bookends=%s mix=%s pool=%d seed=%d", gap/1000, bkName, mixName, pool, seed)
						sweepCheck(t, label, cat, gap, brk)
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
				adapter := pod.NewAdapter(stubCatalog{clips: cat}, nil, func() pod.Policy {
					return pod.Policy{PodMax: 4, BreakDurationMs: gap}
				}, discardLogger())
				for seed := int64(0); seed < 8; seed++ {
					brk, err := adapter.Preview(context.Background(), "ch", seed, pod.Selection{Audience: pod.General, Era: pod.Year(1992)})
					if err != nil {
						t.Fatal(err)
					}
					label := fmt.Sprintf("adapter gap=%ds mix=%s pool=%d seed=%d", gap/1000, mixName, pool, seed)
					sweepCheck(t, label, cat, gap, brk)
				}
			}
		}
	}
}
