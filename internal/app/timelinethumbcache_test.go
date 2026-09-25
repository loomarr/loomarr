package app

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/images"
	"github.com/loomarr/loomarr/internal/tmdb"
)

// countingTMDB serves one episode still (series 456 S1E6 → /lisa.jpg) and counts EVERY request,
// which is the number these tests exist to hold down: the guide's cost was outbound calls per
// request, so an assertion that only looked at the returned URL would pass with no cache at all.
type countingTMDB struct {
	hits   atomic.Int64
	status atomic.Int64 // 0 ⇒ serve normally; otherwise fail every request with this status
	srv    *httptest.Server
}

func newCountingTMDB(t *testing.T) (*countingTMDB, *tmdb.Client) {
	t.Helper()
	c := &countingTMDB{}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.hits.Add(1)
		if s := int(c.status.Load()); s != 0 {
			w.WriteHeader(s)
			return
		}
		if r.URL.Path == "/tv/456/season/1/episode/6" {
			_, _ = w.Write([]byte(`{"still_path":"/lisa.jpg"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(c.srv.Close)
	return c, tmdb.NewWithBase(c.srv.URL, "key")
}

// gatedFetch is a FetchNow whose result the test releases, so "the request did not wait for the
// image" is provable: ThumbFor must return while the fetch is still blocked.
type gatedFetch struct {
	release chan struct{}
	calls   atomic.Int64
	warm    map[string]images.Image
}

func (f *gatedFetch) FetchNow(_ context.Context, _ []images.Image, _ time.Duration) map[string]images.Image {
	f.calls.Add(1)
	<-f.release
	return f.warm
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func newThumbsUnderTest(
	t *testing.T, client *tmdb.Client, fetch timelineImageFetcher, logs *lockedBuffer,
) *timelineThumbs {
	t.Helper()
	svc := newTestImageService(t, t.TempDir(), "https://machine-client-only.invalid", newMemImageStore())
	log := slog.New(slog.NewTextHandler(logs, nil))
	return newTimelineThumbs(t.Context(), timelineThumbResolver{tmdb: client, images: svc, fetch: fetch}, log)
}

// A cold programme image must not be fetched — or even looked up — while the Guide request is open.
// The artwork is a hover enhancement; the maintainer accepted it appearing on a LATER refresh.
func TestTimelineThumbsColdArtworkIsWarmedOutsideTheRequest(t *testing.T) {
	t.Parallel()
	_, client := newCountingTMDB(t)
	contentHash := strings.Repeat("d", 64)
	fetch := &gatedFetch{release: make(chan struct{}), warm: map[string]images.Image{
		imgBase + "/lisa.jpg": {
			Hash: contentHash, SourceURL: imgBase + "/lisa.jpg", Role: images.RoleBackdrop,
			OriginFetchedAt: time.Unix(1_700_000_000, 0),
		},
	}}
	th := newThumbsUnderTest(t, client, fetch, &lockedBuffer{})

	done := make(chan struct{})
	var url, hash string
	go func() {
		url, hash = th.ThumbFor(t.Context(), "series:tmdb:456", 1, 6)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ThumbFor blocked on the cold image; the request path must not wait for a fetch")
	}
	if url != "" || hash != "" {
		t.Errorf("cold ThumbFor = (%q, %q), want empty — the image appears on a later refresh", url, hash)
	}

	close(fetch.release)
	th.wait()
	if fetch.calls.Load() != 1 {
		t.Errorf("FetchNow calls = %d, want the one background warm", fetch.calls.Load())
	}
	want := "/v1/images/" + contentHash + "/w300.jpg?r=loomarr-rendition-v2"
	if url, hash := th.ThumbFor(t.Context(), "series:tmdb:456", 1, 6); url != want || hash != contentHash {
		t.Errorf("warm ThumbFor = (%q, %q), want (%q, %q)", url, hash, want, contentHash)
	}
}

// Repeated Guide loads over the same programmes must not re-ask TMDB inside the TTL — for a title
// that has artwork AND for one TMDB has nothing for (a 404 is an answer, not something to retry).
func TestTimelineThumbsRepeatedRequestsMakeNoRepeatTMDBCalls(t *testing.T) {
	t.Parallel()
	tm, client := newCountingTMDB(t)
	fetch := &gatedFetch{release: make(chan struct{}), warm: map[string]images.Image{
		imgBase + "/lisa.jpg": {
			Hash: strings.Repeat("e", 64), SourceURL: imgBase + "/lisa.jpg", Role: images.RoleBackdrop,
			OriginFetchedAt: time.Unix(1_700_000_000, 0),
		},
	}}
	close(fetch.release)
	th := newThumbsUnderTest(t, client, fetch, &lockedBuffer{})

	ask := func() {
		th.ThumbFor(t.Context(), "series:tmdb:456", 1, 6) // has a still
		th.ThumbFor(t.Context(), "series:tmdb:456", 9, 9) // TMDB 404s
	}
	ask()
	th.wait()
	afterWarm := tm.hits.Load()
	if afterWarm == 0 {
		t.Fatal("no TMDB calls at all — the warm-up did not run")
	}
	for range 25 {
		ask()
		th.wait()
	}
	if got := tm.hits.Load(); got != afterWarm {
		t.Errorf("TMDB calls after 25 repeat requests = %d, want %d (no repeat calls inside the TTL)", got, afterWarm)
	}
}

// A TMDB failure is recorded with its CLASS (it used to be swallowed to "") and is negatively
// cached, so a failing key is not retried on every Guide load — but is retried once the short
// negative TTL passes.
func TestTimelineThumbsRecordsFailureClassAndNegativelyCaches(t *testing.T) {
	t.Parallel()
	tm, client := newCountingTMDB(t)
	tm.status.Store(http.StatusInternalServerError)
	logs := &lockedBuffer{}
	th := newThumbsUnderTest(t, client, nil, logs)
	clock := time.Unix(1_700_000_000, 0)
	th.now = func() time.Time { return clock }

	th.ThumbFor(t.Context(), "series:tmdb:456", 1, 6)
	th.wait()
	first := tm.hits.Load()
	if first == 0 {
		t.Fatal("the failing lookup never reached TMDB")
	}
	if out := logs.String(); !strings.Contains(out, "class=server_error") {
		t.Errorf("failure log = %q, want it to carry class=server_error", out)
	}

	for range 10 {
		th.ThumbFor(t.Context(), "series:tmdb:456", 1, 6)
		th.wait()
	}
	if got := tm.hits.Load(); got != first {
		t.Errorf("TMDB calls while negatively cached = %d, want %d", got, first)
	}

	clock = clock.Add(timelineThumbFailureTTL + time.Second)
	th.ThumbFor(t.Context(), "series:tmdb:456", 1, 6)
	th.wait()
	if got := tm.hits.Load(); got <= first {
		t.Errorf("TMDB calls after the negative TTL = %d, want a retry (> %d)", got, first)
	}
}

// The cache must stay bounded however many distinct programmes pass through it.
func TestTimelineThumbsCacheIsBounded(t *testing.T) {
	t.Parallel()
	_, client := newCountingTMDB(t)
	th := newThumbsUnderTest(t, client, nil, &lockedBuffer{})
	for i := range timelineThumbMaxEntries * 2 {
		th.store(timelineThumbID{key: "movie:tmdb:1", season: i}, thumbEntry{})
	}
	if n := th.size(); n > timelineThumbMaxEntries {
		t.Errorf("cache holds %d entries, want ≤ %d", n, timelineThumbMaxEntries)
	}
}

// #1397 acceptance: outbound TMDB calls per /v1/guide request, before vs after. "Before" is the
// resolver invoked straight from the request, which is exactly what the old ThumbFor did on every
// programme of every load; "after" is the same window served through timelineThumbs. The numbers
// are logged for the PR; the assertions pin the shape (none on the request path, none on repeats).
func TestTimelineThumbsOutboundCallsPerGuideRequest(t *testing.T) {
	t.Parallel()
	const programmes, requests = 50, 10
	tm, client := newCountingTMDB(t)
	fetch := &gatedFetch{release: make(chan struct{}), warm: map[string]images.Image{
		imgBase + "/lisa.jpg": {
			Hash: strings.Repeat("f", 64), SourceURL: imgBase + "/lisa.jpg", Role: images.RoleBackdrop,
			OriginFetchedAt: time.Unix(1_700_000_000, 0),
		},
	}}
	close(fetch.release)
	th := newThumbsUnderTest(t, client, fetch, &lockedBuffer{})

	// Before: every request resolves every programme inline.
	for range requests {
		for e := range programmes {
			_, _, _ = th.src.resolve(t.Context(), "series:tmdb:456", 1, 6+e%2*100)
		}
	}
	before := tm.hits.Load()

	// After: same window, same number of requests, through the cache.
	tm.hits.Store(0)
	perRequest := make([]int64, 0, requests)
	for range requests {
		start := tm.hits.Load()
		for e := range programmes {
			th.ThumbFor(t.Context(), "series:tmdb:456", 1, 6+e%2*100)
		}
		th.wait() // count the request's background warms too: they are still calls it caused
		perRequest = append(perRequest, tm.hits.Load()-start)
	}
	after := tm.hits.Load()
	t.Logf("TMDB calls over %d requests x %d programmes: before=%d after=%d (per request after: %v)",
		requests, programmes, before, after, perRequest)

	for i, n := range perRequest[1:] {
		if n != 0 {
			t.Errorf("request %d made %d TMDB calls, want 0 once warm", i+2, n)
		}
	}
	if after >= before/requests*2 {
		t.Errorf("after=%d is not a large reduction from before=%d", after, before)
	}
}
