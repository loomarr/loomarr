package reference

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Discoverer resolves a public programming-block label, never a household Intent.
// Empty evidence means no unambiguous source; errors mean retrieval failed.
type Discoverer interface {
	Discover(context.Context, string) (Evidence, error)
}

const discoveryAPI = "https://en.wikipedia.org/w/api.php"

// Discover finds a single matching programming block and reads its named category.
// Search results locate a source; only explicit category members supply anchors.
func (w *Web) Discover(ctx context.Context, label string) (Evidence, error) {
	label = strings.Join(strings.Fields(label), " ")
	if !validBlockLabel(label) {
		return Evidence{}, errors.New("reference: invalid public block label")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var search struct {
		Query struct {
			Search []struct {
				PageID  int64  `json:"pageid"`
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := w.discoveryJSON(ctx, url.Values{
		"action": {"query"}, "list": {"search"}, "srsearch": {strconv.Quote(label)},
		"srnamespace": {"0"}, "srlimit": {"5"}, "srprop": {"snippet"},
	}, &search); err != nil {
		return Evidence{}, err
	}
	if len(search.Query.Search) > 5 {
		return Evidence{}, errors.New("reference: too many discovery results")
	}
	var pageID int64
	var pageTitle string
	for _, hit := range search.Query.Search {
		description := strings.ToLower(cleanFragment(hit.Snippet))
		if hit.PageID <= 0 || !sameBlockLabel(hit.Title, label) ||
			!strings.Contains(description, "programming block") || strings.Contains(description, "may refer to") {
			continue
		}
		if pageID != 0 && pageID != hit.PageID {
			return Evidence{}, nil
		}
		pageID, pageTitle = hit.PageID, hit.Title
	}
	if pageID == 0 {
		return Evidence{}, nil
	}
	var category struct {
		Query struct {
			Members []struct {
				PageID int64  `json:"pageid"`
				Title  string `json:"title"`
			} `json:"categorymembers"`
		} `json:"query"`
	}
	categoryTitle := "Category:" + pageTitle
	if err := w.discoveryJSON(ctx, url.Values{
		"action": {"query"}, "list": {"categorymembers"}, "cmtitle": {categoryTitle},
		"cmnamespace": {"0"}, "cmlimit": {"128"},
	}, &category); err != nil {
		return Evidence{}, err
	}
	if len(category.Query.Members) > MaxTitleAnchors {
		return Evidence{}, errors.New("reference: too many category members")
	}
	// The discovered subject must itself belong to this exact category. A title
	// guess or another similarly named category cannot supply membership evidence.
	containsSubject := false
	var titles []positionedAnchor
	for i, member := range category.Query.Members {
		if member.PageID == pageID && member.Title == pageTitle {
			containsSubject = true
			continue
		}
		if member.PageID <= 0 {
			return Evidence{}, errors.New("reference: invalid category member identity")
		}
		title := displayMemberTitle(member.Title)
		if title != "" && len(title) <= 120 {
			titles = append(titles, positionedAnchor{position: i, value: title})
		}
	}
	anchors := dedupeAnchors(titles)
	if !containsSubject || len(anchors) == 0 {
		return Evidence{}, nil
	}
	source := &url.URL{Scheme: "https", Host: "en.wikipedia.org", Path: "/wiki/" + strings.ReplaceAll(categoryTitle, " ", "_")}
	return Evidence{URL: source.String(), Title: pageTitle, Excerpt: "Source category members observed for this request: " + strings.Join(anchors, "; "), TitleAnchors: anchors}, nil
}

// Wikipedia's parenthetical media disambiguators are not display-title words.
// Removing one cannot choose a Catalog year or identity; that ambiguity remains
// the caller's existing unfiltered exact-title check.
func displayMemberTitle(title string) string {
	name, suffix, found := strings.Cut(title, " (")
	if found && strings.HasSuffix(suffix, ")") && (strings.Contains(suffix, "TV series") || strings.Contains(suffix, "television series")) {
		return name
	}
	return title
}

func (w *Web) discoveryJSON(ctx context.Context, query url.Values, out any) error {
	query.Set("format", "json")
	u, _ := url.Parse(discoveryAPI)
	u.RawQuery = query.Encode()
	body, mediaType, err := w.read(ctx, u, "application/json", true)
	if err != nil {
		return err
	}
	if mediaType != "application/json" {
		return errors.New("reference: discovery response is not JSON")
	}
	var status struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return fmt.Errorf("reference: invalid discovery JSON: %w", err)
	}
	if len(status.Error) != 0 {
		return errors.New("reference: source discovery API failed")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("reference: invalid discovery data: %w", err)
	}
	return nil
}

func validBlockLabel(label string) bool {
	if label == "" || len(label) > 120 {
		return false
	}
	for _, r := range label {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(" &'-", r) {
			return false
		}
	}
	return true
}

func sameBlockLabel(title, label string) bool {
	title, _, _ = strings.Cut(title, " (")
	normalize := func(s string) string { return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "the ") }
	return normalize(title) == normalize(label)
}
