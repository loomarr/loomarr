package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/taxonomy"
)

// Filler (§10): clips, their lifecycle, the source registry, pulls and split proposals.
//
// ⚠ These were spread across FOUR non-contiguous regions of the old single file — clip tests
// at four separate offsets with sources, pulls and splits interleaved between them. The domain
// was always coherent; only the file ordering (historical, by when each test was written) was
// not.

// testFillerSources covers the persisted REMOTE source registry (§10, V33) on BOTH backends.
//
// The interesting assertions are the two that protect against silent data loss: a re-register
// must not reset "last fetched", and deleting a source must not take its clips with it.
// clipAt builds a catalog clip whose identity (`Hash`) is DISTINCT from its location (`Path`).
//
// ⚠ Hash and Path must never be equal here, and this is the whole point of the helper. They were
// the same string until V41, and that single fact hid two production defects for two releases:
// `DeleteClipsNotIn` wiped the entire catalog on every sync (see the post-mortem on
// `store.SetClipsRemoved`), and `UpdateClipTags`/`RecordClipPlay` were being called with a path
// against a `WHERE hash = ?` — so the AI tagger aborted on its first clip and play counters never
// moved. Every assertion passed throughout, because a fixture where the two keys are equal cannot
// tell them apart. A key-confusion bug is invisible by construction against such a fixture.
//
// ⚠ Real 64-hex hashes are still deliberately NOT used. The store does not care what a hash looks
// like — only `filler.ClipPath` validates the shape, and that is tested where it belongs
// (`filler/clippath_test.go`). A wall of hex would cost readability without buying coverage. The
// property that matters is that the two fields are DISTINGUISHABLE, not that the hash is realistic,
// so the readable path gets a short readable hash beside it.
func clipAt(path, name string, kind filler.Kind, durationMs int64) filler.Clip {
	return filler.Clip{Hash: clipHashFor(path), Path: path, Name: name, Kind: kind, DurationMs: durationMs}
}

// clipHashFor derives a fixture clip's identity from its path, and clipPathFor derives a fixture
// clip's location from its identity. The two builders in this file start from opposite ends —
// `clipAt` is given a readable path, `sampleClip` a readable id — so each needs the other half.
//
// Both are deterministic (the suite asserts exact values and runs against two backends), readable
// in failure output, and — the load-bearing property — never equal to the value they derive from.
func clipHashFor(path string) string { return "h:" + path }

func clipPathFor(hash string) string { return "p/" + hash + ".mp4" }

func sampleClip(id, name string, kind filler.Kind, era int, aud filler.Audience, cat string) Clip {
	c := Clip{}
	// ⚠ Identity is the HASH since V38c, not the path (§10).
	//
	// These tests use the READABLE id as the hash — "c1", not 64 hex characters — so assertions
	// stay legible (`GetClip(ctx, "c1")`) and a failure names a clip a human recognises. The
	// store does not care what a hash looks like; only `filler.ClipPath` validates the shape, and
	// that is covered where it belongs, in `filler/clippath_test.go`. Using real hashes here
	// would make every assertion a wall of hex and would test nothing extra.
	//
	// ⚠ But `Path` must NOT be the id as well. It was until V41, and hash-keyed and path-keyed
	// store methods became indistinguishable to this suite: `UpdateClipTags` (`WHERE hash = ?`)
	// was being called with a path in production and every test still passed. See `clipAt` for
	// the full post-mortem. Keep the two fields distinct in every fixture, always.
	c.Hash = id
	c.Path = clipPathFor(id)
	c.TunarrProgramID = "tun-" + id
	c.Name = name
	c.Kind = kind
	c.Era = era
	c.Audience = aud
	c.Category = cat
	c.DurationMs = 30000
	c.Source = "archive"
	c.UpdatedAt = time.Unix(1_700_000_000, 0).UTC()
	return c
}

func cachedClipFingerprint(ctx context.Context, s Store, clipHash, algorithm string) ([]uint64, bool, error) {
	all, err := s.ListClipFingerprints(ctx, algorithm)
	if err != nil {
		return nil, false, err
	}
	frames, found := all[clipHash]
	return frames, found, nil
}

func ids2(clips []Clip) []string {
	out := make([]string, len(clips))
	for i, c := range clips {
		out[i] = c.Path
	}
	return out
}

// findSource returns one source by id, and whether it is still listed.
//
// ⚠ By id, not by position. V37's migration seeds two singleton rows (`folder`, `library`) so a
// fresh store can still express "not configured", which means `ListFillerSources(ctx)[0]` is no
// longer the row a test just added — it is whichever seeded row sorts first. Every positional
// read in this suite was rewritten through here after exactly that broke.
func findSource(t *testing.T, s Store, id string) (FillerSource, bool) {
	t.Helper()
	all, err := s.ListFillerSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all {
		if f.ID == id {
			return f, true
		}
	}
	return FillerSource{}, false
}

// src1 is the source this suite registers first, re-read. Fatals if it has gone missing, so a
// caller can assert on its fields without a nil check at every use.
func src1(t *testing.T, s Store) FillerSource {
	t.Helper()
	f, ok := findSource(t, s, "src-1")
	if !ok {
		t.Fatal("src-1 is not listed")
	}
	return f
}

func testClipFilters(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	_ = s.UpsertClip(ctx, sampleClip("c1", "Frosted Flakes", filler.Commercial, 1992, filler.Kids, "cereal"))
	_ = s.UpsertClip(ctx, sampleClip("c2", "TMNT figures", filler.Commercial, 1994, filler.Kids, "toys"))
	_ = s.UpsertClip(ctx, sampleClip("b1", "Bumper", filler.Bumper, 1992, filler.General, ""))
	_ = s.UpsertClip(ctx, sampleClip("u1", "untagged.mp4", filler.Commercial, 0, "", "")) // untagged

	// Round-trip.
	got, err := s.GetClip(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Frosted Flakes" || got.Kind != filler.Commercial || got.Era != 1992 || got.Audience != filler.Kids || got.Category != "cereal" {
		t.Errorf("clip round-trip mismatch: %+v", got.Clip)
	}
	if got.DurationMs != 30000 {
		t.Errorf("duration lost: %d", got.DurationMs)
	}
	if _, err := s.GetClip(ctx, "nope"); err != ErrNotFound {
		t.Errorf("GetClip(missing) = %v, want ErrNotFound", err)
	}

	// Filter by kind.
	comms, _ := s.ListClips(ctx, ClipFilter{Kind: filler.Commercial})
	if len(comms) != 3 {
		t.Errorf("kind=commercial = %d, want 3", len(comms))
	}
	// Filter by audience + era.
	// ⚠ Assert on Hash, not Path: identity is the hash (§10 V38c). Asserting the path here read
	// as correct only while the fixture made the two equal.
	kids92, _ := s.ListClips(ctx, ClipFilter{Audience: filler.Kids, Era: 1992})
	if len(kids92) != 1 || kids92[0].Hash != "c1" {
		t.Errorf("kids+1992 = %+v, want just c1", ids2(kids92))
	}
	// Untagged only.
	untagged, _ := s.ListClips(ctx, ClipFilter{UntaggedOnly: true})
	if len(untagged) != 1 || untagged[0].Hash != "u1" {
		t.Errorf("untagged = %+v, want just u1", ids2(untagged))
	}
	// Empty filter = all.
	all, _ := s.ListClips(ctx, ClipFilter{})
	if len(all) != 4 {
		t.Errorf("no filter = %d, want 4", len(all))
	}
}

func testClipTagsAndPrune(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	_ = s.UpsertClip(ctx, sampleClip("u1", "untagged.mp4", filler.Commercial, 0, "", ""))
	_ = s.UpsertClip(ctx, sampleClip("keep", "keep.mp4", filler.Bumper, 1992, filler.General, ""))

	// Tag the untagged clip (the AI-tagging job path).
	if err := s.UpdateClipTags(ctx, "u1", 1994, "kids", "cereal", 0, true, now); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetClip(ctx, "u1")
	if got.Era != 1994 || got.Audience != filler.Kids || got.Category != "cereal" || !got.AITagged {
		t.Errorf("tag update didn't persist: %+v", got.Clip)
	}
	if !got.Tagged() {
		t.Error("clip should be Tagged() after update")
	}
	// Tagging a missing clip → ErrNotFound.
	if err := s.UpdateClipTags(ctx, "gone", 1990, "kids", "toys", 0, false, now); err != ErrNotFound {
		t.Errorf("UpdateClipTags(missing) = %v, want ErrNotFound", err)
	}

	// Era suggestions (§10 V34) — the conditional suggested_era write:
	//  **record** an ungrounded suggestion on an era-less clip,
	//  **keep** it across a tag edit that carries neither era nor suggestion,
	//  **clear** it in the same write that sets era (the operator confirming).
	if err := s.UpdateClipTags(ctx, "keep", 0, "family", "", 1985, false, now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, "keep")
	if got.SuggestedEra != 1985 || got.Era != 0 {
		t.Errorf("suggestion not recorded: era=%d suggestedEra=%d, want 0/1985", got.Era, got.SuggestedEra)
	}
	if err := s.UpdateClipTags(ctx, "keep", 0, "general", "", 0, false, now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, "keep")
	if got.SuggestedEra != 1985 {
		t.Errorf("era-less tag edit wiped the suggestion: suggestedEra=%d, want 1985", got.SuggestedEra)
	}
	if err := s.UpdateClipTags(ctx, "keep", 1985, "", "", 0, false, now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, "keep")
	if got.Era != 1985 || got.SuggestedEra != 0 {
		t.Errorf("confirming era did not clear the suggestion: era=%d suggestedEra=%d, want 1985/0", got.Era, got.SuggestedEra)
	}
	// A suggestion survives a sync upsert (sync.go merges it like the other tags).
	keep, _ := s.GetClip(ctx, "keep")
	keep.SuggestedEra = 1990
	if err := s.UpsertClip(ctx, keep); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, "keep")
	if got.SuggestedEra != 1990 {
		t.Errorf("suggested_era did not round-trip through UpsertClip: %d, want 1990", got.SuggestedEra)
	}

	// Prune: keep only "keep" — u1 is removed (it left the media server's library).
	n, err := s.DeleteClipsNotIn(ctx, []string{"keep"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("prune removed %d, want 1", n)
	}
	if _, err := s.GetClip(ctx, "u1"); err != ErrNotFound {
		t.Error("pruned clip still present")
	}
	if _, err := s.GetClip(ctx, "keep"); err != nil {
		t.Error("kept clip was wrongly pruned")
	}
	// Prune with empty keep set deletes all.
	n, _ = s.DeleteClipsNotIn(ctx, nil)
	if n != 1 {
		t.Errorf("prune-all removed %d, want 1", n)
	}
}

// testClipNameSearch covers the §7.2 `name LIKE` clip search. It is in the shared suite
// because the two dialects disagree by default: SQLite's LIKE folds ASCII case while
// Postgres's does not, so a naive implementation would make search case-sensitive on
// exactly one backend — the dialect fork the store rules forbid.
func testClipNameSearch(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	_ = s.UpsertClip(ctx, sampleClip("c1", "Frosted Flakes", filler.Commercial, 1992, filler.Kids, "cereal"))
	_ = s.UpsertClip(ctx, sampleClip("c2", "TMNT figures", filler.Commercial, 1994, filler.Kids, "toys"))
	_ = s.UpsertClip(ctx, sampleClip("c3", "100% Juice", filler.Commercial, 1993, filler.Kids, "drinks"))

	names := func(f ClipFilter) []string {
		t.Helper()
		got, err := s.ListClips(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(got))
		for _, c := range got {
			out = append(out, c.Name)
		}
		return out
	}

	// Substring, and case-insensitive on BOTH backends.
	if got := names(ClipFilter{Query: "flakes"}); len(got) != 1 || got[0] != "Frosted Flakes" {
		t.Errorf("Query=flakes → %v, want [Frosted Flakes] (case-insensitive on both dialects)", got)
	}
	if got := names(ClipFilter{Query: "FROSTED"}); len(got) != 1 {
		t.Errorf("Query=FROSTED → %v, want 1 match", got)
	}

	// A literal % must not act as a wildcard. Without escaping this returns everything,
	// which reads as "search is broken" and scans the whole table.
	if got := names(ClipFilter{Query: "%"}); len(got) != 1 || got[0] != "100% Juice" {
		t.Errorf("Query=%% → %v, want only [100%% Juice] — %% must be literal, not a wildcard", got)
	}
	// Likewise _, which would otherwise match any single character.
	if got := names(ClipFilter{Query: "_"}); len(got) != 0 {
		t.Errorf("Query=_ → %v, want none — _ must be literal, not a single-char wildcard", got)
	}

	// Search composes with the other filters rather than replacing them.
	if got := names(ClipFilter{Query: "e", Category: "toys"}); len(got) != 1 || got[0] != "TMNT figures" {
		t.Errorf("Query+Category → %v, want [TMNT figures]", got)
	}
}

