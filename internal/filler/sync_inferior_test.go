package filler_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

type inferiorCopies map[string]bool

func (c inferiorCopies) InferiorCopyHashes(context.Context) (map[string]bool, error) { return c, nil }

// A copy that lost to a better copy of its ad (#2033) stays out of the catalog and out of intake:
// a row a failed merge left behind is pruned, and an arrival with its bytes is left untouched
// where it was dropped, never filed.
func TestSync_NeverCataloguesOrFilesAKnownInferiorCopy(t *testing.T) {
	dir := t.TempDir()
	watch := filepath.Join(dir, "incoming")
	if err := os.MkdirAll(watch, 0o755); err != nil {
		t.Fatal(err)
	}
	arrival := filepath.Join(watch, "Burger Barn 1988.mp4")
	if err := os.WriteFile(arrival, []byte("a re-upload of an ad the catalog holds in a better copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	arrivalHash, err := filler.ClipID(arrival)
	if err != nil {
		t.Fatal(err)
	}
	const leftover = "leftover-inferior-row"
	source := &fakeSource{clips: []filler.RawClip{
		raw("survivor", "Burger Barn", filler.Commercial, 30_000, 0),
		raw(leftover, "Burger Barn (copy)", filler.Commercial, 30_000, 0),
	}}
	st := newMemStore()
	st.clips[leftover] = filler.StoreClip{Clip: filler.Clip{Hash: leftover, Path: "le/ft/" + leftover + ".mp4"}}
	syncer := filler.NewSyncer(source, st, testLayout(dir, watch), time.Now, discardLog()).
		WithInferiorCopies(inferiorCopies{leftover: true, arrivalHash: true})

	res, err := syncer.Sync(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.clips[leftover]; ok || res.Inferior != 1 {
		t.Fatalf("known inferior copy catalogued: inferior=%d, clips=%v", res.Inferior, st.clips)
	}
	if _, ok := st.clips["survivor"]; !ok {
		t.Fatal("the surviving copy was not catalogued")
	}
	if _, err := os.Stat(arrival); err != nil {
		t.Fatalf("a known inferior arrival was moved or removed from where it was dropped: %v", err)
	}
	if filed, _ := filler.ClipPath(dir, arrivalHash, ".mp4"); fileExists(filed) {
		t.Fatal("a known inferior arrival was filed into the clip folder")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
