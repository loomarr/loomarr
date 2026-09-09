package libraryfixture

import (
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

// Availability supplies explicit playable and editorial evidence to pure
// scheduling tests. Missing map entries remain unavailable.
type Availability struct {
	Movies map[provision.Key]schedule.ResolvedProgram
	Series map[provision.Key]schedule.EpisodeResolution
}

func (a Availability) Resolve(key provision.Key) (string, int64, bool) {
	p, ok := a.Movies[key]
	return p.LibraryItemID, p.DurationMs, ok
}

func (a Availability) ResolveEpisodes(key provision.Key) schedule.EpisodeResolution {
	r := a.Series[key]
	r.Programs = append([]schedule.ResolvedProgram(nil), r.Programs...)
	return r
}
