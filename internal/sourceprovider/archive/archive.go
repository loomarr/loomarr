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
	"time"

	"github.com/loomarr/loomarr/internal/clipfetch"
	"github.com/loomarr/loomarr/internal/sourceprovider"
)

// ID is the provider's persisted identity.
const ID sourceprovider.ID = "archive"

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
	EnumerateCollection(ctx context.Context, ref string, limit int) (clipfetch.DiscoveryResult, error)
	SearchCollection(ctx context.Context, ref, query string, limit int) (clipfetch.DiscoveryResult, error)
	Enrich(ctx context.Context, items []clipfetch.DiscoveredItem)
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
}

// Provider lists archive.org collections.
type Provider struct {
	catalog  Catalog
	runtimes func() bool
	log      *slog.Logger
}

// New builds the provider.
func New(cfg Config) *Provider {
	p := &Provider{catalog: cfg.Catalog, runtimes: cfg.Runtimes, log: cfg.Log}
	if p.catalog == nil {
		p.catalog = clipfetch.NewArchiveDownloader()
	}
	if p.log == nil {
		p.log = slog.New(slog.DiscardHandler)
	}
	return p
}

// ID implements sourceprovider.Provider.
func (*Provider) ID() sourceprovider.ID { return ID }

// Describe implements sourceprovider.Provider.
func (*Provider) Describe() sourceprovider.Description {
	return sourceprovider.Description{
		Label: "Internet Archive", AddressHint: "https://archive.org/details/collection", Fetched: true,
	}
}

// Items converts archive.org's discovered items to provider items.
func Items(items []clipfetch.DiscoveredItem) []sourceprovider.Item {
	out := make([]sourceprovider.Item, 0, len(items))
	for _, it := range items {
		out = append(out, sourceprovider.Item{
			ID: it.ID, URL: "https://archive.org/details/" + it.ID,
			Title: it.Title, License: it.License, ObservedYear: it.Year,
			PublishedAt: it.Date, DurationMS: it.DurationMS, Height: it.Height,
		})
	}
	return out
}

// List reads a page of the collection, puts the hits of its station searches ahead of it, and
// offers what req.Reject lets through. It does not resume: its checkpoint comes back as given.
func (p *Provider) List(ctx context.Context, src sourceprovider.Source, req sourceprovider.ListRequest) (sourceprovider.Listing, error) {
	res, err := p.catalog.EnumerateCollection(ctx, src.URI, req.List*overList)
	if err != nil {
		return sourceprovider.Listing{}, err
	}
	if p.runtimes != nil && p.runtimes() {
		// Archive.org throttles per-item metadata (clipfetch.Enrich: ~25 items in 15–25 s),
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

	out := sourceprovider.Listing{
		Checkpoint: src.Checkpoint, ReplaceOutcomes: true, Outcomes: sourceprovider.Outcomes{},
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

// Search implements sourceprovider.Searcher: it finds items inside one collection.
func (p *Provider) Search(ctx context.Context, src sourceprovider.Source, query string, limit int) ([]sourceprovider.Item, error) {
	res, err := p.catalog.SearchCollection(ctx, src.URI, query, limit)
	if err != nil {
		return nil, err
	}
	return Items(res.Items), nil
}

// withStationHits searches the source for up to StationSearchesPerSource of the request's call
// signs and puts the hits ahead of its listing, each marked with the call sign that found it. A
// search that fails costs only its hits: the listing still runs the pass.
func (p *Provider) withStationHits(ctx context.Context, src sourceprovider.Source, items []sourceprovider.Item, req sourceprovider.ListRequest) []sourceprovider.Item {
	callSigns := req.Stations
	if len(callSigns) == 0 {
		return items
	}
	n := min(StationSearchesPerSource, len(callSigns))
	start := int(req.Now.Unix()/3600) % len(callSigns)
	found := map[string]string{}
	var hits []sourceprovider.Item
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
