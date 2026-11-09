package pod_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

// #1670: on a 1989–99 channel one ad aired twice in an evening 70 minutes apart, because the
// install-wide 30 minute cooldown had long expired. A channel's own commercial spacing must make
// the ladder reach for a widened-era clip before it repeats one inside that spacing.
func spacingAdapter(clips []pod.Clip, records []pod.Airing) *pod.Adapter {
	return pod.NewAdapter(stubCatalog{clips: clips}, &stubExposures{items: records}, func() pod.Policy {
		return pod.Policy{PodMax: 1, BreakDurationMs: 30_000, Cooldown: 30 * time.Minute}
	}, discardLogger())
}

func spacingPod(t *testing.T, adapter *pod.Adapter, sel pod.Selection, at time.Time) pod.Pod {
	t.Helper()
	pods, err := adapter.PreviewAtMany(context.Background(), "ch-1", sel, []pod.Break{{Seed: 7, Start: at}})
	if err != nil {
		t.Fatal(err)
	}
	return pods[0]
}

func TestPodAdapter_ChannelSpacingPrefersWidenedEraOverARepeat(t *testing.T) {
	at := time.Date(2026, 10, 12, 20, 0, 0, 0, time.UTC)
	clips := []pod.Clip{
		{Hash: "exact", Path: "exact.mp4", Role: pod.Commercial, Era: 1994, Audience: pod.General, DurationMs: 30_000},
		{Hash: "widened", Path: "widened.mp4", Role: pod.Commercial, Era: 1984, Audience: pod.General, DurationMs: 30_000},
	}
	adapter := spacingAdapter(clips, []pod.Airing{aired("exact", at.Add(-70*time.Minute))})
	sel := pod.Selection{Era: pod.EraRange{From: 1989, To: 1999}, Audience: pod.General}

	// Inheriting the 30 minute install default, the exact-era clip is rested and leads.
	if got := commercialIDs(spacingPod(t, adapter, sel, at)); !reflect.DeepEqual(got, []string{"exact"}) {
		t.Fatalf("inherited spacing pod = %v, want the rested exact-era clip", got)
	}

	fourHours := 4 * time.Hour
	sel.Spacing = &fourHours
	brk := spacingPod(t, adapter, sel, at)
	if got := commercialIDs(brk); !reflect.DeepEqual(got, []string{"widened"}) {
		t.Fatalf("4h spacing pod = %v, want the widened-era clip ahead of a repeat 70 minutes later", got)
	}
	if brk.CooldownRelaxed {
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
	clips := []pod.Clip{
		{Hash: "only", Path: "only.mp4", Role: pod.Commercial, Era: 1994, Audience: pod.General, DurationMs: 30_000},
	}
	adapter := spacingAdapter(clips, []pod.Airing{aired("only", at.Add(-10*time.Minute))})
	day := 24 * time.Hour
	brk := spacingPod(t, adapter, pod.Selection{Audience: pod.General, Spacing: &day}, at)
	if got := commercialIDs(brk); !reflect.DeepEqual(got, []string{"only"}) || !brk.CooldownRelaxed {
		t.Fatalf("pod = %v relaxed=%v, want the only clip, recorded as a relaxed repeat", got, brk.CooldownRelaxed)
	}
}

// The channel page names the pool's location limit from coverage, never from its own copy.
func TestCoverage_ReportsTheLocationBoundaryItCounted(t *testing.T) {
	clips := []pod.Clip{
		{Hash: "us", Path: "us.mp4", Role: pod.Commercial, GeographicScope: pod.GeographicNational, Country: "US", DurationMs: 30_000},
		{Hash: "gb", Path: "gb.mp4", Role: pod.Commercial, GeographicScope: pod.GeographicNational, Country: "GB", DurationMs: 30_000},
		{Hash: "unknown", Path: "unknown.mp4", Role: pod.Commercial, DurationMs: 30_000},
	}
	w := pod.Window{ChannelID: "ch-1", GapMs: 600_000, PodMax: 4}
	report := pod.Coverage(clips, w, pod.Policy{Geography: pod.Geography{Country: "us"}})
	if report.Geography.Country != "US" || report.Total != 1 {
		t.Fatalf("coverage geography %+v total %d, want US with the one US clip", report.Geography, report.Total)
	}
	if open := pod.Coverage(clips, w, pod.Policy{}); open.Geography.Country != "" || open.Total != 3 {
		t.Fatalf("no installation country: geography %+v total %d, want no limit and all clips", open.Geography, open.Total)
	}
}

// Without bumpers nothing is reserved: the 2 minute default is still four whole 30 second ads.
func TestPodAdapter_DefaultBreakWithoutBumpersIsFourThirtySecondAds(t *testing.T) {
	clips := make([]pod.Clip, 0, 8)
	for i := 0; i < 8; i++ {
		clips = append(clips, pod.Clip{Hash: string(rune('a' + i)), Path: string(rune('a'+i)) + ".mp4",
			Role: pod.Commercial, Era: 1994, Audience: pod.General, DurationMs: 30_000})
	}
	adapter := pod.NewAdapter(stubCatalog{clips: clips}, nil, func() pod.Policy {
		return pod.Policy{PodMax: 4}
	}, discardLogger())
	brk, err := adapter.Preview(context.Background(), "ch-1", 7, pod.Selection{Audience: pod.General})
	if err != nil {
		t.Fatal(err)
	}
	if len(brk.Entries) != 4 || brk.TotalMs != 120_000 {
		t.Fatalf("2m default = %d entries / %dms, want 4 / 120000", len(brk.Entries), brk.TotalMs)
	}
}

// The household default break is 2 minutes, and the slot the scheduler cuts is exactly that long: a
// pod longer than the slot loses its tail, which is the return bumper plus part of the last ad.
// So the intro bumper, the ads and the return bumper must all fit inside the break — the bumpers
// come out of the 2 minutes, they are not added on top of four 30 second ads.
func TestPodAdapter_DefaultBreakFitsBumpersAndWholeAdsInTheSlot(t *testing.T) {
	const slotMs = 2 * 60_000
	clips := make([]pod.Clip, 0, 12)
	for i := 0; i < 8; i++ {
		clips = append(clips, pod.Clip{Hash: string(rune('a' + i)), Path: string(rune('a'+i)) + ".mp4",
			Role: pod.Commercial, Era: 1994, Audience: pod.General, DurationMs: 30_000})
	}
	// Short whole clips to finish the remainder the 30 second ads leave, so no ad is ever cut.
	for i := 0; i < 2; i++ {
		clips = append(clips, pod.Clip{Hash: string(rune('s' + i)), Path: string(rune('s'+i)) + ".mp4",
			Role: pod.Commercial, Era: 1994, Audience: pod.General, DurationMs: 15_000})
	}
	for i := 0; i < 2; i++ {
		clips = append(clips, pod.Clip{Hash: "bumper-" + string(rune('a'+i)), Path: "bumper-" + string(rune('a'+i)) + ".mp4",
			Role: pod.Bumper, Era: 1994, Audience: pod.General, DurationMs: 6_000})
	}
	// The DEFAULT setting: no per-channel or install break duration, so the 2 minute default applies.
	adapter := pod.NewAdapter(stubCatalog{clips: clips}, nil, func() pod.Policy {
		return pod.Policy{PodMax: 4}
	}, discardLogger())
	for seed := int64(1); seed <= 20; seed++ {
		brk, err := adapter.Preview(context.Background(), "ch-1", seed, pod.Selection{Audience: pod.General})
		if err != nil {
			t.Fatal(err)
		}
		if brk.TotalMs > slotMs {
			t.Fatalf("seed %d: pod is %dms, longer than the %dms slot — the tail would be cut", seed, brk.TotalMs, slotMs)
		}
		n := len(brk.Entries)
		if n < 3 || brk.Entries[0].Role != pod.Bumper || brk.Entries[n-1].Role != pod.Bumper {
			t.Fatalf("seed %d: pod %v must open and close with a bumper", seed, brk.Entries)
		}
		if len(commercialIDs(brk)) == 0 {
			t.Fatalf("seed %d: pod has no commercials", seed)
		}
	}
}
