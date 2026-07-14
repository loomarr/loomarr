// Package catalog is the Catalog boundary (design §7.2, §8): federated search over
// the library + TMDB + the clip catalog, returning grounded Candidates with real
// external ids and an in_library flag. It is THE single implementation behind
// both GET /v1/search (the human's box) and the LLM's grounding tool — so the
// model and the operator see identical results, and "why did the suggester
// pick/miss X" is debuggable by typing the query into the UI (§7.2). Loomarr
// builds no search index (§7.2): each corpus is queried through its owner.
//
// This is the grounding chokepoint (§8): the LLM never invents titles; it can
// only choose from what Search returns, and every Candidate carries a real id.
package catalog

import (
	"context"
	"sort"
	"strconv"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/provision"
)

// Scope selects which corpora a search fans out to (§7.2).
type Scope string

const (
	ScopeLibrary Scope = "library"
	ScopeTMDB    Scope = "tmdb"
	ScopeClips   Scope = "clips"
	ScopeAll     Scope = "all"
)

// ParseScope validates a scope, defaulting to all.
func ParseScope(s string) Scope {
	switch Scope(s) {
	case ScopeLibrary, ScopeTMDB, ScopeClips:
		return Scope(s)
	default:
		return ScopeAll
	}
}

// Candidate is one grounded search result (§7.2). Identity is always a real
// external id — never a bare title — which is what makes the suggester's output
// actionable and safe (§8). InLibrary + LibraryItemID are set when the title is
// already present.
type Candidate struct {
	MediaType     provision.MediaType `json:"mediaType"`
	TMDBID        int                 `json:"tmdbId,omitempty"`
	TVDBID        int                 `json:"tvdbId,omitempty"`
	Name          string              `json:"name"`
	Year          int                 `json:"year,omitempty"`
	InLibrary     bool                `json:"inLibrary"`
	LibraryItemID string              `json:"libraryItemId,omitempty"`
	// Genres + Overview are DISPLAY/REASONING signals (§8): they let the model
	// judge theme-fit and feed deterministic theme scoring. They are NEVER part of
	// identity — Key() and dedupeKey() must not read them, or the grounding gate
	// and in-library-first ordering would shift. Populated from TMDB (genre_ids→
	// names, overview) and the media server (Genres field), merged additively.
	Genres   []string `json:"genres,omitempty"`
	Overview string   `json:"overview,omitempty"`
	// Source records which corpus surfaced this candidate first (for debugging
	// "why did the model see this"); after dedupe it's the merged view.
	Source Scope `json:"source"`
}

// Key derives the provisioning key for a candidate (§3), so a candidate can flow
// straight into an acquisition/lineup slot. Errors on an under-identified
// candidate (no usable id) — the grounding guarantee is that this never happens
// for a real Candidate, but Key enforces it.
func (c Candidate) Key() (provision.Key, error) {
	t := provision.Title{
		MediaType: c.MediaType, TMDBID: c.TMDBID, TVDBID: c.TVDBID,
		Name: c.Name, Year: c.Year,
	}
	return t.Key()
}

// LibrarySearcher is the library-scope corpus (implemented by library.Client).
type LibrarySearcher interface {
	Search(ctx context.Context, term string, limit int) ([]library.SearchResult, error)
}

// TMDBSearcher is the TMDB-scope corpus (implemented by tmdb.Client).
type TMDBSearcher interface {
	Search(ctx context.Context, term string, limit int) ([]Candidate, error)
}

// TMDBDiscoverer is the TMDB discovery corpus — by genre + era rather than title
// text (§8). Implemented by tmdb.Client. Optional: a nil tmdb ⇒ discovery is a
// keyword fallback (the caller degrades to Search).
type TMDBDiscoverer interface {
	Discover(ctx context.Context, mt provision.MediaType, genres []string, yearFrom, yearTo, limit int) ([]Candidate, error)
}

// ClipSearcher is the clip-catalog corpus (a name LIKE over the store — §7.2).
// Wired in Phase 12 when the clip catalog exists; nil until then.
type ClipSearcher interface {
	SearchClips(ctx context.Context, term string, limit int) ([]Candidate, error)
}

// Catalog federates the corpora. TMDB/clips may be nil (scope skipped).
type Catalog struct {
	lib   LibrarySearcher
	tmdb  TMDBSearcher
	clips ClipSearcher
}

// New builds a Catalog. Any corpus may be nil; its scope is then skipped.
func New(lib LibrarySearcher, tmdb TMDBSearcher, clips ClipSearcher) *Catalog {
	return &Catalog{lib: lib, tmdb: tmdb, clips: clips}
}

