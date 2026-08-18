package app

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// The Filler page's pool strip (§10 V35) aggregates per-channel coverage. §6 of the build plan
// makes one thing non-negotiable about anything ladder-shaped: it must consume the SAME pools
// assembly does, never a second copy that agrees today. These tests are what keeps that true.

func newPoolAdapter(t *testing.T, clips []filler.Clip, chans []store.Channel) (podPreviewAdapter, store.Store) {
	t.Helper()
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)

	for _, c := range clips {
		if err := st.UpsertClip(ctx, store.Clip{Clip: c, UpdatedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	for _, ch := range chans {
		if _, err := st.SaveChannel(ctx, ch); err != nil {
			t.Fatal(err)
		}
	}
	pods := filler.NewPodAdapter(clipCatalogAdapter{st: st}, nil, slog.New(slog.DiscardHandler))
	return podPreviewAdapter{store: st, pods: pods}, st
}

func liveChannel(id, name string, number, era int) store.Channel {
	return store.Channel{
		Channel: schedule.Channel{ID: id, Name: name, Number: number, Status: schedule.StatusLive},
		Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
			Scope: schedule.ScopePolicy{Era: &schedule.Range{From: era, To: era + 9}},
		}},
	}
}

func commercial(path string, era int, aud filler.Audience) filler.Clip {
	return filler.Clip{
		// Hash AND Path — identity is the hash since V38c, and the store keys on it.
		Hash: path,
		Path: path, Name: path, Kind: filler.Commercial,
		Era: era, Audience: aud, Category: "toys", DurationMs: 30_000,
		TunarrProgramID: "tp-" + path,
	}
}

// THE invariant. The strip and the channel page are the two surfaces an operator compares when
// a channel plays the wrong commercials; if they can disagree, the comparison is worthless and
// the disagreement is unreproducible. They cannot disagree here because the strip's per-channel
// numbers ARE the channel route's — this test fails the moment someone gives the aggregate its
// own opinion about the ladder.
func TestPool_AgreesWithPerChannelCoverage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a, _ := newPoolAdapter(t,
		[]filler.Clip{
			commercial("1992/toys.mp4", 1992, filler.Kids),
			commercial("1992/cereal.mp4", 1992, filler.Kids),
			commercial("1978/cars.mp4", 1978, filler.General),
		},
		[]store.Channel{
			liveChannel("ch-90s", "Saturday Mornings", 42, 1990),
			liveChannel("ch-70s", "Drive-In", 7, 1970),
			liveChannel("ch-quiet", "Test Card", 99, 1930),
		},
	)

	report, err := a.Pool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Channels) != 3 {
		t.Fatalf("Pool listed %d channels, want 3", len(report.Channels))
	}

	for _, got := range report.Channels {
		want, err := a.Coverage(ctx, got.ChannelID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Report.Level != want.Level {
			t.Errorf("%s: pool strip says level %q, the channel page says %q — the two "+
				"surfaces an operator compares now disagree", got.ChannelID, got.Report.Level, want.Level)
		}
		if got.Report.Total != want.Total {
			t.Errorf("%s: pool strip says total %d, the channel page says %d",
				got.ChannelID, got.Report.Total, want.Total)
		}
	}
}

// Worst first, so the strip's diagnosis line can name a channel without the caller sorting.
func TestPool_ListsChannelsWorstFirst(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a, _ := newPoolAdapter(t,
		// Only 1990s kids material exists, so the 90s channel matches exactly and the
		// others fall further down the ladder.
		[]filler.Clip{
			commercial("1992/toys.mp4", 1992, filler.Kids),
			commercial("1994/cereal.mp4", 1994, filler.Kids),
		},
		[]store.Channel{
			liveChannel("ch-90s", "Saturday Mornings", 42, 1990),
			liveChannel("ch-30s", "Newsreel", 3, 1930),
		},
	)

	report, err := a.Pool(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Asserted as a concrete expected ORDER rather than a pairwise comparison. The obvious
	// spelling — "first is not worse than last, unless they are equal" — passes whenever the
	// two channels happen to land on the same rung, which makes it a test that guards nothing
	// on the day someone changes the fixture. These two levels are checked too, so a fixture
	// that stops exercising the sort fails here instead of silently going quiet.
	want := []struct {
		id    string
		level filler.MatchLevel
	}{
		// Nothing from the 1930s exists, so this one falls to any-audience — the worse rung.
		{"ch-30s", filler.MatchAudience},
		// ⚠ **This was `MatchWidened`, and the change IS the V51f fix — visible in a fixture that
		// had encoded the right answer all along.** `liveChannel` builds a scope era of
		// `{From: 1990, To: 1999}`, a real decade. The old code read `.From` and threw the rest
		// away, so the exact rung demanded the literal year 1990, no 90s clip could ever satisfy
		// it, and every 90s channel permanently reported "widened" — a channel telling its
		// operator its ads were only approximately right while sitting on a catalog of exact
		// matches. With both bounds honoured, 1992 and 1994 are inside 1990–1999 and the rung is
		// `exact`. The two channels still land on different rungs, so the worst-first ordering
		// this test exists to check is still exercised.
		{"ch-90s", filler.MatchExact},
	}
	if len(report.Channels) != len(want) {
		t.Fatalf("want %d channels, got %d", len(want), len(report.Channels))
	}
	for i, w := range want {
		got := report.Channels[i]
		if got.ChannelID != w.id {
			t.Errorf("position %d: got %q, want %q — channels are not worst-first", i, got.ChannelID, w.id)
		}
		if got.Report.Level != w.level {
			t.Errorf("%s: level %q, want %q — the fixture no longer exercises the ordering",
				got.ChannelID, got.Report.Level, w.level)
		}
	}
}

// A paused channel is not airing, so reporting it as uncovered would tell the operator to fix
// a problem they created on purpose — and would push a real gap further down the list.
func TestPool_ExcludesPausedAndDetachedChannels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	paused := liveChannel("ch-paused", "Off Air", 8, 1990)
	paused.Status = schedule.StatusPaused
	detached := liveChannel("ch-gone", "Deleted", 9, 1990)
	detached.Status = schedule.StatusDetached

	a, _ := newPoolAdapter(t,
		[]filler.Clip{commercial("1992/toys.mp4", 1992, filler.Kids)},
		[]store.Channel{liveChannel("ch-live", "Live One", 1, 1990), paused, detached},
	)

	report, err := a.Pool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Channels) != 1 {
		t.Fatalf("Pool listed %d channels, want 1 — paused and detached must not count", len(report.Channels))
	}
	if report.Channels[0].ChannelID != "ch-live" {
		t.Errorf("listed %q, want ch-live", report.Channels[0].ChannelID)
	}
}

