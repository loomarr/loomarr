package api

import (
	"context"
	"fmt"
	"sync"

	"github.com/loomarr/loomarr/internal/filler/station"
)

// stationCache holds the broadcast station directory once it has been read. A failed read is
// retried by the next request.
type stationCache struct {
	mu  sync.Mutex
	dir *station.Directory
}

func (s *Server) stationDirectory(ctx context.Context) (*station.Directory, error) {
	s.stations.mu.Lock()
	defer s.stations.mu.Unlock()
	if s.stations.dir == nil {
		stations, err := s.store.ListBroadcastStations(ctx)
		if err != nil {
			return nil, fmt.Errorf("load broadcast stations: %w", err)
		}
		s.stations.dir = station.NewDirectory(stations)
	}
	return s.stations.dir, nil
}

// labelStations is the directory for naming stations in a response. Without it a clip shows its
// stored station id, so a failed read is logged rather than failing the request.
func (s *Server) labelStations(ctx context.Context) *station.Directory {
	dir, err := s.stationDirectory(ctx)
	if err != nil {
		s.log.Warn("filler: station names unavailable; showing station ids", "err", err)
	}
	return dir
}

// stationNames names a clip's station the way a viewer knows it: the call sign it held on the
// clip's air date, and a label that adds its community of license ("WEWS · Cleveland, OH"). An id
// the directory does not hold is shown as stored.
func stationNames(dir *station.Directory, id, airDate string) (callSign, label string) {
	if dir == nil || id == "" {
		return id, id
	}
	station, ok := dir.Station(id)
	if !ok {
		return id, id
	}
	callSign = station.CallSignOn(airDate)
	label = callSign
	if station.City != "" {
		label += " · " + station.City
		if station.Region != "" {
			label += ", " + station.Region
		}
	}
	return callSign, label
}
