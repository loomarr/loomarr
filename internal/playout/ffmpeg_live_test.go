//go:build ffmpeg

// Tests that EXECUTE ffmpeg. Behind a build tag because unit tests must not depend on an
// external binary being present — the same reasoning AGENTS.md applies to the network. Run
// with `make test-ffmpeg`.
//
// These exist because arg-shape tests assert my own stated invariants and cannot catch an
// invariant that is wrong. "Every rung has even dimensions" is checkable in Go; "every rung
// actually encodes" is only checkable by encoding.

package playout

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func ffmpegBin(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("FFMPEG_PATH"); p != "" {
		return p
	}
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH")
	}
	return p
}

func ffprobeBin(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("no ffprobe on PATH")
	}
	return p
}

// Every rung on every ladder must actually encode. A rung can satisfy "even dimensions,
// descending bitrate" and still be rejected by an encoder — only ffmpeg knows.
func TestLive_EveryLadderRungEncodes(t *testing.T) {
	bin := ffmpegBin(t)
	enc := DetectObserved(context.Background(), bin, DefaultProfile(), "", nil).Chosen
	t.Logf("verifying ladders against %s", enc)

	for _, tier := range []Tier{TierQuality, TierBalanced, TierEfficient} {
		for rung := range LadderHeights(tier) {
			p := Resolve(tier, enc, rung)
			// The live pipeline, as the capability trial builds it: testsrc's CPU frames take the
			// family's upload, GPU scale and encoder, so a rung the graph cannot carry fails here.
			pipe, err := Build(HostFor(enc, false, GPUFilters{}), MediaFormat{}, ChannelOutput(p))
			if err != nil {
				t.Errorf("%s rung=%d: build: %v", tier, rung, err)
				continue
			}
			args := []string{"-hide_banner", "-loglevel", "error"}
			args = append(args, pipe.PreInput...)
			args = append(args, "-f", "lavfi", "-i",
				"testsrc=duration=1:size="+strconv.Itoa(p.Width)+"x"+strconv.Itoa(p.Height)+":rate="+strconv.Itoa(p.Framerate))
			args = append(args, "-vf", pipe.VideoFilter)
			args = append(args, pipe.VideoEncode...)
			args = append(args, "-frames:v", "10", "-f", "null", "-")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			b, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
			cancel()
			if err != nil {
				t.Errorf("%s rung=%d (%dx%d @%dk) does not encode: %v\n%s",
					tier, rung, p.Width, p.Height, p.VideoBitrate, err, b)
			}
		}
	}
}

// Detect must always return something usable, and must never claim an encoder works without
// having encoded with it.
func TestLive_DetectChoosesSomethingThatActuallyWorks(t *testing.T) {
	bin := ffmpegBin(t)
	c := DetectObserved(context.Background(), bin, DefaultProfile(), "", nil)

	if c.Chosen == "" {
		t.Fatal("Detect returned no encoder — software is always a valid answer")
	}
	if c.MaxChannels < 1 {
		t.Errorf("MaxChannels = %d, want at least 1", c.MaxChannels)
	}
	t.Logf("chosen %s: max channels %d, measured host memory per encode %d MiB",
		c.Chosen, c.MaxChannels, c.EncodeHostBytes>>20)
	// ⚠ A WORKING ENCODER MUST HAVE A MEASURED SPEED. This is the assertion that catches a VACUOUS
	// probe, and it is here because this test was green for months while trialEncode encoded
	// nothing at all.
	//
	// The failure composed from three individually reasonable decisions: os.CreateTemp creates the
	// output file, ffmpeg without `-y` refuses to overwrite it and EXITS ZERO, and hasKeyframe is
	// deliberately best-effort about an unreadable file. Result: `Works: true` for every encoder
	// the build lists — including h264_amf on a machine with no AMD hardware — and Speed 0 for all
	// of them, so MaxChannels sat at capacityFloor (measured: 2 instead of 11 on an RTX 3080 Ti).
	//
	// "Works" alone cannot detect that, because a vacuous probe reports exactly what a real one
	// does. A measured throughput cannot be faked by an encode that never ran.
	for _, x := range c.All {
		// The trial child's peak RSS sizes the ResourceBudget's host-memory gate; on platforms that
		// report it, a working trial that measured nothing would silently fall back to a default.
		if x.Works && x.PeakRSSBytes <= 0 && (runtime.GOOS == "linux" || runtime.GOOS == "darwin") {
			t.Errorf("%s reports Works with no measured peak RSS", x.Encoder)
		}
		if x.Works && x.Speed <= 0 {
			t.Errorf("%s reports Works with no measured speed — the trial exited cleanly without "+
				"encoding anything, so this probe proves nothing", x.Encoder)
		}
		if !x.Works && x.Err == "" {
			t.Errorf("%s failed with no reason recorded; the wizard's transcode check shows that "+
				"text to an operator", x.Encoder)
		}
	}

	// Whatever it chose must appear in All as Works.
	for _, x := range c.All {
		if x.Encoder == c.Chosen {
			if !x.Works {
				t.Errorf("Detect chose %s but its probe did not pass: %q", c.Chosen, x.Err)
			}
			return
		}
	}
	t.Errorf("Detect chose %s but it is not in the probe results", c.Chosen)
}

// A failed probe must carry ffmpeg's own message, not a category we invented — that text is
// what the wizard's transcode check shows an operator.
func TestLive_FailedProbesCarryFfmpegsOwnMessage(t *testing.T) {
	c := DetectObserved(context.Background(), ffmpegBin(t), DefaultProfile(), "", nil)
	for _, x := range c.All {
		if x.Works || x.Err == "" {
			continue
		}
		if x.Err == "failed" || x.Err == "error" {
			t.Errorf("%s: error is a useless category, want ffmpeg's text: %q", x.Encoder, x.Err)
		}
		t.Logf("%s: %s", x.Encoder, x.Err)
	}
}

// probeColor returns pix_fmt, transfer, primaries, matrix and range for a file's video stream.
func probeColor(t *testing.T, probe, path string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	got, err := exec.CommandContext(ctx, probe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=pix_fmt,color_transfer,color_primaries,color_space,color_range",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	// An MPEG-TS repeats its program map, so ffprobe can print the same stream more than once.
	return strings.TrimSpace(strings.Split(strings.TrimSpace(string(got)), "\n")[0])
}

// replaceOutput drops an item command's stdout target and appends extra (an output file).
func replaceOutput(args []string, extra ...string) []string {
	out := make([]string, 0, len(args)+len(extra))
	for _, a := range args {
		if a == "pipe:1" {
			continue
		}
		out = append(out, a)
	}
	return append(out, extra...)
}
