package filler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeLister stands in for a media server. `err` makes the server unreachable.
type fakeLister struct {
	clips []LibraryClip
	err   error
	calls int
}

func (f *fakeLister) ListLibraryClips(context.Context, string) ([]LibraryClip, error) {
	f.calls++
	return f.clips, f.err
}

// libraryOnDisk writes files where a media server would have them, and returns the LibraryClips
// pointing at those paths — the shared-storage case that makes a library scannable.
func libraryOnDisk(t *testing.T, names ...string) (dir string, clips []LibraryClip) {
	t.Helper()
	dir = t.TempDir()
	for i, name := range names {
		p := filepath.Join(dir, name)
		body := make([]byte, 2048)
		for j := range body {
			body[j] = byte(i*7 + j%251)
		}
		if err := os.WriteFile(p, body, 0o600); err != nil {
			t.Fatal(err)
		}
		clips = append(clips, LibraryClip{Name: name, Path: p})
	}
	return dir, clips
}

// The whole point of the kind: clips land in the WATCH folder, so intake files them exactly as it
// files a hand-dropped file. Copying them straight into the clip folder would be a second way in
// — a divergent path where the hash, the dedupe and the sidecar could each be forgotten.
func TestLibraryScan_CopiesIntoTheWatchFolderForIntake(t *testing.T) {
	_, clips := libraryOnDisk(t, "Frosted Flakes 1993.mp4", "Station ID.mp4")
	watch := filepath.Join(t.TempDir(), "watch")

	s := NewLibraryScanner(&fakeLister{clips: clips}, nil)
	res, err := s.Scan(context.Background(), "Commercials", watch)
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != 2 || res.Copied != 2 || res.Unreadable != 0 {
		t.Fatalf("Scan = %+v, want 2 found and 2 copied", res)
	}

	// ⚠ The ORIGINAL FILENAME is preserved. §8 grounds an era only on a year that appears
	// literally in a text signal, and intake records the arrival name as `originalName` — so
	// renaming to `lib-0001.mp4` on the way in would strip 1993 before intake ever saw it and
	// the clip would be ungrounded on arrival.
	if _, err := os.Stat(filepath.Join(watch, "Frosted Flakes 1993.mp4")); err != nil {
		t.Errorf("the original filename did not survive the copy: %v", err)
	}
}

// A re-scan of an unchanged library writes nothing. Without this every scheduled scan re-copies
// the whole library; intake would correctly discard it all as duplicates, but Loomarr would be
// reading the operator's entire library off disk on a timer forever.
func TestLibraryScan_ReScanDoesNotReCopy(t *testing.T) {
	_, clips := libraryOnDisk(t, "a.mp4", "b.mp4")
	watch := filepath.Join(t.TempDir(), "watch")
	s := NewLibraryScanner(&fakeLister{clips: clips}, nil)

	if _, err := s.Scan(context.Background(), "Commercials", watch); err != nil {
		t.Fatal(err)
	}
	res, err := s.Scan(context.Background(), "Commercials", watch)
	if err != nil {
		t.Fatal(err)
	}
	if res.Copied != 0 {
		t.Errorf("Copied = %d on a re-scan, want 0 — the library is being re-read every pass", res.Copied)
	}
}

// ⚠ Listed clips that cannot be OPENED mean Loomarr and the media server do not share storage —
// a mount problem, not an empty library. A silent zero result is indistinguishable from "this
// library has no clips", and the two remedies are completely different: mount a volume, or add
// some clips. The named error is what lets the UI say the right one.
func TestLibraryScan_UnreadableStorageIsNamedNotSilent(t *testing.T) {
	clips := []LibraryClip{
		{Name: "a.mp4", Path: "/not/a/real/mount/a.mp4"},
		{Name: "b.mp4", Path: "/not/a/real/mount/b.mp4"},
	}
	watch := filepath.Join(t.TempDir(), "watch")

	s := NewLibraryScanner(&fakeLister{clips: clips}, nil)
	res, err := s.Scan(context.Background(), "Commercials", watch)
	if !errors.Is(err, ErrLibraryUnreachableStorage) {
		t.Errorf("err = %v, want ErrLibraryUnreachableStorage — an operator cannot tell a mount "+
			"problem from an empty library without it", err)
	}
	if res.Unreadable != 2 {
		t.Errorf("Unreadable = %d, want 2", res.Unreadable)
	}
}

