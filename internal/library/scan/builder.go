package scan

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
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
	// movies is the cheaply known ids of the main video in each folder, for the extras beside it.
	movies map[string][]inventory.ExternalID
}

type seriesSeen struct {
	dir     string
	managed *mp.Record
	ids     []mp.ExternalID
	write   bool
}

// identity is what the cheap sources say a file is: no file is opened to learn it.
type identity struct {
	kind     mp.Kind
	managed  managed
	embedded mp.Record
	// seriesDir is an episode's series folder, relative to the root.
	seriesDir string
	// parent is an extra's item ids.
	parent []inventory.ExternalID
	digest string
}

func newBuilder(im *Importer, root string, idx managerIndex) *builder {
	return &builder{
		im: im, root: root, idx: idx, seen: map[inventory.OriginKey]bool{},
		series: map[string]*seriesSeen{}, movies: map[string][]inventory.ExternalID{},
	}
}

// order puts main videos before extras, so an extra finds the ids of the item it belongs to.
func (b *builder) order(files []File) []File {
	out := append([]File(nil), files...)
	sort.SliceStable(out, func(i, j int) bool { return !out[i].Extra && out[j].Extra })
	return out
}

func (b *builder) identify(file File) identity {
	m := b.idx.byPath[file.Path]
	embedded, seriesDir := embeddedRecord(file)
	id := identity{kind: embedded.Kind, managed: m, embedded: embedded, seriesDir: seriesDir}
	if len(m.records) > 0 {
		id.kind = m.records[0].Kind
		if id.kind == mp.KindEpisode && seriesDir == "" {
			id.seriesDir = topFolder(file.Rel)
		}
	}
	b.seen[fileOrigin(b.root, file.Rel)] = true
	switch {
	case file.Extra:
		id.parent = b.movies[mainFolder(file.Rel)]
	case id.kind == mp.KindEpisode:
		s := b.series[id.seriesDir]
		if s == nil {
			s = &seriesSeen{dir: id.seriesDir, ids: embedded.Parent}
			b.series[id.seriesDir] = s
		}
		if m.series != nil {
			s.managed = m.series
		}
		b.seen[seriesOrigin(b.root, id.seriesDir)] = true
	default:
		var ids []mp.ExternalID
		if len(m.records) > 0 {
			ids = m.records[0].IDs
		} else {
			ids = embedded.IDs
		}
		dir := path.Dir(file.Rel)
		if _, ok := b.movies[dir]; !ok {
			b.movies[dir] = toInventory(ids)
		}
	}
	id.digest = identityDigest(file, m, embedded, id.parent)
	return id
}

// snapshot is the file's inventory item without its header facts. It reads the file's NFO
// sidecar, so it runs only for a file that is written or waits for its header.
func (b *builder) snapshot(ctx context.Context, file File, id identity, now time.Time) inventory.Snapshot {
	snapshot := inventory.Snapshot{
		Origin: fileOrigin(b.root, file.Rel),
		Observation: inventory.Observation[inventory.ItemFacts]{
			SchemaVersion: 1, ObservedAt: now,
		},
	}
	if file.Extra {
		snapshot.Kind = id.itemKind(file)
		snapshot.Observation.Facts.Name = fallbackName(file.Rel)
		for _, p := range id.parent {
			snapshot.Observation.Facts.Relationships = append(snapshot.Observation.Facts.Relationships,
				inventory.Relationship{Kind: "parent", ExternalID: p.Namespace + ":" + p.Value})
		}
		snapshot.Observation.Coverage = map[string]inventory.Coverage{"sources": inventory.CoveragePresent}
		return snapshot
	}

	records := append([]mp.Record(nil), id.managed.records...)
	if len(records) > 1 && id.kind == mp.KindEpisode {
		// A multi-episode file is one item, as the media server keeps it: its first episode,
		// numbered through its last.
		first := records[0]
		if first.Numbering != nil && records[len(records)-1].Numbering != nil {
			n := *first.Numbering
			n.EpisodeEnd = records[len(records)-1].Numbering.Episode
			first.Numbering = &n
		}
		records = []mp.Record{first}
	}
	records = append(records, id.embedded)
	if rec, ok := b.sidecar(ctx, id.kind, file.Path); ok {
		records = append(records, rec)
	}

	var series []mp.Record
	if id.kind == mp.KindEpisode {
		s := b.series[id.seriesDir]
		s.write = true
		if s.managed != nil {
			series = append(series, *s.managed)
		}
	}
	resolved := b.im.Resolver.Resolve(precedence.Input{Kind: id.kind, Records: records, Series: series})
	facts, coverage := itemFacts(resolved)
	if facts.Name == "" {
		facts.Name = fallbackName(file.Rel)
	}
	coverage["sources"] = inventory.CoveragePresent
	snapshot.Kind = id.itemKind(file)
	snapshot.Observation.Facts, snapshot.Observation.Coverage = facts, coverage
	snapshot.ExternalIDs = externalIDs(resolved.IDs)
	return snapshot
}

