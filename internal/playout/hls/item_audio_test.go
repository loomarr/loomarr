package hls

import (
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/packager"
)

// itemArgsWithTrack is one packaged item's encoder args with an explicit audio track.
func itemArgsWithTrack(t *testing.T, track int) []string {
	t.Helper()
	out := playout.OutputProfile{Width: 1920, Height: 1080, FPS: 25, Quality: 23, TargetKbps: 6000, MaxKbps: 9000, GOPSeconds: 1, AudioKbps: 128}
	_, args, err := packagerItemArgs(encode.HostFor(encode.EncoderSoftware, true, encode.GPUFilters{}), out,
		PackagerItem{Input: "http://library.invalid/stream", Format: testSources()["h264-1080p-sdr-25"], AudioTrack: track},
		nil, nil, packager.Slot{Frames: 250, AudioFrames: 469}, itemFault{})
	if err != nil {
		t.Fatal(err)
	}
	return args
}

// The invariant the channel's single audio track depends on, and the reason the selection could
// not be expressed as two ffmpeg maps: EXACTLY ONE audio track, whatever was chosen.
func TestItemArgsWithAudio_MapsExactlyOneAudioTrack(t *testing.T) {
	for _, track := range []int{0, 1, 3} {
		args := itemArgsWithTrack(t, track)

		var audioMaps []string
		for i, a := range args {
			if a == "-map" && i+1 < len(args) && strings.HasPrefix(args[i+1], "0:a:") {
				audioMaps = append(audioMaps, args[i+1])
			}
		}
		if len(audioMaps) != 1 {
			t.Fatalf("track %d: %d audio maps %v, want exactly 1 — a varying track count breaks -c copy",
				track, len(audioMaps), audioMaps)
		}
		want := "0:a:" + strconv.Itoa(track)
		if audioMaps[0] != want {
			t.Errorf("track %d: mapped %q, want %q", track, audioMaps[0], want)
		}
	}
}

// An item with no preference maps the first track.
func TestItemArgs_DefaultsToTheFirstAudioTrack(t *testing.T) {
	args := itemArgsWithTrack(t, 0)
	if !containsPair(args, "-map", "0:a:0") {
		t.Fatalf("item args did not map 0:a:0; args=%v", args)
	}
}

// Video mapping is untouched by the audio work — the one thing most likely to be broken by a
// careless edit to the same line.
func TestItemArgsWithAudio_StillMapsTheFirstVideoStream(t *testing.T) {
	args := itemArgsWithTrack(t, 2)
	if !containsPair(args, "-map", "0:v:0") {
		t.Fatalf("video map missing or changed; args=%v", args)
	}
}

func containsPair(args []string, flag, val string) bool {
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == val {
			return true
		}
	}
	return false
}