// V28: play counters are written ONLY by RecordClipPlay, and a re-sync must not reset them.
//
// The reset is the bug worth pinning: UpsertClip lists most columns in its ON CONFLICT DO
// UPDATE, so adding play_count there would zero every counter on each sync pass — silently,
// and visible only as "usage never goes up".
func testClipPlayCounters(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()

	c := sampleClip("1994/toys.mp4", "TMNT toys", filler.Commercial, 1994, filler.Kids, "toys")
	c.Thumbnail = "1994/toys.jpg"
	// ⚠ The animated preview is a SEPARATE column, not derived from the still (V39, 00035). Both
	// are asserted here because a column added to the INSERT and forgotten in the SELECT (or
	// vice versa) is a silent data loss the type system cannot see — the two lists are
	// hand-maintained and positional.
	c.Preview = "1994/toys.webp"
	// The detected language (V40, 00036) — a third hand-maintained position in the same lists.
	c.Language = "en"
	if err := s.UpsertClip(ctx, c); err != nil {
		t.Fatalf("seed clip: %v", err)
	}

	// ⚠ Read by HASH, not path. `GetClip`, `RecordClipPlay` and `UpdateClipTags` are all keyed
	// `WHERE hash = ?`; passing a path returns ErrNotFound. This test used `c.Path` throughout
	// while the fixture made the two equal, so it could not distinguish the two keys — and the
	// production callers that passed a path went undetected for two releases (see `clipAt`).
	got, err := s.GetClip(ctx, c.Hash)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Thumbnail != "1994/toys.jpg" {
		t.Errorf("thumbnail = %q, want it round-tripped", got.Thumbnail)
	}
	if got.Preview != "1994/toys.webp" {
		t.Errorf("preview = %q, want it round-tripped — a preview that vanishes on read means "+
			"every card silently falls back to its still", got.Preview)
	}
	// ⚠ A language that vanishes on read reads as NOT YET CHECKED, so the detection job would
	// re-run forever — and on the local backend that is ~341s of QEMU per clip, every cycle.
	if got.Language != "en" {
		t.Errorf("language = %q, want it round-tripped", got.Language)
	}
	if got.PlayCount != 0 || !got.LastPlayedAt.IsZero() {
		t.Errorf("a fresh clip must start unplayed, got count=%d at=%v", got.PlayCount, got.LastPlayedAt)
	}

	aired := time.Unix(1_800_000_000, 0).UTC()
	for i := 0; i < 3; i++ {
		if err := s.RecordClipPlay(ctx, c.Hash, aired); err != nil {
			t.Fatalf("record play %d: %v", i, err)
		}
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.PlayCount != 3 {
		t.Errorf("play count = %d, want 3", got.PlayCount)
	}
	if !got.LastPlayedAt.Equal(aired) {
		t.Errorf("last played = %v, want %v", got.LastPlayedAt, aired)
	}

	// A re-sync (what the periodic scan does) must leave the counters alone. Everything else
	// about the row is legitimately refreshed.
	c.Name = "TMNT toys (renamed)"
	if err := s.UpsertClip(ctx, c); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.Name != "TMNT toys (renamed)" {
		t.Errorf("a re-sync must refresh the name, got %q", got.Name)
	}
	if got.PlayCount != 3 {
		t.Errorf("a re-sync RESET the play count to %d — play_count must not be in the DO UPDATE list", got.PlayCount)
	}
	if !got.LastPlayedAt.Equal(aired) {
		t.Errorf("a re-sync reset last_played_at to %v", got.LastPlayedAt)
	}

	// Playout may resolve a clip the catalog has since pruned; that is telemetry missing a
	// row, not a playback error.
	if err := s.RecordClipPlay(ctx, "gone/missing.mp4", aired); err != nil {
		t.Errorf("recording a play for a pruned clip must not error, got %v", err)
	}
}

// testClipKeyIsHashNotPath pins WHICH key the clip writers take (§10 V38c).
//
// ⚠ This test exists because the absence of it cost two releases. `Hash` (identity) and `Path`
// (location) are both strings, so passing the wrong one is invisible to the compiler — and every
// fixture in this suite set them to the same value, so it was invisible to the tests too. Two
// production callers passed a path into a hash-keyed UPDATE: the AI tagger aborted on its first
// clip (ErrNotFound is fatal there) and play counters silently never moved.
//
// The assertions are deliberately negative — "a path must NOT work" — because the positive case
// passes just as happily against a store that accepts either.
func testClipKeyIsHashNotPath(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()

	c := sampleClip("k1", "keyed.mp4", filler.Commercial, 0, "", "")
	if c.Hash == c.Path {
		t.Fatalf("fixture bug: hash and path are equal (%q) — this test cannot distinguish the "+
			"two keys and neither can any other test in this file", c.Hash)
	}
	if err := s.UpsertClip(ctx, c); err != nil {
		t.Fatal(err)
	}

	// GetClip is hash-keyed.
	if _, err := s.GetClip(ctx, c.Path); err != ErrNotFound {
		t.Errorf("GetClip(path) = %v, want ErrNotFound — GetClip is keyed WHERE hash = ?", err)
	}
	if _, err := s.GetClip(ctx, c.Hash); err != nil {
		t.Errorf("GetClip(hash) = %v, want it to resolve", err)
	}

	// UpdateClipTags is hash-keyed, and its ErrNotFound is fatal to the tagging job.
	if err := s.UpdateClipTags(ctx, c.Path, 1994, "kids", "cereal", 0, true, now); err != ErrNotFound {
		t.Errorf("UpdateClipTags(path) = %v, want ErrNotFound — the tagger must pass the hash", err)
	}
	if err := s.UpdateClipTags(ctx, c.Hash, 1994, "kids", "cereal", 0, true, now); err != nil {
		t.Errorf("UpdateClipTags(hash) = %v, want it to apply", err)
	}

	// RecordClipPlay is hash-keyed and deliberately silent on a miss, so assert the COUNTER
	// rather than the error — a path that no-ops returns nil either way.
	if err := s.RecordClipPlay(ctx, c.Path, now); err != nil {
		t.Errorf("RecordClipPlay(path) = %v, want a silent no-op", err)
	}
	got, _ := s.GetClip(ctx, c.Hash)
	if got.PlayCount != 0 {
		t.Errorf("play count = %d after recording against a PATH, want 0", got.PlayCount)
	}
	if err := s.RecordClipPlay(ctx, c.Hash, now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.PlayCount != 1 {
		t.Errorf("play count = %d after recording against the HASH, want 1", got.PlayCount)
	}

	// SetClipLanguage is the other half of the split: path-keyed, by design (the language job
	// walks the filesystem). Pinned so a well-meaning "consistency" change has to be deliberate.
	if err := s.SetClipLanguage(ctx, c.Path, "en", now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.Language != "en" {
		t.Errorf("language = %q, want %q — SetClipLanguage is keyed WHERE path = ?", got.Language, "en")
	}

	// SetClipTranscript joins the path-keyed job writers (§10 V44). Same split, same reason.
	if err := s.SetClipTranscript(ctx, c.Path, "buy our cereal today", now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.Transcript != "buy our cereal today" {
		t.Errorf("transcript = %q, want the recorded text — SetClipTranscript is keyed WHERE path = ?", got.Transcript)
	}

	// SetClipBrand is the TEXT tagger's grounded brand writer (§10 V44) — path-keyed like the
	// transcript writer, and it must write `brand` WITHOUT stamping `vision_tagged`: a brand grounded
	// in the transcript is not a brand a vision pass read off a frame, and conflating the two would
	// make a re-run skip the vision it never actually ran.
	if err := s.SetClipBrand(ctx, c.Path, "Post", now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.Brand != "Post" {
		t.Errorf("brand = %q, want %q — SetClipBrand is keyed WHERE path = ?", got.Brand, "Post")
	}
	if got.VisionTagged {
		t.Errorf("vision_tagged set by a TEXT brand write — SetClipBrand must not masquerade as a vision pass")
	}

	// SetClipVisionTags records the on-screen text, a grounded brand, and (here) a category. era is
	// passed 0, so the era set by UpdateClipTags above (1994) must SURVIVE — the CASE guard, not a
	// blanket overwrite. It overwrites the text-grounded brand above, which is fine — both are
	// grounded writers of the same column.
	if err := s.SetClipVisionTags(ctx, c.Path, "Kellogg's", "KELLOGG'S FROSTED FLAKES", 0, 0, "cereal", now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.Brand != "Kellogg's" || got.VisibleText != "KELLOGG'S FROSTED FLAKES" || !got.VisionTagged {
		t.Errorf("vision tags = {brand:%q visible:%q tagged:%v}, want them recorded", got.Brand, got.VisibleText, got.VisionTagged)
	}
	if got.Era != 1994 {
		t.Errorf("era = %d after a vision write with era=0, want 1994 preserved — the CASE guard must not blank an existing era", got.Era)
	}
	// ⚠ A grounded era SUPPRESSES the frame-heuristic suggestion: this clip already has era 1994, so
	// passing suggestedEra=1960 must NOT set suggested_era — a clip with a known era has no question
	// to ask. This pins the "era = 0" precondition in the store's CASE guard.
	if err := s.SetClipVisionTags(ctx, c.Path, "Kellogg's", "KELLOGG'S FROSTED FLAKES", 0, 1960, "cereal", now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.SuggestedEra != 0 {
		t.Errorf("suggested_era = %d, want 0 — a frame hint must not seed a suggestion when the clip already has a grounded era", got.SuggestedEra)
	}

	// The frame-heuristic path on a clip with NO era: a fresh clip, vision grounds no era, the hint
	// seeds a suggestion. This is the one write that turns a monochrome-4:3 measurement into the
	// operator-confirms field.
	hintClip := sampleClip("vhint", "wordless.mp4", filler.Commercial, 0, "", "")
	if err := s.UpsertClip(ctx, hintClip); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipVisionTags(ctx, hintClip.Path, "", "", 0, 1960, "", now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, hintClip.Hash)
	if got.SuggestedEra != 1960 {
		t.Errorf("suggested_era = %d, want 1960 — a frame hint must seed the suggestion when the clip has no grounded era", got.SuggestedEra)
	}
	if got.Era != 0 {
		t.Errorf("era = %d, want 0 — a frame hint is a SUGGESTION, never a grounded era", got.Era)
	}

	// ⚠ The load-bearing property: a re-sync (UpsertClip) must NOT clobber the job-written columns.
	// The scan re-upserts every file it finds carrying a ZERO transcript/brand — if any of these
	// rode UpsertClip's DO UPDATE list, this upsert would wipe the work above and re-trigger Whisper
	// / a paid vision call on the next pass. This is the same discipline the language/held/counter
	// columns already pin; without it a green suite would hide the exact regression 00038 guards.
	resync := c
	resync.Transcript, resync.Brand, resync.VisibleText, resync.VisionTagged = "", "", "", false
	if err := s.UpsertClip(ctx, resync); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.Transcript != "buy our cereal today" {
		t.Errorf("transcript = %q after a re-sync, want it PRESERVED — UpsertClip must omit transcript from DO UPDATE", got.Transcript)
	}
	if got.Brand != "Kellogg's" || got.VisibleText != "KELLOGG'S FROSTED FLAKES" || !got.VisionTagged {
		t.Errorf("vision tags lost on re-sync {brand:%q visible:%q tagged:%v} — UpsertClip must omit them from DO UPDATE", got.Brand, got.VisibleText, got.VisionTagged)
	}
}

// An internal transform changes the content hash without changing what the clip means to the
// operator. Every durable reference must follow in one transaction; otherwise a rescan produces
// a fresh hash-titled row while tags, lineage, pipeline progress and channel overrides stay on an
// orphan identity.
func testClipIdentityReplacement(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0).UTC()

	old := sampleClip("old-content", "McDonald's 1993", filler.Commercial, 1993, filler.Kids, "fast_food")
	old.ParentHash = "parent-reel"
	old.Held = true
	old.Confidence = 91
	old.Transcript = "two all beef patties"
	if err := s.UpsertClip(ctx, old); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertClipFingerprint(ctx, old.Hash, "dhash-v1", []uint64{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	child := sampleClip("child", "Child", filler.Commercial, 1993, filler.Kids, "toys")
	child.ParentHash = old.Hash
	if err := s.UpsertClip(ctx, child); err != nil {
		t.Fatal(err)
	}

	taxa, err := s.ListTaxa(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipTags(ctx, old.Hash, []string{"cereal"}, taxonomy.New(taxa), now); err != nil {
		t.Fatal(err)
	}
	pipeline := filler.ClipPipeline{
		ClipHash: old.Hash, Stage: filler.StageTranscode, Status: filler.StatusRunning,
		Disposition: filler.DispositionRunning, EnrolledAt: now, UpdatedAt: now,
	}
	if err := s.UpsertClipPipeline(ctx, pipeline); err != nil {
		t.Fatal(err)
	}
	proposal := filler.SplitProposal{ID: "proposal", ClipHash: old.Hash, CreatedAt: now}
	if err := s.UpsertSplitProposal(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	channel := sampleChannel("clip-ref", 711, now)
	channel.Policy.Filler = &schedule.FillerSelection{
		Pinned: []string{"keep", old.Hash}, Excluded: []string{old.Hash, "other"},
	}
	channel, err = s.SaveChannel(ctx, channel)
	if err != nil {
		t.Fatal(err)
	}

	replacement := old
	replacement.Hash = "new-content"
	replacement.Path = "ne/wc/new-content.mp4"
	replacement.DurationMs = 30_033
	replacement.Quality = "480p"
	replacement.UpdatedAt = now.Add(time.Minute)
	if err := s.ReplaceClipIdentity(ctx, old.Hash, replacement); err != nil {
		t.Fatal(err)
	}

	if _, err := s.GetClip(ctx, old.Hash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old identity still resolves: %v", err)
	}
	got, err := s.GetClip(ctx, replacement.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != old.Name || got.Source != old.Source || got.ParentHash != old.ParentHash ||
		got.Transcript != old.Transcript || !got.Held || got.Confidence != old.Confidence {
		t.Errorf("metadata did not follow identity replacement: %+v", got)
	}
	if got.Path != replacement.Path || got.DurationMs != replacement.DurationMs || got.Quality != replacement.Quality {
		t.Errorf("transformed facts were not installed: %+v", got)
	}
	if len(got.Tags) == 0 {
		t.Error("taxonomy tags stayed behind on the old identity")
	}
	if row, found, err := s.GetClipPipeline(ctx, replacement.Hash); err != nil || !found || row.Stage != filler.StageTranscode {
		t.Errorf("pipeline did not follow replacement: %+v, %v, %v", row, found, err)
	}
	if _, found, err := s.GetClipPipeline(ctx, old.Hash); err != nil || found {
		t.Errorf("old pipeline row survived: found=%v err=%v", found, err)
	}
	proposals, err := s.ListSplitProposals(ctx)
	if err != nil || len(proposals) != 1 || proposals[0].ClipHash != replacement.Hash {
		t.Errorf("split proposal did not follow replacement: %+v (%v)", proposals, err)
	}
	gotChild, err := s.GetClip(ctx, child.Hash)
	if err != nil || gotChild.ParentHash != replacement.Hash {
		t.Errorf("child lineage did not follow replacement: %+v (%v)", gotChild, err)
	}
	gotChannel, err := s.GetChannel(ctx, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotChannel.Policy.Filler == nil || gotChannel.Policy.Filler.Pinned[1] != replacement.Hash ||
		gotChannel.Policy.Filler.Excluded[0] != replacement.Hash {
		t.Errorf("channel overrides did not follow replacement: %+v", gotChannel.Policy.Filler)
	}
	if gotChannel.Revision != channel.Revision+1 {
		t.Errorf("channel policy rekey revision = %d, want %d", gotChannel.Revision, channel.Revision+1)
	}
	if _, found, err := cachedClipFingerprint(ctx, s, old.Hash, "dhash-v1"); err != nil || found {
		t.Errorf("old-byte fingerprint survived identity replacement: found=%v err=%v", found, err)
	}
	if _, found, err := cachedClipFingerprint(ctx, s, replacement.Hash, "dhash-v1"); err != nil || found {
		t.Errorf("old-byte fingerprint was re-keyed onto replacement bytes: found=%v err=%v", found, err)
	}
}

// testClipFingerprintCache pins the cache's two correctness properties on both backends: only an
// exact content+algorithm key hits, and catalog pruning removes sibling-table orphans.
func testClipFingerprintCache(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	one := sampleClip("fingerprint-one", "One", filler.Commercial, 1993, filler.General, "")
	two := sampleClip("fingerprint-two", "Two", filler.Commercial, 1994, filler.General, "")
	if err := s.UpsertClip(ctx, one); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertClip(ctx, two); err != nil {
		t.Fatal(err)
	}

	if err := s.UpsertClipFingerprint(ctx, one.Hash, "dhash-v1", nil); err == nil {
		t.Fatal("empty fingerprint was persisted")
	}
	want := []uint64{0, 1, ^uint64(0)}
	if err := s.UpsertClipFingerprint(ctx, one.Hash, "dhash-v1", want); err != nil {
		t.Fatal(err)
	}
	got, found, err := cachedClipFingerprint(ctx, s, one.Hash, "dhash-v1")
	if err != nil || !found || len(got) != len(want) || got[2] != want[2] {
		t.Fatalf("fingerprint round-trip = (%v, %v, %v), want %v", got, found, err, want)
	}
	if _, found, err := cachedClipFingerprint(ctx, s, one.Hash, "dhash-v2"); err != nil || found {
		t.Errorf("different algorithm hit the cache: found=%v err=%v", found, err)
	}
	if err := s.UpsertClipFingerprint(ctx, one.Hash, "dhash-v1", []uint64{9}); err != nil {
		t.Fatal(err)
	}
	got, found, err = cachedClipFingerprint(ctx, s, one.Hash, "dhash-v1")
	if err != nil || !found || len(got) != 1 || got[0] != 9 {
		t.Errorf("idempotent upsert = (%v, %v, %v), want [9]", got, found, err)
	}
	if err := s.UpsertClipFingerprint(ctx, two.Hash, "dhash-v1", []uint64{7}); err != nil {
		t.Fatal(err)
	}

	// Keep only clip two. The cache has no FK by design, so DeleteClipsNotIn owns this cleanup.
	if _, err := s.DeleteClipsNotIn(ctx, []string{two.Hash}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := cachedClipFingerprint(ctx, s, one.Hash, "dhash-v1"); err != nil || found {
		t.Errorf("pruned clip left an orphan fingerprint: found=%v err=%v", found, err)
	}
	if got, found, err := cachedClipFingerprint(ctx, s, two.Hash, "dhash-v1"); err != nil || !found || len(got) != 1 || got[0] != 7 {
		t.Errorf("kept clip lost its fingerprint: got=%v found=%v err=%v", got, found, err)
	}
}

// testCompositeLineage pins the V45 composite/lineage invariants (§10): a composite is excluded from
// the default listing (the pod-assembly path), its segments link back via parent_hash, and neither
// is_composite nor parent_hash is clobbered by a re-sync.
func testCompositeLineage(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()

	// A composite (the recorded break) and two segments split out of it.
	comp := sampleClip("break1", "kcpq-1996.mp4", filler.Commercial, 1996, filler.General, "")
	comp.DurationMs = 971_000
	if err := s.UpsertClip(ctx, comp); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipComposite(ctx, comp.Hash, true, now); err != nil {
		t.Fatal(err)
	}
	seg1 := sampleClip("seg1", "aw-root-beer.mp4", filler.Commercial, 1996, filler.General, "fast_food")
	seg1.ParentHash = comp.Hash
	seg2 := sampleClip("seg2", "kfc.mp4", filler.Commercial, 1996, filler.General, "fast_food")
	seg2.ParentHash = comp.Hash
	if err := s.UpsertClip(ctx, seg1); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertClip(ctx, seg2); err != nil {
		t.Fatal(err)
	}

	// ⚠ THE load-bearing exclusion: a composite must NOT appear in the default (pod-assembly) listing.
	// Airing a 16-minute break as one "commercial" is the bug the flag removes.
	def, _ := s.ListClips(ctx, ClipFilter{})
	for _, c := range def {
		if c.Hash == comp.Hash {
			t.Errorf("composite %q appeared in the default listing — pod assembly would air a 16-min block as one commercial", comp.Hash)
		}
	}
	// The two segments ARE airable and present by default.
	if !containsHash(def, seg1.Hash) || !containsHash(def, seg2.Hash) {
		t.Errorf("segments missing from the default listing — they are the airable clips")
	}

	// Opt-in surfaces the composite (the catalog/lineage view).
	withComp, _ := s.ListClips(ctx, ClipFilter{IncludeComposites: true})
	if !containsHash(withComp, comp.Hash) {
		t.Errorf("IncludeComposites did not surface the composite")
	}
	only, _ := s.ListClips(ctx, ClipFilter{CompositesOnly: true})
	if len(only) != 1 || only[0].Hash != comp.Hash {
		t.Errorf("CompositesOnly = %d clips, want just the composite", len(only))
	}

	// Lineage: the segments of one composite, by parent_hash.
	kids, _ := s.ListClips(ctx, ClipFilter{ParentHash: comp.Hash})
	if len(kids) != 2 {
		t.Errorf("parent_hash query returned %d segments, want 2", len(kids))
	}
	for _, k := range kids {
		if k.ParentHash != comp.Hash {
			t.Errorf("segment %q parent_hash = %q, want %q", k.Hash, k.ParentHash, comp.Hash)
		}
	}

	// ⚠ A re-sync (the folder scan finding the original file) must NOT flip the composite back to
	// airable, nor blank a segment's lineage — is_composite/parent_hash are omitted from DO UPDATE.
	resync := comp
	resync.IsComposite = false // the scan-built Clip knows nothing of the composite mark
	if err := s.UpsertClip(ctx, resync); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetClip(ctx, comp.Hash)
	if !got.IsComposite {
		t.Errorf("is_composite lost on re-sync — UpsertClip must omit it from DO UPDATE, else a confirmed break re-airs")
	}
	resyncSeg := seg1
	resyncSeg.ParentHash = "" // the scan does not know whose segment this is
	if err := s.UpsertClip(ctx, resyncSeg); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, seg1.Hash)
	if got.ParentHash != comp.Hash {
		t.Errorf("parent_hash = %q after re-sync, want it PRESERVED — UpsertClip must omit it from DO UPDATE", got.ParentHash)
	}
}

func containsHash(clips []Clip, hash string) bool {
	for _, c := range clips {
		if c.Hash == hash {
			return true
		}
	}
	return false
}

// testTaxonomy pins the V45a taxonomy store: the default forest is seeded on open, tagging a clip
// expands to denormalised rollups, and re-tagging REPLACES (never accumulates) leaves while rollups
// track the current graph.
func testTaxonomy(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()

	// ⚠ Seeded on open: a fresh store already has the default forest (the boot seeder ran in Open).
	taxa, err := s.ListTaxa(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(taxa) < 40 {
		t.Fatalf("taxonomy not seeded on open: %d taxa, want the default forest (~55)", len(taxa))
	}
	forest := taxonomy.New(taxa)
	if _, ok := forest.Get("beer"); !ok {
		t.Fatal("seeded forest missing 'beer' — the seed did not load from SeedForest")
	}

	// Tag a clip `beer` → the denormalised set must be beer(leaf) + alcohol + drinks (rollups).
	if err := s.SetClipTags(ctx, "clipA", []string{"beer"}, forest, now); err != nil {
		t.Fatal(err)
	}
	full, _ := s.GetClipTags(ctx, "clipA", false)
	assertSet(t, "clipA full tags", full, []string{"alcohol", "beer", "drinks"})
	leaves, _ := s.GetClipTags(ctx, "clipA", true)
	assertSet(t, "clipA leaves", leaves, []string{"beer"})

	// Two leaves sharing an ancestor: beer + spirits → alcohol/drinks stored ONCE (as rollups).
	if err := s.SetClipTags(ctx, "clipB", []string{"beer", "spirits"}, forest, now); err != nil {
		t.Fatal(err)
	}
	full, _ = s.GetClipTags(ctx, "clipB", false)
	assertSet(t, "clipB full tags", full, []string{"alcohol", "beer", "drinks", "spirits"})

	// ⚠ The READ PATH loads Tags (§10 V45a): a real clip row, tagged, must come back from GetClip and
	// ListClips with its full leaf+rollup set in Clip.Tags — the batched attachTags load. Without this
	// the whole pod/DTO layer would read empty tags off every clip. Seed an actual clip (the tag rows
	// above are bare taxon rows with no clip); tag it; read it back both ways.
	realClip := Clip{UpdatedAt: now}
	realClip.Hash = "clipReal"
	realClip.Path = "cr/clipReal.mp4"
	realClip.Name = "Real"
	realClip.Kind = filler.Commercial
	realClip.DurationMs = 30000
	if err := s.UpsertClip(ctx, realClip); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipTags(ctx, "clipReal", []string{"beer"}, forest, now); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetClip(ctx, "clipReal")
	if err != nil {
		t.Fatal(err)
	}
	assertSet(t, "GetClip loads Tags (full rollup set)", got.Tags, []string{"alcohol", "beer", "drinks"})
	listed, err := s.ListClips(ctx, ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range listed {
		if c.Hash == "clipReal" {
			found = true
			assertSet(t, "ListClips loads Tags (full rollup set)", c.Tags, []string{"alcohol", "beer", "drinks"})
		}
	}
	if !found {
		t.Error("clipReal missing from ListClips")
	}

	// ⚠ Re-tag REPLACES, never accumulates: clipA re-tagged `cereal` must lose beer/alcohol/drinks and
	// gain cereal/food — not keep the old alcohol lineage.
	if err := s.SetClipTags(ctx, "clipA", []string{"cereal"}, forest, now); err != nil {
		t.Fatal(err)
	}
	full, _ = s.GetClipTags(ctx, "clipA", false)
	assertSet(t, "clipA after re-tag", full, []string{"cereal", "food"})

	// The reindex work list: every clip's asserted leaves (clipA→cereal, clipB→beer,spirits).
	leavesByClip, _ := s.ListClipHashesLeaves(ctx)
	assertSet(t, "clipA leaves in worklist", leavesByClip["clipA"], []string{"cereal"})
	assertSet(t, "clipB leaves in worklist", leavesByClip["clipB"], []string{"beer", "spirits"})

	// Operator edit: add a taxon; it must appear in ListTaxa (the CRUD path).
	if err := s.UpsertTaxon(ctx, taxonomy.Taxon{Slug: "energy-drink", Label: "Energy drink", Parent: "drinks", Axis: taxonomy.AxisProduct}, now); err != nil {
		t.Fatal(err)
	}
	taxa2, _ := s.ListTaxa(ctx)
	if _, ok := taxonomy.New(taxa2).Get("energy-drink"); !ok {
		t.Error("UpsertTaxon did not persist the new taxon")
	}

	testReindex(t, s, ctx, now)
}

// testReindex pins the V45a bulk reindex (§10): the set-based RebuildClosure+RebuildRollups must
// produce the SAME rollups the per-clip SetClipTags does (the two-writer equivalence), and a graph
// edit followed by a reindex must re-derive rollups against the NEW graph. Runs on the store
// testTaxonomy leaves behind (default forest, clipA=cereal, clipB=beer+spirits).
func testReindex(t *testing.T, s Store, ctx context.Context, now time.Time) {
	// Snapshot what the per-clip writer produced, before the bulk writer touches anything.
	beforeA, _ := s.GetClipTags(ctx, "clipA", false)
	beforeB, _ := s.GetClipTags(ctx, "clipB", false)

	// ⚠ TWO-WRITER EQUIVALENCE. Rebuild the closure from the current graph, then rebuild ALL rollups
	// in one set-based statement. The result must MATCH what SetClipTags wrote per clip — if the bulk
	// SQL join and the Go WithRollups loop ever disagreed, a re-tag and a reindex would race to
	// different answers. This is the invariant the whole closure-table design turns on.
	taxa, _ := s.ListTaxa(ctx)
	if err := s.RebuildClosure(ctx, taxonomy.New(taxa), now); err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildRollups(ctx); err != nil {
		t.Fatal(err)
	}
	afterA, _ := s.GetClipTags(ctx, "clipA", false)
	afterB, _ := s.GetClipTags(ctx, "clipB", false)
	assertSet(t, "clipA rollups: bulk reindex == per-clip write", afterA, beforeA)
	assertSet(t, "clipB rollups: bulk reindex == per-clip write", afterB, beforeB)

	// ⚠ Leaves must SURVIVE the rollup rebuild — RebuildRollups deletes only rollup rows (leaf=false)
	// and re-derives the rest; an asserted leaf is never touched. (Sabotage check: if RebuildRollups
	// deleted leaves too, these go empty and the equivalence above would also have failed.)
	leavesA, _ := s.GetClipTags(ctx, "clipA", true)
	assertSet(t, "clipA leaves survive reindex", leavesA, []string{"cereal"})
	leavesB, _ := s.GetClipTags(ctx, "clipB", true)
	assertSet(t, "clipB leaves survive reindex", leavesB, []string{"beer", "spirits"})

	// ⚠ GRAPH EDIT → REINDEX picks up the new lineage. Insert a taxon BETWEEN an existing leaf and its
	// parent, so the rollup set genuinely changes. `lager` under a new `ale-family` under `beer`.
	if err := s.UpsertTaxon(ctx, taxonomy.Taxon{Slug: "ale-family", Label: "Ale family", Parent: "beer", Axis: taxonomy.AxisProduct}, now); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertTaxon(ctx, taxonomy.Taxon{Slug: "lager", Label: "Lager", Parent: "ale-family", Axis: taxonomy.AxisProduct}, now); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipTags(ctx, "clipC", []string{"lager"}, taxonomy.New(mustList(t, s, ctx)), now); err != nil {
		t.Fatal(err)
	}
	// With the ale-family hop present: lager → ale-family → beer → alcohol → drinks.
	afterC, _ := s.GetClipTags(ctx, "clipC", false)
	assertSet(t, "clipC rollups with ale-family hop", afterC, []string{"lager", "ale-family", "beer", "alcohol", "drinks"})

	// ⚠ DELETE the MIDDLE taxon: DeleteTaxon REPARENTS lager to the grandparent (beer), so the lineage
	// survives minus the removed level — it does NOT orphan lager or leave a phantom 'ale-family'
	// rollup. This is the "remove a middle category" behaviour, and the direction a stale closure OR a
	// dangling-parent Ancestors would get wrong.
	if err := s.DeleteTaxon(ctx, "ale-family"); err != nil {
		t.Fatal(err)
	}
	// Confirm the reparent landed at the graph level before reindexing.
	if lg, ok := taxonomy.New(mustList(t, s, ctx)).Get("lager"); !ok || lg.Parent != "beer" {
		t.Fatalf("DeleteTaxon did not reparent lager to beer: got parent %q (ok=%v)", lg.Parent, ok)
	}
	taxa2 := mustList(t, s, ctx)
	if err := s.RebuildClosure(ctx, taxonomy.New(taxa2), now); err != nil {
		t.Fatal(err)
	}
	if err := s.RebuildRollups(ctx); err != nil {
		t.Fatal(err)
	}
	afterC, _ = s.GetClipTags(ctx, "clipC", false)
	// ale-family is GONE (reparented away, not a phantom ancestor); the rest of the lineage survives.
	assertSet(t, "clipC rollups after middle-taxon delete", afterC, []string{"lager", "beer", "alcohol", "drinks"})
	// lager itself (the asserted leaf) survives the graph edit.
	leavesC, _ := s.GetClipTags(ctx, "clipC", true)
	assertSet(t, "clipC leaf survives ancestor deletion", leavesC, []string{"lager"})
}

// mustList fetches the whole taxonomy or fails the test — a small helper for the reindex assertions
// that rebuild a forest from the live graph.
func mustList(t *testing.T, s Store, ctx context.Context) []taxonomy.Taxon {
	t.Helper()
	taxa, err := s.ListTaxa(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return taxa
}

// assertSet compares two string slices as SETS (order-independent), for the tag assertions above.
func assertSet(t *testing.T, what string, got, want []string) {
	t.Helper()
	gm := map[string]bool{}
	for _, g := range got {
		gm[g] = true
	}
	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v (set)", what, got, want)
		return
	}
	for _, w := range want {
		if !gm[w] {
			t.Errorf("%s = %v, missing %q (want %v)", what, got, w, want)
		}
	}
}

// testClipCounts pins the count queries against the listing they replaced.
//
// ⚠ Every assertion compares COUNT(*) against len(ListClips(sameFilter)) rather than a literal.
// That is the whole point: the counts exist to avoid materialising rows, so the only property
// worth pinning is that they still answer the SAME question as the listing. A hard-coded number
// would keep passing if the two predicates drifted apart, which is the one failure mode here.
func testClipCounts(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()

	seed := func(id, source string, held, autoFiled bool, era int) {
		c := sampleClip(id, id+".mp4", filler.Commercial, era, filler.Kids, "toys")
		c.Source = source
		c.Held = held
		c.AutoFiled = autoFiled
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	seed("a1", "youtube", false, false, 1990)
	seed("a2", "youtube", false, true, 1991)
	seed("a3", "archive", false, false, 0) // untagged: era 0
	seed("a4", "archive", true, false, 1992)
	seed("a5", "", false, false, 1993)

	filters := map[string]ClipFilter{
		"catalog":   {},
		"held":      {HeldOnly: true},
		"untagged":  {UntaggedOnly: true},
		"autofiled": {AutoFiledOnly: true},
		"by-kind":   {Kind: filler.Commercial},
	}
	for name, f := range filters {
		listed, err := s.ListClips(ctx, f)
		if err != nil {
			t.Fatalf("%s list: %v", name, err)
		}
		got, err := s.CountClips(ctx, f)
		if err != nil {
			t.Fatalf("%s count: %v", name, err)
		}
		if got != len(listed) {
			t.Errorf("CountClips(%s) = %d, but ListClips returned %d — the two predicates have drifted",
				name, got, len(listed))
		}
	}

	// AutoFiledOnly must actually narrow, or the assertion above passes vacuously against a
	// filter the WHERE builder ignores.
	if n, _ := s.CountClips(ctx, ClipFilter{AutoFiledOnly: true}); n != 1 {
		t.Errorf("auto-filed count = %d, want exactly the 1 seeded auto-filed clip", n)
	}

	// The per-source rollup must agree with the catalog total, or the Sources page's "N sources ·
	// M clips" line contradicts itself.
	bySource, err := s.CountClipsBySource(ctx, ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	sum := 0
	for _, n := range bySource {
		sum += n
	}
	total, _ := s.CountClips(ctx, ClipFilter{})
	if sum != total {
		t.Errorf("per-source counts sum to %d but the catalog holds %d — a clip vanished from the rollup", sum, total)
	}
	if bySource["youtube"] != 2 {
		t.Errorf("youtube = %d, want 2", bySource["youtube"])
	}
	// ⚠ The empty source must survive as its own bucket. `source` is free text, so an unknown or
	// blank value is possible, and dropping it would silently lose clips from a page whose whole
	// job is accounting for where they came from.
	if bySource[""] != 1 {
		t.Errorf("blank source = %d, want the 1 seeded clip — an unattributed clip must not vanish", bySource[""])
	}
	// Held is excluded by default, exactly as in the listing.
	if bySource["archive"] != 1 {
		t.Errorf("archive = %d, want 1 (the held clip is not in the catalog)", bySource["archive"])
	}
}

// testClipLicense pins that a clip's declared licence round-trips on BOTH backends, and that an
// absent one stays absent. ⚠ Empty means UNKNOWN, never "public domain" — ~92% of archive.org
// items declare none, so the empty case is the common one and must not acquire a default.
func testClipLicense(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	licensed := Clip{Clip: filler.Clip{
		Hash: "licensed.mp4",
		Path: "licensed.mp4", Name: "Licensed", Kind: filler.Commercial, DurationMs: 30000,
		License: "https://creativecommons.org/publicdomain/zero/1.0/",
	}, UpdatedAt: now}
	unknown := Clip{Clip: filler.Clip{
		Hash: "unknown.mp4",
		Path: "unknown.mp4", Name: "Unknown", Kind: filler.Commercial, DurationMs: 30000,
	}, UpdatedAt: now}
	for _, c := range []Clip{licensed, unknown} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.GetClip(ctx, "licensed.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if got.License != licensed.License {
		t.Errorf("licence = %q, want %q", got.License, licensed.License)
	}
	if got, err = s.GetClip(ctx, "unknown.mp4"); err != nil {
		t.Fatal(err)
	}
	if got.License != "" {
		t.Errorf("a clip with no declared licence has %q, want empty", got.License)
	}
}

// testClipHeld covers the V38 clip lifecycle on BOTH backends: a held clip is recorded but is not
// in the playable catalog, and only SetClipsHeld moves it.
//
// ⚠ The first assertion is the property the whole lifecycle rests on. Pod assembly, coverage, the
// filler-list builder and the catalog listing all read through ListClips with a zero filter, so if
// held clips were not excluded THERE, every untagged unreviewed download would air.
func testClipHeld(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	at := time.Now().UTC().Truncate(time.Second)

	filed := Clip{Clip: filler.Clip{
		Hash: "filed.mp4",
		Path: "filed.mp4", Name: "Filed", Kind: filler.Commercial, DurationMs: 30000,
		Era: 1990, Audience: filler.Kids, Category: "toys",
	}, UpdatedAt: at}
	held := Clip{Clip: filler.Clip{
		Hash: "held.mp4",
		Path: "held.mp4", Name: "Held", Kind: filler.Commercial, DurationMs: 30000,
		Era: 1990, Audience: filler.Kids, Category: "toys", Held: true, Confidence: 40,
	}, UpdatedAt: at}
	for _, c := range []Clip{filed, held} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	// ⚠ A ZERO filter is what pod assembly passes. A held clip must not be in this answer.
	got, err := s.ListClips(ctx, ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if c.Path == "held.mp4" {
			t.Fatal("a HELD clip came back from a zero-filter ListClips — pod assembly reads " +
				"exactly this, so an unreviewed clip would air")
		}
	}
	if len(got) != 1 {
		t.Fatalf("catalog has %d clips, want 1 (the filed one)", len(got))
	}

	// The review queue is the inverse, and it is how Incoming finds its work.
	queue, err := s.ListClips(ctx, ClipFilter{HeldOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].Path != "held.mp4" {
		t.Fatalf("HeldOnly returned %d clips, want just held.mp4", len(queue))
	}
	if queue[0].Confidence != 40 {
		t.Errorf("confidence = %d, want 40 — the score must round-trip", queue[0].Confidence)
	}

	// ⚠ THE trap this lifecycle has to survive: `clips` is a synced CACHE, so the folder scan
	// re-upserts every file it finds with held=false. If `held` rode along in UpsertClip's DO
	// UPDATE list, one scan pass would file every held clip — emptying the review queue into live
	// channels with no operator action and nothing in the logs.
	rescan := held
	rescan.Held = false
	rescan.Confidence = 0 // a scan knows nothing about tagging
	if err := s.UpsertClip(ctx, rescan); err != nil {
		t.Fatal(err)
	}
	after, err := s.ListClips(ctx, ClipFilter{HeldOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatal("a re-scan FILED a held clip — UpsertClip must omit `held` from its DO UPDATE " +
			"list, exactly as it omits the removal tombstone")
	}
	if after[0].Confidence != 40 {
		t.Errorf("a re-scan blanked the confidence score (%d) — it must be omitted too, or a "+
			"trusted clip starts asking again for no reason", after[0].Confidence)
	}

	// ⚠ **`SetClipConfidence` is the score's WRITER, and the assertions above cannot see whether
	// one exists.** They seed the value through `UpsertClip`'s INSERT and prove it round-trips and
	// survives a re-scan — all true, and all true for two phases while NOTHING in the application
	// ever wrote a score. `confidence` was 0 in every real catalog. So exercise the writer itself:
	// it is path-keyed like its `SetClipLanguage`/`SetClipTranscript` neighbours, and its value has
	// to outlive the folder scan the same way the seeded one does.
	if err := s.SetClipConfidence(ctx, "held.mp4", 92, at); err != nil {
		t.Fatal(err)
	}
	scored, err := s.ListClips(ctx, ClipFilter{HeldOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(scored) != 1 || scored[0].Confidence != 92 {
		t.Fatalf("SetClipConfidence did not write the score: %+v", scored)
	}
	if err := s.UpsertClip(ctx, rescan); err != nil {
		t.Fatal(err)
	}
	rescored, err := s.ListClips(ctx, ClipFilter{HeldOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rescored) != 1 || rescored[0].Confidence != 92 {
		t.Errorf("a re-scan blanked a WRITTEN score (%+v) — `confidence` must stay out of "+
			"UpsertClip's DO UPDATE list for the writer's value, not just the seeded one", rescored)
	}

	// Filing is the only way out, and it records that nobody looked.
	if _, err := s.SetClipsHeld(ctx, []string{"held.mp4"}, false, true, at); err != nil {
		t.Fatal(err)
	}
	catalog, err := s.ListClips(ctx, ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 2 {
		t.Fatalf("after filing, catalog has %d clips, want 2", len(catalog))
	}
	var flag bool
	for _, c := range catalog {
		if c.Path == "held.mp4" {
			flag = c.AutoFiled
		}
	}
	if !flag {
		t.Error("auto_filed did not survive — it is the only thing that can answer " +
			"'which of these did I never see?'")
	}
}

func testFillerSources(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)

	// ⚠ A FRESH install ships with fetchable sources (§10 V38c.8, migration 00034). Asserted here,
	// on BOTH backends, because a seed that lands on sqlite and not on postgres is exactly the
	// dialect drift this one-suite-two-backends rule exists to catch — and it would show up as
	// "filler mysteriously does nothing" on one deployment only.
	for _, want := range []struct{ id, label, uri string }{
		{"archive:classic_tv_commercials", "Classic TV Commercials", "classic_tv_commercials"},
		{"archive:vhscommercials", "Commercials From The Vault", "vhscommercials"},
		{"archive:tv_ads", "TV Ads", "tv_ads"},
	} {
		got, ok := findSource(t, s, want.id)
		if !ok {
			t.Fatalf("a fresh store is missing the seeded source %q — a new install cannot fetch", want.id)
		}
		// ⚠ The LABEL is human-readable. `vhscommercials` is not a name an operator recognises,
		// and the row renders the label above the target.
		if got.Label != want.label {
			t.Errorf("%s label = %q, want %q", want.id, got.Label, want.label)
		}
		if got.URI != want.uri {
			t.Errorf("%s uri = %q, want %q", want.id, got.URI, want.uri)
		}
		if !got.Enabled {
			t.Errorf("%s seeded switched OFF — it would sit in the UI doing nothing", want.id)
		}
		// ⚠ Fetchable, which is the whole point: `folder` and `library` are SCANNED, so before
		// this seed a fresh install had no source it could download from at all.
		if !got.Fetchable() {
			t.Errorf("%s is not fetchable — the seed exists so a new install CAN fetch", want.id)
		}
		// ⚠ EMPTY licence, and that is correct rather than missing data. All three declare none,
		// and §10 defines empty as UNKNOWN — never "public domain". A reassuring default here
		// would have Loomarr asserting a legal fact nobody checked.
		if got.License != "" {
			t.Errorf("%s licence = %q, want empty (unknown, NOT public domain)", want.id, got.License)
		}
	}

	// ⚠ YouTube seeds PRESENT BUT EMPTY. §10: Loomarr never recommends YouTube content itself, so
	// the operator brings the playlist — a seeded target would be that recommendation. The empty
	// uri also fails `Fetchable()`, which keeps the row out of every pull plan until it is filled
	// in; without that, approval would hand `Ingest` an empty string.
	if yt, ok := findSource(t, s, "youtube"); !ok {
		t.Error("a fresh store is missing the YouTube row — the mock draws it, unconfigured")
	} else {
		if yt.URI != "" {
			t.Errorf("youtube seeded with uri %q — Loomarr must not recommend a playlist", yt.URI)
		}
		if yt.Fetchable() {
			t.Error("an unconfigured youtube row is fetchable — a pull would ingest an empty string")
		}
	}

	created := time.Now().UTC().Truncate(time.Second)
	src := FillerSource{
		// Enabled explicitly: a Go bool zero-values to false, so a literal that omits it
		// describes a source that is switched OFF. Real add paths go through
		// NewFillerSource for exactly that reason.
		Enabled: true,
		// ⚠ NOT `classic_tv_commercials` — that is a SEEDED row now (00034), and 00032's unique
		// index on (kind, uri) correctly refuses a second row pointing at the same collection.
		// The fixture needs its own target; the index is doing its job.
		ID: "src-1", Kind: "archive", URI: "conformance_fixture_collection",
		Label:   "Classic TV commercials",
		License: "https://creativecommons.org/licenses/by-nc-sa/4.0/",
		// ⚠ Only ~8% of archive items declare a licence, so the empty case below is the
		// common one — both are covered.
		CreatedAt: created,
	}
	if err := s.UpsertFillerSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	unlicensed := FillerSource{ID: "src-2", Kind: "archive", URI: "vintage_ads", CreatedAt: created.Add(time.Second)}
	if err := s.UpsertFillerSource(ctx, unlicensed); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListFillerSources(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// ⚠ V37: the list is no longer empty on a fresh store. Migration 00029 materialises the two
	// CONFIG-BACKED singletons (`folder`, `library`) so the flat list can still say "you could
	// set up a drop-folder but have not" — §10's own answer to "why is my catalog empty?", which
	// a table of things-that-exist otherwise cannot express. So this suite asserts on the rows it
	// added, BY ID, rather than by position in the whole list.
	byID := map[string]FillerSource{}
	for _, f := range all {
		byID[f.ID] = f
	}
	for _, want := range []string{"folder", "library"} {
		if _, ok := byID[want]; !ok {
			t.Fatalf("singleton row %q missing — a fresh store must be able to say 'not configured'", want)
		}
	}
	if byID["folder"].URI != "" {
		t.Errorf("seeded folder URI = %q, want empty (= not configured, never a guessed path)", byID["folder"].URI)
	}

	// Ordering is still oldest-first and still explicit — an unordered list reshuffles between
	// reads on Postgres and the Sources tab's rows would move under the pointer. Checked over the
	// two rows this test added rather than the whole list, whose head is now seeded.
	//
	// ⚠ Filtered by ID, not by KIND. `Kind == "archive"` was unambiguous while every archive row
	// came from this test; migration 00034 seeds three of them, so the kind filter started
	// collecting the seed too and the count assertion below failed for the right reason.
	wanted := map[string]bool{"src-1": true, "src-2": true}
	var added []FillerSource
	for _, f := range all {
		if wanted[f.ID] {
			added = append(added, f)
		}
	}
	if len(added) != 2 {
		t.Fatalf("listed %d of this test's own sources, want 2", len(added))
	}
	if added[0].ID != "src-1" || added[1].ID != "src-2" {
		t.Errorf("order = %s,%s; want src-1,src-2 (oldest first)", added[0].ID, added[1].ID)
	}
	if added[0].License != src.License {
		t.Errorf("licence = %q, want %q", added[0].License, src.License)
	}
	if added[1].License != "" {
		t.Errorf("unlicensed source has licence %q, want empty (= unknown)", added[1].License)
	}
	if !added[0].LastFetchedAt.IsZero() {
		t.Errorf("a never-fetched source has LastFetchedAt %v, want zero", added[0].LastFetchedAt)
	}

	// ⚠ THE invariant the flat model has to carry itself (§10), MOVED in V38c from the kind to
	// the TARGET. 00029 allowed exactly one folder row; 00032 allows many, because commercials
	// living in two places is ordinary and V37 gave it no expression.
	//
	// What must still be impossible is ONE folder appearing as TWO rows — a stale row disagreeing
	// with another about the same directory, which is the precedence question 00023 refused to
	// have. So a DISTINCT path is accepted and a DUPLICATE path is refused, by the database
	// rather than by a Go guard the next caller forgets.
	second := FillerSource{ID: "folder-2", Kind: "folder", URI: "/other", Enabled: true, CreatedAt: created}
	if err := s.UpsertFillerSource(ctx, second); err != nil {
		t.Errorf("a second DISTINCT folder was refused (%v) — V38c allows many watched folders", err)
	}
	dup := FillerSource{ID: "folder-3", Kind: "folder", URI: "/other", Enabled: true, CreatedAt: created}
	if err := s.UpsertFillerSource(ctx, dup); err == nil {
		t.Error("a DUPLICATE folder path was accepted — one directory must not appear as two rows")
	}

	// ⚠ THE three-state encoding (§10 V38c). `nil` = inherit the global, `0` = never fetch this
	// source, `N` = every N seconds. They cannot collapse: `filler.fetch.every = 0` already means
	// "off", so a non-nullable column would make "unset" and "never" the same value and read as
	// "every existing source is switched off" on upgrade — 00026's mistake exactly.
	never, every900 := 0, 900
	if err := s.SetFillerSourceFetchPolicy(ctx, "src-2", &never, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFillerSourceFetchPolicy(ctx, "folder-2", &every900, &every900); err != nil {
		t.Fatal(err)
	}
	policies, err := s.ListFillerSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byPolicyID := map[string]FillerSource{}
	for _, f := range policies {
		byPolicyID[f.ID] = f
	}
	// src-1 was never given a policy: nil, and it must resolve to the global.
	if got := byPolicyID["src-1"]; got.FetchEverySeconds != nil {
		t.Errorf("src-1 override = %v, want nil (inherit)", *got.FetchEverySeconds)
	} else if d, ok := got.FetchEvery(time.Hour); !ok || d != time.Hour {
		t.Errorf("an un-overridden source resolved to (%v, %v), want the global hour", d, ok)
	}
	// src-2 was set to 0 = NEVER. ⚠ It must NOT read as "inherit" — that is the collapse.
	if got := byPolicyID["src-2"]; got.FetchEverySeconds == nil {
		t.Error("a 0 override round-tripped as NULL — 'never' collapsed into 'inherit'")
	} else if _, ok := got.FetchEvery(time.Hour); ok {
		t.Error("a 0 override resolved to a poll interval — 0 must mean NEVER")
	}
	if got := byPolicyID["folder-2"]; got.FetchEverySeconds == nil || *got.FetchEverySeconds != 900 {
		t.Errorf("folder-2 override did not round-trip: %v", got.FetchEverySeconds)
	} else if d, _ := got.FetchEvery(time.Hour); d != 900*time.Second {
		t.Errorf("resolved to %v, want 15m from the override rather than the global", d)
	}
	// Clearing goes back to inherit — a real operator action ("stop treating this specially").
	if err := s.SetFillerSourceFetchPolicy(ctx, "src-2", nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := findSource(t, s, "src-2"); got.FetchEverySeconds != nil {
		t.Error("clearing an override did not return the source to inherit")
	}

	// ⚠ Two BLANK-uri rows must both survive. A seeded-but-unconfigured row carries no target —
	// that is how "you could set up a drop-folder but have not" is expressed (§10) — and a plain
	// unique index rather than a partial one would allow only ONE blank row across the table,
	// so a fresh install could not have both an unconfigured folder and an unconfigured library.
	for _, blank := range []FillerSource{
		{ID: "blank-a", Kind: "folder", URI: "", Enabled: true, CreatedAt: created},
		{ID: "blank-b", Kind: "library", URI: "", Enabled: true, CreatedAt: created},
	} {
		if err := s.UpsertFillerSource(ctx, blank); err != nil {
			t.Errorf("an unconfigured %s row was refused (%v) — 'not configured' must stay expressible",
				blank.Kind, err)
		}
	}

	// Fetch stamps.
	fetched := created.Add(time.Hour)
	if err := s.MarkFillerSourceFetched(ctx, "src-1", fetched); err != nil {
		t.Fatal(err)
	}
	if !src1(t, s).LastFetchedAt.Equal(fetched) {
		t.Errorf("LastFetchedAt = %v, want %v", src1(t, s).LastFetchedAt, fetched)
	}

	// ⚠ THE assertion this table's ON CONFLICT clause exists for. Re-registering a source
	// (an operator fixing its label) knows nothing about fetches; if last_fetched_at joined
	// the DO UPDATE list, a working source would silently look like it had never run.
	relabelled := src
	relabelled.Label = "Renamed"
	if err := s.UpsertFillerSource(ctx, relabelled); err != nil {
		t.Fatal(err)
	}
	if src1(t, s).Label != "Renamed" {
		t.Errorf("label = %q, want Renamed", src1(t, s).Label)
	}
	if !src1(t, s).LastFetchedAt.Equal(fetched) {
		t.Errorf("re-registering reset LastFetchedAt to %v — it must survive an upsert", src1(t, s).LastFetchedAt)
	}

	// The Sources tab's on/off switch (V35). Two properties, each a claim the switch's own
	// copy makes to the operator.
	if !src1(t, s).Enabled {
		t.Error("source is not enabled — a registered source must be on until switched off")
	}
	if err := s.SetFillerSourceEnabled(ctx, "src-1", false); err != nil {
		t.Fatal(err)
	}
	if src1(t, s).Enabled {
		t.Error("source still enabled after being switched off")
	}

	// 1. ⚠ Disabling is NOT deleting. The row keeps its licence and its fetch history, which
	//    is what makes switching it back on restore what was there instead of starting over.
	if src1(t, s).License != src.License {
		t.Errorf("licence lost on disable: %q", src1(t, s).License)
	}
	if !src1(t, s).LastFetchedAt.Equal(fetched) {
		t.Error("fetch history lost on disable — the row was rewritten rather than updated")
	}

	// 2. ⚠ A re-register must not flip the switch back. `UpsertFillerSource` deliberately omits
	//    `enabled` from its DO UPDATE list, for the same reason last_fetched_at is omitted: a
	//    caller fixing a label knows nothing about the switch, and a Go bool zero-values to
	//    FALSE, so writing it would silently disable a source behind the operator's back. The
	//    first draft of V35 had exactly that bug.
	reRegistered := src
	reRegistered.Label = "Renamed again"
	reRegistered.Enabled = true // what a caller who does not know about the switch would send
	if err := s.UpsertFillerSource(ctx, reRegistered); err != nil {
		t.Fatal(err)
	}
	if src1(t, s).Enabled {
		t.Error("re-registering re-enabled a disabled source — the switch is not the upsert's business")
	}
	if src1(t, s).Label != "Renamed again" {
		t.Errorf("label = %q, want the re-registered one", src1(t, s).Label)
	}

	// Put it back on, so the delete assertions below run against the normal state.
	if err := s.SetFillerSourceEnabled(ctx, "src-1", true); err != nil {
		t.Fatal(err)
	}

	// ⚠ Deleting a source must NOT delete its clips: they are real files already tagged and
	// possibly pinned into a channel, and forgetting where something came from is not a
	// reason to throw it away.
	if err := s.UpsertClip(ctx, Clip{Clip: filler.Clip{
		Hash: "from-src-1.mp4",
		Path: "from-src-1.mp4", Name: "From src 1", Kind: filler.Commercial, DurationMs: 30000,
		Source: "archive", License: src.License,
	}, UpdatedAt: created}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFillerSource(ctx, "src-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetClip(ctx, "from-src-1.mp4"); err != nil {
		t.Errorf("deleting a source removed its clip: %v", err)
	}
	if _, ok := findSource(t, s, "src-1"); ok {
		t.Error("after delete, src-1 is still listed")
	}

	// An unknown id is ErrNotFound on both write paths, so a caller cannot believe it
	// recorded something.
	if err := s.DeleteFillerSource(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete unknown = %v, want ErrNotFound", err)
	}
	if err := s.MarkFillerSourceFetched(ctx, "nope", fetched); !errors.Is(err, ErrNotFound) {
		t.Errorf("mark unknown fetched = %v, want ErrNotFound", err)
	}
	if err := s.SetFillerSourceEnabled(ctx, "nope", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("set enabled on unknown = %v, want ErrNotFound", err)
	}
}

// testSeededDefaultSources covers what migration 00034 puts in a FRESH store, on BOTH backends
// (§10 V38c.8).
//
// ⚠ **This is the ONLY test that may depend on the seeded set.** Every other suite clears it —
// `newFillerServer` in internal/api does so explicitly — because eleven tests phrased as absolute
// counts ("want 1", "unconfigured") went red the moment the migration landed, none of them wrong
// about the behaviour they described. Concentrating the dependency here means the next change to
// the seed breaks exactly one test, and it breaks the one whose job is to notice.
//
// ⚠ It runs on both dialects because the seed is HAND-DUPLICATED per backend — two nearly
// identical SQL files, differing in `unixepoch()` vs its Postgres spelling. That is precisely the
// shape that drifts: a row added to one file and forgotten in the other produces a Postgres
// install that silently ships fewer sources than a SQLite one, and no single-dialect test can see
// it.
func testSeededDefaultSources(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)

	all, err := s.ListFillerSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]FillerSource{}
	for _, f := range all {
		byID[f.ID] = f
	}

	// The three VERIFIED archive collections (checked against the live API 2026-08-03 — five
	// plausible-looking identifiers returned zero items, which is why this list is short).
	for _, want := range []struct{ id, label string }{
		{"archive:classic_tv_commercials", "Classic TV Commercials"},
		{"archive:vhscommercials", "Commercials From The Vault"},
		{"archive:tv_ads", "TV Ads"},
	} {
		got, ok := byID[want.id]
		if !ok {
			t.Errorf("%s is missing — a fresh install must have something it can fetch", want.id)
			continue
		}
		if !got.Enabled {
			t.Errorf("%s is disabled; the seeded defaults are on so filler works on day one", want.id)
		}
		if !got.Fetchable() {
			t.Errorf("%s is not fetchable — a scanned-only default would leave the install stuck", want.id)
		}
		// ⚠ A human-readable name, not the identifier. `vhscommercials` is not something an
		// operator recognises in the Sources list.
		if got.Label != want.label {
			t.Errorf("%s label = %q, want %q", want.id, got.Label, want.label)
		}
		// ⚠ EMPTY licence, deliberately. ~92% of archive items declare none and §10 defines empty
		// as UNKNOWN, never "public domain" — a reassuring default here would have Loomarr
		// asserting a legal fact nobody checked.
		if got.License != "" {
			t.Errorf("%s license = %q, want empty (unknown) — absence carries no permission",
				want.id, got.License)
		}
	}

	// ⚠ The YouTube row is present but has NO target, and that is the design rather than an
	// oversight. §10 says Loomarr never recommends YouTube content itself; the operator brings
	// their own playlist. An empty uri also keeps the row out of every pull plan on its own,
	// because Fetchable() requires one.
	yt, ok := byID["youtube"]
	if !ok {
		t.Fatal("the youtube row is missing — the mock draws it as a present, empty prompt")
	}
	if yt.URI != "" {
		t.Errorf("youtube uri = %q, want empty — seeding a target IS the recommendation §10 forbids",
			yt.URI)
	}
	if yt.Fetchable() {
		t.Error("the empty youtube row is fetchable; it must not reach the ingest job until someone fills it in")
	}
}

// testFillerPulls covers the filler approval gate (§10 V35) on BOTH backends.
//
// The assertions that matter are the ones protecting the AUDIT: a decided pull is kept, and a
// dropped plan row is retained with its flag rather than removed. "We approved this" is only
// meaningful next to what was proposed.
func testFillerPulls(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	created := time.Now().UTC().Truncate(time.Second)

	p := filler.Pull{
		ID: "pull_1", Title: "Top up the 1990s", Reason: "Saturday Mornings falls back to bumpers.",
		ProposedBy: "admin-1", Status: filler.PullPending, CreatedAt: created,
		Plan: []filler.PullPlanRow{
			{SourceID: "classic", Tag: "1990s", Name: "Classic TV commercials", Why: "Era match", EstimateClips: 40},
			{SourceID: "psa", Tag: "psa", Name: "Public service", Why: "Filler variety", EstimateClips: 12},
		},
	}
	if err := s.UpsertPull(ctx, p); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetPull(ctx, "pull_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan) != 2 || got.Plan[0].SourceID != "classic" || got.Plan[0].EstimateClips != 40 {
		t.Errorf("plan did not round-trip: %+v", got.Plan)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, created)
	}
	// Pending means undecided, and that must be legible as a ZERO time rather than an epoch
	// date nobody meant.
	if !got.DecidedAt.IsZero() {
		t.Errorf("a pending pull has DecidedAt %v, want zero", got.DecidedAt)
	}
	if got.EstimatedClips() != 52 {
		t.Errorf("EstimatedClips = %d, want 52", got.EstimatedClips())
	}

	// Approve with one row dropped.
	decided := created.Add(time.Hour)
	got.Plan[1].Dropped = true
	got.Status = filler.PullApproved
	got.Note = "no local dealers"
	got.DecidedAt = decided
	got.DecidedBy = "admin-2"
	if err := s.UpsertPull(ctx, got); err != nil {
		t.Fatal(err)
	}

	after, err := s.GetPull(ctx, "pull_1")
	if err != nil {
		t.Fatal(err)
	}
	// ⚠ The dropped row is STILL THERE, flagged. Removing it would leave a record of what was
	// fetched with no record of what was declined, which is the half a reviewer needs.
	if len(after.Plan) != 2 {
		t.Fatalf("plan has %d rows after approval, want 2 — a dropped row must be retained", len(after.Plan))
	}
	if !after.Plan[1].Dropped {
		t.Error("the dropped flag did not persist")
	}
	if n := len(after.Committed()); n != 1 {
		t.Errorf("Committed() = %d rows, want 1", n)
	}
	if after.EstimatedClips() != 40 {
		t.Errorf("EstimatedClips after drop = %d, want 40", after.EstimatedClips())
	}
	if !after.DecidedAt.Equal(decided) || after.DecidedBy != "admin-2" || after.Note != "no local dealers" {
		t.Errorf("decision not recorded: %+v", after)
	}

	// Status filtering, and the fact that a decided pull is KEPT rather than deleted.
	if pending, err := s.ListPulls(ctx, filler.PullPending); err != nil || len(pending) != 0 {
		t.Errorf("pending = %d (%v), want 0", len(pending), err)
	}
	approved, err := s.ListPulls(ctx, filler.PullApproved)
	if err != nil || len(approved) != 1 {
		t.Fatalf("approved = %d (%v), want 1 — the history must survive the decision", len(approved), err)
	}
	if all, err := s.ListPulls(ctx, ""); err != nil || len(all) != 1 {
		t.Errorf("all = %d (%v), want 1", len(all), err)
	}

	if _, err := s.GetPull(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetPull unknown = %v, want ErrNotFound", err)
	}
}

// testSplitProposals covers the persisted split proposal (§10, V34) on BOTH backends: the
// segments JSON round-trip, ONE proposal per clip (re-detection replaces, and the new id
// wins), delete, and DeleteClip (the confirm path's drop of the compilation row).
func testSplitProposals(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	p := filler.SplitProposal{
		ID: "sp_1", ClipHash: clipHashFor("comps/1987.mp4"), CreatedAt: now,
		Segments: []filler.SplitSegment{
			{Index: 0, StartMs: 0, EndMs: 30000, Name: "comps/1987 part 1", Era: 1987, Audience: filler.Kids, Category: "toys"},
			{Index: 1, StartMs: 30000, EndMs: 61000, Name: "unknown", SuggestedEra: 1985, DupOf: "old/ad.mp4", Looked: true},
			{Index: 2, StartMs: 61000, EndMs: 149000, Name: "comps/1987 part 3", Unsplittable: true, Transcript: "[00:00] …"},
		},
	}
	if err := s.UpsertSplitProposal(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSplitProposal(ctx, "sp_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ClipHash != p.ClipHash || len(got.Segments) != 3 || !got.CreatedAt.Equal(now) {
		t.Fatalf("proposal round-trip = %+v", got)
	}
	// Every segment field survives the JSON round-trip — including the V34-specific
	// suggestion, dedup flag, and unsplittable marker the review renders.
	s1 := got.Segments[1]
	if s1.SuggestedEra != 1985 || s1.DupOf != "old/ad.mp4" || s1.Era != 0 || !s1.Looked {
		t.Errorf("segment suggestion/dedup fields lost: %+v", s1)
	}
	if !got.Segments[2].Unsplittable || got.Segments[2].Transcript == "" {
		t.Errorf("unsplittable marker/transcript lost: %+v", got.Segments[2])
	}

	draft := filler.SplitProposal{
		ID: "sp_draft", ClipHash: clipHashFor("comps/long.mp4"), CreatedAt: now.Add(time.Minute),
		Detection: &filler.SplitDetectionProgress{
			ScannedThroughMs: 600_000,
			Black:            []filler.Interval{{StartMs: 29_900, EndMs: 30_100}},
		},
	}
	if err := s.UpsertSplitProposal(ctx, draft); err != nil {
		t.Fatal(err)
	}
	gotDraft, err := s.GetSplitProposal(ctx, draft.ID)
	if err != nil || gotDraft.Ready() || gotDraft.Detection.ScannedThroughMs != 600_000 || len(gotDraft.Detection.Black) != 1 {
		t.Fatalf("detector checkpoint round-trip = (%+v, %v)", gotDraft, err)
	}
	if err := s.DeleteSplitProposal(ctx, draft.ID); err != nil {
		t.Fatal(err)
	}

	// ⚠ Re-detection REPLACES the pending proposal for the same clip — two competing
	// cut-lists for one file is a review bug, not a choice. The NEW id answers the old
	// one's GET with ErrNotFound.
	p2 := filler.SplitProposal{ID: "sp_2", ClipHash: p.ClipHash, CreatedAt: now.Add(time.Hour),
		Segments: []filler.SplitSegment{{Index: 0, StartMs: 0, EndMs: 149000, Name: "whole"}}}
	if err := s.UpsertSplitProposal(ctx, p2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSplitProposal(ctx, "sp_1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("stale proposal after re-detection = %v, want ErrNotFound", err)
	}
	got2, err := s.GetSplitProposal(ctx, "sp_2")
	if err != nil || len(got2.Segments) != 1 {
		t.Fatalf("replacement proposal = (%+v, %v)", got2, err)
	}

	// DeleteClip (confirm drops the compilation row) + proposal cleanup.
	if err := s.UpsertClip(ctx, Clip{Clip: clipAt("comps/1987.mp4", "1987", filler.Commercial, 149000), UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteClip(ctx, "comps/1987.mp4"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetClip(ctx, "comps/1987.mp4"); !errors.Is(err, ErrNotFound) {
		t.Errorf("compilation row survived DeleteClip: %v", err)
	}
	if err := s.DeleteClip(ctx, "comps/1987.mp4"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteClip twice = %v, want ErrNotFound", err)
	}
	if err := s.DeleteSplitProposal(ctx, "sp_2"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSplitProposal(ctx, "sp_2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteSplitProposal twice = %v, want ErrNotFound", err)
	}

	// --- UpdateSplitProposalSegments: grounding accumulates across passes (§10 V54) ---
	//
	// Split-time grounding is a read-modify-write spanning MINUTES of vision calls, so the write
	// races `Confirm`. Two properties are pinned here, and the second is the load-bearing one.
	p3 := filler.SplitProposal{
		ID: "sp_3", ClipHash: "h:comps/1991.mp4", CreatedAt: now,
		Segments: []filler.SplitSegment{
			{Index: 0, StartMs: 0, EndMs: 30_000, Name: "one"},
			{Index: 1, StartMs: 30_000, EndMs: 61_000, Name: "two"},
		},
	}
	if err := s.UpsertSplitProposal(ctx, p3); err != nil {
		t.Fatal(err)
	}

	// 1. The grounding fields round-trip — including `Looked`, which is what distinguishes
	// "looked at and found nothing" from "not reached yet". Inferring it from Category/Era would
	// make a resumable budget retry the ungroundable segments forever.
	grounded := []filler.SplitSegment{
		{Index: 0, StartMs: 0, EndMs: 30_000, Name: "one", Looked: true, Category: "toys", Era: 1991},
		{Index: 1, StartMs: 30_000, EndMs: 61_000, Name: "two", Looked: true},
	}
	if err := s.UpdateSplitProposalSegments(ctx, "sp_3", grounded); err != nil {
		t.Fatal(err)
	}
	got3, err := s.GetSplitProposal(ctx, "sp_3")
	if err != nil {
		t.Fatal(err)
	}
	if len(got3.Segments) != 2 {
		t.Fatalf("segments after update = %+v, want 2", got3.Segments)
	}
	if !got3.Segments[0].Looked || got3.Segments[0].Category != "toys" || got3.Segments[0].Era != 1991 {
		t.Errorf("grounding lost in round-trip: %+v", got3.Segments[0])
	}
	if !got3.Segments[1].Looked || got3.Segments[1].Category != "" {
		t.Errorf("segment 1 = %+v, want Looked with NO category — 'looked and found nothing'", got3.Segments[1])
	}
	// ⚠ `created_at` must be untouched: ListSplitProposals orders by it, so writing it would let a
	// reel jump the review queue merely for having been grounded.
	if !got3.CreatedAt.Equal(p3.CreatedAt) {
		t.Errorf("created_at moved on a grounding write: %v, want %v", got3.CreatedAt, p3.CreatedAt)
	}

	// 2. ⚠ **THE safety property: the update must NEVER insert.** If it were an upsert, a grounding
	// write landing after `Confirm` consumed the proposal would RESURRECT it — a pending review for
	// a reel already cut, pointing at a composite whose segments are in the catalog.
	if err := s.DeleteSplitProposal(ctx, "sp_3"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateSplitProposalSegments(ctx, "sp_3", grounded); !errors.Is(err, ErrNotFound) {
		t.Errorf("update after delete = %v, want ErrNotFound", err)
	}
	if _, err := s.GetSplitProposal(ctx, "sp_3"); !errors.Is(err, ErrNotFound) {
		t.Error("a grounding write RESURRECTED a confirmed proposal — the reel would be reviewed twice")
	}
	if err := s.UpdateSplitProposalSegments(ctx, "sp_never_existed", grounded); !errors.Is(err, ErrNotFound) {
		t.Errorf("update of an unknown id = %v, want ErrNotFound", err)
	}

	// --- The other side of the no-foreign-key independence: the PRUNE takes proposals too ---
	//
	// ⚠ `filler_split_proposals` is a sibling of `clips` with no FK, so nothing cleaned it up.
	// Measured 2026-08-11: deleting every clip file and running filler-sync pruned `clips` to 0
	// and left **48** proposals behind, which Incoming rendered as 48 "compilations to review"
	// titled with raw content hashes, each opening a review of a file that was gone.
	keeper := clipAt("comps/keeper.mp4", "Keeper", filler.Commercial, 149_000)
	orphan := clipAt("comps/orphan.mp4", "Orphan", filler.Commercial, 149_000)
	for _, c := range []filler.Clip{keeper, orphan} {
		if err := s.UpsertClip(ctx, Clip{Clip: c, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	seg := []filler.SplitSegment{{Index: 0, StartMs: 0, EndMs: 30_000}}
	for id, hash := range map[string]string{"sp_keep": keeper.Hash, "sp_orphan": orphan.Hash} {
		if err := s.UpsertSplitProposal(ctx, filler.SplitProposal{
			ID: id, ClipHash: hash, CreatedAt: now, Segments: seg,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// ⚠ A KEEPER is enrolled first on purpose, so the assertion distinguishes "pruned the orphan"
	// from "emptied the table" — a prune with a broken predicate passes the orphan check alone.
	if _, err := s.DeleteClipsNotIn(ctx, []string{keeper.Hash}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSplitProposal(ctx, "sp_orphan"); !errors.Is(err, ErrNotFound) {
		t.Errorf("orphan proposal survived the prune = %v; Incoming would render it as a "+
			"hash-titled reel pointing at a deleted compilation", err)
	}
	if _, err := s.GetSplitProposal(ctx, "sp_keep"); err != nil {
		t.Errorf("the prune took a LIVE proposal with it: %v", err)
	}
	// ⚠ Asserted on the LIST too, because that is the surface the defect was seen on.
	remaining, err := s.ListSplitProposals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].ID != "sp_keep" {
		t.Errorf("ListSplitProposals = %+v, want only sp_keep", remaining)
	}

	// --- the sweep's tombstone: a REAPED composite survives the prune (§10 V54) ---
	//
	// ⚠ **The cascade this prevents.** The split sweep deletes a spent recording on purpose, so the
	// next scan legitimately does not see it. Without the exemption `DeleteClipsNotIn` removes the
	// row — and every clip cut out of that reel carries `parent_hash` pointing at it, so one sweep
	// would dangle all of its children and take V45's lineage with it.
	reaped := clipAt("comps/reaped.mp4", "Reaped", filler.Commercial, 149_000)
	child := clipAt("cuts/child.mp4", "Child", filler.Commercial, 30_000)
	child.ParentHash = reaped.Hash
	for _, c := range []filler.Clip{reaped, child} {
		if err := s.UpsertClip(ctx, Clip{Clip: c, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkClipReaped(ctx, reaped.Hash, now); err != nil {
		t.Fatal(err)
	}

	// The scan now reports only the child — the reel's bytes are gone, which is the point.
	if _, err := s.DeleteClipsNotIn(ctx, []string{child.Hash}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetClip(ctx, reaped.Hash); err != nil {
		t.Errorf("a reaped composite was pruned (%v) — every clip cut from it now has a dangling "+
			"parent_hash", err)
	}

	// ⚠ …and the EMPTY-scan branch takes the same exemption. An unreadable drop folder is exactly
	// when a swept reel looks most like a deleted one.
	if _, err := s.DeleteClipsNotIn(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetClip(ctx, reaped.Hash); err != nil {
		t.Errorf("an empty scan pruned the reaped composite: %v", err)
	}
}

// testClipPipeline covers the per-clip ingest pipeline's state (§10 V51b) on BOTH backends: the
// ladder round-trip, the work-list's due/terminal filtering and total order, lazy enrolment, and —
// the property the whole table exists for — that a folder re-scan cannot touch it.
func testClipPipeline(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	clip := clipAt("a/b/one.mp4", "One", filler.Commercial, 30_000)
	if err := s.UpsertClip(ctx, Clip{Clip: clip, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	// --- Lazy enrolment: a catalogued clip with no row is work waiting to be picked up. ---
	missing, err := s.ListClipsWithoutPipeline(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0].Hash != clip.Hash {
		t.Fatalf("ListClipsWithoutPipeline = %+v, want just %s", missing, clip.Hash)
	}

	p := filler.ClipPipeline{
		ClipHash: clip.Hash, Stage: filler.StageTag, Status: filler.StatusRunning,
		Progress: 40, Disposition: filler.DispositionRunning,
		Attempts: 1, NextRun: now, EnrolledAt: now, UpdatedAt: now,
		Stages: []filler.StageRecord{
			{Stage: filler.StageProbe, Status: filler.StatusDone, At: now},
			{Stage: filler.StageTranscribe, Status: filler.StatusSkipped, Note: "the description already says enough", At: now},
		},
	}
	if err := s.UpsertClipPipeline(ctx, p); err != nil {
		t.Fatal(err)
	}

	// Enrolled clips drop out of the enrolment list — otherwise every pass would re-enrol
	// everything and reset the catalog to the start of the pipeline.
	if again, aerr := s.ListClipsWithoutPipeline(ctx, 10); aerr != nil || len(again) != 0 {
		t.Fatalf("an enrolled clip is still listed as missing: %+v (%v)", again, aerr)
	}

	got, found, err := s.GetClipPipeline(ctx, clip.Hash)
	if err != nil || !found {
		t.Fatalf("GetClipPipeline = (%+v, %v, %v)", got, found, err)
	}
	if got.Stage != filler.StageTag || got.Status != filler.StatusRunning || got.Progress != 40 || got.Attempts != 1 {
		t.Errorf("header round-trip lost fields: %+v", got)
	}
	// The LADDER is what the Incoming tab renders as history — including WHY a stage was skipped.
	// A skip with no note reads as "nothing happened", which is a different and false claim.
	if len(got.Stages) != 2 || got.Stages[1].Status != filler.StatusSkipped ||
		got.Stages[1].Note != "the description already says enough" {
		t.Errorf("ladder round-trip = %+v", got.Stages)
	}

	// --- An absent row is ordinary, not an error. An un-enrolled clip is the common case. ---
	if _, ok, gerr := s.GetClipPipeline(ctx, "no-such-clip"); gerr != nil || ok {
		t.Errorf("GetClipPipeline(missing) = (%v, %v), want (false, nil)", ok, gerr)
	}

	// --- The work list: due, non-terminal, oldest first. ---
	work, err := s.ListPipelineWork(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(work) != 1 || work[0].ClipHash != clip.Hash {
		t.Fatalf("work list = %+v, want the running clip", work)
	}
	// ⚠ A row backing off is NOT due. Without this the retry schedule is decorative: a failing
	// stage would be re-run on the very next pass, which is the "retried at full cost forever"
	// behaviour the backoff was added to stop.
	future := p
	future.NextRun = now.Add(time.Hour)
	if err := s.UpsertClipPipeline(ctx, future); err != nil {
		t.Fatal(err)
	}
	if backed, berr := s.ListPipelineWork(ctx, now, 10); berr != nil || len(backed) != 0 {
		t.Errorf("a backing-off row was returned as due: %+v (%v)", backed, berr)
	}

	// ⚠ A TERMINAL row is never work again, whatever its schedule says. `review` counts as
	// terminal here: the pipeline has done all it can and is waiting on a person, so re-running
	// the ladder would burn Whisper and vision calls on a clip whose only missing input is a
	// human decision.
	for _, d := range []filler.Disposition{filler.DispositionReview, filler.DispositionFiled, filler.DispositionRejected} {
		term := p
		term.Disposition = d
		term.NextRun = now.Add(-time.Hour) // overdue on purpose
		if err := s.UpsertClipPipeline(ctx, term); err != nil {
			t.Fatal(err)
		}
		if w, werr := s.ListPipelineWork(ctx, now, 10); werr != nil || len(w) != 0 {
			t.Errorf("disposition %q was returned as work: %+v (%v)", d, w, werr)
		}
	}

	// --- The rejected read model carries the CODE and the measured detail. ---
	rej := p
	rej.Disposition = filler.DispositionRejected
	rej.RejectReason = filler.ReasonTooShort
	rej.RejectDetail = "8.2s; floor is 10s"
	if err := s.UpsertClipPipeline(ctx, rej); err != nil {
		t.Fatal(err)
	}
	rejected, err := s.ListClipPipelines(ctx, filler.PipelineFilter{RejectedOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rejected) != 1 || rejected[0].RejectReason != filler.ReasonTooShort ||
		rejected[0].RejectDetail != "8.2s; floor is 10s" {
		t.Fatalf("rejected read model = %+v — the code AND the measured fact must both survive", rejected)
	}

	// --- ⚠ THE property this table exists for. ---
	//
	// `clips` is a synced CACHE: the folder scan re-upserts every file it finds. Pipeline state
	// records that ~341s of Whisper and a paid vision call have ALREADY been spent. If a re-scan
	// could touch these rows, one sync would re-run the whole catalog through the whole pipeline
	// and re-spend the money — which is precisely the class of failure `UpsertClip`'s DO UPDATE
	// omission list defends against by hand, one table over. Here it is structural: the scan does
	// not know this table exists.
	rescan := Clip{Clip: clip, UpdatedAt: now.Add(time.Minute)}
	if err := s.UpsertClip(ctx, rescan); err != nil {
		t.Fatal(err)
	}
	after, found, err := s.GetClipPipeline(ctx, clip.Hash)
	if err != nil || !found {
		t.Fatalf("a re-scan DELETED the pipeline row (%v, %v)", found, err)
	}
	if after.Stage != rej.Stage || after.Disposition != rej.Disposition ||
		after.RejectReason != rej.RejectReason || len(after.Stages) != len(rej.Stages) {
		t.Errorf("a re-scan altered pipeline state: %+v, want %+v", after, rej)
	}

	// --- ⚠ The other side of that independence: the PRUNE must take the rows with it. ---
	//
	// The sibling table has no foreign key, deliberately, so it survives a `clips` rebuild. The
	// price is that nothing else will ever clean it up — and an orphan row is not inert. It stays
	// in the work list, `advance` cannot find its clip, and it is re-tombstoned as "no longer in
	// the catalog" on every pass, forever. `DeleteClipsNotIn` is the one place clips disappear in
	// bulk, so it is the one place that has to prune them.
	//
	// A second clip is enrolled first, so the assertion distinguishes "pruned the orphan" from
	// "emptied the table".
	keeper := clipAt("c/d/two.mp4", "Two", filler.Commercial, 30_000)
	if err := s.UpsertClip(ctx, Clip{Clip: keeper, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	kept := filler.ClipPipeline{
		ClipHash: keeper.Hash, Stage: filler.StageProbe, Status: filler.StatusQueued,
		Disposition: filler.DispositionRunning, EnrolledAt: now, UpdatedAt: now,
	}
	if err := s.UpsertClipPipeline(ctx, kept); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DeleteClipsNotIn(ctx, []string{keeper.Hash}); err != nil {
		t.Fatal(err)
	}
	if _, orphan, oerr := s.GetClipPipeline(ctx, clip.Hash); oerr != nil || orphan {
		t.Errorf("the pruned clip's pipeline row survived (found=%v, %v) — it would be worked forever", orphan, oerr)
	}
	if _, stillThere, kerr := s.GetClipPipeline(ctx, keeper.Hash); kerr != nil || !stillThere {
		t.Errorf("the prune took a LIVE clip's pipeline row with it (found=%v, %v)", stillThere, kerr)
	}
}

// clipHashes is the identity projection the paging property compares on. ⚠ Hashes, not paths:
// `ids2` returns paths and the tie-break sorts on `hash`, so comparing paths would be comparing a
// column the ORDER BY never mentions.
func clipHashes(clips []Clip) []string {
	out := make([]string, len(clips))
	for i, c := range clips {
		out[i] = c.Hash
	}
	return out
}

func sameHashes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// testClipPaging covers the catalog's paging, sorting and widened search (§10 V51d) on BOTH
// backends.
//
// ⚠ **The centrepiece is the CONCATENATION PROPERTY**: for every sort key, both directions, at
// several page sizes, all pages concatenated must equal the unpaginated list exactly. One
// assertion catches three distinct bugs that are individually easy to miss — a missing tie-break
// (a row appears on two pages and another vanishes), an off-by-one offset, and a per-dialect
// collation difference — and it catches them on whichever backend has them.
//
// ⚠ **The fixture is load-bearing in two specific ways.** It contains deliberate TIES on every
// tieable column, because a total order is untestable against distinct values; and it contains
// CASE-MIXED names, because SQLite's BINARY collation puts 'Z' before 'a' while Postgres's locale
// collation does not — so a missing `LOWER()` fails on exactly one backend, which is the class
// `make test-pg` exists to catch.
func testClipPaging(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()

	// ⚠ Names deliberately straddle the case boundary. Under SQLite's BINARY collation every
	// capital sorts before every lowercase ('Z' = 90 < 'a' = 97); under Postgres's locale
	// collation they interleave. `LOWER(name)` is what makes the two agree, and these rows are
	// what prove it: an implementation without it orders "Zeppelin Ad" and "apple juice"
	// differently on the two backends while every other assertion here still passes.
	type row struct {
		id       string
		name     string
		duration int64
		plays    int64
		conf     int
		created  int64
	}
	rows := []row{
		{"p1", "apple juice", 30000, 5, 70, 1_700_000_100},
		{"p2", "Zeppelin Ad", 30000, 5, 70, 1_700_000_200}, // ties p1 on duration/plays/confidence
		{"p3", "banana split", 15000, 0, 10, 1_700_000_300},
		{"p4", "Banana bread", 15000, 0, 10, 1_700_000_300}, // ties p3 on EVERYTHING, created included
		{"p5", "cereal, morning", 60000, 12, 95, 1_700_000_400},
		{"p6", "Cereal, evening", 45000, 12, 95, 1_700_000_500},
		{"p7", "zzz last", 5000, 1, 0, 1_700_000_600},
		{"p8", "AAA first", 90000, 99, 100, 1_700_000_700},
		{"p9", "middle of the road", 30000, 5, 50, 1_700_000_800},
	}
	for _, r := range rows {
		c := sampleClip(r.id, r.name, filler.Commercial, 1990, filler.Kids, "toys")
		c.DurationMs = r.duration
		c.PlayCount = r.plays
		c.Confidence = r.conf
		c.CreatedAt = time.Unix(r.created, 0).UTC()
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	sorts := []ClipSort{"", ClipSortName, ClipSortDuration, ClipSortAdded, ClipSortPlays, ClipSortConfidence}

	t.Run("PagesConcatenateToTheWholeList", func(t *testing.T) {
		for _, sort := range sorts {
			for _, desc := range []bool{false, true} {
				full, err := s.ListClips(ctx, ClipFilter{Sort: sort, Desc: desc})
				if err != nil {
					t.Fatalf("sort %q desc=%v: %v", sort, desc, err)
				}
				want := clipHashes(full)
				for _, size := range []int{1, 2, 3, 4, 7, 100} {
					var got []string
					for offset := 0; ; offset += size {
						page, err := s.ListClips(ctx, ClipFilter{Sort: sort, Desc: desc, Limit: size, Offset: offset})
						if err != nil {
							t.Fatalf("sort %q desc=%v page@%d: %v", sort, desc, offset, err)
						}
						if len(page) == 0 {
							break
						}
						if len(page) > size {
							t.Fatalf("sort %q: page of %d rows exceeds limit %d", sort, len(page), size)
						}
						got = append(got, clipHashes(page)...)
						if offset > len(rows)*2 { // a paranoid stop, so a broken LIMIT cannot loop forever
							t.Fatalf("sort %q: paging did not terminate", sort)
						}
					}
					if !sameHashes(got, want) {
						t.Errorf("sort %q desc=%v size %d: pages concatenate to\n  %v\nbut the unpaginated list is\n  %v\n"+
							"— a duplicated or dropped row means the ORDER BY is not a TOTAL order",
							sort, desc, size, got, want)
					}
				}
			}
		}
	})

	t.Run("DescendingIsTheExactReverse", func(t *testing.T) {
		for _, sort := range sorts {
			asc, err := s.ListClips(ctx, ClipFilter{Sort: sort})
			if err != nil {
				t.Fatal(err)
			}
			desc, err := s.ListClips(ctx, ClipFilter{Sort: sort, Desc: true})
			if err != nil {
				t.Fatal(err)
			}
			a, d := clipHashes(asc), clipHashes(desc)
			for i := range a {
				if a[i] != d[len(d)-1-i] {
					t.Errorf("sort %q: descending is not the reverse of ascending (%v vs %v) — the tie-break "+
						"does not flip with the sort", sort, a, d)
					break
				}
			}
		}
	})

	// ⚠ THE dialect assertion. Without `LOWER(name)` this passes on one backend and fails on the
	// other, which is the entire reason the store conformance suite runs twice.
	t.Run("NameSortsCaseInsensitivelyOnBothBackends", func(t *testing.T) {
		got, err := s.ListClips(ctx, ClipFilter{Sort: ClipSortName})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"p8", "p1", "p4", "p3", "p6", "p5", "p9", "p2", "p7"}
		if !sameHashes(clipHashes(got), want) {
			t.Errorf("name order = %v, want %v — 'Zeppelin Ad' after 'middle of the road' and "+
				"'AAA first' before 'apple juice' only hold when the column is LOWER()ed",
				clipHashes(got), want)
		}
	})

	t.Run("UnknownSortIsAnErrorNotAFallback", func(t *testing.T) {
		if _, err := s.ListClips(ctx, ClipFilter{Sort: "; DROP TABLE clips"}); !errors.Is(err, ErrUnknownClipSort) {
			t.Errorf("unknown sort returned %v, want ErrUnknownClipSort — a silent fall-back to the "+
				"default order makes a broken sort control look like a UI glitch", err)
		}
	})

	t.Run("CountIgnoresLimitOffsetAndSort", func(t *testing.T) {
		total, err := s.CountClips(ctx, ClipFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if total != len(rows) {
			t.Fatalf("catalog total = %d, want %d", total, len(rows))
		}
		paged, err := s.CountClips(ctx, ClipFilter{Limit: 2, Offset: 4, Sort: ClipSortPlays, Desc: true})
		if err != nil {
			t.Fatal(err)
		}
		if paged != total {
			t.Errorf("CountClips with a page = %d, want %d — the total is 'how many match', not "+
				"'how many are on this page', or the pager reports 'showing 1-2 of 2'", paged, total)
		}
	})

	t.Run("OffsetPastTheEndIsEmpty", func(t *testing.T) {
		got, err := s.ListClips(ctx, ClipFilter{Sort: ClipSortName, Limit: 5, Offset: 500})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("offset past the end returned %d rows, want none", len(got))
		}
	})

	// ⚠ Offset with no limit must be IGNORED rather than emulated: sqlite rejects a bare OFFSET,
	// so an implementation that renders one errors on one backend only.
	t.Run("OffsetWithoutLimitIsIgnored", func(t *testing.T) {
		got, err := s.ListClips(ctx, ClipFilter{Offset: 3})
		if err != nil {
			t.Fatalf("offset with no limit: %v", err)
		}
		if len(got) != len(rows) {
			t.Errorf("got %d rows, want the whole catalog (%d) — a page with no size is not a page", len(got), len(rows))
		}
	})

	t.Run("HashesReadsExactlyTheAskedForClips", func(t *testing.T) {
		got, err := s.ListClips(ctx, ClipFilter{Hashes: []string{"p3", "p8", "not-a-clip"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d clips, want 2 (the unknown hash is simply absent, never an error)", len(got))
		}
		n, err := s.CountClips(ctx, ClipFilter{Hashes: []string{"p3", "p8", "not-a-clip"}})
		if err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Errorf("CountClips over the same hashes = %d, want 2 — the predicate is shared, so it cannot differ", n)
		}
	})
}

// testClipSearchWidened covers the V51d search across name | brand | visible_text | tags, and the
// opt-in transcript (§10 V51d).
//
// ⚠ Every case asserts CountClips alongside ListClips. They share `clipWhere` precisely so a
// search's total cannot disagree with its rows, and an EXISTS that was written as a JOIN would
// break exactly that — a clip with three matching tags counting three times.
func testClipSearchWidened(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()

	named := sampleClip("s1", "Crunchy Flakes 1994", filler.Commercial, 1994, filler.Kids, "cereal")
	branded := sampleClip("s2", "unnamed spot", filler.Commercial, 1994, filler.Kids, "cereal")
	branded.Brand = "Kellogg's"
	seen := sampleClip("s3", "silent visual", filler.Commercial, 1994, filler.Kids, "cereal")
	seen.VisibleText = "FORD TOUGH"
	spoken := sampleClip("s4", "chatty spot", filler.Commercial, 1994, filler.Kids, "cereal")
	spoken.Transcript = "you can afford a Ford this weekend"
	for _, c := range []Clip{named, branded, seen, spoken} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	// ⚠ A REAL slug from the seeded forest, tagged through the real writer — a hand-inserted
	// clip_tags row would not carry the rollups, and the search must match those too.
	if err := s.SetClipTags(ctx, "s1", []string{"cereal"}, taxonomy.New(mustList(t, s, ctx)), time.Unix(1_800_000_000, 0).UTC()); err != nil {
		t.Fatal(err)
	}

	hits := func(t *testing.T, f ClipFilter) []string {
		t.Helper()
		got, err := s.ListClips(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		n, err := s.CountClips(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		if n != len(got) {
			t.Errorf("CountClips = %d but ListClips returned %d for %+v — the shared predicate has drifted", n, len(got), f)
		}
		return clipHashes(got)
	}

	t.Run("MatchesName", func(t *testing.T) {
		if got := hits(t, ClipFilter{Query: "crunchy"}); !sameHashes(got, []string{"s1"}) {
			t.Errorf("name search = %v, want [s1]", got)
		}
	})
	t.Run("MatchesBrand", func(t *testing.T) {
		if got := hits(t, ClipFilter{Query: "kellogg"}); !sameHashes(got, []string{"s2"}) {
			t.Errorf("brand search = %v, want [s2] — a catalog that cannot find a clip by its "+
				"advertiser is a search box that looks broken", got)
		}
	})
	t.Run("MatchesVisibleText", func(t *testing.T) {
		if got := hits(t, ClipFilter{Query: "ford tough"}); !sameHashes(got, []string{"s3"}) {
			t.Errorf("visible-text search = %v, want [s3]", got)
		}
	})
	t.Run("MatchesTags", func(t *testing.T) {
		if got := hits(t, ClipFilter{Query: "cereal"}); !sameHashes(got, []string{"s1"}) {
			t.Errorf("tag search = %v, want [s1] exactly ONCE — a JOIN here would return the clip "+
				"per matching tag and make the total disagree with the rows", got)
		}
	})
	// ⚠ The transcript is opt-in, and this pair is what proves the flag does something. "afford"
	// contains "ford", which is also the noise argument for keeping it opt-in.
	t.Run("TranscriptOnlyWhenAskedFor", func(t *testing.T) {
		if got := hits(t, ClipFilter{Query: "weekend"}); len(got) != 0 {
			t.Errorf("default search matched the transcript (%v) — it is kilobytes per clip and the "+
				"noisiest column; it must be opt-in", got)
		}
		if got := hits(t, ClipFilter{Query: "weekend", QueryTranscript: true}); !sameHashes(got, []string{"s4"}) {
			t.Errorf("transcript search = %v, want [s4]", got)
		}
	})
}

// testClipTopLevelOnly pins the composite-as-container listing (§10 V51d).
//
// ⚠ The load-bearing half is the SECOND assertion: the ZERO filter must still return segments.
// TopLevelOnly is opt-in because pod assembly reads through that zero filter, and segments are
// the airable clips — an opt-out would delete every split-out advert from every channel's breaks.
func testClipTopLevelOnly(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()

	brk := sampleClip("brk", "KCPQ break 1996", filler.Commercial, 1996, filler.General, "")
	brk.IsComposite = true
	standalone := sampleClip("solo", "a single advert", filler.Commercial, 1996, filler.General, "toys")
	segA := sampleClip("seg-a", "advert 1", filler.Commercial, 1996, filler.General, "toys")
	segA.ParentHash = "brk"
	segB := sampleClip("seg-b", "advert 2", filler.Commercial, 1996, filler.General, "toys")
	segB.ParentHash = "brk"
	for _, c := range []Clip{brk, standalone, segA, segB} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	zero, err := s.ListClips(ctx, ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got := clipHashes(zero); !sameHashes(got, []string{"seg-a", "seg-b", "solo"}) {
		t.Errorf("zero filter = %v, want the two SEGMENTS and the standalone clip (the composite is "+
			"excluded, its segments are what air)", got)
	}

	top, err := s.ListClips(ctx, ClipFilter{TopLevelOnly: true, IncludeComposites: true, Sort: ClipSortName})
	if err != nil {
		t.Fatal(err)
	}
	if got := clipHashes(top); !sameHashes(got, []string{"solo", "brk"}) {
		t.Errorf("top-level listing = %v, want [solo brk] — a break paginates as ONE container row", got)
	}

	// Expanding a break loads its segments, and TopLevelOnly must not silently empty that.
	seg, err := s.ListClips(ctx, ClipFilter{ParentHash: "brk", TopLevelOnly: true, Sort: ClipSortName})
	if err != nil {
		t.Fatal(err)
	}
	if got := clipHashes(seg); !sameHashes(got, []string{"seg-a", "seg-b"}) {
		t.Errorf("lineage read = %v, want both segments — ParentHash wins over TopLevelOnly, or "+
			"expanding a break shows nothing on a break with twenty adverts in it", got)
	}
}

// testClipCreatedAt pins the "recently added" column's single-writer rule (§10 V51d, 00045).
//
// ⚠ The whole point of the column is that a RE-SYNC must not move it. If `created_at` rode
// UpsertClip's DO UPDATE list, every clip would read as "just added" after each folder scan and
// the sort would be worthless — a failure with no error and no log line.
func testClipCreatedAt(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()

	arrived := time.Unix(1_700_000_000, 0).UTC()
	c := sampleClip("ca1", "arrived once", filler.Commercial, 1994, filler.Kids, "toys")
	c.CreatedAt = arrived
	c.UpdatedAt = arrived
	if err := s.UpsertClip(ctx, c); err != nil {
		t.Fatal(err)
	}

	// A re-sync: same clip, a later UpdatedAt, and — as the folder scan does — no CreatedAt at all.
	rescanned := sampleClip("ca1", "arrived once", filler.Commercial, 1994, filler.Kids, "toys")
	rescanned.UpdatedAt = arrived.Add(48 * time.Hour)
	if err := s.UpsertClip(ctx, rescanned); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetClip(ctx, "ca1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(arrived) {
		t.Errorf("created_at = %v after a re-sync, want %v — the scan supplies a fresh timestamp "+
			"every pass, so this column must be absent from the DO UPDATE list", got.CreatedAt, arrived)
	}
	if !got.UpdatedAt.Equal(arrived.Add(48 * time.Hour)) {
		t.Errorf("updated_at = %v, want the re-sync's timestamp — it is the column that DOES ride", got.UpdatedAt)
	}

	// A writer that never heard of the column (every pre-V51d call site) still gets an honest
	// value rather than a 0 that sorts to the far end of "recently added" forever.
	fresh := sampleClip("ca2", "no created_at supplied", filler.Commercial, 1994, filler.Kids, "toys")
	fresh.UpdatedAt = arrived
	if err := s.UpsertClip(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	got2, err := s.GetClip(ctx, "ca2")
	if err != nil {
		t.Fatal(err)
	}
	if !got2.CreatedAt.Equal(arrived) {
		t.Errorf("created_at = %v with none supplied, want the UpdatedAt fallback (%v)", got2.CreatedAt, arrived)
	}
}
