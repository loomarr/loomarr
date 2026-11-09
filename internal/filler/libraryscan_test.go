package filler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/asset"
)

// libraryOnDisk writes clip files into a library folder, with subfolders as the names say.
func libraryOnDisk(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for i, name := range names {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		body := make([]byte, 2048)
		for j := range body {
			body[j] = byte(i*7 + j%251)
		}
		if err := os.WriteFile(p, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The whole point of the kind: clips land in the WATCH folder, so intake files them exactly as it
// files a hand-dropped file. Copying them straight into the clip folder would be a second way in
// — a divergent path where the hash, the dedupe and the sidecar could each be forgotten. The
// library folder itself is the household's and is left exactly as it was.
func TestLibraryScan_CopiesIntoTheWatchFolderForIntake(t *testing.T) {
	lib := libraryOnDisk(t, "Crunchlings 1993.mp4", "Idents/Station ID.mkv", "Idents/cover.jpg", ".staging/part.mp4")
	watch := filepath.Join(t.TempDir(), "watch")

	res, err := NewLibraryScanner(nil).Scan(context.Background(), lib, watch, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != 2 || res.Copied != 2 || res.Unreadable != 0 {
		t.Fatalf("Scan = %+v, want the 2 clips in the folder and its subfolder copied, the cover and "+
			"the dot-folder skipped", res)
	}
	for _, name := range []string{"Crunchlings 1993.mp4", "Idents/Station ID.mkv"} {
		if _, err := os.Stat(filepath.Join(lib, name)); err != nil {
			t.Errorf("the library's own %s is gone — a library folder is copied from, never drained", name)
		}
	}

	// ⚠ The ORIGINAL FILENAME is preserved. §8 grounds an era only on a year that appears
	// literally in a text signal, and intake records the arrival name as `originalName` — so
	// renaming to `lib-0001.mp4` on the way in would strip 1993 before intake ever saw it and
	// the clip would be ungrounded on arrival.
	if _, err := os.Stat(filepath.Join(watch, "Crunchlings 1993.mp4")); err != nil {
		t.Errorf("the original filename did not survive the copy: %v", err)
	}
}

// A clip still waiting in the watch folder, or one the catalog already holds, is not copied again,
// and an all-catalogued folder is not mistaken for unreachable storage (#1605). Intake empties the
// watch folder, so without the catalog check every scheduled scan would read the household's whole
// folder off disk again, forever.
func TestLibraryScan_SkipsClipsTheCatalogAlreadyHolds(t *testing.T) {
	lib := libraryOnDisk(t, "filed.mp4", "new.mp4", "waiting.mp4")
	filedID, err := asset.ClipID(filepath.Join(lib, "filed.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	watch := filepath.Join(t.TempDir(), "watch")
	if err := os.MkdirAll(watch, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(watch, "waiting.mp4"), []byte("queued"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := NewLibraryScanner(nil).Scan(context.Background(), lib, watch, func(id, ext string) bool {
		return id == filedID && ext == ".mp4"
	})
	if err != nil || res.Found != 3 || res.Copied != 1 || res.Catalogued != 1 || res.Unreadable != 0 {
		t.Fatalf("Scan = %+v, %v; want the new clip copied and the filed and waiting ones skipped", res, err)
	}
	if _, err := os.Stat(filepath.Join(watch, "filed.mp4")); err == nil {
		t.Error("a catalogued clip was copied into the watch folder again")
	}

	all := libraryOnDisk(t, "filed.mp4")
	res, err = NewLibraryScanner(nil).Scan(context.Background(), all, filepath.Join(t.TempDir(), "watch"),
		func(string, string) bool { return true })
	if err != nil || res.Catalogued != 1 {
		t.Errorf("all-catalogued Scan = %+v, %v; want no error", res, err)
	}
}

// ⚠ A folder that isn't there is a mount problem, not an empty library. A silent zero result is
// indistinguishable from "this folder has no clips", and the two remedies are completely
// different: mount a volume, or add some clips. The named error is what lets the UI say which.
func TestLibraryScan_AnUnreadableFolderIsNamedNotSilent(t *testing.T) {
	watch := filepath.Join(t.TempDir(), "watch")
	_, err := NewLibraryScanner(nil).Scan(context.Background(), filepath.Join(t.TempDir(), "not-mounted"), watch, nil)
	if !errors.Is(err, ErrLibraryUnreachableStorage) {
		t.Errorf("err = %v, want ErrLibraryUnreachableStorage — an operator cannot tell a mount "+
			"problem from an empty library without it", err)
	}
}

// A genuinely EMPTY folder is not a storage error. The mirror of the test above: if both answered
// the same way, the named error would carry no information.
func TestLibraryScan_AnEmptyLibraryIsNotAnError(t *testing.T) {
	watch := filepath.Join(t.TempDir(), "watch")
	res, err := NewLibraryScanner(nil).Scan(context.Background(), t.TempDir(), watch, nil)
	if err != nil {
		t.Errorf("an empty library reported %v, want no error", err)
	}
	if res != (LibraryScanResult{}) {
		t.Errorf("Scan = %+v, want an empty result", res)
	}
}

// Nothing configured is a no-op. The sync calls this for every source row, and a library row with
// no folder must not become a scan of everything.
func TestLibraryScan_UnconfiguredIsANoOp(t *testing.T) {
	s := NewLibraryScanner(nil)
	for _, tc := range []struct{ name, lib, watch string }{
		{"no folder", "", "/tmp/watch"},
		{"no watch folder", t.TempDir(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := s.Scan(context.Background(), tc.lib, tc.watch, nil)
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

// ⚠ THE §9.1 guarantee, one level down. One unreadable source must not cost the others: an
// operator whose library volume is unmounted must still get the clips sitting in their other
// folders. A `return err` in that loop would mean a single dead source silently costs a channel
// every commercial it already had on disk.
func TestDrainScanSources_OneBadSourceDoesNotStopTheRest(t *testing.T) {
	good := libraryOnDisk(t, "reachable.mp4")
	watch := filepath.Join(t.TempDir(), "watch")

	clipDir := t.TempDir()
	s := &Syncer{
		dir:   clipDir,
		watch: watch,
		scanSources: fakeScanSources{srcs: []ScanSource{
			{ID: "lib:broken", Kind: "library", URI: filepath.Join(t.TempDir(), "not-mounted")},
			{ID: "lib:working", Kind: "library", URI: good},
		}},
		libraries: NewLibraryScanner(nil),
	}
	s.drainScanSources(context.Background(), nil)

	var filed string
	_ = filepath.WalkDir(clipDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(path) == ".mp4" {
			filed = path
		}
		return nil
	})
	if filed == "" {
		t.Error("the working library's clip never arrived — one dead source cost the others")
	} else if tags, ok := asset.ReadSidecarTags(filed); !ok || tags.SourceID != "lib:working" || !asset.SidecarFetchedByUs(filed) {
		t.Errorf("working library provenance = %+v/%v, want lib:working held-pipeline marker", tags, ok)
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
		watch:       filepath.Join(clipDir, WatchDirName),
		scanSources: fakeScanSources{srcs: []ScanSource{{ID: "folder:extra", Kind: "folder", URI: extra}}},
	}
	s.drainScanSources(context.Background(), nil)

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
	if len(filed) == 1 {
		if tags, ok := asset.ReadSidecarTags(filed[0]); !ok || tags.SourceID != "folder:extra" || !asset.SidecarFetchedByUs(filed[0]) {
			t.Errorf("registered-folder provenance = %+v/%v, want folder:extra held-pipeline marker", tags, ok)
		}
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
		watch:       filepath.Join(clipDir, WatchDirName),
		scanSources: fakeScanSources{srcs: []ScanSource{{ID: "folder:self", Kind: "folder", URI: clipDir}}},
	}
	s.drainScanSources(context.Background(), nil)

	if _, err := os.Stat(filepath.Join(clipDir, "already-here.mp4")); err != nil {
		t.Errorf("the clip folder was drained into itself — the whole catalog would re-file "+
			"on every pass: %v", err)
	}
}

func TestDrainScanSources_SkipsEveryAliasAndOverlapOfTheClipLibrary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source func(t *testing.T, parent, clipDir, filed string) string
	}{
		{
			name: "symlink alias",
			source: func(t *testing.T, _, clipDir, _ string) string {
				alias := filepath.Join(t.TempDir(), "clip-alias")
				if err := os.Symlink(clipDir, alias); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				return alias
			},
		},
		{
			name: "symlink descendant",
			source: func(t *testing.T, _, _, filed string) string {
				alias := filepath.Join(t.TempDir(), "shard-alias")
				if err := os.Symlink(filepath.Dir(filed), alias); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				return alias
			},
		},
		{name: "parent", source: func(_ *testing.T, parent, _, _ string) string { return parent }},
		{name: "descendant", source: func(_ *testing.T, _, _, filed string) string { return filepath.Dir(filed) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			clipDir := filepath.Join(parent, "clips")
			layout, err := NewLayout(clipDir, "")
			if err != nil {
				t.Fatal(err)
			}
			body := make([]byte, 2048)
			for i := range body {
				body[i] = byte(i % 251)
			}
			candidate := filepath.Join(t.TempDir(), "candidate.mp4")
			if err := os.WriteFile(candidate, body, 0o600); err != nil {
				t.Fatal(err)
			}
			id, err := asset.ClipID(candidate)
			if err != nil {
				t.Fatal(err)
			}
			filed, err := ClipPath(layout.ClipDir(), id, ".mp4")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(filed), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(candidate, filed); err != nil {
				t.Fatal(err)
			}

			source := tc.source(t, parent, layout.ClipDir(), filed)
			if _, err := layout.intakeSource(source); err == nil {
				t.Fatalf("overlapping registered source %q passed layout validation", source)
			}
			s := &Syncer{
				layout:      layout,
				dir:         layout.ClipDir(),
				watch:       layout.WatchDir(),
				scanSources: fakeScanSources{srcs: []ScanSource{{ID: "folder:overlap", Kind: "folder", URI: source}}},
			}
			s.drainScanSources(context.Background(), nil)

			if _, err := os.Stat(filed); err != nil {
				t.Errorf("registered overlap removed the live catalog clip: %v", err)
			}
		})
	}
}

// A library row still naming a media-server library rather than a folder (one the cutover could
// not rewrite) does no work, and the folder rows still drain.
func TestDrainScanSources_AnUnrewrittenLibraryRowDoesNoWork(t *testing.T) {
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
		watch: filepath.Join(clipDir, WatchDirName),
		scanSources: fakeScanSources{srcs: []ScanSource{
			{ID: "lib:none", Kind: "library", URI: "Commercials"},
			{ID: "folder:extra", Kind: "folder", URI: extra},
		}},
		libraries: NewLibraryScanner(nil),
	}
	s.drainScanSources(context.Background(), nil)

	if left, _ := os.ReadDir(extra); len(left) != 0 {
		t.Error("a library row naming a library rather than a folder stopped the folder rows from draining")
	}
}

// No scan sources wired at all ⇒ the pre-V38c behaviour, unchanged. Every existing Syncer
// construction must keep working without opting in.
func TestDrainScanSources_UnwiredIsANoOp(t *testing.T) {
	s := &Syncer{dir: t.TempDir()}
	s.drainScanSources(context.Background(), nil) // must not panic
}

// #1605: re-syncing an UNCHANGED library folder must add nothing. Intake moves each copy out of
// the watch folder, and the transcode then re-keys the filed clip to its normalized derivative's
// hash, so neither the watch folder nor the raw hash's filed path remembers the clip any more.
// A second scan that copies the clip again files it as new, and its transcode later collides
// with the first copy's derivative on clips.hash.
func TestDrainScanSources_ReSyncOfAnUnchangedLibraryAddsNothing(t *testing.T) {
	lib := libraryOnDisk(t, "Crunchlings 1993.mp4")
	rawID, err := asset.ClipID(filepath.Join(lib, "Crunchlings 1993.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	clipDir := t.TempDir()
	s := &Syncer{
		dir:         clipDir,
		watch:       filepath.Join(t.TempDir(), "watch"),
		scanSources: fakeScanSources{srcs: []ScanSource{{ID: "lib:ads", Kind: "library", URI: lib}}},
		libraries:   NewLibraryScanner(nil),
	}

	s.drainScanSources(context.Background(), nil)
	rawFiled, err := ClipPath(clipDir, rawID, ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rawFiled); err != nil {
		t.Fatalf("first sync did not file the library clip: %v", err)
	}

	// The transcode retains the raw bytes as a source master under the media-asset root (keyed by
	// SHA-256, not by the clip identity), publishes the normalized derivative under its own hash
	// with a sidecar naming that master, and retires the raw filed path.
	tags, _ := asset.ReadSidecarTags(rawFiled)
	master, err := asset.PreserveSourceMasterWithStorage(context.Background(), clipDir, rawFiled, rawID, tags, nil)
	if err != nil {
		t.Fatal(err)
	}
	derived := make([]byte, 4096)
	for i := range derived {
		derived[i] = byte(255 - i%251)
	}
	derivedProbe := filepath.Join(t.TempDir(), "derived.mp4")
	if err := os.WriteFile(derivedProbe, derived, 0o600); err != nil {
		t.Fatal(err)
	}
	derivedID, err := asset.ClipID(derivedProbe)
	if err != nil {
		t.Fatal(err)
	}
	derivedFiled, err := ClipPath(clipDir, derivedID, ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(derivedFiled), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(derivedFiled, derived, 0o600); err != nil {
		t.Fatal(err)
	}
	tags.MediaAssets = &asset.MediaAssetManifest{Version: asset.MediaAssetManifestVersion, SourceMaster: master}
	if err := asset.WriteSidecarTags(derivedFiled, tags, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(asset.SidecarPathFor(rawFiled)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(rawFiled); err != nil {
		t.Fatal(err)
	}
	if got, ok := asset.ReadSidecarTags(derivedFiled); !ok || got.MediaAssets == nil || got.MediaAssets.SourceMaster.ClipHash != rawID {
		t.Fatalf("derivative sidecar = %+v/%v, want it to name the raw source master", got, ok)
	}

	s.drainScanSources(context.Background(), nil)
	if _, err := os.Stat(rawFiled); err == nil {
		t.Fatal("the re-sync filed the unchanged library clip again under its raw hash")
	}
	if left, _ := os.ReadDir(s.watch); len(left) != 0 {
		t.Errorf("the re-sync left %d copies waiting in the watch folder", len(left))
	}
}