// A genuinely EMPTY library is not a storage error. The mirror of the test above: if both
// answered the same way, the named error would carry no information.
func TestLibraryScan_AnEmptyLibraryIsNotAnError(t *testing.T) {
	watch := filepath.Join(t.TempDir(), "watch")
	s := NewLibraryScanner(&fakeLister{}, nil)

	res, err := s.Scan(context.Background(), "Commercials", watch)
	if err != nil {
		t.Errorf("an empty library reported %v, want no error", err)
	}
	if res != (LibraryScanResult{}) {
		t.Errorf("Scan = %+v, want an empty result", res)
	}
}

// ⚠ A partially readable library still copies what it can. Failing the whole scan because one
// file is missing would let a single stray item cost an operator their entire commercial break.
func TestLibraryScan_PartialReadabilityStillCopiesTheRest(t *testing.T) {
	_, clips := libraryOnDisk(t, "good.mp4")
	clips = append(clips, LibraryClip{Name: "gone.mp4", Path: "/not/a/real/mount/gone.mp4"})
	watch := filepath.Join(t.TempDir(), "watch")

	s := NewLibraryScanner(&fakeLister{clips: clips}, nil)
	res, err := s.Scan(context.Background(), "Commercials", watch)
	if err != nil {
		t.Fatalf("a partly readable library failed the whole scan: %v", err)
	}
	if res.Copied != 1 || res.Unreadable != 1 {
		t.Errorf("Scan = %+v, want 1 copied and 1 unreadable", res)
	}
}

// An unreachable media server is an ERROR the caller logs and moves past — never a panic, and
// never a silent success that would look like an empty library.
func TestLibraryScan_UnreachableServerReportsRatherThanPretending(t *testing.T) {
	watch := filepath.Join(t.TempDir(), "watch")
	s := NewLibraryScanner(&fakeLister{err: errors.New("connection refused")}, nil)

	if _, err := s.Scan(context.Background(), "Commercials", watch); err == nil {
		t.Error("an unreachable media server reported success — it is indistinguishable from " +
			"an empty library, which is the wrong thing to tell an operator")
	}
}

// Nothing configured is a no-op. The sync calls this for every source row, and a library row with
// no name must not become a scan of everything.
func TestLibraryScan_UnconfiguredIsANoOp(t *testing.T) {
	s := NewLibraryScanner(&fakeLister{clips: []LibraryClip{{Name: "a", Path: "/tmp/a"}}}, nil)
	for _, tc := range []struct{ name, lib, watch string }{
		{"no library name", "", "/tmp/watch"},
		{"no watch folder", "Commercials", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := s.Scan(context.Background(), tc.lib, tc.watch)
			if err != nil || res != (LibraryScanResult{}) {
				t.Errorf("Scan = %+v, %v; want an empty no-op", res, err)
			}
		})
	}
}

// --- The per-source drain inside the sync (§10 V38c) ---

// fakeScanSources is a ScanSourceStore returning a fixed list.
type fakeScanSources struct {
	srcs []ScanSource
	err  error
}

func (f fakeScanSources) ListScanSources(context.Context) ([]ScanSource, error) {
	return f.srcs, f.err
}

// countingLister fails for one named library and succeeds for the rest, so a test can prove the
// loop CONTINUES past a failure rather than merely that it does not crash.
type countingLister struct {
	failFor string
	byName  map[string][]LibraryClip
	seen    []string
}

func (c *countingLister) ListLibraryClips(_ context.Context, name string) ([]LibraryClip, error) {
	c.seen = append(c.seen, name)
	if name == c.failFor {
		return nil, errors.New("connection refused")
	}
	return c.byName[name], nil
}

// ⚠ THE §9.1 guarantee, one level down. One unreachable source must not cost the others: an
// operator whose media server is off must still get the clips sitting in their watched folders.
// A `return err` in that loop would mean a single dead source silently costs a channel every
// commercial it already had on disk.
func TestDrainScanSources_OneBadSourceDoesNotStopTheRest(t *testing.T) {
	_, good := libraryOnDisk(t, "reachable.mp4")
	watch := filepath.Join(t.TempDir(), "watch")
	lister := &countingLister{
		failFor: "Broken",
		byName:  map[string][]LibraryClip{"Working": good},
	}

	s := &Syncer{
		dir:   t.TempDir(),
		watch: func() string { return watch },
		scanSources: fakeScanSources{srcs: []ScanSource{
			{ID: "lib:broken", Kind: "library", URI: "Broken"},
			{ID: "lib:working", Kind: "library", URI: "Working"},
		}},
		libraries: NewLibraryScanner(lister, nil),
	}
	s.drainScanSources(context.Background())

	// The loop reached the SECOND library after the first one failed.
	if len(lister.seen) != 2 {
		t.Fatalf("listed %v, want both libraries — the loop stopped at the failure", lister.seen)
	}
	if _, err := os.Stat(filepath.Join(watch, "reachable.mp4")); err != nil {
		t.Errorf("the working library's clip never arrived: %v — one dead source cost the others", err)
	}
}

