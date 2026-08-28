// Package web embeds the built SPA and serves it same-origin at / (main doc §12).
// The Vite build (web/apps/web) outputs into ./dist, which `make fe` populates;
// only the .gitkeep placeholder is committed, so `//go:embed all:dist` compiles
// even on a Go-only checkout — then Handler serves a "not built" notice instead
// of the app.
package web

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed all:dist
var distFS embed.FS

type compressedCache struct {
	sync.Once
	compress func(fs.FS) map[string][]byte
	files    map[string][]byte
}

var embeddedCompressed compressedCache

// Handler serves the embedded SPA: real files (hashed assets, favicon) are served
// directly with long-cache headers; every other path falls back to index.html so
// react-router owns client-side routing. If only the placeholder is embedded
// (frontend not built), it serves a friendly notice.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return notBuilt()
	}
	return handlerForCached(sub, &embeddedCompressed)
}

func handlerFor(sub fs.FS) http.Handler {
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return notBuilt() // only .gitkeep present — `make fe` hasn't run
	}
	return handlerForWithIndex(sub, index, precompress(sub))
}

func (c *compressedCache) get(files fs.FS) map[string][]byte {
	c.Do(func() {
		compress := c.compress
		if compress == nil {
			compress = precompress
		}
		c.files = compress(files)
	})
	return c.files
}

func handlerForCached(sub fs.FS, cache *compressedCache) http.Handler {
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return notBuilt() // only .gitkeep present — `make fe` hasn't run
	}
	return handlerForWithIndex(sub, index, cache.get(sub))
}

func handlerForWithIndex(sub fs.FS, index []byte, compressed map[string][]byte) http.Handler {
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean != "" && clean != "index.html" {
			if f, err := sub.Open(clean); err == nil {
				_ = f.Close()
				if strings.HasPrefix(clean, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				if body, ok := compressed[clean]; ok {
					w.Header().Set("Vary", "Accept-Encoding")
					if acceptsGzip(r.Header.Get("Accept-Encoding")) {
						serveCompressed(w, r, clean, body)
						return
					}
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback: unknown path → index.html (client routing), no-store so a
		// deploy's new asset hashes are always picked up.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Vary", "Accept-Encoding")
		if acceptsGzip(r.Header.Get("Accept-Encoding")) {
			serveCompressed(w, r, "index.html", compressed["index.html"])
			return
		}
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
	})
}

func precompress(files fs.FS) map[string][]byte {
	compressed := make(map[string][]byte)
	_ = fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !compressible(name) {
			return nil
		}
		body, err := fs.ReadFile(files, name)
		if err != nil {
			return nil
		}
		var result bytes.Buffer
		writer := gzip.NewWriter(&result)
		if _, err := writer.Write(body); err != nil {
			return nil
		}
		if err := writer.Close(); err != nil {
			return nil
		}
		compressed[name] = result.Bytes()
		return nil
	})
	return compressed
}

func compressible(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".css", ".html", ".js", ".json", ".map", ".mjs", ".svg", ".txt", ".wasm", ".webmanifest", ".xml":
		return true
	default:
		return false
	}
}

func acceptsGzip(header string) bool {
	wildcard := false
	for _, value := range strings.Split(header, ",") {
		parts := strings.Split(value, ";")
		encoding := strings.ToLower(strings.TrimSpace(parts[0]))
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, value, found := strings.Cut(strings.TrimSpace(parameter), "=")
			if found && strings.EqualFold(key, "q") {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					quality = 0
				} else {
					quality = parsed
				}
			}
		}
		if encoding == "gzip" {
			return quality > 0
		}
		if encoding == "*" {
			wildcard = quality > 0
		}
	}
	return wildcard
}

func serveCompressed(w http.ResponseWriter, r *http.Request, name string, body []byte) {
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
}

func notBuilt() http.Handler {
	const page = `<!doctype html><meta charset="utf-8"><title>Loomarr</title>` +
		`<body style="font-family:ui-monospace,monospace;background:#0B0C0E;color:#E7EAF0;padding:2rem">` +
		`<h1>Loomarr</h1><p>The frontend isn't built yet. Run <code>make fe</code>, then reload.</p></body>`
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})
}
