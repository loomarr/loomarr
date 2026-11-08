package scan

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/localart"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// builder turns one root's walked files into inventory snapshots for one pass.
type builder struct {
	im   *Importer
	root string
	idx  managerIndex
	// seen is every origin the pass found present: each file, and each series with a file.
	seen map[inventory.OriginKey]bool
	// series is each series folder the pass found, and whether one of its episodes was written.
	series map[string]*seriesSeen
}

type seriesSeen struct {
	dir     string
	managed *mp.Record
	ids     []mp.ExternalID
	write   bool
	// itemID is the series item writeSeries wrote, "" until then.
	itemID string
	// last is the highest episode number on disk in each season, from every file the pass
	// identified before writing any.
	last map[int]int

	once       sync.Once
	resolution resolution
	// seasons are its season records the pass read, by catalog id.
	seasonsMu sync.Mutex
	seasons   map[string]cachedSeason
}

// listed finishes a movie or series the pass wrote: its library row came with the write (the
// store's LibraryProjection); its poster and its set's record are asked for here.
func (b *builder) listed(ctx context.Context, id string, snapshot inventory.Snapshot) {
	facts := snapshot.Observation.Facts
	set := setKey(facts.Relationships)
	b.poster(ctx, id, facts.Artwork)
	// The set's own record names it and every movie in it; the Enricher fetches it once asked.
	if set.String() != "" && b.im.Catalog != nil {
		if err := b.im.Catalog.WantLibraryCatalog(ctx, []store.LibraryCatalogKey{set}); err != nil {
			b.im.log().Warn("library scan: set record not asked for", "catalog", set.String(), "err", err)
		}
	}
}

// setKey is the catalog row of the ready-made set a movie's facts name; zero for none. The key's
// provider is the namespace the Enricher fetches it by.
func setKey(rels []inventory.Relationship) store.LibraryCatalogKey {
	for _, r := range rels {
		if r.Kind != string(mp.KindCollection) {
			continue
		}
		ns, id, ok := strings.Cut(r.ExternalID, ":")
		if ok && ns == string(mp.NamespaceTMDBCollection) && id != "" {
			return store.LibraryCatalogKey{Provider: ns, Kind: string(mp.KindCollection), ExternalID: id}
		}
	}
	return store.LibraryCatalogKey{}
}

// match is what said which title resolved is: the source of its highest-ranked catalog id (a
// collection's id alone says nothing), with that source's page for it. Identity is picked in rank
// order, so the first such id is the deciding one.
func match(resolved precedence.Resolved, records []mp.Record) store.LibraryMatch {
	for _, id := range resolved.IDs {
		switch id.Value.Namespace {
		case mp.NamespaceTMDB, mp.NamespaceTVDB, mp.NamespaceIMDB:
		default:
			continue
		}
		if id.Value.Value == "" {
			continue
		}
		m := store.LibraryMatch{Basis: string(id.From.Basis), App: string(id.From.Provider)}
		for _, rec := range records {
			if rec.Source == id.From && rec.Page != "" {
				m.Page = rec.Page
				break
			}
		}
		return m
	}
	return store.LibraryMatch{}
}

func libraryOrigin(o inventory.OriginKey) store.LibraryOrigin {
	return store.LibraryOrigin{Authority: string(o.Authority), Key: o.ExternalItemID}
}

// poster hands the title's resolved poster, or none, to the image service. One that cannot be
// imported leaves the title as it was until it is written again; the grid shows its name.
func (b *builder) poster(ctx context.Context, id string, art []inventory.ArtworkRef) {
	if b.im.Posters == nil {
		return
	}
	ref := ""
	for _, a := range art {
		if a.Kind == localart.KindPoster {
			ref = a.Ref
			break
		}
	}
	if err := b.im.Posters.Poster(ctx, id, ref); err != nil {
		b.im.log().Warn("library scan: poster not imported", "item", id, "err", err)
	}
}

