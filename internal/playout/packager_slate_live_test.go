//go:build ffmpeg

package playout

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/playout/packager"
)

// TestSlateSampleDescriptionMatchesEveryItem is the slate's contract with the channel: the slate
// is encoded by the channel's own builder and encoder, so its sample descriptions (stsd: SPS/PPS,
// colour, AAC config, and the track list itself) are byte-identical to any item's. A difference is
// a decoder re-init at every slate boundary, or a slate the channel's decoders cannot play.
//
// The sources carry what real films do and lavfi test sources do not: HDR mastering-display and
// content-light metadata (live, it survived the tone-map into mdcv/clli boxes) and chapters (the
// mp4 muxer wrote them as a third, text track).
//
// Software always; NVENC too with LOOMARR_TEST_NVENC=1 (run it under the GPU lock).
func TestSlateSampleDescriptionMatchesEveryItem(t *testing.T) {
	dir := t.TempDir()
	chapters := filepath.Join(dir, "chapters.txt")
	if err := os.WriteFile(chapters, []byte(";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1000\ntitle=One\n"+
		"[CHAPTER]\nTIMEBASE=1/1000\nSTART=1000\nEND=3000\ntitle=Two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	type src struct {
		name   string
		gen    []string
		format MediaFormat
	}
	sources := []src{
		{"sdr-film", []string{"-f", "lavfi", "-i", "testsrc2=s=1920x800:r=24000/1001", "-f", "lavfi", "-i", "sine=r=48000",
			"-i", chapters, "-map", "0", "-map", "1", "-map_chapters", "2", "-t", "3",
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", "6"},
			MediaFormat{VideoCodec: "h264", Width: 1920, Height: 800, FrameRate: 23.976, PixelFormat: "yuv420p",
				AudioCodec: "aac", AudioChannels: 6, AudioSampleRate: 48000, Container: "matroska,webm"}},
		{"hdr-film", []string{"-f", "lavfi", "-i", "testsrc2=s=3840x1600:r=24000/1001", "-f", "lavfi", "-i", "sine=r=48000",
			"-i", chapters, "-map", "0", "-map", "1", "-map_chapters", "2", "-t", "3",
			"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-x265-params",
			"log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:hdr10=1:" +
				"master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):max-cll=1000,400",
			"-c:a", "aac"},
			MediaFormat{VideoCodec: "hevc", Width: 3840, Height: 1600, FrameRate: 23.976, PixelFormat: "yuv420p10le",
				ColorTransfer: "smpte2084", AudioCodec: "aac", AudioChannels: 1, AudioSampleRate: 48000,
				Container: "matroska,webm"}},
		{"hdr-1080", []string{"-f", "lavfi", "-i", "testsrc2=s=1920x800:r=24000/1001", "-f", "lavfi", "-i", "sine=r=48000",
			"-i", chapters, "-map", "0", "-map", "1", "-map_chapters", "2", "-t", "3",
			"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-x265-params",
			"log-level=error:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:hdr10=1:" +
				"master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):max-cll=1000,400",
			"-c:a", "aac"},
			MediaFormat{VideoCodec: "hevc", Width: 1920, Height: 800, FrameRate: 23.976, PixelFormat: "yuv420p10le",
				ColorTransfer: "smpte2084", AudioCodec: "aac", AudioChannels: 1, AudioSampleRate: 48000,
				Container: "matroska,webm"}},
		{"sd-ad", []string{"-f", "lavfi", "-i", "smptebars=s=720x480:r=30000/1001", "-f", "lavfi", "-i", "sine=r=44100",
			"-t", "3", "-vf", "setsar=10/11", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"},
			MediaFormat{VideoCodec: "h264", Width: 720, Height: 480, FrameRate: 29.97, PixelFormat: "yuv420p",
				AudioCodec: "aac", AudioChannels: 1, AudioSampleRate: 44100, Container: "matroska,webm"}},
	}
	for _, s := range sources {
		ffmpegRun(t, append(s.gen, filepath.Join(dir, s.name+".mkv"))...)
	}

	out := OutputProfile{Width: 1920, Height: 1080, FPS: 25, Quality: 23, TargetKbps: 6000, MaxKbps: 9000, GOPSeconds: 1, AudioKbps: 160}
	hosts := map[string]HostProfile{"software": HostFor(EncoderSoftware, true, GPUFilters{})}
	if os.Getenv("LOOMARR_TEST_NVENC") == "1" {
		hosts["nvenc"] = HostFor(EncoderNVENC, true, GPUFiltersFor("ffmpeg")())
	}
	for name, host := range hosts {
		t.Run(name, func(t *testing.T) {
			m, err := NewPackagerHLS(nil, "ffmpeg", t.TempDir(), 0, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(m.Stop)
			slate, err := m.slate(context.Background(), host, out)
			if err != nil {
				t.Fatal(err)
			}
			tested := 0
			for _, s := range sources {
				pl, err := Build(host, s.format, out)
				if errors.Is(err, ErrRefused) {
					t.Logf("%s: refused on this host: %v", s.name, err)
					continue
				}
				if err != nil {
					t.Fatalf("%s: %v", s.name, err)
				}
				tested++
				args := pl.FragmentArgs(filepath.Join(dir, s.name+".mkv"), 0, 0, int64(out.FPS), 48, out.FPS, 0)
				var stderr bytes.Buffer
				cmd := exec.Command("ffmpeg", args...)
				cmd.Stderr = &stderr
				encoded, err := cmd.Output()
				if err != nil {
					t.Fatalf("%s encode: %v\n%s", s.name, err, stderr.Bytes())
				}
				item, err := packager.NewSlate(encoded) // parses the init exactly as the packager does
				if err != nil {
					t.Fatalf("%s: %v", s.name, err)
				}
				if !packager.SameDecoderConfig(slate.Init(), item.Init()) {
					t.Errorf("%s (fallbacks %v): sample descriptions differ from the slate's\nslate %x\nitem  %x",
						s.name, pl.Fallbacks, packager.SampleDescriptions(slate.Init()), packager.SampleDescriptions(item.Init()))
				}
			}
			// Software refuses HDR tone-mapping, so HDR is covered by a GPU host (LOOMARR_TEST_NVENC=1);
			// the two SDR sources build everywhere.
			if tested < 2 {
				t.Fatalf("only %d of %d sources built on this host", tested, len(sources))
			}
		})
	}
}
