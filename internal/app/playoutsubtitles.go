package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
)

// Subtitles answer GET /v1/channels/{id}/subtitles (#2088 step 7): for each airing in [from, to)
// of the channel's wall clock, its text tracks and the picked track's cues, moved from source time
// onto the wall clock by the airing's offset. It reads the accepted lineup without resolving
// filler or recording history (AiringAt's side effects), so a window into the past or just ahead
// of live is a pure read. Breaks carry no tracks: only programmes are subtitled.

// subtitleLeadIn widens each window's source span backwards, so a cue that began just before the
// window and is still on screen is found.
const subtitleLeadIn = 10 * time.Second

// maxSubtitleAirings bounds one window's walk (a window of very short slots, or a broken lineup).
const maxSubtitleAirings = 16

func (r *playoutResolver) Subtitles(ctx context.Context, channelID string, from, to time.Time, pick *playout.SubtitlePick) ([]playout.SubtitleAiring, error) {
	var out []playout.SubtitleAiring
	for at := from; at.Before(to) && len(out) < maxSubtitleAirings; {
		slots, epoch, err := r.acceptedCycle(ctx, channelID, at)
		if err != nil {
			return nil, err
		}
		airing := playout.AiringAt(slots, epoch, at)
		stop := at.Add(airing.Remaining)
		if airing.Remaining <= 0 {
			stop = at.Add(time.Second) // never stall the walk
		}
		start := airing.StartedAt
		if start.IsZero() || start.After(at) {
			start = at
		}
		sa := playout.SubtitleAiring{Start: start, Stop: stop, Tracks: []playout.Track{}, Cues: []playout.SubtitleWallCue{}}
		if airing.Kind == schedule.SlotProgram && airing.LibraryItemID != "" {
			r.fillSubtitles(ctx, &sa, airing, at, from, to, pick)
		}
		out = append(out, sa)
		at = stop
	}
	return out, nil
}

// fillSubtitles reads the programme's text tracks and, with a pick, the cues the window shows.
// Failures degrade to no tracks: subtitles must never fail the player's request.
func (r *playoutResolver) fillSubtitles(ctx context.Context, sa *playout.SubtitleAiring, airing playout.Airing,
	at, from, to time.Time, pick *playout.SubtitlePick,
) {
	if r.lib == nil || r.probeTracks == nil {
		return
	}
	var pm library.PathMap
	if r.pathMap != nil {
		pm = r.pathMap()
	}
	input := r.lib.ResolveInput(ctx, airing.LibraryItemID, pm, library.StatReadableFile).URL
	if input == "" {
		return
	}
	tracks, err := r.subtitles.tracks(ctx, input, r.probeTracks)
	if err != nil {
		r.logDebug("playout: subtitle tracks not probed", "item", airing.LibraryItemID, "err", err)
		return
	}
	sa.Tracks = tracks
	if pick == nil || r.extractSubtitles == nil {
		return
	}
	chosen := playout.PickSubtitle(tracks, *pick)
	if chosen < 0 {
		return
	}
	// Source time at wall instant w is airing.Offset + (w - at). This part of the programme began
	// at sa.Start, so nothing before that source point airs in it.
	toSource := func(w time.Time) time.Duration { return airing.Offset + w.Sub(at) }
	lo, hi := maxTime(from, sa.Start), minTime(to, sa.Stop)
	partStart := toSource(sa.Start)
	srcLo, srcHi := max(toSource(lo)-subtitleLeadIn, partStart, 0), toSource(hi)
	for k := srcLo / playout.SubtitleChunk; k*playout.SubtitleChunk < srcHi; k++ {
		cues, err := r.subtitles.chunk(ctx, input, tracks[chosen].Index, k, r.extractSubtitles)
		if err != nil {
			r.logDebug("playout: subtitles not extracted", "item", airing.LibraryItemID, "chunk", k, "err", err)
			return
		}
		sa.Cues = append(sa.Cues, wallCues(cues, airing.Offset, at, partStart, lo, hi, sa.Stop)...)
	}
}

// wallCues places source-time cues on the wall clock: source `offset` aired at `at`. A cue from
// before this part of the programme (partStart) aired in an earlier part; a cue running past the
// airing's stop is cut there; only cues on screen in [lo, hi) are kept.
func wallCues(cues []playout.SubtitleCue, offset time.Duration, at time.Time, partStart time.Duration,
	lo, hi, stop time.Time,
) []playout.SubtitleWallCue {
	var out []playout.SubtitleWallCue
	for _, c := range cues {
		if c.Start < partStart {
			continue
		}
		start, end := at.Add(c.Start-offset), at.Add(c.End-offset)
		if end.After(stop) {
			end = stop
		}
		if end.After(lo) && start.Before(hi) {
			out = append(out, playout.SubtitleWallCue{Start: start, End: end, Text: c.Text})
		}
	}
	return out
}

func (r *playoutResolver) logDebug(msg string, args ...any) {
	if r.log != nil {
		r.log.Debug(msg, args...)
	}
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// subtitleCache holds probed text tracks per source and extracted cues per (source, track, chunk),
// so every viewer of a channel, polling every half minute, shares one probe and one extraction.
// Bounded first-in first-out: a channel walks forward through its sources, so the oldest entry is
// the one nobody needs.
type subtitleCache struct {
	mu      sync.Mutex
	entries map[string]any
	order   []string
	group   singleflight.Group
}

const subtitleCacheEntries = 256

func (c *subtitleCache) tracks(ctx context.Context, input string, probe playout.TrackProber) ([]playout.Track, error) {
	v, err := c.load("tracks|"+input, func() (any, error) {
		all, err := probe(ctx, input)
		if err != nil {
			return nil, err
		}
		text := []playout.Track{}
		for _, t := range all.Subtitles {
			if playout.IsTextSubtitle(t.Codec) {
				text = append(text, t)
			}
		}
		return text, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]playout.Track), nil
}

func (c *subtitleCache) chunk(ctx context.Context, input string, track int, k time.Duration,
	extract playout.SubtitleExtractor,
) ([]playout.SubtitleCue, error) {
	v, err := c.load(fmt.Sprintf("cues|%d|%d|%s", track, k, input), func() (any, error) {
		return extract(ctx, input, track, k*playout.SubtitleChunk, (k+1)*playout.SubtitleChunk)
	})
	if err != nil {
		return nil, err
	}
	return v.([]playout.SubtitleCue), nil
}

// load returns the cached value, or computes it once however many viewers ask at the same moment.
// Errors are not cached: the next poll tries again.
func (c *subtitleCache) load(key string, compute func() (any, error)) (any, error) {
	c.mu.Lock()
	if v, ok := c.entries[key]; ok {
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()
	v, err, _ := c.group.Do(key, func() (any, error) {
		v, err := compute()
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.entries == nil {
			c.entries = map[string]any{}
		}
		if _, ok := c.entries[key]; !ok {
			c.entries[key] = v
			c.order = append(c.order, key)
			if len(c.order) > subtitleCacheEntries {
				delete(c.entries, c.order[0])
				c.order = c.order[1:]
			}
		}
		return v, nil
	})
	return v, err
}