// sidecar reads the NFO beside a file. No NFO, or one that does not parse, says nothing.
func (b *builder) sidecar(ctx context.Context, kind mp.Kind, mediaPath string) (mp.Record, bool) {
	if b.im.NFO == nil {
		return mp.Record{}, false
	}
	rec, err := b.im.NFO.Fetch(ctx, mp.Ref{Kind: kind, ID: mp.ExternalID{Namespace: mp.NamespaceFile, Value: mediaPath}})
	if err != nil {
		if !errors.Is(err, mp.ErrNotFound) {
			b.im.log().Debug("library scan: sidecar not read", "err", err)
		}
		return mp.Record{}, false
	}
	return rec, true
}

// writeSeries writes each series one of whose episodes this pass wrote.
func (b *builder) writeSeries(ctx context.Context, now time.Time) error {
	for _, s := range b.series {
		if !s.write {
			continue
		}
		records := []mp.Record{{Source: mp.Source{Basis: mp.BasisEmbeddedID}, Kind: mp.KindSeries, IDs: s.ids}}
		if s.managed != nil {
			records = append(records, *s.managed)
		}
		if s.dir != "" {
			if rec, ok := b.sidecar(ctx, mp.KindSeries, filepath.Join(b.root, filepath.FromSlash(s.dir))); ok {
				records = append(records, rec)
			}
		}
		resolved := b.im.Resolver.Resolve(precedence.Input{Kind: mp.KindSeries, Records: records})
		facts, coverage := itemFacts(resolved)
		if facts.Name == "" {
			facts.Name = strings.TrimSpace(embeddedID.ReplaceAllString(s.dir, ""))
		}
		coverage["sources"] = inventory.CoverageEmpty
		err := b.im.apply(ctx, inventory.Snapshot{
			Origin: seriesOrigin(b.root, s.dir), Kind: kindSeries,
			Observation: inventory.Observation[inventory.ItemFacts]{
				SchemaVersion: 1, ObservedAt: now, Coverage: coverage, Facts: facts,
			},
			ExternalIDs: externalIDs(resolved.IDs),
		})
		if err != nil && !errors.Is(err, inventory.ErrIdentityConflict) {
			return err
		}
		if err != nil {
			b.im.log().Warn("library scan: a series' ids name two items", "series", s.dir)
		}
	}
	return nil
}

// itemKind is the inventory kind the file is written as.
func (id identity) itemKind(file File) inventory.ItemKind {
	if file.Extra {
		return kindExtra
	}
	return kindOf(id.kind)
}

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

func topFolder(rel string) string {
	if i := strings.Index(rel, "/"); i > 0 {
		return rel[:i]
	}
	return ""
}

// mainFolder is the folder of the item an extra belongs to: its own folder, or the one above an
// extras folder.
func mainFolder(rel string) string {
	dir := path.Dir(rel)
	if extrasDirs[strings.ToLower(path.Base(dir))] {
		return path.Dir(dir)
	}
	return dir
}
