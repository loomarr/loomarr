package clipfetch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// Discovery: listing what a remote collection HOLDS, without downloading any of it (§10, V33).
//
// ⚠ **The distinction from the walk is the whole point.** `walk` resolves a source and fetches
// its video files; this asks one search question and returns rows. An operator browsing
// archive.org to decide whether a collection is worth having must not trigger a multi-gigabyte
// download to find out — and the walk cannot be reused with a "don't save" flag, because its
// per-item metadata fetch is what makes it expensive in the first place.
//
// One request, whatever the collection's size. The search endpoint returns title and licence
// alongside the identifier, so a listing needs no per-item call at all.

// DiscoveredItem is one item a collection listing found.
type DiscoveredItem struct {
	// ID is the archive.org identifier — the thing `Download` would be given.
	ID string
	// Title is Archive's own; may be empty, in which case a caller shows the id.
	Title string
	// License is the declared licence URL. ⚠ EMPTY MEANS UNKNOWN, never "public domain":
	// ~92% of items declare none, so absence is the common case and carries no permission.
	License string
	// Year is Archive's catalogued year; 0 when the item declares none, which is common. A
	// WEAK hint only — never used to set a clip's era (see searchDoc.Year).
	Year int
	// Date is Archive's catalogued date (RFC3339), empty when it declares none. Carries the
	// same weak-hint caveat as Year — see searchDoc.Date.
	Date string
	// DurationMS is the item's runtime, 0 when unknown. ⚠ Zero means UNKNOWN, not "zero
	// seconds": it is unset until the per-item enrichment pass runs (see enrich), and an item
	// whose files Archive never probed keeps it. A caller renders 0 as "—", never as "0:00".
	DurationMS int
	// Height is the best derivative's vertical resolution, 0 when unknown — the quality hint
	// the Sources search row renders as "480p". Same zero-means-unknown rule as DurationMS.
	Height int
}

// DiscoveryResult is a page of a collection's contents.
type DiscoveryResult struct {
	Items []DiscoveredItem
	// Total is the collection's FULL size, not len(Items). Reporting len(Items) would tell an
	// operator that an 8362-item collection holds 5 — the number they use to decide whether to
	// add it is the one they must be shown.
	Total int
}

// maxDiscoverRows caps one listing page.
//
// A listing is for DECIDING, not for browsing an entire collection: an operator looks at a
// handful of titles to judge whether a source is the right sort of thing, then adds it and lets
// the walk do the rest. Fetching thousands of rows to render a wall nobody reads costs the
// operator latency and archive.org bandwidth for no decision-making value.
const maxDiscoverRows = 25

// DiscoverCollection lists what a collection holds, without downloading anything.
//
// `ref` may be a full archive.org URL, a `/details/<id>` path, or a bare identifier — the same
// spellings `Download` accepts, because an operator pasting a URL should not have to know which
// form a given field wants.
func (d *ArchiveDownloader) DiscoverCollection(ctx context.Context, ref string, limit int) (DiscoveryResult, error) {
	return d.client.discover(ctx, ref, limit)
}

func (c *archiveClient) discover(ctx context.Context, ref string, limit int) (DiscoveryResult, error) {
	id := archiveIDFromURL(ref)
	if id == "" {
		return DiscoveryResult{}, fmt.Errorf("archive discover: %q is not an archive.org collection or item", ref)
	}
	if limit <= 0 || limit > maxDiscoverRows {
		limit = maxDiscoverRows
	}

	q := url.Values{}
	// ⚠ The identifier is a VALUE inside the query, so it is quoted and escaped rather than
	// concatenated: a collection id containing a space or a colon would otherwise change the
	// meaning of the Solr query rather than being searched for.
	q.Set("q", `collection:"`+strings.ReplaceAll(id, `"`, `\"`)+`"`)
	// Ask for everything a listing renders in ONE request. The download walk asks for
	// `identifier` alone because it fetches each item's metadata anyway; a listing that did
	// that would make N+1 requests to show N rows.
	q["fl[]"] = []string{"identifier", "title", "licenseurl", "year", "date"}
	q.Set("rows", strconv.Itoa(limit))
	q.Set("output", "json")

	var out searchResp
	if err := c.getJSON(ctx, c.base+"/advancedsearch.php?"+q.Encode(), &out); err != nil {
		return DiscoveryResult{}, fmt.Errorf("archive discover %s: %w", id, err)
	}

	res := toResult(out)
	c.enrich(ctx, res.Items)
	return res, nil
}

