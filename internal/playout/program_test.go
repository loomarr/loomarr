package playout

import (
	"strings"
	"testing"
	"time"
)

const testStreamURL = "http://emby.local:8096/Videos/abc/stream?static=true&api_key=k"

// Arg ORDER is the thing that bites, because ffmpeg is order-sensitive in ways that fail
// silently rather than loudly. This finds the index of a flag so order can be asserted.
func argIndex(args []string, flag string) int {
	for i, a := range args {
		if a == flag {
			return i
		}
	}
	return -1
}

// THE SEEK PLACEMENT. `-ss` before `-i` makes ffmpeg seek (the HTTP server serves a byte
// range, 2.9s for a 40-minute offset into 4K). After `-i` it decodes and DISCARDS from the
// start of the file — minutes of burnt CPU producing nothing, for identical-looking args.
func TestProgramArgs_SeekGoesBeforeTheInput(t *testing.T) {
	args := ProgramArgs(DefaultProfile(), testStreamURL, 40*time.Minute, time.Hour)

	ss, in := argIndex(args, "-ss"), argIndex(args, "-i")
	if ss < 0 {
		t.Fatal("no -ss for a mid-program tune-in — the joiner would restart the show")
	}
	if ss > in {
		t.Errorf("-ss at %d is AFTER -i at %d: ffmpeg would decode-and-discard 40 minutes "+
			"of 4K rather than seeking", ss, in)
	}
	if v, _ := argsAfter(args, "-ss"); v != "2400.000" {
		t.Errorf("-ss = %q, want 2400.000", v)
	}
}

// Tuning in at the very start must not emit a zero seek — harmless, but it means the arg
// builder is not distinguishing "no offset" from "offset of nothing".
func TestProgramArgs_NoSeekWhenStartingAtTheBeginning(t *testing.T) {
	args := ProgramArgs(DefaultProfile(), testStreamURL, 0, time.Hour)
	if argIndex(args, "-ss") >= 0 {
		t.Errorf("emitted a seek for a zero offset: %v", args)
	}
}

// Sub-second precision matters: a channel is a wall clock, and rounding every tune-in to
// whole seconds accumulates drift across a cycle.
func TestProgramArgs_SeekKeepsSubSecondPrecision(t *testing.T) {
	args := ProgramArgs(DefaultProfile(), testStreamURL, 90*time.Second+500*time.Millisecond, 0)
	if v, _ := argsAfter(args, "-ss"); v != "90.500" {
		t.Errorf("-ss = %q, want 90.500 — sub-second precision was lost", v)
	}
}

// EXPLICIT TRACK MAPS, mandatory not tidy. The verified test item had THREE audio tracks;
// without maps ffmpeg's default selection can vary between programs, and a varying track
// count breaks the parent's `-c copy` exactly like a varying resolution does.
func TestProgramArgs_MapsExactlyOneVideoAndOneAudioTrack(t *testing.T) {
	got := joined(ProgramArgs(DefaultProfile(), testStreamURL, 0, time.Hour))
	if !strings.Contains(got, "-map 0:v:0") {
		t.Error("no explicit video map — track selection could vary between programs")
	}
	if !strings.Contains(got, "-map 0:a:0") {
		t.Error("no explicit audio map — a 3-audio-track remux would break -c copy on the parent")
	}
	if strings.Count(got, "-map ") != 2 {
		t.Errorf("want exactly two maps (one video, one audio), got %q", got)
	}
}

// A child must EXIT at its slot boundary — that EOF is the sequencing signal the parent's
// concat demuxer acts on. A child that played to the end of the file would overrun its slot.
func TestProgramArgs_BoundsTheChildToItsSlot(t *testing.T) {
	args := ProgramArgs(DefaultProfile(), testStreamURL, 0, 20*time.Minute)
	if v, ok := argsAfter(args, "-t"); !ok || v != "1200.000" {
		t.Errorf("-t = %q, want 1200.000 so the child exits at the slot boundary", v)
	}
	// And a child must never loop — that would pin the channel to one program forever.
	if strings.Contains(joined(args), "-stream_loop") {
		t.Error("a child must not loop; only the parent does")
	}
}

