package playout

import (
	"strings"
	"testing"
)

// argVal returns the token immediately after the first occurrence of flag, or "".
func argVal(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// Direct play: the remux STREAM-COPIES, never transcodes. This is what makes a live channel play
// right away (no encoder spin-up) and cost ~no CPU.
func TestHLSArgs_IsDirectCopy(t *testing.T) {
	args := hlsArgs("/seg")
	if got := argVal(args, "-c"); got != "copy" {
		t.Fatalf("remux codec is %q, want copy (direct play)", got)
	}
	// No transcoder, no filtergraph — those would reintroduce the startup latency direct-play exists
	// to avoid.
	for _, banned := range []string{"libx264", "-filter_complex", "-var_stream_map", "-b:v"} {
		for _, a := range args {
			if a == banned {
				t.Fatalf("direct-play args must not contain %q (that is a transcode)", banned)
			}
		}
	}
}

// A single live media playlist, not a master — there are no renditions to list.
func TestHLSArgs_SinglePlaylist(t *testing.T) {
	args := hlsArgs("/seg")
	last := args[len(args)-1]
	if !strings.HasSuffix(last, hlsPlaylistName) {
		t.Fatalf("output target is %q, want it to end in %s", last, hlsPlaylistName)
	}
	if argVal(args, "-master_pl_name") != "" {
		t.Fatal("direct-play is a single playlist — there must be no -master_pl_name")
	}
}

// Live, not VOD: the flags that stop a player treating a never-ending channel as a finite file.
func TestHLSArgs_LiveFlags(t *testing.T) {
	args := hlsArgs("/seg")
	flags := argVal(args, "-hls_flags")
	for _, want := range []string{"delete_segments", "omit_endlist", "independent_segments"} {
		if !strings.Contains(flags, want) {
			t.Fatalf("-hls_flags %q missing %s", flags, want)
		}
	}
	if argVal(args, "-hls_allow_cache") != "0" {
		t.Fatal("live segments must not be cacheable")
	}
}
