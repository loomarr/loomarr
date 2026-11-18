package pod_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

// biasCatalog is a set of ads of one era and audience with irregular lengths, in the shape of the
// install the epic measured: a few short and long ads among many of ordinary length.
func biasCatalog() []pod.Clip {
	return catalogOf(32, 14, 14, 12, 60, 15, 30, 31, 10, 28, 40, 30, 30, 30, 31, 31, 29, 10, 30)
}

func catalogOf(lengths ...int64) []pod.Clip {
	var out []pod.Clip
	for i, s := range lengths {
		id := fmt.Sprintf("ad%02d", i)
		out = append(out, pod.Clip{Hash: id, Path: id + ".mp4", Role: pod.Commercial, Era: 1992, Audience: pod.General,
			Category: fmt.Sprintf("cat%d", i%7), DurationMs: s * 1000})
	}
	return out
}

// watchedBreaks runs n breaks of one channel the way playout does: every break is sized by the
// history-free pod, assembled from the history of the breaks before it, and its clips are fed back
// as airings.
func watchedBreaks(t *testing.T, catalog []pod.Clip, n int) (map[string]int, int64) {
	t.Helper()
	exposures := &stubExposures{}
	adapter := pod.NewAdapter(stubCatalog{clips: catalog}, exposures, func() pod.Policy {
		return pod.Policy{BreakDurationMs: 120_000, PodMax: 4, Cooldown: 4 * time.Hour}
	}, discardLogger())
	ctx := context.Background()
	length := adapter.PlayableDurationMs(ctx, "ch-1", 7, pod.Selection{})
	counts := map[string]int{}
	start := time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		at := start.Add(time.Duration(i) * 25 * time.Minute)
		pods, err := adapter.PreviewAtMany(ctx, "ch-1", pod.Selection{},
			[]pod.Break{{Seed: int64(1000 + i), Start: at, Length: time.Duration(length) * time.Millisecond}})
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range pods[0].Entries {
			if e.IsFallbackCard {
				continue
			}
			counts[e.Hash]++
			exposures.items = append(exposures.items, aired(e.Hash, at))
		}
	}
	return counts, length
}

// A break's length is a ceiling the least recently aired clips fill, so no clip airs more often
// because of its length. The household's catalog has a few short ads among ordinary ones; with the
// length fixed from the history-free pod, the short ones filled whatever gap the ordinary ones left,
// and aired about twice as often as the rest (the epic's repeat read, #3838).
func TestBreakLengthIsACeilingNotABias(t *testing.T) {
	catalog := biasCatalog()
	counts, _ := watchedBreaks(t, catalog, 400)
	var ids []string
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lo, hi := 1<<30, 0
	for _, c := range catalog {
		n := counts[c.Hash]
		lo, hi = min(lo, n), max(hi, n)
	}
	if lo == 0 || float64(hi) > 1.25*float64(lo) {
		t.Errorf("airings per clip over 400 breaks ranged %d to %d (%v); want within a quarter of each other", lo, hi, counts)
	}
}
