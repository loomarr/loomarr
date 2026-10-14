package mediatools

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestTranscodeArgumentsMakeTheDerivativeContractExplicit(t *testing.T) {
	recipe := EvidenceDerivativeRecipe()
	args := transcodeArguments(TranscodeRequest{
		In: "source.mkv", Out: "evidence.mp4", HadAudio: true, Profile: recipe.Profile(),
	}, "evidence.tmp.mp4")
	wants := [][]string{
		{"-map", "0:v:0"}, {"-map", "0:a:0?"}, {"-map_metadata", "-1"}, {"-map_chapters", "-1"},
		{"-fps_mode", "passthrough"}, {"-avoid_negative_ts", "make_zero"}, {"-movflags", "+faststart"},
	}
	for _, want := range wants {
		if !containsArgumentPair(args, want[0], want[1]) {
			t.Errorf("transcode args missing %v: %v", want, args)
		}
	}
	for _, forbidden := range []string{"scale", "crop", "yadif", "bwdif", "hqdn3d", "unsharp", "loudnorm"} {
		if slices.Contains(args, forbidden) {
			t.Errorf("evidence args contain undeclared transform %q: %v", forbidden, args)
		}
		for _, arg := range args {
			if len(arg) >= len(forbidden) && arg[:len(forbidden)] == forbidden {
				t.Errorf("evidence args contain undeclared transform %q: %v", forbidden, args)
			}
		}
	}
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
		"evidence": EvidenceDerivativeRecipe().Profile(),
	} {
		args := transcodeArguments(TranscodeRequest{In: "in.mkv", Out: "out.mp4", HadAudio: true, Profile: profile}, "tmp.mp4")
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
