package filler

import (
	"testing"

	"github.com/loomarr/loomarr/internal/media/tools"
)

func iv(start, end int64) tools.Interval { return tools.Interval{StartMs: start, EndMs: end} }

func TestAirWindowFrom(t *testing.T) {
	const d = 30_000
	for _, tc := range []struct {
		name               string
		q                  tools.MediaQuality
		wantStart, wantEnd int64
	}{
		{name: "no dead air", q: tools.MediaQuality{DurationMs: d}, wantStart: 0, wantEnd: d},
		{name: "black and silent opening", wantStart: 1_300, wantEnd: d,
			q: tools.MediaQuality{DurationMs: d, Black: []tools.Interval{iv(0, 1_500)}, Silence: []tools.Interval{iv(0, 1_800)}}},
		{name: "black and silent close", wantStart: 0, wantEnd: 28_200,
			q: tools.MediaQuality{DurationMs: d, Black: []tools.Interval{iv(28_000, d)}, Silence: []tools.Interval{iv(27_500, d)}}},
		{name: "both edges", wantStart: 800, wantEnd: 29_600,
			q: tools.MediaQuality{DurationMs: d,
				Black:   []tools.Interval{iv(40, 1_000), iv(29_400, d)},
				Silence: []tools.Interval{iv(0, 1_200), iv(29_400, d)}}},
		// Black under sound is a voice-over or a closing line, not dead air.
		{name: "black under sound stays", wantStart: 0, wantEnd: d,
			q: tools.MediaQuality{DurationMs: d, Black: []tools.Interval{iv(0, 2_000), iv(27_000, d)}}},
		{name: "silence under picture stays", wantStart: 0, wantEnd: d,
			q: tools.MediaQuality{DurationMs: d, Silence: []tools.Interval{iv(0, 2_000), iv(27_000, d)}}},
		{name: "edge within the pad is kept", wantStart: 0, wantEnd: d,
			q: tools.MediaQuality{DurationMs: d, Black: []tools.Interval{iv(0, 150)}, Silence: []tools.Interval{iv(0, 150)}}},
		// A run that does not touch the edge is inside the ad, not at its edge.
		{name: "interior run is not an edge", wantStart: 0, wantEnd: d,
			q: tools.MediaQuality{DurationMs: d, Black: []tools.Interval{iv(500, 2_000)}, Silence: []tools.Interval{iv(500, 2_000)}}},
		{name: "report from another timeline", wantStart: 0, wantEnd: d,
			q: tools.MediaQuality{DurationMs: d + 40, Black: []tools.Interval{iv(0, 2_000)}, Silence: []tools.Interval{iv(0, 2_000)}}},
		{name: "mostly dead air airs whole", wantStart: 0, wantEnd: d,
			q: tools.MediaQuality{DurationMs: d, Black: []tools.Interval{iv(0, 16_000)}, Silence: []tools.Interval{iv(0, 16_000)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end := AirWindowFrom(tc.q, d)
			if start != tc.wantStart || end != tc.wantEnd {
				t.Errorf("window = [%d, %d), want [%d, %d)", start, end, tc.wantStart, tc.wantEnd)
			}
		})
	}
}
