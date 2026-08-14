package filler_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// Clip artwork (V28 still + V39 animation), rendered by ONE ffmpeg pass.
//
// These were two test files until V39 merged the two generators into a single exec. Every
// assertion from `thumbnail_test.go` survives here — the still's contract did not change when it
// gained a travelling companion, and losing those tests to a refactor would have been the real
// cost of merging.

// writeAt creates a file with content, making parents as needed. (scan_test.go has a writeFile
// with a different signature; this one takes a full path.)
func writeAt(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeArtwork writes stand-in files, recording the start offset it was asked for. No binary runs
// — the same injection seam Prober uses (unit tests never exec, per AGENTS.md).
func fakeArtwork(starts *[]float64, fail bool) filler.ArtworkRenderer {
	return func(_ context.Context, _, stillDst, animDst string, startSeconds float64) error {
		*starts = append(*starts, startSeconds)
		if fail {
			return errors.New("ffmpeg: unknown encoder libwebp")
		}
		if err := os.MkdirAll(filepath.Dir(stillDst), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(stillDst, []byte("\xff\xd8\xff jpeg"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(animDst, []byte("RIFF....WEBP"), 0o600)
	}
}

// The paths mirror the clip's own structure — flattening would collide two clips named intro.mp4
// in different era folders, silently, with one overwriting the other.
func TestArtworkPathsPreserveStructure(t *testing.T) {
	for _, tc := range []struct{ in, still, anim string }{
		{"1994/toys-transformers.mp4", "1994/toys-transformers.jpg", "1994/toys-transformers.webp"},
		{"1988/intro.mkv", "1988/intro.jpg", "1988/intro.webp"},
		// The V38c sharded-hash layout, which is what real paths look like now.
		{"14/36/14365f2b.mp4", "14/36/14365f2b.jpg", "14/36/14365f2b.webp"},
		{"", "", ""},
	} {
		if got := filler.ThumbPathFor(tc.in); got != tc.still {
			t.Errorf("ThumbPathFor(%q) = %q, want %q", tc.in, got, tc.still)
		}
		if got := filler.PreviewPathFor(tc.in); got != tc.anim {
			t.Errorf("PreviewPathFor(%q) = %q, want %q", tc.in, got, tc.anim)
		}
	}

	if filler.ThumbPathFor("1994/intro.mp4") == filler.ThumbPathFor("1988/intro.mp4") {
		t.Error("two clips named intro.mp4 in different folders must not share an artwork path")
	}
}

// Both files are written, both paths recorded, from ONE call to the renderer.
func TestGenerateArtwork_FillsBothPathsInOnePass(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "1994", "toys.mp4"), "video")
	clips := []filler.RawClip{{Path: "1994/toys.mp4", Name: "toys", DurationMs: 30_000}}

	var starts []float64
	if failed := filler.GenerateArtwork(context.Background(), dir, clips, fakeArtwork(&starts, false)); failed != 0 {
		t.Fatalf("failed = %d, want 0", failed)
	}
	if clips[0].Thumbnail != "1994/toys.jpg" {
		t.Errorf("thumbnail = %q, want the relative path", clips[0].Thumbnail)
	}
	if clips[0].Preview != "1994/toys.webp" {
		t.Errorf("preview = %q, want the relative path", clips[0].Preview)
	}
	// Relative in the row, absolute on disk.
	for _, name := range []string{"toys.jpg", "toys.webp"} {
		if _, err := os.Stat(filepath.Join(dir, filler.ThumbDirName, "1994", name)); err != nil {
			t.Errorf("no %s written: %v", name, err)
		}
	}
	// ⚠ ONE invocation for both outputs. Two would mean two decodes of the same file, which is
	// exactly what merging the passes removed.
	if len(starts) != 1 {
		t.Errorf("renderer called %d times, want 1 — both assets come from a single decode", len(starts))
	}
}

// ⚠ **THE rule that decides where the window sits.** A flat offset does not survive real clips.
// Every case here is one that produced bad artwork when a constant was used unchanged.
func TestArtworkStart_PicksAWindowThatFitsTheClip(t *testing.T) {
	// Asserted through GenerateArtwork rather than by exporting the rule, because what matters is
	// the value ffmpeg is actually handed.
	start := func(t *testing.T, durationMs int64) float64 {
		t.Helper()
		dir := t.TempDir()
		writeAt(t, filepath.Join(dir, "c.mp4"), "video")
		clips := []filler.RawClip{{Path: "c.mp4", DurationMs: durationMs}}
		var starts []float64
		if failed := filler.GenerateArtwork(context.Background(), dir, clips, fakeArtwork(&starts, false)); failed != 0 {
			t.Fatalf("%d failed, want 0", failed)
		}
		if len(starts) != 1 {
			t.Fatalf("rendered %d times, want 1", len(starts))
		}
		return starts[0]
	}

	// A 30s spot: 15% in clears the fade and leaves the whole 6s window inside the clip.
	if got := start(t, 30_000); got != 4.5 {
		t.Errorf("30s clip starts at %.2fs, want 4.5s (15%%)", got)
	}

	// ⚠ A 15s spot at a flat 3s would give the LAST 40% of the advert including the end card —
	// the least representative six seconds in it.
	if got := start(t, 15_000); got != 2.25 {
		t.Errorf("15s clip starts at %.2fs, want 2.25s — a flat 3s offset would preview the end card", got)
	}

	// ⚠ A clip SHORTER than the window has no window to place. Starting anywhere but zero makes
	// ffmpeg write a truncated file, or nothing at all.
	if got := start(t, 5_000); got != 0 {
		t.Errorf("5s clip starts at %.2fs, want 0 — it is shorter than the preview window", got)
	}
	// Exactly the window length is the boundary of the same case.
	if got := start(t, 6_000); got != 0 {
		t.Errorf("6s clip starts at %.2fs, want 0", got)
	}

	// ⚠ The floor. On a 7s clip 15% is 1.05s — the rule must not drift toward zero and start on
	// the black frame it exists to skip.
	if got := start(t, 7_000); got != 1 {
		t.Errorf("7s clip starts at %.2fs, want the 1s floor", got)
	}

	// ⚠ Unprobed (duration 0) is a REAL state — a clip ffprobe could not read. Guessing a
	// proportion of an unknown length is meaningless, so it falls back to the known-good 3s
	// rather than to 0, which would start on the fade-in.
	if got := start(t, 0); got != 3 {
		t.Errorf("unprobed clip starts at %.2fs, want the 3s fallback", got)
	}
}

// ⚠ The window never runs off the end, even when the 1s floor and the end-clamp CONTRADICT each
// other. Applying the floor as an early return let it win that argument silently, and ffmpeg was
// asked for a 6s window with 5.5s of clip left.
func TestArtworkStart_NeverRunsPastTheEnd(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "c.mp4"), "video")
	// 6.5s: 15% is 0.975 → floored to 1, but only 0.5s of clip remains after the window.
	clips := []filler.RawClip{{Path: "c.mp4", DurationMs: 6_500}}
	var starts []float64
	filler.GenerateArtwork(context.Background(), dir, clips, fakeArtwork(&starts, false))

	if starts[0] > 0.5 {
		t.Errorf("start %.2fs leaves less than the 6s window; ffmpeg would write a short file", starts[0])
	}
}

