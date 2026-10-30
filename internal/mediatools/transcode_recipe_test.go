package mediatools

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/encode"
)

// Every family, decoding on the GPU or not, encodes the picture it decoded: the encode branch only
// converts the pixel format (on the device that holds the frames), the detectors watch the full
// picture in a branch that ends in a sink, and the derivative contract's mapping and container
// flags hold.
func TestTranscodeArgumentsMakeTheDerivativeContractExplicit(t *testing.T) {
	for _, c := range []struct {
		family    encode.Family
		gpuDecode bool
		encoder   string
	}{
		{encode.FamilySoftware, false, "libx264"},
		{encode.FamilyNVENC, false, "h264_nvenc"},
		{encode.FamilyNVENC, true, "h264_nvenc"},
		{encode.FamilyVAAPI, false, "h264_vaapi"},
		{encode.FamilyVAAPI, true, "h264_vaapi"},
	} {
		name := fmt.Sprintf("%s gpuDecode=%v", c.family, c.gpuDecode)
		profile := EvidenceDerivativeRecipe(DefaultMezzanine()).Profile()
		if c.family != encode.FamilySoftware {
			profile.Family, profile.Preset = c.family, ""
		}
		args := transcodeArguments(TranscodeRequest{
			In: "source.mkv", Out: "evidence.mp4", HadAudio: true, Profile: profile,
			Host: encode.HostProfile{RenderNode: "/dev/dri/renderD128"},
		}, "evidence.tmp.mp4", c.gpuDecode)
		wants := [][]string{
			{"-c:v", c.encoder}, {"-map", "[v]"}, {"-map", "0:a:0?"}, {"-map_metadata", "-1"}, {"-map_chapters", "-1"},
			{"-fps_mode", "passthrough"}, {"-avoid_negative_ts", "make_zero"}, {"-movflags", "+faststart"},
		}
		for _, want := range wants {
			if !containsArgumentPair(args, want[0], want[1]) {
				t.Errorf("%s: args missing %v: %v", name, want, args)
			}
		}
		graph, _ := argumentAfter(args, "-filter_complex")
		det, enc, ok := strings.Cut(strings.TrimPrefix(graph, "[0:v]split=2[enc][det];[det]"), ";[enc]")
		if !ok || !strings.HasSuffix(enc, "[v]") {
			t.Fatalf("%s: graph is not one split into a detector and an encode branch: %q", name, graph)
		}
		if !strings.HasSuffix(det, qualityVideoFilters+",nullsink") {
			t.Errorf("%s: detector branch %q does not end in the detectors and a sink", name, det)
		}
		// Neither branch resizes: a downscaled detector copy invents freezes (#2121).
		for branch, filters := range map[string]string{"encode": enc, "detector": det} {
			for _, forbidden := range []string{"=w=", ":w=", ":h=", "scale=", "crop", "pad", "yadif", "bwdif", "hqdn3d", "unsharp", "fps", "setsar"} {
				if strings.Contains(filters, forbidden) {
					t.Errorf("%s: %s branch %q contains undeclared transform %q", name, branch, filters, forbidden)
				}
			}
		}
		if slices.Contains(args, "loudnorm") {
			t.Errorf("%s: evidence args normalise loudness: %v", name, args)
		}
		_, hwDecode := argumentAfter(args, "-hwaccel")
		if hwDecode != c.gpuDecode {
			t.Errorf("%s: -hwaccel present=%v: %v", name, hwDecode, args)
		}
	}
}

func argumentAfter(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func TestVerifyPreservedMediaRejectsUndeclaredSignalChanges(t *testing.T) {
	base := Probed{
		DurationMs: 30_000, Width: 720, Height: 480, Cadence: "30000/1001",
		SampleAspect: "8:9", DisplayAspect: "4:3", FieldOrder: "progressive",
		VideoStartMs: 0, VideoDurationMs: 30_000, VideoTimingKnown: true,
		AudioStartMs: 20, AudioDurationMs: 29_980, AudioTimingKnown: true,
	}
	equivalent := base
	equivalent.Cadence = "60000/2002"
	if err := verifyPreservedMedia(base, equivalent); err != nil {
		t.Fatalf("equivalent rational observations were refused: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*Probed)
		want   string
	}{
		{name: "geometry", mutate: func(p *Probed) { p.Width = 640 }, want: "geometry changed"},
		{name: "cadence", mutate: func(p *Probed) { p.Cadence = "25/1" }, want: "cadence changed"},
		{name: "aspect", mutate: func(p *Probed) { p.DisplayAspect = "16:9" }, want: "display aspect changed"},
		{name: "interlace", mutate: func(p *Probed) { p.FieldOrder = "tt" }, want: "field order changed"},
		{name: "start skew", mutate: func(p *Probed) { p.AudioStartMs += 100 }, want: "A/V start skew changed"},
		{name: "end skew", mutate: func(p *Probed) { p.AudioDurationMs += 700 }, want: "A/V end skew changed"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			output := base
			test.mutate(&output)
			if err := verifyPreservedMedia(base, output); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("verify error = %v, want %q", err, test.want)
			}
		})
	}
}

func containsArgumentPair(args []string, first, second string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return true
		}
	}
	return false
}

// A filler encode is background batch work in a container that also serves the app and live
// playout, so it must never fan out to every core (#1512 G5: an uncapped libx264 encode measured
// ~350% CPU in a 4-CPU container and slowed every page).
func TestTranscodeArgumentsCapThreads(t *testing.T) {
	for name, profile := range map[string]MezzanineProfile{
		"playback": DefaultMezzanine(),
		"evidence": EvidenceDerivativeRecipe(DefaultMezzanine()).Profile(),
	} {
		args := transcodeArguments(TranscodeRequest{In: "in.mkv", Out: "out.mp4", HadAudio: true, Profile: profile}, "tmp.mp4", false)
		if !containsArgumentPair(args, "-threads", strconv.Itoa(BackgroundThreads)) {
			t.Errorf("%s: no -threads %d: %v", name, BackgroundThreads, args)
		}
		// libx264 keeps its own thread pool (frame threads + a lookahead thread) that `-threads` does not
		// fully bound on every build, so it is capped explicitly too.
		if !containsArgumentPair(args, "-x264-params", "threads="+strconv.Itoa(BackgroundThreads)) {
			t.Errorf("%s: libx264 thread pool not capped: %v", name, args)
		}
		// -threads is per-side: before -i it bounds the DECODER, after it the ENCODER. Both need it.
		in, first := slices.Index(args, "-i"), slices.Index(args, "-threads")
		if first < 0 || first > in || slices.Index(args[in:], "-threads") < 0 {
			t.Errorf("%s: -threads must cap both the decoder (before -i) and the encoder (after): %v", name, args)
		}
	}
}
