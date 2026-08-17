package playout

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLiveHLSRetainsTheSharedDVRHorizon(t *testing.T) {
	if DVRHorizon != 15*time.Minute {
		t.Fatalf("DVRHorizon = %v, want 15m", DVRHorizon)
	}
	args := hlsArgs("/seg", PlanBaseline)
	if got := argVal(args, "-hls_list_size"); got != strconv.Itoa(15*60/4) {
		t.Fatalf("live segment window = %q, want 225 four-second segments", got)
	}
	if got := argVal(args, "-hls_delete_threshold"); got != "1" {
		t.Fatalf("live delete threshold = %q, want one unreferenced segment", got)
	}
	if flags := argVal(args, "-hls_flags"); !strings.Contains(flags, "program_date_time") {
		t.Fatalf("-hls_flags %q missing program_date_time", flags)
	}
}

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
	args := hlsArgs("/seg", PlanBaseline)
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
	args := hlsArgs("/seg", PlanBaseline)
	last := args[len(args)-1]
	if !strings.HasSuffix(last, hlsPlaylistName) {
		t.Fatalf("output target is %q, want it to end in %s", last, hlsPlaylistName)
	}
	if argVal(args, "-master_pl_name") != "" {
		t.Fatal("direct-play is a single playlist — there must be no -master_pl_name")
	}
}

// ⚠ The baseline plan uses MPEG-TS segments (h264 plays in TS everywhere, and it is the proven
// first-paint path); the HEVC plans MUST use fMP4 (Apple's HLS spec — HEVC in MPEG-TS black-screens
// in a browser). This is the fix for the HEVC-direct-play black screen (§9.1 V48).
func TestHLSArgs_SegmentContainerByPlan(t *testing.T) {
	baseline := hlsArgs("/seg", PlanBaseline)
	if got := argVal(baseline, "-hls_segment_type"); got != "mpegts" {
		t.Fatalf("baseline segment type is %q, want mpegts", got)
	}
	if !strings.HasSuffix(argVal(baseline, "-hls_segment_filename"), ".ts") {
		t.Fatal("baseline segments must be .ts")
	}
	if argVal(baseline, "-hls_fmp4_init_filename") != "" {
		t.Fatal("baseline (MPEG-TS) must have no fMP4 init segment")
	}

	for _, plan := range []EncodePlan{PlanHEVC8, PlanHEVC10, PlanFull} {
		args := hlsArgs("/seg", plan)
		if got := argVal(args, "-hls_segment_type"); got != "fmp4" {
			t.Fatalf("%v segment type is %q, want fmp4 (HEVC HLS must be fMP4)", plan, got)
		}
		if !strings.HasSuffix(argVal(args, "-hls_segment_filename"), ".m4s") {
			t.Fatalf("%v segments must be .m4s", plan)
		}
		if argVal(args, "-hls_fmp4_init_filename") != "init.mp4" {
			t.Fatalf("%v must declare an fMP4 init segment", plan)
		}
		// Still a copy — a REMUX (TS→fMP4), never a transcode.
		if argVal(args, "-c") != "copy" {
			t.Fatalf("%v must still -c copy (remux, not transcode)", plan)
		}
		// ⚠ The two TS→fMP4 bitstream fixes — WITHOUT these the mux fails (AAC) or the browser
		// black-screens (hev1). Verified by feeding real HEVC through this exact command; guard so it
		// can never silently regress.
		if argVal(args, "-bsf:a") != "aac_adtstoasc" {
			t.Fatalf("%v fMP4 remux must carry -bsf:a aac_adtstoasc (else the MP4 muxer rejects AAC-in-ADTS and the mux dies)", plan)
		}
		if argVal(args, "-tag:v") != "hvc1" {
			t.Fatalf("%v fMP4 remux must tag video hvc1 (MSE/Safari decode hvc1, not the default hev1)", plan)
		}
	}
}

// Live, not VOD: the flags that stop a player treating a never-ending channel as a finite file.
func TestHLSArgs_LiveFlags(t *testing.T) {
	args := hlsArgs("/seg", PlanBaseline)
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
