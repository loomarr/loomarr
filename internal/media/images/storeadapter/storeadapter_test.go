package storeadapter_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/media/images/storeadapter"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// Through the real store: a remote image still waiting for its bytes reads back with none, its
// failure record survives an ordinary write, and the download that lands clears it. The domain
// fakes hold the zero time, so only a store-backed read can show the scan decodes the sentinel.
func TestPendingImageThroughTheRealStore(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "images.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a := storeadapter.New(st)
	at := time.Unix(1_700_000_000, 0)
	pending := images.Image{
		Hash: strings.Repeat("c4", 32), Origin: images.OriginRemote, Role: images.RolePoster, MIME: "image/jpeg",
		Visibility: images.VisibilityMember, SourceURL: "https://image.example.test/pending.jpg",
		CreatedAt: at, UpdatedAt: at, LastUsedAt: at, FetchQueuedAt: at,
	}
	if err := a.PutImage(ctx, pending); err != nil {
		t.Fatal(err)
	}
	got, err := a.GetImage(ctx, pending.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasBytes() || !got.OriginFetchedAt.IsZero() {
		t.Fatalf("a pending row reads back as fetched at %v", got.OriginFetchedAt)
	}
	if !got.FetchQueuedAt.Equal(at) {
		t.Fatalf("queued at %v, want %v", got.FetchQueuedAt, at)
	}

	failedAt := at.Add(time.Minute)
	if err := a.RecordFetchFailure(ctx, pending.Hash, failedAt, failedAt.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, _ = a.GetImage(ctx, pending.Hash)
	got.LastUsedAt = at.Add(time.Hour)
	if err := a.PutImage(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, _ = a.GetImage(ctx, pending.Hash)
	if got.FetchFailures != 1 || !got.FetchFailedAt.Equal(failedAt) || !got.FetchRetryAt.Equal(failedAt.Add(5*time.Minute)) {
		t.Fatalf("an ordinary write lost the failure record: %d, failed %v, retry %v", got.FetchFailures, got.FetchFailedAt, got.FetchRetryAt)
	}

	got.OriginFetchedAt = at.Add(2 * time.Hour)
	if err := a.PutImage(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, _ = a.GetImage(ctx, pending.Hash)
	if !got.HasBytes() || got.FetchFailures != 0 || !got.FetchFailedAt.IsZero() || !got.FetchRetryAt.IsZero() {
		t.Fatalf("a landed download = bytes %v, %d failures, failed %v, retry %v", got.HasBytes(), got.FetchFailures,
			got.FetchFailedAt, got.FetchRetryAt)
	}
}
