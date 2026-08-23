package reconcile

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/requester"
	"github.com/loomarr/loomarr/internal/store"
)

// fakeQueue returns scripted queue items regardless of the titles passed — enough to drive the
// poller's promotion + progress-persist logic.
type fakeQueue struct {
	items []requester.QueueItem
	calls int
}

func (f *fakeQueue) QueueStatus(_ context.Context, _ []provision.Title) ([]requester.QueueItem, error) {
	f.calls++
	return f.items, nil
}

func newQueueStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/q.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// A requested title that shows up in the arr queue is promoted to downloading (Grabbed) and its
// progress is persisted on the record.
func TestQueuePoll_PromotesAndPersistsProgress(t *testing.T) {
	st := newQueueStore(t)
	key := provision.Key("movie:tmdb:603")
	put(t, st, key, provision.Requested, 603, now.Add(48*time.Hour))

	fq := &fakeQueue{items: []requester.QueueItem{
		{Key: key, Grabbed: true, Progress: 0.5, ETAText: "00:05:00", Status: "downloading"},
	}}
	emit := &captureEmitter{}
	p := NewQueuePoll(st, fq, emit, 12*time.Hour, func() time.Time { return now }, slog.New(slog.DiscardHandler))

	n, err := p.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("active = %d, want 1", n)
	}
	got, _ := st.GetTitle(context.Background(), key)
	if got.State != provision.Downloading {
		t.Errorf("state = %s, want downloading (Grabbed)", got.State)
	}
	if got.Progress != 0.5 || got.ETAText != "00:05:00" || got.DownloadStatus != "downloading" {
		t.Errorf("progress not persisted: %+v", got)
	}
	// The Grabbed transition reset the deadline to now + downloadingTTL (12h).
	if !got.Deadline.Equal(now.Add(12 * time.Hour)) {
		t.Errorf("deadline = %v, want now+12h", got.Deadline)
	}
}

// A title NOT in the queue is left untouched (still requested, no progress).
func TestQueuePoll_IgnoresUngrabbed(t *testing.T) {
	st := newQueueStore(t)
	key := provision.Key("movie:tmdb:603")
	put(t, st, key, provision.Requested, 603, now.Add(48*time.Hour))

	fq := &fakeQueue{items: []requester.QueueItem{{Key: key, Grabbed: false}}}
	p := NewQueuePoll(st, fq, &captureEmitter{}, 12*time.Hour, func() time.Time { return now }, slog.New(slog.DiscardHandler))

	n, err := p.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("active = %d, want 0", n)
	}
	got, _ := st.GetTitle(context.Background(), key)
	if got.State != provision.Requested || got.Progress != 0 {
		t.Errorf("ungrabbed title should be untouched: %+v", got)
	}
}

// An already-downloading title keeps downloading (no needless re-Grab) but its progress updates.
func TestQueuePoll_UpdatesProgressWhenAlreadyDownloading(t *testing.T) {
	st := newQueueStore(t)
	key := provision.Key("movie:tmdb:603")
	put(t, st, key, provision.Downloading, 603, now.Add(12*time.Hour))

	fq := &fakeQueue{items: []requester.QueueItem{
		{Key: key, Grabbed: true, Progress: 0.9, Status: "downloading"},
	}}
	p := NewQueuePoll(st, fq, &captureEmitter{}, 12*time.Hour, func() time.Time { return now }, slog.New(slog.DiscardHandler))

	if _, err := p.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetTitle(context.Background(), key)
	if got.State != provision.Downloading || got.Progress != 0.9 {
		t.Errorf("progress should update on a downloading title: %+v", got)
	}
}

// With nothing in-flight the poll short-circuits and never calls the arr.
func TestQueuePoll_NoInflightNoQueueCall(t *testing.T) {
	st := newQueueStore(t)
	fq := &fakeQueue{}
	p := NewQueuePoll(st, fq, &captureEmitter{}, 12*time.Hour, func() time.Time { return now }, slog.New(slog.DiscardHandler))
	if _, err := p.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fq.calls != 0 {
		t.Errorf("no in-flight titles should skip the queue call, got %d calls", fq.calls)
	}
}
