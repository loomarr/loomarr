package station

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/loomarr/loomarr/internal/filler/research"
)

// Lazy loads the broadcast station directory on first use. Boot seeds it and nothing rewrites it
// while the process runs, so one load serves every pass; a failed load is retried.
type Lazy struct {
	list func(context.Context) ([]Station, error)
	mu   sync.Mutex
	dir  *Directory
}

// NewLazy returns a Lazy that reads the stations with list on first use.
func NewLazy(list func(context.Context) ([]Station, error)) *Lazy {
	return &Lazy{list: list}
}

// Get returns the directory, loading it if no earlier call has.
func (s *Lazy) Get(ctx context.Context) (*Directory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dir == nil {
		stations, err := s.list(ctx)
		if err != nil {
			return nil, fmt.Errorf("load broadcast stations: %w", err)
		}
		s.dir = NewDirectory(stations)
	}
	return s.dir, nil
}

// ResolveDuring adapts the directory for research, which names a call sign and a span. An
// unreadable directory resolves nothing, so the named station answers no geography.
func (s *Lazy) ResolveDuring(log *slog.Logger) research.StationResolver {
	return func(callSign, from, to string) (string, string, bool) {
		dir, err := s.Get(context.Background())
		if err != nil {
			log.Warn("filler research: broadcast stations unavailable; the named station is unresolved", "err", err)
			return "", "", false
		}
		station, ok := dir.ResolveDuring(callSign, from, to)
		return station.ID, station.Country, ok
	}
}
