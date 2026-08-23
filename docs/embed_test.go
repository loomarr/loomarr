package docs_test

import (
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
