package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// memPathCache is an in-memory PathCache that records writes.
type memPathCache struct {
	mu   sync.Mutex
	m    map[string]string
	puts []string
}

func (p *memPathCache) ItemPath(_ context.Context, id string) (string, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.m[id]
	return v, ok, nil
}

func (p *memPathCache) SetItemPath(_ context.Context, id, path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = map[string]string{}
	}
	p.m[id] = path
	p.puts = append(p.puts, id+"="+path)
	return nil
}

// refusingServer models a media-server OUTAGE: every request is counted and answered 503, so a
// double that silently succeeded (or ignored the call) cannot hide an airtime request.
func refusingServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestResolveInput_CachedPathMakesNoMediaServerRequest(t *testing.T) {
	srv, calls := refusingServer(t)
	cache := &memPathCache{m: map[string]string{"item1": "/data/tv/x.mkv"}}
	c := New(Emby, srv.URL, "tok", "dev").WithPathCache(cache)
	pm := ParsePathMap("/data=>/mnt")

	src := c.ResolveInput(t.Context(), "item1", pm, func(p string) bool { return p == "/mnt/tv/x.mkv" })
	if src.Kind != InputFile || src.URL != "/mnt/tv/x.mkv" {
		t.Fatalf("cached path must direct-play, got %+v", src)
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("airtime made %d media-server requests, want 0", n)
	}
}

func TestResolveInput_StaleCacheFallsBackAndRefreshes(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"Items":[{"Path":"/data/tv/moved.mkv"}]}`))
	}))
	t.Cleanup(srv.Close)
	cache := &memPathCache{m: map[string]string{"item1": "/data/tv/old.mkv"}}
	c := New(Emby, srv.URL, "tok", "dev").WithPathCache(cache)
	pm := ParsePathMap("/data=>/mnt")

	src := c.ResolveInput(t.Context(), "item1", pm, func(p string) bool { return p == "/mnt/tv/moved.mkv" })
	if src.Kind != InputFile || src.URL != "/mnt/tv/moved.mkv" {
		t.Fatalf("stale cache must fall back to ItemPath, got %+v", src)
	}
	if calls.Load() != 1 {
		t.Fatalf("want exactly one fallback lookup, got %d", calls.Load())
	}
	if got, _, _ := cache.ItemPath(t.Context(), "item1"); got != "/data/tv/moved.mkv" {
		t.Fatalf("cache not refreshed, holds %q", got)
	}
}

func TestResolveInput_NoCacheNoMappingIsHTTPStream(t *testing.T) {
	srv, calls := refusingServer(t)
	c := New(Emby, srv.URL, "tok", "dev").WithPathCache(&memPathCache{})
	src := c.ResolveInput(t.Context(), "item1", nil, func(string) bool { return true })
	if src.Kind != InputHTTP {
		t.Fatalf("no mapping ⇒ HTTP stream, got %+v", src)
	}
	if calls.Load() != 0 {
		t.Fatalf("no mapping must not look the path up, got %d requests", calls.Load())
	}
}

func TestResolveInput_ColdCacheLooksUpAndStores(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Items":[{"Path":"/data/tv/x.mkv"}]}`))
	}))
	t.Cleanup(srv.Close)
	cache := &memPathCache{}
	c := New(Emby, srv.URL, "tok", "dev").WithPathCache(cache)
	src := c.ResolveInput(t.Context(), "item1", ParsePathMap("/data=>/mnt"), func(string) bool { return true })
	if src.Kind != InputFile {
		t.Fatalf("got %+v", src)
	}
	if len(cache.puts) != 1 || cache.puts[0] != "item1=/data/tv/x.mkv" {
		t.Fatalf("server path (never the mapped path) must be cached, got %v", cache.puts)
	}
}
