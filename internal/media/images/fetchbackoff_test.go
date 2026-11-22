package images

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/jobs"
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
	if fs.expiredReads != 3 {
		t.Fatalf("the purge read the expired set %d times, want 3 (two full pages and a short one)", fs.expiredReads)
	}
	if left, _ := fs.ListExpiredBefore(ctx, fixedNow.Add(-ttl), ExpiryCursor{}, due); len(left) != 0 {
		t.Fatalf("%d images still past the ceiling", len(left))
	}
	for _, img := range fs.images {
		if !img.FetchQueuedAt.Equal(fixedNow) {
			t.Fatalf("a requeued image joined the queue at %v, want the purge's time: it goes behind what is waiting", img.FetchQueuedAt)
		}
	}
}

// seedExpired stores n remote images past the ceiling, oldest first, each referenced so the orphan
// sweep leaves it alone, and returns their hashes in the purge's order.
func seedExpired(t *testing.T, fs *fakeStore, ttl time.Duration, n int) []string {
	t.Helper()
	ctx := context.Background()
	hashes := make([]string, n)
	for i := range n {
		img := Image{
			Hash: fmt.Sprintf("%064x", i+1), Origin: OriginRemote, Role: RolePoster, MIME: "image/png",
			SourceURL:       fmt.Sprintf("https://image.tmdb.org/t/p/original/%d.png", i),
			OriginFetchedAt: fixedNow.Add(-ttl - time.Duration(n-i)*time.Second),
		}
		if err := fs.PutImage(ctx, img); err != nil {
			t.Fatal(err)
		}
		if err := fs.PutRef(ctx, Ref{ImageHash: img.Hash, OwnerKind: "channel", OwnerID: "ch-ttl", Role: RolePoster}); err != nil {
			t.Fatal(err)
		}
		hashes[i] = img.Hash
	}
	return hashes
}

// Images whose purge keeps failing are passed over, each tried once per run: they neither stall the
// run (a whole page of them) nor get re-read page after page (fewer than a page).
func TestGCPassesOverImagesWhosePurgeFails(t *testing.T) {
	const ttl = 30 * 24 * time.Hour
	for _, failing := range []int{3, gcBatch} {
		t.Run(fmt.Sprintf("%d failing", failing), func(t *testing.T) {
			gc, _, fs, _ := newTestGC(t, ttl, 0)
			hashes := seedExpired(t, fs, ttl, failing+7)
			for _, h := range hashes[:failing] {
				fs.failDeleteDerivatives[h] = true
			}
			res, err := gc.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if res.Expired != 7 {
				t.Fatalf("purged %d, want the 7 behind the %d failing", res.Expired, failing)
			}
			for _, h := range hashes[:failing] {
				if n := fs.deleteDerivativeCalls[h]; n != 1 {
					t.Fatalf("a failing image was tried %d times in one run, want once", n)
				}
			}
			if want := failing/gcBatch + 1; fs.expiredReads != want {
				t.Fatalf("the purge read the expired set %d times, want %d", fs.expiredReads, want)
			}
		})
	}
}

// A run its context ends stops between images, skips the duties it has not reached, and fails
// with what it purged; the next run purges the rest.
func TestGCCutShortFailsAndTheNextRunResumes(t *testing.T) {
	const ttl = 30 * 24 * time.Hour
	gc, _, fs, _ := newTestGC(t, ttl, 0)
	seedExpired(t, fs, ttl, 10)
	ctx, cancel := context.WithCancel(context.Background())
	requeued := 0
	fs.afterPut = func(img Image) {
		if img.OriginFetchedAt.IsZero() {
			if requeued++; requeued == 3 {
				cancel()
			}
		}
	}
	res, err := gc.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cut-short run returned %v, want it to fail on the canceled context", err)
	}
	if res.Expired != 3 {
		t.Fatalf("the cut-short run purged %d, want the 3 before the cancel", res.Expired)
	}
	fs.afterPut = nil
	res, err = gc.Run(context.Background())
	if err != nil || res.Expired != 7 {
		t.Fatalf("the next run = %+v, %v; want the other 7 purged", res, err)
	}
}

// The maintenance job runs under the long-job timeout: a purge of thousands of images does not
// fit River's one-minute default.
func TestMaintenanceJobHasTheLongJobTimeout(t *testing.T) {
	gc, _, _, _ := newTestGC(t, time.Hour, 0)
	if got := MaintenanceJob(nil, gc).Timeout; got != jobs.LongJobTimeout {
		t.Fatalf("maintenance timeout = %v, want %v", got, jobs.LongJobTimeout)
	}
}

