package filler_test

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// ⚠ **The setting→policy edge, which no other test in this package touches.**
//
// Every other filler test builds a `filler.Policy` literal and calls `Assemble`/`Coverage`/
// `PoolCounts` directly — so they all exercise the FIELD and none of them exercise how a value
// gets into it. `app.go` used to build the struct once at boot, which made every filler setting
// silently restart-only: the write landed, the API read it back, and the assembler kept the old
// number. Found on the live stack (`filler.max_clip_duration=45s` left `PoolReport.Eligible` at
// 20 with a 64s clip in the catalog; it dropped to 19 only after a re-exec).
//
// The property is therefore about the RESOLVER, not about any one setting: the adapter must ask
// again on every call, so whatever the boundary reads from settings is what assembly uses.
func TestPodAdapter_ResolvesPolicyPerCall(t *testing.T) {
	catalog := []filler.Clip{
		{Hash: "short", Path: "short", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, DurationMs: 30_000},
		{Hash: "long", Path: "long", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, DurationMs: 64_000},
	}

	// The live policy, as a boundary that reads settings would supply it.
	maxClipMs := int64(0) // 0 = off, the default
	calls := 0
	adapter := filler.NewPodAdapter(stubCatalog{clips: catalog}, func() filler.Policy {
		calls++
		return filler.Policy{MaxClipMs: maxClipMs}
	}, discardLogger())

	ctx := context.Background()

	before, err := adapter.PoolCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.Eligible != 2 {
		t.Fatalf("eligible = %d, want 2 with no ceiling set", before.Eligible)
	}

	// The operator writes the setting. Nothing is restarted, nothing is rebuilt.
	maxClipMs = 45_000

	after, err := adapter.PoolCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Eligible != 1 {
		t.Errorf("eligible = %d, want 1 — a 45s ceiling must exclude the 64s clip WITHOUT a restart", after.Eligible)
	}
	if after.Commercials != 2 {
		t.Errorf("commercials = %d, want 2 — the ceiling is an eligibility filter, not a reject", after.Commercials)
	}

	// ⚠ Asserted explicitly: a resolver that were called once and cached would still satisfy the
	// counts above if the cache happened to refresh, and this states the actual contract.
	if calls < 2 {
		t.Errorf("policy resolver called %d times across two assemblies — it must be asked again per call", calls)
	}
}

// ⚠ A nil resolver is the zero policy, not a panic. Most callers — every test above, and any
// install with nothing configured — pass nil, and a constructor that required a closure for
// "no policy" would make the common case the noisy one.
func TestPodAdapter_NilPolicyResolverIsTheZeroPolicy(t *testing.T) {
	adapter := filler.NewPodAdapter(stubCatalog{clips: sampleCatalog()}, nil, discardLogger())

	report, err := adapter.PoolCounts(context.Background())
	if err != nil {
		t.Fatalf("a nil policy resolver must not fail: %v", err)
	}
	if report.Eligible != report.Commercials {
		t.Errorf("eligible=%d commercials=%d — with no policy every commercial is eligible",
			report.Eligible, report.Commercials)
	}
}

func TestPodAdapter_BreakDurationRaisesSoftPodLimit(t *testing.T) {
	clips := make([]filler.Clip, 0, 12)
	for i := 0; i < 12; i++ {
		clips = append(clips, filler.Clip{
			Hash: string(rune('a' + i)), Path: string(rune('a'+i)) + ".mp4",
			Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, DurationMs: 30_000,
		})
	}
	adapter := filler.NewPodAdapter(stubCatalog{clips: clips}, func() filler.Policy {
		return filler.Policy{PodMax: 4, BreakDurationMs: 5 * 60_000}
	}, discardLogger())

	pod, err := adapter.Preview(context.Background(), "ch-1", 7, filler.Selection{
		Era: filler.Year(1992), Audience: filler.Kids,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pod.Entries) != 10 || pod.TotalMs != 300_000 {
		t.Fatalf("5m target with 30s clips = %d entries / %dms, want 10 / 300000; pod_max must be soft", len(pod.Entries), pod.TotalMs)
	}
}

func TestPodAdapter_ChannelBreakDurationOverridesGlobal(t *testing.T) {
	clips := make([]filler.Clip, 0, 12)
	for i := 0; i < 12; i++ {
		clips = append(clips, filler.Clip{
			Hash: string(rune('a' + i)), Path: string(rune('a'+i)) + ".mp4",
			Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, DurationMs: 30_000,
		})
	}
	adapter := filler.NewPodAdapter(stubCatalog{clips: clips}, func() filler.Policy {
		return filler.Policy{PodMax: 4, BreakDurationMs: 5 * 60_000}
	}, discardLogger())

	pod, err := adapter.Preview(context.Background(), "ch-1", 7, filler.Selection{
		Era: filler.Year(1992), Audience: filler.Kids, BreakDurationMs: 2 * 60_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pod.Entries) != 4 || pod.TotalMs != 120_000 {
		t.Fatalf("2m channel override = %d entries / %dms, want 4 / 120000", len(pod.Entries), pod.TotalMs)
	}
}