// toResult maps a Solr response to the result both discovery paths return. Shared so a
// collection listing and a keyword search cannot drift in how they read the same wire shape.
func toResult(out searchResp) DiscoveryResult {
	res := DiscoveryResult{
		// Non-nil even when empty: a JSON `null` would make every consumer guard before
		// iterating, and "nothing found" is a real answer, not a missing one.
		Items: make([]DiscoveredItem, 0, len(out.Response.Docs)),
		Total: out.Response.NumFound,
	}
	for _, doc := range out.Response.Docs {
		if doc.Identifier == "" {
			continue // a doc with no id is not actionable — nothing could download it
		}
		res.Items = append(res.Items, DiscoveredItem{
			ID:      doc.Identifier,
			Title:   strings.TrimSpace(doc.Title),
			License: strings.TrimSpace(doc.LicenseURL),
			Year:    doc.Year,
			Date:    strings.TrimSpace(doc.Date),
		})
	}
	return res
}

// enrichConcurrency caps the per-item metadata fan-out.
//
// ⚠ This is the cost the one-request design deliberately avoided (see the file header), taken
// on knowingly: the mock's search row draws `date · duration · quality`, and Solr indexes none
// of duration or quality at item level — measured, not assumed. So a listing that shows them
// must ask archive.org once per row.
//
// 6 is chosen against archive.org's shared infrastructure rather than our own capacity: the
// rows are capped at 25 (maxDiscoverRows), so a search costs at most 25 requests in bursts of
// 6, and a person clicking search repeatedly cannot open 25 sockets per keystroke.
const enrichConcurrency = 6

// enrich fills DurationMS + Height by fetching each item's metadata.
//
// ⚠ **Every failure is non-fatal, per item.** A row whose metadata call fails keeps its search
// fields and renders duration/quality as unknown — losing the whole listing because one item of
// 25 timed out would be a far worse trade than an incomplete row. This mirrors the guide's
// per-channel posture (`api/guide.go`), and it is why the function returns nothing: there is no
// error a caller could act on that is not better handled by showing the rows.
//
// The context is honoured, so a cancelled request stops the fan-out rather than finishing 25
// calls nobody is waiting for.
func (c *archiveClient) enrich(ctx context.Context, items []DiscoveredItem) {
	// Write BY INDEX, never append: the listing's order is Solr's relevance/date ranking, and
	// an append-as-you-finish would reorder results by whichever item's metadata returned first.
	sem := make(chan struct{}, enrichConcurrency)
	var wg sync.WaitGroup

	for i := range items {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// ⚠ This select is an OPTIMISATION, not the cancellation guarantee. `metadata` →
			// `getJSON` builds its request with http.NewRequestWithContext, which refuses a
			// dead context on its own — verified by sabotage: deleting this select changes
			// nothing observable. What it saves is the queued goroutines behind the semaphore
			// (up to 19 of 25 rows) each opening a request that would only be refused.
			//
			// So do not "simplify" this into a bare `sem <- struct{}{}` on the grounds that a
			// test still passes; the test that covers it is the one detaching the request
			// context, and it pins the transport's behaviour rather than this line's.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			meta, err := c.metadata(ctx, items[i].ID)
			if err != nil {
				return // unknown duration/quality; the row still renders
			}
			items[i].DurationMS, items[i].Height = bestVideoStats(meta.Files)
		}(i)
	}
	wg.Wait()
}