// identity is what the cheap sources say a file is: no file is opened to learn it.
type identity struct {
	kind     mp.Kind
	managed  managed
	embedded mp.Record
	// seriesDir is an episode's series folder, relative to the root.
	seriesDir string
	digest    string
}

func newBuilder(im *Importer, root string, idx managerIndex) *builder {
	return &builder{
		im: im, root: root, idx: idx, seen: map[inventory.OriginKey]bool{},
		series: map[string]*seriesSeen{},
	}
}

func (b *builder) identify(file File) identity {
	m := b.idx.byPath[file.Path]
	embedded, seriesDir := embeddedRecord(file)
	id := identity{kind: embedded.Kind, managed: m, embedded: embedded, seriesDir: seriesDir}
	if len(m.records) > 0 {
		id.kind = m.records[0].Kind
		if id.kind == mp.KindEpisode && seriesDir == "" {
			id.seriesDir = seriesFolder(file.Rel)
		}
	}
	b.seen[fileOrigin(b.root, file.Rel)] = true
	if id.kind == mp.KindEpisode {
		s := b.series[id.seriesDir]
		if s == nil {
			s = &seriesSeen{dir: id.seriesDir, ids: embedded.Parent, last: map[int]int{}}
			b.series[id.seriesDir] = s
		}
		if m.series != nil {
			s.managed = m.series
		}
		if n := id.numbering(); n != nil {
			s.last[n.Season] = max(s.last[n.Season], n.Episode, n.EpisodeEnd)
		}
		b.seen[seriesOrigin(b.root, id.seriesDir)] = true
	}
	id.digest = identityDigest(file, m, embedded)
	return id
}

// numbering is where the cheap sources place an episode file, through its last episode; nil for
// none.
func (id identity) numbering() *mp.Numbering {
	recs := id.managed.records
	if len(recs) == 0 {
		return id.embedded.Numbering
	}
	if recs[0].Numbering == nil {
		return nil
	}
	n := *recs[0].Numbering
	if last := recs[len(recs)-1].Numbering; last != nil && last.Episode > n.Episode {
		n.EpisodeEnd = last.Episode
	}
	return &n
}

// markWritten records that the pass writes the file, so its series is written too. It runs before
// any header read: snapshot is read-only on the builder, and header reads build snapshots
// concurrently.
func (b *builder) markWritten(id identity) {
	if id.kind == mp.KindEpisode {
		b.series[id.seriesDir].write = true
	}
}

// cheapIDs are the ids the cheap sources give a file, and its series': enough to put a wanted file's
// header read first without building its item.
func (id identity) cheapIDs() []inventory.ExternalID {
	var ids []mp.ExternalID
	for _, r := range id.managed.records {
		ids = append(ids, r.IDs...)
	}
	if id.managed.series != nil {
		ids = append(ids, id.managed.series.IDs...)
	}
	ids = append(ids, id.embedded.IDs...)
	ids = append(ids, id.embedded.Parent...)
	return toInventory(ids)
}

