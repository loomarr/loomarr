package filler_test

import (
	"context"
	"io"
	"log/slog"
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
type memStore struct{ clips map[string]filler.StoreClip }

func newMemStore() *memStore { return &memStore{clips: map[string]filler.StoreClip{}} }

func (m *memStore) UpsertClip(_ context.Context, c filler.StoreClip) error {
	m.clips[c.TunarrProgramID] = c
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

func raw(id, name string, kind filler.Kind, dur int64, era int) filler.RawClip {
	return filler.RawClip{TunarrProgramID: id, Name: name, Kind: kind, DurationMs: dur, Era: era}
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
