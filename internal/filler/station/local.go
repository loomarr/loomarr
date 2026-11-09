package station

import (
	"context"
	"log/slog"
	"strconv"
	"sync"

	"github.com/loomarr/loomarr/internal/filler/location"
)

// LocalResolver resolves a picked place to the broadcast stations within the household's local
// radius (#2059 decisions 2 and 7), for filler.Policy.LocalStations. A place the index does not
// know, a place outside the asked country, or an unreadable directory resolves to no stations, so
// local clips fail closed rather than airing in the wrong city.
type LocalResolver struct {
	Stations *Lazy
	RadiusKM func() int
	Log      *slog.Logger

	mu   sync.Mutex
	memo map[string]map[string]bool
}

// Resolve is the filler.LocalStations function: the stations near place within the radius.
func (l *LocalResolver) Resolve(country, place string) map[string]bool {
	radius := l.RadiusKM()
	key := country + "\x00" + place + "\x00" + strconv.Itoa(radius)
	l.mu.Lock()
	near, ok := l.memo[key]
	l.mu.Unlock()
	if ok {
		return near
	}
	// The directory has its own lock; a failure is not memoised, so the next break retries it.
	directory, err := l.Stations.Get(context.Background())
	if err != nil {
		l.Log.Warn("filler: broadcast stations unavailable; no clip is local", "err", err)
		return nil
	}
	// Everything below is settled for this key, including the answers that leave no clip local,
	// so each is logged once rather than on every break.
	if index, err := location.Default(); err != nil {
		l.Log.Warn("filler: place index unavailable; no clip is local", "err", err)
	} else if at, found := index.Lookup(place); !found || at.Country != country {
		l.Log.Warn("filler: place is not a city in its country; no clip is local", "place", place, "country", country)
	} else {
		near = directory.Near(country, at.Latitude, at.Longitude, float64(radius))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.memo == nil {
		l.memo = map[string]map[string]bool{}
	}
	l.memo[key] = near
	return near
}
