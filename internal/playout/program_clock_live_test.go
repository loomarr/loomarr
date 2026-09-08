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

// Compare real transport packets, not just arguments: a different session origin must move
// every audio/video timestamp equally without changing payloads, source seek or finite end.
func TestLive_ProgramClockPreservesSeekEndAndCommonAVShift(t *testing.T) {
	bin, probe := ffmpegBin(t), ffprobeBin(t)
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	for _, encoder := range []Encoder{EncoderSoftware, EncoderSoftwareHEVC} {
		t.Run(string(encoder), func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source.mp4")
			args := []string{"-hide_banner", "-loglevel", "error", "-nostdin",
				"-f", "lavfi", "-i", "testsrc2=duration=6:size=320x180:rate=25",
				"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000:duration=6",
				"-c:v", string(encoder), "-pix_fmt", "yuv420p", "-g", "25", "-bf", "0",
				"-c:a", "aac", "-ac", "2", "-ar", "48000", "-shortest", source}
			if output, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
				t.Fatalf("source: %v: %s", err, output)
			}
			for _, tc := range []struct {
				name string
				plan CopyPlan
			}{
				{"copy=true", CopyPlan{CopyVideo: true, CopyAudio: true}},
				{"copy=false", CopyPlan{}},
				{"copy video only", CopyPlan{CopyVideo: true}},
				{"copy audio only", CopyPlan{CopyAudio: true}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					profile := DefaultProfile()
					profile.Encoder, profile.Width, profile.Height = encoder, 320, 180
					spec := ProgramSpec{
						Profile: profile, Input: source, Offset: 3237 * time.Millisecond, Limit: time.Second,
						Plan: tc.plan,
						// This test isolates timestamp coordinates from realtime pacing.
						UnpacedInput: true,
						Clock:        ProgramClock{Origin: time.Unix(1000, 0), StartedAt: time.Unix(1010, 0)},
					}
					var reference map[string][]muxPacket
					for index := range 2 {
						out := filepath.Join(t.TempDir(), "child.ts")
						proc, err := Start(ctx, bin, replaceOutput(ProgramArgs(spec), out), nil, nil)
						if err != nil {
							t.Fatal(err)
						}
						if _, err := io.Copy(io.Discard, proc.Stdout); err != nil {
							proc.Stop()
							t.Fatal(err)
						}
						if err := proc.Wait(); err != nil {
							t.Fatalf("child: %v: %s", err, proc.LastError())
						}
						packets := packetsByCodec(t, probeMuxPackets(t, ctx, probe, out), "clock child")
						if index == 0 {
							reference = packets
							first, _, _ := parseMuxPacket(t, packets["video"][0], "first video")
							last, _, duration := parseMuxPacket(t, packets["video"][len(packets["video"])-1], "last video")
							start := int64((10*time.Second + spec.Offset).Seconds() * 90000)
							end := int64((10*time.Second + spec.Offset + spec.Limit).Seconds() * 90000)
							if first < start-3600 || first >= end || abs64(last+duration-end) > 7200 {
								t.Fatalf("video interval %d..%d, want seek >= %d and end near %d", first, last+duration, start, end)
							}
						} else {
							assertProgrammesPreserved(t, []map[string][]muxPacket{reference}, packets)
							before, _, _ := parseMuxPacket(t, reference["video"][0], "reference")
							after, _, _ := parseMuxPacket(t, packets["video"][0], "shifted")
							if after-before != 15*90000 {
								t.Fatalf("origin shift = %d ticks, want %d", after-before, 15*90000)
							}
						}
						spec.Clock.Origin = spec.Clock.Origin.Add(-15 * time.Second)
					}
				})
			}
		})
	}
}
