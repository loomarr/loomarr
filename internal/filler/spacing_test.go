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

// The channel page names the pool's location limit from coverage, never from its own copy.
func TestCoverage_ReportsTheLocationBoundaryItCounted(t *testing.T) {
	clips := []filler.Clip{
		{Hash: "us", Path: "us.mp4", Kind: filler.Commercial, GeographicScope: filler.GeographicNational, Country: "US", DurationMs: 30_000},
		{Hash: "gb", Path: "gb.mp4", Kind: filler.Commercial, GeographicScope: filler.GeographicNational, Country: "GB", DurationMs: 30_000},
		{Hash: "unknown", Path: "unknown.mp4", Kind: filler.Commercial, DurationMs: 30_000},
	}
	w := filler.Window{ChannelID: "ch-1", GapMs: 600_000, PodMax: 4}
	report := filler.Coverage(clips, w, filler.Policy{Geography: filler.Geography{Country: "us"}})
	if report.Geography.Country != "US" || report.Total != 1 {
		t.Fatalf("coverage geography %+v total %d, want US with the one US clip", report.Geography, report.Total)
	}
	if open := filler.Coverage(clips, w, filler.Policy{}); open.Geography.Country != "" || open.Total != 3 {
		t.Fatalf("no installation country: geography %+v total %d, want no limit and all clips", open.Geography, open.Total)
	}
}

// Without bumpers nothing is reserved: the 2 minute default is still four whole 30 second ads.
func TestPodAdapter_DefaultBreakWithoutBumpersIsFourThirtySecondAds(t *testing.T) {
	clips := make([]filler.Clip, 0, 8)
	for i := 0; i < 8; i++ {
		clips = append(clips, filler.Clip{Hash: string(rune('a' + i)), Path: string(rune('a'+i)) + ".mp4",
			Kind: filler.Commercial, Era: 1994, Audience: filler.General, DurationMs: 30_000})
	}
	adapter := filler.NewPodAdapter(stubCatalog{clips: clips}, nil, func() filler.Policy {
		return filler.Policy{PodMax: 4}
	}, discardLogger())
	pod, err := adapter.Preview(context.Background(), "ch-1", 7, filler.Selection{Audience: filler.General})
	if err != nil {
		t.Fatal(err)
	}
	if len(pod.Entries) != 4 || pod.TotalMs != 120_000 {
		t.Fatalf("2m default = %d entries / %dms, want 4 / 120000", len(pod.Entries), pod.TotalMs)
	}
}

// The household default break is 2 minutes, and the slot the scheduler cuts is exactly that long: a
// pod longer than the slot loses its tail, which is the return bumper plus part of the last ad.
// So the intro bumper, the ads and the return bumper must all fit inside the break — the bumpers
// come out of the 2 minutes, they are not added on top of four 30 second ads.
func TestPodAdapter_DefaultBreakFitsBumpersAndWholeAdsInTheSlot(t *testing.T) {
	const slotMs = 2 * 60_000
	clips := make([]filler.Clip, 0, 12)
	for i := 0; i < 8; i++ {
		clips = append(clips, filler.Clip{Hash: string(rune('a' + i)), Path: string(rune('a'+i)) + ".mp4",
			Kind: filler.Commercial, Era: 1994, Audience: filler.General, DurationMs: 30_000})
	}
	// Short whole clips to finish the remainder the 30 second ads leave, so no ad is ever cut.
	for i := 0; i < 2; i++ {
		clips = append(clips, filler.Clip{Hash: string(rune('s' + i)), Path: string(rune('s'+i)) + ".mp4",
			Kind: filler.Commercial, Era: 1994, Audience: filler.General, DurationMs: 15_000})
	}
	for i := 0; i < 2; i++ {
		clips = append(clips, filler.Clip{Hash: "bumper-" + string(rune('a'+i)), Path: "bumper-" + string(rune('a'+i)) + ".mp4",
			Kind: filler.Bumper, Era: 1994, Audience: filler.General, DurationMs: 6_000})
	}
	// The DEFAULT setting: no per-channel or install break duration, so the 2 minute default applies.
	adapter := filler.NewPodAdapter(stubCatalog{clips: clips}, nil, func() filler.Policy {
		return filler.Policy{PodMax: 4}
	}, discardLogger())
	for seed := int64(1); seed <= 20; seed++ {
		pod, err := adapter.Preview(context.Background(), "ch-1", seed, filler.Selection{Audience: filler.General})
		if err != nil {
			t.Fatal(err)
		}
		if pod.TotalMs > slotMs {
			t.Fatalf("seed %d: pod is %dms, longer than the %dms slot — the tail would be cut", seed, pod.TotalMs, slotMs)
		}
		n := len(pod.Entries)
		if n < 3 || pod.Entries[0].Kind != filler.Bumper || pod.Entries[n-1].Kind != filler.Bumper {
			t.Fatalf("seed %d: pod %v must open and close with a bumper", seed, pod.Entries)
		}
		if len(commercialIDs(pod)) == 0 {
			t.Fatalf("seed %d: pod has no commercials", seed)
		}
	}
}
