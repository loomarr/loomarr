//go:build ffmpeg

// Tests that execute ffmpeg. Unit tests stay external-binary-free; run with `make test-ffmpeg`.

package prepared_test

import (
	"bytes"
	"context"
	"crypto/subtle"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/prepared"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestLiveFFmpegPackagerPublishesPlayableHLS(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if configured := os.Getenv("FFMPEG_PATH"); configured != "" {
		bin, err = configured, nil
	}
	if err != nil {
		t.Skip("no ffmpeg on PATH")
	}
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("no ffprobe on PATH")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	source := filepath.Join(t.TempDir(), "source.mp4")
	generate := exec.CommandContext(ctx, bin,
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "lavfi", "-i", "testsrc2=duration=3:size=320x180:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=3",
		"-c:v", "libx264", "-c:a", "aac", "-shortest", source,
	)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate source: %v\n%s", err, output)
	}

	media, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	const token = "prepared-http-test-token"
	var authenticatedRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("api_key")), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		authenticatedRequests.Add(1)
		http.ServeContent(w, r, "source.mp4", time.Time{}, bytes.NewReader(media))
	}))
	defer server.Close()

	library, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := prepared.RenditionContract{
		VideoCodec: "h264", VideoProfile: "high", VideoLevel: "4.1", PixelFormat: "yuv420p", HDR: "sdr",
		AudioCodec: "aac", AudioLayout: "stereo", Width: 320, Height: 180, FrameRate: 25,
		VideoBitrateKbps: 500, AudioBitrateKbps: 96, SegmentDurationMS: 1000, PackagingVersion: prepared.CurrentPackagingVersion,
	}
	preparer := prepared.NewPreparer(prepared.PreparerDependencies{
		Library: library, Packager: prepared.NewFFmpegPackager(bin), Access: &testkit.PreparedSourceAccess{
			Input: prepared.HTTPInput(server.URL + "/original?api_key=" + token),
		},
	})
	request := prepared.Request{Source: prepared.Source{
		ItemID: "live-http-item", SourceID: "live-http-source", Revision: "generated-v1",
	}, Rendition: r}
	if _, ok, err := preparer.Lookup(request); err != nil || ok {
		t.Fatalf("cold prepared lookup = (_, %v, %v), want miss", ok, err)
	}
	pub, err := preparer.Prepare(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := preparer.Lookup(request); err != nil || !ok {
		t.Fatalf("prepared publication lookup = (_, %v, %v), want hit", ok, err)
	}

	if authenticatedRequests.Load() == 0 {
		t.Fatal("ffmpeg made no authenticated HTTP original request")
	}
	manifest, ok, err := library.Open(pub.Key, prepared.MediaManifestName)
	if err != nil || !ok {
		t.Fatalf("manifest = (_, %v, %v), want hit", ok, err)
	}
	body := make([]byte, 8192)
	n, err := manifest.Content.Read(body)
	_ = manifest.Content.Close()
	if err != nil && n == 0 {
		t.Fatal(err)
	}
	if text := string(body[:n]); !strings.Contains(text, "#EXT-X-MAP") || !strings.Contains(text, ".m4s") {
		t.Fatalf("manifest is not playable fMP4 HLS:\n%s", text)
	}

	manifestPath := filepath.Join(pub.Directory, prepared.MediaManifestName)
	output, err := exec.CommandContext(ctx, probe, "-v", "error",
		"-show_entries", "stream=codec_type,codec_name,has_b_frames", "-of", "default=noprint_wrappers=1",
		manifestPath,
	).Output()
	if err != nil {
		t.Fatalf("ffprobe publication: %v", err)
	}
	for _, want := range []string{"codec_name=h264", "codec_type=video", "codec_name=aac", "codec_type=audio", "has_b_frames=0"} {
		if !strings.Contains(string(output), want) {
			t.Errorf("publication probe missing %q:\n%s", want, output)
		}
	}
	if output, err := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-nostdin",
		"-i", manifestPath, "-frames:v", "1", "-f", "null", "-",
	).CombinedOutput(); err != nil {
		t.Fatalf("decode first prepared frame: %v\n%s", err, output)
	}

	t.Run("HEVC also establishes zero reordering", func(t *testing.T) {
		hevc := r
		hevc.VideoCodec, hevc.VideoProfile = "hevc", "main"
		if _, err := prepared.NewFFmpegPackager(bin).Package(ctx, t.TempDir(), prepared.LocalInput(source), 0, hevc); err != nil {
			t.Fatalf("HEVC preparation: %v", err)
		}
	})
	t.Run("encoder override cannot publish reordered output", func(t *testing.T) {
		packager := prepared.NewFFmpegPackager(bin, func(prepared.RenditionContract) (prepared.VideoPlan, error) {
			// libx264's private options can override generic -bf 0. Exercise actual rejected
			// output rather than a mock encoder which merely returns an argument list.
			return prepared.VideoPlan{OutputArgs: []string{
				"-c:v", "libx264", "-x264-params", "bframes=3:b-adapt=0", "-pix_fmt", "yuv420p",
			}}, nil
		})
		otherLibrary, err := prepared.NewLibrary(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		otherPreparer := prepared.NewPreparer(prepared.PreparerDependencies{
			Library: otherLibrary, Packager: packager,
			Access: &testkit.PreparedSourceAccess{Input: prepared.LocalInput(source)},
		})
		if _, err := otherPreparer.Prepare(ctx, request); err == nil || !strings.Contains(err.Error(), "zero video reordering") {
			t.Fatalf("reordered encoder output was not rejected: %v", err)
		}
		if _, ok, err := otherPreparer.Lookup(request); err != nil || ok {
			t.Fatalf("rejected output lookup = (_, %v, %v), want miss", ok, err)
		}
	})
}
