package records

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/media/creative"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
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
// `store.SetClipsRemoved`), and `UpdateClipClassification`/`RecordClipPlay` were called with a path
// against a `WHERE hash = ?` — so the AI tagger aborted on its first clip and play counters never
// moved. Every assertion passed throughout, because a fixture where the two keys are equal cannot
// tell them apart. A key-confusion bug is invisible by construction against such a fixture.
//
// ⚠ Real 64-hex hashes are still deliberately NOT used. The store does not care what a hash looks
// like — only `filler.ClipPath` validates the shape, and that is tested where it belongs
// (`filler/clippath_test.go`). A wall of hex would cost readability without buying coverage. The
// property that matters is that the two fields are DISTINGUISHABLE, not that the hash is realistic,
// so the readable path gets a short readable hash beside it.
func clipAt(path, name string, kind filler.Role, durationMs int64) filler.Clip {
	return filler.Clip{Hash: clipHashFor(path), Path: path, Name: name, Role: kind, DurationMs: durationMs}
}

// clipHashFor derives a fixture clip's identity from its path, and clipPathFor derives a fixture
// clip's location from its identity. The two builders in this file start from opposite ends —
// `clipAt` is given a readable path, `sampleClip` a readable id — so each needs the other half.
//
// Both are deterministic (the suite asserts exact values and runs against two backends), readable
// in failure output, and — the load-bearing property — never equal to the value they derive from.
func clipHashFor(path string) string { return "h:" + path }

func clipPathFor(hash string) string { return "p/" + hash + ".mp4" }

func sampleClip(id, name string, kind filler.Role, era int, aud filler.Audience, cat string) Clip {
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
	// store methods became indistinguishable to this suite: `UpdateClipClassification` (`WHERE hash = ?`)
	// was being called with a path in production and every test still passed. See `clipAt` for
	// the full post-mortem. Keep the two fields distinct in every fixture, always.
	c.Hash = id
	c.Path = clipPathFor(id)
	c.TunarrProgramID = "tun-" + id
	c.Name = name
	c.Role = kind
	c.Era = era
	c.Audience = aud
	c.Category = cat
	c.DurationMs = 30000
	c.Source = "archive"
	c.UpdatedAt = time.Unix(1_700_000_000, 0).UTC()
	return c
}

// putClip upserts a sample clip and asserts its category as a taxonomy leaf. Category is derived
// from the tags on read (#2323), so a fixture's category exists only once it is tagged.
func putClip(t *testing.T, s Store, c Clip) {
	t.Helper()
	ctx := context.Background()
	if err := s.UpsertClip(ctx, c); err != nil {
		t.Fatal(err)
	}
	if c.Category != "" {
		if err := s.SetClipTags(ctx, c.Hash, []string{c.Category}); err != nil {
			t.Fatal(err)
		}
	}
}

// cachedClipFingerprint reads one clip's cached video frames under an algorithm.
func cachedClipFingerprint(ctx context.Context, s Store, clipHash, algorithm string) ([]uint64, bool, error) {
	all, err := s.ListClipFingerprints(ctx, algorithm)
	if err != nil {
		return nil, false, err
	}
	fp, found := all[clipHash]
	return fp.Video, found, nil
}

