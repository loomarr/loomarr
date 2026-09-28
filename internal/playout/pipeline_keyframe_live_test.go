//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// THE FIRST FRAME OF EVERY SLOT IS AN IDR, AND THE CADENCE HOLDS (#1561). The retired chain forced
// a keyframe on frame 0 with -force_key_frames: its cold transcode child fed an HLS remux that could
// only cut on a keyframe, and a first keyframe a GOP late left the player black. The channel packager
// starts a fresh encoder per slot and cuts one fragment per closed GOP, so the same rule must hold
// by construction: frame 0 of every slot is an IDR (a seek lands mid-GOP in the source, so it is not
// a copied keyframe), and every later keyframe sits on an exact GOP boundary. Each family that
// encodes on this host is checked through the production Build and FragmentArgs.
func TestLive_FragmentStartsOnAnIDRAndHoldsTheGOP(t *testing.T) {
	bin, probe := ffmpegBin(t), ffprobeBin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	src := filepath.Join(t.TempDir(), "long-gop.mp4")
	// A 10 s source GOP, so the 3.3 s seek below is far from any source keyframe.
	if b, err := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=duration=12:size=640x360:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=12",
		"-c:v", "libx264", "-g", "300", "-keyint_min", "300", "-sc_threshold", "0", "-c:a", "aac", src).CombinedOutput(); err != nil {
		t.Fatalf("synthesize the source: %v\n%s", err, b)
	}
	facts, err := FFprobeFormatNextTo(bin)(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	out := OutputProfile{Width: 640, Height: 360, FPS: 30, Quality: 23, TargetKbps: 1500, MaxKbps: 2000, GOPSeconds: 1, AudioKbps: 128}
	const frames = 120
	for _, enc := range []Encoder{EncoderSoftware, EncoderNVENC, EncoderVAAPI} {
		t.Run(string(enc), func(t *testing.T) {
			pipe, err := Build(HostFor(enc, false, GPUFilters{}), facts, out)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			fragment := filepath.Join(t.TempDir(), "slot.mp4")
			args := pipe.FragmentArgs(src, 3300*time.Millisecond, 0, frames, 4*48000/1024, out.FPS, 0)
			cmd := exec.CommandContext(ctx, bin, args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				if enc == EncoderSoftware {
					t.Fatalf("software fragment: %v\n%s", err, stderr.String())
				}
				t.Skipf("%s does not encode on this host: %v\n%s", enc, err, stderr.String())
			}
			if err := os.WriteFile(fragment, stdout.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			raw, err := exec.CommandContext(ctx, probe, "-v", "error", "-select_streams", "v:0", "-show_frames",
				"-show_entries", "frame=key_frame,pict_type", "-of", "json", fragment).Output()
			if err != nil {
				t.Fatal(err)
			}
			var probed struct {
				Frames []struct {
					KeyFrame int    `json:"key_frame"`
					PictType string `json:"pict_type"`
				} `json:"frames"`
			}
			if err := json.Unmarshal(raw, &probed); err != nil {
				t.Fatal(err)
			}
			if len(probed.Frames) < frames {
				t.Fatalf("%d frames decoded, want at least %d", len(probed.Frames), frames)
			}
			if first := probed.Frames[0]; first.KeyFrame != 1 || first.PictType != "I" {
				t.Fatalf("slot frame 0 = %+v, want an IDR", first)
			}
			gop := out.gop()
			for n, frame := range probed.Frames {
				if key := frame.KeyFrame == 1; key != (n%gop == 0) {
					t.Errorf("frame %d key=%v (%s), want keyframes exactly every %d frames", n, key, frame.PictType, gop)
				}
			}
			t.Logf("%s: %d frames, frame 0 an IDR, keyframes every %d", enc, len(probed.Frames), gop)
		})
	}
}
