package clipfetch

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
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
	q["fl[]"] = []string{"identifier", "title", "licenseurl", "year"}
	q.Set("rows", strconv.Itoa(limit))
	q.Set("output", "json")

	var out searchResp
	if err := c.getJSON(ctx, c.base+"/advancedsearch.php?"+q.Encode(), &out); err != nil {
		return DiscoveryResult{}, fmt.Errorf("archive discover %s: %w", id, err)
	}

	res := DiscoveryResult{
		// Non-nil even when empty: a JSON `null` would make every consumer guard before
		// iterating, and "this collection is empty" is a real answer, not a missing one.
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
		})
	}
	return res, nil
}
