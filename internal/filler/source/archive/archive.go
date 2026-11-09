// Package archive is the archive.org source provider: a collection the operator registers, listed
// through archive.org's search API and searched inside for the call signs of nearby stations.
//
// The provider owns what is particular to archive.org. A collection is thousands of items, so a
// check reads a page several times the size it will take, because a page of items already held
// would otherwise report nothing new while the rest of the collection sat unfetched; the search
// carries no runtimes, so while compilation reels are held back each item's runtime is read, within
// a time budget, to tell a reel from a single advert; and a station's call sign is searched inside
// the collection and its hits taken ahead of the listing. Filler owns the budget, the lease, the
// dedupe against what is already known, and the selection.
package archive

import (
	"context"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler/fetch"
	"github.com/loomarr/loomarr/internal/filler/source"
)

// ID is the provider's persisted identity.
const ID source.ID = "archive"

// overList is how many times the budget a check reads, so a page full of items already held still
// yields new ones.
const overList = 4

// StationSearchesPerSource bounds the station searches one source gets in one pass. The call signs
// rotate by the hour, so a long list is worked through across passes instead of in one burst
// against the provider.
const StationSearchesPerSource = 4

// RuntimeBudget bounds reading one source's item runtimes (#1773).
const RuntimeBudget = 20 * time.Second

// Catalog is the metadata-only half of the archive.org downloader.
type Catalog interface {
	EnumerateCollection(ctx context.Context, ref string, limit int) (fetch.DiscoveryResult, error)
	SearchCollection(ctx context.Context, ref, query string, limit int) (fetch.DiscoveryResult, error)
	Enrich(ctx context.Context, items []fetch.DiscoveredItem)
}

// Config builds a Provider.
type Config struct {
	// Catalog lists and searches collections; nil uses the live service, uncached.
	Catalog Catalog
	// Runtimes reports whether items need their runtimes before selection. The collection search
	// carries none, so while compilation reels are held back (#1773) each listed item's metadata
	// is read to tell a reel from a single advert. nil never does.
	Runtimes func() bool
	// Log receives a failed station search. nil discards.
	Log *slog.Logger
	// Finder suggests collections and verifies an address; nil uses the live service.
	Finder source.Finder
}

// Provider lists archive.org collections.
type Provider struct {
	catalog  Catalog
	runtimes func() bool
	log      *slog.Logger
	finder   source.Finder
}

// New builds the provider.
func New(cfg Config) *Provider {
	p := &Provider{catalog: cfg.Catalog, runtimes: cfg.Runtimes, log: cfg.Log, finder: cfg.Finder}
	if p.catalog == nil {
		p.catalog = fetch.NewArchiveDownloader()
	}
	if p.finder == nil {
		p.finder = fetch.NewArchiveSourceFinder()
	}
	if p.log == nil {
		p.log = slog.New(slog.DiscardHandler)
	}
	return p
}

// ID implements source.Provider.
func (*Provider) ID() source.ID { return ID }

// Describe implements source.Provider.
func (*Provider) Describe() source.Description {
	return source.Description{
		Label: "Archive.org", AddressHint: "https://archive.org/details/collection", Fetched: true,
		GroupDetail:  "collections you've added — searched and downloaded on their configured schedule",
		SourceDetail: "New clips are checked before they can play.",
		BadAddress: source.Problem{
			Title:  "That doesn't look like an archive.org collection",
			Detail: "Paste a collection identifier (like classic_tv_commercials) or its archive.org URL.",
		},
		Blurb:          "Public collections of commercials, bumpers, and other broadcast material.",
		SourceNoun:     "Archive.org collection",
		PreviewHeading: "Popular videos in this collection",
		Find: source.FindCopy{
			Label:       "Find an Archive.org collection",
			Placeholder: "Search collections or paste an archive.org URL",
			Add:         "Add collection",
			NoMatches:   "No collections found. Paste the exact URL to check it.",
			MinQuery:    2,
		},
	}
}

var (
	// archiveURL is an archive.org collection page or its metadata.
	archiveURL = regexp.MustCompile(`(?i)^(https?://)?(www\.)?archive\.org/(details|metadata)/`)
	// archiveSlug is a bare identifier. It must carry a separator, or any single word would pass
	// for a collection and a search for "cartoons" would be checked as one.
	archiveSlug = regexp.MustCompile(`(?i)^[a-z0-9][a-z0-9_-]*[_-][a-z0-9_-]+$`)
)

// Recognizes implements source.Recognizer.
func (*Provider) Recognizes(input string) bool {
	input = strings.TrimSpace(input)
	return archiveURL.MatchString(input) || archiveSlug.MatchString(input)
}

// SourceURL implements source.Linker: a stored identifier is a collection's details page.
func (*Provider) SourceURL(address string) string {
	v := strings.TrimSpace(address)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(v), "http://") || strings.HasPrefix(strings.ToLower(v), "https://") {
		return v
	}
	return "https://archive.org/details/" + url.PathEscape(v)
}

// EmbedURL implements source.Linker: an item's details page plays in archive.org's embed.
func (*Provider) EmbedURL(itemURL string) string {
	u, err := url.Parse(itemURL)
	if err != nil || strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") != "archive.org" {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "details" || parts[1] == "" {
		return ""
	}
	return "https://archive.org/embed/" + url.PathEscape(parts[1]) + "?autoplay=1"
}

