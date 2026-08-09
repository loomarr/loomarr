package images

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestFetcher wires a Fetcher at a TLS test server.
//
// ⚠ The client is swapped for the test server's — which trusts its self-signed certificate —
// rather than the guard being relaxed to allow http. Relaxing it would mean the https rule is
// never exercised by anything, and a rule no test can see is one a later refactor deletes.
// CheckRedirect is re-attached explicitly, because taking srv.Client() wholesale would silently
// drop the redirect guard and every redirect assertion below would pass vacuously.
func newTestFetcher(t *testing.T, srv *httptest.Server, hosts []string) (*Fetcher, *Service, *fakeStore) {
	t.Helper()
	svc, fs := newTestService(t)
	f := NewFetcher(svc, fs,
		func() bool { return true },
		func() int { return 2 },
		func() []string { return hosts },
		nil)
	client := srv.Client()
	client.CheckRedirect = f.checkRedirect
	f.client = client
	return f, svc, fs
}

// The fetch job's core behaviour: an adopted row keyed on its URL becomes a row keyed on the bytes.
func TestFetchRekeysAdoptedRowOntoTheContentHash(t *testing.T) {
	body := pngBytes(t, testImage(400, 600))
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	f, svc, fs := newTestFetcher(t, srv, []string{"127.0.0.1"})
	ctx := context.Background()

	adopted, err := svc.Adopt(ctx, srv.URL+"/poster.png", IngestRequest{
		Role: RolePoster, OwnerKind: "channel", OwnerID: "ch-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	contentHash := HashBytes(body)
	if adopted.Hash == contentHash {
		t.Fatal("the adopted row is already keyed on the content hash — this test cannot see the re-key")
	}

	res, err := f.FetchPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 1 || res.Rekeyed != 1 {
		t.Fatalf("FetchPending = %+v, want one fetched and one re-keyed", res)
	}

	got, err := fs.GetImage(ctx, contentHash)
	if err != nil {
		t.Fatalf("no row under the content hash after fetching: %v", err)
	}
	if got.Width != 400 || got.Height != 600 {
		t.Errorf("dimensions %dx%d, want 400x600 — they come from the decoded bytes, not the row", got.Width, got.Height)
	}
	if got.OriginFetchedAt.IsZero() {
		t.Error("originFetchedAt is still zero — the row stays on the fetch queue forever")
	}
	if got.SourceURL == "" {
		t.Error("sourceURL was dropped in the re-key — the image is now merely cached, not recoverable")
	}
	if got.Placeholder == "" {
		t.Error("no ThumbHash after fetching — the surface has nothing to show while the rendition loads")
	}

	// The placeholder row is gone and its ref travelled.
	if _, err := fs.GetImage(ctx, adopted.Hash); err == nil {
		t.Error("the URL-keyed placeholder row survived the re-key")
	}
	refs := fs.refsFor(contentHash)
	if len(refs) != 1 || refs[0].OwnerID != "ch-1" {
		t.Fatalf("refs on the content hash = %+v, want the channel ref moved across", refs)
	}
	if len(fs.refsFor(adopted.Hash)) != 0 {
		t.Error("the placeholder still has refs — it will never be collected as an orphan")
	}

	// And the bytes are on disk under the content hash, so a rendition can be produced.
	if _, err := svc.Rendition(ctx, contentHash, FormatWebP, 342); err != nil {
		t.Errorf("no rendition after a fetch: %v", err)
	}
}

// ⚠ Re-adopting a source URL whose bytes already landed must not queue a second download.
//
// The re-key DELETES the URL-keyed placeholder row, so `Adopt`'s `GetImage(hashOfURL(src))` misses
// every time afterwards and mints a fresh placeholder — and the fetch job then re-downloads bytes
// the disk already holds. Nothing noticed while `Adopt` had no production caller. V52 phase 7 gave
// it one on an INTERACTIVE surface, where the symptom is that opening the icon picker re-downloads
// a dozen posters from TMDB every single time, against an origin that caps us at 20 connections.
func TestReAdoptingAFetchedURLDoesNotRefetch(t *testing.T) {
	body := pngBytes(t, testImage(400, 600))
	hits := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	f, svc, _ := newTestFetcher(t, srv, []string{"127.0.0.1"})
	ctx := context.Background()
	src := srv.URL + "/poster.png"

	if _, err := svc.Adopt(ctx, src, IngestRequest{Role: RoleIcon}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.FetchPending(ctx); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("origin hits after the first fetch = %d, want 1", hits)
	}

	// The same URL again — what every re-render of a surface does.
	again, err := svc.Adopt(ctx, src, IngestRequest{Role: RoleIcon})
	if err != nil {
		t.Fatal(err)
	}
	if want := HashBytes(body); again.Hash != want {
		t.Errorf("re-adopt returned %s, want the content hash %s — it minted a second placeholder", again.Hash, want)
	}
	if again.OriginFetchedAt.IsZero() {
		t.Error("re-adopt returned an unfetched row — it is queued for a download it does not need")
	}

	if _, err := f.FetchPending(ctx); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Errorf("origin hits = %d, want 1 — re-adopting re-downloaded bytes already on disk", hits)
	}
}

// A re-fetch that yields identical bytes must not churn: same hash, same row, same URLs.
func TestRefetchOfIdenticalBytesKeepsTheSameIdentity(t *testing.T) {
	body := pngBytes(t, testImage(300, 300))
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	f, svc, fs := newTestFetcher(t, srv, []string{"127.0.0.1"})
	ctx := context.Background()

	if _, err := svc.Adopt(ctx, srv.URL+"/a.png", IngestRequest{Role: RolePoster}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.FetchPending(ctx); err != nil {
		t.Fatal(err)
	}

	// Force it back onto the queue the way the TTL sweep does, then fetch again.
	contentHash := HashBytes(body)
	rec, _ := fs.GetImage(ctx, contentHash)
	rec.OriginFetchedAt = time.Time{}
	if err := fs.PutImage(ctx, rec); err != nil {
		t.Fatal(err)
	}

	res, err := f.FetchPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 1 {
		t.Fatalf("FetchPending = %+v, want the requeued row fetched", res)
	}
	if res.Rekeyed != 0 {
		t.Error("identical bytes re-keyed the row — every derivative URL would change for nothing, " +
			"and §22 promises a re-fetch of unchanged artwork is invisible downstream")
	}
	if _, err := fs.GetImage(ctx, contentHash); err != nil {
		t.Errorf("the row moved even though the bytes did not: %v", err)
	}
}

// Rehydrate is a DISK sweep: a row is only work if its file is actually gone.
func TestRehydrateOnlyRefetchesImagesWhoseFilesAreMissing(t *testing.T) {
	body := pngBytes(t, testImage(200, 300))
	var hits int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	f, svc, fs := newTestFetcher(t, srv, []string{"127.0.0.1"})
	ctx := context.Background()

	if _, err := svc.Adopt(ctx, srv.URL+"/b.png", IngestRequest{Role: RolePoster}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.FetchPending(ctx); err != nil {
		t.Fatal(err)
	}
	before := hits

	// Everything is present: rehydrate must do nothing at all.
	if res, err := f.Rehydrate(ctx); err != nil {
		t.Fatal(err)
	} else if res.Considered != 0 {
		t.Fatalf("Rehydrate on a healthy install considered %d images, want 0 — it is a stat sweep, "+
			"and re-downloading intact artwork daily would hammer upstream for nothing", res.Considered)
	}
	if hits != before {
		t.Fatalf("Rehydrate made %d extra requests on a healthy install", hits-before)
	}

	// Now lose the file the way a restore onto an empty volume does.
	contentHash := HashBytes(body)
	rec, _ := fs.GetImage(ctx, contentHash)
	path, err := svc.blob.OriginalPath(rec.Hash, extForMIME(rec.MIME))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.blob.Remove(path); err != nil {
		t.Fatal(err)
	}

	res, err := f.Rehydrate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 1 {
		t.Fatalf("Rehydrate = %+v, want the missing file re-fetched", res)
	}
	if _, ok := svc.blob.Stat(path); !ok {
		t.Error("the file is still missing after rehydrating")
	}
}

// An upstream that answers 200 with something that is not an image must not reach disk.
func TestFetchRejectsABodyThatIsNotAnImage(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A 200 carrying an HTML error page, declared as an image — the exact shape a captive
		// portal or a misconfigured CDN produces, and the reason the sniff is the allowlist.
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("<!doctype html><title>404</title>"))
	}))
	t.Cleanup(srv.Close)

	f, svc, fs := newTestFetcher(t, srv, []string{"127.0.0.1"})
	ctx := context.Background()

	adopted, err := svc.Adopt(ctx, srv.URL+"/c.jpg", IngestRequest{Role: RolePoster})
	if err != nil {
		t.Fatal(err)
	}
	res, err := f.FetchPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 {
		t.Fatalf("FetchPending = %+v, want the non-image rejected", res)
	}
	// The row stays exactly as it was, which is what keeps it on the queue for the next pass.
	got, err := fs.GetImage(ctx, adopted.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OriginFetchedAt.IsZero() {
		t.Error("a rejected fetch stamped originFetchedAt — the row would never be retried")
	}
}

