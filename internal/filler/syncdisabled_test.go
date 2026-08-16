package filler_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// countingSource records whether the syncer reached the filesystem at all.
type countingSource struct {
	ensured int
	listed  int
	clips   []filler.RawClip
}

func (c *countingSource) EnsureLocalSource(context.Context, string) error { c.ensured++; return nil }
func (c *countingSource) ListLocalClips(context.Context) ([]filler.RawClip, error) {
	c.listed++
	return c.clips, nil
}

// recordingStore fails loudly if a disabled sync writes anything.
type recordingStore struct{ upserts, prunes int }

func (r *recordingStore) UpsertClip(context.Context, filler.StoreClip) error { r.upserts++; return nil }
func (r *recordingStore) GetClip(context.Context, string) (filler.StoreClip, bool, error) {
	return filler.StoreClip{}, false, nil
}
func (r *recordingStore) DeleteClipsNotIn(context.Context, []string) (int, error) {
	r.prunes++
	return 0, nil
}

// The Sources tab's switch says Loomarr "stops scanning" a source that is off. That is a
// behaviour claim, so it is asserted as behaviour: no scan, no writes, and a distinct error
// the caller can turn into an answer naming the switch.
func TestSync_DisabledSourceDoesNoWork(t *testing.T) {
	src := &countingSource{clips: []filler.RawClip{{Path: "a.mp4", Name: "a.mp4", DurationMs: 30_000}}}
	st := &recordingStore{}
	syncer := filler.NewSyncer(src, st, testLayout("/data/filler"), nil, discardLogger()).
		WithEnabled(func() bool { return false })

	res, err := syncer.Sync(context.Background())

	if !errors.Is(err, filler.ErrSourceDisabled) {
		t.Fatalf("err = %v, want ErrSourceDisabled", err)
	}
	if res != (filler.SyncResult{}) {
		t.Errorf("result = %+v, want zero — a disabled sync did work", res)
	}
	// ⚠ The important half. An error return with the scan already done would still have
	// re-registered the folder with Tunarr and read the disk, which is exactly what the
	// switch promises not to do.
	if src.listed != 0 || src.ensured != 0 {
		t.Errorf("scanned anyway: ensured=%d listed=%d", src.ensured, src.listed)
	}
	if st.upserts != 0 || st.prunes != 0 {
		t.Errorf("wrote anyway: upserts=%d prunes=%d", st.upserts, st.prunes)
	}
}

// Switching it back on resumes: the switch withdraws a source from future work, it does not
// tear anything down.
func TestSync_EnabledSourceScansNormally(t *testing.T) {
	src := &countingSource{clips: []filler.RawClip{{Path: "a.mp4", Name: "a.mp4", DurationMs: 30_000}}}
	on := false
	syncer := filler.NewSyncer(src, &recordingStore{}, testLayout("/data/filler"), nil, discardLogger()).
		WithEnabled(func() bool { return on })

	if _, err := syncer.Sync(context.Background()); !errors.Is(err, filler.ErrSourceDisabled) {
		t.Fatalf("expected the disabled error first, got %v", err)
	}

	// The switch is read PER SYNC, not captured — the setting hot-applies, so the next
	// scheduled pass must see the change without a restart.
	on = true
	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("sync after re-enabling: %v", err)
	}
	if src.listed != 1 {
		t.Errorf("listed %d times, want 1 — re-enabling did not resume the scan", src.listed)
	}
}

// A syncer built without the gate behaves exactly as before. Every existing construction
// relies on this, so it is pinned rather than assumed.
func TestSync_NoGateMeansEnabled(t *testing.T) {
	src := &countingSource{}
	syncer := filler.NewSyncer(src, &recordingStore{}, testLayout("/data/filler"), nil, discardLogger())

	if _, err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("ungated sync returned %v", err)
	}
	if src.listed != 1 {
		t.Errorf("listed %d times, want 1", src.listed)
	}
}