// snapshot is the file's inventory item without its header facts. It reads the file's NFO
// sidecar, so it runs only for a file that is written or whose header is read. It does not change
// the builder.
func (b *builder) snapshot(ctx context.Context, file File, id identity, now time.Time) inventory.Snapshot {
	snapshot := inventory.Snapshot{
		Origin: fileOrigin(b.root, file.Rel),
		Observation: inventory.Observation[inventory.ItemFacts]{
			SchemaVersion: 1, ObservedAt: now,
		},
	}
	records := append([]mp.Record(nil), id.managed.records...)
	// further are the other episodes of a multi-episode file. The file is one item, as the media
	// server keeps it: its first episode, numbered through its last, carrying every episode's ids
	// so a catalog that keys the file by any of them finds it.
	var further []mp.Record
	if len(records) > 1 && id.kind == mp.KindEpisode {
		first := records[0]
		if first.Numbering != nil && records[len(records)-1].Numbering != nil {
			n := *first.Numbering
			n.EpisodeEnd = records[len(records)-1].Numbering.Episode
			first.Numbering = &n
		}
		further = records[1:]
		records = []mp.Record{first}
	}
	records = append(records, id.embedded)
	if file.hasNFO() {
		if rec, ok := b.sidecar(ctx, b.im.NFO, id.kind, file.Path); ok {
			records = append(records, rec)
			if id.kind == mp.KindEpisode {
				further = append(further, rec.More...)
			}
		}
	}
	if names := file.art(); b.im.Art != nil && len(names) > 0 {
		// The walk has just listed the folder: its artwork files are the file's sidecars.
		if rec, err := localart.Beside(id.kind, file.Path, names, file.Sole); err == nil {
			records = append(records, rec)
		}
	}

	var series []mp.Record
	if id.kind == mp.KindEpisode {
		if s := b.series[id.seriesDir]; s.managed != nil {
			series = append(series, *s.managed)
		}
	}
	in := precedence.Input{Kind: id.kind, Records: records, Series: series}
	res := b.resolve(ctx, snapshot.Origin, in)
	if id.kind == mp.KindEpisode && res.catalog.String() == "" {
		res = b.byNumber(ctx, b.series[id.seriesDir], in, res)
	}
	resolved := res.resolved
	facts, coverage := itemFacts(resolved)
	if facts.Name == "" {
		facts.Name = fallbackName(file.Rel)
	}
	coverage["sources"] = inventory.CoveragePresent
	snapshot.Kind = kindOf(id.kind)
	snapshot.Observation.Facts, snapshot.Observation.Coverage = facts, coverage
	var seriesKey inventory.OriginKey
	if id.kind == mp.KindEpisode && id.seriesDir != "" {
		seriesKey = seriesOrigin(b.root, id.seriesDir)
	}
	snapshot.Observation.Extension = originExtensionOf(id.kind, res, len(id.managed.records) > 0, seriesKey)
	snapshot.ExternalIDs = withFurtherIDs(externalIDs(resolved.IDs), further)
	return snapshot
}

