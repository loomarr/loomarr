//go:build ffmpeg

package playout

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLive_CopyStartKeepsOpeningFrameAndTranscodesUnsafeLateSeek(t *testing.T) {
	bin, probe := ffmpegBin(t), ffprobeBin(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	source := filepath.Join(t.TempDir(), "long-gop.mkv")
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc2=duration=12:size=320x180:rate=25", "-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000:duration=12", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "250", "-bf", "0", "-c:a", "eac3", "-ac", "2", "-ar", "48000", "-shortest", source}
	if out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
		t.Fatalf("source: %v %s", err, out)
	}
	for _, tc := range []struct {
		name    string
		offset  time.Duration
		copy    bool
		clocked bool
	}{{"opening", time.Millisecond, true, true}, {"late", 11 * time.Second, false, true}, {"standalone opening", time.Millisecond, true, false}, {"standalone late", 11 * time.Second, false, false}} {
		t.Run(tc.name, func(t *testing.T) {
			seek, ok := FFprobeCopyStartNextTo(bin)(ctx, source, tc.offset, time.Second, 25)
			if ok != tc.copy {
				t.Fatalf("copy eligible=%t want=%t seek=%s", ok, tc.copy, seek)
			}
			profile := DefaultProfile()
			profile.Encoder, profile.Width, profile.Height = EncoderSoftware, 320, 180
			spec := ProgramSpec{Profile: profile, Input: source, Offset: tc.offset, Limit: time.Second, Plan: CopyPlan{CopyVideo: ok}, UnpacedInput: true, Clock: ProgramClock{Origin: time.Unix(1000, 0), StartedAt: time.Unix(1010, 0)}}
			if !tc.clocked {
				spec.Clock = ProgramClock{}
			}
			if ok {
				spec.VideoCopySeek = &seek
			}
			output := filepath.Join(t.TempDir(), "child.ts")
			process, err := Start(ctx, bin, replaceOutput(ProgramArgs(spec), output), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(io.Discard, process.Stdout); err != nil {
				process.Stop()
				t.Fatal(err)
			}
			if err := process.Wait(); err != nil {
				t.Fatalf("child: %v %s", err, process.LastError())
			}
			packets := packetsByCodec(t, probeMuxPackets(t, ctx, probe, output), "copy start child")
			if len(packets["video"]) < 24 || len(packets["audio"]) < 40 {
				t.Fatalf("incomplete finite A/V: video=%d audio=%d", len(packets["video"]), len(packets["audio"]))
			}
			first, _, _ := parseMuxPacket(t, packets["video"][0], "first video")
			want := int64((10*time.Second + tc.offset).Seconds() * 90000)
			if !tc.clocked {
				// Standalone MPEG-TS chooses its own mux origin. Video must
				// still begin with the initial audio, not at the next GOP.
				want, _, _ = parseMuxPacket(t, packets["audio"][0], "initial standalone audio")
			}
			if abs64(first-want) > 3600 {
				t.Fatalf("first video=%d expected within one frame of %d", first, want)
			}
		})
	}
}

func TestLive_CopyStartRejectsDecoderReordering(t *testing.T) {
	bin := ffmpegBin(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	source := filepath.Join(t.TempDir(), "reordered.mp4")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=duration=3:size=320x180:rate=25",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-g", "25", "-bf", "3", "-b_strategy", "0", source}
	if out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
		t.Fatalf("generate reordered source: %v: %s", err, out)
	}
	if _, ok := FFprobeCopyStartNextTo(bin)(ctx, source, 0, time.Second, 25); ok {
		t.Fatal("reordered source was admitted for a mixed session handoff")
	}
}
