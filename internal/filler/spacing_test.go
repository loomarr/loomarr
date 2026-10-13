package filler_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// #1670: on a 1989–99 channel one ad aired twice in an evening 70 minutes apart, because the
// install-wide 30 minute cooldown had long expired. A channel's own commercial spacing must make
// the ladder reach for a widened-era clip before it repeats one inside that spacing.
func spacingAdapter(clips []filler.Clip, records map[string]filler.ExposureRecord) *filler.PodAdapter {
	return filler.NewPodAdapter(stubCatalog{clips: clips}, &stubExposures{items: records}, func() filler.Policy {
		return filler.Policy{PodMax: 1, BreakDurationMs: 30_000, Cooldown: 30 * time.Minute}
	}, discardLogger())
}

func spacingPod(t *testing.T, adapter *filler.PodAdapter, sel filler.Selection, at time.Time) filler.Pod {
	t.Helper()
	pods, err := adapter.PreviewAtMany(context.Background(), "ch-1", sel, []filler.Break{{Seed: 7, Start: at}})
	if err != nil {
		t.Fatal(err)
	}
	return pods[0]
}

func TestPodAdapter_ChannelSpacingPrefersWidenedEraOverARepeat(t *testing.T) {
	at := time.Date(2026, 10, 12, 20, 0, 0, 0, time.UTC)
	clips := []filler.Clip{
		{Hash: "exact", Path: "exact.mp4", Kind: filler.Commercial, Era: 1994, Audience: filler.General, DurationMs: 30_000},
		{Hash: "widened", Path: "widened.mp4", Kind: filler.Commercial, Era: 1984, Audience: filler.General, DurationMs: 30_000},
	}
	adapter := spacingAdapter(clips, map[string]filler.ExposureRecord{
		"exact": {PlayCount: 3, LastPlayedAt: at.Add(-70 * time.Minute)},
	})
	sel := filler.Selection{Era: filler.EraRange{From: 1989, To: 1999}, Audience: filler.General}

	// Inheriting the 30 minute install default, the exact-era clip is rested and leads.
	if got := commercialIDs(spacingPod(t, adapter, sel, at)); !reflect.DeepEqual(got, []string{"exact"}) {
		t.Fatalf("inherited spacing pod = %v, want the rested exact-era clip", got)
	}

	fourHours := 4 * time.Hour
	sel.Spacing = &fourHours
	pod := spacingPod(t, adapter, sel, at)
	if got := commercialIDs(pod); !reflect.DeepEqual(got, []string{"widened"}) {
		t.Fatalf("4h spacing pod = %v, want the widened-era clip ahead of a repeat 70 minutes later", got)
	}
	if pod.CooldownRelaxed {
		t.Fatal("a widened-era clip filled the break, so no spacing was relaxed")
	}

	none := time.Duration(0)
	sel.Spacing = &none
	if got := commercialIDs(spacingPod(t, adapter, sel, at)); !reflect.DeepEqual(got, []string{"exact"}) {
		t.Fatalf("zero spacing pod = %v, want the exact-era clip; 0 lets a clip repeat", got)
	}
}

func TestPodAdapter_ChannelSpacingNeverEmptiesABreak(t *testing.T) {
	at := time.Date(2026, 10, 12, 20, 0, 0, 0, time.UTC)
	clips := []filler.Clip{
		{Hash: "only", Path: "only.mp4", Kind: filler.Commercial, Era: 1994, Audience: filler.General, DurationMs: 30_000},
	}
	adapter := spacingAdapter(clips, map[string]filler.ExposureRecord{
		"only": {PlayCount: 9, LastPlayedAt: at.Add(-10 * time.Minute)},
	})
	day := 24 * time.Hour
	pod := spacingPod(t, adapter, filler.Selection{Audience: filler.General, Spacing: &day}, at)
	if got := commercialIDs(pod); !reflect.DeepEqual(got, []string{"only"}) || !pod.CooldownRelaxed {
		t.Fatalf("pod = %v relaxed=%v, want the only clip, recorded as a relaxed repeat", got, pod.CooldownRelaxed)
	}
}

// The household default is now 2 minutes. With 30 second clips that is four clips, so the default
// pod_max of 4 does not cap the break.
func TestPodAdapter_TwoMinuteDefaultBreakFillsWithThirtySecondClips(t *testing.T) {
	clips := make([]filler.Clip, 0, 8)
	for i := 0; i < 8; i++ {
		clips = append(clips, filler.Clip{Hash: string(rune('a' + i)), Path: string(rune('a'+i)) + ".mp4",
			Kind: filler.Commercial, Era: 1994, Audience: filler.General, DurationMs: 30_000})
	}
	adapter := filler.NewPodAdapter(stubCatalog{clips: clips}, nil, func() filler.Policy {
		return filler.Policy{PodMax: 4, BreakDurationMs: 2 * 60_000}
	}, discardLogger())
	pod, err := adapter.Preview(context.Background(), "ch-1", 7, filler.Selection{Audience: filler.General})
	if err != nil {
		t.Fatal(err)
	}
	if len(pod.Entries) != 4 || pod.TotalMs != 120_000 {
		t.Fatalf("2m default = %d entries / %dms, want 4 / 120000", len(pod.Entries), pod.TotalMs)
	}
}
