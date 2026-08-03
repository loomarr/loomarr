package filler_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
)

// fakeSource is a Tunarr local filler source returning a fixed set of raw clips.
// ensures counts EnsureLocalSource calls so the idempotent-setup path is covered.
type fakeSource struct {
	clips   []filler.RawClip
	ensures int
}

func (f *fakeSource) EnsureLocalSource(_ context.Context, _ string) error {
	f.ensures++
	return nil
}
func (f *fakeSource) ListLocalClips(_ context.Context) ([]filler.RawClip, error) {
	return f.clips, nil
}

// memStore is an in-memory filler.Store for sync tests.
//
// ⚠ Keyed on the clip's ID (its content hash), exactly as the real store is since V38c. It used
// to key on `c.Path`, which quietly made every sync test agree with a Syncer that also keyed on
// the path — so the re-key left `Sync` reading the wrong field and nothing went red. A double
// that models identity differently from the real thing tests the double.
type memStore struct{ clips map[string]filler.StoreClip }

func newMemStore() *memStore { return &memStore{clips: map[string]filler.StoreClip{}} }

func (m *memStore) UpsertClip(_ context.Context, c filler.StoreClip) error {
	m.clips[c.ID()] = c
	return nil
}
func (m *memStore) GetClip(_ context.Context, id string) (filler.StoreClip, bool, error) {
	c, ok := m.clips[id]
	return c, ok, nil
}
func (m *memStore) DeleteClipsNotIn(_ context.Context, keep []string) (int, error) {
	keepSet := map[string]bool{}
	for _, id := range keep {
		keepSet[id] = true
	}
	n := 0
	for id := range m.clips {
		if !keepSet[id] {
			delete(m.clips, id)
			n++
		}
	}
	return n, nil
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// raw builds a scanned clip. ⚠ ID and Path are set SEPARATELY — id is the content hash, path is
// where the file sits. The fixtures below pass the same string for both where the distinction
// does not matter; the tests that turn on it (see the two-folder case) pass different ones.
func raw(id, name string, kind filler.Kind, dur int64, era int) filler.RawClip {
	return filler.RawClip{ID: id, Path: id, Name: name, Kind: kind, DurationMs: dur, Era: era}
}

func newSyncer(source *fakeSource, st *memStore) *filler.Syncer {
	return filler.NewSyncer(source, st, "/drop",
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, discardLog())
}

func TestSync_AddsClips_DurationFromServer(t *testing.T) {
	source := &fakeSource{clips: []filler.RawClip{
		raw("c1", "Frosted Flakes 1992", filler.Commercial, 30000, 1992),
		raw("b1", "Bumper", filler.Bumper, 5000, 0),
	}}
	st := newMemStore()
	res, err := newSyncer(source, st).Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 || res.Total != 2 {
		t.Fatalf("sync result = %+v, want added 2 / total 2", res)
	}
	c1 := st.clips["c1"]
	if c1.DurationMs != 30000 {
		t.Errorf("duration not from server: %d", c1.DurationMs)
	}
	if c1.Era != 1992 {
		t.Errorf("initial era from filename lost: %d", c1.Era)
	}
}

// THE KEY PROPERTY (§10): a re-sync PRESERVES loomarr-owned tags. A clip tagged
// (by AI or by hand) keeps its era/audience/category when the media server
// re-lists it — the server only owns id/name/duration.
func TestSync_PreservesTagsOnResync(t *testing.T) {
	source := &fakeSource{clips: []filler.RawClip{raw("c1", "clip", filler.Commercial, 30000, 0)}}
	st := newMemStore()
	s := newSyncer(source, st)

	// First sync creates the clip untagged.
	_, _ = s.Sync(context.Background())

	// A human/AI tags it.
	tagged := st.clips["c1"]
	tagged.Era = 1994
	tagged.Audience = filler.Kids
	tagged.Category = "cereal"
	tagged.AITagged = true
	st.clips["c1"] = tagged

	// The media server re-lists the same clip (maybe with a corrected name).
	source.clips = []filler.RawClip{raw("c1", "clip (renamed)", filler.Commercial, 30000, 0)}
	res, err := s.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	after := st.clips["c1"]
	// Tags survive.
	if after.Era != 1994 || after.Audience != filler.Kids || after.Category != "cereal" || !after.AITagged {
		t.Fatalf("re-sync clobbered loomarr-owned tags: %+v", after.Clip)
	}
	// Server-owned name updated.
	if after.Name != "clip (renamed)" {
		t.Errorf("server name not updated: %q", after.Name)
	}
	if res.Updated != 1 {
		t.Errorf("renamed clip should count as updated, got %+v", res)
	}
}

// Idempotent: a no-change re-sync makes no updates.
func TestSync_IdempotentNoChange(t *testing.T) {
	source := &fakeSource{clips: []filler.RawClip{raw("c1", "clip", filler.Commercial, 30000, 1992)}}
	st := newMemStore()
	s := newSyncer(source, st)
	_, _ = s.Sync(context.Background())
	res, _ := s.Sync(context.Background())
	if res.Added != 0 || res.Updated != 0 {
		t.Errorf("no-change re-sync should be a no-op, got %+v", res)
	}
}

// ⚠ **Artwork that appears on a LATER pass must still reach the database.**
//
// `serverFieldsUnchanged` gates the write, so a scan-owned field it does not compare can never be
// persisted for an existing clip: the merge assigns it and the sync then skips the row as
// unchanged. This is not hypothetical — it is exactly what happened live in V39. All 13 clips had
// their previews rendered to disk, `merged.Preview = rc.Preview` ran, and every `preview` column
// stayed empty, because Name/DurationMs/Kind were identical.
//
// That path is the NORMAL one on upgrade: the whole existing catalog gains previews on a re-scan
// of clips whose rows already exist. `Thumbnail` carried the same latent bug and had simply never
// been exercised, because a still was always generated on the pass that first inserted the clip.
func TestSync_LateArtworkIsPersistedOnAReScan(t *testing.T) {
	// First pass: a clip whose artwork has not been rendered yet (the pre-V39 catalog).
	bare := raw("c1", "clip", filler.Commercial, 30000, 1992)
	source := &fakeSource{clips: []filler.RawClip{bare}}
	st := newMemStore()
	s := newSyncer(source, st)
	if _, err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Second pass: the artwork now exists, and NOTHING else about the clip has changed.
	withArt := bare
	withArt.Thumbnail = "c1.jpg"
	withArt.Preview = "c1.webp"
	source.clips = []filler.RawClip{withArt}

	res, err := s.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Errorf("updated = %d, want 1 — newly rendered artwork is a REAL change, and skipping "+
			"the write means it can never be stored", res.Updated)
	}

	got, ok, err := st.GetClip(context.Background(), "c1")
	if err != nil || !ok {
		t.Fatalf("get c1: ok=%v err=%v", ok, err)
	}
	if got.Preview != "c1.webp" {
		t.Errorf("preview = %q, want it persisted — every hover on this install renders nothing "+
			"until this column is written", got.Preview)
	}
	if got.Thumbnail != "c1.jpg" {
		t.Errorf("thumbnail = %q, want it persisted", got.Thumbnail)
	}
}