// When every attempt of a pass that went to the network got no response, the network is down: each
// unreachable image waits the first backoff step without a failure counted, and a refused URL in
// the same pass neither hides the outage nor escapes its own record. A pass in which upstream
// answers with an error counts the failure.
func TestAnOutageDefersWithoutCounting(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	f, svc, fs := newTestFetcher(t, srv, []string{"127.0.0.1"})
	clock := fixedNow
	svc.now = func() time.Time { return clock }
	ctx := context.Background()
	var hashes []string
	for i := range 2 {
		img, err := svc.Adopt(ctx, fmt.Sprintf("%s/p%d.png", srv.URL, i), IngestRequest{Role: RolePoster})
		if err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, img.Hash)
	}
	refused, err := svc.Adopt(ctx, "https://elsewhere.example.test/p.png", IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}
	reachable := f.client.Transport
	f.client.Transport = unreachableTransport{}
	if res, err := f.FetchPending(ctx); err != nil || res.Failed != 3 {
		t.Fatalf("outage pass = %+v, %v; want all three failed", res, err)
	}
	for _, h := range hashes {
		if got := fs.images[h]; got.FetchFailures != 0 || !got.FetchRetryAt.Equal(fixedNow.Add(5*time.Minute)) {
			t.Fatalf("outage: %d failures, retry %v; want none counted and five minutes on", got.FetchFailures, got.FetchRetryAt)
		}
	}
	if got := fs.images[refused.Hash]; got.FetchFailures != 1 || !got.FetchRetryAt.Equal(fixedNow.Add(24*time.Hour)) {
		t.Fatalf("refused URL in an outage: %d failures, retry %v; want 1 and a day on", got.FetchFailures, got.FetchRetryAt)
	}

	f.client.Transport = reachable
	clock = fixedNow.Add(5 * time.Minute)
	if _, err := f.FetchPending(ctx); err != nil {
		t.Fatal(err)
	}
	for _, h := range hashes {
		if got := fs.images[h]; got.FetchFailures != 1 {
			t.Fatalf("an upstream error was not recorded: %d failures", got.FetchFailures)
		}
	}
}

// A pass of one image whose download never gets a response is an outage pass every time: the
// image is not counted as failing, and it waits five minutes between attempts rather than being
// tried every minute.
func TestALoneUnreachableImageWaitsBetweenAttempts(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)
	f, svc, fs := newTestFetcher(t, srv, []string{"127.0.0.1"})
	clock := fixedNow
	svc.now = func() time.Time { return clock }
	ctx := context.Background()
	img, err := svc.Adopt(ctx, srv.URL+"/slow.png", IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}
	f.client.Transport = unreachableTransport{}
	for minute, want := range []int{1, 0, 0, 0, 0, 1} {
		clock = fixedNow.Add(time.Duration(minute) * time.Minute)
		res, err := f.FetchPending(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res.Considered != want {
			t.Fatalf("minute %d considered %d, want %d", minute, res.Considered, want)
		}
	}
	if got := fs.images[img.Hash]; got.FetchFailures != 0 {
		t.Fatalf("an unreachable image counted %d failures, want none", got.FetchFailures)
	}
}

// unreachableTransport fails every request without a response, as a dead network does.
type unreachableTransport struct{}

func (unreachableTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("dial tcp: connect: network is unreachable")
}

