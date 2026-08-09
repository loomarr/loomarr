package images

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A store stub recording what the job wrote back.
type adoptStoreStub struct {
	pending []PendingArtwork
	saved   map[string][2]string
	saves   int
}

func (a *adoptStoreStub) ListPendingArtwork(_ context.Context, _ int) ([]PendingArtwork, error) {
	return a.pending, nil
}

func (a *adoptStoreStub) SetAdoptedArtwork(_ context.Context, ownerID, still, anim string, _ time.Time) error {
	if a.saved == nil {
		a.saved = map[string][2]string{}
	}
	a.saved[ownerID] = [2]string{still, anim}
	a.saves++
	return nil
}

// solidPNG is a real, decodable PNG of the given size — the job hands bytes to a real Ingest,
// which probes them, so a placeholder file would fail for the wrong reason. Distinct sizes give
// distinct content hashes, which is what the "hashes must differ" assertion rests on.
func solidPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	return pngBytes(t, image.NewRGBA(image.Rect(0, 0, w, h)))
}

func newAdoptFixture(t *testing.T) (*Service, *adoptStoreStub) {
	t.Helper()
	svc, _ := newTestService(t)
	return svc, &adoptStoreStub{}
}

// The ordinary case: both assets present, both adopted, and the two hashes DIFFER because the
// files differ. ⚠ Asserting they differ is the point — a job that returned one hash for both, or
// keyed on the owner rather than the bytes, would look correct in every other assertion.
func TestAdoptJob_AdoptsBothAssetsWithDistinctHashes(t *testing.T) {
	svc, st := newAdoptFixture(t)
	dir := t.TempDir()
	still, anim := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	if err := os.WriteFile(still, solidPNG(t, 8, 8), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(anim, solidPNG(t, 16, 16), 0o600); err != nil {
		t.Fatal(err)
	}
	st.pending = []PendingArtwork{{OwnerID: "clip-1", StillPath: still, AnimPath: anim}}

	res, err := NewAdoptJob(svc, st, nil, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Adopted != 1 || res.Failed != 0 {
		t.Fatalf("result = %+v, want 1 adopted / 0 failed", res)
	}
	got := st.saved["clip-1"]
	if got[0] == "" || got[1] == "" {
		t.Fatalf("hashes = %v, want both set", got)
	}
	if got[0] == got[1] {
		t.Errorf("still and hover share hash %q; identity must follow the BYTES, and these files differ", got[0])
	}
}

// ⚠ A missing FILE is not a failure. `thumbnail`/`preview` record what was rendered once, and the
// artwork cache under FILLER_DIR is explicitly regenerable — an operator who cleared it has rows
// pointing at files that are legitimately gone. Counting that as an error would make an ordinary
// state look broken and bury real failures under it.
func TestAdoptJob_MissingFileIsCountedSeparatelyFromFailure(t *testing.T) {
	svc, st := newAdoptFixture(t)
	st.pending = []PendingArtwork{{OwnerID: "clip-1", StillPath: filepath.Join(t.TempDir(), "gone.png")}}

	res, err := NewAdoptJob(svc, st, nil, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 0 {
		t.Errorf("failed = %d, want 0 — a cleared artwork cache is regenerable, not an error", res.Failed)
	}
	if res.Missing != 1 {
		t.Errorf("missing = %d, want 1", res.Missing)
	}
	if st.saves != 0 {
		t.Error("wrote a row with nothing adopted; there is no identity to record")
	}
}

// ⚠ A PARTIAL success is still recorded. The still and the animation are rendered by passes that
// fail independently, so "still adopted, animation never existed" is the ordinary state for a clip
// whose animation render failed. Holding the write until BOTH succeed would re-list that owner
// every run, forever.
func TestAdoptJob_RecordsAPartialAdoption(t *testing.T) {
	svc, st := newAdoptFixture(t)
	dir := t.TempDir()
	still := filepath.Join(dir, "a.png")
	if err := os.WriteFile(still, solidPNG(t, 8, 8), 0o600); err != nil {
		t.Fatal(err)
	}
	// No animation at all — the clip's hover render never produced a file.
	st.pending = []PendingArtwork{{OwnerID: "clip-1", StillPath: still}}

	res, err := NewAdoptJob(svc, st, nil, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Adopted != 1 {
		t.Fatalf("adopted = %d, want 1", res.Adopted)
	}
	got := st.saved["clip-1"]
	if got[0] == "" {
		t.Error("still hash empty; it was adopted")
	}
	if got[1] != "" {
		t.Errorf("hover hash = %q, want empty — no animation was rendered", got[1])
	}
}

// Adoption must be IDEMPOTENT on content: the same bytes adopt to the same hash, so a re-run after
// a crash mid-batch cannot produce a second copy of the same image.
func TestAdoptJob_SameBytesAdoptToTheSameHash(t *testing.T) {
	svc, st := newAdoptFixture(t)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, solidPNG(t, 8, 8), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	st.pending = []PendingArtwork{
		{OwnerID: "clip-1", StillPath: a},
		{OwnerID: "clip-2", StillPath: b},
	}

	if _, err := NewAdoptJob(svc, st, nil, nil).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if one, two := st.saved["clip-1"][0], st.saved["clip-2"][0]; one != two {
		t.Errorf("identical bytes adopted to %q and %q; content addressing means one image", one, two)
	}
}
