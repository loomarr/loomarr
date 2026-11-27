package scan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	"github.com/loomarr/loomarr/internal/library/tmdb"
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
	// dirs are the folders, relative to the root, that hold a file the pass identified: a series'
	// key that is none is a loose show's name (joinShows).
	dirs map[string]bool
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
	// them, the series, in a full walk and a folder pass alike (#3226). sidecarsRead is whether they
	// are: only a file the pass writes needs them, so a folder pass reads no other show's folder.
	sidecars     map[string]Stat
	sidecarsRead bool
	// last is the highest episode number on disk in each season, from every file the pass
	// identified before writing any.
	last map[int]int
	// years counts the years its episodes' file names give the show ("Show (1963) - S01E01"), from
	// the same files.
	years map[int]int
	// named counts the names its episodes' file names give the show before their marks, by
	// normName: "Unit S9 S01E01" gives "unit s9".
	named map[string]int

	// resolution is the series resolved from records, its own (its folder's sidecars, read once a
	// pass) and its catalog's; resolvedAt is when, zero until it is. One still waiting for its
	// catalog record is resolved again, at most every recheckWaiting (lookedAgain).
	resolveMu  sync.Mutex
	records    []mp.Record
	resolution resolution
	resolvedAt time.Time
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
	b.artwork(ctx, id, ImportedArt, snapshot.Observation.Facts.Artwork)
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

