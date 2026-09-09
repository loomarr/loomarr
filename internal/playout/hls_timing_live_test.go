//go:build ffmpeg

package playout

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestLive_HLSPreservesPacketTimingAcrossSourceGaps(t *testing.T) {
	for _, bframes := range []string{"0", "3"} {
		t.Run("bframes="+bframes, func(t *testing.T) {
			bin, probe := ffmpegBin(t), ffprobeBin(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			dir := t.TempDir()
			source := filepath.Join(dir, "source.ts")
			// Several short gaps ensure at least one boundary falls inside an AAC PES
			// payload group. Both source streams retain those gaps instead of filling them.
			shift := `PTS+(gte(T\,1.3)+gte(T\,2.7)+gte(T\,4.1))*0.24/TB`
			args := []string{
				"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=duration=6:size=320x180:rate=25",
				"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=6",
				"-vf", "setpts=" + shift, "-af", "asetpts=" + shift,
				"-fps_mode", "passthrough", "-c:v", "libx264", "-preset", "ultrafast", "-g", "25", "-bf", bframes,
				"-c:a", "aac", "-ac", "2", "-ar", "48000", "-muxdelay", "0", "-f", "mpegts", source,
			}
			if out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
				t.Fatalf("generate gap source: %v: %s", err, out)
			}
			want := packetsByCodec(t, probeMuxPackets(t, ctx, probe, source), "gap source")
			for _, codec := range []string{"video", "audio"} {
				gaps := 0
				// B-frames are stored in decode order. Count source gaps in
				// presentation order, then compare original packet order below.
				ordered := append([]muxPacket(nil), want[codec]...)
				sort.Slice(ordered, func(i, j int) bool {
					left, _, _ := parseMuxPacket(t, ordered[i], "source PTS")
					right, _, _ := parseMuxPacket(t, ordered[j], "source PTS")
					return left < right
				})
				for i := 1; i < len(ordered); i++ {
					previous, _, duration := parseMuxPacket(t, ordered[i-1], "preceding packet")
					current, _, _ := parseMuxPacket(t, ordered[i], "following packet")
					if current-previous-duration > 9000 {
						gaps++
					}
				}
				if gaps != 3 {
					t.Fatalf("source %s has %d gaps, want three", codec, gaps)
				}
			}
			input, err := os.Open(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = input.Close() }()
			cmd := exec.CommandContext(ctx, bin, hlsArgs(dir, PlanBaseline)...)
			cmd.Stdin = input
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("HLS remux: %v: %s", err, out)
			}
			manifest, err := os.ReadFile(filepath.Join(dir, hlsPlaylistName))
			if err != nil {
				t.Fatal(err)
			}
			joinedPath := filepath.Join(dir, "hls.ts")
			joined, err := os.Create(joinedPath)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = joined.Close() }()
			segments := 0
			for _, line := range strings.Split(string(manifest), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if filepath.Base(line) != line || !strings.HasSuffix(line, ".ts") {
					t.Fatal("HLS fixture emitted a non-local segment")
				}
				segment, err := os.Open(filepath.Join(dir, line))
				if err != nil {
					t.Fatal(err)
				}
				_, copyErr := io.Copy(joined, segment)
				closeErr := segment.Close()
				if copyErr != nil || closeErr != nil {
					t.Fatalf("collect segment: copy=%v close=%v", copyErr, closeErr)
				}
				segments++
			}
			if segments < 2 {
				t.Fatalf("HLS segments=%d, want at least two", segments)
			}
			if err := joined.Close(); err != nil {
				t.Fatal(err)
			}
			got := packetsByCodec(t, probeMuxPackets(t, ctx, probe, joinedPath), "HLS output")
			assertProgrammesPreserved(t, []map[string][]muxPacket{want}, got)
		})
	}
}
