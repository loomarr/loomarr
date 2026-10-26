package filler_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// lengthCatalog is a catalog whose clips differ irregularly in length, so a pod that takes
// different clips cannot top itself up to the same total (evenly spaced lengths can, by chance).
func lengthCatalog() []filler.Clip {
	var cat []filler.Clip
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("c%02d", i)
		cat = append(cat, filler.Clip{
			Hash: id, Path: id + ".mp4", Kind: filler.Commercial,
			DurationMs: int64(15_000 + i*1_733 + (i*i*317)%977), Category: fmt.Sprintf("cat%d", i%4),
		})
	}
	return cat
}

// A break's scheduled length is a property of the catalog and the channel's selection, never of
// what aired (live smoke, 2026-10-24): reconcile sized every break from a pod ranked against the
// live play history, so each commercial that aired re-sized all of the channel's breaks, the whole
// timeline shifted, and the viewer was thrown back into the programme or into a break again.
func TestPlayableDurationIgnoresPlayHistory(t *testing.T) {
	cat := lengthCatalog()
	policy := func() filler.Policy { return filler.Policy{PodMax: 4, Cooldown: 4 * time.Hour} }
	fresh := filler.NewPodAdapter(stubCatalog{clips: cat}, &stubExposures{}, policy, discardLogger())
	want := fresh.PlayableDurationMs(context.Background(), "ch-1", 7, filler.Selection{})
	if want <= 0 {
		t.Fatalf("fresh catalog sized a break at %d ms", want)
	}

	// Every clip the fresh pod drew has just aired, so rotation would now rank all of them behind
	// the rested ones.
	pod, err := fresh.Preview(context.Background(), "ch-1", 7, filler.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	aired := map[string]filler.ExposureRecord{}
	for _, e := range pod.Entries {
		aired[e.Hash] = filler.ExposureRecord{PlayCount: 1, LastPlayedAt: time.Now().Add(-time.Minute)}
	}
	played := filler.NewPodAdapter(stubCatalog{clips: cat}, &stubExposures{items: aired}, policy, discardLogger())
	if got := played.PlayableDurationMs(context.Background(), "ch-1", 7, filler.Selection{}); got != want {
		t.Fatalf("break sized %d ms after its clips aired, %d ms before: airing re-sized the schedule", got, want)
	}
}

// The break that airs is assembled to its own scheduled slot, not to the configured break length:
// a slot cut shorter than the setting must not be overrun (an ad cut off at the slot end), and the
// guide's card for that slot must list only what fits in it.
func TestPreviewAtManyAssemblesToTheSlotLength(t *testing.T) {
	cat := lengthCatalog()
	policy := func() filler.Policy {
		return filler.Policy{PodMax: 4, Cooldown: time.Hour, BreakDurationMs: 120_000}
	}
	adapter := filler.NewPodAdapter(stubCatalog{clips: cat}, &stubExposures{}, policy, discardLogger())
	start := time.Date(2026, 10, 24, 21, 20, 37, 0, time.UTC)
	for _, length := range []time.Duration{45 * time.Second, 70 * time.Second, 113_366 * time.Millisecond} {
		pods, err := adapter.PreviewAtMany(context.Background(), "ch-1", filler.Selection{},
			[]filler.Break{{Seed: 11, Start: start, Length: length}})
		if err != nil {
			t.Fatal(err)
		}
		var total time.Duration
		for _, e := range pods[0].Entries {
			total += time.Duration(e.DurationMs) * time.Millisecond
		}
		if total == 0 || total > length {
			t.Fatalf("%v slot: pod of %v (%d entries), want real clips within the slot", length, total, len(pods[0].Entries))
		}
	}
}