// frames is a fingerprint carrying only the given video frames.
func frames(video ...uint64) creative.Fingerprint {
	return creative.Fingerprint{DurationMs: 30_000, Video: video}
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

// exposuresBefore is a break's rotation snapshot the way pod assembly reads it: the household's
// stored airings, cut before `before` by the filler domain (a zero cutoff keeps everything).
func exposuresBefore(ctx context.Context, s Store, before time.Time, settle time.Duration) (map[string]pod.Exposure, error) {
	airings, err := s.FillerAirings(ctx)
	if err != nil {
		return nil, err
	}
	return pod.ExposuresBefore(pod.AiringHistory{Airings: airings, Settle: settle}, before), nil
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
	c1 := sampleClip("c1", "Crunchlings", filler.Commercial, 1992, filler.Kids, "cereal")
	c1.GeographicScope, c1.Country = pod.GeographicLocal, "US"
	c1.Network, c1.Station, c1.AirDate, c1.GeoEvidence = "Kestrel One", "fcc:900001", "1992-04-03", "filename: WKTL 1992-04-03"
	putClip(t, s, c1)
	putClip(t, s, sampleClip("c2", "Shell Squad figures", filler.Commercial, 1994, filler.Kids, "toys"))
	putClip(t, s, sampleClip("b1", "Bumper", filler.Bumper, 1992, filler.General, ""))
	putClip(t, s, sampleClip("u1", "untagged.mp4", filler.Commercial, 0, "", "")) // untagged

	// Round-trip.
	got, err := s.GetClip(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Crunchlings" || got.Role != filler.Commercial || got.Era != 1992 || got.Audience != filler.Kids || got.Category != "cereal" {
		t.Errorf("clip round-trip mismatch: %+v", got.Clip)
	}
	if got.DurationMs != 30000 {
		t.Errorf("duration lost: %d", got.DurationMs)
	}
	if got.GeographicScope != pod.GeographicLocal || got.Country != "US" ||
		got.Network != "Kestrel One" || got.Station != "fcc:900001" || got.AirDate != "1992-04-03" || got.GeoEvidence == "" {
		t.Errorf("geography round-trip mismatch: %+v", got.Clip)
	}
	if _, err := s.GetClip(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetClip(missing) = %v, want ErrNotFound", err)
	}

	// Filter by kind.
	comms, _ := s.ListClips(ctx, ClipFilter{Role: filler.Commercial})
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
	local, _ := s.ListClips(ctx, ClipFilter{GeographicScope: pod.GeographicLocal, Country: "us"})
	if len(local) != 1 || local[0].Hash != "c1" {
		t.Errorf("US local geography = %+v, want just c1", ids2(local))
	}
	if err := s.UpdateClipGeography(ctx, "c1", "national", "CA", "Loon Network", "", "", "operator", time.Now()); err != nil {
		t.Fatal(err)
	}
	updated, _ := s.GetClip(ctx, "c1")
	if updated.GeographicScope != pod.GeographicNational || updated.Country != "CA" || updated.GeoEvidence != "operator" {
		t.Errorf("updated geography = %+v", updated.Clip)
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

// testClipNeedsDetailsAgreesWithSQL pins the store's UntaggedOnly SQL to clipcatalog's Go
// NeedsDetails predicate, which feeds ClipDTO.needsDetails. If they drift, the Library's
// "Needs details" chip and its "Untagged only" filter disagree about the same clip.
//
// The verdict is taken from the STORED rows, so a column that does not round-trip cannot make the
// Go side agree by accident.
func testClipNeedsDetailsAgreesWithSQL(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	fixtures := []Clip{
		sampleClip("full", "full.mp4", filler.Commercial, 1991, filler.Kids, "toys"),
		sampleClip("no-era", "no-era.mp4", filler.Commercial, 0, filler.Kids, "toys"),
		sampleClip("no-aud", "no-aud.mp4", filler.Commercial, 1991, "", "toys"),
		sampleClip("no-cat", "no-cat.mp4", filler.Commercial, 1991, filler.Kids, ""),
		sampleClip("bare", "bare.mp4", filler.Commercial, 0, "", ""),
		sampleClip("bumper", "bumper.mp4", filler.Bumper, 0, "", ""),
		sampleClip("station", "station.mp4", filler.StationID, 0, "", ""),
		sampleClip("psa", "psa.mp4", filler.PSA, 1991, "", ""),
	}
	for _, c := range fixtures {
		putClip(t, s, c)
	}
	stored, err := s.ListClips(ctx, ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != len(fixtures) {
		t.Fatalf("stored %d clips, want %d", len(stored), len(fixtures))
	}
	want := map[string]bool{}
	for _, c := range stored {
		if c.NeedsDetails() {
			want[c.Hash] = true
		}
	}
	// Not vacuous: four commercials miss a tag, and no non-commercial ever needs details.
	if len(want) != 4 || want["full"] || want["bumper"] || want["station"] || want["psa"] {
		t.Fatalf("Go NeedsDetails = %v, want exactly no-era, no-aud, no-cat, bare", want)
	}
	untagged, err := s.ListClips(ctx, ClipFilter{UntaggedOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range untagged {
		got[c.Hash] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("UntaggedOnly SQL selected %v, Go NeedsDetails selected %v — the predicates have drifted", got, want)
	}
}

func testClipTagsAndPrune(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	_ = s.UpsertClip(ctx, sampleClip("u1", "untagged.mp4", filler.Commercial, 0, "", ""))
	_ = s.UpsertClip(ctx, sampleClip("keep", "keep.mp4", filler.Bumper, 1992, filler.General, ""))

	// Tag the untagged clip (the AI-tagging job path).
	if err := s.SetClipTags(ctx, "u1", []string{"cereal"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateClipClassification(ctx, "u1", 1994, "kids", now); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetClip(ctx, "u1")
	if got.Era != 1994 || got.Audience != filler.Kids || got.Category != "cereal" {
		t.Errorf("tag update didn't persist: %+v", got.Clip)
	}
	if !got.Tagged() {
		t.Error("clip should be Tagged() after update")
	}
	// Tagging a missing clip → ErrNotFound.
	if err := s.UpdateClipClassification(ctx, "gone", 1990, "kids", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateClipClassification(missing) = %v, want ErrNotFound", err)
	}

	// Prune: keep only "keep" — u1 is removed (it left the media server's library).
	n, err := s.DeleteClipsNotIn(ctx, []string{"keep"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("prune removed %d, want 1", n)
	}
	if _, err := s.GetClip(ctx, "u1"); !errors.Is(err, ErrNotFound) {
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
	putClip(t, s, sampleClip("c1", "Brightfield Crunchlings", filler.Commercial, 1992, filler.Kids, "cereal"))
	putClip(t, s, sampleClip("c2", "Shell Squad figures", filler.Commercial, 1994, filler.Kids, "toys"))
	putClip(t, s, sampleClip("c3", "100% Juice", filler.Commercial, 1993, filler.Kids, "drinks"))

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
	if got := names(ClipFilter{Query: "crunchlings"}); len(got) != 1 || got[0] != "Brightfield Crunchlings" {
		t.Errorf("Query=crunchlings → %v, want [Brightfield Crunchlings] (case-insensitive on both dialects)", got)
	}
	if got := names(ClipFilter{Query: "BRIGHTFIELD"}); len(got) != 1 {
		t.Errorf("Query=BRIGHTFIELD → %v, want 1 match", got)
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
	if got := names(ClipFilter{Query: "e", Category: "toys"}); len(got) != 1 || got[0] != "Shell Squad figures" {
		t.Errorf("Query+Category → %v, want [Shell Squad figures]", got)
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
	mustSaveChannel(t, s, sampleChannel("channel-1", 1, time.Time{}))

	c := sampleClip("1994/toys.mp4", "Shell Squad toys", filler.Commercial, 1994, filler.Kids, "toys")
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

	// ⚠ Read by HASH, not path. `GetClip`, `RecordClipPlay` and `UpdateClipClassification` are all keyed
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
		at := aired.Add(time.Duration(i) * time.Minute)
		if _, err := s.RecordClipPlay(ctx, "channel-1", c.Hash, at, at); err != nil {
			t.Fatalf("record play %d: %v", i, err)
		}
	}
	lastAired := aired.Add(2 * time.Minute)
	got, _ = s.GetClip(ctx, c.Hash)
	if got.PlayCount != 3 {
		t.Errorf("play count = %d, want 3", got.PlayCount)
	}
	if !got.LastPlayedAt.Equal(lastAired) {
		t.Errorf("last played = %v, want %v", got.LastPlayedAt, lastAired)
	}

	// A re-sync (what the periodic scan does) must leave the counters alone. Everything else
	// about the row is legitimately refreshed.
	c.Name = "Shell Squad toys (renamed)"
	if err := s.UpsertClip(ctx, c); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.Name != "Shell Squad toys (renamed)" {
		t.Errorf("a re-sync must refresh the name, got %q", got.Name)
	}
	if got.PlayCount != 3 {
		t.Errorf("a re-sync RESET the play count to %d — play_count must not be in the DO UPDATE list", got.PlayCount)
	}
	if !got.LastPlayedAt.Equal(lastAired) {
		t.Errorf("a re-sync reset last_played_at to %v", got.LastPlayedAt)
	}

	// Playout may resolve a clip the catalog has since pruned; that is telemetry missing a
	// row, not a playback error.
	if _, err := s.RecordClipPlay(ctx, "channel-1", "gone/missing.mp4", aired, aired); err != nil {
		t.Errorf("recording a play for a pruned clip must not error, got %v", err)
	}
}

// V58, #2017: rotation history is the household's, durable when a catalog row or a channel
// disappears, bounded per clip, and cut at a break's start and settle so an airing recorded while
// the break airs cannot reshuffle its tail.
func testClipExposureRotation(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	mustSaveChannel(t, s, sampleChannel("channel-a", 1, time.Time{}))
	mustSaveChannel(t, s, sampleChannel("channel-b", 2, time.Time{}))
	clip := sampleClip("rotation-hash", "rotation.mp4", filler.Commercial, 1994, filler.Kids, "toys")
	if err := s.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	first := time.UnixMilli(1_800_000_000_123).UTC()
	second := first.Add(2 * time.Second)
	ahead := func(at time.Time) time.Time { return at.Add(-12 * time.Second) } // playout's run-ahead
	if recorded, err := s.RecordClipPlay(ctx, "channel-a", clip.Hash, first, ahead(first)); err != nil {
		t.Fatal(err)
	} else if !recorded {
		t.Fatal("first scheduled start was not reported as a new airing")
	}
	// A resolver can ask about the active clip more than once. The scheduled start is stable,
	// so a repeated write for that start is one airing, not another viewer or another play.
	if recorded, err := s.RecordClipPlay(ctx, "channel-a", clip.Hash, first, first); err != nil {
		t.Fatal(err)
	} else if recorded {
		t.Fatal("duplicate scheduled start was reported as another airing")
	}
	if recorded, err := s.RecordClipPlay(ctx, "channel-b", clip.Hash, second, ahead(second)); err != nil {
		t.Fatal(err)
	} else if !recorded {
		t.Fatal("same clip on another channel was not reported as an independent airing")
	}
	if recorded, err := s.RecordClipPlay(ctx, "channel-a", clip.Hash, second, ahead(second)); err != nil {
		t.Fatal(err)
	} else if !recorded {
		t.Fatal("later scheduled start was not reported as a new airing")
	}

	all, err := exposuresBefore(ctx, s, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := all[clip.Hash]; got.PlayCount != 3 || !got.LastPlayedAt.Equal(second) {
		t.Fatalf("household exposure = %+v, want both channels' three airings, latest at %v", got, second)
	}
	// Supply sizes each era's pool from the channels its evenings were watched on (#2035).
	airings, err := s.FillerAirings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	onChannel := map[string]int{}
	for _, a := range airings {
		onChannel[a.ChannelID]++
	}
	if onChannel["channel-a"] != 2 || onChannel["channel-b"] != 1 {
		t.Fatalf("airings by channel = %v, want two on channel-a and one on channel-b", onChannel)
	}
	storedClip, err := s.GetClip(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if storedClip.PlayCount != 3 {
		t.Fatalf("global clip count = %d, want the three distinct channel/start airings", storedClip.PlayCount)
	}

	beforeBreak, err := exposuresBefore(ctx, s, second, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := beforeBreak[clip.Hash]; got.PlayCount != 1 || !got.LastPlayedAt.Equal(first) {
		t.Fatalf("break snapshot = %+v, want the history immediately before its cutoff", got)
	}
	// A tune-in on channel-b records, as the break airs, a clip that started before the break.
	tuneIn := sampleClip("tune-in-hash", "tune-in.mp4", filler.Commercial, 1994, filler.Kids, "toys")
	if _, err := s.RecordClipPlay(ctx, "channel-b", tuneIn.Hash, first.Add(-time.Minute), second.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	onAir, err := exposuresBefore(ctx, s, second, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := onAir[tuneIn.Hash]; ok {
		t.Fatalf("break snapshot = %+v counts an airing recorded after the break began", onAir)
	}

	// Duplicate callbacks for one scheduled start are idempotent, including when two replicas
	// race. This pins the upsert arithmetic on both SQLite and Postgres rather than relying on
	// process-local memory.
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.RecordClipPlay(ctx, "channel-a", clip.Hash, second, second)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent record: %v", err)
		}
	}
	all, err = exposuresBefore(ctx, s, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := all[clip.Hash]; got.PlayCount != 3 || !got.LastPlayedAt.Equal(second) {
		t.Fatalf("concurrent exposure = %+v, want three distinct channel/start airings", got)
	}

	// Every airing inside the history age is kept, however many channels aired the clip: a break on
	// air may count any of them, and supply sizes the pool from them.
	for i := 1; i <= 5; i++ {
		at := second.Add(time.Duration(i) * time.Hour)
		if _, err := s.RecordClipPlay(ctx, "channel-b", clip.Hash, at, ahead(at)); err != nil {
			t.Fatal(err)
		}
	}
	all, err = exposuresBefore(ctx, s, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := all[clip.Hash]; got.PlayCount != 8 {
		t.Fatalf("exposure within the history age = %+v, want all eight airings kept", got)
	}
	// Past the history age, only a clip's newest four are kept, so the history stays bounded.
	day := clips.AiringHistoryAge + time.Hour
	for i := 1; i <= 5; i++ {
		at := second.Add(time.Duration(i) * day)
		if _, err := s.RecordClipPlay(ctx, "channel-b", clip.Hash, at, ahead(at)); err != nil {
			t.Fatal(err)
		}
	}
	latest := second.Add(5 * day)
	all, err = exposuresBefore(ctx, s, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := all[clip.Hash]; got.PlayCount != 4 || !got.LastPlayedAt.Equal(latest) {
		t.Fatalf("trimmed exposure = %+v, want the newest four airings, latest at %v", got, latest)
	}
	// A break between the kept airings still sees the latest one before it.
	mid, err := exposuresBefore(ctx, s, second.Add(3*day+time.Minute), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := mid[clip.Hash]; got.PlayCount != 2 || !got.LastPlayedAt.Equal(second.Add(3*day)) {
		t.Fatalf("snapshot inside the kept airings = %+v, want the two before it", got)
	}
	// An airing older than all of those, written late, is not trimmed by its own write: resolving
	// it again is still one airing.
	for i, want := range []bool{true, false} {
		recorded, err := s.RecordClipPlay(ctx, "channel-c", clip.Hash, first, latest)
		if err != nil {
			t.Fatal(err)
		}
		if recorded != want {
			t.Fatalf("late write %d of one old airing reported recorded=%v, want %v", i+1, recorded, want)
		}
	}

	// A missing catalog row still records actual airing truth. Re-admission must retain it.
	missing := "removed-and-reready"
	if _, err := s.RecordClipPlay(ctx, "channel-a", missing, second, ahead(second)); err != nil {
		t.Fatal(err)
	}
	all, err = exposuresBefore(ctx, s, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := all[missing]; got.PlayCount != 1 || !got.LastPlayedAt.Equal(second) {
		t.Fatalf("pruned clip exposure = %+v, want durable actual airing", got)
	}
	restored := sampleClip(missing, "restored.mp4", filler.Commercial, 1994, filler.Kids, "toys")
	if err := s.UpsertClip(ctx, restored); err != nil {
		t.Fatal(err)
	}
	all, err = exposuresBefore(ctx, s, time.Time{}, 0)
	if err != nil || all[missing].PlayCount != 1 {
		t.Fatalf("re-admission lost exposure: %+v (err=%v)", all[missing], err)
	}
	// What the household saw on a channel it deleted, it still saw.
	channel, err := s.GetChannel(ctx, "channel-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteChannel(ctx, channel.ID, channel.Revision); err != nil {
		t.Fatal(err)
	}
	all, err = exposuresBefore(ctx, s, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if all[missing].PlayCount != 1 {
		t.Fatalf("deleting a channel dropped the household's airings: %+v", all)
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
	mustSaveChannel(t, s, sampleChannel("channel-1", 1, time.Time{}))

	c := sampleClip("k1", "keyed.mp4", filler.Commercial, 0, "", "")
	if c.Hash == c.Path {
		t.Fatalf("fixture bug: hash and path are equal (%q) — this test cannot distinguish the "+
			"two keys and neither can any other test in this file", c.Hash)
	}
	if err := s.UpsertClip(ctx, c); err != nil {
		t.Fatal(err)
	}

	// GetClip is hash-keyed.
	if _, err := s.GetClip(ctx, c.Path); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetClip(path) = %v, want ErrNotFound — GetClip is keyed WHERE hash = ?", err)
	}
	if _, err := s.GetClip(ctx, c.Hash); err != nil {
		t.Errorf("GetClip(hash) = %v, want it to resolve", err)
	}

	// UpdateClipClassification is hash-keyed, and its ErrNotFound is fatal to the tagging job.
	if err := s.UpdateClipClassification(ctx, c.Path, 1994, "kids", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateClipClassification(path) = %v, want ErrNotFound — the tagger must pass the hash", err)
	}
	if err := s.UpdateClipClassification(ctx, c.Hash, 1994, "kids", now); err != nil {
		t.Errorf("UpdateClipClassification(hash) = %v, want it to apply", err)
	}

	// RecordClipPlay is hash-keyed and deliberately silent on a miss, so assert the COUNTER
	// rather than the error — a path that no-ops returns nil either way.
	if _, err := s.RecordClipPlay(ctx, "channel-1", c.Path, now, now); err != nil {
		t.Errorf("RecordClipPlay(path) = %v, want a silent no-op", err)
	}
	got, _ := s.GetClip(ctx, c.Hash)
	if got.PlayCount != 0 {
		t.Errorf("play count = %d after recording against a PATH, want 0", got.PlayCount)
	}
	if _, err := s.RecordClipPlay(ctx, "channel-1", c.Hash, now, now); err != nil {
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
	// transcript writer, and it writes `brand` and nothing else.
	if err := s.SetClipBrand(ctx, c.Path, "Post", now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.Brand != "Post" {
		t.Errorf("brand = %q, want %q — SetClipBrand is keyed WHERE path = ?", got.Brand, "Post")
	}
	if got.VisibleText != "" {
		t.Errorf("visible text = %q after a TEXT brand write — SetClipBrand must not masquerade as a vision pass", got.VisibleText)
	}

	// ApplyClipVision records the on-screen text, a grounded brand, and (here) a taxonomy tag. era is
	// passed 0, so the era set by UpdateClipClassification above (1994) must SURVIVE — the CASE guard, not a
	// blanket overwrite. It overwrites the text-grounded brand above, which is fine — both are
	// grounded writers of the same column.
	if err := s.ApplyClipVision(ctx, c.Hash, c.Path, "Brightfield's", "BRIGHTFIELD'S CRUNCHLINGS", 0, []string{"cereal"}, now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.Brand != "Brightfield's" || got.VisibleText != "BRIGHTFIELD'S CRUNCHLINGS" {
		t.Errorf("vision facts = {brand:%q visible:%q}, want them recorded", got.Brand, got.VisibleText)
	}
	if got.Era != 1994 {
		t.Errorf("era = %d after a vision write with era=0, want 1994 preserved — the CASE guard must not blank an existing era", got.Era)
	}
	if got.Category != "cereal" || len(got.AssertedTags) != 1 || got.AssertedTags[0] != "cereal" {
		t.Errorf("vision taxonomy = category %q / asserted %v, want cereal / [cereal]", got.Category, got.AssertedTags)
	}
	if err := s.ApplyClipVision(ctx, c.Hash, c.Path, "Uncommitted", "SHOULD ROLL BACK", 0, []string{"not-a-live-taxon"}, now); !errors.Is(err, ErrTaxonConflict) {
		t.Fatalf("invalid vision taxonomy error = %v, want ErrTaxonConflict", err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.Brand != "Brightfield's" || got.VisibleText != "BRIGHTFIELD'S CRUNCHLINGS" {
		t.Errorf("invalid vision taxonomy partially committed facts: brand=%q text=%q", got.Brand, got.VisibleText)
	}

	// ⚠ The load-bearing property: a re-sync (UpsertClip) must NOT clobber the job-written columns.
	// The scan re-upserts every file it finds carrying a ZERO transcript/brand — if any of these
	// rode UpsertClip's DO UPDATE list, this upsert would wipe the work above and re-trigger Whisper
	// / a paid vision call on the next pass. This is the same discipline the language/held/counter
	// columns already pin; without it a green suite would hide the exact regression 00038 guards.
	resync := c
	resync.Transcript, resync.Brand, resync.VisibleText = "", "", ""
	if err := s.UpsertClip(ctx, resync); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, c.Hash)
	if got.Transcript != "buy our cereal today" {
		t.Errorf("transcript = %q after a re-sync, want it PRESERVED — UpsertClip must omit transcript from DO UPDATE", got.Transcript)
	}
	if got.Brand != "Brightfield's" || got.VisibleText != "BRIGHTFIELD'S CRUNCHLINGS" {
		t.Errorf("vision facts lost on re-sync {brand:%q visible:%q} — UpsertClip must omit them from DO UPDATE", got.Brand, got.VisibleText)
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

	old := sampleClip("old-content", "Burger Barn 1993", filler.Commercial, 1993, filler.Kids, "fast_food")
	old.ParentHash = "parent-reel"
	old.Held = true
	old.Transcript = "two all beef patties"
	if err := s.UpsertClip(ctx, old); err != nil {
		t.Fatal(err)
	}
	run := filler.AcquisitionRun{
		ID: "identity-acquisition", Trigger: filler.AcquisitionSource, SourceID: "youtube:ads",
		Status: filler.AcquisitionSuccess, Requested: 1, Fetched: 1,
		StartedAt: now, CompletedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertAcquisitionRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	artifact := filler.AcquisitionArtifact{
		ID: "identity-artifact", AcquisitionID: run.ID, SourceID: run.SourceID,
		Provider: "youtube", SourceURL: "https://www.youtube.com/watch?v=identity-artifact",
		RemoteID: "identity-artifact", MediaPath: old.Path,
		MediaSHA256: strings.Repeat("a", 64), MediaBytes: 42, ClipHash: old.Hash,
		State: filler.ArtifactConsumed, CompletedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertAcquisitionArtifacts(ctx, []filler.AcquisitionArtifact{artifact}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertClipFingerprint(ctx, old.Hash, "creative-test", frames(1, 2, 3)); err != nil {
		t.Fatal(err)
	}
	child := sampleClip("child", "Child", filler.Commercial, 1993, filler.Kids, "toys")
	child.ParentHash = old.Hash
	if err := s.UpsertClip(ctx, child); err != nil {
		t.Fatal(err)
	}

	if err := s.SetClipTags(ctx, old.Hash, []string{"cereal"}); err != nil {
		t.Fatal(err)
	}
	pipeline := pipelinestate.ClipPipeline{
		ClipHash: old.Hash, Stage: pipelinestate.StageTranscode, Status: pipelinestate.StatusRunning,
		Disposition: pipelinestate.DispositionRunning, EnrolledAt: now, UpdatedAt: now,
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
	channel, err := s.SaveChannel(ctx, channel)
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
		got.Transcript != old.Transcript || !got.Held {
		t.Errorf("metadata did not follow identity replacement: %+v", got)
	}
	if got.Path != replacement.Path || got.DurationMs != replacement.DurationMs || got.Quality != replacement.Quality {
		t.Errorf("transformed facts were not installed: %+v", got)
	}
	if len(got.Tags) == 0 {
		t.Error("taxonomy tags stayed behind on the old identity")
	}
	if row, found, err := s.GetClipPipeline(ctx, replacement.Hash); err != nil || !found || row.Stage != pipelinestate.StageTranscode {
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
	if _, found, err := cachedClipFingerprint(ctx, s, old.Hash, "creative-test"); err != nil || found {
		t.Errorf("old-byte fingerprint survived identity replacement: found=%v err=%v", found, err)
	}
	if _, found, err := cachedClipFingerprint(ctx, s, replacement.Hash, "creative-test"); err != nil || found {
		t.Errorf("old-byte fingerprint was re-keyed onto replacement bytes: found=%v err=%v", found, err)
	}
	gotArtifact, found, err := s.AcquisitionArtifactForClip(ctx, replacement.Path, replacement.Hash)
	if err != nil || !found || gotArtifact.ID != artifact.ID || gotArtifact.ClipHash != replacement.Hash {
		t.Errorf("acquisition provenance did not follow replacement: %+v, found=%v err=%v", gotArtifact, found, err)
	}
}

// A Ready clip has been screened, enriched and researched. When its bytes are re-encoded, that
// history is about the same content and moves with it, including the web-research ledger, whose
// foreign key to clips(hash) has no ON UPDATE and used to make the re-key fail outright (#2121).
func testReadyClipIdentityReplacement(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)

	old := sampleClip("ready-old-bytes", "Soda Splash 1991", filler.Commercial, 1991, filler.Kids, "drinks")
	old.Held, old.UpdatedAt = true, at
	if err := s.UpsertClip(ctx, old); err != nil {
		t.Fatal(err)
	}
	attempt := research.WebAttempt{ClipHash: old.Hash, InputRevision: 2,
		AdapterVersion: "web-v1:brave", ReservedAt: at}
	if _, err := s.ReserveFillerResearchWebRequest(ctx, "2026-11", research.WebProviderBrave, 5, attempt); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ApplyFillerEnrichment(ctx, enrichment.State{
		ClipHash: old.Hash, Axis: enrichment.AxisRole, Status: enrichment.StatusComplete,
		Value: enrichment.Value{Text: "commercial"},
		Evidence: enrichment.Evidence{Kind: enrichment.EvidenceItem, Reference: "item.title", Confidence: 100,
			Producer: enrichment.DeterministicProducer, ProducerVersion: enrichment.DeterministicProducerVersion,
			ObservedAt: at},
	}, at); err != nil {
		t.Fatal(err)
	}
	pipeline := pipelinestate.ClipPipeline{
		ClipHash: old.Hash, AcquisitionID: "ready-acquisition", Stage: pipelinestate.StageFinish,
		Status: pipelinestate.StatusRunning, Progress: 100, Disposition: pipelinestate.DispositionRunning,
		EnrolledAt: at, UpdatedAt: at,
	}
	for _, stage := range pipelinestate.StageOrder[:len(pipelinestate.StageOrder)-1] {
		pipeline.Stages = append(pipeline.Stages, pipelinestate.StageRecord{Stage: stage, Status: pipelinestate.StatusDone, At: at})
	}
	if err := s.UpsertClipPipeline(ctx, pipeline); err != nil {
		t.Fatal(err)
	}
	pipeline.Status, pipeline.Disposition = pipelinestate.StatusDone, pipelinestate.DispositionReady
	pipeline.Stages = append(pipeline.Stages, pipelinestate.StageRecord{Stage: pipelinestate.StageFinish, Status: pipelinestate.StatusDone, At: at})
	if err := s.CommitFillerReady(ctx, filler.ReadyCommit{Pipeline: pipeline, Event: filler.ReadyEvent{
		ID: "ready:" + old.Hash, ClipHash: old.Hash, AcquisitionID: pipeline.AcquisitionID,
		Enrollment: filler.Enrollment{Kind: filler.EnrollmentAcquisition, Reference: pipeline.AcquisitionID},
		Placement:  filler.PlacementBreakBody, CreatedAt: at,
	}}); err != nil {
		t.Fatal(err)
	}
	reencoded := old
	reencoded.Hash, reencoded.Path = "ready-new-bytes", "re/ad/ready-new-bytes.mp4"
	reencoded.UpdatedAt = at.Add(time.Hour)
	if err := s.ReplaceClipIdentity(ctx, old.Hash, reencoded); err != nil {
		t.Fatalf("re-key a researched Ready clip: %v", err)
	}

	moved := attempt
	moved.ClipHash = reencoded.Hash
	if _, err := s.ReserveFillerResearchWebRequest(ctx, "2026-11", research.WebProviderBrave, 5, moved); !errors.Is(err, research.ErrWebSearchAttempted) {
		t.Errorf("web-research attempt did not follow the re-key: reserve again = %v, want already attempted", err)
	}
	if axes, err := s.ListFillerEnrichment(ctx, reencoded.Hash); err != nil || len(axes) != 1 {
		t.Errorf("enrichment did not follow the re-key: %+v, %v", axes, err)
	}
	if axes, err := s.ListFillerEnrichment(ctx, old.Hash); err != nil || len(axes) != 0 {
		t.Errorf("enrichment stayed on the retired identity: %+v, %v", axes, err)
	}
	if event, found, err := s.GetFillerReadyEvent(ctx, reencoded.Hash); err != nil || !found || event.ID != "ready:"+old.Hash {
		t.Errorf("ready event did not follow the re-key: %+v, found=%v err=%v", event, found, err)
	}
}

// #2121: an airing clip switches to its re-encode in one transaction. The re-key drops the record
// measured on the old bytes and the replacement's takes its place under the new hash; a record
// the store refuses rolls the whole switch back, so the clip still airs under its old identity.
func testAiringClipReplacement(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)
	old := sampleClip("airing-old-bytes", "Soda Splash 1991", filler.Commercial, 1991, filler.Kids, "drinks")
	old.UpdatedAt = at
	if err := s.UpsertClip(ctx, old); err != nil {
		t.Fatal(err)
	}
	record := func(blur float64) clipstore.ClipDefectRecord {
		r := defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlurMean: blur}
		a := defects.Assess(r, old.Era)
		verdict, reason := defects.Decide(a, nil)
		return clipstore.ClipDefectRecord{ClipHash: old.Hash, Readings: r, Era: old.Era, Assessment: a,
			Verdict: verdict, VerdictReason: reason, MeasuredAt: at}
	}
	if err := s.UpsertClipDefects(ctx, record(4)); err != nil {
		t.Fatal(err)
	}
	reencoded := old
	reencoded.Hash, reencoded.Path = "airing-new-bytes", "ai/ri/airing-new-bytes.mp4"
	reencoded.UpdatedAt = at.Add(time.Hour)

	if err := s.ReplaceAiringClip(ctx, old.Hash, reencoded, clipstore.ClipDefectRecord{}); err == nil {
		t.Fatal("a switch with no defect verdict committed")
	}
	if _, err := s.GetClip(ctx, old.Hash); err != nil {
		t.Fatalf("a refused switch moved the clip: %v", err)
	}
	if _, found, err := s.GetClipDefects(ctx, old.Hash); err != nil || !found {
		t.Fatalf("a refused switch dropped the airing clip's record: found=%v %v", found, err)
	}

	replacement := record(3)
	if err := s.ReplaceAiringClip(ctx, old.Hash, reencoded, replacement); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.GetClipDefects(ctx, old.Hash); err != nil || found {
		t.Errorf("the old bytes' record outlived the switch: found=%v %v", found, err)
	}
	got, found, err := s.GetClipDefects(ctx, reencoded.Hash)
	if err != nil || !found || got.Readings.BlurMean != 3 || got.Verdict != replacement.Verdict {
		t.Errorf("new bytes' record = %+v found=%v %v", got, found, err)
	}
	if c, err := s.GetClip(ctx, reencoded.Hash); err != nil || c.Path != reencoded.Path {
		t.Errorf("clip after switch = %+v, %v", c, err)
	}
}

// A pending conditioned target may be reconstructed by Sync before the source row is re-keyed.
// The store adopts that exact held row and moves the source-owned graph in one transaction; the
// same operation then recognizes the target-only post-rekey state after a process restart.
func testConditioningPublicationCommit(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_900_634_000, 0).UTC()

	source := sampleClip("conditioning-source", "Reviewed child", filler.Commercial, 1993, filler.Kids, "food")
	source.ParentHash = "retained-parent"
	source.Transcript = "source-owned transcript"
	source.UpdatedAt = now
	if err := s.UpsertClip(ctx, source); err != nil {
		t.Fatal(err)
	}
	run := filler.AcquisitionRun{
		ID: "conditioning-acquisition", Trigger: filler.AcquisitionSource, SourceID: "youtube:ads",
		Status: filler.AcquisitionSuccess, Requested: 1, Fetched: 1,
		StartedAt: now, CompletedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertAcquisitionRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	artifact := filler.AcquisitionArtifact{
		ID: "conditioning-artifact", AcquisitionID: run.ID, SourceID: run.SourceID,
		Provider: "youtube", SourceURL: "https://www.youtube.com/watch?v=conditioning-artifact",
		RemoteID: "conditioning-artifact", MediaPath: source.Path,
		MediaSHA256: strings.Repeat("b", 64), MediaBytes: 84, ClipHash: source.Hash,
		State: filler.ArtifactConsumed, CompletedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertAcquisitionArtifacts(ctx, []filler.AcquisitionArtifact{artifact}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
		ClipHash: source.Hash, Stage: pipelinestate.StageTranscode, Status: pipelinestate.StatusRunning,
		Disposition: pipelinestate.DispositionRunning, EnrolledAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	target := source
	target.Hash = "conditioning-target"
	target.Path = clipPathFor(target.Hash)
	target.DurationMs = 30_033
	target.Quality = "480p"
	target.TunarrProgramID = ""
	reconstructed := target
	reconstructed.Name = "reconstructed from sidecar"
	reconstructed.Held = true
	reconstructed.Transcript = ""
	if err := s.UpsertClip(ctx, reconstructed); err != nil {
		t.Fatal(err)
	}
	publication := filler.ConditioningPublication{
		State: "pending", Owner: "0123456789abcdef0123456789abcdef",
		SourceHash: source.Hash, TargetHash: target.Hash,
	}
	if err := s.CommitConditioningPublication(ctx, publication, target); err != nil {
		t.Fatalf("adopt reconstructed target: %v", err)
	}
	if _, err := s.GetClip(ctx, source.Hash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source identity survived adoption: %v", err)
	}
	got, err := s.GetClip(ctx, target.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != source.Name || got.Transcript != source.Transcript || got.Held != source.Held {
		t.Fatalf("adopted target did not inherit source ownership: %+v", got)
	}
	if got.Path != target.Path || got.DurationMs != target.DurationMs || got.Quality != target.Quality {
		t.Fatalf("adopted target lost transformed facts: %+v", got)
	}
	if pipeline, found, err := s.GetClipPipeline(ctx, target.Hash); err != nil || !found || pipeline.Stage != pipelinestate.StageTranscode {
		t.Fatalf("adopted target pipeline = %+v, found=%v err=%v", pipeline, found, err)
	}
	gotArtifact, found, err := s.AcquisitionArtifactForClip(ctx, target.Path, target.Hash)
	if err != nil || !found || gotArtifact.ID != artifact.ID || gotArtifact.ClipHash != target.Hash {
		t.Fatalf("adopted target provenance = %+v, found=%v err=%v", gotArtifact, found, err)
	}
	if err := s.CommitConditioningPublication(ctx, publication, got); err != nil {
		t.Fatalf("recognize post-rekey target: %v", err)
	}
}

// A sidecar's pending publication owns exactly one source/target pair. Any malformed marker or
// catalog shape outside the three recovery states must leave both the source graph and any
// reconstructed target untouched for review; guessing which row to retire would lose evidence.
func testConditioningPublicationFailsClosed(t *testing.T, newStore NewStoreFunc) {
	for _, tc := range []struct {
		name          string
		mutateMarker  func(*filler.ConditioningPublication, *Clip)
		mutateSource  func(*Clip)
		mutateTarget  func(*Clip)
		withTargetRow bool
		heldTarget    bool
	}{
		{
			name: "owner is absent",
			mutateMarker: func(publication *filler.ConditioningPublication, _ *Clip) {
				publication.Owner = ""
			},
		},
		{
			name: "marker target differs from staged target",
			mutateMarker: func(publication *filler.ConditioningPublication, _ *Clip) {
				publication.TargetHash = "other-conditioned-target"
			},
		},
		{
			name: "marker collapses source and target ownership",
			mutateMarker: func(publication *filler.ConditioningPublication, _ *Clip) {
				publication.TargetHash = publication.SourceHash
			},
		},
		{
			name: "source no longer matches staged lineage",
			mutateSource: func(source *Clip) {
				source.ParentHash = "different-retained-parent"
			},
		},
		{
			name:          "target exists without Sync quarantine",
			withTargetRow: true,
		},
		{
			name:          "held target does not match pending evidence",
			withTargetRow: true,
			heldTarget:    true,
			mutateTarget: func(target *Clip) {
				target.Path = "wrong/conditioned-target.mp4"
			},
		},
		{
			name:          "target-only state is not exact",
			withTargetRow: true,
			heldTarget:    true,
			mutateSource: func(source *Clip) {
				source.Hash = "absent-conditioning-source"
			},
			mutateTarget: func(target *Clip) {
				target.Held = true
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t)
			ctx := context.Background()
			now := time.Unix(1_900_634_100, 0).UTC()

			source := sampleClip("conditioning-ambiguous-source", "Reviewed child", filler.Commercial, 1993, filler.Kids, "food")
			source.ParentHash = "retained-parent"
			source.UpdatedAt = now
			target := source
			target.Hash = "conditioning-ambiguous-target"
			target.Path = clipPathFor(target.Hash)
			target.DurationMs = 30_033
			target.Quality = "480p"
			publication := filler.ConditioningPublication{
				State: "pending", Owner: "0123456789abcdef0123456789abcdef",
				SourceHash: source.Hash, TargetHash: target.Hash,
			}
			if tc.mutateMarker != nil {
				tc.mutateMarker(&publication, &target)
			}
			if tc.mutateSource != nil {
				tc.mutateSource(&source)
			}
			if source.Hash != publication.SourceHash {
				// The target-only state deliberately does not install a source catalog row.
			} else if err := s.UpsertClip(ctx, source); err != nil {
				t.Fatal(err)
			}
			if source.Hash == publication.SourceHash {
				if err := s.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
					ClipHash: publication.SourceHash, Stage: pipelinestate.StageTranscode, Status: pipelinestate.StatusRunning,
					Disposition: pipelinestate.DispositionRunning, EnrolledAt: now, UpdatedAt: now,
				}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.withTargetRow {
				reconstructed := target
				reconstructed.Held = tc.heldTarget
				if tc.mutateTarget != nil {
					tc.mutateTarget(&reconstructed)
				}
				if err := s.UpsertClip(ctx, reconstructed); err != nil {
					t.Fatal(err)
				}
			}

			if err := s.CommitConditioningPublication(ctx, publication, target); !errors.Is(err, ErrConditioningPublicationMismatch) {
				t.Fatalf("CommitConditioningPublication error = %v, want ErrConditioningPublicationMismatch", err)
			}
			if source.Hash == publication.SourceHash {
				got, err := s.GetClip(ctx, publication.SourceHash)
				if err != nil || got.Hash != source.Hash || got.ParentHash != source.ParentHash || got.Path != source.Path {
					t.Fatalf("ambiguous publication changed source evidence: %+v, %v", got, err)
				}
				if pipeline, found, err := s.GetClipPipeline(ctx, publication.SourceHash); err != nil || !found || pipeline.Stage != pipelinestate.StageTranscode {
					t.Fatalf("ambiguous publication lost source reference: %+v, found=%v, err=%v", pipeline, found, err)
				}
			}
			if tc.withTargetRow {
				got, err := s.GetClip(ctx, target.Hash)
				if err != nil || got.Held != tc.heldTarget {
					t.Fatalf("ambiguous publication changed reconstructed target: %+v, %v", got, err)
				}
			}
		})
	}
}

// A transform may legitimately preserve the content hash: remuxing an already-normalized clip
// can produce byte-for-byte identical output. That is an update to transformed facts, not a
// re-key, and must not run self-referential updates through unique sibling keys.
func testClipIdentityReplacementSameHash(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0).UTC()

	clip := sampleClip("unchanged-content", "Already normalized", filler.Commercial, 1995, filler.General, "")
	clip.Path = "un/ch/unchanged-content.mkv"
	clip.TunarrProgramID = "old-program"
	if err := s.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertClipFingerprint(ctx, clip.Hash, "creative-test", frames(1, 2, 3)); err != nil {
		t.Fatal(err)
	}
	pipeline := pipelinestate.ClipPipeline{
		ClipHash: clip.Hash, Stage: pipelinestate.StageTranscode, Status: pipelinestate.StatusRunning,
		Disposition: pipelinestate.DispositionRunning, EnrolledAt: now, UpdatedAt: now,
	}
	if err := s.UpsertClipPipeline(ctx, pipeline); err != nil {
		t.Fatal(err)
	}

	updated := clip
	updated.Path = "un/ch/unchanged-content.mp4"
	updated.DurationMs = 30_033
	updated.Quality = "480p"
	updated.UpdatedAt = now.Add(time.Minute)
	if err := s.ReplaceClipIdentity(ctx, clip.Hash, updated); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetClip(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != updated.Path || got.DurationMs != updated.DurationMs || got.Quality != updated.Quality {
		t.Errorf("same-identity transformed facts = %+v, want path %q duration %d quality %q",
			got, updated.Path, updated.DurationMs, updated.Quality)
	}
	if got.TunarrProgramID != "" {
		t.Errorf("Tunarr program id = %q, want cleared after path change", got.TunarrProgramID)
	}
	// Once Tunarr knows the canonical path, an idempotent retry at that same path must not
	// manufacture registration work. This also pins the CASE expression to the pre-update path
	// semantics shared by SQLite and Postgres.
	got.TunarrProgramID = "canonical-program"
	if err := s.UpsertClip(ctx, got); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceClipIdentity(ctx, got.Hash, got); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetClip(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.TunarrProgramID != "canonical-program" {
		t.Errorf("same-path retry cleared Tunarr program id: %q", got.TunarrProgramID)
	}
	if row, found, err := s.GetClipPipeline(ctx, clip.Hash); err != nil || !found || row.Stage != pipelinestate.StageTranscode {
		t.Errorf("same-identity pipeline changed: %+v, found=%v err=%v", row, found, err)
	}
	if cached, found, err := cachedClipFingerprint(ctx, s, clip.Hash, "creative-test"); err != nil || !found || len(cached) != 3 {
		t.Errorf("fingerprint for unchanged bytes = (%v, %v, %v), want preserved", cached, found, err)
	}
}

func testClipPipelineOverview(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0).UTC()
	rows := []pipelinestate.ClipPipeline{
		{ClipHash: "due", Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusQueued,
			Disposition: pipelinestate.DispositionRunning, NextRun: now.Add(-time.Minute)},
		{ClipHash: "active", Stage: pipelinestate.StageTranscode, Status: pipelinestate.StatusRunning,
			Disposition: pipelinestate.DispositionRunning},
		{ClipHash: "backoff", Stage: pipelinestate.StageVision, Status: pipelinestate.StatusFailed,
			Disposition: pipelinestate.DispositionRunning, NextRun: now.Add(time.Hour)},
		{ClipHash: "deferred", Stage: pipelinestate.StageSplit, Status: pipelinestate.StatusQueued,
			Disposition: pipelinestate.DispositionRunning, NextRun: now.Add(time.Hour)},
		{ClipHash: "review", Stage: pipelinestate.StageFinish, Status: pipelinestate.StatusDone,
			Disposition: pipelinestate.DispositionReview},
		{ClipHash: "ready", Stage: pipelinestate.StageFinish, Status: pipelinestate.StatusDone,
			Disposition: pipelinestate.DispositionReady},
		{ClipHash: "complete", Stage: pipelinestate.StageFinish, Status: pipelinestate.StatusDone,
			Disposition: pipelinestate.DispositionComplete},
		{ClipHash: "rejected", Stage: pipelinestate.StageTranscode, Status: pipelinestate.StatusFailed,
			Disposition: pipelinestate.DispositionRejected, RejectReason: pipelinestate.ReasonUnplayable},
		{ClipHash: "dismissed", Stage: pipelinestate.StageVision, Status: pipelinestate.StatusDone,
			Disposition: pipelinestate.DispositionDismissed},
	}
	for i := range rows {
		rows[i].EnrolledAt, rows[i].UpdatedAt = now, now
		if err := s.UpsertClipPipeline(ctx, rows[i]); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.PipelineOverview(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	want := pipelinestate.PipelineOverview{
		Runnable: 1, InProgress: 1, Scheduled: 2, NeedsDecision: 1,
		Ready: 1, Complete: 1, Rejected: 1, Dismissed: 1, Recoverable: 1,
	}
	if got != want {
		t.Fatalf("PipelineOverview() = %+v, want %+v", got, want)
	}
}

func testFillerAcquisitionRuns(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0).UTC()
	older := filler.AcquisitionRun{
		ID: "acq-old", Trigger: filler.AcquisitionScheduled, SourceID: "archive:classic",
		Status: filler.AcquisitionRunning, Requested: 3, StartedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	newer := filler.AcquisitionRun{
		ID: "acq-new", Trigger: filler.AcquisitionPull, SourceID: "archive:vault", PullID: "pull-7",
		Status: filler.AcquisitionSuccess, Requested: 4, Fetched: 2, Skipped: 1, Failed: 1,
		StartedAt: now.Add(-time.Minute), CompletedAt: now, UpdatedAt: now,
	}
	// Establish the pull-bound run through the same durable approval boundary.
	approval := filler.Pull{ID: newer.PullID, Status: filler.PullPending, CreatedAt: newer.StartedAt,
		Plan: []filler.PullPlanRow{{SourceID: "archive:vault"}, {SourceID: "archive:vault"}, {SourceID: "archive:vault"}, {SourceID: "archive:vault"}}}
	if err := s.UpsertPull(ctx, approval); err != nil {
		t.Fatal(err)
	}
	approval.Status, approval.DecidedAt = filler.PullApproved, newer.StartedAt
	initial := filler.AcquisitionRun{ID: newer.ID, Trigger: newer.Trigger, SourceID: newer.SourceID, PullID: newer.PullID, Status: filler.AcquisitionQueued, Requested: newer.Requested, StartedAt: newer.StartedAt, UpdatedAt: newer.StartedAt}
	if err := s.CommitPullApproval(ctx, approval, initial); err != nil {
		t.Fatal(err)
	}
	for _, run := range []filler.AcquisitionRun{older, newer} {
		if err := s.UpsertAcquisitionRun(ctx, run); err != nil {
			t.Fatal(err)
		}
	}
	queued := filler.AcquisitionRun{
		ID: "acq-queued", Trigger: filler.AcquisitionManual, Status: filler.AcquisitionQueued,
		Requested: 1, StartedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour),
	}
	if err := s.UpsertAcquisitionRun(ctx, queued); err != nil {
		t.Fatal(err)
	}
	pipelines := []pipelinestate.ClipPipeline{
		{ClipHash: "preparing", AcquisitionID: newer.ID, Stage: pipelinestate.StageVision, Status: pipelinestate.StatusRunning,
			Disposition: pipelinestate.DispositionRunning, EnrolledAt: now, UpdatedAt: now},
		{ClipHash: "review", AcquisitionID: newer.ID, Stage: pipelinestate.StageFinish, Status: pipelinestate.StatusDone,
			Disposition: pipelinestate.DispositionReview, EnrolledAt: now, UpdatedAt: now},
		{ClipHash: "ready", AcquisitionID: newer.ID, Stage: pipelinestate.StageFinish, Status: pipelinestate.StatusDone,
			Disposition: pipelinestate.DispositionReady, EnrolledAt: now, UpdatedAt: now},
		{ClipHash: "rejected", AcquisitionID: newer.ID, Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusFailed,
			Disposition: pipelinestate.DispositionRejected, EnrolledAt: now, UpdatedAt: now},
	}
	for _, row := range pipelines {
		if err := s.UpsertClipPipeline(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	// #749: "ready" was downloaded for a coverage gap and "rejected" was split out of it, so both
	// count toward that gap; "review" was an unsteered download of the same run.
	for _, a := range []filler.AcquisitionArtifact{
		{ID: "gap-art", ClipHash: "ready", Gap: "era:1990-1999", MediaPath: "ready.mp4"},
		{ID: "plain-art", ClipHash: "review", MediaPath: "review.mp4"},
	} {
		a.AcquisitionID, a.Provider, a.SourceURL = newer.ID, "archive", "https://archive.org/details/"+a.ID
		a.MediaSHA256, a.MediaBytes, a.State, a.CompletedAt, a.UpdatedAt = strings.Repeat("d", 64), 1, filler.ArtifactConsumed, now, now
		if err := s.UpsertAcquisitionArtifacts(ctx, []filler.AcquisitionArtifact{a}); err != nil {
			t.Fatal(err)
		}
	}
	split := sampleClip("rejected", "rejected.mp4", filler.Commercial, 1994, filler.General, "")
	split.ParentHash = "ready"
	if err := s.UpsertClip(ctx, split); err != nil {
		t.Fatal(err)
	}

	runs, err := s.ListAcquisitionRuns(ctx, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != newer.ID {
		t.Fatalf("latest acquisition = %+v, want only %s", runs, newer.ID)
	}
	wantOutcome := filler.AcquisitionOutcome{Enrolled: 4, Preparing: 1, NeedsDecision: 1, Ready: 1, Rejected: 1}
	if runs[0].Outcome != wantOutcome {
		t.Fatalf("outcome = %+v, want %+v", runs[0].Outcome, wantOutcome)
	}
	if runs[0].PullID != "pull-7" || runs[0].Fetched != 2 || !runs[0].CompletedAt.Equal(now) {
		t.Fatalf("run facts did not round-trip: %+v", runs[0])
	}
	wantYield := []filler.AcquisitionGapYield{{Gap: "era:1990-1999", Downloads: 1,
		Outcome: filler.AcquisitionOutcome{Enrolled: 2, Ready: 1, Rejected: 1}}}
	if !reflect.DeepEqual(runs[0].GapYield, wantYield) {
		t.Fatalf("gap yield = %+v, want %+v", runs[0].GapYield, wantYield)
	}

	newer.Status, newer.Error, newer.UpdatedAt = filler.AcquisitionError, "catalogue failed", now.Add(time.Minute)
	if err := s.UpsertAcquisitionRun(ctx, newer); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetAcquisitionRun(ctx, newer.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != filler.AcquisitionError || got.Error != "catalogue failed" || got.Outcome != wantOutcome {
		t.Fatalf("updated acquisition = %+v", got)
	}
	if _, err := s.GetAcquisitionRun(ctx, "missing", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing acquisition = %v, want ErrNotFound", err)
	}

	recoveredAt := now.Add(2 * time.Minute)
	if n, err := s.RecoverInterruptedAcquisitionRuns(ctx, recoveredAt); err != nil || n != 2 {
		t.Fatalf("recover interrupted = %d, %v; want two queued/running runs", n, err)
	}
	for _, id := range []string{older.ID, queued.ID} {
		run, err := s.GetAcquisitionRun(ctx, id, recoveredAt)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status != filler.AcquisitionError || run.Error == "" || !run.CompletedAt.Equal(recoveredAt) {
			t.Fatalf("recovered %s = %+v", id, run)
		}
	}
	terminal, err := s.GetAcquisitionRun(ctx, newer.ID, recoveredAt)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.UpdatedAt.Equal(recoveredAt) {
		t.Fatal("startup recovery rewrote an already-terminal acquisition")
	}
}

func testFillerAcquisitionArtifacts(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0).UTC()
	run := filler.AcquisitionRun{
		ID: "acq-manifest", Trigger: filler.AcquisitionSource, SourceID: "archive:classic",
		Status: filler.AcquisitionRunning, Requested: 2, StartedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertAcquisitionRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	artifacts := []filler.AcquisitionArtifact{
		{
			ID: "artifact-one", AcquisitionID: run.ID, SourceID: run.SourceID,
			Provider: "youtube", SourceURL: "https://youtube.com/watch?v=artifact-one", RemoteID: "artifact-one",
			StagingPath: ".loomarr-acquisitions/acq-manifest/one.mp4", MediaPath: "one.mp4",
			SidecarPath: "one.info.json", MediaSHA256: strings.Repeat("a", 64), MediaBytes: 42,
			ProviderArchiveEntry: "youtube artifact-one", ProviderArchiveCommitted: true,
			Gap:   "era:1990-1999", // #749: which coverage gap this download was for
			State: filler.ArtifactStaged, CompletedAt: now, UpdatedAt: now,
		},
		{
			ID: "artifact-two", AcquisitionID: run.ID, SourceID: run.SourceID,
			Provider: "archive", SourceURL: "https://archive.org/details/two", RemoteID: "two",
			StagingPath: ".loomarr-acquisitions/acq-manifest/two.mp4", MediaPath: "two.mp4",
			SidecarPath: "two.info.json", MediaSHA256: strings.Repeat("b", 64), MediaBytes: 84,
			State: filler.ArtifactPublished, CompletedAt: now, UpdatedAt: now,
		},
	}
	if err := s.UpsertAcquisitionArtifacts(ctx, artifacts); err != nil {
		t.Fatal(err)
	}
	page, err := s.ListRecoverableAcquisitionArtifactsAfter(ctx, filler.AcquisitionArtifactCursor{UpdatedAt: now, ID: artifacts[0].ID}, 10)
	if err != nil || len(page) != 1 || page[0].ID != artifacts[1].ID {
		t.Fatalf("recoverable page after first artifact = %+v, %v", page, err)
	}
	got, found, err := s.AcquisitionArtifactForClip(ctx, "one.mp4", "")
	if err != nil || !found || got != artifacts[0] {
		t.Fatalf("artifact by path = %+v, %v, %v; want %+v", got, found, err, artifacts[0])
	}
	artifacts[0].ClipHash = strings.Repeat("c", 64)
	artifacts[0].MediaPath = "aa/bb/" + artifacts[0].ClipHash + ".mp4"
	artifacts[0].State = filler.ArtifactConsumed
	artifacts[0].UpdatedAt = now.Add(time.Minute)
	if err := s.UpsertAcquisitionArtifacts(ctx, artifacts[:1]); err != nil {
		t.Fatal(err)
	}
	got, found, err = s.AcquisitionArtifactForClip(ctx, "elsewhere.mp4", artifacts[0].ClipHash)
	if err != nil || !found || got != artifacts[0] {
		t.Fatalf("artifact by clip hash = %+v, %v, %v; want %+v", got, found, err, artifacts[0])
	}
	recoverable, err := s.ListRecoverableAcquisitionArtifacts(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recoverable) != 1 || recoverable[0].ID != artifacts[1].ID {
		t.Fatalf("recoverable artifacts = %+v, want only published artifact", recoverable)
	}
	remoteStates, err := s.ListAcquisitionRemoteStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if remoteStates[(filler.RemoteIdentity{Provider: "youtube", SourceID: run.SourceID, RemoteID: "artifact-one"}).Key()] != filler.RemoteCatalogued ||
		remoteStates[(filler.RemoteIdentity{Provider: "archive", SourceID: run.SourceID, RemoteID: "two"}).Key()] != filler.RemoteQueued {
		t.Fatalf("remote acquisition states = %v", remoteStates)
	}
	runs, err := s.ListAcquisitionRuns(ctx, 10, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Artifacts.Consumed != 1 || runs[0].Artifacts.Published != 1 {
		t.Fatalf("run artifact outcome = %+v", runs)
	}

	invalid := artifacts[1]
	invalid.ID = "invalid"
	invalid.MediaPath = "../escape.mp4"
	if err := s.UpsertAcquisitionArtifacts(ctx, []filler.AcquisitionArtifact{invalid}); err == nil {
		t.Fatal("invalid manifest reached persistence")
	}
}

func testInteractiveOperations(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_900_100_000, 0).UTC()

	split := InteractiveOperation{
		ID: "split-1", Kind: InteractiveOperationFillerSplit, Subject: "clip-hash",
		Status: InteractiveOperationRunning, StartedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertInteractiveOperation(ctx, split); err != nil {
		t.Fatal(err)
	}
	split.Status = InteractiveOperationSuccess
	split.ResultID = "sp-1"
	split.CompletedAt, split.UpdatedAt = now.Add(time.Minute), now.Add(time.Minute)
	if err := s.UpsertInteractiveOperation(ctx, split); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetInteractiveOperation(ctx, split.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != split {
		t.Fatalf("split operation = %+v, want %+v", got, split)
	}

	pull := InteractiveOperation{
		ID: "pull-1", Kind: InteractiveOperationLLMPull, Subject: "qwen3:8b",
		Status: InteractiveOperationRunning, Percent: 42, Completed: 420, Total: 1000,
		StartedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
	}
	if err := s.UpsertInteractiveOperation(ctx, pull); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetInteractiveOperation(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing operation = %v, want ErrNotFound", err)
	}

	recoveredAt := now.Add(2 * time.Minute)
	if n, err := s.RecoverInterruptedInteractiveOperations(ctx, recoveredAt); err != nil || n != 1 {
		t.Fatalf("recover operations = %d, %v; want one running pull", n, err)
	}
	recovered, err := s.GetInteractiveOperation(ctx, pull.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != InteractiveOperationError || recovered.Error == "" || !recovered.CompletedAt.Equal(recoveredAt) {
		t.Fatalf("recovered pull = %+v", recovered)
	}
	terminal, err := s.GetInteractiveOperation(ctx, split.ID)
	if err != nil {
		t.Fatal(err)
	}
	if terminal != split {
		t.Fatalf("recovery rewrote terminal split = %+v, want %+v", terminal, split)
	}
}

func testClipPipelineRetry(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0).UTC()
	clip := clipAt("retry/failed.mp4", "Failed encode", filler.Commercial, 30_000)
	if err := s.UpsertClip(ctx, Clip{Clip: clip, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetClipsRemoved(ctx, []string{clip.Path}, now); err != nil {
		t.Fatal(err)
	}
	failed := pipelinestate.ClipPipeline{
		ClipHash: clip.Hash, Stage: pipelinestate.StageTranscode, Status: pipelinestate.StatusFailed,
		Disposition: pipelinestate.DispositionRejected, RejectReason: pipelinestate.ReasonUnplayable,
		Attempts: filler.MaxAttempts, EnrolledAt: now, UpdatedAt: now,
	}
	if err := s.UpsertClipPipeline(ctx, failed); err != nil {
		t.Fatal(err)
	}
	retry := failed
	retry.Status, retry.Disposition, retry.Attempts = pipelinestate.StatusQueued, pipelinestate.DispositionRunning, 0
	retry.RejectReason, retry.ForceRun, retry.UpdatedAt = "", true, now.Add(time.Minute)
	if err := s.RetryClipPipeline(ctx, failed, retry, true); err != nil {
		t.Fatal(err)
	}
	if err := s.RetryClipPipeline(ctx, failed, retry, true); !errors.Is(err, filler.ErrPipelineNotRetryable) {
		t.Fatalf("stale retry = %v, want ErrPipelineNotRetryable", err)
	}
	got, err := s.GetClip(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if !got.RemovedAt.IsZero() || !got.Held {
		t.Fatalf("restored clip = removed:%v held:%v, want present and held", got.RemovedAt, got.Held)
	}
	row, found, err := s.GetClipPipeline(ctx, clip.Hash)
	if err != nil || !found || row.Status != pipelinestate.StatusQueued || row.Disposition != pipelinestate.DispositionRunning || !row.ForceRun {
		t.Fatalf("retry row = %+v found=%v err=%v", row, found, err)
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

	if err := s.UpsertClipFingerprint(ctx, one.Hash, "creative-test", frames()); err == nil {
		t.Fatal("empty fingerprint was persisted")
	}
	want := creative.Fingerprint{DurationMs: 30_000, Video: []uint64{0, 1, ^uint64(0)}, Audio: []uint32{5, creative.QuietHop}}
	if err := s.UpsertClipFingerprint(ctx, one.Hash, "creative-test", want); err != nil {
		t.Fatal(err)
	}
	all, err := s.ListClipFingerprints(ctx, "creative-test")
	if got := all[one.Hash]; err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("fingerprint round-trip = (%+v, %v), want %+v", got, err, want)
	}
	if _, found, err := cachedClipFingerprint(ctx, s, one.Hash, "creative-other"); err != nil || found {
		t.Errorf("different algorithm hit the cache: found=%v err=%v", found, err)
	}
	if err := s.UpsertClipFingerprint(ctx, one.Hash, "creative-test", frames(9)); err != nil {
		t.Fatal(err)
	}
	got, found, err := cachedClipFingerprint(ctx, s, one.Hash, "creative-test")
	if err != nil || !found || len(got) != 1 || got[0] != 9 {
		t.Errorf("idempotent upsert = (%v, %v, %v), want [9]", got, found, err)
	}
	if err := s.UpsertClipFingerprint(ctx, two.Hash, "creative-test", frames(7)); err != nil {
		t.Fatal(err)
	}

	// Keep only clip two. The cache has no FK by design, so DeleteClipsNotIn owns this cleanup.
	if _, err := s.DeleteClipsNotIn(ctx, []string{two.Hash}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := cachedClipFingerprint(ctx, s, one.Hash, "creative-test"); err != nil || found {
		t.Errorf("pruned clip left an orphan fingerprint: found=%v err=%v", found, err)
	}
	if got, found, err := cachedClipFingerprint(ctx, s, two.Hash, "creative-test"); err != nil || !found || len(got) != 1 || got[0] != 7 {
		t.Errorf("kept clip lost its fingerprint: got=%v found=%v err=%v", got, found, err)
	}
}

// testClipDefects pins the defect floor's record (#2034): it round-trips, a later measurement
// replaces it, a re-key drops it because it describes the old bytes, and a pruned clip takes its
// record with it.
func testClipDefects(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	one := sampleClip("defects-one", "One", filler.Commercial, 1988, filler.General, "")
	two := sampleClip("defects-two", "Two", filler.Commercial, 2012, filler.General, "")
	for _, c := range []Clip{one, two} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	record := func(hash string, era int, blur float64, j *defects.Judgement) clipstore.ClipDefectRecord {
		r := defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40, BlurMean: blur}
		a := defects.Assess(r, era)
		verdict, reason := defects.Decide(a, j)
		return clipstore.ClipDefectRecord{ClipHash: hash, Readings: r, Era: era, Assessment: a, Judgement: j,
			Verdict: verdict, VerdictReason: reason, MeasuredAt: now}
	}
	if err := s.UpsertClipDefects(ctx, clipstore.ClipDefectRecord{ClipHash: one.Hash}); err == nil {
		t.Fatal("a record with no measurement or verdict version was stored")
	}
	unjudged := record(one.Hash, 1988, 4, nil)
	unjudged.Verdict = ""
	if err := s.UpsertClipDefects(ctx, unjudged); err == nil {
		t.Fatal("a record with no verdict was stored")
	}
	if err := s.UpsertClipDefects(ctx, record(one.Hash, 1988, 4, nil)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertClipDefects(ctx, record(one.Hash, 2012, 11, nil)); err != nil {
		t.Fatal(err)
	}
	// Borderline tape softness, settled by the judge: the ruling round-trips with the record.
	ruled := &defects.Judgement{Version: defects.JudgeVersion, Ruling: defects.RulingPass, Reason: "clean enough", Model: "m1"}
	if err := s.UpsertClipDefects(ctx, record(two.Hash, 1988, 8.5, ruled)); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListClipDefects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := record(one.Hash, 2012, 11, nil); !reflect.DeepEqual(got[one.Hash], want) {
		t.Fatalf("record = %+v, want the later measurement %+v", got[one.Hash], want)
	}
	if got[one.Hash].Assessment.Outcome != defects.Fail || got[one.Hash].Verdict != defects.VerdictFail {
		t.Fatalf("an HD-era blur of 11 was recorded as %q / %q", got[one.Hash].Assessment.Outcome, got[one.Hash].Verdict)
	}
	if want := record(two.Hash, 1988, 8.5, ruled); !reflect.DeepEqual(got[two.Hash], want) || got[two.Hash].Verdict != defects.VerdictPass {
		t.Fatalf("judged record = %+v, want %+v", got[two.Hash], want)
	}

	// The maintainer's labels (#2034 step 4c): only on a measured clip, for the era its record was
	// judged against, kept when the clip is judged again, cleared by an empty label.
	if err := s.SetClipDefectLabel(ctx, "never-measured", defects.LabelFail, now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("labelling a clip with no record = %v, want ErrNotFound", err)
	}
	if err := s.SetClipDefectLabel(ctx, one.Hash, "maybe", now); err == nil {
		t.Fatal("an unknown label was stored")
	}
	if err := s.SetClipDefectLabel(ctx, one.Hash, defects.LabelFail, now); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipDefectLabel(ctx, two.Hash, defects.LabelPass, now); err != nil {
		t.Fatal(err)
	}
	// Re-dated after labelling: the label keeps the era it was given for.
	if err := s.UpsertClipDefects(ctx, record(one.Hash, 1999, 11, nil)); err != nil {
		t.Fatal(err)
	}
	labels, err := s.ListClipDefectLabels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	labelTwo := clipstore.ClipDefectLabel{Label: defects.LabelPass, Era: 1988, LabelledAt: now}
	if want := map[string]clipstore.ClipDefectLabel{
		one.Hash: {Label: defects.LabelFail, Era: 2012, LabelledAt: now}, two.Hash: labelTwo,
	}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("labels = %+v, want %+v: judging again must keep a label and the era it was given for", labels, want)
	}
	if err := s.SetClipDefectLabel(ctx, one.Hash, "", now); err != nil {
		t.Fatal(err)
	}
	if labels, err = s.ListClipDefectLabels(ctx); err != nil || len(labels) != 1 || labels[two.Hash] != labelTwo {
		t.Fatalf("labels after clearing one = %+v (%v), want only two's", labels, err)
	}

	transformed := two
	transformed.Hash = "defects-two-conditioned"
	if err := s.ReplaceClipIdentity(ctx, two.Hash, transformed); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListClipDefects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, kept := got[two.Hash]; kept || len(got) != 1 {
		t.Fatalf("records after re-key = %v, want only clip one's: the old bytes' verdict must not survive", got)
	}
	if labels, err = s.ListClipDefectLabels(ctx); err != nil || len(labels) != 0 {
		t.Fatalf("labels after re-key = %+v (%v), want none: a label describes the old bytes", labels, err)
	}
	if _, err := s.DeleteClipsNotIn(ctx, []string{transformed.Hash}); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListClipDefects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("records = %v, want none: one was pruned with its clip, two re-keyed onto new bytes", got)
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
	comp := sampleClip("break1", "wktl-1996.mp4", filler.Commercial, 1996, filler.General, "")
	comp.DurationMs = 971_000
	if err := s.UpsertClip(ctx, comp); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipComposite(ctx, comp.Hash, true, now); err != nil {
		t.Fatal(err)
	}
	seg1 := sampleClip("seg1", "aw-root-beer.mp4", filler.Commercial, 1996, filler.General, "fast_food")
	seg1.ParentHash = comp.Hash
	seg2 := sampleClip("seg2", "drumstick-shack.mp4", filler.Commercial, 1996, filler.General, "fast_food")
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

	// A completed re-split replaces the old generation atomically. seg2 is explicitly pinned and
	// therefore survives even though the new detector did not reproduce it; seg1 is retired but
	// remains recoverable as a tombstoned row. A newly accepted hash is restored if an earlier
	// generation had already tombstoned it.
	seg3 := sampleClip("seg3", "better-cut.mp4", filler.Commercial, 1996, filler.General, "drinks")
	seg3.ParentHash = comp.Hash
	if err := s.UpsertClip(ctx, seg3); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetClipsRemoved(ctx, []string{seg3.Path}, now); err != nil {
		t.Fatal(err)
	}
	ch := sampleChannel("pinned-split-child", 809, time.Time{})
	ch.Policy.Filler = &schedule.FillerSelection{Pinned: []string{seg2.Hash}}
	if _, err := s.SaveChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}
	retired, err := s.ReplaceSplitChildren(ctx, comp.Hash, []string{seg3.Hash}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if retired != 1 {
		t.Errorf("retired = %d, want only superseded unpinned seg1", retired)
	}
	active, err := s.ListClips(ctx, ClipFilter{ParentHash: comp.Hash})
	if err != nil {
		t.Fatal(err)
	}
	if containsHash(active, seg1.Hash) || !containsHash(active, seg2.Hash) || !containsHash(active, seg3.Hash) {
		t.Errorf("active replacement generation = %+v, want pinned seg2 + new seg3", active)
	}
	all, err := s.ListClips(ctx, ClipFilter{ParentHash: comp.Hash, IncludeRemoved: true})
	if err != nil {
		t.Fatal(err)
	}
	if !containsHash(all, seg1.Hash) {
		t.Error("superseded seg1 row was deleted instead of tombstoned")
	}
}

func testSplitConfirmationAtomic(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_900_000_000, 0).UTC()
	parent := sampleClip("atomic-parent", "atomic-parent.mp4", filler.Commercial, 1994, filler.General, "")
	parent.Held = true
	old := sampleClip("atomic-old", "atomic-old.mp4", filler.Commercial, 1994, filler.General, "")
	old.ParentHash = parent.Hash
	first := sampleClip("atomic-first", "atomic-first.mp4", filler.Commercial, 1994, filler.General, "")
	first.ParentHash = parent.Hash
	second := sampleClip("atomic-second", "atomic-second.mp4", filler.Commercial, 1994, filler.General, "")
	second.ParentHash = parent.Hash
	for _, clip := range []Clip{parent, old, first, second} {
		if err := s.UpsertClip(ctx, clip); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SetClipsRemoved(ctx, []string{first.Path, second.Path}, now); err != nil {
		t.Fatal(err)
	}
	parentPipeline := pipelinestate.ClipPipeline{
		ClipHash: parent.Hash, Stage: pipelinestate.StageSplit, Status: pipelinestate.StatusQueued,
		Disposition: pipelinestate.DispositionReview, NextRun: now.Add(time.Hour),
		EnrolledAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Minute),
	}
	firstPipeline := pipelinestate.ClipPipeline{
		ClipHash: first.Hash, Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusQueued,
		Disposition: pipelinestate.DispositionReview, EnrolledAt: now, UpdatedAt: now,
	}
	if err := s.UpsertClipPipeline(ctx, parentPipeline); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertClipPipeline(ctx, firstPipeline); err != nil {
		t.Fatal(err)
	}
	proposal := filler.SplitProposal{
		ID: "atomic-proposal", ClipHash: parent.Hash, CreatedAt: now,
		Segments: []split.Segment{{StartMs: 0, EndMs: 10_000, Name: "first"}},
	}
	if err := s.UpsertSplitProposal(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	completion := filler.SplitCompletion{
		ProposalID: proposal.ID, ClaimToken: "atomic-owner", ParentHash: parent.Hash,
		ChildHashes: []string{first.Hash, second.Hash}, ActivateHashes: []string{first.Hash, second.Hash}, At: now,
	}
	if _, err := s.AcquireSplitProposalClaim(ctx, proposal.ID, completion.ClaimToken, now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	wrongToken := completion
	wrongToken.ClaimToken = "stale-owner"
	if _, err := s.CompleteSplitConfirmation(ctx, wrongToken); !errors.Is(err, filler.ErrProposalClaimed) {
		t.Fatalf("stale full completion = %v, want ErrProposalClaimed", err)
	}

	// The missing second pipeline fails after the transaction has updated the parent and first
	// child. Every one of those statements must roll back with proposal consumption and selection.
	if _, err := s.CompleteSplitConfirmation(ctx, completion); err == nil {
		t.Fatal("completion succeeded without every staged child pipeline")
	}
	if got, err := s.GetSplitProposal(ctx, proposal.ID); err != nil || got.ClipHash != parent.Hash {
		t.Fatalf("failed completion consumed proposal: got %+v, err %v", got, err)
	}
	gotParent, err := s.GetClip(ctx, parent.Hash)
	if err != nil || gotParent.IsComposite || !gotParent.Held {
		t.Fatalf("failed completion changed parent: %+v, %v", gotParent, err)
	}
	gotParentPipeline, found, err := s.GetClipPipeline(ctx, parent.Hash)
	if err != nil || !found || gotParentPipeline.Disposition != pipelinestate.DispositionReview {
		t.Fatalf("failed completion changed parent pipeline: %+v, found=%v err=%v", gotParentPipeline, found, err)
	}
	gotFirstPipeline, found, err := s.GetClipPipeline(ctx, first.Hash)
	if err != nil || !found || gotFirstPipeline.Disposition != pipelinestate.DispositionReview {
		t.Fatalf("failed completion activated first child: %+v, found=%v err=%v", gotFirstPipeline, found, err)
	}
	active, err := s.ListClips(ctx, ClipFilter{ParentHash: parent.Hash})
	if err != nil || !containsHash(active, old.Hash) || containsHash(active, first.Hash) || containsHash(active, second.Hash) {
		t.Fatalf("failed completion changed selected generation: %+v, %v", active, err)
	}

	secondPipeline := firstPipeline
	secondPipeline.ClipHash = second.Hash
	if err := s.UpsertClipPipeline(ctx, secondPipeline); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteSplitConfirmation(ctx, completion); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSplitProposal(ctx, proposal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("committed completion retained proposal: %v", err)
	}
	gotParent, err = s.GetClip(ctx, parent.Hash)
	if err != nil || !gotParent.IsComposite || gotParent.Held {
		t.Fatalf("committed completion parent = %+v, %v", gotParent, err)
	}
	gotParentPipeline, found, err = s.GetClipPipeline(ctx, parent.Hash)
	if err != nil || !found || gotParentPipeline.Disposition != pipelinestate.DispositionComplete {
		t.Fatalf("committed completion parent pipeline = %+v, found=%v err=%v", gotParentPipeline, found, err)
	}
	active, err = s.ListClips(ctx, ClipFilter{ParentHash: parent.Hash})
	if err != nil || containsHash(active, old.Hash) || !containsHash(active, first.Hash) || !containsHash(active, second.Hash) {
		t.Fatalf("committed selected generation = %+v, %v", active, err)
	}
	for _, hash := range []string{first.Hash, second.Hash} {
		pipeline, found, err := s.GetClipPipeline(ctx, hash)
		if err != nil || !found || pipeline.Disposition != pipelinestate.DispositionRunning {
			t.Errorf("committed child pipeline %s = %+v, found=%v err=%v", hash, pipeline, found, err)
		}
	}
}

func testSplitConfirmationRequiresReviewParent(t *testing.T, newStore NewStoreFunc) {
	for _, tc := range []struct {
		name              string
		parentDisposition *pipelinestate.Disposition
	}{
		{name: "missing parent pipeline"},
		{name: "parent already complete", parentDisposition: func() *pipelinestate.Disposition {
			disposition := pipelinestate.DispositionComplete
			return &disposition
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t)
			ctx := context.Background()
			now := time.Unix(1_900_100_000, 0).UTC()
			parent := sampleClip("review-parent", "review-parent.mp4", filler.Commercial, 1994, filler.General, "")
			parent.Held = true
			old := sampleClip("review-old", "review-old.mp4", filler.Commercial, 1994, filler.General, "")
			old.ParentHash = parent.Hash
			child := sampleClip("review-child", "review-child.mp4", filler.Commercial, 1994, filler.General, "")
			child.ParentHash = parent.Hash
			for _, clip := range []Clip{parent, old, child} {
				if err := s.UpsertClip(ctx, clip); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.SetClipsRemoved(ctx, []string{child.Path}, now); err != nil {
				t.Fatal(err)
			}
			childPipeline := pipelinestate.ClipPipeline{
				ClipHash: child.Hash, Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusQueued,
				Disposition: pipelinestate.DispositionReview, EnrolledAt: now, UpdatedAt: now,
			}
			if err := s.UpsertClipPipeline(ctx, childPipeline); err != nil {
				t.Fatal(err)
			}
			if tc.parentDisposition != nil {
				if err := s.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
					ClipHash: parent.Hash, Stage: pipelinestate.StageSplit, Status: pipelinestate.StatusQueued,
					Disposition: *tc.parentDisposition, EnrolledAt: now, UpdatedAt: now,
				}); err != nil {
					t.Fatal(err)
				}
			}
			proposal := filler.SplitProposal{
				ID: "review-proposal", ClipHash: parent.Hash, CreatedAt: now,
				Segments: []split.Segment{{StartMs: 0, EndMs: 10_000, Name: "child"}},
			}
			if err := s.UpsertSplitProposal(ctx, proposal); err != nil {
				t.Fatal(err)
			}
			const claimToken = "review-owner"
			if _, err := s.AcquireSplitProposalClaim(ctx, proposal.ID, claimToken, now, now.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}

			_, err := s.CompleteSplitConfirmation(ctx, filler.SplitCompletion{
				ProposalID: proposal.ID, ClaimToken: claimToken, ParentHash: parent.Hash,
				ChildHashes: []string{child.Hash}, ActivateHashes: []string{child.Hash}, At: now,
			})
			if err == nil {
				t.Fatal("completion succeeded without exactly one review parent pipeline transition")
			}
			if got, err := s.GetSplitProposal(ctx, proposal.ID); err != nil || got.ClipHash != parent.Hash {
				t.Fatalf("failed completion consumed proposal: got %+v, err %v", got, err)
			}
			gotParent, err := s.GetClip(ctx, parent.Hash)
			if err != nil || gotParent.IsComposite || !gotParent.Held {
				t.Fatalf("failed completion changed held parent: %+v, %v", gotParent, err)
			}
			gotChildPipeline, found, err := s.GetClipPipeline(ctx, child.Hash)
			if err != nil || !found || gotChildPipeline.Disposition != pipelinestate.DispositionReview {
				t.Fatalf("failed completion activated child: %+v, found=%v err=%v", gotChildPipeline, found, err)
			}
			active, err := s.ListClips(ctx, ClipFilter{ParentHash: parent.Hash})
			if err != nil || !containsHash(active, old.Hash) || containsHash(active, child.Hash) {
				t.Fatalf("failed completion changed selected generation: %+v, %v", active, err)
			}
		})
	}
}

// A proposal on a filed clip (not held for split review) cannot be confirmed (#1761). The store
// must say why, distinctly from a missing proposal, which the API renders as 404.
func testSplitConfirmationRefusesFiledParent(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_900_200_000, 0).UTC()
	parent := sampleClip("filed-parent", "filed-parent.mp4", filler.Commercial, 1994, filler.General, "")
	child := sampleClip("filed-child", "filed-child.mp4", filler.Commercial, 1994, filler.General, "")
	child.ParentHash = parent.Hash
	for _, clip := range []Clip{parent, child} {
		if err := s.UpsertClip(ctx, clip); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SetClipsRemoved(ctx, []string{child.Path}, now); err != nil {
		t.Fatal(err)
	}
	for _, p := range []pipelinestate.ClipPipeline{
		{ClipHash: parent.Hash, Stage: pipelinestate.StageSplit, Status: pipelinestate.StatusQueued,
			Disposition: pipelinestate.DispositionReview, EnrolledAt: now, UpdatedAt: now},
		{ClipHash: child.Hash, Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusQueued,
			Disposition: pipelinestate.DispositionReview, EnrolledAt: now, UpdatedAt: now},
	} {
		if err := s.UpsertClipPipeline(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	proposal := filler.SplitProposal{
		ID: "filed-proposal", ClipHash: parent.Hash, CreatedAt: now,
		Segments: []split.Segment{{StartMs: 0, EndMs: 10_000, Name: "child"}},
	}
	if err := s.UpsertSplitProposal(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	const claimToken = "filed-owner"
	if _, err := s.AcquireSplitProposalClaim(ctx, proposal.ID, claimToken, now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	_, err := s.CompleteSplitConfirmation(ctx, filler.SplitCompletion{
		ProposalID: proposal.ID, ClaimToken: claimToken, ParentHash: parent.Hash,
		ChildHashes: []string{child.Hash}, ActivateHashes: []string{child.Hash}, At: now,
	})
	if !errors.Is(err, filler.ErrSplitParentNotHeld) || errors.Is(err, ErrNotFound) {
		t.Fatalf("confirm on a filed parent = %v, want ErrSplitParentNotHeld and not ErrNotFound", err)
	}
	if got, err := s.GetSplitProposal(ctx, proposal.ID); err != nil || got.ClipHash != parent.Hash {
		t.Fatalf("refused completion consumed proposal: got %+v, err %v", got, err)
	}
	if got, err := s.GetClip(ctx, parent.Hash); err != nil || got.IsComposite || got.Held {
		t.Fatalf("refused completion changed the filed parent: %+v, %v", got, err)
	}
}

func testSplitProposalClaimFencesConfirmers(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_901_000_000, 0).UTC()
	proposal := filler.SplitProposal{
		ID: "claim-proposal", ClipHash: "claim-parent", CreatedAt: now,
		Segments: []split.Segment{{StartMs: 0, EndMs: 10_000, Name: "one"}},
	}
	if err := s.UpsertSplitProposal(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.AcquireSplitProposalClaim(ctx, proposal.ID, "owner-a", now, now.Add(time.Minute))
	if err != nil || claimed.ID != proposal.ID {
		t.Fatalf("first claim = %+v, %v", claimed, err)
	}
	if _, err := s.AcquireSplitProposalClaim(ctx, proposal.ID, "owner-b", now.Add(time.Second), now.Add(2*time.Minute)); !errors.Is(err, filler.ErrProposalClaimed) {
		t.Fatalf("concurrent claim = %v, want ErrProposalClaimed", err)
	}

	changed := proposal
	changed.Segments = []split.Segment{{StartMs: 10_000, EndMs: 20_000, Name: "two"}}
	changed.Spawned = []string{"claim-child"}
	childPipeline := pipelinestate.ClipPipeline{
		ClipHash: "claim-child", Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusQueued,
		Disposition: pipelinestate.DispositionReview, EnrolledAt: now, UpdatedAt: now,
	}
	if err := s.UpsertClipPipeline(ctx, childPipeline); err != nil {
		t.Fatal(err)
	}
	partial := filler.SplitPartialCompletion{
		Proposal: changed, ClaimToken: "owner-b", ActivateHashes: []string{"claim-child"}, At: now,
	}
	if err := s.CompletePartialSplitConfirmation(ctx, partial); !errors.Is(err, filler.ErrProposalClaimed) {
		t.Fatalf("losing partial completion = %v, want ErrProposalClaimed", err)
	}
	if got, err := s.GetSplitProposal(ctx, proposal.ID); err != nil || !reflect.DeepEqual(got.Segments, proposal.Segments) {
		t.Fatalf("losing token mutated proposal: %+v, %v", got, err)
	}

	// Expiry is the crash-recovery path. The new fencing token takes ownership; the stale owner can
	// neither release nor complete after that transition.
	claimed, err = s.AcquireSplitProposalClaim(ctx, proposal.ID, "owner-b", now.Add(time.Minute), now.Add(2*time.Minute))
	if err != nil || claimed.ID != proposal.ID {
		t.Fatalf("expired claim recovery = %+v, %v", claimed, err)
	}
	if err := s.ReleaseSplitProposalClaim(ctx, proposal.ID, "owner-a"); !errors.Is(err, filler.ErrProposalClaimed) {
		t.Fatalf("stale release = %v, want ErrProposalClaimed", err)
	}
	partial.ClaimToken = "owner-a"
	if err := s.CompletePartialSplitConfirmation(ctx, partial); !errors.Is(err, filler.ErrProposalClaimed) {
		t.Fatalf("stale completion = %v, want ErrProposalClaimed", err)
	}
	partial.ClaimToken = "owner-b"
	partial.ActivateHashes = []string{"claim-child", "missing-child"}
	if err := s.CompletePartialSplitConfirmation(ctx, partial); err == nil {
		t.Fatal("partial completion succeeded without every staged child pipeline")
	}
	if pipeline, found, err := s.GetClipPipeline(ctx, "claim-child"); err != nil || !found || pipeline.Disposition != pipelinestate.DispositionReview {
		t.Fatalf("failed partial completion activated first child: %+v, found=%v err=%v", pipeline, found, err)
	}
	if got, err := s.GetSplitProposal(ctx, proposal.ID); err != nil || !reflect.DeepEqual(got.Segments, proposal.Segments) {
		t.Fatalf("failed child activation mutated proposal: %+v, %v", got, err)
	}
	partial.ActivateHashes = []string{"claim-child"}
	if err := s.CompletePartialSplitConfirmation(ctx, partial); err != nil {
		t.Fatalf("winning partial completion: %v", err)
	}
	got, err := s.GetSplitProposal(ctx, proposal.ID)
	if err != nil || !reflect.DeepEqual(got.Segments, changed.Segments) {
		t.Fatalf("winning partial completion = %+v, %v", got, err)
	}
	if pipeline, found, err := s.GetClipPipeline(ctx, "claim-child"); err != nil || !found || pipeline.Disposition != pipelinestate.DispositionRunning {
		t.Fatalf("winning partial completion pipeline: %+v, found=%v err=%v", pipeline, found, err)
	}
	if _, err := s.AcquireSplitProposalClaim(ctx, proposal.ID, "owner-c", now.Add(2*time.Minute), now.Add(3*time.Minute)); err != nil {
		t.Fatalf("partial completion did not release claim: %v", err)
	}
	if err := s.ReleaseSplitProposalClaim(ctx, proposal.ID, "owner-c"); err != nil {
		t.Fatalf("release current claim: %v", err)
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
	for _, hash := range []string{"clipA", "clipB"} {
		c := sampleClip(hash, hash+".mp4", filler.Commercial, 0, "", "")
		c.Held = true
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	// Tag a clip `beer` → the denormalised set must be beer(leaf) + alcohol + drinks (rollups).
	if err := s.SetClipTags(ctx, "clipA", []string{"beer"}); err != nil {
		t.Fatal(err)
	}
	full, _ := s.GetClipTags(ctx, "clipA", false)
	assertSet(t, "clipA full tags", full, []string{"alcohol", "beer", "drinks"})
	leaves, _ := s.GetClipTags(ctx, "clipA", true)
	assertSet(t, "clipA leaves", leaves, []string{"beer"})

	// Two leaves sharing an ancestor: beer + spirits → alcohol/drinks stored ONCE (as rollups).
	if err := s.SetClipTags(ctx, "clipB", []string{"beer", "spirits", "beer"}); err != nil {
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
	realClip.Role = filler.Commercial
	realClip.DurationMs = 30000
	if err := s.UpsertClip(ctx, realClip); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipTags(ctx, "clipReal", []string{"beer"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetClip(ctx, "clipReal")
	if err != nil {
		t.Fatal(err)
	}
	assertSet(t, "GetClip loads Tags (full rollup set)", got.Tags, []string{"alcohol", "beer", "drinks"})
	assertSet(t, "GetClip loads only authored AssertedTags", got.AssertedTags, []string{"beer"})
	listed, err := s.ListClips(ctx, ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range listed {
		if c.Hash == "clipReal" {
			found = true
			assertSet(t, "ListClips loads Tags (full rollup set)", c.Tags, []string{"alcohol", "beer", "drinks"})
			assertSet(t, "ListClips loads only authored AssertedTags", c.AssertedTags, []string{"beer"})
		}
	}
	if !found {
		t.Error("clipReal missing from ListClips")
	}

	// ⚠ Re-tag REPLACES, never accumulates: clipA re-tagged `cereal` must lose beer/alcohol/drinks and
	// gain cereal/food — not keep the old alcohol lineage.
	if err := s.SetClipTags(ctx, "clipA", []string{"cereal"}); err != nil {
		t.Fatal(err)
	}
	full, _ = s.GetClipTags(ctx, "clipA", false)
	assertSet(t, "clipA after re-tag", full, []string{"cereal", "food"})
	if err := s.SetClipTags(ctx, "clipA", []string{"taxonomy-changed-under-editor"}); !errors.Is(err, ErrTaxonConflict) {
		t.Fatalf("retag with missing current taxon = %v, want ErrTaxonConflict", err)
	}
	full, _ = s.GetClipTags(ctx, "clipA", false)
	assertSet(t, "rejected retag preserves prior generation", full, []string{"cereal", "food"})
	clipA, err := s.GetClip(ctx, "clipA")
	if err != nil || clipA.Category != "cereal" {
		t.Fatalf("rejected retag category = %q, err=%v; want cereal", clipA.Category, err)
	}

	// Operator edit: add a taxon; it must appear in ListTaxa (the CRUD path).
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Create: true, Taxon: taxonomy.Taxon{Slug: "energy-drink", Label: "Energy drink", Parent: "drinks", Axis: taxonomy.AxisProduct}}, now); err != nil {
		t.Fatal(err)
	}
	taxa2, _ := s.ListTaxa(ctx)
	if _, ok := taxonomy.New(taxa2).Get("energy-drink"); !ok {
		t.Error("ApplyTaxonomyEdit did not persist the new taxon")
	}

	// Invalid graph edits are rejected atomically: a cross-axis parent must change neither the row
	// nor the closure/rollups the catalog is matching against.
	bad := taxonomy.Taxon{Slug: "energy-drink", Label: "Energy drink", Parent: "promo", Axis: taxonomy.AxisProduct}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Slug: bad.Slug, Taxon: bad}, now); !errors.Is(err, taxonomy.ErrInvalidForest) {
		t.Fatalf("cross-axis edit = %v, want ErrInvalidForest", err)
	}
	if got, _ := taxonomy.New(mustList(t, s, ctx)).Get("energy-drink"); got.Parent != "drinks" {
		t.Errorf("rejected edit changed parent to %q, want drinks", got.Parent)
	}

	// Concurrent editors must serialize around the WHOLE graph generation. Postgres otherwise lets
	// both requests validate the same snapshot, then interleave their closure replacement so one
	// newly-added node has no ancestry. SQLite serializes writes itself; the same test pins both.
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, slug := range []string{"sports-drink", "sparkling-water"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Create: true, Taxon: taxonomy.Taxon{
				Slug: slug, Label: slug, Parent: "drinks", Axis: taxonomy.AxisProduct,
			}}, now)
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent taxonomy edit: %v", err)
		}
	}
	concurrentForest := taxonomy.New(mustList(t, s, ctx))
	sqls := store.HandleOf(s)
	if sqls == nil {
		t.Fatalf("taxonomy conformance store = %T, want a SQL store", s)
	}
	for _, slug := range []string{"sports-drink", "sparkling-water"} {
		if got := concurrentForest.Ancestors(slug); len(got) != 1 || got[0] != "drinks" {
			t.Errorf("concurrent taxon %q ancestors = %v, want [drinks]", slug, got)
		}
		var closureRows int
		if err := sqls.QueryRowContext(ctx, sqls.Rebind(
			`SELECT COUNT(*) FROM taxa_closure WHERE ancestor = ? AND descendant = ?`), "drinks", slug).Scan(&closureRows); err != nil {
			t.Fatal(err)
		}
		if closureRows != 1 {
			t.Errorf("concurrent taxon %q closure rows = %d, want drinks ancestor committed atomically", slug, closureRows)
		}
	}

	// A used asserted taxon cannot be deleted: vocabulary cleanup may not erase library knowledge.
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Delete: true, Slug: "beer"}, now); !errors.Is(err, ErrTaxonConflict) {
		t.Fatalf("delete asserted beer = %v, want ErrTaxonConflict", err)
	}
	usage, err := s.TaxonomyUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if usage.TotalClips != 1 || usage.TaggedClips != 1 {
		t.Errorf("taxonomy coverage = %d total/%d tagged, want 1/1", usage.TotalClips, usage.TaggedClips)
	}
	if got := usage.ByTaxon["beer"]; got.Asserted != 1 || got.Matched != 1 {
		t.Errorf("beer usage = %+v, want asserted=1 matched=1", got)
	}
	if got := usage.ByTaxon["beer"].Stored; got != 2 {
		t.Errorf("beer stored assignments = %d, want 2 including the non-playable fixture", got)
	}
	if got := usage.ByTaxon["drinks"]; got.Asserted != 0 || got.Matched != 1 {
		t.Errorf("drinks usage = %+v, want asserted=0 matched=1", got)
	}

	// Preview uses the same prospective-graph validation as commit but does not mutate it. It counts
	// distinct stored assertions across the affected subtree, separately identifies playable clips
	// that may change channel fit, and explains resolver behavior changes.
	beer, ok := taxonomy.New(mustList(t, s, ctx)).Get("beer")
	if !ok {
		t.Fatal("seed taxonomy has no beer taxon")
	}
	beer.Synonyms = append(beer.Synonyms, "pint")
	impact, err := s.PreviewTaxonomyEdit(ctx, TaxonomyEdit{Slug: beer.Slug, Taxon: beer})
	if err != nil {
		t.Fatal(err)
	}
	if impact.DirectStoredClips != 2 || impact.DescendantStoredClips != 0 || impact.AffectedStoredClips != 2 {
		t.Errorf("beer preview stored impact = direct %d/descendant %d/affected %d, want 2/0/2", impact.DirectStoredClips, impact.DescendantStoredClips, impact.AffectedStoredClips)
	}
	if len(impact.PlayableClipHashes) != 0 {
		t.Errorf("resolver-only preview playable hashes = %v, want none because eligibility is unchanged", impact.PlayableClipHashes)
	}
	assertSet(t, "beer preview added resolver terms", impact.ResolverTermsAdded, []string{"pint"})
	if len(impact.ResolverTermsRemoved) != 0 {
		t.Errorf("beer preview removed resolver terms = %v, want none", impact.ResolverTermsRemoved)
	}
	if liveBeer, _ := taxonomy.New(mustList(t, s, ctx)).Get("beer"); len(liveBeer.Synonyms) == len(beer.Synonyms) {
		t.Error("PreviewTaxonomyEdit mutated the live graph")
	}

	alcoholImpact, err := s.PreviewTaxonomyEdit(ctx, TaxonomyEdit{Delete: true, Slug: "alcohol"})
	if err != nil {
		t.Fatal(err)
	}
	if alcoholImpact.DirectStoredClips != 0 || alcoholImpact.DescendantStoredClips != 2 || alcoholImpact.AffectedStoredClips != 2 {
		t.Errorf("alcohol delete impact = direct %d/descendant %d/affected %d, want 0/2/2", alcoholImpact.DirectStoredClips, alcoholImpact.DescendantStoredClips, alcoholImpact.AffectedStoredClips)
	}
	gotDescendants := make([]string, 0, len(alcoholImpact.Descendants))
	for _, descendant := range alcoholImpact.Descendants {
		gotDescendants = append(gotDescendants, descendant.Slug)
	}
	assertSet(t, "alcohol descendants", gotDescendants, []string{"beer", "spirits"})
	if _, err := s.PreviewTaxonomyEdit(ctx, TaxonomyEdit{Slug: "beer", Taxon: taxonomy.Taxon{Slug: "beer", Label: "Beer", Parent: "promo", Axis: taxonomy.AxisProduct}}); !errors.Is(err, taxonomy.ErrInvalidForest) {
		t.Fatalf("invalid taxonomy preview = %v, want ErrInvalidForest", err)
	}
	if usage.ByAxis[taxonomy.AxisProduct] != 1 || usage.ByAxis[taxonomy.AxisFormat] != 0 {
		t.Errorf("axis coverage product=%d format=%d, want 1/0 unique playable clips", usage.ByAxis[taxonomy.AxisProduct], usage.ByAxis[taxonomy.AxisFormat])
	}

	// `category` is a compatibility shadow consumed by pod matching. A graph edit that moves an
	// asserted node off the product axis must clear it in the SAME transaction as the graph and
	// rollups, or the catalog and scheduler disagree about what the clip is.
	shadowTaxon := taxonomy.Taxon{Slug: "shadow-product", Label: "Shadow product", Axis: taxonomy.AxisProduct}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Create: true, Taxon: shadowTaxon}, now); err != nil {
		t.Fatal(err)
	}
	shadowClip := Clip{Clip: clipAt("shadow.mp4", "Shadow", filler.Commercial, 30_000), UpdatedAt: now}
	if err := s.UpsertClip(ctx, shadowClip); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipTags(ctx, shadowClip.Hash, []string{shadowTaxon.Slug}); err != nil {
		t.Fatal(err)
	}
	beforeShadow, err := s.GetClip(ctx, shadowClip.Hash)
	if err != nil || beforeShadow.Category != shadowTaxon.Slug {
		t.Fatalf("per-clip taxonomy write category = %q, err=%v; want %q", beforeShadow.Category, err, shadowTaxon.Slug)
	}
	shadowTaxon.Axis = taxonomy.AxisFormat
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Slug: shadowTaxon.Slug, Taxon: shadowTaxon}, now); err != nil {
		t.Fatal(err)
	}
	gotShadow, err := s.GetClip(ctx, shadowClip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if gotShadow.Category != "" || len(gotShadow.AssertedTags) != 1 || gotShadow.AssertedTags[0] != shadowTaxon.Slug {
		t.Errorf("axis edit left category/assertions = %q / %v, want empty / [%s]", gotShadow.Category, gotShadow.AssertedTags, shadowTaxon.Slug)
	}

	testAtomicTaxonomyEdit(t, s, ctx, now)
}

// testAtomicTaxonomyEdit pins the V55 contract: changing the graph re-derives every affected
// clip's rollups before the semantic edit returns. There is no second public rebuild operation or
// scheduled repair job that can expose two vocabulary generations.
func testAtomicTaxonomyEdit(t *testing.T, s Store, ctx context.Context, now time.Time) {
	// Insert an intermediate parent above cereal. clipA already asserts cereal, so the graph edit
	// itself must add breakfast to that existing clip while preserving its direct assertion.
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Create: true, Taxon: taxonomy.Taxon{
		Slug: "breakfast", Label: "Breakfast", Parent: "food", Axis: taxonomy.AxisProduct,
	}}, now); err != nil {
		t.Fatal(err)
	}
	cereal, ok := taxonomy.New(mustList(t, s, ctx)).Get("cereal")
	if !ok {
		t.Fatal("seed taxonomy has no cereal taxon")
	}
	cereal.Parent = "breakfast"
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Slug: cereal.Slug, Taxon: cereal}, now); err != nil {
		t.Fatal(err)
	}
	fullA, _ := s.GetClipTags(ctx, "clipA", false)
	assertSet(t, "graph edit atomically reindexes existing clip", fullA, []string{"cereal", "breakfast", "food"})
	leavesA, _ := s.GetClipTags(ctx, "clipA", true)
	assertSet(t, "graph edit preserves asserted leaves", leavesA, []string{"cereal"})

	// ⚠ A graph edit updates existing clip lineages. Insert a taxon BETWEEN an existing leaf and its
	// parent, so the rollup set genuinely changes. `pilsner` under a new `ale-family` under `beer`.
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Create: true, Taxon: taxonomy.Taxon{Slug: "ale-family", Label: "Ale family", Parent: "beer", Axis: taxonomy.AxisProduct}}, now); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Create: true, Taxon: taxonomy.Taxon{Slug: "pilsner", Label: "Pilsner", Parent: "ale-family", Axis: taxonomy.AxisProduct}}, now); err != nil {
		t.Fatal(err)
	}
	clipC := sampleClip("clipC", "clipC.mp4", filler.Commercial, 0, "", "")
	clipC.Held = true
	if err := s.UpsertClip(ctx, clipC); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipTags(ctx, "clipC", []string{"pilsner"}); err != nil {
		t.Fatal(err)
	}
	// With the ale-family hop present: pilsner → ale-family → beer → alcohol → drinks.
	afterC, _ := s.GetClipTags(ctx, "clipC", false)
	assertSet(t, "clipC rollups with ale-family hop", afterC, []string{"pilsner", "ale-family", "beer", "alcohol", "drinks"})

	// ⚠ DELETE the MIDDLE taxon: DeleteTaxon REPARENTS lager to the grandparent (beer), so the lineage
	// survives minus the removed level — it does NOT orphan lager or leave a phantom 'ale-family'
	// rollup. This is the "remove a middle category" behaviour, and the direction a stale closure OR a
	// dangling-parent Ancestors would get wrong.
	if err := s.ApplyTaxonomyEdit(ctx, TaxonomyEdit{Delete: true, Slug: "ale-family"}, now); err != nil {
		t.Fatal(err)
	}
	// Confirm the reparent landed at the graph level.
	if lg, ok := taxonomy.New(mustList(t, s, ctx)).Get("pilsner"); !ok || lg.Parent != "beer" {
		t.Fatalf("ApplyTaxonomyEdit did not reparent pilsner to beer: got parent %q (ok=%v)", lg.Parent, ok)
	}
	afterC, _ = s.GetClipTags(ctx, "clipC", false)
	// ale-family is GONE (reparented away, not a phantom ancestor); the rest of the lineage survives.
	assertSet(t, "clipC rollups after middle-taxon delete", afterC, []string{"pilsner", "beer", "alcohol", "drinks"})
	// pilsner itself (the asserted leaf) survives the graph edit.
	leavesC, _ := s.GetClipTags(ctx, "clipC", true)
	assertSet(t, "clipC leaf survives ancestor deletion", leavesC, []string{"pilsner"})
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

	seed := func(id, source string, held bool, era int) {
		c := sampleClip(id, id+".mp4", filler.Commercial, era, filler.Kids, "toys")
		c.Source = source
		c.Held = held
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	seed("a1", "youtube", false, 1990)
	seed("a2", "youtube", false, 1991)
	seed("a3", "archive", false, 0) // untagged: era 0
	seed("a4", "archive", true, 1992)
	seed("a5", "", false, 1993)

	filters := map[string]ClipFilter{
		"catalog":  {},
		"held":     {HeldOnly: true},
		"untagged": {UntaggedOnly: true},
		"by-kind":  {Role: filler.Commercial},
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
		Path: "licensed.mp4", Name: "Licensed", Role: filler.Commercial, DurationMs: 30000,
		License: "https://creativecommons.org/publicdomain/zero/1.0/",
	}, UpdatedAt: now}
	unknown := Clip{Clip: filler.Clip{
		Hash: "unknown.mp4",
		Path: "unknown.mp4", Name: "Unknown", Role: filler.Commercial, DurationMs: 30000,
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

// testClipNameSource pins that a name's provenance round-trips on BOTH backends and travels with
// the name on a re-upsert (#1988), and that a writer which records none stores 'unknown', the same
// value migration 00134 gives a row it cannot classify.
func testClipNameSource(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	named := Clip{Clip: filler.Clip{
		Hash: "named.mp4", Path: "named.mp4", Name: "Widgetmax Ad 1991", NameSource: clips.NameFromFilename,
		Role: filler.Commercial, DurationMs: 30000,
	}, UpdatedAt: now}
	unrecorded := Clip{Clip: filler.Clip{
		Hash: "unrecorded.mp4", Path: "unrecorded.mp4", Name: "Unrecorded", Role: filler.Commercial, DurationMs: 30000,
	}, UpdatedAt: now}
	for _, c := range []Clip{named, unrecorded} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	for hash, want := range map[string]clips.NameSource{
		"named.mp4": clips.NameFromFilename, "unrecorded.mp4": clips.NameFromUnknown,
	} {
		got, err := s.GetClip(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		if got.NameSource != want {
			t.Errorf("%s name source = %q, want %q", hash, got.NameSource, want)
		}
	}

	// A sidecar title found by a later scan replaces the name AND its provenance.
	named.Name, named.NameSource = "Fizzbolt Summer Sale", clips.NameFromSidecar
	if err := s.UpsertClip(ctx, named); err != nil {
		t.Fatal(err)
	}
	listed, err := s.ListClips(ctx, ClipFilter{Hashes: []string{"named.mp4"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Name != "Fizzbolt Summer Sale" || listed[0].NameSource != clips.NameFromSidecar {
		t.Fatalf("after re-upsert = %+v, want the sidecar title with its provenance", listed)
	}

	// An operator rename (#1990) writes the name and 'curated' together; a missing clip is ErrNotFound.
	if err := s.RenameClip(ctx, "named.mp4", "Fizzbolt Midnight Special", clips.NameFromCurated, now); err != nil {
		t.Fatal(err)
	}
	renamed, err := s.GetClip(ctx, "named.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "Fizzbolt Midnight Special" || renamed.NameSource != clips.NameFromCurated {
		t.Fatalf("after rename = %q (%q), want the curated name", renamed.Name, renamed.NameSource)
	}
	if err := s.RenameClip(ctx, "missing.mp4", "Nobody", clips.NameFromCurated, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rename of a missing clip = %v, want ErrNotFound", err)
	}
}

// testClipHeld covers the clip lifecycle on BOTH backends: a held clip is recorded but is not in
// the playable catalog, ordinary storage can only add a hold, and only confirmed composite repair
// can use the narrowly constrained release capability. Non-composite publication belongs to the
// terminal applied-admission transaction.
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
		Path: "filed.mp4", Name: "Filed", Role: filler.Commercial, DurationMs: 30000,
		Era: 1990, Audience: filler.Kids, Category: "toys",
	}, UpdatedAt: at}
	held := Clip{Clip: filler.Clip{
		Hash: "held.mp4",
		Path: "held.mp4", Name: "Held", Role: filler.Commercial, DurationMs: 30000,
		Era: 1990, Audience: filler.Kids, Category: "toys", Held: true,
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

	// ⚠ THE trap this lifecycle has to survive: `clips` is a synced CACHE, so the folder scan
	// re-upserts every file it finds with held=false. If `held` rode along in UpsertClip's DO
	// UPDATE list, one scan pass would file every held clip — emptying the review queue into live
	// channels with no operator action and nothing in the logs.
	rescan := held
	rescan.Held = false
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
	// The ordinary release capability cannot publish a non-composite, even when a caller asks.
	if n, err := s.ReleaseCompositeHolds(ctx, []string{"held.mp4"}, at); err != nil || n != 0 {
		t.Fatalf("release non-composite = %d, %v; want 0", n, err)
	}
	stillHeld, err := s.GetClip(ctx, "held.mp4")
	if err != nil || !stillHeld.Held {
		t.Fatalf("non-composite release bypassed admission: clip=%+v err=%v", stillHeld, err)
	}

	// Holding a playable clip is deliberately one-way.
	if _, err := s.HoldClips(ctx, []string{"filed.mp4"}, at); err != nil {
		t.Fatal(err)
	}
	catalog, err := s.ListClips(ctx, ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 0 {
		t.Fatalf("after holding both clips, catalog has %d clips, want 0", len(catalog))
	}
	withdrawn, err := s.GetClip(ctx, "filed.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if !withdrawn.Held {
		t.Errorf("withdrawn clip = held:%v, want true", withdrawn.Held)
	}

	// Composite release is retained solely to finish lineage repair. The WHERE clause itself owns
	// the constraint, so a caller cannot widen it accidentally.
	if err := s.SetClipComposite(ctx, "held.mp4", true, at); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ReleaseCompositeHolds(ctx, []string{"held.mp4"}, at); err != nil || n != 1 {
		t.Fatalf("release composite = %d, %v; want 1", n, err)
	}
	released, err := s.GetClip(ctx, "held.mp4")
	if err != nil || released.Held {
		t.Fatalf("released composite = held:%v err=%v, want false", released.Held, err)
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

	// ⚠ YouTube seeds PRESENT BUT EMPTY: the row is the operator's own channel or playlist prompt.
	// Recommended channels arrive as separate curated rows, switched off (decision 0045). The empty
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
		Label:     "Classic TV commercials",
		License:   "https://creativecommons.org/licenses/by-nc-sa/4.0/",
		Geography: pod.Geography{Country: "US"},
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
	if added[0].Geography != src.Geography {
		t.Errorf("source geography = %+v, want %+v", added[0].Geography, src.Geography)
	}
	if err := s.SetFillerSourceGeography(ctx, "src-1", pod.Geography{Country: "ca"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := findSource(t, s, "src-1"); got.Geography != (pod.Geography{Country: "CA"}) {
		t.Errorf("updated source geography = %+v", got.Geography)
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
	checked := fetched.Add(30 * time.Minute)
	claimAt, leaseUntil := fetched.Add(10*time.Minute), fetched.Add(40*time.Minute)
	claimed, err := s.ClaimFillerSourceCheck(ctx, "src-1", time.Time{}, claimAt, leaseUntil)
	if err != nil || !claimed {
		t.Fatalf("first source check claim = %v, %v", claimed, err)
	}
	if claimed, err := s.ClaimFillerSourceCheck(ctx, "src-1", time.Time{}, claimAt, leaseUntil); err != nil || claimed {
		t.Fatalf("overlapping source check claim = %v, %v, want false/nil", claimed, err)
	}
	if err := s.CompleteFillerSourceCheck(ctx, "src-1", leaseUntil.Add(time.Second), filler.SourceCheckCompletion{CheckedAt: checked}); !errors.Is(err, filler.ErrSourceCheckClaimLost) {
		t.Fatalf("stale source check completion = %v, want claim lost", err)
	}
	if err := s.CompleteFillerSourceCheck(ctx, "src-1", leaseUntil, filler.SourceCheckCompletion{CheckedAt: checked}); err != nil {
		t.Fatal(err)
	}
	if !src1(t, s).LastCheckedAt.Equal(checked) {
		t.Errorf("LastCheckedAt = %v, want %v", src1(t, s).LastCheckedAt, checked)
	}
	if !src1(t, s).LastFetchedAt.Equal(fetched) {
		t.Errorf("marking a check changed LastFetchedAt to %v, want %v", src1(t, s).LastFetchedAt, fetched)
	}

	// A provider failure is durably recorded and releases the claim into bounded backoff. The
	// next successful completion clears it.
	failedClaimAt, failedLease := checked.Add(time.Hour), checked.Add(90*time.Minute)
	claimed, err = s.ClaimFillerSourceCheck(ctx, "src-1", checked, failedClaimAt, failedLease)
	if err != nil || !claimed {
		t.Fatalf("failure source check claim = %v, %v", claimed, err)
	}
	retryAt := failedClaimAt.Add(5 * time.Minute)
	if err := s.FailFillerSourceCheck(ctx, "src-1", failedLease, retryAt); err != nil {
		t.Fatal(err)
	}
	failedSource := src1(t, s)
	if failedSource.CheckFailureCount != 1 || !failedSource.CheckRetryAt.Equal(retryAt) || !failedSource.CheckLeaseUntil.IsZero() {
		t.Fatalf("durable source failure state = %+v", failedSource)
	}
	retryLease := retryAt.Add(30 * time.Minute)
	claimed, err = s.ClaimFillerSourceCheck(ctx, "src-1", checked, retryAt, retryLease)
	if err != nil || !claimed {
		t.Fatalf("retry source check claim = %v, %v", claimed, err)
	}
	checked = retryAt.Add(time.Minute)
	if err := s.CompleteFillerSourceCheck(ctx, "src-1", retryLease, filler.SourceCheckCompletion{CheckedAt: checked}); err != nil {
		t.Fatal(err)
	}
	recoveredSource := src1(t, s)
	if recoveredSource.CheckFailureCount != 0 || !recoveredSource.CheckRetryAt.IsZero() {
		t.Fatalf("successful source check did not clear backoff: %+v", recoveredSource)
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
	if !src1(t, s).LastCheckedAt.Equal(checked) {
		t.Errorf("re-registering reset LastCheckedAt to %v — it must survive an upsert", src1(t, s).LastCheckedAt)
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
		Path: "from-src-1.mp4", Name: "From src 1", Role: filler.Commercial, DurationMs: 30000,
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

	// Restart durability is part of the scheduler contract, not merely a same-process read.
	// Persist all three timing facts plus a source override, close the production store, reopen
	// the same database, and prove the next process sees the exact state on both SQL backends.
	restartSource := NewFillerSource("restart-policy", "youtube", "https://www.youtube.com/@restart/videos", "Restart policy", created)
	if err := s.UpsertFillerSource(ctx, restartSource); err != nil {
		t.Fatal(err)
	}
	restartEvery, restartMax := 12*60*60, 7
	if err := s.SetFillerSourceFetchPolicy(ctx, restartSource.ID, &restartEvery, &restartMax); err != nil {
		t.Fatal(err)
	}
	restartChecked := created.Add(12 * time.Hour)
	restartLease := restartChecked.Add(-time.Minute)
	claimed, err = s.ClaimFillerSourceCheck(ctx, restartSource.ID, time.Time{}, restartChecked.Add(-2*time.Minute), restartLease)
	if err != nil || !claimed {
		t.Fatalf("restart source success claim = %v, %v", claimed, err)
	}
	if err := s.CompleteFillerSourceCheck(ctx, restartSource.ID, restartLease, filler.SourceCheckCompletion{
		CheckedAt: restartChecked,
		Checkpoint: source.Checkpoint{
			Cursor: "video-10", PendingWatermark: "video-1", Watermark: "prior-video",
		},
		ReplaceOutcomes: true,
		Outcomes: source.Outcomes{
			source.OutcomeQueued:      3,
			source.OutcomeTooLong:     12,
			source.OutcomeUnavailable: 2,
		},
	}); err != nil {
		t.Fatal(err)
	}
	continuedAt := restartChecked.Add(time.Minute)
	continuedLease := continuedAt.Add(30 * time.Minute)
	claimed, err = s.ClaimFillerSourceCheck(ctx, restartSource.ID, restartChecked, continuedAt, continuedLease)
	if err != nil || !claimed {
		t.Fatalf("continuation source check claim = %v, %v", claimed, err)
	}
	if err := s.CompleteFillerSourceCheck(ctx, restartSource.ID, continuedLease, filler.SourceCheckCompletion{
		CheckedAt:  continuedAt,
		Checkpoint: source.Checkpoint{Watermark: "video-1"},
		Outcomes:   source.Outcomes{source.OutcomeTooShort: 2},
	}); err != nil {
		t.Fatal(err)
	}
	restartChecked = continuedAt
	continuedSource, _ := findSource(t, s, restartSource.ID)
	if continuedSource.LastCheckSummary[source.OutcomeQueued] != 3 ||
		continuedSource.LastCheckSummary[source.OutcomeTooShort] != 2 ||
		continuedSource.ScanCheckpoint != (source.Checkpoint{Watermark: "video-1"}) {
		t.Fatalf("continued source summary/checkpoint = %+v / %+v", continuedSource.LastCheckSummary, continuedSource.ScanCheckpoint)
	}
	failureAt := restartChecked.Add(time.Hour)
	failureLease := failureAt.Add(30 * time.Minute)
	claimed, err = s.ClaimFillerSourceCheck(ctx, restartSource.ID, restartChecked, failureAt, failureLease)
	if err != nil || !claimed {
		t.Fatalf("restart source failure claim = %v, %v", claimed, err)
	}
	restartRetry := failureAt.Add(5 * time.Minute)
	if err := s.FailFillerSourceCheck(ctx, restartSource.ID, failureLease, restartRetry); err != nil {
		t.Fatal(err)
	}

	reopened := restartConformanceStore(t, s)
	restarted, ok := findSource(t, reopened, restartSource.ID)
	if !ok {
		_ = reopened.Close()
		t.Fatal("source check state disappeared across store restart")
	}
	if restarted.FetchEverySeconds == nil || *restarted.FetchEverySeconds != restartEvery ||
		restarted.FetchMaxPerRun == nil || *restarted.FetchMaxPerRun != restartMax ||
		!restarted.LastCheckedAt.Equal(restartChecked) || restarted.CheckFailureCount != 1 ||
		!restarted.CheckRetryAt.Equal(restartRetry) || !restarted.CheckLeaseUntil.IsZero() ||
		restarted.ScanCheckpoint != (source.Checkpoint{Watermark: "video-1"}) ||
		restarted.LastCheckSummary[source.OutcomeQueued] != 3 ||
		restarted.LastCheckSummary[source.OutcomeTooShort] != 2 ||
		restarted.LastCheckSummary[source.OutcomeTooLong] != 12 ||
		restarted.LastCheckSummary[source.OutcomeUnavailable] != 2 {
		_ = reopened.Close()
		t.Fatalf("source policy/check state after restart = %+v", restarted)
	}
	newSweepAt, newSweepLease := restartRetry, restartRetry.Add(30*time.Minute)
	claimed, err = reopened.ClaimFillerSourceCheck(ctx, restartSource.ID, restartChecked, newSweepAt, newSweepLease)
	if err != nil || !claimed {
		_ = reopened.Close()
		t.Fatalf("new sweep claim after restart = %v, %v", claimed, err)
	}
	if err := reopened.CompleteFillerSourceCheck(ctx, restartSource.ID, newSweepLease, filler.SourceCheckCompletion{
		CheckedAt: newSweepAt, Checkpoint: source.Checkpoint{Watermark: "new-video"},
		ReplaceOutcomes: true,
		Outcomes:        source.Outcomes{source.OutcomeLive: 1},
	}); err != nil {
		_ = reopened.Close()
		t.Fatal(err)
	}
	replaced, _ := findSource(t, reopened, restartSource.ID)
	if len(replaced.LastCheckSummary) != 1 || replaced.LastCheckSummary[source.OutcomeLive] != 1 {
		_ = reopened.Close()
		t.Fatalf("new sweep outcomes = %+v, want only live:1", replaced.LastCheckSummary)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

// testFillerProviderPolicy protects the master-switch contract through the store's public seam on
// both SQL adapters. A provider pause changes effective acquisition policy without rewriting the
// operator's child choices, so resuming restores the exact mix that existed before the pause.
func testFillerProviderPolicy(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)

	providers, err := s.ListFillerProviders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !providerEnabled(providers, "archive") || !providerEnabled(providers, "youtube") {
		t.Fatalf("fresh provider policy = %+v, want archive and youtube enabled", providers)
	}

	if err := s.UpsertFillerSource(ctx, NewFillerSource("provider-policy-on", "archive", "provider_policy_on", "On", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	off := NewFillerSource("provider-policy-off", "archive", "provider_policy_off", "Off", time.Now().UTC().Add(time.Second))
	if err := s.UpsertFillerSource(ctx, off); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFillerSourceEnabled(ctx, off.ID, false); err != nil {
		t.Fatal(err)
	}

	if err := s.SetFillerProviderEnabled(ctx, "archive", false); err != nil {
		t.Fatal(err)
	}
	sources, err := s.ListFillerSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]FillerSource, len(sources))
	for _, source := range sources {
		byID[source.ID] = source
	}
	if !byID["provider-policy-on"].Enabled || byID["provider-policy-on"].EffectiveEnabled() {
		t.Errorf("enabled child while provider paused = %+v, want choice retained but effective state off", byID["provider-policy-on"])
	}
	if byID["provider-policy-off"].Enabled || byID["provider-policy-off"].EffectiveEnabled() {
		t.Errorf("disabled child while provider paused = %+v, want child and effective state off", byID["provider-policy-off"])
	}

	if err := s.SetFillerProviderEnabled(ctx, "archive", true); err != nil {
		t.Fatal(err)
	}
	sources, err = s.ListFillerSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID = make(map[string]FillerSource, len(sources))
	for _, source := range sources {
		byID[source.ID] = source
	}
	if !byID["provider-policy-on"].EffectiveEnabled() {
		t.Error("resuming archive did not restore its previously enabled child")
	}
	if byID["provider-policy-off"].EffectiveEnabled() {
		t.Error("resuming archive enabled a child the operator had left off")
	}
	if err := s.SetFillerProviderEnabled(ctx, "not-a-provider", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("set unknown provider = %v, want ErrNotFound", err)
	}
}

func providerEnabled(providers []FillerProvider, kind string) bool {
	for _, provider := range providers {
		if provider.Kind == kind {
			return provider.Enabled
		}
	}
	return false
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
	// oversight: it prompts for the operator's own playlist, while recommended channels are separate
	// curated rows (decision 0045). An empty uri also keeps the row out of every pull plan on its own,
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
		Intent: filler.AcquisitionIntent{Version: filler.AcquisitionIntentVersion, EraStart: 1990, EraEnd: 1999, Count: 2},
		Plan: []filler.PullPlanRow{
			{SourceID: "classic", Provider: "archive", RemoteID: "ad-one", URL: "https://archive.org/details/ad-one", Tag: "1990s", Name: "Classic TV commercials", Why: "Era match", EstimateClips: 1},
			{SourceID: "psa", Provider: "archive", RemoteID: "psa-one", URL: "https://archive.org/details/psa-one", Tag: "psa", Name: "Public service", Why: "Filler variety", EstimateClips: 1},
		},
		Rejected: []filler.AcquisitionDecision{{Candidate: filler.AcquisitionCandidate{Identity: filler.RemoteIdentity{Provider: "archive", SourceID: "classic", RemoteID: "old"}, URL: "https://archive.org/details/old"}, Disposition: filler.CandidateAlreadyCatalogued, Detail: "already present"}},
		Sources:  []filler.AcquisitionSourceDecision{{SourceID: "classic", Provider: "archive", Disposition: filler.AcquisitionSourceEnumerated, CandidateCount: 2, Detail: "enumerated metadata"}},
	}
	if err := s.UpsertPull(ctx, p); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetPull(ctx, "pull_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan) != 2 || got.Plan[0].RemoteID != "ad-one" || got.Plan[0].EstimateClips != 1 {
		t.Errorf("plan did not round-trip: %+v", got.Plan)
	}
	if got.Intent.EraStart != 1990 || len(got.Rejected) != 1 || got.Rejected[0].Disposition != filler.CandidateAlreadyCatalogued || len(got.Sources) != 1 {
		t.Errorf("intent/evidence did not round-trip: %+v", got)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, created)
	}
	// Pending means undecided, and that must be legible as a ZERO time rather than an epoch
	// date nobody meant.
	if !got.DecidedAt.IsZero() {
		t.Errorf("a pending pull has DecidedAt %v, want zero", got.DecidedAt)
	}
	if got.EstimatedClips() != 2 {
		t.Errorf("EstimatedClips = %d, want 2", got.EstimatedClips())
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
	if after.EstimatedClips() != 1 {
		t.Errorf("EstimatedClips after drop = %d, want 1", after.EstimatedClips())
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

	source := split.SourceAsset{
		Role: split.SourceEvidence, SHA256: strings.Repeat("a", 64), Bytes: 42,
		ClipHash: strings.Repeat("b", 64), Path: ".loomarr-media/evidence/aa/bb/evidence.mp4", DurationMs: 149_000,
	}
	assessed, err := split.Assess(split.Input{
		Source: source, AssessedAt: now,
		Observations: []split.Observation{
			{ID: "black", Kind: split.ObservationBlackInterval, Effect: split.ObservationProposesBoundary, StartMs: 29_900, EndMs: 30_100, Producer: "fixture:v1", EvidenceSHA256: strings.Repeat("c", 64)},
			{ID: "silence", Kind: split.ObservationSilenceInterval, Effect: split.ObservationProposesBoundary, StartMs: 29_900, EndMs: 30_100, Producer: "fixture:v1", EvidenceSHA256: strings.Repeat("d", 64)},
			{ID: "role-left", Kind: split.ObservationOCRLogoChange, Effect: split.ObservationSupportsBoundary, StartMs: 30_000, EndMs: 30_000, Producer: "fixture:v1", EvidenceSHA256: strings.Repeat("e", 64)},
			{ID: "role-right", Kind: split.ObservationTranscriptChange, Effect: split.ObservationSupportsBoundary, StartMs: 30_000, EndMs: 30_000, Producer: "fixture:v1", EvidenceSHA256: strings.Repeat("f", 64)},
		},
		RoleClaims: []split.RoleClaim{
			{StartMs: 0, EndMs: 30_000, Role: split.RoleCommercial, EvidenceIDs: []string{"role-left"}, Reason: "commercial evidence"},
			{StartMs: 30_000, EndMs: 149_000, Role: split.RolePromo, EvidenceIDs: []string{"role-right"}, Reason: "promo evidence"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	roleEvidence, err := split.NewRoleEvidence(split.RoleEvidenceInput{
		Source: source, StartMs: 0, EndMs: 30_000, Role: split.RoleCommercial,
		Reason: "product offer and closing brand card", Frames: [][]byte{[]byte("opening"), []byte("closing")},
		PromptVersion: "filler-vision-grounding-v2", Prompt: "bounded segment prompt", Response: `{"role":"commercial"}`,
		RequestedProvider: "ollama", ResolvedProvider: "ollama", RequestedModel: "vision", ResolvedModel: "vision@sha256:fixture",
		Modalities: []string{"image", "text"}, Tokens: split.RoleTokenUsage{Prompt: 20, Completion: 5, Image: 2},
		LatencyMs: 100, Attempts: 1, AssessedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	videoRoleEvidence, err := split.NewRoleEvidence(split.RoleEvidenceInput{
		Source: source, StartMs: 30_000, EndMs: 61_000, Role: split.RolePromo,
		Reason: "complete sequence promotes a programme", Video: []byte("bounded-video-derivative"),
		PromptVersion: "filler-segment-video-role-v1", Prompt: "bounded video prompt", Response: `{"role":"promo"}`,
		RequestedProvider: "openrouter", ResolvedProvider: "openrouter", RequestedModel: "video", ResolvedModel: "video",
		Modalities: []string{"audio", "text", "video"}, Tokens: split.RoleTokenUsage{Prompt: 30, Completion: 5, Video: 4, Audio: 2},
		LatencyMs: 200, Attempts: 1, GenerationID: "video-generation", AssessedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	decisionSource := structure.Source{SHA256: source.SHA256, Bytes: source.Bytes, DurationMS: source.DurationMs}
	structureMedia := structure.AssessmentMedia{SHA256: strings.Repeat("6", 64), Bytes: source.Bytes, DurationMS: source.DurationMs, ProfileSHA256: strings.Repeat("5", 64), LineageSHA256: strings.Repeat("4", 64)}
	structureInput, err := structure.NewCompleteVideoInput(decisionSource, structureMedia)
	if err != nil {
		t.Fatal(err)
	}
	structureCandidate := func(id, family, assessmentDigest string) structure.Candidate {
		return structure.Candidate{
			Source: decisionSource, InputSHA256: structureInput.SHA256,
			Assessor: structure.Assessor{
				ID: id, ModelFamily: family, Provider: "fixture-provider", Model: "fixture-model",
				ModelDigest: strings.Repeat("8", 64), CapabilitySHA256: strings.Repeat("7", 64),
				PromptVersion: "structure-prompt-v1", EvidenceContract: "assessment-v1",
				AssessmentSHA256: assessmentDigest,
			},
			Unit: structure.UnitCompilation,
			Segments: []structure.Segment{
				{StartMS: 0, EndMS: 30_000, Role: structure.RoleCommercial},
				{StartMS: 30_000, EndMS: 149_000, Role: structure.RolePromo},
			},
		}
	}
	structureDecision, err := structure.NewArtifact(structure.Request{
		Source: decisionSource, Input: structureInput, BoundaryToleranceMS: 2_000,
		Candidates: []structure.Candidate{
			structureCandidate("structure-assessor-a", "structure-family-a", strings.Repeat("9", 64)),
			structureCandidate("structure-assessor-b", "structure-family-b", strings.Repeat("a", 64)),
		},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	p := filler.SplitProposal{
		ID: "sp_1", ClipHash: clipHashFor("comps/1987.mp4"), CreatedAt: now,
		Source: source, Structure: &assessed, StructureDecision: &structureDecision,
		ArtworkPrepared:    true,
		LanguagePreference: "en",
		LanguageExclusions: []filler.SplitLanguageExclusion{{StartMs: 149_000, EndMs: 179_000, Name: "Polish advert", DetectedLanguage: "pl", ExpectedLanguage: "en", Reason: filler.SplitExclusionLanguageMismatch}},
		Segments: []split.Segment{
			{Index: 0, StartMs: 0, EndMs: 30000, Name: "comps/1987 part 1", Era: 1987, Audience: filler.Kids, Category: "toys", RoleEvidence: &roleEvidence, Language: "en", LanguageChecked: true, ArtworkChecked: true, ArtworkImageHash: "preview-one"},
			{Index: 1, StartMs: 30000, EndMs: 61000, Name: "unknown", SuggestedEra: 1985, DupOf: "old/ad.mp4", Looked: true, RoleEvidence: &videoRoleEvidence, ArtworkChecked: true},
			{Index: 2, StartMs: 61000, EndMs: 149000, Name: "comps/1987 part 3", Unsplittable: true, Transcript: "[00:00] …", LanguageChecked: true, LanguageReason: filler.SplitLanguageFailed, LanguageNote: "Language could not be checked"},
		},
	}
	if err := s.UpsertSplitProposal(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSplitProposal(ctx, "sp_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ClipHash != p.ClipHash || got.Source != p.Source || !reflect.DeepEqual(got.Structure, p.Structure) || !reflect.DeepEqual(got.StructureDecision, p.StructureDecision) || got.LanguagePreference != "en" || !reflect.DeepEqual(got.LanguageExclusions, p.LanguageExclusions) || len(got.Segments) != 3 || !got.CreatedAt.Equal(now) {
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
	if got.Segments[0].Language != "en" || !got.Segments[0].LanguageChecked || !got.Segments[2].LanguageChecked || got.Segments[2].LanguageReason != filler.SplitLanguageFailed || got.Segments[2].LanguageNote == "" {
		t.Errorf("pre-review language fields lost: %+v / %+v", got.Segments[0], got.Segments[2])
	}
	if !reflect.DeepEqual(got.Segments[0].RoleEvidence, &roleEvidence) {
		t.Errorf("segment role evidence lost: %+v", got.Segments[0].RoleEvidence)
	}
	if !reflect.DeepEqual(got.Segments[1].RoleEvidence, &videoRoleEvidence) || got.Segments[1].RoleEvidence.VideoSHA256 == "" {
		t.Errorf("segment video role evidence lost: %+v", got.Segments[1].RoleEvidence)
	}
	if !got.ArtworkPrepared || !got.Segments[0].ArtworkChecked || got.Segments[0].ArtworkImageHash != "preview-one" || !got.Segments[1].ArtworkChecked || got.Segments[1].ArtworkImageHash != "" {
		t.Errorf("segment artwork state lost: prepared=%v segments=%+v", got.ArtworkPrepared, got.Segments)
	}

	draft := filler.SplitProposal{
		ID: "sp_draft", ClipHash: clipHashFor("comps/long.mp4"), CreatedAt: now.Add(time.Minute),
		Language: &filler.SplitLanguageProgress{Want: "en", Next: 1},
		Detection: &split.DetectionProgress{
			ScannedThroughMs: 600_000,
			Black:            []filler.Interval{{StartMs: 29_900, EndMs: 30_100}},
			ChapterEdges:     []int64{0, 30_000, 33_000, 600_000},
			Discarded:        []filler.Interval{{StartMs: 30_000, EndMs: 33_000}},
		},
	}
	if err := s.UpsertSplitProposal(ctx, draft); err != nil {
		t.Fatal(err)
	}
	gotDraft, err := s.GetSplitProposal(ctx, draft.ID)
	if err != nil || gotDraft.Ready() || gotDraft.Language == nil || gotDraft.Language.Want != "en" || gotDraft.Language.Next != 1 || gotDraft.Detection.ScannedThroughMs != 600_000 || len(gotDraft.Detection.Black) != 1 || len(gotDraft.Detection.ChapterEdges) != 4 || len(gotDraft.Detection.Discarded) != 1 {
		t.Fatalf("detector checkpoint round-trip = (%+v, %v)", gotDraft, err)
	}
	if err := s.DeleteSplitProposal(ctx, draft.ID); err != nil {
		t.Fatal(err)
	}

	// ⚠ Re-detection REPLACES the pending proposal for the same clip — two competing
	// cut-lists for one file is a review bug, not a choice. The NEW id answers the old
	// one's GET with ErrNotFound.
	p2 := filler.SplitProposal{ID: "sp_2", ClipHash: p.ClipHash, CreatedAt: now.Add(time.Hour),
		Segments: []split.Segment{{Index: 0, StartMs: 0, EndMs: 149000, Name: "whole"}}}
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

	// --- UpdateSplitProposal: grounding and partial output accumulate across passes (§10 V54) ---
	//
	// Split-time grounding is a read-modify-write spanning MINUTES of vision calls, so the write
	// races `Confirm`. Two properties are pinned here, and the second is the load-bearing one.
	p3 := filler.SplitProposal{
		ID: "sp_3", ClipHash: "h:comps/1991.mp4", CreatedAt: now,
		Segments: []split.Segment{
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
	grounded := []split.Segment{
		{Index: 0, StartMs: 0, EndMs: 30_000, Name: "one", Looked: true, Category: "toys", Era: 1991},
		{Index: 1, StartMs: 30_000, EndMs: 61_000, Name: "two", Looked: true},
	}
	p3.Segments = grounded
	p3.Spawned = []string{"new-child"}
	if err := s.UpdateSplitProposal(ctx, p3); err != nil {
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
	if len(got3.Spawned) != 1 || got3.Spawned[0] != "new-child" {
		t.Errorf("partial-confirm output lost in round-trip: %+v", got3.Spawned)
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
	if err := s.UpdateSplitProposal(ctx, p3); !errors.Is(err, ErrNotFound) {
		t.Errorf("update after delete = %v, want ErrNotFound", err)
	}
	if _, err := s.GetSplitProposal(ctx, "sp_3"); !errors.Is(err, ErrNotFound) {
		t.Error("a grounding write RESURRECTED a confirmed proposal — the reel would be reviewed twice")
	}
	p3.ID = "sp_never_existed"
	if err := s.UpdateSplitProposal(ctx, p3); !errors.Is(err, ErrNotFound) {
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
	seg := []split.Segment{{Index: 0, StartMs: 0, EndMs: 30_000}}
	for id, hash := range map[string]string{"sp_keep": keeper.Hash, "sp_orphan": orphan.Hash} {
		if err := s.UpsertSplitProposal(ctx, filler.SplitProposal{
			ID: id, ClipHash: hash, CreatedAt: now, Segments: seg,
		}); err != nil {
			t.Fatal(err)
		}
	}
	preview := Image{
		Hash: "split-preview", Origin: "extracted", Visibility: "member", Role: "thumb",
		MIME: "image/jpeg", Width: 320, Height: 180, Bytes: 12,
		CreatedAt: now, UpdatedAt: now, LastUsedAt: now,
	}
	if err := s.PutImage(ctx, preview); err != nil {
		t.Fatal(err)
	}
	for _, proposalID := range []string{"sp_keep", "sp_orphan"} {
		if err := s.PutImageRef(ctx, ImageRef{
			ImageHash: preview.Hash, OwnerKind: "filler_split_proposal", OwnerID: proposalID, Role: "thumb",
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
	if images, err := s.ImagesForOwner(ctx, "filler_split_proposal", "sp_orphan"); err != nil || len(images) != 0 {
		t.Errorf("orphan proposal kept its image references: images=%+v err=%v", images, err)
	}
	if images, err := s.ImagesForOwner(ctx, "filler_split_proposal", "sp_keep"); err != nil || len(images) != 1 {
		t.Errorf("live proposal lost its image references: images=%+v err=%v", images, err)
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

	p := pipelinestate.ClipPipeline{
		ClipHash: clip.Hash, Stage: pipelinestate.StageVision, Status: pipelinestate.StatusRunning,
		Progress: 40, Disposition: pipelinestate.DispositionRunning,
		Attempts: 1, ForceRun: true, NextRun: now, EnrolledAt: now, UpdatedAt: now,
		PreparationAttempt: 2, PreparationStartedAt: now.Add(-5 * time.Minute),
		PreparationStartReason: pipelinestate.PreparationStartedByRestart, PreparationProgress: 61,
		StageQueuedAt: now.Add(-2 * time.Minute), StageStartedAt: now.Add(-time.Minute),
		Stages: []pipelinestate.StageRecord{
			{Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusDone, StartedAt: now.Add(-4 * time.Minute), At: now},
			{Stage: pipelinestate.StageTranscribe, Status: pipelinestate.StatusSkipped, Note: "the description already says enough", At: now},
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
	if got.Stage != pipelinestate.StageVision || got.Status != pipelinestate.StatusRunning || got.Progress != 40 || got.Attempts != 1 {
		t.Errorf("header round-trip lost fields: %+v", got)
	}
	if !got.ForceRun {
		t.Error("pipeline round-trip lost the explicit rerun marker")
	}
	if got.PreparationAttempt != 2 || got.PreparationProgress != 61 ||
		got.PreparationStartReason != pipelinestate.PreparationStartedByRestart ||
		!got.PreparationStartedAt.Equal(p.PreparationStartedAt) || !got.StageQueuedAt.Equal(p.StageQueuedAt) ||
		!got.StageStartedAt.Equal(p.StageStartedAt) || !got.Stages[0].StartedAt.Equal(p.Stages[0].StartedAt) {
		t.Errorf("preparation evidence round-trip lost fields: %+v", got)
	}
	// The LADDER is what the Incoming tab renders as history — including WHY a stage was skipped.
	// A skip with no note reads as "nothing happened", which is a different and false claim.
	if len(got.Stages) != 2 || got.Stages[1].Status != pipelinestate.StatusSkipped ||
		got.Stages[1].Note != "the description already says enough" {
		t.Errorf("ladder round-trip = %+v", got.Stages)
	}
	preparation, err := s.ListPreparationWork(ctx, pipelinestate.PipelineFilter{Dispositions: []pipelinestate.Disposition{pipelinestate.DispositionRunning}, Limit: 1})
	if err != nil || len(preparation) != 1 || preparation[0].Pipeline.ClipHash != clip.Hash || preparation[0].DurationMs != 30_000 {
		t.Fatalf("ListPreparationWork = %+v, err=%v", preparation, err)
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
	for _, d := range []pipelinestate.Disposition{
		pipelinestate.DispositionReview, pipelinestate.DispositionReady, pipelinestate.DispositionComplete, pipelinestate.DispositionRejected,
	} {
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
	rej.Disposition = pipelinestate.DispositionRejected
	rej.RejectReason = pipelinestate.ReasonTooShort
	rej.RejectDetail = "8.2s; floor is 10s"
	if err := s.UpsertClipPipeline(ctx, rej); err != nil {
		t.Fatal(err)
	}
	rejected, err := s.ListClipPipelines(ctx, pipelinestate.PipelineFilter{
		Dispositions: []pipelinestate.Disposition{pipelinestate.DispositionRejected},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rejected) != 1 || rejected[0].RejectReason != pipelinestate.ReasonTooShort ||
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
	kept := pipelinestate.ClipPipeline{
		ClipHash: keeper.Hash, Stage: pipelinestate.StageProbe, Status: pipelinestate.StatusQueued,
		Disposition: pipelinestate.DispositionRunning, EnrolledAt: now, UpdatedAt: now,
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
		created  int64
	}
	rows := []row{
		{"p1", "apple juice", 30000, 5, 1_700_000_100},
		{"p2", "Zeppelin Ad", 30000, 5, 1_700_000_200}, // ties p1 on duration/plays
		{"p3", "banana split", 15000, 0, 1_700_000_300},
		{"p4", "Banana bread", 15000, 0, 1_700_000_300}, // ties p3 on EVERYTHING, created included
		{"p5", "cereal, morning", 60000, 12, 1_700_000_400},
		{"p6", "Cereal, evening", 45000, 12, 1_700_000_500},
		{"p7", "zzz last", 5000, 1, 1_700_000_600},
		{"p8", "AAA first", 90000, 99, 1_700_000_700},
		{"p9", "middle of the road", 30000, 5, 1_700_000_800},
	}
	for _, r := range rows {
		c := sampleClip(r.id, r.name, filler.Commercial, 1990, filler.Kids, "toys")
		c.DurationMs = r.duration
		c.PlayCount = r.plays
		c.CreatedAt = time.Unix(r.created, 0).UTC()
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	sorts := []ClipSort{"", ClipSortName, ClipSortDuration, ClipSortAdded, ClipSortPlays}

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
	branded.Brand = "Brightfield's"
	seen := sampleClip("s3", "silent visual", filler.Commercial, 1994, filler.Kids, "cereal")
	seen.VisibleText = "VENTOR TOUGH"
	spoken := sampleClip("s4", "chatty spot", filler.Commercial, 1994, filler.Kids, "cereal")
	spoken.Transcript = "every inventor drives a Ventor this weekend"
	for _, c := range []Clip{named, branded, seen, spoken} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	// ⚠ A REAL slug from the seeded forest, tagged through the real writer — a hand-inserted
	// clip_tags row would not carry the rollups, and the search must match those too.
	if err := s.SetClipTags(ctx, "s1", []string{"cereal"}); err != nil {
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
		if got := hits(t, ClipFilter{Query: "brightfield"}); !sameHashes(got, []string{"s2"}) {
			t.Errorf("brand search = %v, want [s2] — a catalog that cannot find a clip by its "+
				"advertiser is a search box that looks broken", got)
		}
	})
	t.Run("MatchesVisibleText", func(t *testing.T) {
		if got := hits(t, ClipFilter{Query: "ventor tough"}); !sameHashes(got, []string{"s3"}) {
			t.Errorf("visible-text search = %v, want [s3]", got)
		}
	})
	t.Run("MatchesTags", func(t *testing.T) {
		if got := hits(t, ClipFilter{Query: "cereal"}); !sameHashes(got, []string{"s1"}) {
			t.Errorf("tag search = %v, want [s1] exactly ONCE — a JOIN here would return the clip "+
				"per matching tag and make the total disagree with the rows", got)
		}
	})
	// ⚠ The transcript is opt-in, and this pair is what proves the flag does something. "inventor"
	// contains "ventor", which is also the noise argument for keeping it opt-in.
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

	brk := sampleClip("brk", "WKTL break 1996", filler.Commercial, 1996, filler.General, "")
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

func testFillerProgressiveEnrichment(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Unix(1_700_000_000, 0).UTC()
	clip := sampleClip("enrichment-clip", "Brambly Sauce Advert", filler.Commercial, 0, "", "")
	clip.UpdatedAt = at
	if err := s.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	evidence := func(kind enrichment.EvidenceKind, confidence int) enrichment.Evidence {
		return enrichment.Evidence{Kind: kind, Reference: "fixture", Confidence: confidence,
			Producer: "conformance", ProducerVersion: "1", ObservedAt: at}
	}
	item := enrichment.State{ClipHash: clip.Hash, Axis: enrichment.AxisEra,
		Status: enrichment.StatusComplete, Value: enrichment.Value{Year: 1999},
		Evidence: evidence(enrichment.EvidenceItem, 90)}
	accepted, changed, err := s.ApplyFillerEnrichment(ctx, item, at)
	if err != nil || !changed || accepted.Value.Year != 1999 {
		t.Fatalf("first apply = %+v, changed %v, err %v", accepted, changed, err)
	}
	if _, changed, err := s.ApplyFillerEnrichment(ctx, item, at.Add(time.Second)); err != nil || changed {
		t.Fatalf("idempotent apply changed = %v, err %v", changed, err)
	}
	weak := item
	weak.Value.Year = 1980
	weak.Evidence = evidence(enrichment.EvidenceInference, 100)
	accepted, changed, err = s.ApplyFillerEnrichment(ctx, weak, at.Add(2*time.Second))
	if err != nil || changed || accepted.Value.Year != 1999 {
		t.Fatalf("weak inference apply = %+v, changed %v, err %v", accepted, changed, err)
	}
	brand := enrichment.State{ClipHash: clip.Hash, Axis: enrichment.AxisBrand,
		Status: enrichment.StatusComplete, Value: enrichment.Value{Text: "Brambly Sauce"},
		Evidence: evidence(enrichment.EvidenceItem, 100)}
	if _, changed, err := s.ApplyFillerEnrichment(ctx, brand, at.Add(3*time.Second)); err != nil || !changed {
		t.Fatalf("brand apply changed = %v, err %v", changed, err)
	}
	candidates, err := s.ListFillerEnrichmentCandidates(ctx, "deterministic-metadata", "1", "seed-v2", 10)
	if err != nil || len(candidates) != 1 || candidates[0].Hash != clip.Hash {
		t.Fatalf("candidates before pass = %+v, err %v", candidates, err)
	}
	pass := enrichment.Pass{ClipHash: clip.Hash, Producer: "deterministic-metadata", ProducerVersion: "1",
		TaxonomyVersion: "seed-v2", CompletedAt: at.Add(4 * time.Second), States: []enrichment.State{item, brand, {
			ClipHash: clip.Hash, Axis: enrichment.AxisProduct, Status: enrichment.StatusComplete,
			Value: enrichment.Value{Tags: []string{"condiments"}}, Evidence: enrichment.Evidence{
				Kind: enrichment.EvidenceTrustedMap, Reference: "trusted.product_mapping:brambly sauce", Confidence: 100,
				Producer: "deterministic-metadata", ProducerVersion: "1", TaxonomyVersion: "seed-v2", ObservedAt: at,
			},
		}}}
	pass.States[0].Evidence.Producer = pass.Producer
	pass.States[0].Evidence.ProducerVersion = pass.ProducerVersion
	pass.States[1].Evidence.Producer = pass.Producer
	pass.States[1].Evidence.ProducerVersion = pass.ProducerVersion
	if changed, err := s.ApplyFillerEnrichmentPass(ctx, pass); err != nil || changed != 1 {
		t.Fatalf("pass changed = %d, err %v; only the new product axis should change", changed, err)
	}
	candidates, err = s.ListFillerEnrichmentCandidates(ctx, pass.Producer, pass.ProducerVersion, pass.TaxonomyVersion, 10)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("candidates after pass = %+v, err %v", candidates, err)
	}
	rescanned, err := s.GetClip(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	rescanned.UpdatedAt = at.Add(5 * time.Second)
	if err := s.UpsertClip(ctx, rescanned); err != nil {
		t.Fatal(err)
	}
	candidates, err = s.ListFillerEnrichmentCandidates(ctx, pass.Producer, pass.ProducerVersion, pass.TaxonomyVersion, 10)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("candidates after a routine catalog rescan = %+v, err %v", candidates, err)
	}
	if err := s.SetClipTranscript(ctx, clip.Path, "Brambly makes everything taste better", at.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	candidates, err = s.ListFillerEnrichmentCandidates(ctx, pass.Producer, pass.ProducerVersion, pass.TaxonomyVersion, 10)
	if err != nil || len(candidates) != 1 || candidates[0].Hash != clip.Hash {
		t.Fatalf("candidates after a new transcript = %+v, err %v", candidates, err)
	}
	pass.CompletedAt = at.Add(7 * time.Second)
	if changed, err := s.ApplyFillerEnrichmentPass(ctx, pass); err != nil || changed != 0 {
		t.Fatalf("refreshed pass changed = %d, err %v", changed, err)
	}
	if err := s.SetClipTranscript(ctx, clip.Path, "Brambly makes everything taste better", at.Add(8*time.Second)); err != nil {
		t.Fatal(err)
	}
	candidates, err = s.ListFillerEnrichmentCandidates(ctx, pass.Producer, pass.ProducerVersion, pass.TaxonomyVersion, 10)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("candidates after the same transcript = %+v, err %v", candidates, err)
	}
	if err := s.ApplyClipVision(ctx, clip.Hash, clip.Path, "", "BRAMBLY SAUCE", 0, nil, at.Add(9*time.Second)); err != nil {
		t.Fatal(err)
	}
	candidates, err = s.ListFillerEnrichmentCandidates(ctx, pass.Producer, pass.ProducerVersion, pass.TaxonomyVersion, 10)
	if err != nil || len(candidates) != 1 || candidates[0].Hash != clip.Hash {
		t.Fatalf("candidates after new frame text = %+v, err %v", candidates, err)
	}
	capabilityCandidates, err := s.ListFillerEnrichmentCapabilityCandidates(ctx,
		pass.Producer, pass.ProducerVersion, pass.TaxonomyVersion, 10)
	if err != nil || len(capabilityCandidates) != 0 {
		t.Fatalf("paid capability was selected again after another input changed = %+v, err %v", capabilityCandidates, err)
	}
	capabilityCandidates, err = s.ListFillerEnrichmentCapabilityCandidates(ctx,
		pass.Producer, "2", pass.TaxonomyVersion, 10)
	if err != nil || len(capabilityCandidates) != 1 || capabilityCandidates[0].Hash != clip.Hash {
		t.Fatalf("new paid capability identity candidates = %+v, err %v", capabilityCandidates, err)
	}
	candidates, err = s.ListFillerEnrichmentCandidates(ctx, pass.Producer, "2", pass.TaxonomyVersion, 10)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("new producer version candidates = %+v, err %v", candidates, err)
	}
	states, err := s.ListFillerEnrichment(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 3 || states[0].Axis != enrichment.AxisBrand || states[1].Axis != enrichment.AxisEra || states[1].Value.Year != 1999 || states[2].Axis != enrichment.AxisProduct {
		t.Fatalf("states = %+v", states)
	}
	projected, err := s.GetClip(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if projected.Era != 1999 || projected.Brand != "Brambly Sauce" || projected.Category != "condiments" ||
		len(projected.AssertedTags) != 1 || projected.AssertedTags[0] != "condiments" {
		t.Fatalf("projected clip = %+v", projected)
	}
	unworked := enrichment.State{ClipHash: clip.Hash, Axis: enrichment.AxisLanguage, Status: enrichment.StatusMissing}
	if _, _, err := s.ApplyFillerEnrichment(ctx, unworked, at); !errors.Is(err, enrichment.ErrInvalidState) {
		t.Fatalf("persist missing state error = %v, want ErrInvalidState", err)
	}
	missing := item
	missing.ClipHash = "missing-clip"
	if _, _, err := s.ApplyFillerEnrichment(ctx, missing, at); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing clip error = %v, want ErrNotFound", err)
	}
}

// Only geography no later pass can change is grounds for removal: a settled national or local
// answer on an active clip, never an inference or unknown (#2064).
func testSettledGeographyClips(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Unix(1_700_000_900, 0).UTC()
	apply := func(id string, geography enrichment.Geography, kind enrichment.EvidenceKind) string {
		clip := sampleClip(id, id, filler.Commercial, 1990, "", "")
		if err := s.UpsertClip(ctx, clip); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.ApplyFillerEnrichment(ctx, enrichment.State{
			ClipHash: clip.Hash, Axis: enrichment.AxisGeography, Status: enrichment.StatusComplete,
			Value: enrichment.Value{Geography: geography},
			Evidence: enrichment.Evidence{Kind: kind, Reference: "test:" + id, Confidence: 90,
				Producer: "test", ProducerVersion: "1", ObservedAt: at},
		}, at); err != nil {
			t.Fatal(err)
		}
		return clip.Hash
	}
	declared := apply("declared-foreign", enrichment.Geography{Scope: "national", Country: "GB"}, enrichment.EvidenceSourceDefault)
	station := apply("station-local", enrichment.Geography{Scope: "local", Country: "US", Station: "fcc:7"}, enrichment.EvidenceTrustedMap)
	apply("inferred-foreign", enrichment.Geography{Scope: "national", Country: "GB"}, enrichment.EvidenceInference)
	if err := s.UpsertClip(ctx, sampleClip("unknown", "unknown", filler.Commercial, 1990, "", "")); err != nil {
		t.Fatal(err)
	}
	removed := apply("removed-local", enrichment.Geography{Scope: "local", Country: "US", Station: "fcc:8"}, enrichment.EvidenceTrustedMap)
	removedClip, err := s.GetClip(ctx, removed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetClipsRemoved(ctx, []string{removedClip.Path}, at); err != nil {
		t.Fatal(err)
	}

	settled, err := s.ListSettledGeographyClips(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]SettledGeographyClip{}
	for _, c := range settled {
		got[c.Hash] = c
	}
	if len(got) != 2 || got[declared].Country != "GB" || got[declared].Scope != "national" ||
		got[station].Station != "fcc:7" || got[station].Scope != "local" || got[station].Path == "" {
		t.Fatalf("settled geography clips = %+v; want the declared foreign and the station-local clip only", settled)
	}
}

// An uncited era guess is a Context observation (#2323): shown only when nothing cited exists,
// never counted as research, never outranking a cited country, and skipped for a clip whose era
// is already verified.
func testFillerEraObservation(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Unix(1_700_000_500, 0).UTC()
	guess := sampleClip("guess-clip", "Twirlpop Classic Commercial", filler.Commercial, 0, "", "")
	guess.Source = "archive:classic_tv_commercials"
	guess.UpdatedAt = at
	known := sampleClip("known-clip", "Known Commercial", filler.Commercial, 1992, "", "")
	known.UpdatedAt = at
	for _, c := range []Clip{guess, known} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.RecordFillerEraObservation(ctx, guess.Hash, 1985, research.ObservedByVision, at.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	observed, err := s.LatestFillerResearchReport(ctx, guess.Hash)
	if err != nil || !observed.IsObservation() || observed.Suggestion.Year != 1985 ||
		observed.Suggestion.Explanation == "" || len(observed.Cited()) != 0 {
		t.Fatalf("observation = %+v, err %v", observed, err)
	}
	if err := s.RecordFillerEraObservation(ctx, known.Hash, 1985, research.ObservedBySplit, at); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LatestFillerResearchReport(ctx, known.Hash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a guess was kept beside a verified era: %+v, err %v", got, err)
	}

	// An observation is not research: the clip is still a research candidate.
	candidates, err := s.ListFillerResearchCandidates(ctx, "context-model:fixture", "prompt:model",
		"mediawiki", research.MediaWikiAdapterVersion, 3)
	if err != nil || len(candidates) != 1 || candidates[0].ClipHash != guess.Hash {
		t.Fatalf("candidates beside an observation = %+v, err %v", candidates, err)
	}
	cited := research.Report{ClipHash: guess.Hash, InputRevision: candidates[0].InputRevision,
		Producer: "context-model:fixture", ProducerVersion: "prompt:model", CompletedAt: at.Add(time.Second),
		Suggestion: research.Suggestion{CountryCode: "US", Country: "United States",
			Confidence: 70, Explanation: "Aired in the US.", CitationIDs: []int{1}},
		Packet: research.Packet{Query: guess.Name, Adapter: "mediawiki",
			AdapterVersion: research.MediaWikiAdapterVersion, RetrievedAt: at,
			Citations: []research.Citation{{ID: 1, Title: "Twirlpop",
				URL: "https://en.wikipedia.org/wiki/Twirlpop", Extract: "It aired on US television."}}}}
	if err := s.SaveFillerResearchReport(ctx, cited); err != nil {
		t.Fatal(err)
	}
	// Cited research outranks the NEWER uncited guess.
	if got, err := s.LatestFillerResearchReport(ctx, guess.Hash); err != nil || got.IsObservation() ||
		got.Suggestion.CountryCode != "US" {
		t.Fatalf("latest beside cited research = %+v, err %v", got, err)
	}
	// Nor may the newer guess hide the cited country from catch-up promotion.
	if err := s.UpdateClipGeography(ctx, guess.Hash, string(pod.GeographicUnknown), "", "", "", "", "", at.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	if promoted, err := s.PromoteStoredFillerResearchCountries(ctx, 3); err != nil || promoted != 1 {
		t.Fatalf("promotion beside a newer observation = %d, %v, want 1", promoted, err)
	}
}

func testFillerContextResearch(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Unix(1_700_000_500, 0).UTC()
	clip := sampleClip("context-clip", "Twirlpop Classic Commercial", filler.Commercial, 0, "", "")
	clip.Source = "archive:classic_tv_commercials"
	clip.UpdatedAt = at
	if err := s.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	candidates, err := s.ListFillerResearchCandidates(ctx, "context-model:fixture", "prompt:model",
		"mediawiki", research.MediaWikiAdapterVersion, 3)
	if err != nil || len(candidates) != 1 || candidates[0].ClipHash != clip.Hash || candidates[0].InputRevision < 1 {
		t.Fatalf("candidates = %+v, err %v", candidates, err)
	}
	report := research.Report{ClipHash: clip.Hash, InputRevision: candidates[0].InputRevision,
		Producer: "context-model:fixture", ProducerVersion: "prompt:model", CompletedAt: at.Add(time.Second),
		Suggestion: research.Suggestion{Decade: 1970, CountryCode: "US", Country: "United States",
			Confidence: 70, Explanation: "Likely campaign context; the exact cut is not proven.", CitationIDs: []int{1}},
		Packet: research.Packet{Query: clip.Name, Adapter: "mediawiki",
			AdapterVersion: research.MediaWikiAdapterVersion, RetrievedAt: at,
			Citations: []research.Citation{{ID: 1, Title: "Twirlpop",
				URL:     "https://en.wikipedia.org/wiki/Twirlpop",
				Extract: "The animated commercial debuted on US television in 1970."}}}}
	if err := s.SaveFillerResearchReport(ctx, report); err != nil {
		t.Fatal(err)
	}
	projected, err := s.GetClip(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if projected.GeographicScope != pod.GeographicNational || projected.Country != "US" ||
		projected.Station != "" || !strings.HasPrefix(projected.GeoEvidence, "context.report:mediawiki:citations:") {
		t.Fatalf("cited country projection = %+v", projected.Clip)
	}
	states, err := s.ListFillerEnrichment(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	var geography enrichment.State
	for _, state := range states {
		if state.Axis == enrichment.AxisGeography {
			geography = state
		}
	}
	if geography.Value.Geography.Country != "US" || geography.Evidence.Kind != enrichment.EvidenceInference ||
		geography.Evidence.Confidence != 70 {
		t.Fatalf("cited geography state = %+v", geography)
	}
	got, err := s.LatestFillerResearchReport(ctx, clip.Hash)
	if err != nil || !reflect.DeepEqual(got, report) {
		t.Fatalf("latest report = %+v, err %v, want %+v", got, err, report)
	}
	candidates, err = s.ListFillerResearchCandidates(ctx, report.Producer, report.ProducerVersion,
		report.Packet.Adapter, report.Packet.AdapterVersion, 3)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("same identity candidates after save = %+v, err %v", candidates, err)
	}
	// Simulate a pre-projection database: the cited report and accepted state exist, but the clip
	// projection is absent. The bounded catch-up must repair it without another retrieval.
	if err := s.UpdateClipGeography(ctx, clip.Hash, string(pod.GeographicUnknown), "", "", "", "", "", at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	promoted, err := s.PromoteStoredFillerResearchCountries(ctx, 3)
	if err != nil || promoted != 1 {
		t.Fatalf("PromoteStoredFillerResearchCountries() = %d, %v", promoted, err)
	}
	projected, err = s.GetClip(ctx, clip.Hash)
	if err != nil || projected.Country != "US" || projected.GeographicScope != pod.GeographicNational {
		t.Fatalf("caught-up country = %+v, err %v", projected.Clip, err)
	}
	// A newer report that abstains supersedes an older confident suggestion for catch-up. Do not
	// resurrect stale context just because it is the newest report that happens to qualify.
	if err := s.UpdateClipGeography(ctx, clip.Hash, string(pod.GeographicUnknown), "", "", "", "", "", at.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	abstained := report
	abstained.ProducerVersion = "prompt:newer"
	abstained.CompletedAt = at.Add(4 * time.Second)
	abstained.Suggestion = research.Suggestion{Confidence: 10, Explanation: "The cited page does not identify a country."}
	if err := s.SaveFillerResearchReport(ctx, abstained); err != nil {
		t.Fatal(err)
	}
	if promoted, err := s.PromoteStoredFillerResearchCountries(ctx, 3); err != nil || promoted != 0 {
		t.Fatalf("promotion after newer abstention = %d, %v, want 0", promoted, err)
	}
	// A station's own material, named and resolved, is local to that station (#2063).
	local := report
	local.ProducerVersion = "prompt:local"
	local.CompletedAt = at.Add(4500 * time.Millisecond)
	local.Suggestion.Scope, local.Suggestion.CallSign, local.Suggestion.Station = "local", "WQXT", "fcc:7"
	if err := s.SaveFillerResearchReport(ctx, local); err != nil {
		t.Fatal(err)
	}
	projected, err = s.GetClip(ctx, clip.Hash)
	if err != nil || projected.GeographicScope != pod.GeographicLocal || projected.Country != "US" ||
		projected.Station != "fcc:7" {
		t.Fatalf("local research projection = %+v, err %v", projected.Clip, err)
	}
	// An explicit operator clear is terminal even when an older report was confident.
	if _, _, err := s.ApplyFillerEnrichment(ctx, enrichment.State{
		ClipHash: clip.Hash, Axis: enrichment.AxisGeography, Status: enrichment.StatusComplete,
		Evidence: enrichment.Evidence{Kind: enrichment.EvidenceOperator,
			Reference: "operator.clip_edit", Confidence: 100, Producer: "operator",
			ProducerVersion: "1", ObservedAt: at.Add(5 * time.Second)},
	}, at.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if promoted, err := s.PromoteStoredFillerResearchCountries(ctx, 3); err != nil || promoted != 0 {
		t.Fatalf("promotion after operator clear = %d, %v, want 0", promoted, err)
	}
	candidates, err = s.ListFillerResearchCandidates(ctx, report.Producer, "prompt:new-model",
		report.Packet.Adapter, report.Packet.AdapterVersion, 3)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("changed model candidates = %+v, err %v", candidates, err)
	}
	if err := s.SetClipTranscript(ctx, clip.Path, "How many licks?", at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFillerResearchReport(ctx, report); !errors.Is(err, ErrFillerResearchStale) {
		t.Fatalf("save stale report error = %v, want ErrFillerResearchStale", err)
	}
	candidates, err = s.ListFillerResearchCandidates(ctx, report.Producer, report.ProducerVersion,
		report.Packet.Adapter, report.Packet.AdapterVersion, 3)
	if err != nil || len(candidates) != 1 || candidates[0].InputRevision == report.InputRevision {
		t.Fatalf("changed-input candidates = %+v, err %v", candidates, err)
	}
	verified := clip
	verified.Hash = "known-context"
	verified.Path = clipPathFor(verified.Hash)
	verified.Era = 1999
	verified.Country = "GB"
	if err := s.UpsertClip(ctx, verified); err != nil {
		t.Fatal(err)
	}
	candidates, err = s.ListFillerResearchCandidates(ctx, report.Producer, report.ProducerVersion,
		report.Packet.Adapter, report.Packet.AdapterVersion, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range candidates {
		if candidate.ClipHash == verified.Hash {
			t.Fatalf("fully verified clip was selected: %+v", candidate)
		}
	}
}

func testFillerResearchWebUsage(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	month := "2026-09"
	at := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	usage, err := s.FillerResearchWebUsage(ctx, month)
	if err != nil || usage.Month != month || usage.RequestCount != 0 {
		t.Fatalf("empty usage = %+v, err %v", usage, err)
	}
	usage, err = s.ReserveFillerResearchWebRequest(ctx, month, research.WebProviderBrave, 2, research.WebAttempt{})
	if err != nil || usage.RequestCount != 1 || usage.LastProvider != research.WebProviderBrave {
		t.Fatalf("first reservation = %+v, err %v", usage, err)
	}
	if err := s.CompleteFillerResearchWebRequest(ctx, month, true, at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveFillerResearchWebRequest(ctx, month, research.WebProviderSearXNG, 2, research.WebAttempt{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveFillerResearchWebRequest(ctx, month, research.WebProviderBrave, 2, research.WebAttempt{}); !errors.Is(err, research.ErrWebSearchLimit) {
		t.Fatalf("third reservation error = %v, want limit", err)
	}
	usage, err = s.FillerResearchWebUsage(ctx, month)
	if err != nil || usage.RequestCount != 2 || usage.LastSuccessAt != at || usage.LastProvider != research.WebProviderSearXNG {
		t.Fatalf("bounded usage = %+v, err %v", usage, err)
	}
	next, err := s.FillerResearchWebUsage(ctx, "2026-10")
	if err != nil || next.RequestCount != 0 || next.Month != "2026-10" {
		t.Fatalf("next month usage = %+v, err %v", next, err)
	}
	clip := sampleClip("web-attempt", "Web attempt", filler.Commercial, 0, "", "")
	if err := s.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	attempt := research.WebAttempt{ClipHash: clip.Hash, InputRevision: 2,
		AdapterVersion: "web-v1:brave", ReservedAt: at}
	if _, err := s.ReserveFillerResearchWebRequest(ctx, "2026-10", research.WebProviderBrave, 2, attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReserveFillerResearchWebRequest(ctx, "2026-10", research.WebProviderBrave, 2, attempt); !errors.Is(err, research.ErrWebSearchAttempted) {
		t.Fatalf("duplicate attempt error = %v, want already attempted", err)
	}
	next, err = s.FillerResearchWebUsage(ctx, "2026-10")
	if err != nil || next.RequestCount != 1 {
		t.Fatalf("deduplicated usage = %+v, err %v", next, err)
	}
}

// testIncomingConveyorCount keeps the bounded Incoming total on the same readiness rule as the
// list it describes. A split detection checkpoint is private pipeline state, so its composite
// remains on the conveyor; only a completed proposal claims that composite into the reels list.
// This runs unchanged against SQLite and Postgres because a backend-specific JSON predicate here
// would be an easy source of a count that disagrees on one install type.
func testIncomingConveyorCount(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()

	for _, c := range []Clip{
		{Clip: filler.Clip{Hash: "draft-reel", Path: "reels/draft.mp4", Name: "Draft reel",
			Role: filler.Commercial, DurationMs: 1_180_000, IsComposite: true, Source: "archive:test"}},
		{Clip: filler.Clip{Hash: "ready-reel", Path: "reels/ready.mp4", Name: "Ready reel",
			Role: filler.Commercial, DurationMs: 1_180_000, IsComposite: true, Source: "archive:test"}},
	} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertClipPipeline(ctx, pipelinestate.ClipPipeline{
			ClipHash: c.Hash, Stage: pipelinestate.StageSplit, Status: pipelinestate.StatusQueued,
			Disposition: pipelinestate.DispositionRunning, UpdatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UpsertSplitProposal(ctx, filler.SplitProposal{
		ID: "sp-draft", ClipHash: "draft-reel", CreatedAt: time.Now().UTC(),
		Detection: &split.DetectionProgress{ScannedThroughMs: 600_000},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertSplitProposal(ctx, filler.SplitProposal{
		ID: "sp-ready", ClipHash: "ready-reel", CreatedAt: time.Now().UTC(),
		Segments: []split.Segment{{Index: 0, StartMs: 0, EndMs: 30_000}},
	}); err != nil {
		t.Fatal(err)
	}

	bySource, err := s.CountIncomingConveyorBySource(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(bySource) != 1 || bySource["archive:test"] != 1 {
		t.Errorf("Incoming conveyor count = %v, want 1 draft reel under archive:test; the ready reel has its own row", bySource)
	}
}

func testFillerInferenceAccountingAndBudgets(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 8, 24, 15, 0, 0, 0, time.UTC)
	versions := InferenceVersions{
		Evidence: "e1", Extractor: "x1", Prompt: "p1", Schema: "s1", Taxonomy: "t1",
		AdmissionPolicy: "a1", RolePolicy: "r1", CapabilitySnapshot: "c1",
	}
	reserve := func(id, clip string, nano int64) InferenceEvaluation {
		e, err := s.ReserveInferenceEvaluation(ctx, InferenceEvaluation{
			ID: id, ClipHash: clip, RunID: "cert-1", Role: "filler_frames", Rung: "frames",
			RequestedProvider: "openrouter", RequestedModel: "openai/gpt-5-mini",
			Modalities: []string{"text", "image"}, DerivativeBytes: 4096, DerivativePixels: 2_073_600,
			ReservedNanoUSD: nano, Versions: versions, CreatedAt: at,
		}, InferenceBudget{PerClipNanoUSD: 100, PerDayNanoUSD: 1000, PerRunNanoUSD: 1000})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}

	first := reserve("eval-1", "clip-a", 60)
	if first.State != InferenceReserved {
		t.Fatalf("first reservation = %+v", first)
	}
	denied := reserve("eval-2", "clip-a", 50)
	if denied.State != InferenceHeldBudget || denied.ReservedNanoUSD != 0 || denied.FailureReason == "" {
		t.Fatalf("over-budget reservation = %+v", denied)
	}

	settled, err := s.SettleInferenceEvaluation(ctx, first.ID, InferenceSettlement{
		State: InferenceCompleted, ResolvedProvider: "OpenAI", ResolvedModel: "openai/gpt-5-mini-2026-08-07",
		Tokens:        InferenceTokens{Prompt: 194, Completion: 12, Reasoning: 5, Cached: 31, Image: 80},
		ChargedAmount: "0.000000040", ChargedCurrency: "USD", ChargedNanoUSD: 40,
		EstimatedNanoUSD: 42, PriceSnapshot: "prices-1", LatencyMS: 812, Attempts: 1,
		GenerationID: "gen-1", Outcome: InferenceWindowComplete, UpdatedAt: at.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if settled.State != InferenceCompleted || settled.ReservedNanoUSD != 40 || settled.ChargedAmount != "0.000000040" || settled.Tokens.Image != 80 {
		t.Fatalf("settled evaluation = %+v", settled)
	}

	third := reserve("eval-3", "clip-a", 50)
	if third.State != InferenceReserved {
		t.Fatalf("unused reservation was not released: %+v", third)
	}
	over, err := s.SettleInferenceEvaluation(ctx, third.ID, InferenceSettlement{
		State: InferenceCompleted, ChargedAmount: "0.000000055", ChargedCurrency: "USD",
		ChargedNanoUSD: 55, UpdatedAt: at.Add(2 * time.Minute),
	})
	if !errors.Is(err, ErrInferenceBudgetExceeded) || over.State != InferenceHeldBudget || over.ChargedNanoUSD != 55 {
		t.Fatalf("provider overspend = %+v, %v", over, err)
	}
	if _, err := s.SettleInferenceEvaluation(ctx, third.ID, InferenceSettlement{UpdatedAt: at.Add(3 * time.Minute)}); !errors.Is(err, ErrInferenceNotReserved) {
		t.Fatalf("second settlement = %v", err)
	}

	dayDenied := reserve("eval-4", "clip-b", 906)
	if dayDenied.State != InferenceHeldBudget {
		t.Fatalf("daily/run ceiling did not hold request: %+v", dayDenied)
	}
	rows, err := s.ListInferenceEvaluations(ctx, InferenceEvaluationFilter{RunID: "cert-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[0].ID != "eval-4" {
		t.Fatalf("evaluation history = %+v", rows)
	}
	got, err := s.GetInferenceEvaluation(ctx, "eval-1")
	if err != nil || got.GenerationID != "gen-1" || got.Versions.RolePolicy != "r1" || len(got.Modalities) != 2 || got.CacheKey == "" {
		t.Fatalf("inspect evaluation = %+v, %v", got, err)
	}
}

// The ordinary scan upsert cannot release a hold, even when its caller supplies held=false. An
// unknown role is not rejected at this boundary; terminal readiness owns its Placement. A released
// composite of an unknown role stays out of playback.
func testFillerHeldClipUpsertAndCompositeRelease(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 13, 2, 0, 0, 0, time.UTC)
	hash := strings.Repeat("8", 64)
	if err := s.UpsertClip(ctx, Clip{Clip: filler.Clip{
		Hash: hash, Path: "88/88/" + hash + ".mp4", Name: "Held candidate", Role: filler.Unclassified,
		DurationMs: 30_000, Held: true,
	}, UpdatedAt: at}); err != nil {
		t.Fatal(err)
	}
	clip, err := s.GetClip(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	unheld := clip
	unheld.Held = false
	if err := s.UpsertClip(ctx, unheld); err != nil {
		t.Fatalf("ordinary upsert rejected descriptive unclassified state: %v", err)
	}
	clip, err = s.GetClip(ctx, hash)
	if err != nil || !clip.Held {
		t.Fatalf("ordinary upsert released hold = %+v, err = %v", clip, err)
	}
	if err := s.SetClipComposite(ctx, hash, true, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReleaseCompositeHolds(ctx, []string{clip.Path}, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	playable, err := s.ListClips(ctx, ClipFilter{Hashes: []string{hash}, IncludeComposites: true})
	if err != nil || len(playable) != 0 {
		t.Fatalf("released composite became playable despite not_playable placement: %+v, err=%v", playable, err)
	}
}

func testFillerTerminalReadyTransaction(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, time.September, 13, 18, 0, 0, 0, time.UTC)
	hash := strings.Repeat("7", 64)
	if err := s.UpsertClip(ctx, Clip{Clip: filler.Clip{
		Hash: hash, Path: "77/77/" + hash + ".mp4", Name: "Unknown enrolled clip",
		Role: filler.Unclassified, Source: "archive:classic_tv_commercials", Held: true,
		DurationMs: 30_000,
	}, UpdatedAt: at}); err != nil {
		t.Fatal(err)
	}
	current := pipelinestate.ClipPipeline{
		ClipHash: hash, AcquisitionID: "acquisition-1", Stage: pipelinestate.StageFinish,
		Status: pipelinestate.StatusRunning, Progress: 100, Disposition: pipelinestate.DispositionRunning,
		EnrolledAt: at, UpdatedAt: at,
	}
	for _, stage := range pipelinestate.StageOrder[:len(pipelinestate.StageOrder)-1] {
		current.Stages = append(current.Stages, pipelinestate.StageRecord{Stage: stage, Status: pipelinestate.StatusDone, At: at})
	}
	if err := s.UpsertClipPipeline(ctx, current); err != nil {
		t.Fatal(err)
	}
	settled := current
	settled.Status = pipelinestate.StatusDone
	settled.Disposition = pipelinestate.DispositionReady
	settled.NextRun = time.Time{}
	settled.Stages = append(settled.Stages, pipelinestate.StageRecord{Stage: pipelinestate.StageFinish, Status: pipelinestate.StatusDone, At: at})
	event := filler.ReadyEvent{
		ID: "ready:" + hash, ClipHash: hash, AcquisitionID: current.AcquisitionID,
		Enrollment: filler.Enrollment{Kind: filler.EnrollmentAcquisition, Reference: current.AcquisitionID},
		Placement:  filler.PlacementBreakBody, CreatedAt: at,
	}
	commit := filler.ReadyCommit{Event: event, Pipeline: settled}
	if err := s.CommitFillerReady(ctx, commit); err != nil {
		t.Fatal(err)
	}
	clip, err := s.GetClip(ctx, hash)
	if err != nil || clip.Held || clip.Placement != filler.PlacementBreakBody || clip.Role != filler.Unclassified {
		t.Fatalf("ready clip = %+v, err = %v", clip, err)
	}
	row, found, err := s.GetClipPipeline(ctx, hash)
	if err != nil || !found || row.Disposition != pipelinestate.DispositionReady || row.Status != pipelinestate.StatusDone {
		t.Fatalf("ready pipeline = %+v, found=%t err=%v", row, found, err)
	}
	gotEvent, found, err := s.GetFillerReadyEvent(ctx, hash)
	if err != nil || !found || gotEvent.ID != event.ID || gotEvent.Placement != filler.PlacementBreakBody ||
		gotEvent.Enrollment != event.Enrollment {
		t.Fatalf("ready event = %+v, found=%t err=%v", gotEvent, found, err)
	}
	playable, err := s.ListClips(ctx, ClipFilter{Hashes: []string{hash}})
	if err != nil || len(playable) != 1 {
		t.Fatalf("ready unknown-role clip is not playable: %+v, err=%v", playable, err)
	}
	if err := s.CommitFillerReady(ctx, commit); err != nil {
		t.Fatalf("exact ready retry was not idempotent: %v", err)
	}

	// A deliberate restart retains the immutable Ready event but puts the clip back on hold and
	// returns its conveyor to running. Reaching the terminal rung again must reapply publication;
	// treating the old event as proof that the current row is already settled leaves the clip at
	// score/running forever while every pass reports success.
	restartedAt := at.Add(time.Minute)
	if n, err := s.HoldClips(ctx, []string{"77/77/" + hash + ".mp4"}, restartedAt); err != nil || n != 1 {
		t.Fatalf("hold ready clip for restart = %d, %v", n, err)
	}
	restarted := current
	restarted.Attempts = 2
	restarted.ForceRun = true
	restarted.PreparationAttempt = 2
	restarted.PreparationStartedAt = restartedAt
	restarted.PreparationStartReason = pipelinestate.PreparationStartedByRestart
	restarted.StageQueuedAt = restartedAt
	restarted.StageStartedAt = restartedAt
	restarted.UpdatedAt = restartedAt
	if err := s.UpsertClipPipeline(ctx, restarted); err != nil {
		t.Fatal(err)
	}
	republished := commit
	republished.Pipeline.Attempts = restarted.Attempts
	republished.Pipeline.PreparationAttempt = restarted.PreparationAttempt
	republished.Pipeline.PreparationStartedAt = restarted.PreparationStartedAt
	republished.Pipeline.PreparationStartReason = restarted.PreparationStartReason
	republished.Pipeline.StageQueuedAt = restarted.StageQueuedAt
	republished.Pipeline.StageStartedAt = restarted.StageStartedAt
	republished.Pipeline.UpdatedAt = restartedAt.Add(time.Second)
	if err := s.CommitFillerReady(ctx, republished); err != nil {
		t.Fatalf("publish after a completed restart: %v", err)
	}
	republishedClip, err := s.GetClip(ctx, hash)
	if err != nil || republishedClip.Held || republishedClip.Placement != filler.PlacementBreakBody {
		t.Fatalf("restarted clip was not republished = %+v, err=%v", republishedClip, err)
	}
	republishedRow, found, err := s.GetClipPipeline(ctx, hash)
	if err != nil || !found || republishedRow.Disposition != pipelinestate.DispositionReady ||
		republishedRow.Status != pipelinestate.StatusDone || republishedRow.PreparationAttempt != 2 {
		t.Fatalf("restarted conveyor was not settled = %+v, found=%t err=%v", republishedRow, found, err)
	}
	unchangedEvent, found, err := s.GetFillerReadyEvent(ctx, hash)
	if err != nil || !found || unchangedEvent.CreatedAt != event.CreatedAt {
		t.Fatalf("restart rewrote immutable Ready event = %+v, found=%t err=%v", unchangedEvent, found, err)
	}

	staleHash := strings.Repeat("6", 64)
	if err := s.UpsertClip(ctx, Clip{Clip: filler.Clip{
		Hash: staleHash, Path: "66/66/" + staleHash + ".mp4", Name: "No pipeline",
		Role: filler.Unclassified, Source: "test-folder", Held: true, DurationMs: 30_000,
	}, UpdatedAt: at}); err != nil {
		t.Fatal(err)
	}
	stale := commit
	stale.Event.ID = "ready:" + staleHash
	stale.Event.ClipHash = staleHash
	stale.Event.AcquisitionID = ""
	stale.Event.Enrollment = filler.Enrollment{Kind: filler.EnrollmentSource, Reference: "test-folder"}
	stale.Pipeline.ClipHash = staleHash
	stale.Pipeline.AcquisitionID = ""
	if err := s.CommitFillerReady(ctx, stale); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing conveyor error = %v, want not found", err)
	}
	staleClip, err := s.GetClip(ctx, staleHash)
	if err != nil || !staleClip.Held || staleClip.Placement != filler.PlacementNotPlayable {
		t.Fatalf("failed ready transaction changed clip = %+v, err=%v", staleClip, err)
	}
	if _, found, err := s.GetFillerReadyEvent(ctx, staleHash); err != nil || found {
		t.Fatalf("failed ready transaction recorded event, found=%t err=%v", found, err)
	}

	// A caller cannot publish a fabricated completed ladder over a row whose header still looks
	// current. The transaction compares every persisted prior rung before changing any table.
	driftHash := strings.Repeat("5", 64)
	if err := s.UpsertClip(ctx, Clip{Clip: filler.Clip{
		Hash: driftHash, Path: "55/55/" + driftHash + ".mp4", Name: "Drifted ladder",
		Role: filler.Unclassified, Source: "archive:classic_tv_commercials", Held: true,
		DurationMs: 30_000,
	}, UpdatedAt: at}); err != nil {
		t.Fatal(err)
	}
	driftedCurrent := current
	driftedCurrent.ClipHash = driftHash
	driftedCurrent.Stages = append([]pipelinestate.StageRecord(nil), current.Stages...)
	driftedCurrent.Stages[0].Note = "the persisted probe evidence changed"
	if err := s.UpsertClipPipeline(ctx, driftedCurrent); err != nil {
		t.Fatal(err)
	}
	driftedCommit := commit
	driftedCommit.Event.ID = "ready:" + driftHash
	driftedCommit.Event.ClipHash = driftHash
	driftedCommit.Pipeline.ClipHash = driftHash
	if err := s.CommitFillerReady(ctx, driftedCommit); !errors.Is(err, filler.ErrReadyStale) {
		t.Fatalf("drifted ladder error = %v, want ErrReadyStale", err)
	}
	driftedClip, err := s.GetClip(ctx, driftHash)
	if err != nil || !driftedClip.Held || driftedClip.Placement != filler.PlacementNotPlayable {
		t.Fatalf("drifted transaction changed clip = %+v, err=%v", driftedClip, err)
	}
	if _, found, err := s.GetFillerReadyEvent(ctx, driftHash); err != nil || found {
		t.Fatalf("drifted transaction recorded event, found=%t err=%v", found, err)
	}
}

func testFillerSplitShadowDecisions(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	decision := filler.StructureSplitShadowDecision{
		SchemaVersion: filler.StructureSplitShadowSchemaVersion, ContractVersion: filler.StructureSplitShadowContractVersion,
		ProposalID: "proposal-shadow-1", ClipHash: "clip-shadow-1",
		SourceSHA256: strings.Repeat("a", 64), AssessmentSHA256: strings.Repeat("b", 64),
		PolicyVersion: "production-shadow-no-certified-slices-v1",
		Legacy: filler.StructureSplitShadowOutcome{
			Confirm: []filler.StructureSplitShadowSpan{{StartMs: 0, EndMs: 30_000}, {StartMs: 30_000, EndMs: 60_000}},
		},
		Certified: filler.StructureSplitShadowOutcome{
			Verdict: filler.RejectStructureUncertified,
			Hold: []filler.StructureSplitShadowSpan{
				{StartMs: 0, EndMs: 30_000, HoldReason: string(filler.RejectStructureUncertified)},
				{StartMs: 30_000, EndMs: 60_000, HoldReason: string(filler.RejectStructureUncertified)},
			},
		},
		ObservedAt: time.Unix(1_900_000_000, 123).UTC(),
	}
	decision.SHA256 = filler.StructureSplitShadowDecisionSHA256(decision)
	decision.ID = "split-shadow-" + decision.SHA256
	if err := filler.ValidateStructureSplitShadowDecision(decision); err != nil {
		t.Fatal(err)
	}
	if err := s.PutStructureSplitShadowDecision(ctx, decision); err != nil {
		t.Fatal(err)
	}
	if err := s.PutStructureSplitShadowDecision(ctx, decision); err != nil {
		t.Fatalf("idempotent retry failed: %v", err)
	}
	got, found, err := s.GetStructureSplitShadowDecision(ctx, decision.ID)
	if err != nil || !found || !reflect.DeepEqual(got, decision) {
		t.Fatalf("get split shadow = %+v, %v, %v; want %+v", got, found, err, decision)
	}
	if _, found, err := s.GetStructureSplitShadowDecision(ctx, "missing-split-shadow"); err != nil || found {
		t.Fatalf("missing split shadow found = %v, error = %v", found, err)
	}
	rows, err := s.ListStructureSplitShadowDecisions(ctx, decision.ClipHash, 10)
	if err != nil || len(rows) != 1 || !reflect.DeepEqual(rows[0], decision) {
		t.Fatalf("split shadow round trip = %+v, %v; want %+v", rows, err, decision)
	}
	other, err := s.ListStructureSplitShadowDecisions(ctx, "another-clip", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("split shadow clip filter = %+v, %v", other, err)
	}
	if _, err := s.ListStructureSplitShadowDecisions(ctx, "", 0); err == nil {
		t.Fatal("invalid split shadow page limit was accepted")
	}
}

func restartConformanceStore(t *testing.T, s Store) Store {
	t.Helper()
	if err := s.Close(); err != nil {
		t.Fatalf("close conformance store for restart: %v", err)
	}
	reopened, err := reopenConformanceStore(t, s)
	if err != nil {
		t.Fatalf("reopen conformance store: %v", err)
	}
	return reopened
}

// reopenConformanceStore opens a new pool on s's database: SQLite through the full boot path
// (migrations are already applied, so it only re-runs startup healing), Postgres as a bare pool.
func reopenConformanceStore(t *testing.T, s Store) (Store, error) {
	t.Helper()
	url := reopenURL(t, s)
	return openExtended(t.Context(), url, store.DialectOf(s) != DialectPostgres)
}

// testCreativeIdentityMerge pins #2033's merge on both backends: folding a losing copy into its
// survivor carries every airing, play, variant link, earlier inferior copy, curated name and
// channel override onto the survivor, deletes the loser's rows, and remembers both of its
// identities so neither is catalogued again.
func testCreativeIdentityMerge(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.UnixMilli(1_800_000_000_000).UTC()
	mustSaveChannel(t, s, sampleChannel("channel-a", 1, time.Time{}))
	mustSaveChannel(t, s, sampleChannel("channel-b", 2, time.Time{}))
	loser := sampleClip("loser", "Burger Barn (curated)", filler.Commercial, 1988, filler.General, "food")
	loser.NameSource = clips.NameFromCurated
	survivor := sampleClip("survivor", "Untitled commercial", filler.Commercial, 1988, filler.General, "food")
	survivor.NameSource = clips.NameFromGrounded
	cutDown := sampleClip("cut-down", "Burger Barn 15", filler.Commercial, 1988, filler.General, "food")
	earlier := sampleClip("earlier-copy", "Burger Barn", filler.Commercial, 1988, filler.General, "food")
	for _, c := range []Clip{loser, survivor, cutDown, earlier} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	// One airing on each copy, one channel each; the loser's is the later one.
	if _, err := s.RecordClipPlay(ctx, "channel-a", survivor.Hash, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordClipPlay(ctx, "channel-b", loser.Hash, now.Add(time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkClipVariants(ctx, loser.Hash, cutDown.Hash); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeInferiorCopy(ctx, InferiorCopy{
		ClipHash: earlier.Hash, SourceHash: "earlier-source", SurvivorHash: loser.Hash, DecidedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	channel := sampleChannel("pinned", 3, now)
	channel.Policy.Filler = &schedule.FillerSelection{Pinned: []string{loser.Hash}}
	if _, err := s.SaveChannel(ctx, channel); err != nil {
		t.Fatal(err)
	}

	if err := s.MergeInferiorCopy(ctx, InferiorCopy{ClipHash: survivor.Hash, SourceHash: "x", SurvivorHash: survivor.Hash}); err == nil {
		t.Fatal("a clip merged into itself")
	}
	if err := s.MergeInferiorCopy(ctx, InferiorCopy{ClipHash: "gone", SourceHash: "x", SurvivorHash: survivor.Hash}); err == nil {
		t.Fatal("a missing clip merged")
	}
	// The loser's defect record describes bytes that are about to be deleted (#2034).
	readings := defects.Readings{Version: defects.MeasurementVersion, Width: 640, Height: 480, Frames: 40}
	if err := s.UpsertClipDefects(ctx, clipstore.ClipDefectRecord{
		ClipHash: loser.Hash, Readings: readings, Era: 1988, Assessment: defects.Assess(readings, 1988),
		Verdict: defects.VerdictPass, MeasuredAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeInferiorCopy(ctx, InferiorCopy{
		ClipHash: loser.Hash, SourceHash: "loser-source", SurvivorHash: survivor.Hash, DecidedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.GetClip(ctx, loser.Hash); err == nil {
		t.Error("the losing copy's catalog row survived the merge")
	}
	if records, err := s.ListClipDefects(ctx); err != nil {
		t.Fatal(err)
	} else if _, ok := records[loser.Hash]; ok {
		t.Error("the losing copy's defect record survived the merge")
	}
	got, err := s.GetClip(ctx, survivor.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlayCount != 2 || !got.LastPlayedAt.Equal(now.Add(time.Hour)) {
		t.Errorf("survivor plays = %d, last %v; want both copies' 2, latest %v", got.PlayCount, got.LastPlayedAt, now.Add(time.Hour))
	}
	if got.Name != loser.Name || got.NameSource != clips.NameFromCurated {
		t.Errorf("survivor name = %q (%s), want the curated %q", got.Name, got.NameSource, loser.Name)
	}
	exposure, err := exposuresBefore(ctx, s, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if exposure[survivor.Hash].PlayCount != 2 || exposure[loser.Hash].PlayCount != 0 {
		t.Errorf("household rotation = %+v, want both airings on the survivor", exposure)
	}
	variants, err := s.ListClipVariants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(variants[cutDown.Hash], []string{survivor.Hash}) || len(variants[loser.Hash]) != 0 {
		t.Errorf("variant links = %v, want the cut-down linked to the survivor only", variants)
	}
	inferior, err := s.InferiorCopyHashes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{loser.Hash, "loser-source", earlier.Hash, "earlier-source"} {
		if !inferior[h] {
			t.Errorf("inferior identity %q was not remembered: %v", h, inferior)
		}
	}
	if inferior[survivor.Hash] {
		t.Error("the survivor was remembered as inferior")
	}
	gotChannel, err := s.GetChannel(ctx, channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotChannel.Policy.Filler == nil || !reflect.DeepEqual(gotChannel.Policy.Filler.Pinned, []string{survivor.Hash}) {
		t.Errorf("channel pin did not follow the merge: %+v", gotChannel.Policy.Filler)
	}

	// A transform that files new bytes carries it to the new hash: the cut-down stays a variant,
	// airings still rest the ad, and copies that lost to it still name a live survivor.
	conditioned := got
	conditioned.Hash = "survivor-conditioned"
	if err := s.ReplaceClipIdentity(ctx, survivor.Hash, conditioned); err != nil {
		t.Fatal(err)
	}
	variants, err = s.ListClipVariants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(variants[cutDown.Hash], []string{conditioned.Hash}) || len(variants[survivor.Hash]) != 0 {
		t.Errorf("variant links after re-key = %v, want the cut-down linked to %q", variants, conditioned.Hash)
	}
	exposure, err = exposuresBefore(ctx, s, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if exposure[conditioned.Hash].PlayCount != 2 || exposure[survivor.Hash].PlayCount != 0 {
		t.Errorf("household rotation after re-key = %+v, want both airings on %q", exposure, conditioned.Hash)
	}
}
