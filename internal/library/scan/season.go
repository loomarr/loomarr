package scan

import (
	"context"
	"strconv"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/platform/store"
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
	case answered && answer.Choice == store.OrderByFiles:
		return res
	case answered && answer.Choice == store.OrderByOrder:
		res.catalog = orderKey(series.catalog, answer.OrderID)
		return b.byEntry(ctx, s, in, res)
	case answered && answer.Choice == store.OrderByCatalog:
	case d == nil || d.Orders == nil:
		return res
	case !numberedLikeDisk(d, s.last):
		b.wantOrders(ctx, s, series.catalog, d.Orders)
		res.unnumbered = true
		return res
	}
	res.catalog = store.LibraryCatalogKey{
		Provider: series.catalog.Provider, Kind: string(mp.KindSeason),
		ExternalID: series.catalog.ExternalID + "/" + strconv.Itoa(n.Value.Season),
	}
	return b.byEntry(ctx, s, in, res)
}

// byEntry describes the episode by the entry of res.catalog's record (a season, or an episode
// order) that numbers it as the file does: by episode in a season, which holds one season, and by
// season and episode in an order. An entry not found leaves it as it was.
func (b *builder) byEntry(ctx context.Context, s *seriesSeen, in precedence.Input, res resolution) resolution {
	rec, ok := b.season(ctx, s, res.catalog)
	if !ok {
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
		res.record = &rec
		return res
	}
	return res
}

// episodeOrder is the household's answer for which order a show's files follow, read once a pass.
func (b *builder) episodeOrder(ctx context.Context, s *seriesSeen, series store.LibraryCatalogKey) (store.LibraryEpisodeOrder, bool) {
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
func (b *builder) wantOrders(ctx context.Context, s *seriesSeen, series store.LibraryCatalogKey, orders []mp.EpisodeOrder) {
	s.seasonsMu.Lock()
	defer s.seasonsMu.Unlock()
	if s.ordersWanted || b.im.Catalog == nil || len(orders) == 0 {
		return
	}
	s.ordersWanted = true
	keys := make([]store.LibraryCatalogKey, 0, len(orders))
	for _, o := range orders {
		keys = append(keys, orderKey(series, o.ID))
	}
	if err := b.im.Catalog.WantLibraryCatalog(ctx, keys); err != nil {
		b.im.log().Warn("library scan: episode orders not asked for", "catalog", series.String(), "err", err)
	}
}

// season is a series' cached season record, read once a pass however many of its episodes ask.
func (b *builder) season(ctx context.Context, s *seriesSeen, key store.LibraryCatalogKey) (mp.Record, bool) {
	s.seasonsMu.Lock()
	defer s.seasonsMu.Unlock()
	if got, ok := s.seasons[key.ExternalID]; ok {
		return got.record, got.ok
	}
	rec, ok := b.im.catalogRecord(ctx, key)
	if s.seasons == nil {
		s.seasons = map[string]cachedSeason{}
	}
	s.seasons[key.ExternalID] = cachedSeason{record: rec, ok: ok}
	return rec, ok
}

type cachedSeason struct {
	record mp.Record
	ok     bool
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