// Prune: a clip removed from the media server's library is removed from the catalog.
func TestSync_PrunesRemovedClips(t *testing.T) {
	source := &fakeSource{clips: []filler.RawClip{
		raw("c1", "keep", filler.Commercial, 30000, 1992),
		raw("c2", "goes away", filler.Commercial, 30000, 1993),
	}}
	st := newMemStore()
	s := newSyncer(source, st)
	_, _ = s.Sync(context.Background())

	// c2 disappears from the media server.
	source.clips = []filler.RawClip{raw("c1", "keep", filler.Commercial, 30000, 1992)}
	res, _ := s.Sync(context.Background())
	if res.Pruned != 1 {
		t.Errorf("pruned = %d, want 1", res.Pruned)
	}
	if _, ok := st.clips["c2"]; ok {
		t.Error("removed clip still in catalog")
	}
	if _, ok := st.clips["c1"]; !ok {
		t.Error("kept clip wrongly pruned")
	}
}

// --- V38: the lifecycle fork ---

// ⚠ THE mechanism the whole review queue depends on. Ingest downloads into the same folder the
// scan watches, so at catalogue time a fetched clip and a hand-copied one are both just files.
// The `.info.json` sidecar `clipfetch` writes is what tells them apart.
//
// If this fork were wrong in the "no sidecar ⇒ hold" direction, an operator's own files would sit
// invisible until approved. Wrong the other way, every download would go straight to air.
func TestSync_HoldsDownloadedClipsAndFilesHandCopiedOnes(t *testing.T) {
	dir := t.TempDir()
	// A downloaded clip. ⚠ The signal is the `fetchedBy` FIELD, not the sidecar's existence
	// (V38c): Loomarr writes sidecars for hand-dropped clips too now, so a bare `{"title":"x"}`
	// no longer means "downloaded" — it means "tagged". This fixture failed on exactly that,
	// which is the change working.
	if err := os.WriteFile(filepath.Join(dir, "fetched.info.json"),
		[]byte(`{"title":"x","loomarr":{"fetchedBy":"loomarr"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	source := &fakeSource{clips: []filler.RawClip{
		raw("fetched.mp4", "Fetched ad", filler.Commercial, 30000, 0),
		raw("copied.mp4", "Copied ad", filler.Commercial, 30000, 0),
	}}
	st := newMemStore()
	sync := filler.NewSyncer(source, st, dir,
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, discardLog())

	if _, err := sync.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	fetched, _, _ := st.GetClip(context.Background(), "fetched.mp4")
	if !fetched.Held {
		t.Error("a DOWNLOADED clip (sidecar present) was filed on sight — it must wait for review")
	}
	copied, _, _ := st.GetClip(context.Background(), "copied.mp4")
	if copied.Held {
		t.Error("a HAND-COPIED clip (no sidecar) was held — a file the operator placed themselves " +
			"would sit invisible until approved")
	}
}

// ⚠ A re-scan must never re-hold a clip a human already filed. The scan sees the same sidecar on
// every pass, so without the preserve in the merge, filing a clip would last exactly until the
// next sync — and the operator would find it back in the queue with no explanation.
func TestSync_ReScanDoesNotReHoldAFiledClip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fetched.info.json"),
		[]byte(`{"title":"x","loomarr":{"fetchedBy":"loomarr"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	source := &fakeSource{clips: []filler.RawClip{
		raw("fetched.mp4", "Fetched ad", filler.Commercial, 30000, 0),
	}}
	st := newMemStore()
	sync := filler.NewSyncer(source, st, dir,
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, discardLog())

	if _, err := sync.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The operator files it from Incoming.
	c, _, _ := st.GetClip(context.Background(), "fetched.mp4")
	c.Held = false
	if err := st.UpsertClip(context.Background(), c); err != nil {
		t.Fatal(err)
	}

	// ⚠ The re-scan must actually WRITE, or this proves nothing. `serverFieldsUnchanged` skips
	// an unchanged clip before any write, so a naive second Sync() passes whatever the merge
	// does with `Held` — a sabotage that recomputed it from the sidecar still went green. The
	// duration change below is what forces the update path this test exists to cover.
	source.clips[0].DurationMs = 31000
	if _, err := sync.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _, _ := st.GetClip(context.Background(), "fetched.mp4")
	if after.DurationMs != 31000 {
		t.Fatal("the re-scan did not write — this test would pass vacuously")
	}
	if after.Held {
		t.Error("a re-scan RE-HELD a clip the operator had filed — the sidecar is still there on " +
			"every pass, so `Held` must be preserved for a clip we already know")
	}
}

// Two watched folders each holding `ads/coke.mp4` — different adverts that happen to share a
// relative path. THE case V38c moved identity off the path for (§10).
//
// ⚠ This is the test the re-key was missing. `Sync` kept keying on `rc.Path` after identity
// became `rc.ID`, and nothing caught it because every fixture set the two to the same string and
// the in-memory double keyed on the path as well. With the path as identity the second clip
// overwrites the first, `keep` carries one entry where two are live, and the prune then deletes a
// clip that is sitting right there on disk.
func TestSync_TwoFoldersSharingAPathAreTwoClips(t *testing.T) {
	source := &fakeSource{clips: []filler.RawClip{
		{ID: "hash-coke-1985", Path: "ads/coke.mp4", Name: "Coke 1985",
			Kind: filler.Commercial, DurationMs: 30000},
		{ID: "hash-coke-1992", Path: "ads/coke.mp4", Name: "Coke 1992",
			Kind: filler.Commercial, DurationMs: 15000},
	}}
	st := newMemStore()

	res, err := newSyncer(source, st).Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 {
		t.Errorf("Added = %d, want 2 — one clip overwrote the other, so identity is still the path",
			res.Added)
	}
	if len(st.clips) != 2 {
		t.Fatalf("store holds %d clips, want 2", len(st.clips))
	}
	// And neither was pruned: `keep` must carry both identities, not one path twice.
	if _, ok, _ := st.GetClip(context.Background(), "hash-coke-1985"); !ok {
		t.Error("the 1985 advert was pruned — `keep` is collecting paths, so a live clip was deleted")
	}
	if _, ok, _ := st.GetClip(context.Background(), "hash-coke-1992"); !ok {
		t.Error("the 1992 advert is missing from the catalog")
	}
	// The location still travels with each row — identity moved, the path did not disappear.
	if got := st.clips["hash-coke-1985"].Path; got != "ads/coke.mp4" {
		t.Errorf("Path = %q, want the on-disk location", got)
	}
}

// --- V38c: intake runs inside the sync ---

// realScanSource wires the ACTUAL DirSource to a temp folder, so this test exercises the real
// intake → scan → catalog path rather than a double's idea of it. The property under test is an
// ORDERING one, and a fake source returning fixed clips cannot express it.
type realScanSource struct{ dir string }

func (r realScanSource) EnsureLocalSource(context.Context, string) error { return nil }
func (r realScanSource) ListLocalClips(ctx context.Context) ([]filler.RawClip, error) {
	clips, _, err := filler.ScanDir(ctx, r.dir, func(context.Context, string) (filler.Probed, error) {
		return filler.Probed{DurationMs: 30_000}, nil
	})
	return clips, err
}

// ⚠ THE ordering property. Intake runs BEFORE the listing, so a file dropped in the watch folder
// is catalogued by the SAME pass that files it. Draining afterwards would leave every arrival
// waiting a full sync interval — 15 minutes by default — which reads to an operator as "I dropped
// a file in and nothing happened".
func TestSync_FilesAndCatalogsAWatchFolderArrivalInOnePass(t *testing.T) {
	clipDir := t.TempDir()
	watchDir := filepath.Join(clipDir, filler.WatchDirName)
	if err := os.MkdirAll(watchDir, 0o750); err != nil {
		t.Fatal(err)
	}
	// A file big enough to hash, with a year in its name — the era signal that must survive the
	// rename by way of the sidecar.
	body := make([]byte, 4096)
	for i := range body {
		body[i] = byte(i % 251)
	}
	if err := os.WriteFile(filepath.Join(watchDir, "Frosted Flakes 1993.mp4"), body, 0o600); err != nil {
		t.Fatal(err)
	}

	st := newMemStore()
	sync := filler.NewSyncer(realScanSource{clipDir}, st, clipDir,
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, discardLog())

	res, err := sync.Sync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Fatalf("Added = %d, want 1 — the arrival was not catalogued by the pass that filed it", res.Added)
	}

	var got filler.StoreClip
	for _, c := range st.clips {
		got = c
	}
	// Filed under its hash, in the sharded layout, NOT at its arrival path.
	if !strings.HasSuffix(got.Path, ".mp4") || strings.Contains(got.Path, filler.WatchDirName) {
		t.Errorf("Path = %q, want a sharded hash path outside the watch folder", got.Path)
	}
	if got.ID() != got.Hash || got.Hash == "" {
		t.Errorf("identity = %q / hash = %q, want a content hash", got.ID(), got.Hash)
	}
	// ⚠ The era survived the rename. Once the file is `a3f9….mp4` the only place the year still
	// exists is the sidecar's originalName — which is exactly why intake captures it.
	if got.Era != 1993 {
		t.Errorf("Era = %d, want 1993 — the filename's era signal was lost in the rename", got.Era)
	}
	if got.Name != "Frosted Flakes 1993" {
		t.Errorf("Name = %q, want the original filename, not the hash", got.Name)
	}
	// The watch folder drained.
	left, _ := os.ReadDir(watchDir)
	for _, e := range left {
		if !e.IsDir() {
			t.Errorf("watch folder still holds %q", e.Name())
		}
	}
}

// A hand-dropped clip is FILED, not held (§10 V38c). Intake writes no `fetchedBy` for it, so the
// sync's held/filed fork must let it straight into the catalog — holding a file the operator
// placed themselves would mean it sits invisible until approved.
func TestSync_AWatchFolderDropIsFiledNotHeld(t *testing.T) {
	clipDir := t.TempDir()
	watchDir := filepath.Join(clipDir, filler.WatchDirName)
	if err := os.MkdirAll(watchDir, 0o750); err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 2048)
	for i := range body {
		body[i] = byte(i % 97)
	}
	if err := os.WriteFile(filepath.Join(watchDir, "dropped.mp4"), body, 0o600); err != nil {
		t.Fatal(err)
	}

	st := newMemStore()
	sync := filler.NewSyncer(realScanSource{clipDir}, st, clipDir,
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, discardLog())
	if _, err := sync.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, c := range st.clips {
		if c.Held {
			t.Error("a hand-dropped clip was HELD — it would sit invisible until approved, " +
				"which is the ceremony §7 warns teaches people to click through gates")
		}
	}
}