// One bad URL must not fail the pass, or a single dead poster would stall the other sixty-three.
func TestOneFailureDoesNotStopTheBatch(t *testing.T) {
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

	for _, p := range []string{"/gone.png", "/ok.png"} {
		if _, err := svc.Adopt(ctx, srv.URL+p, IngestRequest{Role: RolePoster}); err != nil {
			t.Fatal(err)
		}
	}

	res, err := f.FetchPending(ctx)
	if err != nil {
		t.Fatalf("a single 404 failed the whole pass: %v", err)
	}
	if res.Fetched != 1 || res.Failed != 1 {
		t.Fatalf("FetchPending = %+v, want one fetched and one failed", res)
	}
}

// --- The SSRF gate ---

func TestCheckURLRefusesWhatItMust(t *testing.T) {
	f := NewFetcher(nil, nil, nil, nil, func() []string { return []string{"tmdb.org"} }, nil)

	for _, tc := range []struct {
		name, url, wantErr string
	}{
		{"plaintext", "http://image.tmdb.org/t/p/original/x.jpg", "non-https"},
		{"off the allowlist", "https://attacker.example/x.jpg", "not one this install"},
		{"the metadata service", "https://169.254.169.254/latest/meta-data/", "not one this install"},
		{"empty", "", "no source URL"},
		// ⚠ The lookalike is the assertion that matters. A bare `strings.HasSuffix(host, entry)`
		// — the obvious implementation — accepts this and hands an attacker the allowlist.
		{"suffix lookalike", "https://eviltmdb.org/x.jpg", "not one this install"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := f.checkURL(tc.url)
			if err == nil {
				t.Fatalf("checkURL(%q) allowed it", tc.url)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("checkURL(%q) = %v, want it to mention %q", tc.url, err, tc.wantErr)
			}
		})
	}

	if err := f.checkURL("https://image.tmdb.org/t/p/original/x.jpg"); err != nil {
		t.Errorf("checkURL refused the allowlisted host: %v", err)
	}
}

