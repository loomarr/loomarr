package scan

import (
	"context"
	"strconv"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/metadataprovider/precedence"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// byNumber describes an episode with no TVDB id by its series' TMDB season and number (#2151
// decision 53, second case), but only when TMDB numbers the series the way the disk does: TMDB
// lists no other ordering for it, and every season on disk ends at TMDB's count for that season.
// Otherwise, or with no series record cached yet, res is returned as it was.
//
// The episode is then keyed by its season's record, cached or not, so the Enricher fetching or
// changing that record writes it again.
func (b *builder) byNumber(ctx context.Context, s *seriesSeen, in precedence.Input, res resolution) resolution {
	n := res.resolved.Numbering
	if s == nil || !n.Known() || n.Value.Episode <= 0 {
		return res
	}
	if _, onDisk := s.last[n.Value.Season]; !onDisk {
		return res
	}
	series := b.resolveSeries(ctx, s)
	if series.record == nil || !numberedLikeDisk(series.record.Descriptive, s.last) {
		return res
	}
	// A series' catalog row is its TMDB record (catalogKey); its seasons are fetched from the same
	// catalog.
	res.catalog = store.LibraryCatalogKey{
		Provider: series.catalog.Provider, Kind: string(mp.KindSeason),
		ExternalID: series.catalog.ExternalID + "/" + strconv.Itoa(n.Value.Season),
	}
	season, ok := b.season(ctx, s, res.catalog)
	if !ok {
		return res
	}
	for _, entry := range season.Children {
		if entry.Numbering == nil || entry.Numbering.Episode != n.Value.Episode {
			continue
		}
		// The file already says where it is, through its last episode; the entry says what it is.
		entry.Numbering = nil
		in.Records = append(append([]mp.Record(nil), in.Records...), entry)
		res.resolved = b.im.Resolver.Resolve(in)
		res.record = &season
		return res
	}
	return res
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
	if d == nil || d.Orderings == nil || *d.Orderings != 0 || len(last) == 0 {
		return false
	}
	size := make(map[int]int, len(d.Seasons))
	for _, s := range d.Seasons {
		size[s.Number] = s.Episodes
	}
	for season, episode := range last {
		if size[season] != episode {
			return false
		}
	}
	return true
}
