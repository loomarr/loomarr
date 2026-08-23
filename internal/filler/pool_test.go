package filler_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

// PoolCounts is the catalog-wide half of the Filler page's pool strip (§10 V35). These tests
// pin the two distinctions the strip exists to make legible, because both are ones an operator
// gets wrong by looking at a raw clip count.

// A catalog can be large and still fill nothing. This is the number the strip leads with.
func TestPoolCounts_EligibleExcludesClipsTooLongForABreak(t *testing.T) {
	catalog := []filler.Clip{
		{Path: "a.mp4", Kind: filler.Commercial, DurationMs: 30_000},
		{Path: "b.mp4", Kind: filler.Commercial, DurationMs: 15_000},
		// A downloaded compilation: one file, twenty adverts, unplaceable until it is split.
		{Path: "comp.mp4", Kind: filler.Commercial, DurationMs: 900_000},
	}
	policy := filler.Policy{MinClipMs: 5_000, MaxClipMs: 120_000}

	got := filler.PoolCounts(catalog, policy)

	if got.Clips != 3 {
		t.Errorf("Clips = %d, want 3", got.Clips)
	}
	if got.Commercials != 3 {
		t.Errorf("Commercials = %d, want 3", got.Commercials)
	}
	if got.Eligible != 2 {
		t.Errorf("Eligible = %d, want 2 — the 15-minute compilation cannot go in a break, "+
			"and counting it promises coverage assembly cannot deliver", got.Eligible)
	}
}

// Bumpers bookend a pod; they cannot BE one. A catalog of nothing but bumpers reads as
// healthy on `Clips` and produces breaks with no adverts in them.
func TestPoolCounts_OnlyCommercialsFillABreakBody(t *testing.T) {
	catalog := []filler.Clip{
		{Path: "bump.mp4", Kind: filler.Bumper, DurationMs: 5_000},
		{Path: "ident.mp4", Kind: filler.StationID, DurationMs: 4_000},
		{Path: "ad.mp4", Kind: filler.Commercial, DurationMs: 30_000},
	}

	got := filler.PoolCounts(catalog, filler.Policy{})

	if got.Clips != 3 {
		t.Errorf("Clips = %d, want 3", got.Clips)
	}
	if got.Commercials != 1 {
		t.Errorf("Commercials = %d, want 1 — bumpers and station IDs are not break bodies", got.Commercials)
	}
	if got.Eligible != 1 {
		t.Errorf("Eligible = %d, want 1", got.Eligible)
	}
}

// Untagged is counted by the STORE, never here — one definition of the word, shared with the
// AI-tagging job that acts on it. If this ever stops being zero, a second definition has been
// introduced in Go and is free to drift from the job.
func TestPoolCounts_LeavesUntaggedToTheStore(t *testing.T) {
	catalog := []filler.Clip{{Path: "a.mp4", Kind: filler.Commercial, DurationMs: 30_000}}

	if got := filler.PoolCounts(catalog, filler.Policy{}); got.Untagged != 0 {
		t.Errorf("Untagged = %d, want 0 — PoolCounts must not define 'untagged'; "+
			"store/clips.go owns that predicate", got.Untagged)
	}
}

// The strip lists channels worst-first so its diagnosis line can name one without sorting.
// "Worst" is the ladder's own order, which is the order an operator would fix them in.
func TestLevelWorseThan_FollowsTheLadder(t *testing.T) {
	ladder := []filler.MatchLevel{
		filler.MatchBumperCard, // nothing matched: the embedded card plays
		filler.MatchAudience,
		filler.MatchWidened,
		filler.MatchExact, // best
	}
	for i := 0; i < len(ladder)-1; i++ {
		worse, better := ladder[i], ladder[i+1]
		if !filler.LevelWorseThan(worse, better) {
			t.Errorf("%q should rank worse than %q", worse, better)
		}
		if filler.LevelWorseThan(better, worse) {
			t.Errorf("%q should NOT rank worse than %q", better, worse)
		}
	}
}

// A rung nobody taught this function about must not sort to the top of the operator's
// to-fix list — an unknown level is not evidence of a problem.
func TestLevelWorseThan_UnknownLevelIsNotTheWorst(t *testing.T) {
	if filler.LevelWorseThan(filler.MatchLevel("some_future_rung"), filler.MatchBumperCard) {
		t.Error("an unrecognised level outranked bumper_card as 'worst' — a future rung would " +
			"silently become the channel the strip tells the operator to fix")
	}
}