// artwork hands the title's resolved image of each of kinds (ImportedArt for a movie or series,
// EpisodeArt for an episode), or none, to the image service. Precedence already put a local
// sidecar ahead of a provider's image (decision 19), so the first of a kind is the one. One that
// cannot be imported leaves the title's image of that kind as it was until it is written again.
// A provider's image is only recorded here, never fetched: the image service's fetch job pulls
// the bytes in bounded batches, so a series written with hundreds of episodes is no burst.
func (b *builder) artwork(ctx context.Context, id string, kinds []string, art []inventory.ArtworkRef) {
	if b.im.Artwork == nil {
		return
	}
	for _, kind := range kinds {
		ref := ""
		for _, a := range art {
			if a.Kind == kind && a.Season == 0 {
				ref = a.Ref
				break
			}
		}
		if kind == mp.KindThumb {
			// A cached record can name its still at original.
			ref = tmdb.SmallStill(ref)
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
	// named is the show an episode's file name gives before its mark, "" for none: "Brindle
	// Point" of "Brindle Point S01E01.mkv".
	named string
	// runOn is the number across its show a name read by Kodi's three digits also reads as: 214 of
	// "Cutters Bay - 214 - Late", read as season 2's 14th. Its show's catalog says which it is
	// (acrossOnly); 0 for any other name.
	runOn int
	// runTogether is a number across its show of three digits a name outside an anime's shelf and
	// untagged by a release group gives, "Orrin Vale - 101", which may be season 1's first run
	// together: its show's catalog says which (byRunTogether); 0 for any other name.
	runTogether int
	digest      string
}

// runTogetherOf is identity.runTogether for a file whose numbering is n.
func runTogetherOf(file File, n *mp.Numbering) int {
	if n == nil || n.Season != 0 || n.Episode != 0 || n.Absolute < 101 || n.Absolute > 999 || n.Absolute%100 == 0 {
		return 0
	}
	if name := path.Base(file.Rel); leadingTags.MatchString(name) || inAnime(shelved(file, path.Dir(file.Rel))) {
		return 0
	}
	return n.Absolute
}

// runOnOf is identity.runOn for a file whose numbering is n.
func runOnOf(file File, n *mp.Numbering) int {
	m := kodiTitled.FindStringSubmatch(plainStem(path.Base(file.Rel)))
	if m == nil || n == nil {
		return 0
	}
	if v := atoi(m[1]); n.Season == v/100 && n.Episode == v%100 && n.EpisodeEnd == 0 {
		return v
	}
	return 0
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
		series: map[string]*seriesSeen{}, dirs: map[string]bool{},
	}
}

// identify reads what a file is from its name, its arr's record and its own NFO. One the pass
// only sees beside those it walked (other) is never written, so it needs no digest, and its show's
// folder is not read for it.
func (b *builder) identify(file File, other bool) identity {
	m := b.idx.byPath[file.Path]
	for d := path.Dir(file.Rel); d != "." && !b.dirs[d]; d = path.Dir(d) {
		b.dirs[d] = true
	}
	embedded, seriesDir := b.im.namedOnce(file)
	// A file whose name says nothing of an episode is one when its own NFO says so: a scraper
	// placed it ("Kettle Bay - Tidewater.mkv" beside an <episodedetails>). So is a file only its
	// folder places as an episode a film when its own NFO is a <movie>: "TV/Odd/abc123.mkv" beside
	// "abc123.nfo". A file's own mark ("S01E02") or day is never overruled.
	placedOnly := embedded.Kind == mp.KindEpisode && embedded.Numbering == nil &&
		(embedded.Descriptive == nil || embedded.Descriptive.PremiereDate == "")
	if (embedded.Kind == mp.KindMovie || placedOnly) && len(m.records) == 0 && path.Dir(file.Rel) != "." {
		own := strings.ToLower(strings.TrimSuffix(file.Path, filepath.Ext(file.Path)))
		for nfo, st := range file.Sidecars {
			if !isNFO(nfo) || strings.ToLower(strings.TrimSuffix(nfo, filepath.Ext(nfo))) != own {
				continue
			}
			head := b.im.nfoRoots.headOf(nfo, st)
			switch root := head.root; {
			case root == "episodedetails" && (embedded.Kind == mp.KindMovie || placedOnly):
				if embedded.Kind == mp.KindMovie {
					seriesDir = seriesFolderOf(file)
					embedded = mp.Record{Source: embedded.Source, Kind: mp.KindEpisode, Parent: idsIn(path.Base(seriesDir))}
				}
				// The show its NFO names is the episode's when the folder it sits in is no show's of
				// that name: "TV/Unsorted/zz9.mkv" with <showtitle>Ollerton</showtitle> is Ollerton's,
				// kept as a folder of its own there would be.
				if head.show != "" && !namesShow(path.Base(seriesDir), head.show) {
					container := path.Dir(file.Rel)
					if seriesDir != "" {
						container = path.Dir(seriesDir)
					}
					seriesDir = showKey(container, head.show)
					embedded.Parent = nil
				}
			case root == "movie" && placedOnly:
				seriesDir = ""
				embedded = mp.Record{Source: embedded.Source, Kind: mp.KindMovie}
			}
		}
	}
	id := identity{kind: embedded.Kind, managed: m, embedded: embedded, seriesDir: seriesDir}
	if len(m.records) > 0 {
		id.kind = m.records[0].Kind
		if id.kind == mp.KindEpisode && seriesDir == "" {
			id.seriesDir = seriesFolderOf(file)
		}
	} else if id.kind == mp.KindEpisode {
		// An arr's record places its file itself; only the name's own reading is the catalog's to check.
		id.runOn = runOnOf(file, embedded.Numbering)
		id.runTogether = runTogetherOf(file, embedded.Numbering)
	}
	b.seen[fileOrigin(b.root, file.Rel)] = true
	var seriesSidecars map[string]Stat
	if id.kind == mp.KindEpisode {
		s := b.series[id.seriesDir]
		if s == nil {
			s = &seriesSeen{dir: id.seriesDir, ids: embedded.Parent, last: map[int]int{}, years: map[int]int{}, named: map[string]int{}}
			b.series[id.seriesDir] = s
		}
		if !other && !s.sidecarsRead {
			s.sidecars, s.sidecarsRead = seriesFolderSidecars(filepath.Join(b.root, filepath.FromSlash(id.seriesDir))), true
		}
		seriesSidecars = s.sidecars
		if m.series != nil {
			s.managed = m.series
		}
		if n := id.numbering(); n != nil && n.Episode > 0 {
			s.last[n.Season] = max(s.last[n.Season], n.Episode, n.EpisodeEnd)
		}
		if own := looseSeries(path.Base(file.Rel)); own != "" {
			s.named[normName(own)]++
			id.named = own
		}
		if year := showYearIn(path.Base(file.Rel)); year > 0 {
			s.years[year]++
		}
		b.seen[seriesOrigin(b.root, id.seriesDir)] = true
	}
	if !other {
		id.digest = identityDigest(file, m, embedded, b.im.writer(), b.im.reader(), seriesSidecars)
	}
	return id
}

// namesShow is whether a folder's name could be show's, spelled its own way: they share a word
// ("Ollerton (2010)", "The Ollerton Files" for "Ollerton"). "Unsorted" and "Downloads" share none.
func namesShow(folder, show string) bool {
	own := strings.Fields(normName(showFolderName(folder)))
	return slices.ContainsFunc(strings.Fields(normName(show)), func(w string) bool {
		return w != "and" && slices.Contains(own, w)
	})
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

// absolute is the episode's number counted across its show, when its name gives only that (an
// anime release's "Show - 013"); zero otherwise.
func (id identity) absolute() int {
	n := id.embedded.Numbering
	if len(id.managed.records) > 0 || n == nil || n.Episode > 0 {
		return 0
	}
	return n.Absolute
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
//
// waiting reports an item written without a catalog record it asked for: its own, its season's,
// or, for an episode, its series'. The file is then saved with a waiting identity (waitingFor), so
// the next pass over its folder once the record is cached writes it again, and one before leaves
// it. The Enricher asks for that pass once it has the record, but it can
// fetch the record while this pass is still writing, before the file's row is saved; the row's
// identity must not then say the file is written as it will stay (#3565).
func (b *builder) snapshot(ctx context.Context, file File, id identity, now time.Time) (snapshot inventory.Snapshot, waiting bool) {
	snapshot = inventory.Snapshot{
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
		// An episode's NFO that numbers it as another episode than its name's mark does is that
		// episode's, copied beside the wrong file: the name's number stands (precedence), and the
		// NFO's title, overview and ids, another episode's, are set aside with its number.
		if rec, ok := b.sidecar(ctx, b.im.NFO, id.kind, file.Path); ok && (id.kind != mp.KindEpisode || !numbersAnother(rec.Numbering, id.embedded.Numbering)) {
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
	var named told
	if id.kind == mp.KindMovie {
		named = b.movieTold(ctx, snapshot.Origin, file)
	}
	res := b.resolveNamed(ctx, snapshot.Origin, in, named)
	if id.kind == mp.KindEpisode && res.record == nil && !res.waiting {
		if kept, ok := b.unknownNFOAside(ctx, file, in, res); ok {
			in = kept
			res = b.resolve(ctx, snapshot.Origin, in)
		}
	}
	if id.kind == mp.KindEpisode && res.record != nil {
		res = b.ownNumberOver(ctx, snapshot.Origin, file, id, in, res)
	}
	if id.kind == mp.KindEpisode && res.catalog.String() == "" {
		if day := id.airDate(); day != "" {
			res = b.byAirDate(ctx, b.series[id.seriesDir], in, res, day)
			// A day the name writes so it reads either way, "11-05-2019", that airs none of the
			// show's episodes is the other way round when that one airs one: the catalog says how
			// the name is written.
			if other := otherReading(file, day); other != "" && res.catalog.String() == "" && !res.waiting {
				if alt := b.byAirDate(ctx, b.series[id.seriesDir], withDay(in, other), res, other); alt.catalog.String() != "" && !alt.waiting {
					res = alt
				}
			}
		} else if id.runTogether > 0 && id.absolute() == id.runTogether {
			res = b.byRunTogether(ctx, b.series[id.seriesDir], in, res, id.runTogether, toldAfterNumber(file.Rel))
		} else if abs := id.absolute(); abs > 0 {
			res = b.byAbsolute(ctx, b.series[id.seriesDir], in, res, abs, toldAfterNumber(file.Rel))
		} else if id.runOn > 0 && b.acrossOnly(ctx, b.series[id.seriesDir], *id.numbering(), id.runOn) {
			res = b.byAbsolute(ctx, b.series[id.seriesDir], in, res, id.runOn, toldAfterNumber(file.Rel))
		} else {
			res = b.byNumber(ctx, b.series[id.seriesDir], in, res, toldTitle(file.Rel))
		}
	}
	waiting = res.waiting
	if s := b.series[id.seriesDir]; id.kind == mp.KindEpisode && s != nil && b.resolveSeries(ctx, s).waiting {
		waiting = true
	}
	resolved := res.resolved
	facts, coverage := itemFacts(resolved)
	// What its NFO or catalog says the file is, before any name stands in for it.
	known := facts
	if facts.Name == "" {
		facts.Name = fallbackName(file.Rel, id.kind, file.Part, file.Sole)
		// An episode numbered across its show is named by the title after its number, as one
		// numbered "S01E02" is by the title after its mark.
		if told := toldAfterNumber(file.Rel); id.kind == mp.KindEpisode && id.absolute() > 0 && told != "" {
			facts.Name = told
		}
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
		Managed: len(id.managed.records) > 0, Edition: fileEdition(records), Part: file.Part, Trickplay: file.trickplay(),
		External: file.externalTracks(), Markers: readEDL(file.edl()),
	})
	snapshot.ExternalIDs = withFurtherIDs(externalIDs(resolved.IDs), further)
	// Every part of a movie in parts names its set, so the parts are one title from the first
	// pass, before any catalog names it.
	if id.kind == mp.KindMovie && file.Part > 0 && file.PartSet != "" {
		snapshot.ExternalIDs = append(snapshot.ExternalIDs, inventory.ExternalID{Namespace: partsNamespace, Value: partsID(file.PartSet)})
	}
	// Every copy of one episode names it alike, so its copies are one title from the first pass,
	// whatever ids each carries: a 720p beside a 1080p, a "Season 01" beside a "Season 1". One the
	// catalog describes by a title its name doesn't give ("Low Tide" for "The Ebb") is
	// joined by the catalog's, so copies it describes as one episode are one title however their
	// names call it. A name the catalog's title agrees with joins by the name, so copies named alike
	// stay one title though only one is described yet, or only one is in Needs you.
	if id.kind == mp.KindEpisode {
		title := toldTitle(file.Rel)
		if title != "" && res.record != nil && !res.unnumbered && facts.Name != "" && !oneTitle(facts.Name, title) {
			title = facts.Name
		}
		if key := episodeCopyID(b.root, id.seriesDir, title, id.embedded.Numbering); key != "" {
			snapshot.ExternalIDs = append(snapshot.ExternalIDs, inventory.ExternalID{Namespace: copiesNamespace, Value: key})
		}
	}
	// A film's copies in its own folder are one title too, though the catalog grounds only one:
	// "Harrow Fen (2011) - X265 EAC3 5.1" beside the Radarr copy its NFO names. The id is
	// inferred, so the store joins a copy a catalog grounds as another film to no title by it.
	if id.kind == mp.KindMovie {
		if key := filmCopyID(b.root, file.Rel, known.Name, known.ProductionYear); key != "" {
			snapshot.ExternalIDs = append(snapshot.ExternalIDs, inventory.ExternalID{Namespace: filmCopiesNamespace, Value: key})
		}
	}
	return snapshot, waiting
}

// filmCopiesNamespace is copiesNamespace's for a film's copies. It is never listed.
const filmCopiesNamespace = inventory.InferredNamespacePrefix + "film"

// filmCopyID is the id a film's copies in its own folder share: the folder, with its title and
// year, for a file whose name gives that title, small words aside, and that year or none ("Harrow
// Fen.mkv", "Harrow Fen (2011) - 1080p.mkv" in "Harrow Fen (2011)/"). A file named otherwise is
// another film kept there ("Harrow Fen 2 (2013)", "Harrow Fen (1985)"), and a folder with no year
// is no one film's: "" for those, and for a file in no film's folder.
//
// What the file's own NFO or catalog record says it is (title, year) outranks its name: "Harrow
// Fen.mkv" whose NFO says 1985 is the 1985 film kept in the 2011 one's folder. A catalog's year a
// year off its folder's is the same film, as catalogs date a film by its release in one country
// or another; a title sharing no word with the folder's is another film.
func filmCopyID(root, rel, title string, year int) string {
	dir := filmFolder(path.Dir(rel))
	if dir == "." || shelfAt(dir) {
		return ""
	}
	folderTitle, folderYear, _ := searchQuery(path.Base(dir), 0)
	if folderYear == 0 || folderTitle == "" {
		return ""
	}
	named, namedYear, _ := searchQuery(plainStem(path.Base(rel)), 0)
	if (namedYear != 0 && namedYear != folderYear) || !sameWords(named, folderTitle) {
		return ""
	}
	if year != 0 && (year < folderYear-1 || year > folderYear+1) || title != "" && apart(title, folderTitle) {
		return ""
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", Authority(root), dir, folderYear)))
	return hex.EncodeToString(sum[:16])
}

// copiesNamespace is the scanner's own id for an episode's copies: every copy of one episode
// carries the same value, so they are one title before any catalog names it. It is never listed.
const copiesNamespace = inventory.InferredNamespacePrefix + "episode"

// episodeCopyID is the id an episode's copies share: its series folder, its own mark's season and
// episodes, and its title (the catalog's, or else the one its name gives after the mark), small
// words aside and the rest run together ("Gate Ways" is "Gateways"). Two files that number one
// episode but name it apart ("The Pit", "Low Ground") with no catalog to say otherwise are two
// episodes, one of them misnumbered, and never one title's copies; a name with no title of its own
// joins only another with none. "" for an episode with no series folder or no mark of its own.
func episodeCopyID(root, seriesDir, title string, n *mp.Numbering) string {
	if seriesDir == "" || n == nil || n.Episode <= 0 {
		return ""
	}
	var words []string
	for _, w := range strings.Fields(normName(title)) {
		if !smallWord[w] {
			words = append(words, w)
		}
	}
	key := fmt.Sprintf("%s\x00%d/%d-%d\x00%s", seriesOrigin(root, seriesDir).ExternalItemID, n.Season, n.Episode,
		max(n.EpisodeEnd, n.Episode), strings.Join(words, ""))
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
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
	// entry is the entry of a season or order record that describes an episode; nil for none.
	entry *mp.Record
	// unnumbered is an episode its show's catalog could not number, with no answer for the show.
	unnumbered bool
	// waiting is a title written without a catalog record it asked for and the Enricher has not
	// fetched yet: its own, or for an episode its season's.
	waiting bool
}

// resolve applies precedence. A movie or series the maintainer linked to a title (Needs you) has
// the link as a record of its own, below what the files say (decision 27). One whose ids then name
// a cached catalog record is resolved again with it, so its details come from the catalog
// (decision 7); one not cached yet is written without, and again once the Enricher fetches it.
func (b *builder) resolve(ctx context.Context, origin inventory.OriginKey, in precedence.Input) resolution {
	return b.resolveNamed(ctx, origin, in, told{})
}

// resolveNamed is resolve for a movie or a series whose names say what it is (named).
func (b *builder) resolveNamed(ctx context.Context, origin inventory.OriginKey, in precedence.Input, named told) resolution {
	if rec, ok := b.im.linkRecord(ctx, in.Kind, origin); ok {
		in.Records = append(append([]mp.Record(nil), in.Records...), rec)
	}
	out := resolution{resolved: b.im.Resolver.Resolve(in)}
	// The catalog record below adds details, never identity: what decides is known now.
	if in.Kind == mp.KindMovie || in.Kind == mp.KindSeries {
		out.match = match(out.resolved, in.Records)
	}
	out.catalog = b.im.Keys.For(in.Kind, pickedIDs(out.resolved.IDs))
	rec, ok, waiting := b.im.catalogRecord(ctx, out.catalog)
	out.waiting = waiting
	titled := in.Kind == mp.KindMovie || in.Kind == mp.KindSeries
	if ok && titled {
		// A title its NFO knows by another catalog's id is keyed by the catalog's own once found, as
		// its seasons and orders are; the Enricher caches the record by both.
		if own := b.im.Keys.For(in.Kind, rec.IDs); own.String() != "" && own != out.catalog {
			if canon, cached, _ := b.im.catalogRecord(ctx, own); cached {
				out.catalog, rec = own, canon
			}
		}
	}
	// An id only a name or an NFO gives, that the catalog was asked for and does not know, grounds
	// nothing: the title is the household's to say (Needs you), not one matched in silence. Nor
	// does one whose record is another title than the files name: an NFO copied from another
	// film's folder, or an id typed wrong.
	fromFiles := out.match.Basis == string(mp.BasisSidecar) || out.match.Basis == string(mp.BasisEmbeddedID)
	setAside := false
	if ok && titled && fromFiles && namesAnother(out.resolved, named, rec) {
		ok, rec, setAside = false, mp.Record{}, true
		out.match, out.catalog = librarystore.LibraryMatch{}, librarystore.LibraryCatalogKey{}
	}
	// Before the catalog answers, the NFO's own words are all there is: one that names the first
	// film inside its sequel's folder is set aside the same way.
	if !ok && titled && fromFiles && in.Kind == mp.KindMovie {
		for _, own := range in.Records {
			if own.Source.Basis == mp.BasisSidecar && own.Descriptive != nil &&
				namesItsSequel(own.Descriptive.Title, named.name.title, named.folder.title) {
				setAside = true
				out.match, out.catalog, out.waiting = librarystore.LibraryMatch{}, librarystore.LibraryCatalogKey{}, false
				break
			}
		}
	}
	if !ok && !waiting && titled && fromFiles && b.im.catalogMisses(ctx, out.catalog) {
		out.match, setAside = librarystore.LibraryMatch{}, true
	}
	if setAside {
		// An id that grounds nothing joins nothing either: the store would make the sequel whose
		// NFO was copied from the first film's folder a copy of the first film.
		out.resolved.IDs = slices.DeleteFunc(slices.Clone(out.resolved.IDs), func(id precedence.Picked[mp.ExternalID]) bool {
			return id.From.Basis == mp.BasisSidecar || id.From.Basis == mp.BasisEmbeddedID
		})
	}
	if ok {
		out.record = &rec
		in.Records = append(append([]mp.Record(nil), in.Records...), rec)
		out.resolved = b.im.Resolver.Resolve(in)
	}
	if in.Kind == mp.KindMovie {
		out.set = b.im.Keys.For(mp.KindCollection, []mp.ExternalID{out.resolved.Collection.Value.ID})
	}
	return out
}

// numbersAnother says whether an NFO's numbering names another episode than a file's own mark: both
// number a season's episode, and the season or the episode differs. A name that numbers it only
// across the show, or not at all, doesn't say.
func numbersAnother(nfo, own *mp.Numbering) bool {
	return nfo != nil && own != nil && nfo.Episode > 0 && own.Episode > 0 &&
		(nfo.Season != own.Season || nfo.Episode != own.Episode)
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
	s.resolveMu.Lock()
	defer s.resolveMu.Unlock()
	if !s.resolvedAt.IsZero() && !b.lookedAgain(s.resolution.waiting, s.resolvedAt) {
		return s.resolution
	}
	if s.resolvedAt.IsZero() {
		s.records = []mp.Record{{Source: mp.Source{Basis: mp.BasisEmbeddedID}, Kind: mp.KindSeries, IDs: s.ids}}
		if s.managed != nil {
			s.records = append(s.records, *s.managed)
		}
		if s.dir != "" {
			dir := filepath.Join(b.root, filepath.FromSlash(s.dir))
			providers := []mp.Provider{b.im.NFO}
			if b.im.Art != nil {
				providers = append(providers, b.im.Art)
			}
			for _, p := range providers {
				if rec, ok := b.sidecar(ctx, p, mp.KindSeries, dir); ok {
					s.records = append(s.records, rec)
				}
			}
		}
	}
	s.resolution = b.resolveNamed(ctx, seriesOrigin(b.root, s.dir), precedence.Input{Kind: mp.KindSeries, Records: s.records}, told{name: saidBy(path.Base(s.dir))})
	s.resolvedAt = b.im.now()
	return s.resolution
}

// recheckWaiting is how long a pass keeps a catalog record it found still to come before it looks
// again. On a first import the Enricher fetches what the walk asked for while the headers are
// read, so the reads that follow describe their episodes by what it has fetched, rather than
// each episode being written waiting and then again once its pass is over.
const recheckWaiting = 10 * time.Second

// lookedAgain says whether a record read at `at` and still waiting then is due a second look.
func (b *builder) lookedAgain(waiting bool, at time.Time) bool {
	return waiting && b.im.now().Sub(at) >= recheckWaiting
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
			base := filepath.Base(filepath.FromSlash(s.dir))
			facts.Name = showFolderName(base)
			// A show its own episodes name as its folder does, "Unit S9/Unit S9 S01E01.mkv" or
			// "Brockle 1x1/Brockle 1x1 S01E01.mkv", is called so: what reads as a season's mark after
			// the name is the name's.
			if whole := strings.TrimSpace(embeddedID.ReplaceAllString(base, "")); whole != facts.Name && s.named[normName(whole)] > 0 {
				facts.Name = whole
			}
			// A tagged release's folder holding one episode, "[Grp] Ame Furu - 07 [1080p] [ABCD1234]",
			// names its show before the number (showIdentity).
			if bare := strings.TrimSpace(trimReleaseNotes(leadingTags.ReplaceAllString(base, ""))); bare != base {
				if m := animeDashed.FindStringSubmatch(bare); m != nil && strings.TrimSpace(m[1]) != "" {
					facts.Name = strings.TrimSpace(m[1])
				}
			}
			// A show's specials kept in a folder of their own with no show's above it, "Holly Day
			// Specials/Holly Day S00E05.mkv", are the show its episodes name.
			if m := showSpecials.FindStringSubmatch(facts.Name); m != nil && s.named[normName(m[1])] > 0 {
				facts.Name = strings.TrimSpace(m[1])
			}
			// So is one whose folder reads its first word as its year, "2020.Vision.Street", when its
			// episodes name it whole with spaces, "2020 Vision Street S01E01": the year is the title's.
			if title, year, _ := searchQuery(facts.Name, 0); year != 0 && !strings.Contains(facts.Name, " ") && s.named[normName(facts.Name)] > 0 {
				if spaced := strings.Join(strings.FieldsFunc(facts.Name, func(r rune) bool { return r == '.' || r == '_' }), " "); spaced != title {
					facts.Name = spaced
				}
			}
		}
		// A show whose folder and NFO give no year takes the one its episodes' names agree on.
		if _, named, _ := searchQuery(facts.Name, 0); facts.ProductionYear == 0 && named == 0 && len(s.years) == 1 {
			for year := range s.years {
				facts.ProductionYear = year
			}
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
