package playout

import (
	"math"
	"testing"
	"time"
)

func TestCopyStartRequiresUsableKeyframeWithinRequestedInterval(t *testing.T) {
	for _, tc := range []struct {
		name, pts, flags, start string
		offset, limit           time.Duration
		fps                     float64
		want                    time.Duration
		ok                      bool
	}{
		{"opening frame", "0", "K_", "0", time.Millisecond, time.Second, 25, 0, true},
		{"late missing keyframe", "30", "K_", "0", 27 * time.Second, 3 * time.Second, 25, 0, false},
		{"preceding GOP", "20", "K_", "0", 22 * time.Second, 8 * time.Second, 25, 0, false},
		{"next frame", "3.240", "K_", "0", 3237 * time.Millisecond, time.Second, 25, 3237 * time.Millisecond, true},
		{"fractional keyframe", "0.041708", "K_", "0", 42 * time.Millisecond, time.Second, 23.976, 41 * time.Millisecond, true},
		{"outside finite end", "3.240", "K_", "0", 3237 * time.Millisecond, 2 * time.Millisecond, 25, 0, false},
		{"discarded keyframe", "0", "KD", "0", time.Millisecond, time.Second, 25, 0, false},
		{"nonkey frame", "0", "__", "0", time.Millisecond, time.Second, 25, 0, false},
		{"invalid timestamp", "NaN", "K_", "0", time.Millisecond, time.Second, 25, 0, false},
		{"unknown origin", "0", "K_", "", time.Millisecond, time.Second, 25, 0, false},
		{"invalid rate", "0", "K_", "0", time.Millisecond, time.Second, math.NaN(), 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := probed{Streams: []probedStream{{Index: 2, CodecType: "video", HasBFrames: new(int)}}, Packets: []probedPacket{{StreamIndex: 2, PTS: tc.pts, Flags: tc.flags}}, Format: probedFormat{StartTime: tc.start}}
			got, ok := copyStartOf(result, tc.offset, tc.limit, tc.fps)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("copy start=%s,%t want=%s,%t", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestCopyStartRejectsMissingOrReorderedVideo(t *testing.T) {
	for _, reordering := range []*int{nil, new(2)} {
		result := probed{Streams: []probedStream{{Index: 0, CodecType: "video", HasBFrames: reordering}}, Packets: []probedPacket{{StreamIndex: 0, PTS: "0", Flags: "K_"}}, Format: probedFormat{StartTime: "0"}}
		if _, ok := copyStartOf(result, 0, time.Second, 25); ok {
			t.Fatal("video without a zero-reordering proof was copied")
		}
	}
}