// RECONNECT TIERS MUST NOT BE CROSSED (prior-art §5a). `-reconnect_at_eof` on a CHILD means
// the child tries to continue past the end of its own program, which presents as an
// intermittent stall rather than an error.
func TestProgramArgs_UsesChildReconnectTierNotTheParentOne(t *testing.T) {
	got := joined(ProgramArgs(DefaultProfile(), testStreamURL, 0, time.Hour))

	for _, want := range []string{
		"-reconnect 1", "-reconnect_on_network_error 1",
		"-reconnect_streamed 1", "-multiple_requests 1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("child is missing %q — a network blip would kill the slot", want)
		}
	}
	if strings.Contains(got, "-reconnect_at_eof") {
		t.Error("-reconnect_at_eof is the PARENT's flag; on a child it causes intermittent stalls")
	}
}

// Conversely the parent needs exactly that flag, because it is the program-advance MECHANISM
// and not a resilience nicety.
func TestConcatArgs_ReconnectAtEofIsWhatAdvancesPrograms(t *testing.T) {
	got := joined(ConcatArgs("http://loomarr:8080/playout/playlist?c=1"))

	if !strings.Contains(got, "-reconnect_at_eof 1") {
		t.Error("without -reconnect_at_eof the channel plays ONE program and stops — " +
			"a child's EOF must be non-fatal")
	}
	// And it must not get the child tier's flags.
	if strings.Contains(got, "-reconnect_streamed") {
		t.Error("-reconnect_streamed is the CHILD's flag")
	}
}

// `-reconnect*` are options on ffmpeg's HTTP PROTOCOL, not global ones. Against a local file
// input they are a HARD FAILURE — "Option reconnect not found", exit 8, before ffmpeg opens
// anything. Found by executing the args, not by reading them: the arg-shape tests asserted
// the flags were present and passed happily while ffmpeg rejected them.
//
// This is a production path, not a test artifact: filler clips are local files (§10), so an
// unconditional flag list means every commercial break fails to start.
func TestArgs_ReconnectFlagsOnlyForHttpInputs(t *testing.T) {
	local := []struct {
		name string
		args []string
	}{
		{"child on a local file", ProgramArgs(DefaultProfile(), "/media/filler/clip.mp4", 0, time.Minute)},
		{"parent on a local playlist", ConcatArgs("/var/lib/loomarr/list.ffconcat")},
	}
	for _, tc := range local {
		if got := joined(tc.args); strings.Contains(got, "-reconnect") {
			t.Errorf("%s carries a -reconnect flag; ffmpeg exits 8 with "+
				"\"Option reconnect not found\": %q", tc.name, got)
		}
	}

	// …but an http input must still get them.
	if got := joined(ProgramArgs(DefaultProfile(), testStreamURL, 0, time.Minute)); !strings.Contains(got, "-reconnect 1") {
		t.Error("an http child lost its reconnect flags — a network blip would kill the slot")
	}
	if got := joined(ConcatArgs("http://loomarr:8080/playout/playlist")); !strings.Contains(got, "-reconnect_at_eof 1") {
		t.Error("an http parent lost -reconnect_at_eof — the channel would stop after one program")
	}
}

// The protocol whitelist governs the playlist's ENTRIES, so it applies even when the playlist
// itself is a local file — which is the shape the live test uses.
func TestConcatArgs_ProtocolWhitelistIsUnconditional(t *testing.T) {
	if got := joined(ConcatArgs("/var/lib/loomarr/list.ffconcat")); !strings.Contains(got, "-protocol_whitelist") {
		t.Errorf("a local playlist still needs its entries whitelisted: %q", got)
	}
}

