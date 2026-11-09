package pod_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

// lengthCatalog is a catalog whose clips differ irregularly in length, so a pod that takes
// different clips cannot top itself up to the same total (evenly spaced lengths can, by chance).
func lengthCatalog() []pod.Clip {
	var cat []pod.Clip
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("c%02d", i)
		cat = append(cat, pod.Clip{
			Hash: id, Path: id + ".mp4", Role: pod.Commercial,
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
	policy := func() pod.Policy { return pod.Policy{PodMax: 4, Cooldown: 4 * time.Hour} }
	fresh := pod.NewAdapter(stubCatalog{clips: cat}, &stubExposures{}, policy, discardLogger())
	want := fresh.PlayableDurationMs(context.Background(), "ch-1", 7, pod.Selection{})
	if want <= 0 {
		t.Fatalf("fresh catalog sized a break at %d ms", want)
	}

	// Every clip the fresh brk drew has just aired, so rotation would now rank all of them behind
	// the rested ones.
	brk, err := fresh.Preview(context.Background(), "ch-1", 7, pod.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	var history []pod.Airing
	for _, e := range brk.Entries {
		history = append(history, aired(e.Hash, time.Now().Add(-time.Minute)))
	}
	played := pod.NewAdapter(stubCatalog{clips: cat}, &stubExposures{items: history}, policy, discardLogger())
	if got := played.PlayableDurationMs(context.Background(), "ch-1", 7, pod.Selection{}); got != want {
		t.Fatalf("break sized %d ms after its clips aired, %d ms before: airing re-sized the schedule", got, want)
	}
}

// The break that airs is assembled to its own scheduled slot, not to the configured break length:
// a slot cut shorter than the setting must not be overrun (an ad cut off at the slot end), and the
// guide's card for that slot must list only what fits in it.
func TestPreviewAtManyAssemblesToTheSlotLength(t *testing.T) {
	cat := lengthCatalog()
	policy := func() pod.Policy {
		return pod.Policy{PodMax: 4, Cooldown: time.Hour, BreakDurationMs: 120_000}
	}
	adapter := pod.NewAdapter(stubCatalog{clips: cat}, &stubExposures{}, policy, discardLogger())
	start := time.Date(2026, 10, 24, 21, 20, 37, 0, time.UTC)
	for _, length := range []time.Duration{45 * time.Second, 70 * time.Second, 113_366 * time.Millisecond} {
		pods, err := adapter.PreviewAtMany(context.Background(), "ch-1", pod.Selection{},
			[]pod.Break{{Seed: 11, Start: start, Length: length}})
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
