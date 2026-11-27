package filler

import (
	"context"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// A clip's air window (#3384): the part of its file that airs. Ads off archive reels open and end on
// black, and a break of them shows seconds of dead air between spots: on one install 80 of 169
// ready clips had more than 0.2 s of black that was also silent at an edge, 86 s in all past the pad.
// The file is never touched; playout seeks to the window's start and the break books its length.

const (
	// airPadMs is the dead air left at a trimmed edge: a broadcast-style beat between spots, not a cut
	// straight into the next ad's first frame.
	airPadMs = 200
	// airEdgeSlackMs is how near the file's edge a detector run must begin (or end) to be the
	// edge's dead air: the detectors report a run from its first whole frame.
	airEdgeSlackMs = 100
)

// AirWindowFrom is the window a clip airs, from its file's quality report. Only black that is also
// silent is dead air: black under sound is a voice-over or a closing line, and stays. A report
// that is not on the clip's timeline (its length differs from the row's) trims nothing.
func AirWindowFrom(q tools.MediaQuality, durationMs int64) (startMs, endMs int64) {
	if durationMs <= 0 || q.DurationMs != durationMs {
		return 0, durationMs
	}
	startMs, endMs = 0, durationMs
	if lead := deadAirAtStart(q); lead > airPadMs {
		startMs = lead - airPadMs
	}
	if tail := deadAirFromEnd(q, durationMs); tail < durationMs-airPadMs {
		endMs = tail + airPadMs
	}
	// A clip that would keep under half its length is not an ad with dead air at its edges but a
	// problem the quality rung judges (mostly black or silent): it airs whole rather than as a
	// sliver.
	if endMs-startMs < durationMs/2 {
		return 0, durationMs
	}
	return startMs, endMs
}

// deadAirAtStart is where the black and silence that both open the file stop: the earlier of the
// two runs' ends, or 0 when either is missing.
func deadAirAtStart(q tools.MediaQuality) int64 {
	black, ok := edgeRun(q.Black, func(r tools.Interval) bool { return r.StartMs <= airEdgeSlackMs })
	if !ok {
		return 0
	}
	silence, ok := edgeRun(q.Silence, func(r tools.Interval) bool { return r.StartMs <= airEdgeSlackMs })
	if !ok {
		return 0
	}
	return min(black.EndMs, silence.EndMs)
}

// deadAirFromEnd is where the black and silence that both close the file begin: the later of the
// two runs' starts, or the file's end when either is missing.
func deadAirFromEnd(q tools.MediaQuality, durationMs int64) int64 {
	atEnd := func(r tools.Interval) bool { return r.EndMs >= durationMs-airEdgeSlackMs }
	black, ok := edgeRun(q.Black, atEnd)
	if !ok {
		return durationMs
	}
	silence, ok := edgeRun(q.Silence, atEnd)
	if !ok {
		return durationMs
	}
	return max(black.StartMs, silence.StartMs)
}

func edgeRun(runs []tools.Interval, atEdge func(tools.Interval) bool) (tools.Interval, bool) {
	for _, r := range runs {
		if r.EndMs > r.StartMs && atEdge(r) {
			return r, true
		}
	}
	return tools.Interval{}, false
}

// AirWindowStore is what the air-window sweep reads and writes.
type AirWindowStore interface {
	// ClipsWithoutAirWindow lists the airable clips whose window has not been measured.
	ClipsWithoutAirWindow(ctx context.Context) ([]clipstore.Clip, error)
	SetClipAirWindow(ctx context.Context, hash string, startMs, endMs int64, at time.Time) error
}

// WithAirWindows lets each pass measure the air window of every airable clip that has none. It is
// the one place a window is set, for a clip just made ready and for the library a release finds.
func (p *Pipeline) WithAirWindows(store AirWindowStore) *Pipeline {
	p.airWindows = store
	return p
}

// measureAirWindows sets the window of each airable clip without one, from the quality report in
// its sidecar. A clip with no readable report is set to air whole: the sweep must not re-read it
// every pass, and a replaced file is a new clip with its own unmeasured window.
func (p *Pipeline) measureAirWindows(ctx context.Context) (int, error) {
	if p.airWindows == nil || p.clipDir == "" {
		return 0, nil
	}
	clips, err := p.airWindows.ClipsWithoutAirWindow(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range clips {
		if err := ctx.Err(); err != nil {
			return n, err
		}
		if c.Path == "" || c.DurationMs <= 0 {
			continue
		}
		start, end := int64(0), c.DurationMs
		if q, ok := playbackQuality(filepath.Join(p.clipDir, filepath.FromSlash(c.Path))); ok {
			start, end = AirWindowFrom(q, c.DurationMs)
		}
		if err := p.airWindows.SetClipAirWindow(ctx, c.Hash, start, end, p.now().UTC()); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// playbackQuality is the quality report of the file that airs: the playback derivative's own, or
// the clip's report from before derivatives were recorded.
func playbackQuality(full string) (tools.MediaQuality, bool) {
	tags, ok := asset.ReadSidecarTags(full)
	if !ok {
		return tools.MediaQuality{}, false
	}
	if tags.MediaAssets != nil && tags.MediaAssets.Playback != nil {
		return tags.MediaAssets.Playback.Quality, true
	}
	if tags.MediaQuality != nil {
		return *tags.MediaQuality, true
	}
	return tools.MediaQuality{}, false
}