// The parent RE-MUXES and never re-encodes. That is what makes one channel cost one encode
// regardless of program count — and if it ever encoded, the children's normalization would
// be pointless work.
func TestConcatArgs_ParentCopiesAndNeverEncodes(t *testing.T) {
	got := joined(ConcatArgs("http://loomarr:8080/playout/playlist?c=1"))
	if !strings.Contains(got, "-c copy") {
		t.Error("the parent must -c copy; encoding here would double the CPU per channel")
	}
	for _, forbidden := range []string{"-c:v", "-b:v", "-preset", "-crf"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("parent carries encode arg %q — it must only re-mux: %q", forbidden, got)
		}
	}
}

// The concat demuxer refuses an http:// entry unless told the protocol is allowed, and
// resolves entries relative to the playlist unless -safe 0. Both failures look like "the
// playlist is broken" rather than naming the flag.
func TestConcatArgs_AllowsHttpEntriesInThePlaylist(t *testing.T) {
	got := joined(ConcatArgs("http://loomarr:8080/playout/playlist?c=1"))
	if !strings.Contains(got, "-protocol_whitelist") || !strings.Contains(got, "http") {
		t.Error("no protocol whitelist — the demuxer refuses http:// playlist entries")
	}
	if !strings.Contains(got, "-safe 0") {
		t.Error("no -safe 0 — absolute URLs in the playlist are rejected")
	}
	if !strings.Contains(got, "-stream_loop -1") {
		t.Error("no -stream_loop -1 — the parent must cycle the playlist forever")
	}
}

// TWO entries, not one. The demuxer needs something to ADVANCE TO when the first hits EOF;
// a one-line playlist ends the channel after the first program.
func TestPlaylist_HasTwoIdenticalEntries(t *testing.T) {
	const u = "http://loomarr:8080/playout/program?c=1&token=t"
	got := Playlist(u)

	if !strings.HasPrefix(got, "ffconcat version 1.0\n") {
		t.Errorf("missing the ffconcat header: %q", got)
	}
	if n := strings.Count(got, "file '"); n != 2 {
		t.Errorf("%d entries, want exactly 2 — the demuxer needs one to advance to", n)
	}
	if !strings.Contains(got, "file '"+u+"'") {
		t.Errorf("entry does not carry the program URL: %q", got)
	}
}

// A quote in the URL would terminate the `file '…'` directive and make the playlist parse as
// something else entirely rather than failing cleanly.
func TestPlaylist_EscapesQuotesAndStripsNewlines(t *testing.T) {
	got := Playlist("http://h/p?c=it's&x=1\ninjected")

	if strings.Contains(got, "injected\n") && strings.Count(got, "\n") > 3 {
		t.Errorf("a newline in the URL added a playlist line: %q", got)
	}
	if strings.Contains(got, "?c=it's") {
		t.Errorf("unescaped quote terminates the directive: %q", got)
	}
	if !strings.Contains(got, `'\''`) {
		t.Errorf("want the ffconcat quote escape, got %q", got)
	}
}

// THE FILTER-GRAPH FAILURE the synthetic card could not find (prior-art §5b): scale emits CPU
// frames, and a hardware encoder that wants GPU frames fails with a 40-line pixel-format dump
// that never names the real problem.
//
// The invariant is NOT "every hardware encoder uploads" — an earlier version of this test
// asserted that and was simply wrong about ffmpeg. Only the families with a separate
// hardware frame pool (vaapi, qsv, vulkan) need it; nvenc, amf, videotoolbox, rkmpp and
// v4l2m2m accept CPU frames directly, and forcing an upload on them CAUSES the failure this
// test exists to prevent. The real invariant is: whoever uploads does it AFTER scaling, and
// the per-encoder decision comes from one place.
func TestScaleFilter_UploadsAfterScalingWhereRequired(t *testing.T) {
	for _, enc := range []Encoder{
		EncoderVAAPI, EncoderNVENC, EncoderQSV, EncoderVulkan,
		EncoderAMF, EncoderVideoToolbox, EncoderRKMPP, EncoderV4L2M2M,
	} {
		p := Profile{Width: 1280, Height: 720, Framerate: 25, Encoder: enc}
		vf, ok := argsAfter(p.scaleFilterArgs(), "-vf")
		if !ok {
			t.Errorf("%s: no filter chain", enc)
			continue
		}
		// Whether this family uploads is the prober's call, not this test's — asking the same
		// helper the args use is what keeps the two from drifting.
		if hardwareUploadFilter(enc) == "" {
			if strings.Contains(vf, "hwupload") {
				t.Errorf("%s: accepts CPU frames directly; an upload here fails at init: %q", enc, vf)
			}
			continue
		}
		if !strings.Contains(vf, "hwupload") {
			t.Errorf("%s: no hwupload after scale — the encoder gets CPU frames and fails "+
				"at init: %q", enc, vf)
		}
		// Order is the whole point: uploading before scaling would run a CPU filter on GPU
		// frames, which is the same error in the other direction.
		if strings.Index(vf, "hwupload") < strings.Index(vf, "scale=") {
			t.Errorf("%s: hwupload precedes scale: %q", enc, vf)
		}
		if !strings.Contains(vf, "format=nv12") {
			t.Errorf("%s: no nv12 conversion before upload: %q", enc, vf)
		}
	}
}

