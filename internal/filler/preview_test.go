package filler_test

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

type stubCatalog struct {
	clips []filler.Clip
	err   error
	reads *int // counts AllClips calls when set
}

type stubExposures struct {
	items  []filler.Airing
	settle time.Duration
	reads  int
}

func (s *stubExposures) FillerAirings(context.Context) (filler.AiringHistory, error) {
	s.reads++
	return filler.AiringHistory{Airings: s.items, Settle: s.settle}, nil
}

// aired is a settled airing: recorded well before it aired, as playout's run-ahead records one.
func aired(hash string, at time.Time) filler.Airing {
	return filler.Airing{ClipHash: hash, AiredAt: at, RecordedAt: at.Add(-time.Minute)}
}

func (s stubCatalog) AllClips(context.Context) ([]filler.Clip, error) {
	if s.reads != nil {
		*s.reads++
	}
	return s.clips, s.err
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestPreviewAtUsesBreakScopedExposureSnapshot(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 123_000_000, time.UTC)
	cat := []filler.Clip{
		{Hash: "recent", Path: "recent.mp4", Kind: filler.Commercial, DurationMs: 30_000, Category: "one"},
		{Hash: "new", Path: "new.mp4", Kind: filler.Commercial, DurationMs: 30_000, Category: "two"},
	}
	// "new" first aired AT this break's start: that play belongs to the break itself, so the
	// snapshot must not see it, or the clip would reshuffle the break it is airing in.
	history := &stubExposures{items: []filler.Airing{aired("recent", start.Add(-time.Minute)), aired("new", start)}}
	adapter := filler.NewPodAdapter(stubCatalog{clips: cat}, history, func() filler.Policy {
		return filler.Policy{PodMax: 2, Cooldown: time.Hour}
	}, discardLogger())
	pod, err := adapter.PreviewAt(context.Background(), "channel-7", 42, filler.Selection{}, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(pod.Entries) < 2 || pod.Entries[0].Hash != "new" || pod.Entries[1].Hash != "recent" {
		t.Fatalf("rotation entries = %+v, want new before recent", pod.Entries)
	}
}

// #1420: a window of breaks reads the catalog and the play history ONCE, and every pod is the
// one PreviewAt assembles for that break alone — same seed, same snapshot cut.
func TestPreviewAtManyReadsOnceAndMatchesPreviewAt(t *testing.T) {
	start := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	var cat []filler.Clip
	for i := 0; i < 8; i++ {
		id := string(rune('a' + i))
		cat = append(cat, filler.Clip{Hash: id, Path: id + ".mp4", Kind: filler.Commercial, DurationMs: 30_000})
	}
	// "a" aired in the second break: breaks before it must not see that play, breaks after must.
	records := []filler.Airing{
		aired("a", start.Add(30*time.Minute)), aired("a", start.Add(-3*time.Hour)), aired("b", start.Add(-10*time.Minute)),
	}
	policy := func() filler.Policy { return filler.Policy{PodMax: 3, Cooldown: time.Hour} }
	reads := 0
	history := &stubExposures{items: records}
	batch := filler.NewPodAdapter(stubCatalog{clips: cat, reads: &reads}, history, policy, discardLogger())

	var breaks []filler.Break
	for i := 0; i < 6; i++ {
		breaks = append(breaks, filler.Break{Seed: int64(100 + i), Start: start.Add(time.Duration(i) * 30 * time.Minute)})
	}
	pods, err := batch.PreviewAtMany(context.Background(), "channel-7", filler.Selection{}, breaks)
	if err != nil {
		t.Fatal(err)
	}
	if reads != 1 || history.reads != 1 {
		t.Fatalf("catalog reads = %d, history reads = %d for %d breaks, want one each", reads, history.reads, len(breaks))
	}
	single := filler.NewPodAdapter(stubCatalog{clips: cat}, &stubExposures{items: records}, policy, discardLogger())
	for i, b := range breaks {
		want, err := single.PreviewAt(context.Background(), "channel-7", b.Seed, filler.Selection{}, b.Start)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(pods[i], want) {
			t.Fatalf("break %d: batch pod %+v, PreviewAt pod %+v", i, pods[i], want)
		}
	}
}

// The cut itself: what aired strictly before the break, and only what was recorded a settle before
// it. A tune-in on another channel records a clip that aired minutes ago at the moment of the
// tune-in; once the break is on air, that airing must not join its snapshot and reshuffle it.
func TestExposuresBeforeCutsAtTheBreakStartAndTheSettle(t *testing.T) {
	at := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	const settle = 15 * time.Second
	h := filler.AiringHistory{Settle: settle, Airings: []filler.Airing{
		aired("twice", at), aired("twice", at.Add(-time.Hour)), // the break's own airing, and one before
		aired("once", at.Add(time.Minute)), // after the break
		aired("old", at.Add(-time.Minute)), aired("old", at.Add(-2*time.Hour)),
		// Aired before the break, recorded by a tune-in on another channel as the break airs.
		{ClipHash: "late", AiredAt: at.Add(-3 * time.Minute), RecordedAt: at.Add(20 * time.Second)},
		// Recorded inside the settle: possibly after the break's first clip was resolved.
		{ClipHash: "unsettled", AiredAt: at.Add(-time.Second), RecordedAt: at.Add(-settle + time.Millisecond)},
		{ClipHash: "settled", AiredAt: at.Add(-time.Second), RecordedAt: at.Add(-settle - time.Millisecond)},
	}}
	got := filler.ExposuresBefore(h, at)
	want := map[string]filler.Exposure{
		"twice":   {PlayCount: 1, LastPlayedAt: at.Add(-time.Hour)},
		"old":     {PlayCount: 2, LastPlayedAt: at.Add(-time.Minute)},
		"settled": {PlayCount: 1, LastPlayedAt: at.Add(-time.Second)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %+v, want %+v", got, want)
	}
	if all := filler.ExposuresBefore(h, time.Time{}); len(all) != 6 || all["twice"].PlayCount != 2 {
		t.Fatalf("zero cutoff = %+v, want all history", all)
	}
}

// THE invariant behind §12's pod preview: preview must return exactly the pool that
// reconcile attaches. They share one code path by construction (BuildFillerList calls
// Preview), and this test is what keeps that true — if someone re-implements either
// side, or changes the Window/policy in one place only, the ids diverge and this fails.
//
// A preview that could drift from what actually ships is worse than no preview: it
// would confidently show an operator commercials their channel never receives.
func TestPreviewMatchesWhatReconcileAttaches(t *testing.T) {
	adapter := filler.NewPodAdapter(stubCatalog{clips: sampleCatalog()}, nil, nil, discardLogger())
	ctx := context.Background()
	const channelID, era = "ch-1", 1992
	const seed int64 = 424242

	pod, err := adapter.Preview(ctx, channelID, seed, filler.Selection{Era: filler.Year(era)})
	if err != nil {
		t.Fatal(err)
	}
	attached, ok := adapter.BuildFillerList(ctx, channelID, seed, filler.Selection{Era: filler.Year(era)})
	if !ok {
		t.Fatal("BuildFillerList returned not-ok for a catalog that previewed fine")
	}

	// The preview carries the embedded fallback card (which has no Tunarr id and is
	// therefore never attached), so compare on the real program ids only.
	// TunarrProgramID on BOTH sides, deliberately: this test is about the TUNARR filler-list,
	// so the comparison must be in Tunarr's namespace. Since §9.1 a clip has two ids — Path
	// (identity, what internal playout hands ffmpeg) and TunarrProgramID (what a filler-list
	// references) — and comparing one against the other would fail on correct code.
	var previewed []string
	for _, e := range pod.Entries {
		if e.TunarrProgramID != "" {
			previewed = append(previewed, e.TunarrProgramID)
		}
	}
	if len(previewed) != len(attached) {
		t.Fatalf("preview has %d real clips, reconcile attaches %d", len(previewed), len(attached))
	}
	for i := range previewed {
		if previewed[i] != attached[i] {
			t.Errorf("clip %d: preview %q, reconcile attaches %q — preview lies about what ships",
				i, previewed[i], attached[i])
		}
	}
}

// REGRESSION (found by building the §12 preview): a channel's filler-list must contain
// actual COMMERCIALS, not just bumpers.
//
// BuildFillerList used to pass Audience: General, with a comment claiming it "matches
// broadly". filterAudience keeps clips where `c.Audience == aud || c.Audience == General`
// — so General matches ONLY general-tagged clips. Every kids/family/late_night
// commercial, and every untagged one, was dropped from every channel, leaving pods of
// bumpers and the fallback card. Commercials being "core to the feels-like-real-TV goal,
// not a garnish" (§10), that was the feature silently not working.
//
// The sample catalog's commercials are all audience=kids, which is exactly the case that
// used to yield nothing.
func TestFillerListContainsCommercialsNotJustBumpers(t *testing.T) {
	adapter := filler.NewPodAdapter(stubCatalog{clips: sampleCatalog()}, nil, nil, discardLogger())
	ids, ok := adapter.BuildFillerList(context.Background(), "ch-1", 42, filler.Selection{Era: filler.Year(1992)})
	if !ok {
		t.Fatal("no filler list built from a catalog full of era-matching commercials")
	}

	// Keyed by TUNARR id: `ids` comes from BuildFillerList, which speaks Tunarr's namespace.
	byID := map[string]filler.Clip{}
	for _, c := range sampleCatalog() {
		byID[c.TunarrProgramID] = c
	}
	var commercials int
	for _, id := range ids {
		if byID[id].Kind == filler.Commercial {
			commercials++
		}
	}
	if commercials == 0 {
		t.Fatalf("filler list has no commercials, only %v — the channel would play bumpers into every break", ids)
	}
}

// Seeded determinism is what makes the comparison above meaningful: same channel + same
// seed must preview identically on every call, or "what you see is what you get" holds
// only until the next refresh (§10 seeded-deterministic, §19).
func TestPreviewIsSeedDeterministic(t *testing.T) {
	adapter := filler.NewPodAdapter(stubCatalog{clips: sampleCatalog()}, nil, nil, discardLogger())
	ctx := context.Background()

	first, err := adapter.Preview(ctx, "ch-1", 99, filler.Selection{Era: filler.Year(1992)})
	if err != nil {
		t.Fatal(err)
	}
	second, err := adapter.Preview(ctx, "ch-1", 99, filler.Selection{Era: filler.Year(1992)})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != len(second.Entries) {
		t.Fatalf("same seed produced %d then %d entries", len(first.Entries), len(second.Entries))
	}
	for i := range first.Entries {
		if first.Entries[i].Path != second.Entries[i].Path {
			t.Errorf("entry %d differs across identical previews: %q vs %q",
				i, first.Entries[i].Path, second.Entries[i].Path)
		}
	}
}

// An empty catalog is a normal state the UI renders as "no clips yet" — not an error,
// and not a reason for the channel to fail. Reconcile treats it as "attach nothing".
func TestPreviewEmptyCatalogIsNotAnError(t *testing.T) {
	adapter := filler.NewPodAdapter(stubCatalog{}, nil, nil, discardLogger())
	pod, err := adapter.Preview(context.Background(), "ch-1", 1, filler.Selection{})
	if err != nil {
		t.Fatalf("empty catalog returned an error: %v", err)
	}
	if len(pod.Entries) != 0 {
		t.Errorf("empty catalog produced %d entries", len(pod.Entries))
	}
	if _, ok := adapter.BuildFillerList(context.Background(), "ch-1", 1, filler.Selection{}); ok {
		t.Error("empty catalog should mean nothing to attach")
	}
}

// Internal playout owns local file paths and never needs a Tunarr program uuid. This is the
// exact media-server-only shape that decides whether reconcile may materialize break gaps.
func TestHasPool_LocalClipNeedsNoTunarrProgramID(t *testing.T) {
	adapter := filler.NewPodAdapter(stubCatalog{clips: []filler.Clip{{
		Hash: "local", Path: "commercials/local.mp4", Name: "Local ad",
		Kind: filler.Commercial, DurationMs: 30_000,
	}}}, nil, nil, discardLogger())

	if !adapter.HasPool(context.Background(), "internal", 42, filler.Selection{}) {
		t.Fatal("local playable clip was treated as no filler pool without a Tunarr uuid")
	}
	if ids, ok := adapter.BuildFillerList(context.Background(), "internal", 42, filler.Selection{}); ok || len(ids) != 0 {
		t.Fatalf("Tunarr filler list = %v, %v; local-only clip must not fabricate a remote uuid", ids, ok)
	}
}

// A catalog READ failure must surface to preview (the operator needs the reason) while
// reconcile degrades to flex — the channel keeps playing (§9 resilience). Same call,
// deliberately different handling at the two call sites.
func TestPreviewSurfacesCatalogErrorWhileReconcileDegrades(t *testing.T) {
	boom := errors.New("store is down")
	adapter := filler.NewPodAdapter(stubCatalog{err: boom}, nil, nil, discardLogger())

	if _, err := adapter.Preview(context.Background(), "ch-1", 1, filler.Selection{}); !errors.Is(err, boom) {
		t.Errorf("preview swallowed the catalog error: %v", err)
	}
	if _, ok := adapter.BuildFillerList(context.Background(), "ch-1", 1, filler.Selection{}); ok {
		t.Error("reconcile should report not-ok on a catalog failure and leave the channel on flex")
	}
}