// Suggest implements source.Finder.
func (p *Provider) Suggest(ctx context.Context, query string, limit int) ([]source.Suggestion, error) {
	return p.finder.Suggest(ctx, query, limit)
}

// Resolve implements source.Finder.
func (p *Provider) Resolve(ctx context.Context, input string) (source.Suggestion, error) {
	return p.finder.Resolve(ctx, input)
}

// Address implements source.Addresser: it pulls the collection identifier out of whatever
// an operator pasted.
//
// Three spellings are accepted because all three are things people actually paste: a bare
// identifier, a `/details/<id>` path, and a full URL. Anything else is refused, because silently
// registering an unparseable source would produce a row that can never fetch.
func (*Provider) Address(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if i := strings.Index(v, "archive.org/details/"); i >= 0 {
		v = v[i+len("archive.org/details/"):]
	} else if i := strings.Index(v, "/details/"); i >= 0 {
		v = v[i+len("/details/"):]
	}
	if j := strings.IndexAny(v, "/?#"); j >= 0 {
		v = v[:j]
	}
	// An identifier is a plain slug. Anything still carrying a scheme, a dot or a space is a
	// URL we did not recognise rather than an id, and guessing would create a dead row.
	if v == "" || strings.ContainsAny(v, ":. \t") {
		return ""
	}
	return v
}

// Items converts archive.org's discovered items to provider items.
func Items(items []fetch.DiscoveredItem) []source.Item {
	out := make([]source.Item, 0, len(items))
	for _, it := range items {
		out = append(out, source.Item{
			ID: it.ID, URL: "https://archive.org/details/" + it.ID,
			Title: it.Title, License: it.License, ObservedYear: it.Year,
			PublishedAt: it.Date, DurationMS: it.DurationMS, Height: it.Height,
		})
	}
	return out
}

// List reads a page of the collection, puts the hits of its station searches ahead of it, and
// offers what req.Reject lets through. It does not resume: its checkpoint comes back as given.
func (p *Provider) List(ctx context.Context, src source.Source, req source.ListRequest) (source.Listing, error) {
	res, err := p.catalog.EnumerateCollection(ctx, src.URI, req.List*overList)
	if err != nil {
		return source.Listing{}, err
	}
	if p.runtimes != nil && p.runtimes() {
		// Archive.org throttles per-item metadata (fetch.ArchiveDownloader.Enrich: ~25 items in 15–25 s),
		// so reading runtimes is bounded per source, and never to more than half the time
		// left before the caller's deadline: a planned pull walks up to 12 sources inside one
		// 90 s deadline, and runtimes must never be what makes it fail. Per item and
		// non-fatal: a runtime not read in time stays unknown, the item is taken, and probe
		// marks it a compilation on arrival if it is one.
		wait := RuntimeBudget
		if deadline, ok := ctx.Deadline(); ok {
			wait = min(wait, time.Until(deadline)/2)
		}
		budget, cancel := context.WithTimeout(ctx, wait)
		p.catalog.Enrich(budget, res.Items)
		cancel()
	}
	items := p.withStationHits(ctx, src, Items(res.Items), req)

	out := source.Listing{
		Checkpoint: src.Checkpoint, ReplaceOutcomes: true, Outcomes: source.Outcomes{},
	}
	for _, item := range items {
		if req.Reject != nil {
			if outcome, rejected := req.Reject(item); rejected {
				out.Outcomes[outcome]++
				continue
			}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// Search implements source.Searcher: it finds items inside one collection.
func (p *Provider) Search(ctx context.Context, src source.Source, query string, limit int) ([]source.Item, error) {
	res, err := p.catalog.SearchCollection(ctx, src.URI, query, limit)
	if err != nil {
		return nil, err
	}
	return Items(res.Items), nil
}

// withStationHits searches the source for up to StationSearchesPerSource of the request's call
// signs and puts the hits ahead of its listing, each marked with the call sign that found it. A
// search that fails costs only its hits: the listing still runs the pass.
func (p *Provider) withStationHits(ctx context.Context, src source.Source, items []source.Item, req source.ListRequest) []source.Item {
	callSigns := req.Stations
	if len(callSigns) == 0 {
		return items
	}
	n := min(StationSearchesPerSource, len(callSigns))
	start := int(req.Now.Unix()/3600) % len(callSigns)
	found := map[string]string{}
	var hits []source.Item
	for i := range n {
		sign := callSigns[(start+i)%len(callSigns)]
		refs, err := p.Search(ctx, src, sign, req.List)
		if err != nil {
			p.log.Warn("filler auto-fetch: station search failed", "source", src.ID, "call_sign", sign, "err", err)
			continue
		}
		for _, ref := range refs {
			if ref.ID == "" || found[ref.ID] != "" {
				continue
			}
			found[ref.ID] = sign
			ref.Station = sign
			hits = append(hits, ref)
		}
	}
	if len(hits) == 0 {
		return items
	}
	for _, item := range items {
		if found[item.ID] == "" {
			hits = append(hits, item)
		}
	}
	return hits
}
