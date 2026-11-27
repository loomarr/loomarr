package cliplibrary_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/media/tools"
)

// countingProbe counts the files it is asked to read.
type countingProbe struct{ calls map[string]int }

func newCountingProbe() *countingProbe { return &countingProbe{calls: map[string]int{}} }

func (p *countingProbe) probe(_ context.Context, path string) (tools.Probed, error) {
	p.calls[filepath.Base(path)]++
	return tools.Probed{DurationMs: 30_000, Height: 480}, nil
}

func (p *countingProbe) total() int {
	n := 0
	for _, c := range p.calls {
		n += c
	}
	return n
}

// The 15-minute sync re-walks the folder; a file whose size and modification time are what they
// were is not read by ffprobe again, and the scan reports the same clip (#3855).
func TestScanDir_UnchangedFilesAreNotProbedAgain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a/b/90000001.mp4")
	writeFile(t, dir, "a/b/90000002.mp4")
	probe, cache := newCountingProbe(), cliplibrary.NewProbeCache()

	first, _, err := cliplibrary.ScanDirCached(t.Context(), dir, probe.probe, cache)
	if err != nil {
		t.Fatal(err)
	}
	if probe.total() != 2 {
		t.Fatalf("first scan probed %d files, want 2", probe.total())
	}
	second, _, err := cliplibrary.ScanDirCached(t.Context(), dir, probe.probe, cache)
	if err != nil {
		t.Fatal(err)
	}
	if probe.total() != 2 {
		t.Fatalf("an unchanged folder was probed again: %v", probe.calls)
	}
	if len(second) != len(first) || second[0].DurationMs != first[0].DurationMs || second[0].Quality != first[0].Quality ||
		second[0].ID != first[0].ID {
		t.Fatalf("second scan = %+v, want the clips the first reported %+v", second, first)
	}
}

// A file that changed is read again: a different size, or the same size with a new modification
// time. Its neighbours are not.
func TestScanDir_ChangedFilesAreProbedAgain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a/b/90000001.mp4")
	writeFile(t, dir, "a/b/90000002.mp4")
	writeFile(t, dir, "a/b/90000003.mp4")
	probe, cache := newCountingProbe(), cliplibrary.NewProbeCache()
	if _, _, err := cliplibrary.ScanDirCached(t.Context(), dir, probe.probe, cache); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "a/b/90000001.mp4"), []byte("longer than before"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "a/b/90000002.mp4"), later, later); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cliplibrary.ScanDirCached(t.Context(), dir, probe.probe, cache); err != nil {
		t.Fatal(err)
	}
	if probe.calls["90000001.mp4"] != 2 || probe.calls["90000002.mp4"] != 2 || probe.calls["90000003.mp4"] != 1 {
		t.Fatalf("probe calls = %v, want the two changed files read again and the third once", probe.calls)
	}
}

// A file that could not be read is never remembered as readable: it is probed on every scan until it
// works.
func TestScanDir_AFileThatFailedItsProbeIsProbedAgain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a/b/90000001.mp4")
	calls := 0
	failing := func(context.Context, string) (tools.Probed, error) {
		calls++
		if calls == 1 {
			return tools.Probed{}, os.ErrInvalid
		}
		return tools.Probed{DurationMs: 30_000}, nil
	}
	cache := cliplibrary.NewProbeCache()
	if clips, _, err := cliplibrary.ScanDirCached(t.Context(), dir, failing, cache); err != nil || len(clips) != 0 {
		t.Fatalf("first scan = %v %v, want the unreadable file skipped", clips, err)
	}
	if clips, _, err := cliplibrary.ScanDirCached(t.Context(), dir, failing, cache); err != nil || len(clips) != 1 {
		t.Fatalf("second scan = %v %v, want the file read again and catalogued", clips, err)
	}
}
