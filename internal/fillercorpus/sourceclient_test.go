package fillercorpus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceClientCachesRawResponseAndRetainsNetworkAccounting(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; _, _ = w.Write([]byte(`{"ok":true}`)) }))
	defer server.Close()
	client, err := NewSourceClient(SourceClientConfig{HTTP: server.Client(), CacheDir: t.TempDir(), UserAgent: "test", MaxRequests: 1, MaxResponseBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, _, err := client.Get(context.Background(), server.URL); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 || client.RequestsUsed() != 1 || client.ResponseBytes() != 22 {
		t.Fatalf("calls=%d requests=%d bytes=%d", calls, client.RequestsUsed(), client.ResponseBytes())
	}
}

func TestSourceClientFailsClosedOnAggregateResponseCeiling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("too large")) }))
	defer server.Close()
	client, err := NewSourceClient(SourceClientConfig{HTTP: server.Client(), CacheDir: t.TempDir(), UserAgent: "test", MaxRequests: 1, MaxResponseBytes: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Get(context.Background(), server.URL); err == nil || !strings.Contains(err.Error(), "ceiling") {
		t.Fatalf("err=%v", err)
	}
}

func TestSourceClientCachesBoundedHeadFacts(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodHead || r.Header.Get("User-Agent") != "test" {
			t.Fatalf("request = %s user-agent=%q", r.Method, r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "123")
	}))
	defer server.Close()
	client, err := NewSourceClient(SourceClientConfig{HTTP: server.Client(), CacheDir: t.TempDir(), UserAgent: "test", MaxRequests: 1, MaxResponseBytes: 1000})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		head, _, err := client.Head(context.Background(), server.URL)
		if err != nil {
			t.Fatal(err)
		}
		if head.ContentLength != 123 || head.ContentType != "video/mp4" {
			t.Fatalf("head = %+v", head)
		}
	}
	if calls != 1 || client.RequestsUsed() != 1 || client.ResponseBytes() <= 0 {
		t.Fatalf("calls=%d requests=%d bytes=%d", calls, client.RequestsUsed(), client.ResponseBytes())
	}
}

func TestSourceClientFailsClosedOnUnreadableOrInvalidCache(t *testing.T) {
	cache := t.TempDir()
	client, err := NewSourceClient(SourceClientConfig{HTTP: http.DefaultClient, CacheDir: cache, UserAgent: "test", MaxRequests: 1, MaxResponseBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	rawURL := "https://example.invalid/video.mp4"
	path := filepath.Join(cache, sourceCacheKey(rawURL)+".head.json")
	if err := os.WriteFile(path, []byte(`{"contentLength":0,"contentType":"video/mp4"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Head(context.Background(), rawURL); err == nil {
		t.Fatal("invalid cached HEAD facts passed")
	}
}
