package upkeep

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/quarantine"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

var t0 = time.Unix(1_900_000_000, 0).UTC()

// hashOf is a clip hash of the shape intake produces (64 hex characters), distinct per name.
func hashOf(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])
}

// fakeStore keeps rows and clips in memory. RetireRejectedClip applies the same qualification as
// the SQL store, which the store conformance suite pins on both backends.
type fakeStore struct {
	rows    map[string]pipelinestate.ClipPipeline
	clips   map[string]clipstore.Clip
	pinned  map[string]bool
	retires int
}

func (f *fakeStore) ListClipPipelines(_ context.Context, _ pipelinestate.PipelineFilter) ([]pipelinestate.ClipPipeline, error) {
	var out []pipelinestate.ClipPipeline
	for _, r := range f.rows {
		if r.Disposition == pipelinestate.DispositionRejected {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStore) RetireRejectedClip(_ context.Context, hash string, rejectedBy, at time.Time) (bool, error) {
	r := f.rows[hash]
	if r.Disposition != pipelinestate.DispositionRejected || r.AcquisitionID == "" || !r.FilesDeletedAt.IsZero() ||
		r.UpdatedAt.After(rejectedBy) || f.pinned[hash] {
		return false, nil
	}
	f.retires++
	r.FilesDeletedAt = at
	f.rows[hash] = r
	return true, nil
}

func (f *fakeStore) GetClip(_ context.Context, hash string) (clipstore.Clip, bool, error) {
	c, ok := f.clips[hash]
	return c, ok, nil
}

func (f *fakeStore) ListClipPaths(context.Context) ([]string, error) {
	var out []string
	for _, c := range f.clips {
		out = append(out, c.Path)
	}
	return out, nil
}

type fixture struct {
	dir   string
	store *fakeStore
}

func newFixture(t *testing.T) *fixture {
	return &fixture{dir: t.TempDir(), store: &fakeStore{
		rows: map[string]pipelinestate.ClipPipeline{}, clips: map[string]clipstore.Clip{}, pinned: map[string]bool{},
	}}
}

func (f *fixture) write(t *testing.T, rel string) string {
	t.Helper()
	full := filepath.Join(f.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(rel), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

// reject adds a rejected clip with a playback file and a sidecar naming master, and returns the
// playback file's full path.
func (f *fixture) reject(t *testing.T, hash, acquisition, master string, rejectedAt time.Time) string {
	t.Helper()
	rel := cliplibrary.ClipRelPath(hash, ".mp4")
	full := f.write(t, rel)
	var tags asset.SidecarTags
	if master != "" {
		f.write(t, master)
		tags.MediaAssets = &asset.Manifest{Version: 1, SourceMaster: asset.Identity{
			Role: asset.RoleSourceMaster, SHA256: hashOf(master), Bytes: int64(len(master)), ClipHash: hash, Path: master,
		}}
	}
	if err := asset.WriteSidecarTags(full, tags, false); err != nil {
		t.Fatal(err)
	}
	c := clipstore.Clip{}
	c.Hash, c.Path, c.Held = hash, rel, true
	f.store.clips[hash] = c
	f.store.rows[hash] = pipelinestate.ClipPipeline{ClipHash: hash, AcquisitionID: acquisition,
		Disposition: pipelinestate.DispositionRejected, RejectReason: pipelinestate.ReasonUnplayable,
		UpdatedAt: rejectedAt}
	return full
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

type cleanerFunc func(context.Context) (acquisition.StorageCleanupResult, error)

func (c cleanerFunc) Clean(ctx context.Context) (acquisition.StorageCleanupResult, error) {
	return c(ctx)
}

func TestRejectsLetGoAfterAWeekAndNothingElseDoes(t *testing.T) {
	f := newFixture(t)
	masterOf := func(name string) string {
		return filepath.ToSlash(filepath.Join(split.MediaAssetRootName, "masters", "aa", "bb", name+".mkv"))
	}
	weekAgo := t0.Add(-RejectRest)
	old := f.reject(t, hashOf("old"), "run-1", masterOf("old"), weekAgo.Add(-time.Hour))
	sharing := f.reject(t, hashOf("sharing"), "run-1", masterOf("shared"), weekAgo.Add(-time.Hour))
	f.reject(t, hashOf("recent sharer"), "run-1", masterOf("shared"), t0) // a recent reject sharing that master
	recent := f.reject(t, hashOf("recent"), "run-1", "", weekAgo.Add(time.Hour))
	byHand := f.reject(t, hashOf("by hand"), "", "", weekAgo.Add(-time.Hour))
	f.store.pinned[hashOf("pinned")] = true
	pinned := f.reject(t, hashOf("pinned"), "run-1", "", weekAgo.Add(-time.Hour))

	cleaned := 0
	job := New(f.store, quarantine.New(f.dir, func() time.Time { return t0 }),
		cleanerFunc(func(context.Context) (acquisition.StorageCleanupResult, error) {
			cleaned++
			return acquisition.StorageCleanupResult{RemovedItems: 2, RemovedBytes: 100}, nil
		}), f.dir, func() time.Time { return t0 }, nil)
	res, err := job.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.LetGo != 2 || res.Quarantined != 2 || res.Leftovers != 2 || cleaned != 1 {
		t.Fatalf("Run = %+v (cleaner ran %d times), want 2 let go, 2 quarantined, 2 leftovers", res, cleaned)
	}
	for _, gone := range []string{old, tools.SidecarPathFor(old), filepath.Join(f.dir, masterOf("old")), sharing} {
		if exists(gone) {
			t.Errorf("%s is still in place, want it in quarantine", gone)
		}
	}
	for _, kept := range []string{filepath.Join(f.dir, masterOf("shared")), recent, byHand, pinned} {
		if !exists(kept) {
			t.Errorf("%s was moved, want it kept", kept)
		}
	}
	held := filepath.Join(f.dir, quarantine.DirName, "1900000000", filepath.FromSlash(cliplibrary.ClipRelPath(hashOf("old"), ".mp4")))
	if !exists(held) {
		t.Fatalf("the retired clip's file is not in quarantine at %s", held)
	}
}

// failingBin injects a failure at the destructive step after the retire has committed.
type failingBin struct{ quarantine.Bin }

func (failingBin) Put(files []string) map[string]error {
	failed := map[string]error{}
	for _, f := range files {
		failed[f] = errors.New("injected: disk went away")
	}
	return failed
}

func TestACrashAfterTheRetireIsFinishedByTheNextPass(t *testing.T) {
	f := newFixture(t)
	master := filepath.ToSlash(filepath.Join(split.MediaAssetRootName, "masters", "aa", "bb", "m.mkv"))
	old := f.reject(t, hashOf("old"), "run-1", master, t0.Add(-RejectRest-time.Hour))
	now := func() time.Time { return t0 }

	first := New(f.store, failingBin{quarantine.New(f.dir, now)}, nil, f.dir, now, nil)
	if _, err := first.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.store.retires != 1 || !exists(old) || !exists(tools.SidecarPathFor(old)) {
		t.Fatalf("after the failed move: retires=%d file=%v sidecar=%v; want retired once, files kept",
			f.store.retires, exists(old), exists(tools.SidecarPathFor(old)))
	}

	// A new process: nothing remembered, the row already says its files are deleted.
	second := New(f.store, quarantine.New(f.dir, now), nil, f.dir, now, nil)
	res, err := second.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.store.retires != 1 || res.LetGo != 0 || res.Quarantined != 1 {
		t.Fatalf("second pass = %+v, retires=%d; want the files moved without retiring again", res, f.store.retires)
	}
	if exists(old) || exists(tools.SidecarPathFor(old)) || exists(filepath.Join(f.dir, master)) {
		t.Fatal("the second pass left a file of the retired clip in place")
	}
}
