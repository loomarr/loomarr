package app

import (
	"context"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/programmer"
)

// guideAdapter maps the Tunarr guide reader to api.GuideReader, converting
// programmer.GuideEntry → api.ChannelNowNext so the API package needn't import the
// programmer client (the house pattern: accept interfaces, adapt at the root).
type guideAdapter struct {
	tunarr *programmer.Tunarr
	// window is how far ahead to ask for. It only has to be long enough to contain the
	// NEXT program — asking for more just makes Tunarr do pointless work.
	window time.Duration
}

// NowNext reads the guide once for every channel and reduces each timeline to the pair a
// channel card shows. Selection lives here, not in the UI, so "what is on" means the same
// thing everywhere (§9) and the FE never reimplements interval math.
func (a guideAdapter) NowNext(ctx context.Context, now time.Time) (map[string]api.ChannelNowNext, error) {
	guide, err := a.tunarr.Guide(ctx, now, now.Add(a.window))
	if err != nil {
		return nil, err
	}
	nowMs := now.UnixMilli()
	out := make(map[string]api.ChannelNowNext, len(guide))

	for tunarrID, entries := range guide {
		var current, upcoming *api.NowNextEntry
		for _, e := range entries {
			entry := toNowNextEntry(e)
			switch {
			// Airing: start <= now < stop. Tunarr returns entries in time order, but
			// this is written as a containment test rather than "the first one", so an
			// out-of-order or overlapping guide can't mislabel what is on.
			case e.StartMs <= nowMs && nowMs < e.StopMs && current == nil:
				current = &entry
			// The earliest entry that starts after now.
			case e.StartMs > nowMs && (upcoming == nil || e.StartMs < upcoming.StartMs):
				upcoming = &entry
			}
		}
		if current == nil && upcoming == nil {
			continue // nothing scheduled in the window
		}
		out[tunarrID] = api.ChannelNowNext{Now: current, Next: upcoming}
	}
	return out, nil
}

func toNowNextEntry(e programmer.GuideEntry) api.NowNextEntry {
	return api.NowNextEntry{
		Title:   e.Title,
		StartMs: e.StartMs,
		StopMs:  e.StopMs,
		Gap:     e.IsGap(),
		TMDBID:  e.TMDBID,
	}
}
