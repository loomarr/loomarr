package demolibrary

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeOrigin serves body at /clip.mp4 with Range support, counting requests and recording the
// Range headers it saw. failFirst answers that many requests with a 503 first.
type fakeOrigin struct {
	body      []byte
	failFirst int32
	requests  atomic.Int32
	mu        sync.Mutex
	ranges    []string
}

func (o *fakeOrigin) seenRanges() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.ranges...)
}

func (o *fakeOrigin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := o.requests.Add(1)
	o.mu.Lock()
	o.ranges = append(o.ranges, r.Header.Get("Range"))
	o.mu.Unlock()
	if n <= o.failFirst {
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	http.ServeContent(w, r, "clip.mp4", time.Time{}, bytes.NewReader(o.body))
}

func testDownloader() downloader {
	return downloader{client: http.DefaultClient, attempts: 3, backoff: time.Millisecond, log: slog.New(slog.DiscardHandler)}
}

func pinFor(t *testing.T, url string, body []byte) PackMedia {
	t.Helper()
	sum := sha256.Sum256(body)
	return PackMedia{URL: url, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))}
}

func TestFetchVerifiesAndIsIdempotent(t *testing.T) {
	body := bytes.Repeat([]byte("loomarr"), 5000)
	o := &fakeOrigin{body: body}
	srv := httptest.NewServer(o)
	t.Cleanup(srv.Close)
	dest := filepath.Join(t.TempDir(), "clip.mp4")
	pin := pinFor(t, srv.URL+"/clip.mp4", body)

	d := testDownloader()
	if err := d.fetch(context.Background(), pin, dest); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dest); !bytes.Equal(got, body) {
		t.Fatal("downloaded file differs from the origin")
	}
	if err := d.fetch(context.Background(), pin, dest); err != nil {
		t.Fatal(err)
	}
	if n := o.requests.Load(); n != 1 {
		t.Fatalf("requests = %d, want 1: a verified file must not be fetched again", n)
	}
}

// The origin serving different bytes of the right size is the case a size check alone would let
// through. It must fail loudly, after every attempt, and leave nothing behind to be served.
func TestFetchFailsLoudlyOnAHashMismatch(t *testing.T) {
	body := bytes.Repeat([]byte("A"), 4096)
	tampered := bytes.Repeat([]byte("B"), 4096)
	o := &fakeOrigin{body: tampered}
	srv := httptest.NewServer(o)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	dest := filepath.Join(dir, "clip.mp4")

	d := testDownloader()
	err := d.fetch(context.Background(), pinFor(t, srv.URL+"/clip.mp4", body), dest)
	if !errors.Is(err, errHashMismatch) {
		t.Fatalf("fetch = %v, want a hash mismatch", err)
	}
	if n := o.requests.Load(); int(n) != d.attempts {
		t.Fatalf("requests = %d, want one per attempt (%d)", n, d.attempts)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("a failed fetch left %d files behind", len(entries))
	}
}

func TestFetchResumesAPartialDownload(t *testing.T) {
	body := make([]byte, 64*1024)
	for i := range body {
		body[i] = byte(i * 7)
	}
	o := &fakeOrigin{body: body}
	srv := httptest.NewServer(o)
	t.Cleanup(srv.Close)
	dest := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(dest+".partial", body[:20000], 0o644); err != nil {
		t.Fatal(err)
	}

	if err := testDownloader().fetch(context.Background(), pinFor(t, srv.URL+"/clip.mp4", body), dest); err != nil {
		t.Fatal(err)
	}
	if r := o.seenRanges(); len(r) != 1 || r[0] != "bytes=20000-" {
		t.Fatalf("ranges = %q, want one request resuming at byte 20000", r)
	}
	if got, _ := os.ReadFile(dest); !bytes.Equal(got, body) {
		t.Fatal("resumed file differs from the origin")
	}
}

func TestFetchRetriesAFlakyOriginAndReplacesACorruptFile(t *testing.T) {
	body := bytes.Repeat([]byte("retro"), 3000)
	o := &fakeOrigin{body: body, failFirst: 2}
	srv := httptest.NewServer(o)
	t.Cleanup(srv.Close)
	dest := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(dest, bytes.Repeat([]byte("x"), len(body)), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := testDownloader().fetch(context.Background(), pinFor(t, srv.URL+"/clip.mp4", body), dest); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	got, _ := io.ReadAll(f)
	if !bytes.Equal(got, body) {
		t.Fatal("the corrupt file was not replaced with the pinned bytes")
	}
}

func TestFetchPrefersTheMirrorAndLeavesTheOriginAlone(t *testing.T) {
	body := bytes.Repeat([]byte("mirror"), 4000)
	mirror, origin := &fakeOrigin{body: body}, &fakeOrigin{body: body}
	ms, os2 := httptest.NewServer(mirror), httptest.NewServer(origin)
	t.Cleanup(ms.Close)
	t.Cleanup(os2.Close)
	d := testDownloader()
	d.mirror = ms.URL + "/"
	dest := filepath.Join(t.TempDir(), "clip.mp4")

	if err := d.fetch(context.Background(), pinFor(t, os2.URL+"/clip.mp4", body), dest); err != nil {
		t.Fatal(err)
	}
	if mirror.requests.Load() != 1 || origin.requests.Load() != 0 {
		t.Fatalf("mirror %d, origin %d requests; want 1 and 0", mirror.requests.Load(), origin.requests.Load())
	}
}

func TestFetchFallsBackToTheOriginWhenTheMirrorFails(t *testing.T) {
	body := bytes.Repeat([]byte("origin"), 4000)
	mirror := &fakeOrigin{body: body, failFirst: 1000}
	origin := &fakeOrigin{body: body}
	ms, os2 := httptest.NewServer(mirror), httptest.NewServer(origin)
	t.Cleanup(ms.Close)
	t.Cleanup(os2.Close)
	d := testDownloader()
	d.mirror = ms.URL + "/"
	dest := filepath.Join(t.TempDir(), "clip.mp4")

	if err := d.fetch(context.Background(), pinFor(t, os2.URL+"/clip.mp4", body), dest); err != nil {
		t.Fatal(err)
	}
	if got := mirror.requests.Load(); got != mirrorAttempts {
		t.Fatalf("the mirror was tried %d times, want %d before falling back", got, mirrorAttempts)
	}
	if got, _ := os.ReadFile(dest); !bytes.Equal(got, body) {
		t.Fatal("the fallback did not deliver the pinned bytes")
	}
}
