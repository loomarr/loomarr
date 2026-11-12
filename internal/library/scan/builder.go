package scan

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/localart"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
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
	// sidecars are the series folder's own tvshow.nfo and folder art, stat'd once per pass. Each
	// episode's identity digest includes them, so editing one rewrites the episodes and, through
	// them, the series, in a full walk and a folder pass alike (#3226).
	sidecars map[string]Stat
	// last is the highest episode number on disk in each season, from every file the pass
	// identified before writing any.
	last map[int]int

	once       sync.Once
	resolution resolution
	// seasons are its season and episode-order records the pass read, by catalog id.
	seasonsMu sync.Mutex
	seasons   map[string]cachedSeason

	// order is the household's answer for which order its files follow, read once a pass.
	orderOnce sync.Once
	order     librarystore.LibraryEpisodeOrder
	answered  bool
	// ordersWanted is whether the pass has asked for its catalog's other orders yet.
	ordersWanted bool
}

// listed finishes a movie or series the pass wrote: its library row came with the write (the
// store's LibraryProjection); its artwork and its set's record are asked for here.
func (b *builder) listed(ctx context.Context, id string, snapshot inventory.Snapshot) {
	var set librarystore.LibraryCatalogKey
	if listed := readOriginExtension(snapshot.Observation.Extension).Listed; listed != nil {
		set = listed.Set
	}
	b.artwork(ctx, id, snapshot.Observation.Facts.Artwork)
	// The set's own record names it and every movie in it; the Enricher fetches it once asked.
	if set.String() != "" && b.im.Catalog != nil {
		if err := b.im.Catalog.WantLibraryCatalog(ctx, []librarystore.LibraryCatalogKey{set}); err != nil {
			b.im.log().Warn("library scan: set record not asked for", "catalog", set.String(), "err", err)
		}
	}
}

// match is what said which title resolved is: the source of its highest-ranked catalog id (a
// collection's id alone says nothing), with that source's page for it. Identity is picked in rank
// order, so the first such id is the deciding one.
func match(resolved precedence.Resolved, records []mp.Record) librarystore.LibraryMatch {
	for _, id := range resolved.IDs {
		if !id.Value.Namespace.Grounds() || id.Value.Value == "" {
			continue
		}
		m := librarystore.LibraryMatch{Basis: string(id.From.Basis), App: string(id.From.Provider)}
		for _, rec := range records {
			if rec.Source == id.From && rec.Page != "" {
				m.Page = rec.Page
				break
			}
		}
		return m
	}
	return librarystore.LibraryMatch{}
}

func libraryOrigin(o inventory.OriginKey) librarystore.LibraryOrigin {
	return librarystore.LibraryOrigin{Authority: string(o.Authority), Key: o.ExternalItemID}
}

