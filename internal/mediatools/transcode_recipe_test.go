package mediatools

import (
	"slices"
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

func containsArgumentPair(args []string, first, second string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return true
		}
	}
	return false
}