// A hardware encoder that uploads frames also needs its DEVICE initialised, before the input.
// Without it the error is "[hwupload] A hardware device reference is required to upload frames
// to" — which reads like a filter bug. The prober found this once; ProgramArgs reproduced it
// by not reusing the prober's helper, so this pins the composition.
func TestProgramArgs_InitialisesTheHardwareDeviceBeforeTheInput(t *testing.T) {
	for _, enc := range encoderPreference {
		want := deviceInitArgs(enc)
		if len(want) == 0 {
			continue // this family initialises its own context
		}
		p := Profile{Width: 1280, Height: 720, Framerate: 25, Encoder: enc}
		args := ProgramArgs(p, testStreamURL, 0, time.Minute)

		if !strings.Contains(joined(args), joined(want)) {
			t.Errorf("%s: missing device init %v — hwupload would fail with a message that "+
				"names the filter, not the device: %v", enc, want, args)
			continue
		}
		// Global option: after -i it silently applies to nothing.
		if argIndex(args, want[0]) > argIndex(args, "-i") {
			t.Errorf("%s: device init is after -i, so it applies to nothing", enc)
		}
	}
}

// Software needs no upload — and adding one would fail, since there is no hardware device.
func TestScaleFilter_SoftwareGetsNoHardwareUpload(t *testing.T) {
	p := Profile{Width: 1280, Height: 720, Framerate: 25, Encoder: EncoderSoftware}
	vf, _ := argsAfter(p.scaleFilterArgs(), "-vf")
	if strings.Contains(vf, "hwupload") {
		t.Errorf("software must not hwupload (there is no device): %q", vf)
	}
	// yuv420p explicitly: a 10-bit HDR source would otherwise carry its pixel format through
	// and produce a stream many players cannot decode.
	if !strings.Contains(vf, "format=yuv420p") {
		t.Errorf("no yuv420p — a 10-bit HDR source would pass its format through: %q", vf)
	}
}