// A redirect is the hop an allowlisted host controls, so it gets the private-range rule the first
// hop deliberately does not.
func TestRedirectIntoAPrivateAddressIsRefused(t *testing.T) {
	body := pngBytes(t, testImage(120, 180))
	var origin *httptest.Server
	origin = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "redirect") {
			// 127.0.0.1 is the test server's own host — loopback, which the guard must refuse
			// even though it is the very host on the allowlist. The allowlist governs hop one.
			http.Redirect(w, r, origin.URL+"/inner.png", http.StatusFound)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(origin.Close)

	f, svc, _ := newTestFetcher(t, origin, []string{"127.0.0.1"})
	ctx := context.Background()

	if _, err := svc.Adopt(ctx, origin.URL+"/redirect.png", IngestRequest{Role: RolePoster}); err != nil {
		t.Fatal(err)
	}
	res, err := f.FetchPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 {
		t.Fatalf("FetchPending = %+v, want the redirect into loopback refused", res)
	}
}

// A disabled install makes no outbound requests at all — the setting's whole purpose.
func TestRemoteFetchDisabledMakesNoRequests(t *testing.T) {
	var hits int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits++ }))
	t.Cleanup(srv.Close)

	svc, fs := newTestService(t)
	f := NewFetcher(svc, fs, func() bool { return false }, func() int { return 2 },
		func() []string { return []string{"127.0.0.1"} }, nil)
	f.client = srv.Client()

	ctx := context.Background()
	if _, err := svc.Adopt(ctx, srv.URL+"/d.png", IngestRequest{Role: RolePoster}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.FetchPending(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Rehydrate(ctx); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Errorf("images.remote_fetch_enabled=false still made %d requests", hits)
	}
}
