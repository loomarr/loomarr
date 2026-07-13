package api

import "net/http"

// docsHandler serves interactive API docs at /docs with NO external assets
// (§7.1 offline rule — Huma's default Stoplight page loads from a CDN, which
// breaks air-gapped LAN installs). This is a minimal self-contained page that
// links to the machine-readable spec; the full Vite SPA (Phase 13) embeds a
// richer viewer. Everything here is inline — no CDN, no fonts, no scripts.
func docsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(docsHTML))
}

const docsHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Loomarr API</title>
<style>
  body{font:16px/1.5 system-ui,sans-serif;max-width:48rem;margin:3rem auto;padding:0 1rem;color:#111}
  code{background:#f2f2f2;padding:.1em .3em;border-radius:3px}
  a{color:#0645ad}
  @media(prefers-color-scheme:dark){body{background:#111;color:#eee}code{background:#222}a{color:#6ea8fe}}
</style></head>
<body>
<h1>Loomarr API</h1>
<p>Code-first OpenAPI 3.1, served offline (§7.1). The machine-readable spec:</p>
<ul>
  <li><a href="/openapi.json">/openapi.json</a></li>
  <li><a href="/openapi.yaml">/openapi.yaml</a></li>
</ul>
<p>Every <code>/v1</code> route requires a session cookie or
<code>Authorization: Bearer &lt;API_TOKEN&gt;</code>. The in-app Help (embedded SPA)
renders the full interactive reference.</p>
</body></html>`