// artwork hands the title's resolved poster, fanart and clear logo, or none of each, to the image
// service. Precedence already put a local sidecar ahead of a provider's image (decision 19), so
// the first of a kind is the one. One that cannot be imported leaves the title's image of that
// kind as it was until it is written again.
func (b *builder) artwork(ctx context.Context, id string, art []inventory.ArtworkRef) {
	if b.im.Artwork == nil {
		return
	}
	for _, kind := range ImportedArt {
		ref := ""
		for _, a := range art {
			if a.Kind == kind && a.Season == 0 {
				ref = a.Ref
				break
			}
		}
		if err := b.im.Artwork.Art(ctx, id, kind, ref); err != nil {
			b.im.log().Warn("library scan: artwork not imported", "item", id, "kind", kind, "err", err)
		}
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

// airDate is the day the file's name says its episode aired, for one named by it rather than by
// season and number; "" otherwise.
func (id identity) airDate() string {
	if id.embedded.Numbering != nil || id.embedded.Descriptive == nil {
		return ""
	}
	return id.embedded.Descriptive.PremiereDate
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
	// A file whose name says nothing of an episode is one when its own NFO says so: a scraper
	// placed it ("Kettle Bay - Tidewater.mkv" beside an <episodedetails>).
	if embedded.Kind == mp.KindMovie && len(m.records) == 0 && path.Dir(file.Rel) != "." {
		own := strings.ToLower(strings.TrimSuffix(file.Path, filepath.Ext(file.Path)))
		for nfo, st := range file.Sidecars {
			if !isNFO(nfo) || strings.ToLower(strings.TrimSuffix(nfo, filepath.Ext(nfo))) != own {
				continue
			}
			if b.im.nfoRoots.rootOf(nfo, st) == "episodedetails" {
				seriesDir = seriesFolderOf(file)
				embedded = mp.Record{Source: embedded.Source, Kind: mp.KindEpisode, Parent: idsIn(path.Base(seriesDir))}
			}
		}
	}
	id := identity{kind: embedded.Kind, managed: m, embedded: embedded, seriesDir: seriesDir}
	if len(m.records) > 0 {
		id.kind = m.records[0].Kind
		if id.kind == mp.KindEpisode && seriesDir == "" {
			id.seriesDir = seriesFolderOf(file)
		}
	}
	b.seen[fileOrigin(b.root, file.Rel)] = true
	var seriesSidecars map[string]Stat
	if id.kind == mp.KindEpisode {
		s := b.series[id.seriesDir]
		if s == nil {
			s = &seriesSeen{dir: id.seriesDir, ids: embedded.Parent, last: map[int]int{},
				sidecars: seriesFolderSidecars(filepath.Join(b.root, filepath.FromSlash(id.seriesDir)))}
			b.series[id.seriesDir] = s
		}
		seriesSidecars = s.sidecars
		if m.series != nil {
			s.managed = m.series
		}
		if n := id.numbering(); n != nil && n.Episode > 0 {
			s.last[n.Season] = max(s.last[n.Season], n.Episode, n.EpisodeEnd)
		}
		b.seen[seriesOrigin(b.root, id.seriesDir)] = true
	}
	id.digest = identityDigest(file, m, embedded, b.im.reader(), seriesSidecars)
	return id
}

// seriesFolderSidecars stats a series folder's own tvshow.nfo and folder art. It reads the folder
// itself rather than the walk's listing, so a folder pass over one season digests the same
// sidecars a full walk does. A loose series names no folder, and an unreadable one has none.
func seriesFolderSidecars(dir string) map[string]Stat {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out map[string]Stat
	for _, e := range entries {
		name := e.Name()
		if !strings.EqualFold(name, "tvshow.nfo") && !localart.IsFolderArt(name) {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if out == nil {
			out = map[string]Stat{}
		}
		out[filepath.Join(dir, name)] = Stat{Size: info.Size(), ModifiedNs: info.ModTime().UnixNano()}
	}
	return out
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
		if day := id.airDate(); day != "" {
			res = b.byAirDate(ctx, b.series[id.seriesDir], in, res, day)
		} else {
			res = b.byNumber(ctx, b.series[id.seriesDir], in, res)
		}
	}
	resolved := res.resolved
	facts, coverage := itemFacts(resolved)
	if facts.Name == "" {
		facts.Name = fallbackName(file.Rel, id.kind)
	}
	// An episode its file names by the day it aired lists that day until a catalog says more.
	if day := id.airDate(); facts.PremiereDate == "" && day != "" {
		facts.PremiereDate = day
	}
	coverage["sources"] = inventory.CoveragePresent
	snapshot.Kind = kindOf(id.kind)
	snapshot.Observation.Facts, snapshot.Observation.Coverage = facts, coverage
	var seriesKey inventory.OriginKey
	if id.kind == mp.KindEpisode && id.seriesDir != "" {
		seriesKey = seriesOrigin(b.root, id.seriesDir)
	}
	snapshot.Observation.Extension = originExtensionOf(id.kind, res, seriesKey, fileNotes{
		Managed: len(id.managed.records) > 0, Edition: fileEdition(records), Trickplay: file.trickplay(),
		External: file.externalTracks(), Markers: readEDL(file.edl()),
	})
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
	match    librarystore.LibraryMatch
	// catalog is the catalog row its details come from, cached or not; zero for none.
	catalog librarystore.LibraryCatalogKey
	// set is a movie's ready-made set, as the catalog row it is fetched by; zero for none.
	set librarystore.LibraryCatalogKey
	// record is that row's cached record, nil until the Enricher fetches it.
	record *mp.Record
	// unnumbered is an episode its show's catalog could not number, with no answer for the show.
	unnumbered bool
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
	out.catalog = b.im.Keys.For(in.Kind, pickedIDs(out.resolved.IDs))
	if rec, ok := b.im.catalogRecord(ctx, out.catalog); ok {
		out.record = &rec
		in.Records = append(append([]mp.Record(nil), in.Records...), rec)
		out.resolved = b.im.Resolver.Resolve(in)
	}
	if in.Kind == mp.KindMovie {
		out.set = b.im.Keys.For(mp.KindCollection, []mp.ExternalID{out.resolved.Collection.Value.ID})
	}
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
		// A show with no name of its own is named by its own folder, not the folders it sits in:
		// "TV/Kids/Kettle Bay" is "Kettle Bay".
		if facts.Name == "" && s.dir != "" {
			facts.Name = showFolderName(filepath.Base(filepath.FromSlash(s.dir)))
		}
		coverage["sources"] = inventory.CoverageEmpty
		snapshot := inventory.Snapshot{
			Origin: seriesOrigin(b.root, s.dir), Kind: kindSeries,
			Observation: inventory.Observation[inventory.ItemFacts]{
				SchemaVersion: 1, ObservedAt: now, Coverage: coverage, Facts: facts,
				Extension: originExtensionOf(mp.KindSeries, res, inventory.OriginKey{}, fileNotes{}),
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