// bestVideoStats picks the runtime + resolution a listing row should show.
//
// ⚠ Duration and height are taken from DIFFERENT files on purpose. Every derivative of one item
// has the same runtime (they are re-encodes of one source), so the first parseable length is
// authoritative — but they differ in resolution, and the honest quality answer is the BEST
// available rather than whichever file happened to be listed first. The pinned fixture has
// exactly this shape: a 480p derivative beside a 960p original, both 91 seconds.
func bestVideoStats(files []archiveFile) (durationMS, height int) {
	for _, f := range files {
		if ms := parseLengthMS(f.Length); ms > 0 && durationMS == 0 {
			durationMS = ms
		}
		if h, err := strconv.Atoi(strings.TrimSpace(f.Height)); err == nil && h > height {
			height = h
		}
	}
	return durationMS, height
}

// parseLengthMS reads archive.org's `length` field, which arrives in three real spellings:
// seconds with a decimal ("91.09"), seconds without one ("660"), and "MM:SS" / "HH:MM:SS".
//
// ⚠ Measured, not assumed: 36 video files across 5 items were all seconds-as-string, but the
// colon form is documented behaviour on some audio derivatives, so it is parsed rather than
// left to silently truncate to 0 the first time one appears in a movies collection.
//
// Returns 0 for anything unparseable, which every caller must treat as UNKNOWN — never as a
// zero-length clip.
func parseLengthMS(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if strings.Contains(raw, ":") {
		var total float64
		for _, part := range strings.Split(raw, ":") {
			n, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
			if err != nil {
				return 0 // a malformed segment makes the whole value untrustworthy
			}
			total = total*60 + n
		}
		return int(total * 1000)
	}
	secs, err := strconv.ParseFloat(raw, 64)
	if err != nil || secs < 0 {
		return 0
	}
	return int(secs * 1000)
}

// Search finds candidate clips across ALL of archive.org by keyword, rather than listing one
// named collection (V33, maintainer decision 2026-07-31).
//
// ⚠ The distinction from DiscoverCollection matters at the API layer: this returns ITEMS an
// operator might want, from anywhere, so each result is a candidate clip rather than a source to
// register. `collection:` is what you browse; this is what you go looking for.
//
// Scoped to `mediatype:movies` because filler is video. Without it the same query returns texts,
// audio and software — real results for the words, useless for a clip catalog.
func (d *ArchiveDownloader) Search(ctx context.Context, query string, limit int) (DiscoveryResult, error) {
	return d.client.search(ctx, query, limit)
}

func (c *archiveClient) search(ctx context.Context, query string, limit int) (DiscoveryResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return DiscoveryResult{}, fmt.Errorf("archive search: empty query")
	}
	if limit <= 0 || limit > maxDiscoverRows {
		limit = maxDiscoverRows
	}

	q := url.Values{}
	// ⚠ The user's words are PARENTHESISED, not quoted. Quoting would force an exact-phrase
	// match, so "80s cereal commercial" would find only items containing that literal string —
	// almost nothing. Parentheses keep Solr's default OR-ish scoring while stopping the
	// mediatype clause from binding to only the last word.
	//
	// Solr's own operators are stripped rather than escaped: a stray `:` or `[` from a person
	// typing a URL would otherwise become a field query or a range, turning a search into a
	// syntax error they cannot see the cause of.
	q.Set("q", "("+sanitizeQuery(query)+") AND mediatype:movies")
	q["fl[]"] = []string{"identifier", "title", "licenseurl", "year", "date"}
	q.Set("rows", strconv.Itoa(limit))
	q.Set("output", "json")

	var out searchResp
	if err := c.getJSON(ctx, c.base+"/advancedsearch.php?"+q.Encode(), &out); err != nil {
		return DiscoveryResult{}, fmt.Errorf("archive search %q: %w", query, err)
	}
	res := toResult(out)
	c.enrich(ctx, res.Items)
	return res, nil
}

// sanitizeQuery strips the Solr syntax a person's search words must not carry. Kept permissive:
// letters, digits, spaces and the punctuation that appears inside real titles.
func sanitizeQuery(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ':', '"', '[', ']', '(', ')', '{', '}', '^', '~', '\\', '/':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
