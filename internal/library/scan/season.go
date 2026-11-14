package scan

import (
	"context"
	"slices"
	"strconv"

	"github.com/loomarr/loomarr/internal/library/inventory"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/library/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// byNumber describes an episode with no TVDB id by its series' catalog, in the order its files
// follow (#2151 decision 53):
//   - the order the household picked for the show: the catalog's own seasons, one of its other
//     orders, or the names and numbers in the files;
//   - with no answer, the catalog's own seasons when they number the series the way the disk does:
//     the catalog lists no other order for it, and every season on disk ends at its count;
//   - otherwise the episode keeps its file's name and number and is marked unnumbered, which puts
//     its show in Needs you, and the catalog's other orders are asked for so Needs you can say
//     which one fits.
//
// With no series record cached yet, or one that does not say what orders it has, res is returned
// as it was, unmarked: the episode is written again once the record says. An episode numbered by
// a record is keyed by it, cached or not, so the Enricher fetching or changing it writes it again.
func (b *builder) byNumber(ctx context.Context, s *seriesSeen, in precedence.Input, res resolution) resolution {
	n := res.resolved.Numbering
	if s == nil || !n.Known() || n.Value.Episode <= 0 {
		return res
	}
	if _, onDisk := s.last[n.Value.Season]; !onDisk {
		return res
	}
	series := b.resolveSeries(ctx, s)
	if series.record == nil || series.catalog.String() == "" {
		return res
	}
	d := series.record.Descriptive
	answer, answered := b.episodeOrder(ctx, s, series.catalog)
	switch {
	case answered && answer.Choice == librarystore.OrderByFiles:
		return res
	case answered && answer.Choice == librarystore.OrderByOrder:
		res.catalog = orderKey(series.catalog, answer.OrderID)
		return b.byEntry(ctx, s, in, res)
	case answered && answer.Choice == librarystore.OrderByCatalog:
	case d == nil || d.Orders == nil:
		return res
	case !numberedLikeDisk(d, s.last):
		b.wantOrders(ctx, s, series.catalog, d.Orders)
		res.unnumbered = true
		return res
	}
	res.catalog = librarystore.LibraryCatalogKey{
		Provider: series.catalog.Provider, Kind: string(mp.KindSeason),
		ExternalID: series.catalog.ExternalID + "/" + strconv.Itoa(n.Value.Season),
	}
	return b.byEntry(ctx, s, in, res)
}

// ownNumberOver checks the catalog record an episode's own ids found (res.record, through its NFO's
// or its name's TVDB id) against the file's own mark. An NFO copied beside the wrong file carries
// another episode's id, and the catalog's TVDB mapping can point one along; both show the episode
// as another. When the show's catalog numbers it as the disk does (byNumber), its entry at the
// file's mark is what the file is: an entry that is not the id's record (they share no id) stands
// over it, and the file is described, and keyed, as its mark says. An arr's record places its
// file itself, and a show numbered otherwise keeps the id's record; so does an episode whose
// season is not cached yet, which is written again once it is.
func (b *builder) ownNumberOver(ctx context.Context, origin inventory.OriginKey, id identity, in precedence.Input, res resolution) resolution {
	own := id.embedded.Numbering
	if len(id.managed.records) > 0 || own == nil || own.Episode <= 0 {
		return res
	}
	bare := in
	bare.Records = make([]mp.Record, 0, len(in.Records))
	for _, r := range in.Records {
		r.IDs = nil
		bare.Records = append(bare.Records, r)
	}
	alt := b.byNumber(ctx, b.series[id.seriesDir], bare, b.resolve(ctx, origin, bare))
	if alt.entry == nil {
		res.waiting = res.waiting || alt.waiting
		return res
	}
	if slices.ContainsFunc(alt.entry.IDs, func(e mp.ExternalID) bool { return slices.Contains(res.record.IDs, e) }) {
		return res
	}
	return alt
}

// byAbsolute places an episode its file numbers across the whole show ("Show - 013") by its series'
// catalog: the seasons, specials aside, in order, each as long as the catalog says, hold the
// episodes one after another, so the 13th of a show of two 12-episode seasons is season 2's 1st.
// It is then described by that season's entry, as a name's "S02E01" would be. With no series
// record cached yet, or one too short to hold it, res is returned as it was.
func (b *builder) byAbsolute(ctx context.Context, s *seriesSeen, in precedence.Input, res resolution, abs int) resolution {
	if s == nil {
		return res
	}
	series := b.resolveSeries(ctx, s)
	if series.record == nil || series.catalog.String() == "" || series.record.Descriptive == nil {
		return res
	}
	season, episode, ok := absoluteAt(series.record.Descriptive.Seasons, abs)
	if !ok {
		return res
	}
	return b.placeAt(ctx, s, series, in, res, mp.Numbering{Season: season, Episode: episode, Absolute: abs})
}

// byRunTogether places an episode its name numbers across the show in three digits, "Orrin Vale -
// 101", that its show's catalog can't hold so, by its season and episode run together: the 101st
// of a show of 24 is season 1's first, when season 1 holds one (identity.runTogether).
func (b *builder) byRunTogether(ctx context.Context, s *seriesSeen, in precedence.Input, res resolution, n int) resolution {
	if s == nil {
		return res
	}
	series := b.resolveSeries(ctx, s)
	if series.record == nil || series.catalog.String() == "" || series.record.Descriptive == nil {
		return res
	}
	seasons := series.record.Descriptive.Seasons
	if _, _, ok := absoluteAt(seasons, n); ok || !slices.ContainsFunc(seasons, func(size mp.SeasonSize) bool {
		return size.Number == n/100 && size.Episodes >= n%100
	}) {
		return b.byAbsolute(ctx, s, in, res, n)
	}
	return b.placeAt(ctx, s, series, in, res, mp.Numbering{Season: n / 100, Episode: n % 100})
}

// placeAt describes an episode by its series' catalog at, as a name's "S02E01" would: the file's
// own record says at, and the season's entry describes it.
func (b *builder) placeAt(ctx context.Context, s *seriesSeen, series resolution, in precedence.Input, res resolution, at mp.Numbering) resolution {
	records := make([]mp.Record, 0, len(in.Records))
	for _, r := range in.Records {
		if r.Source.Basis == mp.BasisEmbeddedID {
			r.Numbering = &at
		}
		records = append(records, r)
	}
	in.Records = records
	res.resolved = b.im.Resolver.Resolve(in)
	res.catalog = librarystore.LibraryCatalogKey{
		Provider: series.catalog.Provider, Kind: string(mp.KindSeason),
		ExternalID: series.catalog.ExternalID + "/" + strconv.Itoa(at.Season),
	}
	return b.byEntry(ctx, s, in, res)
}

// acrossOnly says a show's catalog can't hold an episode where its name's three digits place it
// (identity.runOn), "214" as season 2's 14th, but holds the show's 214th: its seasons are known,
// the one named holds fewer episodes, and all of them hold that many. A season whose size the
// record doesn't say holds it; with no series record cached yet the name's reading stands, and
// the episode is written again once the record comes.
func (b *builder) acrossOnly(ctx context.Context, s *seriesSeen, n mp.Numbering, runOn int) bool {
	if s == nil {
		return false
	}
	series := b.resolveSeries(ctx, s)
	if series.record == nil || series.record.Descriptive == nil {
		return false
	}
	seasons := series.record.Descriptive.Seasons
	if len(seasons) == 0 || slices.ContainsFunc(seasons, func(size mp.SeasonSize) bool {
		return size.Number == n.Season && (size.Episodes <= 0 || size.Episodes >= n.Episode)
	}) {
		return false
	}
	_, _, ok := absoluteAt(seasons, runOn)
	return ok
}

// absoluteAt is where the abs-th episode of a show sits in its seasons, specials aside.
func absoluteAt(seasons []mp.SeasonSize, abs int) (int, int, bool) {
	sorted := slices.SortedFunc(slices.Values(seasons), func(a, b mp.SeasonSize) int { return a.Number - b.Number })
	for _, size := range sorted {
		if size.Number <= 0 || size.Episodes <= 0 {
			continue
		}
		if abs <= size.Episodes {
			return size.Number, abs, true
		}
		abs -= size.Episodes
	}
	return 0, 0, false
}

// byEntry describes the episode by the entry of res.catalog's record (a season, or an episode
// order) that numbers it as the file does: by episode in a season, which holds one season, and by
// season and episode in an order. An entry not found leaves it as it was; a record still to come
// leaves it waiting.
func (b *builder) byEntry(ctx context.Context, s *seriesSeen, in precedence.Input, res resolution) resolution {
	rec, ok, waiting := b.season(ctx, s, res.catalog)
	if !ok {
		res.waiting = res.waiting || waiting
		return res
	}
	n := res.resolved.Numbering.Value
	for _, entry := range rec.Children {
		if entry.Numbering == nil || entry.Numbering.Episode != n.Episode ||
			(rec.Kind == mp.KindEpisodeOrder && entry.Numbering.Season != n.Season) {
			continue
		}
		// The file already says where it is, through its last episode; the entry says what it is.
		entry.Numbering = nil
		in.Records = append(append([]mp.Record(nil), in.Records...), entry)
		res.resolved = b.im.Resolver.Resolve(in)
		res.record, res.entry = &rec, &entry
		return res
	}
	return res
}

// byAirDate describes an episode its file names by the day it aired, by the series' catalog entry
// that aired that day: the entry's season and number stand over any its NFO gives, which may be
// another catalog's numbering. The day names one episode whatever order the household picked.
//
// Each of the series' seasons is read from the cache, and any not cached yet is asked for. While
// the day is not found the episode keeps its file's name, keyed by a season still to come, so the
// Enricher fetching it writes the episode again.
func (b *builder) byAirDate(ctx context.Context, s *seriesSeen, in precedence.Input, res resolution, day string) resolution {
	if s == nil {
		return res
	}
	series := b.resolveSeries(ctx, s)
	if series.record == nil || series.catalog.String() == "" || series.record.Descriptive == nil {
		return res
	}
	var waiting librarystore.LibraryCatalogKey
	asked := false
	for _, size := range series.record.Descriptive.Seasons {
		key := librarystore.LibraryCatalogKey{
			Provider: series.catalog.Provider, Kind: string(mp.KindSeason),
			ExternalID: series.catalog.ExternalID + "/" + strconv.Itoa(size.Number),
		}
		rec, ok, toCome := b.season(ctx, s, key)
		if !ok {
			if waiting.String() == "" {
				waiting = key
			}
			asked = asked || toCome
			continue
		}
		for _, entry := range rec.Children {
			if entry.Descriptive == nil || entry.Descriptive.PremiereDate != day || entry.Numbering == nil {
				continue
			}
			// The name's day is where the file is: it says the entry's season and number, as a name's
			// "S02E02" would. A catalog entry never places a file itself (precedence).
			at := *entry.Numbering
			records := make([]mp.Record, 0, len(in.Records)+1)
			for _, r := range in.Records {
				r.Numbering = nil
				if r.Source.Basis == mp.BasisEmbeddedID {
					r.Numbering = &at
				}
				records = append(records, r)
			}
			entry.Numbering = nil
			in.Records = append(records, entry)
			res.resolved = b.im.Resolver.Resolve(in)
			res.record, res.catalog = &rec, key
			return res
		}
	}
	res.catalog = waiting
	res.waiting = res.waiting || asked
	return res
}

// episodeOrder is the household's answer for which order a show's files follow, read once a pass.
func (b *builder) episodeOrder(ctx context.Context, s *seriesSeen, series librarystore.LibraryCatalogKey) (librarystore.LibraryEpisodeOrder, bool) {
	s.orderOnce.Do(func() {
		if b.im.Answers == nil {
			return
		}
		got, ok, err := b.im.Answers.LibraryEpisodeOrderFor(ctx, series)
		if err != nil {
			b.im.log().Warn("library scan: episode order not read", "catalog", series.String(), "err", err)
			return
		}
		s.order, s.answered = got, ok
	})
	return s.order, s.answered
}

// wantOrders asks, once a pass, for the layout of each of a show's other episode orders, so Needs
// you can say which one its files fit.
func (b *builder) wantOrders(ctx context.Context, s *seriesSeen, series librarystore.LibraryCatalogKey, orders []mp.EpisodeOrder) {
	s.seasonsMu.Lock()
	defer s.seasonsMu.Unlock()
	if s.ordersWanted || b.im.Catalog == nil || len(orders) == 0 {
		return
	}
	s.ordersWanted = true
	keys := make([]librarystore.LibraryCatalogKey, 0, len(orders))
	for _, o := range orders {
		keys = append(keys, orderKey(series, o.ID))
	}
	if err := b.im.Catalog.WantLibraryCatalog(ctx, keys); err != nil {
		b.im.log().Warn("library scan: episode orders not asked for", "catalog", series.String(), "err", err)
	}
}

// season is a series' cached season record, read once a pass however many of its episodes ask.
// waiting reports one asked for and not fetched yet.
func (b *builder) season(ctx context.Context, s *seriesSeen, key librarystore.LibraryCatalogKey) (rec mp.Record, ok, waiting bool) {
	s.seasonsMu.Lock()
	defer s.seasonsMu.Unlock()
	if got, ok := s.seasons[key.ExternalID]; ok {
		return got.record, got.ok, got.waiting
	}
	rec, ok, waiting = b.im.catalogRecord(ctx, key)
	if s.seasons == nil {
		s.seasons = map[string]cachedSeason{}
	}
	s.seasons[key.ExternalID] = cachedSeason{record: rec, ok: ok, waiting: waiting}
	return rec, ok, waiting
}

type cachedSeason struct {
	record      mp.Record
	ok, waiting bool
}

// numberedLikeDisk reports whether a series' catalog numbers its episodes as last, the highest
// episode on disk in each season, does: it knows no other ordering, and each of those seasons
// holds exactly that many episodes there. A record that does not say is not trusted.
func numberedLikeDisk(d *mp.Descriptive, last map[int]int) bool {
	if d == nil || d.Orders == nil || len(d.Orders) != 0 {
		return false
	}
	return fitsDisk(d.Seasons, last)
}

// fitsDisk reports whether a layout's seasons end where the disk's do: each season on disk holds
// exactly as many episodes there as last's highest. Nothing on disk fits nothing.
func fitsDisk(seasons []mp.SeasonSize, last map[int]int) bool {
	if len(last) == 0 {
		return false
	}
	size := make(map[int]int, len(seasons))
	for _, s := range seasons {
		size[s.Number] = s.Episodes
	}
	for season, episode := range last {
		if size[season] != episode {
			return false
		}
	}
	return true
}
