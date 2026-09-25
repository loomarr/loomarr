package setup_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/setup"
	"github.com/loomarr/loomarr/internal/testkit"
)

// fakeEmby serves the media-server surface RefreshTarget touches. Each POST /LiveTv/TunerHosts
// pops the next scripted status (the last one repeats), reproducing #1407's production shape:
// Emby answering "ServiceUnavailable" while it is stalled.
type fakeEmby struct {
	t        *testing.T
	mu       sync.Mutex
	statuses []int
	posts    int
	guide    int
}

func (f *fakeEmby) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/System/Configuration/livetv":
			_, _ = w.Write(testkit.Fixture(f.t, "livetv/livetv_config_jellyfin.json"))
		case r.Method == http.MethodPost && r.URL.Path == "/LiveTv/TunerHosts":
			f.mu.Lock()
			i := f.posts
			f.posts++
			f.mu.Unlock()
			if i >= len(f.statuses) {
				i = len(f.statuses) - 1
			}
			if f.statuses[i] >= 400 {
				http.Error(w, "ServiceUnavailable", f.statuses[i])
				return
			}
			w.WriteHeader(f.statuses[i])
		case r.Method == http.MethodGet && r.URL.Path == "/ScheduledTasks":
			_, _ = w.Write(append(append([]byte("["), testkit.Fixture(f.t, "livetv/guide_refresh_task.json")...), ']'))
		case r.Method == http.MethodPost && r.URL.Path == "/ScheduledTasks/Running/9492d30c70f7f1bec3757c9d0a4feb45":
			f.mu.Lock()
			f.guide++
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			f.t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
}

func refreshFixture(t *testing.T, statuses ...int) (*fakeEmby, *setup.LiveTVConnector, setup.LiveTVURLs) {
	t.Helper()
	fake := &fakeEmby{t: t, statuses: statuses}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	urls := setup.LiveTVURLs{M3U: "http://192.0.2.79:8001/api/channels.m3u", XMLTV: "http://192.0.2.79:8001/api/xmltv.xml"}
	client := library.New(library.Emby, srv.URL, "test-token", "dev-1")
	return fake, setup.NewLiveTVConnectorFixed(client, urls), urls
}

// A stalled Emby that recovers within the retry budget must not fail the refresh.
func TestRefreshTarget_RetriesTransient5xxThenSucceeds(t *testing.T) {
	defer setup.SetRefreshBackoffForTest(time.Millisecond, time.Millisecond)()
	fake, connector, urls := refreshFixture(t, 500, 500, 200)

	if err := connector.RefreshTarget(context.Background(), urls); err != nil {
		t.Fatalf("RefreshTarget = %v, want success after two 500s", err)
	}
	if fake.posts != 3 {
		t.Errorf("tuner POSTs = %d, want 3 (500, 500, 200)", fake.posts)
	}
	if fake.guide != 1 {
		t.Errorf("guide refreshes = %d, want 1", fake.guide)
	}
}

// Persistent 500s exhaust the bounded budget and surface as a TRANSIENT error carrying the
// cause class, so callers can tell "media server is busy" from "wiring is wrong".
func TestRefreshTarget_PersistentServerErrorIsBoundedAndClassified(t *testing.T) {
	defer setup.SetRefreshBackoffForTest(time.Millisecond, time.Millisecond)()
	fake, connector, urls := refreshFixture(t, 500)

	err := connector.RefreshTarget(context.Background(), urls)
	if err == nil {
		t.Fatal("RefreshTarget = nil, want the exhausted transient error")
	}
	var transient interface {
		Transient() bool
		Class() string
	}
	if !errors.As(err, &transient) || !transient.Transient() {
		t.Fatalf("error %v is not marked transient", err)
	}
	if transient.Class() != "server-error" {
		t.Errorf("class = %q, want server-error", transient.Class())
	}
	if fake.posts != 3 {
		t.Errorf("tuner POSTs = %d, want bounded at 3 attempts", fake.posts)
	}
}

// A 4xx means the wiring itself is wrong: retrying only delays the real signal.
func TestRefreshTarget_ClientErrorIsNotRetriedNorTransient(t *testing.T) {
	defer setup.SetRefreshBackoffForTest(time.Millisecond, time.Millisecond)()
	fake, connector, urls := refreshFixture(t, 401)

	err := connector.RefreshTarget(context.Background(), urls)
	if err == nil {
		t.Fatal("RefreshTarget = nil, want the 401")
	}
	var transient interface{ Transient() bool }
	if errors.As(err, &transient) {
		t.Errorf("a 401 must not be classified transient: %v", err)
	}
	if fake.posts != 1 {
		t.Errorf("tuner POSTs = %d, want 1 (no retry on 4xx)", fake.posts)
	}
}

// Cancellation during backoff stops the retries immediately.
func TestRefreshTarget_StopsRetryingWhenContextCancelled(t *testing.T) {
	defer setup.SetRefreshBackoffForTest(time.Hour, time.Hour)()
	fake, connector, urls := refreshFixture(t, 500)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := connector.RefreshTarget(ctx, urls)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("err=%v after %v, want prompt failure on cancellation", err, time.Since(start))
	}
	if fake.posts != 1 {
		t.Errorf("tuner POSTs = %d, want 1", fake.posts)
	}
}