// A download that will never succeed by waiting — the image is gone, or its host is not one this
// install fetches from — waits a day straight away, and a foreground fetch respects the wait.
func TestPermanentFailuresWaitADay(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	f, svc, fs := newTestFetcher(t, srv, []string{"127.0.0.1"})
	clock := fixedNow
	svc.now = func() time.Time { return clock }
	ctx := context.Background()
	gone, err := svc.Adopt(ctx, srv.URL+"/gone.png", IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}
	refused, err := svc.Adopt(ctx, "https://elsewhere.example.test/p.png", IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.FetchPending(ctx); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{gone.Hash, refused.Hash} {
		if got := fs.images[h]; got.FetchFailures != 1 || !got.FetchRetryAt.Equal(fixedNow.Add(24*time.Hour)) {
			t.Fatalf("%s: %d failures, retry %v; want 1 and a day on", fs.images[h].SourceURL, got.FetchFailures, got.FetchRetryAt)
		}
	}

	clock = fixedNow.Add(time.Hour)
	hits.Store(0)
	if got := f.FetchNow(ctx, []Image{fs.images[gone.Hash]}, time.Second); len(got) != 0 || hits.Load() != 0 {
		t.Fatalf("a foreground fetch during the wait = %v with %d downloads, want none", got, hits.Load())
	}
}

// Re-adopting a source URL whose image the expiry purge requeued finds that row instead of minting
// a second one beside it, and gives the new owner a ref on it.
func TestAdoptFindsARequeuedImage(t *testing.T) {
	svc, fs := newTestService(t)
	ctx := context.Background()
	src := "https://image.tmdb.org/t/p/original/requeued.png"
	requeued := Image{Hash: strings.Repeat("ab", 32), Origin: OriginRemote, Role: RolePoster, MIME: "image/png",
		SourceURL: src, CreatedAt: fixedNow.Add(-time.Hour)}
	if err := fs.PutImage(ctx, requeued); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Adopt(ctx, src, IngestRequest{Role: RolePoster, OwnerKind: "channel", OwnerID: "ch-2"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash != requeued.Hash || len(fs.images) != 1 {
		t.Fatalf("adopt = %s with %d rows, want the requeued row and no other", got.Hash, len(fs.images))
	}
	if refs := fs.refsFor(requeued.Hash); len(refs) != 1 || refs[0].OwnerID != "ch-2" {
		t.Fatalf("refs = %+v, want the new owner's", refs)
	}
	if again, err := svc.Lookup(ctx, src); err != nil || again.Hash != requeued.Hash {
		t.Fatalf("lookup = %s, %v; want the requeued row", again.Hash, err)
	}
}

// A fetch that re-keys a pending row between Adopt's lookup and its ref does not cost the new
// owner its ref: Adopt finds where the row moved and puts the ref there.
func TestAdoptKeepsItsRefWhenTheFetchRekeysTheRow(t *testing.T) {
	svc, fs := newTestService(t)
	ctx := context.Background()
	src := "https://image.tmdb.org/t/p/original/racing.png"
	pending, err := svc.Adopt(ctx, src, IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}
	fetched := pending
	fetched.Hash = strings.Repeat("cd", 32)
	fetched.OriginFetchedAt = fixedNow
	fs.beforePutRef = func(Ref) { // the fetch landing: new row, refs moved, old row deleted
		if err := fs.PutImage(ctx, fetched); err != nil {
			t.Fatal(err)
		}
		if err := fs.RepointRefs(ctx, pending.Hash, fetched.Hash); err != nil {
			t.Fatal(err)
		}
		if err := fs.DeleteImage(ctx, pending.Hash); err != nil {
			t.Fatal(err)
		}
	}
	got, err := svc.Adopt(ctx, src, IngestRequest{Role: RolePoster, OwnerKind: "channel", OwnerID: "ch-3"})
	if err != nil {
		t.Fatalf("adopt during a re-key: %v", err)
	}
	if got.Hash != fetched.Hash {
		t.Fatalf("adopt returned %s, want the row the fetch moved it to", got.Hash)
	}
	if refs := fs.refsFor(fetched.Hash); len(refs) != 1 || refs[0].OwnerID != "ch-3" {
		t.Fatalf("refs on the fetched row = %+v, want the new owner's", refs)
	}
}

// The expiry purge requeues into the refresh class, so artwork adopted afterwards is fetched
// first however many images the purge put back.
func TestPurgedImagesWaitBehindNewArtwork(t *testing.T) {
	const ttl = 30 * 24 * time.Hour
	gc, svc, fs, _ := newTestGC(t, ttl, 0)
	ctx := context.Background()
	seedExpired(t, fs, ttl, fetchBatch+1)
	if _, err := gc.Run(ctx); err != nil {
		t.Fatal(err)
	}
	clock := fixedNow.Add(time.Minute)
	svc.now = func() time.Time { return clock }
	fresh, err := svc.Adopt(ctx, "https://image.tmdb.org/t/p/original/new.png", IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}
	queue, err := fs.ListAwaitingFetch(ctx, clock, fetchBatch)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != fetchBatch || queue[0].Hash != fresh.Hash {
		t.Fatalf("front of the queue = %s, want the artwork adopted after the purge", queue[0].Hash)
	}
	for _, img := range queue[1:] {
		if img.FetchPriority != FetchRefresh {
			t.Fatalf("a purged image queued at priority %d, want the refresh class", img.FetchPriority)
		}
	}
}