// The counts come from the catalog and the store, not from the channel walk — an install with
// no channels at all still reports what it holds, which is exactly the fresh-install state the
// strip's "Propose a pull" button exists for.
func TestPool_CountsCatalogWithNoChannels(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a, _ := newPoolAdapter(t,
		[]filler.Clip{
			commercial("1992/toys.mp4", 1992, filler.Kids),
			// Untagged: a commercial with no era/audience/category. The store owns this
			// predicate; the assertion below is what proves Pool asked it rather than
			// counting in Go.
			{Hash: "mystery.mp4", Path: "mystery.mp4", Name: "mystery.mp4", Kind: filler.Commercial, DurationMs: 20_000},
			{Hash: "bump.mp4", Path: "bump.mp4", Name: "bump.mp4", Kind: filler.Bumper, DurationMs: 5_000},
		},
		nil,
	)

	report, err := a.Pool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Channels) != 0 {
		t.Errorf("want no channels, got %d", len(report.Channels))
	}
	if report.Clips != 3 {
		t.Errorf("Clips = %d, want 3", report.Clips)
	}
	if report.Commercials != 2 {
		t.Errorf("Commercials = %d, want 2 — the bumper is not a break body", report.Commercials)
	}
	if report.Untagged != 1 {
		t.Errorf("Untagged = %d, want 1 (from the store's predicate, not a Go copy of it)", report.Untagged)
	}
}

// ⚠ **The report counts the CATALOG, and `Untagged` has to mean the same catalog as `Clips`.**
//
// `ListUntaggedCommercials` passes `IncludeHeld: true` deliberately — held clips are exactly the
// ones the AI tagger must tag. Pool reused that helper for a display statistic and inherited the
// flag invisibly, so an install with one filed clip and twelve held ones rendered
// "CLIPS 1 / 12 clips still need tagging" — a headline contradicted by its own subtext.
//
// Counting them is also unactionable: the strip's advice is to go and tag them, and they are not
// in the Catalog to tag. Incoming owns that queue and carries its own count.
//
// Seen live on the maintainer's install, 2026-08-03.
func TestPool_UntaggedCountsTheCatalogNotTheReviewQueue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a, st := newPoolAdapter(t,
		[]filler.Clip{
			// Filed and untagged: this one IS the operator's work, and must count.
			{Hash: "filed.mp4", Path: "filed.mp4", Name: "filed.mp4", Kind: filler.Commercial, DurationMs: 20_000},
			// Downloaded, untagged, and still awaiting review below.
			{Hash: "held-a.mp4", Path: "held-a.mp4", Name: "held-a.mp4", Kind: filler.Commercial, DurationMs: 20_000},
			{Hash: "held-b.mp4", Path: "held-b.mp4", Name: "held-b.mp4", Kind: filler.Commercial, DurationMs: 20_000},
		},
		nil,
	)
	if _, err := st.SetClipsHeld(ctx, []string{"held-a.mp4", "held-b.mp4"}, true, false, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	report, err := a.Pool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The premise: every other number here already excludes held clips. If this drifts the test
	// below is measuring nothing.
	if report.Clips != 1 {
		t.Fatalf("Clips = %d, want 1 — held clips are not in the catalog", report.Clips)
	}
	if report.Untagged != 1 {
		t.Errorf("Untagged = %d, want 1 — counting the two held clips makes the subtext claim more "+
			"work than the headline has clips, and points at a queue this strip does not show",
			report.Untagged)
	}
}