// Existing artwork is adopted, not re-rendered. A scan runs on a timer, and re-encoding each pass
// would spend an ffmpeg exec per clip per cycle forever to reproduce correct files.
func TestGenerateArtwork_AdoptsWhatIsAlreadyThere(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "1994", "toys.mp4"), "video")
	writeAt(t, filepath.Join(dir, filler.ThumbDirName, "1994", "toys.jpg"), "already here")
	writeAt(t, filepath.Join(dir, filler.ThumbDirName, "1994", "toys.webp"), "RIFF....WEBP")
	clips := []filler.RawClip{{Path: "1994/toys.mp4", DurationMs: 30_000}}

	var starts []float64
	filler.GenerateArtwork(context.Background(), dir, clips, fakeArtwork(&starts, false))
	if len(starts) != 0 {
		t.Errorf("renderer ran %d times for existing artwork, want 0 — a timed scan would do this forever", len(starts))
	}
	if clips[0].Thumbnail != "1994/toys.jpg" || clips[0].Preview != "1994/toys.webp" {
		t.Errorf("existing artwork must still be adopted, got %q / %q", clips[0].Thumbnail, clips[0].Preview)
	}
}

// ⚠ **A clip with a still but NO animation must be re-rendered.** That is every clip catalogued
// before V39, i.e. the entire existing catalog on upgrade. Treating "the still exists" as "done"
// would mean no install ever grows previews for clips it already had — the feature would only
// work for clips added afterwards.
func TestGenerateArtwork_RendersWhenOnlyTheStillExists(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "a.mp4"), "video")
	writeAt(t, filepath.Join(dir, filler.ThumbDirName, "a.jpg"), "pre-V39 still")
	clips := []filler.RawClip{{Path: "a.mp4", DurationMs: 30_000}}

	var starts []float64
	filler.GenerateArtwork(context.Background(), dir, clips, fakeArtwork(&starts, false))

	if len(starts) != 1 {
		t.Fatalf("renderer ran %d times, want 1 — an upgraded catalog must gain previews", len(starts))
	}
	if clips[0].Preview != "a.webp" {
		t.Errorf("preview = %q, want it rendered for a clip that predates V39", clips[0].Preview)
	}
}

