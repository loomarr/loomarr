package web

import (
	"compress/gzip"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"testing/fstest"
)

// `//go:embed all:dist` does not compile unless the directory exists, so .gitkeep is the
// one committed file in there (.gitignore un-ignores exactly that path). Vite's
// `emptyOutDir` deletes it on every build, and a routine `git add -A` then stages the
// deletion — which has broken the clean-clone build TWICE.
//
// The cause is fixed in vite.config.ts (a plugin rewrites the file after each bundle);
// this is the guard that catches it if that plugin is ever removed. It asserts the file
// is TRACKED BY GIT, not merely present on disk: present-but-untracked is exactly the
// state that compiles here and fails on a fresh checkout.
func TestGitkeepIsTracked(t *testing.T) {
	out, err := exec.Command("git", "ls-files", "dist/.gitkeep").Output()
	if err != nil {
		t.Skipf("git unavailable: %v", err) // a source tarball has no git; not a failure
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Fatal("internal/web/dist/.gitkeep is not tracked — `go:embed all:dist` will not " +
			"compile on a clean clone. Vite's emptyOutDir removed it; `git add -f " +
			"internal/web/dist/.gitkeep` and check vite.config.ts's keepEmbedDir plugin.")
	}
}

// The embedded FS must always carry at least one entry, which is what makes the embed
// directive legal.
func TestEmbedHasContent(t *testing.T) {
	entries, err := fs.ReadDir(distFS, "dist")
	if err != nil {
		t.Fatalf("dist is not embedded: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("dist embedded empty")
	}
}

func TestHandlerNegotiatesCompressedAssets(t *testing.T) {
	const javascript = `console.log("Loomarr");` +
		`console.log("The repeated source makes compression useful.");` +
		`console.log("The repeated source makes compression useful.");` +
		`console.log("The repeated source makes compression useful.");`
	assets := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<h1>Loomarr</h1>")},
		"assets/app.js": &fstest.MapFile{Data: []byte(javascript)},
	}
	handler := handlerFor(assets)

	request := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	request.Header.Set("Accept-Encoding", "br, gzip")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := response.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
	if got := response.Header().Get("Content-Length"); got == "" {
		t.Fatal("Content-Length is empty for the precompressed representation")
	}
	if got := response.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}

	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatalf("read gzip response: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("decompress response: %v", err)
	}
	if got := string(decoded); got != javascript {
		t.Fatalf("decoded body = %q, want %q", got, javascript)
	}
}

func TestHandlerFallsBackToIdentityEncoding(t *testing.T) {
	const javascript = `console.log("Loomarr");`
	assets := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<h1>Loomarr</h1>")},
		"assets/app.js": &fstest.MapFile{Data: []byte(javascript)},
	}
	handler := handlerFor(assets)

	for _, acceptEncoding := range []string{"", "gzip;q=0, br"} {
		request := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
		request.Header.Set("Accept-Encoding", acceptEncoding)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if got := response.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Accept-Encoding %q: Content-Encoding = %q, want identity", acceptEncoding, got)
		}
		if got := response.Body.String(); got != javascript {
			t.Fatalf("Accept-Encoding %q: body = %q, want %q", acceptEncoding, got, javascript)
		}
	}
}

func TestHandlerCompressesSPAFallbackWithoutCachingIt(t *testing.T) {
	const index = "<!doctype html><h1>Loomarr</h1>"
	handler := handlerFor(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(index)},
	})
	request := httptest.NewRequest(http.MethodGet, "/guide", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if got := response.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
}
