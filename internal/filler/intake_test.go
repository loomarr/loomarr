package filler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Intake is the ONE route into the catalog (§10 V38c), so these tests are the guard on the
// property that makes that claim worth anything: whatever arrives — a download, a hand-dropped
// file, a duplicate — comes out the far side as `<clipDir>/xx/yy/<hash>.<ext>` with a sidecar
// beside it, or stays in the watch folder to be retried. Never half of each.

// writeClip puts a media file in the watch folder with `size` bytes of deterministic content.
// The content varies with the seed so two clips differ in their hash windows, not just in length.
func writeClip(t *testing.T, dir, name string, size int, seed byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := make([]byte, size)
	for i := range body {
		body[i] = seed + byte(i%251)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// clipsUnder lists every media file in the clip folder, relative to it — the shape assertions
// below are about LAYOUT, so they compare relative paths.
func clipsUnder(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(path, ".info.json") {
			return nil //nolint:nilerr // walk errors are not what this helper reports
		}
		rel, _ := filepath.Rel(dir, path)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func newIntakeDirs(t *testing.T) (watch, clips string) {
	t.Helper()
	root := t.TempDir()
	watch, clips = filepath.Join(root, "watch"), filepath.Join(root, "clips")
	if err := os.MkdirAll(watch, 0o755); err != nil {
		t.Fatal(err)
	}
	return watch, clips
}

// The whole contract in one pass: the file lands at its sharded hash name, the watch folder
// drains, and the original filename survives into the sidecar.
func TestTakeIn_FilesUnderTheHashAndKeepsTheOriginalName(t *testing.T) {
	watch, clips := newIntakeDirs(t)
	src := writeClip(t, watch, "Frosted Flakes 1993.mp4", 4096, 7)

	want, err := ClipID(src)
	if err != nil {
		t.Fatal(err)
	}

	res, err := TakeIn(watch, clips, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Taken != 1 || res.Duplicates != 0 || res.Skipped != 0 {
		t.Fatalf("TakeIn = %+v, want 1 taken and nothing else", res)
	}

	got := clipsUnder(t, clips)
	wantRel := want[0:2] + "/" + want[2:4] + "/" + want + ".mp4"
	if len(got) != 1 || got[0] != wantRel {
		t.Fatalf("clip folder holds %v, want [%s]", got, wantRel)
	}
	// The watch folder drained — an arrival left behind is re-hashed on every pass forever.
	if left := clipsUnder(t, watch); len(left) != 0 {
		t.Errorf("watch folder still holds %v, want empty", left)
	}

	// ⚠ THE point of the sidecar. Once the file is `a3f9….mp4` the path carries no year, so
	// §10's grounding rule — an era must appear literally in a text signal — would reject every
	// clip whose era came from its filename. This is where that signal now lives.
	tags, ok := ReadSidecarTags(filepath.Join(clips, wantRel))
	if !ok {
		t.Fatal("no sidecar written beside the filed clip")
	}
	if tags.OriginalName != "Frosted Flakes 1993.mp4" {
		t.Errorf("originalName = %q, want the pre-rename filename — the era signal is lost without it",
			tags.OriginalName)
	}
}

// The same advert arriving twice is filed once. Crucially the SECOND copy is removed from the
// watch folder rather than left there: a watch folder that never drains is the failure an
// operator actually notices.
func TestTakeIn_DeduplicatesByContentNotByName(t *testing.T) {
	watch, clips := newIntakeDirs(t)
	// Same bytes, different names, and in different subfolders — the case a path-keyed identity
	// got wrong, which is why identity moved to the hash in the first place.
	writeClip(t, watch, "a/coke.mp4", 4096, 3)
	writeClip(t, watch, "b/coca-cola-1985.mp4", 4096, 3)

	res, err := TakeIn(watch, clips, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Taken != 1 || res.Duplicates != 1 {
		t.Errorf("TakeIn = %+v, want 1 taken and 1 duplicate", res)
	}
	if got := clipsUnder(t, clips); len(got) != 1 {
		t.Errorf("clip folder holds %v, want a single copy", got)
	}
	if left := clipsUnder(t, watch); len(left) != 0 {
		t.Errorf("watch folder still holds %v — a duplicate left behind is re-hashed forever", left)
	}
}

// Two genuinely different clips are two clips. The mirror of the dedupe test: without it, a
// hash that collapsed everything to one value would pass the test above perfectly.
func TestTakeIn_KeepsDistinctClipsApart(t *testing.T) {
	watch, clips := newIntakeDirs(t)
	writeClip(t, watch, "toys.mp4", 4096, 1)
	writeClip(t, watch, "cereal.mp4", 4096, 200)

	res, err := TakeIn(watch, clips, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Taken != 2 || res.Duplicates != 0 {
		t.Errorf("TakeIn = %+v, want 2 taken", res)
	}
	if got := clipsUnder(t, clips); len(got) != 2 {
		t.Errorf("clip folder holds %v, want 2 distinct clips", got)
	}
}

// `fetched` is the held/filed fork's signal (§10 V38c), and it is written from the DOWNLOAD path
// only. A hand-dropped clip must not claim Loomarr fetched it — that would file material the
// approval gate never saw.
func TestTakeIn_RecordsFetchedByOnlyForOurOwnDownloads(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fetched bool
		want    bool
	}{
		{"downloaded by us", true, true},
		{"dropped by the operator", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			watch, clips := newIntakeDirs(t)
			writeClip(t, watch, "clip.mp4", 2048, 11)

			if _, err := TakeIn(watch, clips, tc.fetched, nil); err != nil {
				t.Fatal(err)
			}
			filed := clipsUnder(t, clips)
			if len(filed) != 1 {
				t.Fatalf("clip folder holds %v, want one clip", filed)
			}
			if got := SidecarFetchedByUs(filepath.Join(clips, filed[0])); got != tc.want {
				t.Errorf("SidecarFetchedByUs = %v, want %v", got, tc.want)
			}
		})
	}
}

// A downloader's sidecar travels with its clip. The title and description in it are the tagger's
// real text signals — losing them on the move would silently degrade every fetched clip to
// filename-only tagging, which is exactly the defect §10 records the sidecar reader was added for.
func TestTakeIn_CarriesTheDownloadersSidecarAcross(t *testing.T) {
	watch, clips := newIntakeDirs(t)
	src := writeClip(t, watch, "dl.mp4", 2048, 5)
	if err := os.WriteFile(sidecarPathFor(src),
		[]byte(`{"title":"Frosted Flakes","description":"They're grrreat","view_count":99}`),
		0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := TakeIn(watch, clips, true, nil); err != nil {
		t.Fatal(err)
	}
	filed := clipsUnder(t, clips)
	if len(filed) != 1 {
		t.Fatalf("clip folder holds %v, want one clip", filed)
	}
	dst := filepath.Join(clips, filed[0])

	raw, err := os.ReadFile(sidecarPathFor(dst))
	if err != nil {
		t.Fatalf("the downloader's sidecar did not travel with the clip: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["title"] != "Frosted Flakes" {
		t.Errorf("title = %v, want the downloader's — the tagger's text signal is gone", doc["title"])
	}
	// ⚠ A field we do not model. WriteSidecarTags round-trips through a map precisely so keys it
	// does not understand survive; this is the assertion that keeps that true.
	if doc["view_count"] == nil {
		t.Error("view_count was dropped — writing into someone else's file must preserve the unknown")
	}
}

// An unhashable arrival STAYS in the watch folder. This is the retry contract: the common cause
// is a file still being written, and the next pass finds it complete. Deleting or half-filing it
// would lose a clip to a race.
func TestTakeIn_LeavesUnusableArrivalsForTheNextPass(t *testing.T) {
	watch, clips := newIntakeDirs(t)
	// Empty — ClipID refuses it rather than keying every empty file on one shared hash.
	if err := os.WriteFile(filepath.Join(watch, "still-writing.mp4"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := TakeIn(watch, clips, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || res.Taken != 0 {
		t.Errorf("TakeIn = %+v, want 1 skipped and nothing taken", res)
	}
	if left := clipsUnder(t, watch); len(left) != 1 {
		t.Errorf("watch folder holds %v, want the file left for a retry", left)
	}
}

// Sidecars and part-files are not clips. Counting them as arrivals would inflate Skipped on
// every single pass — a permanently-alarming number that means nothing.
func TestTakeIn_IgnoresNonMediaInTheWatchFolder(t *testing.T) {
	watch, clips := newIntakeDirs(t)
	for _, name := range []string{"orphan.info.json", "partial.tmp", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(watch, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	res, err := TakeIn(watch, clips, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res != (IntakeResult{}) {
		t.Errorf("TakeIn = %+v, want an empty result — none of those are clips", res)
	}
}

// Unconfigured folders are a no-op, not an error. A fresh install has neither, and the sync job
// calls this unconditionally.
func TestTakeIn_UnconfiguredFoldersDoNothing(t *testing.T) {
	res, err := TakeIn("", "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res != (IntakeResult{}) {
		t.Errorf("TakeIn with no folders = %+v, want empty", res)
	}
}

// movePath's copy fallback is what makes intake work in a container, where the watch folder and
// the clip folder are separate bind mounts and `os.Rename` returns EXDEV. That cannot be
// reproduced in a unit test on one filesystem, so this exercises the fallback DIRECTLY — the
// copy path must move bytes exactly and leave no source behind.
func TestCopyFallback_MovesBytesAndRemovesTheSource(t *testing.T) {
	dir := t.TempDir()
	src := writeClip(t, dir, "src.mp4", 5000, 42)
	want, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "out", "dst.mp4")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("copied %d bytes, want %d identical", len(got), len(want))
	}
	// ⚠ The temp file copyFile writes must not survive. Left behind at `<hash>.mp4.tmp` it is
	// only litter; left at `<hash>.mp4` after a crash it would be a TRUNCATED file that the
	// duplicate check then treats as the complete clip, discarding every real copy that arrives.
	if _, err := os.Stat(dst + ".tmp"); err == nil {
		t.Error("the temp file survived the copy")
	}
}

// ⚠ A missing source is an ERROR for the media move and success for the sidecar. The two used to
// share one function that quietly returned nil, which meant a vanished clip counted as Taken and
// was catalogued as a row pointing at nothing.
func TestMove_MissingSourceFailsForMediaAndPassesForSidecars(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone.mp4")

	if err := movePath(missing, filepath.Join(dir, "dst.mp4")); err == nil {
		t.Error("movePath reported success for a source that does not exist — " +
			"intake would count it as taken and catalog a clip that is not there")
	}
	if err := moveIfPresent(missing, filepath.Join(dir, "dst.info.json")); err != nil {
		t.Errorf("moveIfPresent on an absent sidecar = %v, want nil — most clips arrive without one", err)
	}
}

// The derived default (§10 V38c). ⚠ It is derived rather than a literal `/data/filler/_watch`
// precisely so that pointing the clip folder at another disk MOVES the watch folder with it —
// a literal would leave arrivals landing under /data while the catalog looked elsewhere, and the
// drop-folder would appear broken with both settings looking correct.
func TestWatchDir_DerivesFromTheClipFolderUnlessSetExplicitly(t *testing.T) {
	for _, tc := range []struct {
		name, clipDir, watchDir, want string
	}{
		{"derived from the clip folder", "/data/filler", "", "/data/filler/" + WatchDirName},
		{"follows the clip folder elsewhere", "/mnt/library", "", "/mnt/library/" + WatchDirName},
		{"an explicit inbox wins", "/data/filler", "/mnt/inbox", "/mnt/inbox"},
		{"nothing configured stays nothing", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := WatchDir(tc.clipDir, tc.watchDir); got != tc.want {
				t.Errorf("WatchDir(%q, %q) = %q, want %q", tc.clipDir, tc.watchDir, got, tc.want)
			}
		})
	}
}

// ⚠ **THE case every unit test missed until the real binary was run.** `FILLER_DIR` has always
// been documented as *the* drop-folder — an operator copies `Frosted Flakes 1993.mp4` straight
// into it, and every release before V38c worked that way.
//
// Without draining the clip folder, such a file is scanned, catalogued at its arrival path, and
// PRUNED in the same pass: V38c's `ClipPath` allow-list accepts only `<hash>.<ext>` or
// `xx/yy/<hash>.<ext>`, so a human-readable name is not a valid clip id. The sync reports
// "1 added, 1 pruned" and the catalog holds nothing — filler silently never works, with every
// test green, because the tests all dropped their fixtures into the WATCH folder.
func TestTakeIn_FilesAClipDroppedStraightIntoTheClipFolder(t *testing.T) {
	root := t.TempDir()
	clips := filepath.Join(root, "clips")
	watch := filepath.Join(clips, WatchDirName)
	if err := os.MkdirAll(watch, 0o755); err != nil {
		t.Fatal(err)
	}
	// Dropped where the operator has always dropped things: the clip folder itself.
	writeClip(t, clips, "Frosted Flakes 1993.mp4", 4096, 9)

	res, err := TakeIn(watch, clips, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Taken != 1 {
		t.Fatalf("TakeIn = %+v, want the hand-dropped clip filed — it would be catalogued and "+
			"then pruned on the same pass", res)
	}

	got := clipsUnder(t, clips)
	if len(got) != 1 || !shardPath.MatchString(got[0]) {
		t.Fatalf("clip folder holds %v, want one clip at a sharded hash path", got)
	}
	// The era signal survived the rename, exactly as it does for a watch-folder arrival.
	tags, ok := ReadSidecarTags(filepath.Join(clips, got[0]))
	if !ok || tags.OriginalName != "Frosted Flakes 1993.mp4" {
		t.Errorf("originalName = %q, want the pre-rename filename", tags.OriginalName)
	}
}

// A clip ALREADY filed under its hash is left alone. Without this every pass would re-file the
// entire catalog — re-hashing, re-moving and re-writing every sidecar, forever.
func TestTakeIn_LeavesAlreadyFiledClipsAlone(t *testing.T) {
	root := t.TempDir()
	clips := filepath.Join(root, "clips")
	watch := filepath.Join(clips, WatchDirName)
	if err := os.MkdirAll(watch, 0o755); err != nil {
		t.Fatal(err)
	}
	writeClip(t, clips, "Toy Ad.mp4", 4096, 21)

	if _, err := TakeIn(watch, clips, false, nil); err != nil {
		t.Fatal(err)
	}
	filed := clipsUnder(t, clips)
	if len(filed) != 1 {
		t.Fatalf("setup: clip folder holds %v, want 1", filed)
	}

	// A second pass must do nothing at all.
	res, err := TakeIn(watch, clips, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res != (IntakeResult{}) {
		t.Errorf("a re-run did %+v, want nothing — the catalog is being re-filed every pass", res)
	}
	if again := clipsUnder(t, clips); len(again) != 1 || again[0] != filed[0] {
		t.Errorf("clip moved from %v to %v on a no-op pass", filed, again)
	}
}

// ⚠ A file waiting in the watch folder is handled ONCE. Draining the clip folder walks the same
// tree the watch folder sits in, so without skipping it an arrival is collected twice in one pass
// — and the second look finds a file the first has already moved.
func TestTakeIn_DoesNotCollectWatchFolderArrivalsTwice(t *testing.T) {
	root := t.TempDir()
	clips := filepath.Join(root, "clips")
	watch := filepath.Join(clips, WatchDirName)
	if err := os.MkdirAll(watch, 0o755); err != nil {
		t.Fatal(err)
	}
	writeClip(t, watch, "arriving.mp4", 4096, 33)

	res, err := TakeIn(watch, clips, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Taken != 1 || res.Skipped != 0 {
		t.Errorf("TakeIn = %+v, want exactly 1 taken and 0 skipped — the arrival was seen twice", res)
	}
}

func TestTakeIn_DoesNotCollectCustomNestedWatchArrivalsTwice(t *testing.T) {
	root := t.TempDir()
	clips := filepath.Join(root, "clips")
	watch := filepath.Join(clips, "inbox")
	if err := os.MkdirAll(watch, 0o755); err != nil {
		t.Fatal(err)
	}
	writeClip(t, watch, "arriving.mp4", 4096, 34)

	res, err := TakeIn(watch, clips, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Taken != 1 || res.Skipped != 0 {
		t.Errorf("TakeIn custom watch = %+v, want exactly 1 taken and 0 skipped", res)
	}
}

func TestTakeIn_AliasedNestedWatchUsesClipTraversalSpelling(t *testing.T) {
	realRoot := t.TempDir()
	aliases := t.TempDir()
	aliasA := filepath.Join(aliases, "a")
	aliasB := filepath.Join(aliases, "b")
	if err := os.Symlink(realRoot, aliasA); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(realRoot, aliasB); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	layout, err := NewLayout(filepath.Join(aliasA, "clips"), filepath.Join(aliasB, "clips", "inbox"))
	if err != nil {
		t.Fatal(err)
	}
	writeClip(t, layout.WatchDir(), "arriving.mp4", 4096, 35)

	res, err := TakeIn(layout.WatchDir(), layout.ClipDir(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Taken != 1 || res.Skipped != 0 {
		t.Errorf("TakeIn aliased custom watch = %+v, want exactly 1 taken and 0 skipped", res)
	}
}

func TestTakeIn_NeverDeletesWhenSourceAndDestinationAreTheSameFile(t *testing.T) {
	clips := filepath.Join(t.TempDir(), "clips")
	candidate := writeClip(t, t.TempDir(), "candidate.mp4", 4096, 36)
	id, err := ClipID(candidate)
	if err != nil {
		t.Fatal(err)
	}
	filed, err := ClipPath(clips, id, ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(candidate, filed); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "shard-alias")
	if err := os.Symlink(filepath.Dir(filed), alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	res, err := TakeIn(alias, clips, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res != (IntakeResult{}) {
		t.Errorf("same-file alias intake = %+v, want no mutation", res)
	}
	if _, err := os.Stat(filed); err != nil {
		t.Fatalf("same-file duplicate cleanup removed the live catalog clip: %v", err)
	}
}