// Search fans out to the requested scope and returns a deduped, merged candidate
// list (§7.2). Library results set InLibrary=true; a TMDB result that matches a
// library candidate by id is merged into the library one (so a title present in
// both shows in_library, not twice). Ordering is deterministic: in-library
// first, then by name — the LLM tool and the UI must see a stable order.
func (c *Catalog) Search(ctx context.Context, term string, scope Scope, limit int) ([]Candidate, error) {
	if limit <= 0 {
		limit = 20
	}
	byKey := map[string]*Candidate{}
	order := []string{}

	add := func(cand Candidate) {
		k := dedupeKey(cand)
		if existing, ok := byKey[k]; ok {
			mergeCandidate(existing, cand)
			return
		}
		cp := cand
		byKey[k] = &cp
		order = append(order, k)
	}

	// Library first, so its in_library + library item id win on merge.
	if scopeIncludes(scope, ScopeLibrary) && c.lib != nil {
		res, err := c.lib.Search(ctx, term, limit)
		if err != nil {
			return nil, err
		}
		for _, r := range res {
			add(fromLibrary(r))
		}
	}
	if scopeIncludes(scope, ScopeTMDB) && c.tmdb != nil {
		res, err := c.tmdb.Search(ctx, term, limit)
		if err != nil {
			return nil, err
		}
		for _, cand := range res {
			add(cand)
		}
	}
	if scopeIncludes(scope, ScopeClips) && c.clips != nil {
		res, err := c.clips.SearchClips(ctx, term, limit)
		if err != nil {
			return nil, err
		}
		for _, cand := range res {
			add(cand)
		}
	}

	return dedupeAndOrder(byKey, order, limit), nil
}

// Discover finds titles by genre + era (§8 discovery path) — the fix for abstract
// intents that title keyword search can't surface. It is TMDB-only (the media
// server has no discover endpoint), but merges/orders through the SAME machinery
// as Search (in-library-first, deduped) so its output is grounding-compatible.
// A discovered title already in the library still comes back InLibrary=false here;
// it merges to in-library IF the same run also keyword-surfaced it, and the
// approve/schedule path re-checks library presence regardless. Returns an error
// only on a real TMDB failure; a nil/unsupported tmdb yields no discovery results.
func (c *Catalog) Discover(ctx context.Context, mt provision.MediaType, genres []string, yearFrom, yearTo, limit int) ([]Candidate, error) {
	if limit <= 0 {
		limit = 20
	}
	d, ok := c.tmdb.(TMDBDiscoverer)
	if !ok || c.tmdb == nil {
		return nil, nil // no discovery corpus wired
	}
	res, err := d.Discover(ctx, mt, genres, yearFrom, yearTo, limit)
	if err != nil {
		return nil, err
	}
	byKey := map[string]*Candidate{}
	order := []string{}
	for _, cand := range res {
		k := dedupeKey(cand)
		if existing, ok := byKey[k]; ok {
			mergeCandidate(existing, cand)
			continue
		}
		cp := cand
		byKey[k] = &cp
		order = append(order, k)
	}
	return dedupeAndOrder(byKey, order, limit), nil
}

// dedupeAndOrder flattens the merged candidate map into the deterministic order
// the tool + UI depend on: in-library first, then by name; truncated to limit.
func dedupeAndOrder(byKey map[string]*Candidate, order []string, limit int) []Candidate {
	out := make([]Candidate, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].InLibrary != out[j].InLibrary {
			return out[i].InLibrary // in-library first
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func scopeIncludes(scope, want Scope) bool { return scope == ScopeAll || scope == want }

// dedupeKey identifies a candidate across corpora. Prefer TMDB id (canonical for
// movies + present on most TMDB/library results); fall back to TVDB, else name.
func dedupeKey(c Candidate) string {
	switch {
	case c.TMDBID > 0:
		return string(c.MediaType) + ":tmdb:" + strconv.Itoa(c.TMDBID)
	case c.TVDBID > 0:
		return string(c.MediaType) + ":tvdb:" + strconv.Itoa(c.TVDBID)
	default:
		return string(c.MediaType) + ":name:" + c.Name
	}
}

// mergeCandidate folds src into dst, preferring library presence + ids that are
// set. Library-sourced candidates carry in_library + the item id; a later TMDB
// hit for the same title must not clear those.
func mergeCandidate(dst *Candidate, src Candidate) {
	if src.InLibrary {
		dst.InLibrary = true
		if src.LibraryItemID != "" {
			dst.LibraryItemID = src.LibraryItemID
		}
	}
	if dst.TMDBID == 0 && src.TMDBID != 0 {
		dst.TMDBID = src.TMDBID
	}
	if dst.TVDBID == 0 && src.TVDBID != 0 {
		dst.TVDBID = src.TVDBID
	}
	if dst.Year == 0 && src.Year != 0 {
		dst.Year = src.Year
	}
	// Enrichment is additive: a library row often has genres, a TMDB row has the
	// overview — take whichever the merged view is still missing. Never part of
	// identity (see Candidate.Genres/Overview doc).
	if len(dst.Genres) == 0 && len(src.Genres) > 0 {
		dst.Genres = src.Genres
	}
	if dst.Overview == "" && src.Overview != "" {
		dst.Overview = src.Overview
	}
}

// fromLibrary converts a library search result into a Candidate (in_library=true).
func fromLibrary(r library.SearchResult) Candidate {
	return Candidate{
		MediaType:     mediaType(r.MediaType),
		TMDBID:        r.TMDBID,
		TVDBID:        r.TVDBID,
		Name:          r.Name,
		Year:          r.Year,
		InLibrary:     true,
		LibraryItemID: r.LibraryItemID,
		Genres:        r.Genres,
		Overview:      r.Overview,
		Source:        ScopeLibrary,
	}
}

// mediaType maps a library.MediaType (Emby "Movie"/"Series") to provision's.
func mediaType(m library.MediaType) provision.MediaType {
	if m == library.Series {
		return provision.Series
	}
	return provision.Movie
}
