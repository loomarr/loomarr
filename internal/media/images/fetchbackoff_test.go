package images

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The wait after each failure in a row doubles from five minutes and stops at a day.
func TestFetchBackoffDoublesUpToADay(t *testing.T) {
	for failures, want := range map[int]time.Duration{
		1: 5 * time.Minute, 2: 10 * time.Minute, 3: 20 * time.Minute, 9: 21*time.Hour + 20*time.Minute,
		10: 24 * time.Hour, 11: 24 * time.Hour, 1000: 24 * time.Hour,
	} {
		if got := fetchBackoff(failures); got != want {
			t.Errorf("fetchBackoff(%d) = %v, want %v", failures, got, want)
		}
	}
}

// A download that fails is recorded and held off the queue until its backoff passes, so it never
// takes a slot of the passes in between; tried again once due, it backs off longer, and the
// download that lands clears it.
func TestFailedDownloadBacksOffAndASuccessClearsIt(t *testing.T) {
	body := pngBytes(t, testImage(150, 225))
	var serve atomic.Bool
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if !serve.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	f, svc, fs := newTestFetcher(t, srv, []string{"127.0.0.1"})
	clock := fixedNow
	svc.now = func() time.Time { return clock }
	ctx := context.Background()
	adopted, err := svc.Adopt(ctx, srv.URL+"/flaky.png", IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}

	pass := func(wantConsidered int) FetchResult {
		t.Helper()
		res, err := f.FetchPending(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res.Considered != wantConsidered {
			t.Fatalf("at %v the pass considered %d, want %d", clock.Sub(fixedNow), res.Considered, wantConsidered)
		}
		return res
	}
	pass(1)
	if got := fs.images[adopted.Hash]; got.FetchFailures != 1 || !got.FetchRetryAt.Equal(fixedNow.Add(5*time.Minute)) {
		t.Fatalf("after one failure = %d failures, retry %v; want 1 and five minutes on", got.FetchFailures, got.FetchRetryAt)
	}
	clock = fixedNow.Add(4 * time.Minute)
	pass(0)
	if hits.Load() != 1 {
		t.Fatalf("%d downloads tried, want the one before the backoff", hits.Load())
	}
	clock = fixedNow.Add(5 * time.Minute)
	pass(1)
	if got := fs.images[adopted.Hash]; got.FetchFailures != 2 || !got.FetchRetryAt.Equal(clock.Add(10*time.Minute)) {
		t.Fatalf("after two failures = %d failures, retry %v; want 2 and ten minutes on", got.FetchFailures, got.FetchRetryAt)
	}

	serve.Store(true)
	clock = clock.Add(10 * time.Minute)
	if res := pass(1); res.Fetched != 1 {
		t.Fatalf("pass = %+v, want the image fetched", res)
	}
	if _, ok := fs.images[adopted.Hash]; ok {
		t.Fatal("the placeholder survived the fetch")
	}
	for _, img := range fs.images {
		if img.SourceURL == adopted.SourceURL && (img.FetchFailures != 0 || !img.FetchRetryAt.IsZero()) {
			t.Errorf("fetched image keeps %d failures, retry %v; want none", img.FetchFailures, img.FetchRetryAt)
		}
	}
}

// A permanently failing image cannot hold the queue: with a whole batch of them failing, the next
// pass takes the images behind them.
func TestFailingImagesDoNotHoldTheQueue(t *testing.T) {
	body := pngBytes(t, testImage(150, 225))
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "gone") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	f, svc, _ := newTestFetcher(t, srv, []string{"127.0.0.1"})
	ctx := context.Background()
	for i := range fetchBatch {
		if _, err := svc.Adopt(ctx, fmt.Sprintf("%s/gone-%d.png", srv.URL, i), IngestRequest{Role: RolePoster}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Adopt(ctx, srv.URL+"/ok.png", IngestRequest{Role: RolePoster}); err != nil {
		t.Fatal(err)
	}
	fetched := 0
	for range 2 {
		res, err := f.FetchPending(ctx)
		if err != nil {
			t.Fatal(err)
		}
		fetched += res.Fetched
	}
	if fetched != 1 {
		t.Fatalf("fetched %d in two passes, want the image behind %d failing ones", fetched, fetchBatch)
	}
}

// Every image past the ceiling is purged in one run, however many batches that takes.
func TestGCPurgesEveryExpiredImageInOneRun(t *testing.T) {
	const ttl = 30 * 24 * time.Hour
	gc, _, fs, _ := newTestGC(t, ttl, 0)
	ctx := context.Background()
	const due = 2*gcBatch + 7
	for i := range due {
		img := Image{
			Hash: fmt.Sprintf("%064x", i+1), Origin: OriginRemote, Role: RolePoster, MIME: "image/png",
			SourceURL:       fmt.Sprintf("https://image.tmdb.org/t/p/original/%d.png", i),
			OriginFetchedAt: fixedNow.Add(-ttl - time.Duration(i+1)*time.Second),
		}
		if err := fs.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
		if err := fs.PutRef(ctx, Ref{ImageHash: img.Hash, OwnerKind: "channel", OwnerID: "ch-ttl", Role: RolePoster}); err != nil {
			t.Fatal(err)
		}
	}
	res, err := gc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Expired != due {
		t.Fatalf("one run purged %d, want all %d", res.Expired, due)
	}
	if left, _ := fs.ListExpiredBefore(ctx, fixedNow.Add(-ttl), due); len(left) != 0 {
		t.Fatalf("%d images still past the ceiling", len(left))
	}
}
