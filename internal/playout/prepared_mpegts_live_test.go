//go:build ffmpeg

package playout

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/prepared"
	"github.com/loomarr/loomarr/internal/schedule"
)

func TestLive_PreparedMPEGTSBlockProducesTransportUnderOneSecond(t *testing.T) {
	bin := ffmpegBin(t)
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.mp4")
	generate := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=duration=24:size=320x180:rate=25",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		"-map", "0:v:0", "-map", "1:a:0", "-shortest", "-t", "24",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-g", "50", "-sc_threshold", "0", "-c:a", "aac", "-ar", "48000", "-ac", "2",
		sourcePath,
	}
	if output, err := exec.Command(bin, generate...).CombinedOutput(); err != nil {
		t.Fatalf("generate source: %v\n%s", err, output)
	}

	lib, err := prepared.NewLibrary(filepath.Join(dir, "prepared"))
	if err != nil {
		t.Fatal(err)
	}
	spec := prepared.Specification{
		SourceFingerprint: "live-prepared-source",
		Rendition: prepared.RenditionContract{
			VideoCodec: "h264", VideoProfile: "high", VideoLevel: "4.1", PixelFormat: "yuv420p",
			HDR: "sdr", AudioCodec: "aac", AudioLayout: "stereo", Width: 320, Height: 180,
			FrameRate: 25, VideoBitrateKbps: 800, AudioBitrateKbps: 96,
			SegmentDurationMS: 2000, PackagingVersion: prepared.CurrentPackagingVersion,
		},
	}
	packager := prepared.NewFFmpegPackager(bin)
	if _, err := lib.Publish(t.Context(), spec, func(ctx context.Context, workspace string) (prepared.Output, error) {
		return packager.Package(ctx, workspace, prepared.LocalInput(sourcePath), 0, spec.Rendition)
	}); err != nil {
		t.Fatal(err)
	}

	startedAt := time.Now().UTC().Add(-12 * time.Second)
	origin := newPreparedOrigin(lib, fixedPreparedResolver{ok: true, window: PreparedWindow{
		Current: PreparedAiring{
			Specification: spec, StartedAt: startedAt, Offset: 12 * time.Second,
			Identity: AiringIdentity{
				StartedAt: startedAt, EndsAt: startedAt.Add(24 * time.Second), Kind: schedule.SlotProgram,
				ContentID: "live-source", ScheduleBlockID: "block-live-source",
			},
		},
	}})

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	begin := time.Now()
	block, err := origin.MPEGTSBlockSource(bin, nil, nil)(ctx, "channel", PlanFull)
	if err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, 188)
	n, err := block.Content.Read(packet)
	firstByte := time.Since(begin)
	if err != nil || n == 0 {
		t.Fatalf("first transport read = %d bytes, err=%v", n, err)
	}
	if packet[0] != 0x47 {
		t.Fatalf("first byte = %#x, want MPEG-TS sync byte 0x47", packet[0])
	}
	if firstByte >= time.Second {
		t.Fatalf("prepared first transport byte = %s, want under 1s", firstByte)
	}
	_ = block.Content.Close()
	t.Logf("prepared first transport byte: %s", firstByte)

	decodeCtx, decodeCancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer decodeCancel()
	decodeBegin := time.Now()
	channel, err := BlockSpawner(bin, origin.MPEGTSBlockSource(bin, nil, nil), nil)(
		decodeCtx, "channel", PlanFull,
	)
	if err != nil {
		t.Fatal(err)
	}
	decoder := exec.CommandContext(decodeCtx, bin,
		"-hide_banner", "-loglevel", "error", "-probesize", "256k", "-analyzeduration", "500000",
		"-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-frames:v", "1", "-f", "null", "-",
	)
	decoder.Stdin = channel.Stdout
	if output, err := decoder.CombinedOutput(); err != nil {
		channel.Stop()
		t.Fatalf("decode first frame: %v\n%s", err, output)
	}
	firstFrame := time.Since(decodeBegin)
	channel.Stop()
	if firstFrame >= time.Second {
		t.Fatalf("prepared first decoded frame = %s, want under 1s", firstFrame)
	}
	t.Logf("prepared first decoded frame: %s", firstFrame)
}
