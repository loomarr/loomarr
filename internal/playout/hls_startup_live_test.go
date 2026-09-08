//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLive_HLSStartsBeforeFiniteInputCloses(t *testing.T) {
	for _, test := range []struct {
		name, encoder, size, audioOffset string
		plan                             EncodePlan
		largeKeyframe                    bool
	}{
		{"h264", "libx264", "320x180", "0", PlanBaseline, false},
		{"hevc", "libx265", "320x180", "0", PlanHEVC8, false},
		{"h264_delayed_audio", "libx264", "320x180", "3", PlanBaseline, false},
		{"hevc_delayed_audio", "libx265", "320x180", "3", PlanHEVC8, false},
		{"h264_large_keyframe", "libx264", "3840x2160", "0.2", PlanBaseline, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			bin, probe := ffmpegBin(t), ffprobeBin(t)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			dir := t.TempDir()
			source := filepath.Join(dir, "source.ts")
			args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=duration=4.6:size=" + test.size + ":rate=25", "-itsoffset", test.audioOffset, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=4.6", "-c:v", test.encoder, "-preset", "ultrafast", "-g", "25", "-bf", "0", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", "2", "-ar", "48000", "-muxdelay", "0"}
			if test.largeKeyframe {
				args = append(args, "-crf", "0", "-threads", "2")
			}
			if test.encoder == "libx265" {
				args = append(args, "-x265-params", "pools=1:frame-threads=1:log-level=error")
			}
			args = append(args, "-f", "mpegts", source)
			if output, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
				t.Fatalf("source encode: %v: %s", err, output)
			}
			if test.largeKeyframe {
				packets := probeMuxPackets(t, ctx, probe, source)
				if len(packets.Packets) == 0 {
					t.Fatal("large-keyframe fixture has no packets")
				}
				first := packets.Packets[0]
				size, err := first.Size.Int64()
				if err != nil || first.Stream != 0 || size <= 256_000 {
					t.Fatalf("first video packet must exceed rejected 256k probe: stream=%d size=%s err=%v", first.Stream, first.Size, err)
				}
				t.Logf("initial video packet is %d bytes", size)
			}
			media, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, bin, hlsArgs(dir, test.plan)...)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			if err := cmd.Start(); err != nil {
				_ = stdin.Close()
				t.Fatal(err)
			}
			done := make(chan struct{})
			var processErr error
			go func() { processErr = cmd.Wait(); close(done) }()
			written := make(chan struct{})
			var writeErr error
			go func() { _, writeErr = stdin.Write(media); close(written) }()
			defer func() { cancel(); _ = stdin.Close(); <-written; <-done }()
			select {
			case <-written:
				if writeErr != nil {
					<-done
					t.Fatalf("feed fixture: %v: %s", writeErr, stderr.String())
				}
			case <-ctx.Done():
				t.Fatal("HLS did not consume the fixture")
			}
			// Four seconds suffice to publish a complete production HLS segment.
			// Keep the remaining pipe open: source EOF cannot discharge analysis.
			deadline := time.NewTimer(2 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
			ready := false
			for !ready {
				select {
				case <-done:
					t.Fatalf("HLS exited before readiness: %v: %s", processErr, stderr.String())
				case <-deadline.C:
					t.Fatal("HLS analysis withheld a complete segment until source EOF")
				case <-tick.C:
					body, err := os.ReadFile(filepath.Join(dir, hlsPlaylistName))
					if err != nil {
						continue
					}
					for _, line := range strings.Split(string(body), "\n") {
						if line == "" || strings.HasPrefix(line, "#") {
							continue
						}
						if info, err := os.Stat(filepath.Join(dir, line)); err == nil && info.Size() > 0 {
							ready = true
						}
					}
				}
			}
			t.Logf("complete segment ready after %s with source pipe held open", time.Since(started))
			if err := stdin.Close(); err != nil {
				t.Fatal(err)
			}
			<-done
			if processErr != nil {
				t.Fatalf("HLS remux: %v: %s", processErr, stderr.String())
			}
			// Probe the complete output after shutdown too: early readiness cannot
			// win by losing the audio stream or discarding packets.
			body, err := os.ReadFile(filepath.Join(dir, hlsPlaylistName))
			if err != nil {
				t.Fatal(err)
			}
			joined, err := os.Create(filepath.Join(dir, "joined.media"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = joined.Close() }()
			if test.plan == PlanHEVC8 {
				init, err := os.ReadFile(filepath.Join(dir, "init.mp4"))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := joined.Write(init); err != nil {
					t.Fatal(err)
				}
			}
			for _, line := range strings.Split(string(body), "\n") {
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				file, err := os.Open(filepath.Join(dir, line))
				if err != nil {
					t.Fatal(err)
				}
				_, copyErr := io.Copy(joined, file)
				closeErr := file.Close()
				if copyErr != nil || closeErr != nil {
					t.Fatalf("collect segment: %v / %v", copyErr, closeErr)
				}
			}
			if err := joined.Close(); err != nil {
				t.Fatal(err)
			}
			counts := func(path string) map[string]int {
				p := probeMuxPackets(t, ctx, probe, path)
				if len(p.Streams) != 2 {
					t.Fatalf("stream count=%d, want two", len(p.Streams))
				}
				streams := map[int]string{}
				result := map[string]int{}
				for _, stream := range p.Streams {
					if stream.CodecType != "video" && stream.CodecType != "audio" {
						t.Fatalf("unexpected stream type=%s", stream.CodecType)
					}
					if _, exists := result[stream.CodecType]; exists {
						t.Fatal("duplicate stream type")
					}
					result[stream.CodecType] = 0
					streams[stream.Index] = stream.CodecType
				}
				for _, packet := range p.Packets {
					kind, ok := streams[packet.Stream]
					if !ok {
						t.Fatal("packet has unknown stream")
					}
					result[kind]++
				}
				return result
			}
			got, want := counts(joined.Name()), counts(source)
			for _, kind := range []string{"video", "audio"} {
				if want[kind] == 0 || got[kind] != want[kind] {
					t.Fatalf("%s packet counts changed: %d/%d", kind, got[kind], want[kind])
				}
			}
		})
	}
}
