package filler_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

type stubCatalog struct {
	clips []filler.Clip
	err   error
}

func (s stubCatalog) AllClips(context.Context) ([]filler.Clip, error) { return s.clips, s.err }

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// THE invariant behind §12's pod preview: preview must return exactly the pool that
// reconcile attaches. They share one code path by construction (BuildFillerList calls
// Preview), and this test is what keeps that true — if someone re-implements either
// side, or changes the Window/policy in one place only, the ids diverge and this fails.
//
// A preview that could drift from what actually ships is worse than no preview: it
// would confidently show an operator commercials their channel never receives.
func TestPreviewMatchesWhatReconcileAttaches(t *testing.T) {
	adapter := filler.NewPodAdapter(stubCatalog{clips: sampleCatalog()}, nil, discardLogger())
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
	adapter := filler.NewPodAdapter(stubCatalog{clips: sampleCatalog()}, nil, discardLogger())
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
	adapter := filler.NewPodAdapter(stubCatalog{clips: sampleCatalog()}, nil, discardLogger())
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
	adapter := filler.NewPodAdapter(stubCatalog{}, nil, discardLogger())
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

// A catalog READ failure must surface to preview (the operator needs the reason) while
// reconcile degrades to flex — the channel keeps playing (§9 resilience). Same call,
// deliberately different handling at the two call sites.
func TestPreviewSurfacesCatalogErrorWhileReconcileDegrades(t *testing.T) {
	boom := errors.New("store is down")
	adapter := filler.NewPodAdapter(stubCatalog{err: boom}, nil, discardLogger())

	if _, err := adapter.Preview(context.Background(), "ch-1", 1, filler.Selection{}); !errors.Is(err, boom) {
		t.Errorf("preview swallowed the catalog error: %v", err)
	}
	if _, ok := adapter.BuildFillerList(context.Background(), "ch-1", 1, filler.Selection{}); ok {
		t.Error("reconcile should report not-ok on a catalog failure and leave the channel on flex")
	}
}
