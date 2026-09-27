// Package docs embeds the user-facing help pages and serves them to the in-app Help
// section (design §13: "docs live as markdown in docs/ ... embedded and rendered as an
// in-app Help section (same embed.FS mechanism as the SPA) — works air-gapped").
//
// WHY A GO FILE LIVES IN docs/: //go:embed cannot reference paths outside its own
// package directory, so the embed must sit beside the markdown. The alternative — moving
// the pages under internal/ — would contradict §13's statement that docs live in docs/,
// and would separate the operator-facing pages from the rest of the documentation set.
// Only the household pages are embedded (Get started, Guides, Explanation); reference,
// contributing and design docs beside them are for the site and the repo, not the app.
package docs

import (
	"embed"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed get-started.md guides/*.md explanation/*.md
var helpFS embed.FS

// helpDirs are the embedded folders, "." being docs/ itself (get-started.md).
var helpDirs = []string{".", "guides", "explanation"}

// movedPages maps a slug from before #1572's restructure to the page that now holds its
// content. An older build, a bookmark or a support answer can still carry the old one.
var movedPages = map[string]string{
	"quickstart":   "get-started",
	"concepts":     "how-loomarr-works",
	"programming":  "curation",
	"member-guide": "add-a-channel",
	"integrations": "connect-services",
}

// movedSections maps an old "slug#anchor" whose heading was renamed or moved to another
// page. Pages keep their headings where they can; TestPreMoveHelpLinksStillLand fails for
// every pre-move link that lands nowhere, and the fix is an entry here.
var movedSections = map[string]string{
	"quickstart#1-start-it":       "get-started#1-start-loomarr",
	"quickstart#2-run-the-wizard": "get-started#2-run-the-setup-wizard",
	"quickstart#3-make-a-channel": "get-started#3-describe-your-first-channel",
	"quickstart#upgrading":        "upgrade",
}

// Page is one help document.
type Page struct {
	// Slug is the URL-facing id ("troubleshooting"): the file's base name, whichever
	// folder it sits in. It is the first half of the `docHref` values the API emits on
	// setup checks, e.g. "troubleshooting#tunarr".
	Slug string
	// Path is the file under docs/ ("guides/troubleshooting.md"), for messages.
	Path string
	// Title is the page's first H1, falling back to the slug.
	Title string
	// Markdown is the raw source. Rendering is the frontend's job — the backend ships
	// markdown so Help can be searched client-side (§7.2) without a render round-trip.
	Markdown string
}

// Pages returns every embedded help page, ordered by slug so the Help nav and its
// manifest are stable across builds.
func Pages() []Page {
	var out []Page
	for _, dir := range helpDirs {
		entries, err := fs.ReadDir(helpFS, dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			file := path.Join(dir, e.Name())
			body, err := fs.ReadFile(helpFS, file)
			if err != nil {
				continue
			}
			slug := strings.TrimSuffix(e.Name(), ".md")
			out = append(out, Page{Slug: slug, Path: file, Title: titleOf(string(body), slug), Markdown: string(body)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// Get returns one page by slug, following movedPages so a pre-restructure slug still
// opens the page that now holds its content.
func Get(slug string) (Page, bool) {
	if moved, ok := movedPages[slug]; ok {
		slug = moved
	}
	for _, p := range Pages() {
		if p.Slug == slug {
			return p, true
		}
	}
	return Page{}, false
}

// Resolve maps a help href ("concepts#who-does-what") to where that content lives now
// ("how-loomarr-works#who-does-what"). An href that never moved comes back unchanged.
func Resolve(href string) string {
	if moved, ok := movedSections[href]; ok {
		return moved
	}
	slug, fragment, hasFragment := strings.Cut(href, "#")
	if moved, ok := movedPages[slug]; ok {
		slug = moved
	}
	if !hasFragment {
		return slug
	}
	return slug + "#" + fragment
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
