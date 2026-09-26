package playout

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// The live child's transcode must keep frames on the GPU from decode to encode when Loomarr knows
// the source (#1512 phase 1a). The old chain decoded on the GPU, downloaded every frame, scaled on
// the CPU and uploaded again: ~0.16 cores and 11x per 1080p stream on the Arc, against ~0.04 cores
// and 21x for the full-GPU graph (spike #1513).

func measuredH264() MediaFormat {
	return MediaFormat{
		VideoCodec: "h264", Width: 1920, Height: 1080, FrameRate: 23.976, PixelFormat: "yuv420p",
		AudioCodec: "eac3", AudioChannels: 6, AudioSampleRate: 48000, Container: "matroska,webm",
	}
}

func liveTranscode(enc Encoder) ProgramSpec {
	return ProgramSpec{
		Profile: Profile{Width: 1920, Height: 1080, Framerate: 25, VideoBitrate: 5000, AudioBitrate: 160, Encoder: enc},
		Input:   "/media/film.mkv", Offset: 90 * time.Second, Limit: 30 * time.Minute,
		Source: measuredH264(),
	}
}

func vfOf(t *testing.T, args []string) string {
	t.Helper()
	i := slices.Index(args, "-vf")
	if i < 0 || i+1 >= len(args) {
		t.Fatalf("no -vf in %q", args)
	}
	return args[i+1]
}

func TestProgramArgs_FullGPUGraphWhenLoomarrKnowsTheSource(t *testing.T) {
	cases := []struct {
		enc                 Encoder
		hwaccel, scale, pad string
	}{
		{EncoderVAAPI, "vaapi", "scale_vaapi=", "pad_vaapi="},
		{EncoderNVENC, "cuda", "scale_cuda=", "pad_cuda="},
	}
	for _, tc := range cases {
		t.Run(string(tc.enc), func(t *testing.T) {
			args := ProgramArgs(liveTranscode(tc.enc))
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "-hwaccel_output_format "+tc.hwaccel) {
				t.Errorf("decoded frames must stay in GPU memory (-hwaccel_output_format %s): %q", tc.hwaccel, joined)
			}
			vf := vfOf(t, args)
			for _, want := range []string{tc.scale, tc.pad, "setparams=color_primaries=bt709"} {
				if !strings.Contains(vf, want) {
					t.Errorf("-vf %q lacks %q", vf, want)
				}
			}
			for _, filter := range strings.Split(vf, ",") {
				name, _, _ := strings.Cut(filter, "=")
				if name == "scale" || name == "pad" || name == "hwupload" || name == "hwupload_cuda" || name == "hwdownload" {
					t.Errorf("CPU round trip %q in a GPU-decodable source's graph %q", name, vf)
				}
			}
			in := slices.Index(args, "-i")
			if p := slices.Index(args, "-probesize"); p < 0 || p > in || args[p+1] != "32768" {
				t.Errorf("stream facts are known, so the input must use minimal probing before -i: %q", joined)
			}
			if g := slices.Index(args, "-g"); g < 0 || args[g+1] != "25" {
				t.Errorf("one closed GOP per 1 s segment at 25 fps wants -g 25: %q", joined)
			}
			for _, a := range args {
				if strings.HasPrefix(a, "-color_") || a == "-colorspace" {
					t.Errorf("output colour flags join format negotiation and break the GPU graph: %q", a)
				}
			}
		})
	}
}
