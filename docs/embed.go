// Package docs embeds the user-facing help pages and serves them to the in-app Help
// section (design §13: "docs live as markdown in docs/ ... embedded and rendered as an
// in-app Help section (same embed.FS mechanism as the SPA) — works air-gapped").
//
// WHY A GO FILE LIVES IN docs/: //go:embed cannot reference paths outside its own
// package directory, so the embed must sit beside the markdown. The alternative — moving
// the pages under internal/ — would contradict §13's statement that docs live in docs/,
// and would separate the operator-facing pages from the rest of the documentation set.
// Only help/ is embedded; the design docs beside it are internal and deliberately not
// shipped to users.
package docs

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed help/*.md
var helpFS embed.FS

// Page is one help document.
type Page struct {
	// Slug is the URL-facing id ("troubleshooting"). It is the first half of the
	// `docHref` values the API emits on setup checks, e.g. "troubleshooting#tunarr".
	Slug string
	// Title is the page's first H1, falling back to the slug.
	Title string
	// Markdown is the raw source. Rendering is the frontend's job — the backend ships
	// markdown so Help can be searched client-side (§7.2) without a render round-trip.
	Markdown string
}

// Pages returns every embedded help page, ordered by slug so the Help nav and its
// manifest are stable across builds.
func Pages() []Page {
	entries, err := fs.ReadDir(helpFS, "help")
	if err != nil {
		return nil
	}
	out := make([]Page, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		body, err := fs.ReadFile(helpFS, "help/"+e.Name())
		if err != nil {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		out = append(out, Page{Slug: slug, Title: titleOf(string(body), slug), Markdown: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// Get returns one page by slug.
func Get(slug string) (Page, bool) {
	for _, p := range Pages() {
		if p.Slug == slug {
			return p, true
		}
	}
	return Page{}, false
}

// titleOf reads the first H1 as the page title. A page without one falls back to its
// slug rather than rendering blank in the nav.
func titleOf(markdown, slug string) string {
	for _, line := range strings.Split(markdown, "\n") {
		if t, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(t)
		}
	}
	return slug
}

// Anchor converts a markdown heading to its GitHub-style fragment id — lowercased,
// non-alphanumerics collapsed to hyphens. This is the same transform the frontend uses to
// build heading ids, so a `docHref` fragment emitted by the API resolves to a real anchor.
// Kept here, next to the content, because the anchors ARE a contract: renaming a heading
// silently breaks a deep-link the backend is already sending.
func Anchor(heading string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(heading) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastHyphen = false
		case r == ' ' || r == '-' || r == '_':
			if !lastHyphen && b.Len() > 0 {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// Anchors returns every heading anchor in a page, so a test can prove that every
// `docHref` the API emits actually lands somewhere.
func Anchors(markdown string) []string {
	var out []string
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		heading := strings.TrimLeft(trimmed, "#")
		if heading == trimmed {
			continue
		}
		out = append(out, Anchor(strings.TrimSpace(heading)))
	}
	return out
}