// A registered folder is DRAINED into the watch folder, not scanned where it sits. Scanning in
// place would make it a second catalog location — the divergent path §10 forbids — and the same
// advert in two folders would become two clips instead of one.
func TestDrainScanSources_RegisteredFolderIsDrainedNotScannedInPlace(t *testing.T) {
	clipDir := t.TempDir()
	extra := t.TempDir()
	body := make([]byte, 2048)
	for i := range body {
		body[i] = byte(i % 251)
	}
	if err := os.WriteFile(filepath.Join(extra, "Toy Ad 1994.mp4"), body, 0o600); err != nil {
		t.Fatal(err)
	}

	s := &Syncer{
		dir:         clipDir,
		watch:       func() string { return filepath.Join(clipDir, WatchDirName) },
		scanSources: fakeScanSources{srcs: []ScanSource{{ID: "folder:extra", Kind: "folder", URI: extra}}},
	}
	s.drainScanSources(context.Background())

	// It was filed into the CLIP folder under its hash, and the registered folder drained.
	var filed []string
	_ = filepath.WalkDir(clipDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(p) == ".mp4" {
			filed = append(filed, p)
		}
		return nil
	})
	if len(filed) != 1 {
		t.Fatalf("clip folder holds %v, want the one drained clip", filed)
	}
	if left, _ := os.ReadDir(extra); len(left) != 0 {
		t.Errorf("the registered folder still holds %d files — it was not drained", len(left))
	}
}

// ⚠ The CONFIGURED clip folder is skipped even if a row names it. Draining it into the watch
// folder would move the entire catalog back through intake on every single pass — every clip
// re-hashed and re-filed forever, for no change.
func TestDrainScanSources_SkipsTheClipFolderItself(t *testing.T) {
	clipDir := t.TempDir()
	body := make([]byte, 2048)
	for i := range body {
		body[i] = byte(i % 251)
	}
	// A clip already filed in the clip folder.
	if err := os.WriteFile(filepath.Join(clipDir, "already-here.mp4"), body, 0o600); err != nil {
		t.Fatal(err)
	}

	s := &Syncer{
		dir:         clipDir,
		watch:       func() string { return filepath.Join(clipDir, WatchDirName) },
		scanSources: fakeScanSources{srcs: []ScanSource{{ID: "folder:self", Kind: "folder", URI: clipDir}}},
	}
	s.drainScanSources(context.Background())

	if _, err := os.Stat(filepath.Join(clipDir, "already-here.mp4")); err != nil {
		t.Errorf("the clip folder was drained into itself — the whole catalog would re-file "+
			"on every pass: %v", err)
	}
}

// An install with NO media server still drains its folders. Library rows do no work rather than
// failing — the same rule Tunarr registration follows, and the one §9.1 exists to protect.
func TestDrainScanSources_NoMediaServerStillDrainsFolders(t *testing.T) {
	clipDir := t.TempDir()
	extra := t.TempDir()
	body := make([]byte, 2048)
	for i := range body {
		body[i] = byte(i % 251)
	}
	if err := os.WriteFile(filepath.Join(extra, "ad.mp4"), body, 0o600); err != nil {
		t.Fatal(err)
	}

	s := &Syncer{
		dir:   clipDir,
		watch: func() string { return filepath.Join(clipDir, WatchDirName) },
		scanSources: fakeScanSources{srcs: []ScanSource{
			{ID: "lib:none", Kind: "library", URI: "Commercials"},
			{ID: "folder:extra", Kind: "folder", URI: extra},
		}},
		libraries: nil, // no media server configured
	}
	s.drainScanSources(context.Background())

	if left, _ := os.ReadDir(extra); len(left) != 0 {
		t.Error("a library row with no media server stopped the folder rows from draining")
	}
}

// No scan sources wired at all ⇒ the pre-V38c behaviour, unchanged. Every existing Syncer
// construction must keep working without opting in.
func TestDrainScanSources_UnwiredIsANoOp(t *testing.T) {
	s := &Syncer{dir: t.TempDir()}
	s.drainScanSources(context.Background()) // must not panic
}
