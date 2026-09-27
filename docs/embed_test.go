package docs_test

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/docs"
)

// docHrefs is every deep-link the API emits on a setup check (internal/api/setup.go).
// It is duplicated here deliberately rather than imported: this test exists to prove the
// two sides AGREE, and importing the source of truth would make it agree with itself.
// Adding a check without a target here is the failure this catches.
var docHrefs = []string{
	"troubleshooting#media-server",
	"troubleshooting#seerr",
	"troubleshooting#tunarr",
	"troubleshooting#llm",
	"troubleshooting#tmdb",
	"troubleshooting#filler",
	"troubleshooting#livetv",
	"troubleshooting#tunarr-library",
}

// §13: "every red check in the wizard deep-links to its section here." The backend has
// been emitting these anchors since phase 8 with nothing to receive them. A dangling
// deep-link is worse than none — it promises help and delivers a blank page, at the exact
// moment the operator is already stuck.
func TestEveryDocHrefResolves(t *testing.T) {
	for _, href := range docHrefs {
		slug, fragment, ok := strings.Cut(href, "#")
		if !ok {
			t.Errorf("docHref %q has no fragment", href)
			continue
		}
		page, found := docs.Get(slug)
		if !found {
			t.Errorf("docHref %q points at page %q, which is not embedded", href, slug)
			continue
		}
		if anchors := docs.Anchors(page.Markdown); !slices.Contains(anchors, fragment) {
			t.Errorf("docHref %q: page %q has no heading anchoring to %q.\n  available: %v",
				href, slug, fragment, anchors)
		}
	}
}

// #1572 moved the help pages out of docs/help/ into the published tree (get-started.md,
// guides/, explanation/) and renamed five of them. Every deep-link that existed before the
// move is frozen in testdata: a bookmark, an older build's docHref or a support answer can
// still carry one, and it must land on the page and heading that now hold that content.
func TestPreMoveHelpLinksStillLand(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "help-anchors-v0.2.0-beta.7.txt"))
	if err != nil {
		t.Fatalf("read frozen links: %v", err)
	}
	var checked int
	for _, line := range strings.Split(string(raw), "\n") {
		old := strings.TrimSpace(line)
		if old == "" || strings.HasPrefix(old, "#") {
			continue
		}
		checked++
		slug, fragment, _ := strings.Cut(docs.Resolve(old), "#")
		page, found := docs.Get(slug)
		if !found {
			t.Errorf("pre-move link %q resolves to page %q, which is not embedded", old, slug)
			continue
		}
		if fragment != "" && !slices.Contains(docs.Anchors(page.Markdown), fragment) {
			t.Errorf("pre-move link %q resolves to %q#%q, which has no such heading; "+
				"add it to movedSections in embed.go", old, slug, fragment)
		}
	}
	if checked < 50 {
		t.Fatalf("only %d frozen links read; the fixture or its parser broke", checked)
	}
}

// The in-app viewer routes every relative link to a help slug (parseDocHref), so a relative
// link from an embedded page to anything that is not embedded (the settings reference, a
// compose file) opens "Help page not found". Those links must be absolute URLs.
func TestEmbeddedPagesLinkOnlyToEmbeddedPages(t *testing.T) {
	for _, p := range docs.Pages() {
		for _, m := range mdTarget.FindAllStringSubmatch(p.Markdown, -1) {
			if strings.HasPrefix(m[1], "!") {
				continue // an image: TestEmbeddedPagesShowOnlyEmbeddedDiagrams checks those
			}
			href := m[2]
			if strings.Contains(href, "://") || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") {
				continue
			}
			target, _, _ := strings.Cut(href, "#")
			resolved := path.Join(path.Dir(p.Path), target)
			slug := strings.TrimSuffix(path.Base(target), ".md")
			page, found := docs.Get(slug)
			if !strings.HasSuffix(target, ".md") || !found || page.Path != resolved {
				t.Errorf("docs/%s links to %q, which the app cannot open; link an embedded page "+
					"by its relative .md path, or anything else by its absolute URL", p.Path, href)
			}
		}
	}
}

// mdTarget matches a Markdown link or image target. Group 1 starts with "!" for an image;
// group 2 is the target.
var mdTarget = regexp.MustCompile(`(!\[[^\]]*\]|\])\(([^)\s]+)\)`)

