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
	ScopeAll     Scope = "all"
	// ScopeAdjacent marks a candidate that came from the recommendation graph walked
	// from a channel's own lineup (programming-design §8.3) rather than from a query.
	//
	// ⚠ PROVENANCE ONLY — never a /v1/search value. `Scope` does double duty: it is the
	// published search enum AND `Candidate.Source`, the "which corpus surfaced this"
	// debugging field. Adjacency belongs to the second and not the first: it has no query
	// to search with, so offering it as a scope would ship an enum value that ignores `q`
	// — the exact shape design.md §7.2 records as the phantom `clips` scope, corrected
	// rather than implemented. ParseScope deliberately does not accept it, so it cannot
	// arrive from a URL.
	ScopeAdjacent Scope = "adjacent"
)

// ParseScope validates a scope, defaulting to all.
func ParseScope(s string) Scope {
	switch Scope(s) {
	case ScopeLibrary, ScopeTMDB:
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
	// OfficialRating is the media server's content rating ("TV-Y7", "PG-13", …),
	// the input to ChannelPolicy audience enforcement (programming-design §4). Like
	// Genres/Overview it is DISPLAY/ENFORCEMENT metadata only — never identity
	// (Key()/dedupeKey() must not read it). Populated from the media server; TMDB
	// search/discover leaves it empty (audience enforcement is library-first).
	OfficialRating string `json:"officialRating,omitempty"`
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

// LibraryPresence checks whether a specific title (by external id) is already in
// the media library — the piece discovery needs that keyword search gets for free.
// Discovery is TMDB-only (the library has no discover-by-genre endpoint), so a
// discovered title you already OWN would otherwise come back InLibrary=false and
// land as an acquisition. Implemented by a main.go adapter over library.Client's
// Lookup. Optional: a Catalog without one simply skips the backfill.
type LibraryPresence interface {
	// Present reports whether the title is in the library, returning its library
	// item id AND the audience-enforcement metadata (rating/genres) when so. The
	// rating is why this is not a bare bool: a discovered-then-owned title that came
	// back without its rating reaches the scheduler unrated, and a channel with an
	// audience ceiling drops it (dead air, §9). Called per discovered candidate.
	Present(ctx context.Context, mt provision.MediaType, tmdbID, tvdbID int) (Presence, bool, error)
}

// Presence is a library ownership hit: the item id plus the enforcement metadata
// discovery would otherwise lack (the library has no discover endpoint, so titles
// arrive from TMDB and the rating is confirmed only here).
type Presence struct {
	LibraryItemID  string
	OfficialRating string
	Genres         []string
}

// NOTE: there is deliberately no clip corpus here (§7.2, revised). Candidate models a
// PROVISIONABLE TITLE — MediaType admits only movie|series, and every Candidate flows
// through the dedupe/identity machinery that grounds the LLM. A clip is not a title
// (§10), so representing one as a Candidate would push an unprovisionable row with an
// invalid media type into the grounding path. Clip search lives on GET /v1/filler?q=,
// which returns real ClipDTOs carrying a Tunarr program id.
// Wired in Phase 12 when the clip catalog exists; nil until then.
// Catalog federates the corpora. TMDB may be nil (scope skipped).
type Catalog struct {
	lib      LibrarySearcher
	tmdb     TMDBSearcher
	presence LibraryPresence // optional; backfills in-library on discovery results
}

// New builds a Catalog. Any corpus may be nil; its scope is then skipped.
func New(lib LibrarySearcher, tmdb TMDBSearcher) *Catalog {
	return &Catalog{lib: lib, tmdb: tmdb}
}

// WithPresence wires the in-library presence check used to backfill discovery
// results (§8) — so a discovered title you already own is a lineup pick, not an
// acquisition. Returns the Catalog for chaining. Nil = discovery skips the backfill.
func (c *Catalog) WithPresence(p LibraryPresence) *Catalog {
	c.presence = p
	return c
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
	out := dedupeAndOrder(byKey, order, limit)
	c.backfillPresence(ctx, out)
	return out, nil
}

// TMDBRecommender is the adjacency corpus: TMDB's behavioural "also watched" graph for
// one title (programming-design §8.3). Optional, like TMDBDiscoverer — a client that
// doesn't implement it simply yields no adjacency candidates.
type TMDBRecommender interface {
	Recommendations(ctx context.Context, mt provision.MediaType, tmdbID, limit int) ([]Candidate, error)
}

// adjacentPerSeed bounds how many neighbours one seed title contributes.
//
// TMDB returns 20 per page and the tail is progressively weaker, so taking the head is
// both cheaper and better: consensus across seeds is the signal we actually rank on, and
// a long tail mostly adds one-vote noise for the cut below to discard.
const adjacentPerSeed = 20

// minAdjacentConsensus is how many seed titles must independently recommend a candidate
// before it is offered.
//
// ONE vote is noise — TMDB's graph will connect almost anything to something. Requiring
// two is what turns "a film someone also watched" into "a film this CHANNEL points at".
// Probed against the dev 25-film action channel, the ≥2 head was Terminator 2, Mad Max,
// Death Race 2000, Demolition Man, Missing in Action, Romancing the Stone — all on-theme,
// two of them (Mad Max, T2) obvious holes the channel already implied.
const minAdjacentConsensus = 2

// Adjacent walks the recommendation graph from a set of seed titles and returns the
// candidates several of them agree on, ranked by that agreement (§8.3).
//
// The seeds are a channel's own lineup, which is what makes this a deterministic
// alternative to asking the model "what else fits?": the query is the channel's contents
// rather than a paraphrase of its intent. Same seeds ⇒ same candidates, no tokens.
//
// `exclude` is the set of keys already on the channel — a neighbour the channel already
// has is not a discovery. Callers pass the lineup's keys; the walk itself never inspects
// channel state.
//
// BEST-EFFORT PER SEED. One seed's lookup failing (a title TMDB has no graph for, a
// transient 5xx) skips that seed and keeps walking: an adjacency pass is a bonus corpus,
// and degrading it to "fewer candidates" is right where failing the whole re-curation
// would not be.
func (c *Catalog) Adjacent(
	ctx context.Context, seeds []Candidate, exclude map[provision.Key]bool, limit int,
) ([]Candidate, error) {
	rec, ok := c.tmdb.(TMDBRecommender)
	if !ok || c.tmdb == nil {
		return nil, nil // no adjacency corpus wired
	}
	if limit <= 0 {
		limit = 20
	}

	votes := map[string]int{}
	byKey := map[string]*Candidate{}
	order := []string{}

	for _, seed := range seeds {
		if seed.TMDBID == 0 {
			continue // no TMDB identity ⇒ no graph to walk (a TVDB-only series)
		}
		neighbours, err := rec.Recommendations(ctx, seed.MediaType, seed.TMDBID, adjacentPerSeed)
		if err != nil {
			continue // see BEST-EFFORT above
		}
		// One vote per SEED, not per occurrence: a neighbour listed twice by the same seed
		// must not out-rank one that two different seeds both point at.
		seen := map[string]bool{}
		for _, n := range neighbours {
			// A candidate with no resolvable provisioning key can neither be matched against
			// `exclude` nor acquired later, so it is dropped here rather than carried as a
			// pick the approve path would reject.
			nk, kerr := n.Key()
			if kerr != nil {
				continue
			}
			k := dedupeKey(n)
			if seen[k] || exclude[nk] {
				continue
			}
			seen[k] = true
			votes[k]++
			if existing, ok := byKey[k]; ok {
				mergeCandidate(existing, n)
				continue
			}
			cp := n
			cp.Source = ScopeAdjacent
			byKey[k] = &cp
			order = append(order, k)
		}
	}

	// Rank by consensus, then by the order first seen so ties are deterministic rather
	// than map-iteration order (§7 reproducibility).
	position := make(map[string]int, len(order))
	for i, k := range order {
		position[k] = i
	}
	kept := make([]string, 0, len(order))
	for _, k := range order {
		if votes[k] >= minAdjacentConsensus {
			kept = append(kept, k)
		}
	}
	sort.SliceStable(kept, func(a, b int) bool {
		if votes[kept[a]] != votes[kept[b]] {
			return votes[kept[a]] > votes[kept[b]]
		}
		return position[kept[a]] < position[kept[b]]
	})

	out := make([]Candidate, 0, limit)
	for _, k := range kept {
		if len(out) >= limit {
			break
		}
		out = append(out, *byKey[k])
	}
	c.backfillPresence(ctx, out)
	return out, nil
}

// backfillPresence marks discovered candidates the library already owns as
// InLibrary (with their library item id), so a title you have becomes a lineup
// pick rather than a needless acquisition (§8). Best-effort: a per-title lookup
// error is skipped (the candidate stays not-in-library — the approve path re-
// checks anyway), never failing discovery. No-op when no presence check is wired.
func (c *Catalog) backfillPresence(ctx context.Context, cands []Candidate) {
	if c.presence == nil {
		return
	}
	for i := range cands {
		if cands[i].InLibrary || cands[i].TMDBID == 0 {
			continue // already known in-library, or no id to look up
		}
		p, present, err := c.presence.Present(ctx, cands[i].MediaType, cands[i].TMDBID, cands[i].TVDBID)
		if err != nil || !present {
			continue
		}
		cands[i].InLibrary = true
		cands[i].LibraryItemID = p.LibraryItemID
		// Carry the enforcement metadata the discovery result lacked. Additive, like
		// the merge below: a candidate that already has a rating (it won't, from TMDB
		// discovery) keeps it. Dropping this is the dead-air bug — the whole reason
		// Presence carries more than an id.
		if cands[i].OfficialRating == "" {
			cands[i].OfficialRating = p.OfficialRating
		}
		if len(cands[i].Genres) == 0 {
			cands[i].Genres = p.Genres
		}
	}
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
	// OfficialRating comes from the library keyword search or the presence backfill —
	// TMDB *search/discover* leaves it empty, though TMDB's content-ratings endpoint
	// can fill it for a not-yet-owned acquisition (see enrichRatings). Keep whichever
	// side actually has it.
	if dst.OfficialRating == "" && src.OfficialRating != "" {
		dst.OfficialRating = src.OfficialRating
	}
}

// fromLibrary converts a library search result into a Candidate (in_library=true).
func fromLibrary(r library.SearchResult) Candidate {
	return Candidate{
		MediaType:      mediaType(r.MediaType),
		TMDBID:         r.TMDBID,
		TVDBID:         r.TVDBID,
		Name:           r.Name,
		Year:           r.Year,
		InLibrary:      true,
		LibraryItemID:  r.LibraryItemID,
		Genres:         r.Genres,
		Overview:       r.Overview,
		OfficialRating: r.OfficialRating,
		Source:         ScopeLibrary,
	}
}

// mediaType maps a library.MediaType (Emby "Movie"/"Series") to provision's.
func mediaType(m library.MediaType) provision.MediaType {
	if m == library.Series {
		return provision.Series
	}
	return provision.Movie
}