// EVERY knob `-c copy` depends on must be pinned, for every encoder family. This is the test
// that catches "a child quietly differed" before it becomes a channel that dies mid-program.
func TestProgramArgs_PinsEverythingConcatDependsOn(t *testing.T) {
	for _, enc := range encoderPreference {
		p := Profile{Width: 1280, Height: 720, Framerate: 25, VideoBitrate: 3000,
			AudioBitrate: 128, Encoder: enc}
		got := joined(ProgramArgs(p, testStreamURL, 0, time.Hour))

		// Resolution AND the pad that preserves it: a bare aspect-preserving scale would
		// emit 960x720 for 4:3 content and break concatenation.
		if !strings.Contains(got, "scale=1280:720") {
			t.Errorf("%s: resolution not pinned: %q", enc, got)
		}
		if !strings.Contains(got, "pad=1280:720") {
			t.Errorf("%s: no pad — 4:3 content would emit different dimensions", enc)
		}
		// Framerate: a 24fps film and a 25fps episode must not produce different rates.
		if !strings.Contains(got, "fps=25") {
			t.Errorf("%s: framerate not pinned: %q", enc, got)
		}
		// Codec + audio layout.
		if !strings.Contains(got, "-c:v "+string(enc)) {
			t.Errorf("%s: video codec not set", enc)
		}
		if !strings.Contains(got, "-c:a aac") || !strings.Contains(got, "-ac 2") ||
			!strings.Contains(got, "-ar 48000") {
			t.Errorf("%s: audio layout not pinned — breaks -c copy like video does", enc)
		}
		// Container + the mid-flight timestamp flag.
		if !strings.Contains(got, "-f mpegts") {
			t.Errorf("%s: not muxing to mpegts", enc)
		}
		if !strings.Contains(got, "+initial_discontinuity") {
			t.Errorf("%s: no initial_discontinuity — we seeked, so timestamps are not zero-based", enc)
		}
	}
}

// Realtime pacing WITH a burst. Pacing alone is correct but feels broken: a joining player
// has an empty buffer and waits for it to fill at 1.0x before showing anything.
func TestProgramArgs_PacesRealtimeWithATuneInBurst(t *testing.T) {
	args := ProgramArgs(DefaultProfile(), testStreamURL, 0, time.Hour)

	if v, ok := argsAfter(args, "-readrate"); !ok || v != "1.0" {
		t.Errorf("-readrate = %q, want 1.0 — without pacing we race ahead of wall-clock", v)
	}
	if v, ok := argsAfter(args, "-readrate_initial_burst"); !ok || v == "0" {
		t.Errorf("-readrate_initial_burst = %q, want a burst so tune-in is not slow", v)
	}
	// Pacing must be an INPUT option (before -i) or it applies to nothing.
	if rr := argIndex(args, "-readrate"); rr > argIndex(args, "-i") {
		t.Error("-readrate is after -i, so it applies to no input")
	}
}

// Progress on a dedicated fd, same as the card — stdout carries the MPEG-TS, so mixing
// progress text into it would corrupt the transport stream.
func TestProgramArgs_ProgressIsStructuredAndOffStdout(t *testing.T) {
	for name, args := range map[string][]string{
		"child":  ProgramArgs(DefaultProfile(), testStreamURL, 0, time.Hour),
		"parent": ConcatArgs("http://loomarr:8080/playout/playlist?c=1"),
	} {
		v, ok := argsAfter(args, "-progress")
		if !ok || !strings.HasPrefix(v, "pipe:") {
			t.Errorf("%s: -progress = %q, want a pipe fd", name, v)
		}
		if v == "pipe:1" {
			t.Errorf("%s: progress on stdout would corrupt the MPEG-TS", name)
		}
	}
}

// Both processes write the stream to stdout, which is what Process.Stdout fans out.
func TestArgs_OutputGoesToStdout(t *testing.T) {
	for name, args := range map[string][]string{
		"child":  ProgramArgs(DefaultProfile(), testStreamURL, 0, time.Hour),
		"parent": ConcatArgs("http://loomarr:8080/playout/playlist?c=1"),
		"card":   TestCardArgs(DefaultProfile(), "", "CH", ""),
	} {
		if args[len(args)-1] != "pipe:1" {
			t.Errorf("%s: last arg = %q, want pipe:1", name, args[len(args)-1])
		}
	}
}

// seconds must never emit exponent notation — ffmpeg would parse "1e-06" as a token rather
// than a duration.
func TestSeconds_NeverUsesExponentNotation(t *testing.T) {
	for _, d := range []time.Duration{
		time.Microsecond, time.Millisecond, time.Second,
		90*time.Minute + 500*time.Millisecond, 0,
	} {
		got := seconds(d)
		if strings.ContainsAny(got, "eE") {
			t.Errorf("seconds(%v) = %q, which ffmpeg cannot parse", d, got)
		}
	}
}