// Help shows a page's diagrams from the binary (GET /v1/docs/diagrams/{name}) so it works
// air-gapped, and it shows nothing else: a remote image would call out of the household's
// network. So every image on an embedded page must be a generated diagram that is embedded
// too, or Help shows a broken image.
func TestEmbeddedPagesShowOnlyEmbeddedDiagrams(t *testing.T) {
	for _, p := range docs.Pages() {
		for _, m := range mdTarget.FindAllStringSubmatch(p.Markdown, -1) {
			if !strings.HasPrefix(m[1], "!") {
				continue
			}
			src := m[2]
			name, isDiagram := strings.CutPrefix(path.Join(path.Dir(p.Path), src), "diagrams/generated/")
			if !isDiagram || strings.Contains(name, "/") || path.Ext(name) != ".svg" {
				t.Errorf("docs/%s shows image %q; Help can show only diagrams from "+
					"docs/diagrams/generated/, by their relative path", p.Path, src)
				continue
			}
			if _, found := docs.Diagram(name); !found {
				t.Errorf("docs/%s shows diagram %q, which is not embedded; run make diagrams", p.Path, src)
			}
		}
	}
}

// Diagram is the only file the diagram route reads, and the name comes from the URL.
func TestDiagramServesOnlyGeneratedSVGs(t *testing.T) {
	if _, found := docs.Diagram("ci.svg"); !found {
		t.Fatal(`Diagram("ci.svg") not found; the generated diagrams are not embedded`)
	}
	for _, name := range []string{"", ".", "..", "../get-started.md", "generated/ci.svg", "ci.d2", "ci", "/ci.svg"} {
		if _, found := docs.Diagram(name); found {
			t.Errorf("Diagram(%q) returned a file; only a generated diagram's base name may", name)
		}
	}
}

// Every page opens by naming its reader and its outcome (docs/contributing/docs.md), so a
// household admin landing from a red setup check knows at once whether it is their page.
func TestEveryHelpPageOpensWithForAndYoullGet(t *testing.T) {
	for _, p := range docs.Pages() {
		var lines []string
		for _, line := range strings.Split(p.Markdown, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "# ") {
				lines = append(lines, trimmed)
			}
			if len(lines) == 2 {
				break
			}
		}
		if len(lines) < 2 || !strings.HasPrefix(lines[0], "**For:**") || !strings.HasPrefix(lines[1], "**You'll get:**") {
			t.Errorf("docs/%s must open with a **For:** line and a **You'll get:** line after its title", p.Path)
		}
	}
}

// A slug is a file's base name, whichever folder it sits in, so two pages with one name
// would make one of them unreachable in the app.
func TestHelpSlugsAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, p := range docs.Pages() {
		if prev, dup := seen[p.Slug]; dup {
			t.Errorf("slug %q is used by both %s and %s", p.Slug, prev, p.Path)
		}
		seen[p.Slug] = p.Path
	}
}

func TestPagesAreEmbeddedWithTitles(t *testing.T) {
	pages := docs.Pages()
	if len(pages) == 0 {
		t.Fatal("no help pages embedded — the Help section would render empty")
	}
	for _, p := range pages {
		if p.Slug == "" {
			t.Error("page with an empty slug is unaddressable")
		}
		if p.Title == p.Slug {
			t.Errorf("page %q has no H1, so the Help nav would show its raw slug", p.Slug)
		}
		if strings.TrimSpace(p.Markdown) == "" {
			t.Errorf("page %q is empty", p.Slug)
		}
	}
}

// Only user-facing pages ship. The design docs sit in the same directory and are
// internal: embedding design.md would put the project's own architecture notes, including
// its open questions, in front of every operator.
func TestOnlyHelpPagesAreEmbedded(t *testing.T) {
	for _, p := range docs.Pages() {
		for _, internal := range []string{"design", "config-design", "programming-design", "configuration", "frontend-build-plan", "README"} {
			if p.Slug == internal {
				t.Errorf("internal doc %q is embedded in the user-facing Help set", p.Slug)
			}
		}
	}
}

func TestAnchorMatchesGitHubStyleSlugs(t *testing.T) {
	cases := map[string]string{
		"Media server":     "media-server",
		"Tunarr library":   "tunarr-library",
		"LiveTV":           "livetv",
		"LLM":              "llm",
		"Seerr":            "seerr",
		"Webhooks":         "webhooks",
		"Trailing dash — ": "trailing-dash",
	}
	for heading, want := range cases {
		if got := docs.Anchor(heading); got != want {
			t.Errorf("Anchor(%q) = %q, want %q", heading, got, want)
		}
	}
}
