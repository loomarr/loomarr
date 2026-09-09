//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"fmt"
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

// A source seek and finite end select the same samples for every video/copy plan. The reference
// is sliced from the original PCM source, independently of FFmpeg seeking and the child filter.
func TestLive_SessionPCMSelectsExactSourceSamples(t *testing.T) {
	bin := ffmpegBin(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	source := filepath.Join(t.TempDir(), "source.mov")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=duration=6:size=320x180:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=733:sample_rate=48000:duration=6", "-c:v", "libx264",
		"-pix_fmt", "yuv420p", "-g", "25", "-bf", "0", "-c:a", "pcm_s16le", "-ac", "2", "-shortest", source}
	if out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
		t.Fatalf("generate PCM source: %v: %s", err, out)
	}
	decode := func(path string) []byte {
		t.Helper()
		pcm, err := exec.CommandContext(ctx, bin, "-v", "error", "-i", path, "-map", "0:a:0", "-c:a", "pcm_s16le", "-f", "s16le", "pipe:1").Output()
		if err != nil {
			t.Fatal(err)
		}
		return pcm
	}
	all := decode(source)
	const offsetSamples, countSamples = 60000, 72000
	want := all[offsetSamples*4 : (offsetSamples+countSamples)*4]
	for _, plan := range []CopyPlan{{}, {CopyVideo: true}, {CopyAudio: true}, {CopyVideo: true, CopyAudio: true}} {
		t.Run(fmt.Sprint(plan), func(t *testing.T) {
			profile := DefaultProfile()
			profile.Encoder, profile.Width, profile.Height, profile.Framerate = EncoderSoftware, 320, 180, 25
			spec := ProgramSpec{SessionAudio: true, Profile: profile, Input: source, Offset: 1250 * time.Millisecond, Limit: 1500 * time.Millisecond, Plan: plan,
				Clock: ProgramClock{Origin: time.Unix(1000, 0), StartedAt: time.Unix(1010, 0)}}
			path := filepath.Join(t.TempDir(), "child.ts")
			proc, err := Start(ctx, bin, replaceOutput(ProgramArgs(spec), path), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(io.Discard, proc.Stdout); err != nil {
				proc.Stop()
				t.Fatal(err)
			}
			if err := proc.Wait(); err != nil {
				t.Fatalf("PCM child: %v: %s", err, proc.LastError())
			}
			got := decode(path)
			if !bytes.Equal(got, want) {
				t.Fatalf("selected PCM differs: got %d bytes, want %d", len(got), len(want))
			}
		})
	}
}
