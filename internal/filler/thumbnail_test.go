package filler_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// writeAt creates a file with content, making parents as needed. (scan_test.go has a
// writeFile with a different signature; this one takes a full path.)
func writeAt(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeThumbs writes a stand-in JPEG, recording what it was asked for. No binary runs — the
// same injection seam Prober uses (unit tests never touch the network or exec, per CLAUDE.md).
func fakeThumbs(calls *[]string, fail bool) filler.Thumbnailer {
	return func(_ context.Context, src, dst string) error {
		*calls = append(*calls, src)
		if fail {
			return errors.New("ffmpeg: no such file or directory")
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return err
		}
		return os.WriteFile(dst, []byte("\xff\xd8\xff jpeg"), 0o600)
	}
}

// The path mirrors the clip's own structure — flattening would collide two clips named
// intro.mp4 in different era folders, silently, with one overwriting the other.
func TestThumbPathForPreservesStructure(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"1994/toys-transformers.mp4", "1994/toys-transformers.jpg"},
		{"1988/intro.mkv", "1988/intro.jpg"},
		{"bumper.webm", "bumper.jpg"},
		{"", ""},
	} {
		if got := filler.ThumbPathFor(tc.in); got != tc.want {
			t.Errorf("ThumbPathFor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// The collision case, stated explicitly.
	a := filler.ThumbPathFor("1994/intro.mp4")
	b := filler.ThumbPathFor("1988/intro.mp4")
	if a == b {
		t.Errorf("two clips named intro.mp4 in different folders must not share a thumbnail path (%q)", a)
	}
}

func TestGenerateThumbnailsFillsPaths(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "1994", "toys.mp4"), "video")
	clips := []filler.RawClip{{Path: "1994/toys.mp4", Name: "toys"}}

	var calls []string
	if failed := filler.GenerateThumbnails(context.Background(), dir, clips, fakeThumbs(&calls, false)); failed != 0 {
		t.Fatalf("failed = %d, want 0", failed)
	}
	if clips[0].Thumbnail != "1994/toys.jpg" {
		t.Errorf("thumbnail = %q, want the relative path", clips[0].Thumbnail)
	}
	// Relative in the row, absolute on disk.
	if _, err := os.Stat(filepath.Join(dir, filler.ThumbDirName, "1994", "toys.jpg")); err != nil {
		t.Errorf("no image written: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("extractor called %d times, want 1", len(calls))
	}
}

// ⚠ A failure leaves Thumbnail EMPTY. Recording a path to a file that does not exist would
// render as a broken image; empty renders as no image, which is the honest state.
func TestGenerateThumbnailsCountsFailuresAndLeavesPathEmpty(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "a.mp4"), "video")
	writeAt(t, filepath.Join(dir, "b.mp4"), "video")
	clips := []filler.RawClip{{Path: "a.mp4"}, {Path: "b.mp4"}}

	var calls []string
	failed := filler.GenerateThumbnails(context.Background(), dir, clips, fakeThumbs(&calls, true))
	if failed != 2 {
		t.Errorf("failed = %d, want 2 — a silent skip is what once produced an empty catalog with no error", failed)
	}
	for _, c := range clips {
		if c.Thumbnail != "" {
			t.Errorf("a failed extraction must leave Thumbnail empty, got %q", c.Thumbnail)
		}
	}
}

// An existing image is adopted, not regenerated: the scan runs periodically, and re-extracting
// every frame each pass would spend an exec per clip per cycle forever to rebuild a correct file.
func TestGenerateThumbnailsAdoptsExistingImages(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "1994", "toys.mp4"), "video")
	writeAt(t, filepath.Join(dir, filler.ThumbDirName, "1994", "toys.jpg"), "already here")
	clips := []filler.RawClip{{Path: "1994/toys.mp4"}}

	var calls []string
	filler.GenerateThumbnails(context.Background(), dir, clips, fakeThumbs(&calls, false))
	if len(calls) != 0 {
		t.Errorf("extractor ran %d times for an existing image, want 0", len(calls))
	}
	if clips[0].Thumbnail != "1994/toys.jpg" {
		t.Errorf("an existing image must still be adopted, got %q", clips[0].Thumbnail)
	}
}

// A zero-byte file is a partial write from a killed ffmpeg. Adopting it would mean the clip
// never gets a real thumbnail again, since every later pass would see "already generated".
func TestGenerateThumbnailsRetriesAZeroByteImage(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "a.mp4"), "video")
	writeAt(t, filepath.Join(dir, filler.ThumbDirName, "a.jpg"), "")
	clips := []filler.RawClip{{Path: "a.mp4"}}

	var calls []string
	filler.GenerateThumbnails(context.Background(), dir, clips, fakeThumbs(&calls, false))
	if len(calls) != 1 {
		t.Errorf("a zero-byte image must be retried, extractor ran %d times", len(calls))
	}
}

// A failed extraction removes any partial file, for the same reason.
func TestGenerateThumbnailsRemovesPartialsOnFailure(t *testing.T) {
	dir := t.TempDir()
	writeAt(t, filepath.Join(dir, "a.mp4"), "video")
	clips := []filler.RawClip{{Path: "a.mp4"}}

	partial := filepath.Join(dir, filler.ThumbDirName, "a.jpg")
	extract := func(_ context.Context, _, dst string) error {
		_ = os.MkdirAll(filepath.Dir(dst), 0o750)
		_ = os.WriteFile(dst, []byte("half"), 0o600)
		return errors.New("killed")
	}
	filler.GenerateThumbnails(context.Background(), dir, clips, extract)

	if _, err := os.Stat(partial); err == nil {
		t.Error("a partial file must be removed, or the next pass adopts it and never retries")
	}
}

// Filler not configured is an empty catalog, not an error — the same contract ScanDir has.
func TestGenerateThumbnailsNoDirIsNotAnError(t *testing.T) {
	var calls []string
	if failed := filler.GenerateThumbnails(context.Background(), "", nil, fakeThumbs(&calls, true)); failed != 0 {
		t.Errorf("failed = %d, want 0 for an unconfigured dir", failed)
	}
}
