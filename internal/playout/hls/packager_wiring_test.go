package hls

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/packager"
)

// An item's image subtitles are burned with no channel bug: the burn alone turns the overlay on.
func TestPackagerItemArgsBurnSubtitlesWithoutABug(t *testing.T) {
	host := encode.HostProfile{Family: encode.FamilyNVENC, DecodeCodecs: encode.CUDADecodes, CPUTonemap: true}
	out := playout.OutputProfile{Width: 1920, Height: 1080, FPS: 25, Quality: 23, TargetKbps: 6000, MaxKbps: 9000, GOPSeconds: 1, AudioKbps: 128}
	burn := &playout.SubtitleBurn{List: "cues.txt", Width: 1920, Height: 1080}
	pl, args, err := packagerItemArgs(host, out, PackagerItem{Input: "movie.mkv", Format: testSources()["h264-1080p-sdr-25"]},
		nil, burn, packager.Slot{Frames: 25, AudioFrames: 47}, itemFault{})
	if err != nil {
		t.Fatal(err)
	}
	if !pl.Subtitles || !strings.Contains(strings.Join(args, " "), "cues.txt") {
		t.Fatalf("the cue list is not burned in: subtitles %v, fallbacks %q, args %q", pl.Subtitles, pl.Fallbacks, args)
	}
}

// A new packager sweeps the roots no process owns before it takes its own.
func TestNewPackagerHLSSweepsAbandonedRoots(t *testing.T) {
	base := t.TempDir()
	abandoned := filepath.Join(base, "loomarr-packager-1")
	if err := os.MkdirAll(abandoned, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(abandoned, old, old); err != nil {
		t.Fatal(err)
	}
	m, err := NewPackagerHLS(nil, "ffmpeg", base, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	if _, err := os.Stat(abandoned); !os.IsNotExist(err) {
		t.Errorf("the abandoned root survived: %v", err)
	}
	if _, err := os.Stat(m.root); err != nil {
		t.Errorf("the packager's own root: %v", err)
	}
}