// withFurtherIDs adds the further episodes' own ids to a multi-episode file's, once each.
func withFurtherIDs(ids []inventory.ExternalID, further []mp.Record) []inventory.ExternalID {
	for _, rec := range further {
		for _, id := range toInventory(rec.IDs) {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// resolution is what one title resolves to.
type resolution struct {
	resolved precedence.Resolved
	match    store.LibraryMatch
	// catalog is the catalog row its details come from, cached or not; zero for none.
	catalog store.LibraryCatalogKey
	// record is that row's cached record, nil until the Enricher fetches it.
	record *mp.Record
}

// resolve applies precedence. A movie or series the maintainer linked to a title (Needs you) has
// the link as a record of its own, below what the files say (decision 27). One whose ids then name
// a cached catalog record is resolved again with it, so its details come from the catalog
// (decision 7); one not cached yet is written without, and again once the Enricher fetches it.
func (b *builder) resolve(ctx context.Context, origin inventory.OriginKey, in precedence.Input) resolution {
	if rec, ok := b.im.linkRecord(ctx, in.Kind, origin); ok {
		in.Records = append(append([]mp.Record(nil), in.Records...), rec)
	}
	out := resolution{resolved: b.im.Resolver.Resolve(in)}
	// The catalog record below adds details, never identity: what decides is known now.
	if in.Kind == mp.KindMovie || in.Kind == mp.KindSeries {
		out.match = match(out.resolved, in.Records)
	}
	out.catalog = catalogKey(in.Kind, pickedIDs(out.resolved.IDs))
	rec, ok := b.im.catalogRecord(ctx, out.catalog)
	if !ok {
		return out
	}
	out.record = &rec
	in.Records = append(append([]mp.Record(nil), in.Records...), rec)
	out.resolved = b.im.Resolver.Resolve(in)
	return out
}

// sidecar reads what a sidecar provider (NFO, artwork) finds beside a file or in a series folder.
// None, or one that does not parse, says nothing.
func (b *builder) sidecar(ctx context.Context, p mp.Provider, kind mp.Kind, mediaPath string) (mp.Record, bool) {
	if p == nil {
		return mp.Record{}, false
	}
	rec, err := p.Fetch(ctx, mp.Ref{Kind: kind, ID: mp.ExternalID{Namespace: mp.NamespaceFile, Value: mediaPath}})
	if err != nil {
		if !errors.Is(err, mp.ErrNotFound) {
			b.im.log().Debug("library scan: sidecar not read", "err", err)
		}
		return mp.Record{}, false
	}
	return rec, true
}

// resolveSeries resolves a series once a pass: its episodes read it to number themselves by its
// catalog's seasons, and writeSeries writes it. Header reads resolve concurrently, so it is
// guarded.
func (b *builder) resolveSeries(ctx context.Context, s *seriesSeen) resolution {
	s.once.Do(func() {
		records := []mp.Record{{Source: mp.Source{Basis: mp.BasisEmbeddedID}, Kind: mp.KindSeries, IDs: s.ids}}
		if s.managed != nil {
			records = append(records, *s.managed)
		}
		if s.dir != "" {
			dir := filepath.Join(b.root, filepath.FromSlash(s.dir))
			providers := []mp.Provider{b.im.NFO}
			if b.im.Art != nil {
				providers = append(providers, b.im.Art)
			}
			for _, p := range providers {
				if rec, ok := b.sidecar(ctx, p, mp.KindSeries, dir); ok {
					records = append(records, rec)
				}
			}
		}
		s.resolution = b.resolve(ctx, seriesOrigin(b.root, s.dir), precedence.Input{Kind: mp.KindSeries, Records: records})
	})
	return s.resolution
}

// writeSeries writes each series one of whose episodes this pass wrote.
func (b *builder) writeSeries(ctx context.Context, now time.Time) error {
	for _, s := range b.series {
		if !s.write {
			continue
		}
		res := b.resolveSeries(ctx, s)
		resolved := res.resolved
		facts, coverage := itemFacts(resolved)
		if facts.Name == "" {
			facts.Name = strings.TrimSpace(embeddedID.ReplaceAllString(s.dir, ""))
		}
		coverage["sources"] = inventory.CoverageEmpty
		snapshot := inventory.Snapshot{
			Origin: seriesOrigin(b.root, s.dir), Kind: kindSeries,
			Observation: inventory.Observation[inventory.ItemFacts]{
				SchemaVersion: 1, ObservedAt: now, Coverage: coverage, Facts: facts,
				Extension: originExtensionOf(mp.KindSeries, res, false, inventory.OriginKey{}),
			},
			ExternalIDs: externalIDs(resolved.IDs),
		}
		id, err := b.im.apply(ctx, snapshot)
		if err != nil && !errors.Is(err, inventory.ErrIdentityConflict) {
			return err
		}
		if err != nil {
			b.im.log().Warn("library scan: a series' ids name two items", "series", s.dir)
			continue
		}
		s.itemID = string(id)
		b.listed(ctx, s.itemID, snapshot)
	}
	return nil
}

// kindOf is the inventory kind a file of kind k is written as.
func kindOf(k mp.Kind) inventory.ItemKind {
	switch k {
	case mp.KindEpisode:
		return kindEpisode
	case mp.KindSeries:
		return kindSeries
	default:
		return kindMovie
	}
}

func toInventory(ids []mp.ExternalID) []inventory.ExternalID {
	out := make([]inventory.ExternalID, 0, len(ids))
	for _, id := range ids {
		out = append(out, inventory.ExternalID{Namespace: string(id.Namespace), Value: id.Value})
	}
	return out
}
