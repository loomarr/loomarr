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

// Upcoming reads one Tunarr channel's guide and returns the program airing now (if any)
// followed by the next programs in airtime order, up to `limit` entries total. Commercial /
// flex GAPS are skipped — a viewer's "what's on later" strip is a list of shows, not the
// break padding between them (the gaps are still real playout, just not guide-worthy). Like
// NowNext, the selection lives here so the strip is the same everywhere and the FE never does
// interval math. Airtimes are Tunarr's (§6). tunarrID is the channel's Tunarr id; an unknown
// id or an empty guide yields an empty slice, not an error (a fresh channel has no guide yet).
func (a guideAdapter) Upcoming(ctx context.Context, tunarrID string, now time.Time, limit int) ([]api.NowNextEntry, error) {
	if limit <= 0 {
		limit = 6
	}
	// Ask for a wider window than NowNext (which only needs the next program) so several
	// upcoming shows fit; the guide read is one upstream call regardless of window length.
	guide, err := a.tunarr.Guide(ctx, now, now.Add(a.window))
	if err != nil {
		return nil, err
	}
	entries, ok := guide[tunarrID]
	if !ok {
		return []api.NowNextEntry{}, nil
	}
	nowMs := now.UnixMilli()
	out := make([]api.NowNextEntry, 0, limit)
	for _, e := range entries {
		if e.IsGap() {
			continue // skip commercial/flex gaps — the strip lists shows, not breaks
		}
		if e.StopMs <= nowMs {
			continue // already finished — not "now or upcoming"
		}
		out = append(out, toNowNextEntry(e))
		if len(out) >= limit {
			break
		}
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
