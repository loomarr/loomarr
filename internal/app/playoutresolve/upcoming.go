package playoutresolve

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// UpcomingProgramme is one programme a channel airs in [from, to) of UpcomingProgrammes.
type UpcomingProgramme struct {
	Start, Stop   time.Time
	LibraryItemID string
	Key           provision.Key
	// Forecast is true for a programme past the rolling window on air.
	Forecast bool
}

// UpcomingProgrammes lists the programmes a channel airs in [from, to), for the stored upcoming
// airings (#4476, scheduling.md "Upcoming airings"). It is the guide's own walk
// (segmentedTimeline over timeline.BroadcastsBetween), so it lists exactly what BroadcastsBetween
// lists, without the metadata pass. Only programmes with a library item count: no filler, flex
// or pending placeholder. A programme crossing `from` keeps its true start, so the one on air is
// included.
func (r *Resolver) UpcomingProgrammes(ctx context.Context, channelID string, from, to time.Time) ([]UpcomingProgramme, error) {
	bs, forecastFrom, err := r.segmentedTimeline(ctx, channelID, from, to, timeline.BroadcastsBetween)
	if err != nil {
		return nil, err
	}
	out := make([]UpcomingProgramme, 0, len(bs))
	for _, b := range bs {
		if b.Kind != schedule.SlotProgram || b.LibraryItemID == "" || b.Nominal {
			continue
		}
		out = append(out, UpcomingProgramme{
			Start: b.Start, Stop: b.Stop, LibraryItemID: b.LibraryItemID, Key: b.Key,
			Forecast: !b.Start.Before(forecastFrom),
		})
	}
	return out, nil
}
