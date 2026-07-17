// Package web embeds the built SPA and serves it same-origin at / (main doc §12).
// The Vite build (web/apps/web) outputs into ./dist, which `make fe` populates;
// only the .gitkeep placeholder is committed, so `//go:embed all:dist` compiles
// even on a Go-only checkout — then Handler serves a "not built" notice instead
// of the app.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded SPA: real files (hashed assets, favicon) are served
// directly with long-cache headers; every other path falls back to index.html so
// react-router owns client-side routing. If only the placeholder is embedded
// (frontend not built), it serves a friendly notice.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return notBuilt()
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return notBuilt() // only .gitkeep present — `make fe` hasn't run
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean != "" {
			if f, err := sub.Open(clean); err == nil {
				_ = f.Close()
				if strings.HasPrefix(clean, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback: unknown path → index.html (client routing), no-store so a
		// deploy's new asset hashes are always picked up.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(index)
	})
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
