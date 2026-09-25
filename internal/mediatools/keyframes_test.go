package mediatools

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This is the semantic-evidence regression loop, not an image-quality proxy. The fake executable
// records the exact windows and filter handed to ffmpeg, so the test catches the two production
// defects without needing a host codec: the old path selected the first few frames with
// thumbnail=n and shrank OCR evidence to the 320 px UI-preview width.
func TestKeyframesIn_UsesBoundedSemanticWindowsIncludingTheEndCard(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$(dirname \"$0\")/calls\"\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	tools := NewFFmpegTools(ffmpeg, "", "", "", "")
	if _, err := tools.KeyframesIn(context.Background(), "clip.mp4", 0, 30_000, 4); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(calls) != 4 {
		t.Fatalf("ffmpeg calls = %d, want 4 semantic windows; calls:\n%s", len(calls), raw)
	}
	for _, call := range calls {
		if strings.Contains(call, "scale=320") {
			t.Fatalf("semantic evidence was reduced to the UI preview width: %s", call)
		}
		if !strings.Contains(call, "min(iw,1920)") {
			t.Fatalf("semantic evidence has no bounded near-full-resolution scale: %s", call)
		}
	}
	if !strings.Contains(calls[len(calls)-1], "-ss 27.000") {
		t.Fatalf("last semantic window is not end-card biased: %s", calls[len(calls)-1])
	}
}

func TestKeyframesIn_ProducesDecodableNativeSizeFrames(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	cmd := exec.Command(ffmpeg, "-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=10:duration=6",
		"-c:v", "mpeg4", "-y", clip)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v: %s", err, out)
	}

	frames, err := NewFFmpegTools(ffmpeg, "", "", "", "").KeyframesIn(
		context.Background(), clip, 0, 6000, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 4 {
		t.Fatalf("frames = %d, want 4", len(frames))
	}
	for i, frame := range frames {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(frame))
		if err != nil {
			t.Fatalf("frame %d is not a JPEG: %v", i, err)
		}
		if cfg.Width != 1280 || cfg.Height != 720 {
			t.Fatalf("frame %d dimensions = %dx%d, want native 1280x720", i, cfg.Width, cfg.Height)
		}
	}
}

// Image tokens scale with pixel area, so SD vision frames (height <= VisionScaleMaxHeight) are
// scaled down 0.8x linear; anything taller keeps today's size because HD fine print did not survive.
// 0.64x the pixels) that keeps the same legibility ratio for SD and HD sources alike.
func TestVisionKeyframesIn_ScalesOnlySDSources(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$(dirname \"$0\")/calls\"\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFFmpegTools(ffmpeg, "", "", "", "").VisionKeyframesIn(context.Background(), "clip.mp4", 0, 30_000, 4); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(calls) != 4 {
		t.Fatalf("ffmpeg calls = %d, want 4 windows", len(calls))
	}
	for _, call := range calls {
		if !strings.Contains(call, "iw*"+VisionFrameScale) || !strings.Contains(call, fmt.Sprintf("lte(ih,%d)", VisionScaleMaxHeight)) {
			t.Fatalf("vision frames are not scaled by VisionFrameScale: %s", call)
		}
	}
	if !strings.Contains(calls[len(calls)-1], "-ss 27.000") {
		t.Fatalf("vision frames lost the end-card window: %s", calls[len(calls)-1])
	}
}

func TestVisionKeyframesIn_SDDownscalesAndLargerStaysNative(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	dir := t.TempDir()
	tools := NewFFmpegTools(ffmpeg, "", "", "", "")
	for _, tc := range []struct {
		name          string
		size          string
		wantW, wantH  int
		wantUnchanged bool
	}{
		{name: "HD 16:9 stays at today's size", size: "1280x720", wantW: 1280, wantH: 720},
		{name: "boundary: 576 high is SD", size: "768x576", wantW: 614, wantH: 460},
		{name: "boundary: 578 high is not SD", size: "768x578", wantW: 768, wantH: 578},
		{name: "SD archive 4:3", size: "640x480", wantW: 512, wantH: 384},
		{name: "tiny source", size: "320x240", wantW: 256, wantH: 192},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clip := filepath.Join(dir, strings.ReplaceAll(tc.size, "x", "_")+".mp4")
			cmd := exec.Command(ffmpeg, "-nostdin", "-v", "error",
				"-f", "lavfi", "-i", "testsrc2=size="+tc.size+":rate=30:duration=6",
				"-c:v", "mpeg4", "-y", clip)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build fixture: %v: %s", err, out)
			}
			frames, err := tools.VisionKeyframesIn(context.Background(), clip, 0, 6000, 4)
			if err != nil {
				t.Fatal(err)
			}
			if len(frames) != 4 {
				t.Fatalf("frames = %d, want 4", len(frames))
			}
			for i, frame := range frames {
				cfg, err := jpeg.DecodeConfig(bytes.NewReader(frame))
				if err != nil {
					t.Fatalf("frame %d is not a JPEG: %v", i, err)
				}
				if cfg.Width != tc.wantW || cfg.Height != tc.wantH {
					t.Fatalf("frame %d = %dx%d, want %dx%d", i, cfg.Width, cfg.Height, tc.wantW, tc.wantH)
				}
			}
		})
	}
}