// ⚠ A failure leaves the paths EMPTY and is COUNTED. Recording a path to a file that does not
// exist renders as a broken image; empty renders as no image, which is the honest state. And a
// silent skip is what once produced an empty catalog with no error anywhere.
func TestGenerateArtwork_CountsFailuresAndLeavesPathsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "a.mp4"), "video")
	writeAt(t, filepath.Join(dir, "b.mp4"), "video")
	clips := []filler.RawClip{{Path: "a.mp4"}, {Path: "b.mp4"}}

	var starts []float64
	failed := filler.GenerateArtwork(context.Background(), dir, clips, fakeArtwork(&starts, true))
	if failed != 2 {
		t.Errorf("failed = %d, want 2 — a silent skip is what once produced an empty catalog", failed)
	}
	for _, c := range clips {
		if c.Thumbnail != "" || c.Preview != "" {
			t.Errorf("a failed render must leave both paths empty, got %q / %q", c.Thumbnail, c.Preview)
		}
	}
}

// ⚠ **A HALF-success must keep the half that worked.** One command with two outputs can write the
// still, fail on the webp, and exit non-zero. Trusting the exit code alone would discard a
// perfectly good thumbnail — and on a catalog-wide libwebp failure that is a grid of blank cards
// where the stills used to be.
func TestGenerateArtwork_KeepsTheStillWhenOnlyTheAnimationFails(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "a.mp4"), "video")
	clips := []filler.RawClip{{Path: "a.mp4", DurationMs: 30_000}}

	// The shape of a missing libwebp: the JPEG lands, the webp does not, ffmpeg exits non-zero.
	halfBroken := func(_ context.Context, _, stillDst, _ string, _ float64) error {
		if err := os.MkdirAll(filepath.Dir(stillDst), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(stillDst, []byte("\xff\xd8\xff jpeg"), 0o600); err != nil {
			return err
		}
		return errors.New("ffmpeg: unknown encoder libwebp")
	}
	failed := filler.GenerateArtwork(context.Background(), dir, clips, halfBroken)

	if failed != 1 {
		t.Errorf("failed = %d, want 1 — the operator still needs telling", failed)
	}
	if clips[0].Thumbnail != "a.jpg" {
		t.Errorf("thumbnail = %q, want it kept — the still rendered fine", clips[0].Thumbnail)
	}
	if clips[0].Preview != "" {
		t.Errorf("preview = %q, want empty — nothing was written", clips[0].Preview)
	}
}

// ⚠ A zero-byte file is a partial write from a killed ffmpeg. Adopting it would mean the clip
// never gets real artwork again, since every later pass would see "already generated".
func TestGenerateArtwork_RetriesAZeroByteFile(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "a.mp4"), "video")
	writeAt(t, filepath.Join(dir, filler.ThumbDirName, "a.jpg"), "")
	writeAt(t, filepath.Join(dir, filler.ThumbDirName, "a.webp"), "")
	clips := []filler.RawClip{{Path: "a.mp4", DurationMs: 30_000}}

	var starts []float64
	filler.GenerateArtwork(context.Background(), dir, clips, fakeArtwork(&starts, false))
	if len(starts) != 1 {
		t.Errorf("a zero-byte file must be retried, renderer ran %d times", len(starts))
	}
}

// ⚠ A partial file from a killed ffmpeg is removed, so the next pass does not adopt it. Only the
// EMPTY ones — a render that failed on the animation must not destroy a good still.
func TestGenerateArtwork_RemovesOnlyTheEmptyPartials(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "a.mp4"), "video")
	clips := []filler.RawClip{{Path: "a.mp4", DurationMs: 30_000}}

	// Writes a good still and a truncated (empty) animation, then fails — a kill mid-encode.
	partial := func(_ context.Context, _, stillDst, animDst string, _ float64) error {
		if err := os.MkdirAll(filepath.Dir(stillDst), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(stillDst, []byte("\xff\xd8\xff jpeg"), 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(animDst, nil, 0o600); err != nil {
			return err
		}
		return errors.New("ffmpeg: killed")
	}
	filler.GenerateArtwork(context.Background(), dir, clips, partial)

	if _, err := os.Stat(filepath.Join(dir, filler.ThumbDirName, "a.webp")); err == nil {
		t.Error("the empty animation survived; the next pass would adopt it and never retry")
	}
	if _, err := os.Stat(filepath.Join(dir, filler.ThumbDirName, "a.jpg")); err != nil {
		t.Error("the good still was deleted along with the failed animation")
	}
}

// The no-op guards: nothing to do, and nowhere to do it.
func TestGenerateArtwork_NoDirOrNoClips(t *testing.T) {
	var starts []float64
	if failed := filler.GenerateArtwork(context.Background(), "", []filler.RawClip{{Path: "c.mp4"}}, fakeArtwork(&starts, false)); failed != 0 {
		t.Errorf("failed = %d with no dir, want 0", failed)
	}
	if failed := filler.GenerateArtwork(context.Background(), t.TempDir(), nil, fakeArtwork(&starts, false)); failed != 0 {
		t.Errorf("failed = %d with no clips, want 0", failed)
	}
	if len(starts) != 0 {
		t.Errorf("rendered %v with nothing to do", starts)
	}
}
