package mediatools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostedVideoInRefusesUnboundedSpanBeforeFFmpeg(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "called")
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ntouch " + marker + "\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	tools := NewFFmpegTools(ffmpeg, "", "", "", "")

	for name, span := range map[string][2]int64{
		"missing":  {0, 0},
		"inverted": {2_000, 1_000},
		"negative": {-1, 1_000},
		"too long": {0, HostedVideoMaxDurationMS + 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tools.HostedVideoIn(context.Background(), "clip.mp4", span[0], span[1]); err == nil {
				t.Fatal("unsafe span must fail closed")
			}
		})
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("invalid span reached ffmpeg: %v", err)
	}
}

func TestHostedVideoInProducesInspectableMetadataFree720pMP4(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe unavailable")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mp4")
	generate := exec.Command(ffmpeg, "-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=10:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=1",
		"-shortest", "-metadata", "title=private-source-title",
		"-c:v", "libx264", "-preset", "ultrafast", "-threads", "1", "-c:a", "aac", source)
	if out, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate source: %v: %s", err, out)
	}

	got, err := NewFFmpegTools(ffmpeg, ffprobe, "", "", "").HostedVideoIn(
		context.Background(), source, 0, 1_000,
	)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(got.MP4)
	if got.SHA256 != fmt.Sprintf("%x", digest) {
		t.Fatalf("hash = %q, want derivative SHA-256", got.SHA256)
	}
	output := filepath.Join(dir, "derivative.mp4")
	if err := os.WriteFile(output, got.MP4, 0o600); err != nil {
		t.Fatal(err)
	}

	probe, err := exec.Command(ffprobe, "-v", "error", "-show_entries",
		"stream=codec_name,codec_type,width,height:format=duration:format_tags=title",
		"-of", "json", output).Output()
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
			CodecType string `json:"codec_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(probe, &info); err != nil {
		t.Fatal(err)
	}
	if len(info.Streams) != 2 {
		t.Fatalf("streams = %+v, want video and optional AAC audio preserved", info.Streams)
	}
	for _, stream := range info.Streams {
		switch stream.CodecType {
		case "video":
			if stream.CodecName != "h264" || stream.Width > 1280 || stream.Height > 720 {
				t.Errorf("video stream = %+v, want bounded H.264", stream)
			}
		case "audio":
			if stream.CodecName != "aac" {
				t.Errorf("audio stream = %+v, want AAC", stream)
			}
		default:
			t.Errorf("unexpected stream = %+v", stream)
		}
	}
	if title := info.Format.Tags["title"]; title != "" {
		t.Errorf("source metadata leaked into derivative: title=%q", title)
	}
}

func TestHostedVideoInUsesMetadataFreeBoundedMP4Contract(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\nprintf 'mp4'\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := NewFFmpegTools(ffmpeg, "", "", "", "").HostedVideoIn(
		context.Background(), "clip source.mov", 1_250, 31_250,
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.MP4) != "mp4" || got.MIMEType != "video/mp4" || got.StartMS != 1_250 || got.EndMS != 31_250 {
		t.Fatalf("derivative = %+v", got)
	}
	if got.SHA256 == "" {
		t.Fatal("derivative must carry its content hash")
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	for _, exact := range []string{
		"-nostdin\n", "-ss\n1.250\n", "-t\n30.000\n", "-i\nclip source.mov\n",
		"-map_metadata\n-1\n", "-map_chapters\n-1\n", "-c:v\nlibx264\n", "-c:a\naac\n",
		"-threads\n1\n",
		"-f\nmp4\n", "-movflags\nfrag_keyframe+empty_moov+default_base_moof\n",
	} {
		if !strings.Contains(args, exact) {
			t.Errorf("ffmpeg args missing %q:\n%s", exact, args)
		}
	}
	if !strings.Contains(args, "1280") || !strings.Contains(args, "720") {
		t.Errorf("ffmpeg scale does not enforce 1280x720: %s", args)
	}
}

func TestHostedVideoInHoldsMalformedInput(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\necho 'invalid media' >&2\nexit 1\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := NewFFmpegTools(ffmpeg, "", "", "", "").HostedVideoIn(
		context.Background(), "broken.mp4", 0, 1_000,
	)
	if err == nil || !strings.Contains(err.Error(), "invalid media") {
		t.Fatalf("derivative = %+v, error = %v; malformed input must stay held with diagnosis", got, err)
	}
	if len(got.MP4) != 0 {
		t.Fatal("malformed input must not return a partial derivative")
	}
}

func TestHostedVideoInBoundsMalformedEncoderDiagnostics(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ndd if=/dev/zero bs=1048576 count=1 >&2 2>/dev/null\nexit 1\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := NewFFmpegTools(ffmpeg, "", "", "", "").HostedVideoIn(
		context.Background(), "broken.mp4", 0, 1_000,
	)
	if err == nil {
		t.Fatal("malformed encoder failure must be returned")
	}
	if len(err.Error()) > HostedVideoMaxDiagnosticBytes+512 {
		t.Fatalf("diagnostic grew to %d bytes, want bounded failure", len(err.Error()))
	}
}

func TestHostedVideoInAbortsOversizedEncoderOutput(t *testing.T) {
	dir := t.TempDir()
	ffmpeg := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ndd if=/dev/zero bs=1048576 count=13 2>/dev/null\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := NewFFmpegTools(ffmpeg, "", "", "", "").HostedVideoIn(
		context.Background(), "clip.mp4", 0, 1_000,
	)
	if err == nil || !strings.Contains(err.Error(), "12") || !strings.Contains(err.Error(), "ceiling") {
		t.Fatalf("error = %v, want explicit 12 MiB ceiling failure", err)
	}
}
