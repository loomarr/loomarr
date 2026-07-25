//go:build ffmpeg

// Tests that EXECUTE ffmpeg. Behind a build tag because unit tests must not depend on an
// external binary being present — the same reasoning CLAUDE.md applies to the network. Run
// with `make test-ffmpeg`.
//
// These exist because arg-shape tests assert my own stated invariants and cannot catch an
// invariant that is wrong. "Every rung has even dimensions" is checkable in Go; "every rung
// actually encodes" is only checkable by encoding.

package playout

import (
	"context"
	"io"
	"os"
	"os/exec"
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

// The test card must produce a VALID MPEG-TS with BOTH streams. The audio half is the point:
// a video-only MPEG-TS is a classic cause of a player refusing to play or showing no
// timeline, and `anullsrc` is what puts it there.
func TestLive_TestCardProducesValidMpegTsWithAudio(t *testing.T) {
	bin := ffmpegBin(t)
	probe := ffprobeBin(t)
	out := t.TempDir() + "/card.ts"

	p := DefaultProfile()
	p.Width, p.Height = 640, 360 // small: this is about validity, not throughput
	args := TestCardArgs(p, FindFont(), "Loomarr", "channel 1")
	// Bound it and write to a file rather than the pipe the real thing uses.
	args = replaceOutput(args, "-t", "2", out)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Through Start, NOT exec directly: TestCardArgs asks for `-progress pipe:3`, and fd 3
	// exists only because Start wires it via ExtraFiles. Running these args with plain exec
	// fails at startup with "Error parsing global options: Bad file descriptor" — which is
	// exactly how this bug was found, and why this test goes through the real supervisor.
	var samples int
	proc, err := Start(ctx, bin, args, nil, func(Progress) { samples++ })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	// Nothing consumes stdout here (output goes to the file), but the pipe still must be
	// drained or ffmpeg blocks writing to it.
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
	if err := proc.Wait(); err != nil {
		t.Fatalf("test card failed to encode: %v\nlast stderr: %s", err, proc.LastError())
	}
	if samples == 0 {
		t.Error("no progress samples — `-progress pipe:3` is not being read")
	}

	got, err := exec.CommandContext(ctx, probe, "-v", "error",
		"-show_entries", "format=format_name", "-show_entries", "stream=codec_type,codec_name",
		"-of", "default=noprint_wrappers=1", out).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	s := string(got)
	for _, want := range []string{"format_name=mpegts", "codec_type=video", "codec_name=h264", "codec_type=audio"} {
		if !strings.Contains(s, want) {
			t.Errorf("probe missing %q — the card is not a playable MPEG-TS:\n%s", want, s)
		}
	}
}

// Every rung on every ladder must actually encode. A rung can satisfy "even dimensions,
// descending bitrate" and still be rejected by an encoder — only ffmpeg knows.
func TestLive_EveryLadderRungEncodes(t *testing.T) {
	bin := ffmpegBin(t)
	enc := Detect(context.Background(), bin, DefaultProfile()).Chosen
	t.Logf("verifying ladders against %s", enc)

	for _, tier := range []Tier{TierQuality, TierBalanced, TierEfficient} {
		for active := 0; active <= 8; active++ {
			p := Resolve(tier, enc, 8, active)
			args := []string{"-hide_banner", "-loglevel", "error"}
			args = append(args, deviceInitArgs(enc)...)
			args = append(args, "-f", "lavfi", "-i",
				"testsrc=duration=1:size="+itoa(p.Width)+"x"+itoa(p.Height)+":rate="+itoa(p.Framerate))
			if vf := hardwareUploadFilter(enc); vf != "" {
				args = append(args, "-vf", vf)
			}
			args = append(args, p.videoEncodeArgs()...)
			args = append(args, "-frames:v", "10", "-f", "null", "-")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			b, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
			cancel()
			if err != nil {
				t.Errorf("%s active=%d (%dx%d @%dk) does not encode: %v\n%s",
					tier, active, p.Width, p.Height, p.VideoBitrate, err, b)
			}
		}
	}
}

// Detect must always return something usable, and must never claim an encoder works without
// having encoded with it.
func TestLive_DetectChoosesSomethingThatActuallyWorks(t *testing.T) {
	bin := ffmpegBin(t)
	c := Detect(context.Background(), bin, DefaultProfile())

	if c.Chosen == "" {
		t.Fatal("Detect returned no encoder — software is always a valid answer")
	}
	if c.MaxChannels < 1 {
		t.Errorf("MaxChannels = %d, want at least 1", c.MaxChannels)
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
	c := Detect(context.Background(), ffmpegBin(t), DefaultProfile())
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

// replaceOutput swaps the trailing "pipe:1" for a bounded file output.
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

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
