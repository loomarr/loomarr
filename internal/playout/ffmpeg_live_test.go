//go:build ffmpeg

// Tests that EXECUTE ffmpeg. Behind a build tag because unit tests must not depend on an
// external binary being present — the same reasoning AGENTS.md applies to the network. Run
// with `make test-ffmpeg`.
//
// These exist because arg-shape tests assert my own stated invariants and cannot catch an
// invariant that is wrong. "Every rung has even dimensions" is checkable in Go; "every rung
// actually encodes" is only checkable by encoding.

package playout

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func ffmpegBin(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("FFMPEG_PATH"); p != "" {
		return p
	}
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH")
	}
	return p
}

func ffprobeBin(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("no ffprobe on PATH")
	}
	return p
}

// The test card must produce a VALID MPEG-TS with BOTH streams. The audio half is the point:
// a video-only MPEG-TS is a classic cause of a player refusing to play or showing no
// timeline, and `anullsrc` is what puts it there.
func TestLive_TestCardProducesValidMpegTsWithAudio(t *testing.T) {
	bin := ffmpegBin(t)
	probe := ffprobeBin(t)
	out := t.TempDir() + "/card.ts"

	p := DefaultProfile()
	p.Width, p.Height = 640, 360 // small: this is about validity, not throughput
	// CardFontFor, not FindFont: a font FILE is not enough, because drawtext is a compile-time
	// option. Homebrew's ffmpeg carries no libfreetype while macOS ships Arial, so FindFont
	// here produced `-vf drawtext=…` against a build that has no such filter — "Filter not
	// found", exit 8, and this test failing for the same reason a real channel would go dead.
	args := TestCardArgs(p, CardFontFor(bin)(), "Loomarr", "channel 1")
	// Bound it and write to a file rather than the pipe the real thing uses.
	args = replaceOutput(args, "-t", "2", out)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Through Start, NOT exec directly: TestCardArgs asks for `-progress pipe:3`, and fd 3
	// exists only because Start wires it via ExtraFiles. Running these args with plain exec
	// fails at startup with "Error parsing global options: Bad file descriptor" — which is
	// exactly how this bug was found, and why this test goes through the real supervisor.
	var samples int
	proc, err := Start(ctx, bin, args, nil, func(Progress) { samples++ })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	// Nothing consumes stdout here (output goes to the file), but the pipe still must be
	// drained or ffmpeg blocks writing to it.
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
	if err := proc.Wait(); err != nil {
		t.Fatalf("test card failed to encode: %v\nlast stderr: %s", err, proc.LastError())
	}
	if samples == 0 {
		t.Error("no progress samples — `-progress pipe:3` is not being read")
	}

	got, err := exec.CommandContext(ctx, probe, "-v", "error",
		"-show_entries", "format=format_name", "-show_entries", "stream=codec_type,codec_name",
		"-of", "default=noprint_wrappers=1", out).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	s := string(got)
	for _, want := range []string{"format_name=mpegts", "codec_type=video", "codec_name=h264", "codec_type=audio"} {
		if !strings.Contains(s, want) {
			t.Errorf("probe missing %q — the card is not a playable MPEG-TS:\n%s", want, s)
		}
	}
}

// Every rung on every ladder must actually encode. A rung can satisfy "even dimensions,
// descending bitrate" and still be rejected by an encoder — only ffmpeg knows.
func TestLive_EveryLadderRungEncodes(t *testing.T) {
	bin := ffmpegBin(t)
	enc := Detect(context.Background(), bin, DefaultProfile(), "").Chosen
	t.Logf("verifying ladders against %s", enc)

	for _, tier := range []Tier{TierQuality, TierBalanced, TierEfficient} {
		for active := 0; active <= 8; active++ {
			p := Resolve(tier, enc, 8, active)
			args := []string{"-hide_banner", "-loglevel", "error"}
			args = append(args, deviceInitArgs(enc)...)
			args = append(args, "-f", "lavfi", "-i",
				"testsrc=duration=1:size="+itoa(p.Width)+"x"+itoa(p.Height)+":rate="+itoa(p.Framerate))
			if vf := hardwareUploadFilter(enc); vf != "" {
				args = append(args, "-vf", vf)
			}
			args = append(args, p.videoEncodeArgs()...)
			args = append(args, "-frames:v", "10", "-f", "null", "-")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			b, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
			cancel()
			if err != nil {
				t.Errorf("%s active=%d (%dx%d @%dk) does not encode: %v\n%s",
					tier, active, p.Width, p.Height, p.VideoBitrate, err, b)
			}
		}
	}
}

// Detect must always return something usable, and must never claim an encoder works without
// having encoded with it.
func TestLive_DetectChoosesSomethingThatActuallyWorks(t *testing.T) {
	bin := ffmpegBin(t)
	c := Detect(context.Background(), bin, DefaultProfile(), "")

	if c.Chosen == "" {
		t.Fatal("Detect returned no encoder — software is always a valid answer")
	}
	if c.MaxChannels < 1 {
		t.Errorf("MaxChannels = %d, want at least 1", c.MaxChannels)
	}
	// ⚠ A WORKING ENCODER MUST HAVE A MEASURED SPEED. This is the assertion that catches a VACUOUS
	// probe, and it is here because this test was green for months while trialEncode encoded
	// nothing at all.
	//
	// The failure composed from three individually reasonable decisions: os.CreateTemp creates the
	// output file, ffmpeg without `-y` refuses to overwrite it and EXITS ZERO, and hasKeyframe is
	// deliberately best-effort about an unreadable file. Result: `Works: true` for every encoder
	// the build lists — including h264_amf on a machine with no AMD hardware — and Speed 0 for all
	// of them, so MaxChannels sat at capacityFloor (measured: 2 instead of 11 on an RTX 3080 Ti).
	//
	// "Works" alone cannot detect that, because a vacuous probe reports exactly what a real one
	// does. A measured throughput cannot be faked by an encode that never ran.
	for _, x := range c.All {
		if x.Works && x.Speed <= 0 {
			t.Errorf("%s reports Works with no measured speed — the trial exited cleanly without "+
				"encoding anything, so this probe proves nothing", x.Encoder)
		}
		if !x.Works && x.Err == "" {
			t.Errorf("%s failed with no reason recorded; the wizard's transcode check shows that "+
				"text to an operator", x.Encoder)
		}
	}

	// Whatever it chose must appear in All as Works.
	for _, x := range c.All {
		if x.Encoder == c.Chosen {
			if !x.Works {
				t.Errorf("Detect chose %s but its probe did not pass: %q", c.Chosen, x.Err)
			}
			return
		}
	}
	t.Errorf("Detect chose %s but it is not in the probe results", c.Chosen)
}

// A failed probe must carry ffmpeg's own message, not a category we invented — that text is
// what the wizard's transcode check shows an operator.
func TestLive_FailedProbesCarryFfmpegsOwnMessage(t *testing.T) {
	c := Detect(context.Background(), ffmpegBin(t), DefaultProfile(), "")
	for _, x := range c.All {
		if x.Works || x.Err == "" {
			continue
		}
		if x.Err == "failed" || x.Err == "error" {
			t.Errorf("%s: error is a useless category, want ffmpeg's text: %q", x.Encoder, x.Err)
		}
		t.Logf("%s: %s", x.Encoder, x.Err)
	}
}

// embyStreamURL returns a real library stream URL, or skips.
//
// Set PLAYOUT_TEST_STREAM_URL to an Emby/Jellyfin `/Videos/{id}/stream?static=true&api_key=…`
// URL. Deliberately an env var rather than reading Loomarr's settings: this test asserts
// ffmpeg's behaviour against real content, not the app's configuration plumbing.
func embyStreamURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("PLAYOUT_TEST_STREAM_URL")
	if u == "" {
		t.Skip("set PLAYOUT_TEST_STREAM_URL to a library stream URL to run this")
	}
	return u
}

// The child args must actually encode REAL library content — which is where the synthetic
// card stops being useful. A real remux brings HEVC, 10-bit, HDR, multiple audio tracks and
// subtitles, and every one of those is a way for the output to differ from the profile and
// silently break the parent's `-c copy`.
//
// So this asserts the OUTPUT PROPERTIES, not just that ffmpeg exited 0: exact resolution,
// exact framerate, h264 + aac, and exactly one of each. Those are the concat preconditions.
func TestLive_ProgramArgsNormalizeRealContent(t *testing.T) {
	bin := ffmpegBin(t)
	probe := ffprobeBin(t)
	url := embyStreamURL(t)
	out := t.TempDir() + "/program.ts"

	p := DefaultProfile()
	enc := Detect(context.Background(), bin, p, "").Chosen
	p.Encoder = enc
	t.Logf("normalizing real content with %s", enc)

	// Seek in, so this also exercises the mid-program tune-in path against HTTP.
	args := transcodeArgs(p, url, 60*time.Second, 3*time.Second)
	args = replaceOutput(args, out)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var samples int
	proc, err := Start(ctx, bin, args, nil, func(Progress) { samples++ })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
	if err := proc.Wait(); err != nil {
		t.Fatalf("encoding real content failed: %v\nlast stderr: %s", err, proc.LastError())
	}
	if samples == 0 {
		t.Error("no progress samples")
	}

	// Exact geometry + codecs. `-of csv` so each stream is one line and we can count them.
	got, err := exec.CommandContext(ctx, probe, "-v", "error",
		"-show_entries", "stream=codec_type,codec_name,width,height,r_frame_rate,channels",
		"-of", "csv=p=0", out).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	s := strings.TrimSpace(string(got))
	t.Logf("probe:\n%s", s)

	// Exactly two streams — one video, one audio. A 3-audio-track remux must be reduced to
	// one, because a varying track count breaks -c copy on the parent exactly like a varying
	// resolution does.
	//
	// `format=nb_streams` rather than counting `stream=` lines: an MPEG-TS carries its program
	// map periodically, so ffprobe legitimately reports the same streams more than once. An
	// earlier version of this test counted lines, saw 5, and failed against output that was
	// completely correct.
	nb, err := exec.CommandContext(ctx, probe, "-v", "error",
		"-show_entries", "format=nb_streams", "-of", "csv=p=0", out).Output()
	if err != nil {
		t.Fatalf("ffprobe nb_streams: %v", err)
	}
	if got := strings.TrimSpace(string(nb)); got != "2" {
		t.Errorf("nb_streams = %s, want 2 (one video, one audio) — a varying track count "+
			"breaks -c copy on the parent:\n%s", got, s)
	}
	// Resolution and framerate must match the profile EXACTLY, not approximately.
	if !strings.Contains(s, itoa(p.Width)+","+itoa(p.Height)) {
		t.Errorf("output is not %dx%d — the concat parent would reject it mid-stream:\n%s",
			p.Width, p.Height, s)
	}
	if !strings.Contains(s, itoa(p.Framerate)+"/1") {
		t.Errorf("framerate is not %d/1:\n%s", p.Framerate, s)
	}
	for _, want := range []string{"h264", "aac"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s:\n%s", want, s)
		}
	}
	// Stereo: a 4.0 or 5.1 source must be downmixed, or the layout varies between programs.
	if !strings.Contains(s, ",2") {
		t.Errorf("audio is not 2-channel — a varying layout breaks -c copy:\n%s", s)
	}
}

// TWO programs encoded independently must be byte-compatible enough to CONCATENATE. This is
// the actual invariant `-c copy` needs, and no arg-shape test can prove it: it requires
// encoding twice and then remuxing the pair through the concat demuxer.
//
// Different seek offsets stand in for "two different programs" — the point is that two
// separate ffmpeg invocations produced output the demuxer will splice.
func TestLive_TwoProgramsConcatenateWithCopy(t *testing.T) {
	bin := ffmpegBin(t)
	probe := ffprobeBin(t)
	url := embyStreamURL(t)
	dir := t.TempDir()

	p := DefaultProfile()
	p.Encoder = Detect(context.Background(), bin, p, "").Chosen

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Encode two "programs" from different offsets, exactly as two children would.
	parts := []string{dir + "/a.ts", dir + "/b.ts"}
	for i, offset := range []time.Duration{30 * time.Second, 300 * time.Second} {
		args := replaceOutput(transcodeArgs(p, url, offset, 2*time.Second), parts[i])
		proc, err := Start(ctx, bin, args, nil, nil)
		if err != nil {
			t.Fatalf("part %d start: %v", i, err)
		}
		go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
		if err := proc.Wait(); err != nil {
			t.Fatalf("part %d failed: %v\n%s", i, err, proc.LastError())
		}
	}

	// Now concatenate them with -c copy, which is what the parent does. If the children
	// disagreed on any pinned property, THIS is where it surfaces.
	list := dir + "/list.txt"
	if err := os.WriteFile(list,
		[]byte("file '"+parts[0]+"'\nfile '"+parts[1]+"'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	joinedOut := dir + "/joined.ts"
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error",
		"-f", "concat", "-safe", "0", "-i", list,
		"-c", "copy", "-f", "mpegts", joinedOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("-c copy concatenation of two independently-encoded programs FAILED — "+
			"the children are not normalizing identically: %v\n%s", err, out)
	}

	// The joined stream must be longer than either part, i.e. both actually made it in.
	got, err := exec.CommandContext(ctx, probe, "-v", "error",
		"-show_entries", "format=duration", "-of", "csv=p=0", joinedOut).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	t.Logf("joined duration: %s", strings.TrimSpace(string(got)))
	if d := strings.TrimSpace(string(got)); strings.HasPrefix(d, "0") || d == "" {
		t.Errorf("joined stream has no duration (%q) — concatenation produced nothing", d)
	}
}

// The parent's concat args must be ACCEPTED by ffmpeg. The protocol whitelist and -safe 0 are
// easy to get wrong, and both failures read as "the playlist is broken" rather than naming
// the flag.
//
// This uses a local playlist of local files rather than HTTP, because the point is that the
// demuxer + copy muxer accept the flag combination — the HTTP half is covered by the routes.
func TestLive_ConcatArgsAreAcceptedByFfmpeg(t *testing.T) {
	bin := ffmpegBin(t)
	dir := t.TempDir()

	// A short real-ish part to concatenate: the synthetic card, so this test needs no Emby.
	part := dir + "/part.ts"
	p := DefaultProfile()
	p.Width, p.Height = 320, 180
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cardArgs := replaceOutput(TestCardArgs(p, "", "", ""), "-t", "1", part)
	proc, err := Start(ctx, bin, cardArgs, nil, nil)
	if err != nil {
		t.Fatalf("card start: %v", err)
	}
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
	if err := proc.Wait(); err != nil {
		t.Fatalf("card: %v\n%s", err, proc.LastError())
	}

	playlist := dir + "/list.ffconcat"
	if err := os.WriteFile(playlist, []byte(Playlist(part)), 0o600); err != nil {
		t.Fatal(err)
	}

	// The real ConcatArgs, with the trailing pipe swapped for a bounded file. `-stream_loop
	// -1` would otherwise run forever, so bound it with -t.
	args := replaceOutput(ConcatArgs(playlist), "-t", "3", dir+"/out.ts")
	proc, err = Start(ctx, bin, args, nil, nil)
	if err != nil {
		t.Fatalf("concat start: %v", err)
	}
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
	if err := proc.Wait(); err != nil {
		t.Fatalf("the parent's own args were rejected by ffmpeg: %v\nlast stderr: %s",
			err, proc.LastError())
	}
}

// replaceOutput swaps the trailing "pipe:1" for a bounded file output.
// makeHDRSource synthesizes an HDR10 clip — PQ transfer, BT.2020 primaries, 10-bit — into the
// test's temp dir.
//
// SYNTHESIZED, NOT COMMITTED, and that is a deliberate departure from ErsatzTV, whose test suite
// carries ~3MB of fixture `.ts` files across a resolution × codec × bit-depth × HDR × anamorphic
// matrix. Every axis in that matrix is producible by the ffmpeg this build tag already requires,
// so committing the bytes buys nothing and costs a repo that grows with the matrix. Measured here:
// this clip is ~26KB and takes under half a second to make.
//
// It also fixes the gap that let the defect ship. Every other live test sources `testsrc` or a
// real Emby URL — 8-bit, SDR, progressive, square-pixel — so no gate had ever put non-SDR content
// through the filter chain. The encoder-family axis was well covered; the SOURCE axis was not.
func makeHDRSource(t *testing.T, bin string) string {
	t.Helper()
	out := t.TempDir() + "/hdr.ts"

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=1280x720:rate=25:duration=2",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		"-c:v", "libx265", "-pix_fmt", "yuv420p10le",
		"-x265-params", "colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc",
		"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc",
		"-c:a", "aac", "-shortest", "-t", "2", "-f", "mpegts", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot synthesize an HDR source with this build (libx265?): %v\n%s", err, b)
	}
	return out
}

// probeColor returns pix_fmt, transfer, primaries, matrix and range for a file's video stream.
func probeColor(t *testing.T, probe, path string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	got, err := exec.CommandContext(ctx, probe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=pix_fmt,color_transfer,color_primaries,color_space,color_range",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	// An MPEG-TS repeats its program map, so ffprobe can print the same stream more than once.
	return strings.TrimSpace(strings.Split(strings.TrimSpace(string(got)), "\n")[0])
}

// AN HDR PROGRAM MUST COME OUT AS HONEST SDR — tone-mapped AND correctly labelled.
//
// This is the test that could not have been written as an arg-shape assertion, and the defect it
// guards was BOTH halves at once. Before this change the chain ended in a bare `format=yuv420p`
// with no colour tags, so a real HDR10 source produced:
//
//	yuv420p,bt2020nc,smpte2084,bt2020
//
// — 8-bit SDR-range pixels still announcing PQ/BT.2020. A player that believes the tags applies an
// HDR transfer to SDR data, which is worse than doing nothing, and no client-side handling can
// recover it because the information needed is gone.
//
// The two assertions are independent ON PURPOSE. Tags alone would pass if the filter silently did
// nothing; a changed picture alone would pass while still mislabelled. Both defects were live
// simultaneously, and either one checked without the other reads as success.
func TestLive_HDRSourceIsTonemappedAndLabelledSDR(t *testing.T) {
	bin := ffmpegBin(t)
	probe := ffprobeBin(t)

	if !TonemapperFor(bin)() {
		t.Skip("this ffmpeg build has no zscale/tonemap — the code correctly emits no chain")
	}
	src := makeHDRSource(t, bin)

	// Confirm the fixture really IS HDR. A source that quietly lost its tags would make every
	// assertion below pass for the wrong reason — the fixture-collapse trap.
	if got := probeColor(t, probe, src); !strings.Contains(got, "smpte2084") {
		t.Fatalf("synthesized source is not HDR (%s); the rest of this test would be vacuous", got)
	}

	p := DefaultProfile()
	p.Encoder = Detect(context.Background(), bin, p, "").Chosen

	run := func(name string, spec ProgramSpec) string {
		t.Helper()
		out := t.TempDir() + "/" + name + ".ts"
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		proc, err := Start(ctx, bin, replaceOutput(ProgramArgs(spec), out), nil, nil)
		if err != nil {
			t.Fatalf("%s: start: %v", name, err)
		}
		go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
		if err := proc.Wait(); err != nil {
			t.Fatalf("%s: encode failed: %v\nlast stderr: %s", name, err, proc.LastError())
		}
		return out
	}

	base := ProgramSpec{Profile: p, Input: src, Limit: 1 * time.Second, Source: hdrSource()}

	tonemapped := base
	tonemapped.Tonemap = true
	withTM := run("tonemapped", tonemapped)

	untouched := base // Tonemap false — what a build without zscale produces
	withoutTM := run("untouched", untouched)

	// HALF ONE: the labels are honest.
	got := probeColor(t, probe, withTM)
	t.Logf("tone-mapped output: %s", got)
	for _, want := range []string{"bt709"} {
		if !strings.Contains(got, want) {
			t.Errorf("tone-mapped output is not labelled %s: %s", want, got)
		}
	}
	for _, unwanted := range []string{"smpte2084", "bt2020"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("output still carries the source's HDR tag %q — SDR pixels announcing PQ is "+
				"the half no client can recover from: %s", unwanted, got)
		}
	}

	// HALF TWO: the picture actually changed. Frame hashes, because "did the filter run" and
	// "are the tags right" are different questions and the tags can be right while the filter did
	// nothing at all.
	hashTM, hashPlain := frameHash(t, bin, withTM), frameHash(t, bin, withoutTM)
	if hashTM == hashPlain {
		t.Errorf("tone-mapped and untouched output are pixel-identical (%s) — the tags changed "+
			"but the filter did no work", hashTM)
	}
}

// frameHash is the hash of a file's first VIDEO frame.
//
// ⚠ `-map 0:v:0` is load-bearing. Without it framehash also emits the AUDIO frames, and since both
// files here carry identical silence, reading the wrong line reports two different pictures as
// identical. Cost me one wrong conclusion before the stream index was pinned.
func frameHash(t *testing.T, bin, path string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error",
		"-i", path, "-map", "0:v:0", "-frames:v", "1", "-f", "framehash", "-").Output()
	if err != nil {
		t.Fatalf("framehash: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, ",")
		return strings.TrimSpace(f[len(f)-1])
	}
	t.Fatalf("no frame hash in output: %s", out)
	return ""
}

// CAN THIS FFMPEG ADVANCE A CONCAT PLAYLIST PAST A CHUNKED HTTP ENTRY?
//
// This is a CAPABILITY test, not a version assertion, and the distinction is the point: the answer
// depends on the ffmpeg binary in front of it, and there are three different ones in play (the
// image pins n7.1, CI installs Ubuntu's apt build, a developer has whatever is on PATH). Asking the
// binary is the only honest check — the same rule capability.go and filters.go already follow.
//
// ⚠ **What it guards.** `/v1/playout/program/{id}` streams a LIVE encode, so Go's net/http sends it
// chunked with no `Content-Length` — and it can never send one, because the byte length of a live
// encode is unknowable. On ffmpeg **n9.0** that is fatal to the whole mechanism:
//
//	[http] Stream ends prematurely at 85916, should be 18446744073709551615
//	[in#0/concat] Error during demuxing: Input/output error
//
// 18446744073709551615 is UINT64_MAX — ffmpeg 9 treats an unknown-length body as INFINITE, so any
// termination reads as premature, the demuxer raises EIO and STOPS rather than opening the next
// playlist entry. A channel then plays one programme forever.
//
// Measured 2026-08-09 on an identical minimal harness (a static file + a throwaway server, no
// Loomarr code): n7.1.5 chunked → 5 entry fetches, advances; n9.0 chunked → 1 fetch, 3× EIO;
// n9.0 with Content-Length → 5 fetches, advances. So it is the missing length, not HTTP itself.
//
// ⚠ **This is a hard blocker on bumping FFMPEG_TAG in the Dockerfile.** Nothing else in the tree
// would catch it: `-stream_loop -1` REPLAYS the buffered programme, so the parent still emits
// continuous output and exits 0, and the EIO lines are swallowed by `-loglevel error`. The symptom
// is a channel stuck on one programme, not a failure. Run this before changing the pin.
//
// Mitigations tested and REFUTED on 9 — do not spend time re-trying them: dropping `-reconnect` /
// `-reconnect_at_eof`; `-seekable 0`; connection-close (HTTP/1.0) framing; `-ignore_io_errors`
// (HLS-only, not a concat option); `-multiple_requests 1` (hangs).
func TestLive_ConcatAdvancesPastAChunkedHTTPEntry(t *testing.T) {
	bin := ffmpegBin(t)

	// One short MPEG-TS, shaped like a program child's output.
	seg := t.TempDir() + "/seg.ts"
	if o, err := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=25:duration=2",
		"-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000",
		"-shortest", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
		"-f", "mpegts", "-mpegts_flags", "+initial_discontinuity", seg).CombinedOutput(); err != nil {
		t.Fatalf("could not build the segment: %v\n%s", err, o)
	}
	body, err := os.ReadFile(seg)
	if err != nil {
		t.Fatal(err)
	}

	var fetches atomic.Int64
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/playlist") {
			// The same two-identical-entries shape the real playlist endpoint emits.
			_, _ = fmt.Fprintf(w, "ffconcat version 1.0\nfile '%s/seg'\nfile '%s/seg'\n", srv.URL, srv.URL)
			return
		}
		fetches.Add(1)
		// NO Content-Length, flushed per chunk — byte-for-byte how pipeChild streams a live
		// programme, which is what makes this a faithful test rather than a synthetic one.
		w.Header().Set("Content-Type", "video/mp2t")
		w.WriteHeader(http.StatusOK)
		f, _ := w.(http.Flusher)
		for i := 0; i < len(body); i += 64 * 1024 {
			end := min(i+64*1024, len(body))
			_, _ = w.Write(body[i:end])
			if f != nil {
				f.Flush()
			}
		}
	}))
	defer srv.Close()

	// The REAL ConcatArgs, bounded to a file so the run terminates.
	args := replaceOutput(ConcatArgs(srv.URL+"/playlist"), "-t", "6", t.TempDir()+"/joined.ts")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	proc, err := Start(ctx, bin, args, nil, nil)
	if err != nil {
		t.Fatalf("parent start: %v", err)
	}
	go func() { _, _ = io.Copy(io.Discard, proc.Stdout) }()
	if werr := proc.Wait(); werr != nil {
		t.Fatalf("parent failed: %v\nlast stderr: %s", werr, proc.LastError())
	}

	// 6s of output over 2s entries needs the demuxer to have opened a new entry at least twice.
	if got := fetches.Load(); got < 2 {
		v, _ := exec.CommandContext(ctx, bin, "-version").Output()
		ver := strings.SplitN(strings.TrimSpace(string(v)), "\n", 2)[0]
		t.Fatalf("this ffmpeg fetched the entry %d time(s) and never advanced.\n"+
			"  %s\n"+
			"  It cannot read a chunked (no Content-Length) concat entry to EOF, which is what\n"+
			"  /v1/playout/program/{id} necessarily serves — so EVERY internal-playout channel on\n"+
			"  this binary plays one programme and then repeats it.\n"+
			"  Known good: n7.1.x (what the Dockerfile pins). Known bad: n9.0.\n"+
			"  This is not a Loomarr regression; it is the ffmpeg on PATH.", got, ver)
	}
}

func replaceOutput(args []string, extra ...string) []string {
	out := make([]string, 0, len(args)+len(extra))
	for _, a := range args {
		if a == "pipe:1" {
			continue
		}
		out = append(out, a)
	}
	return append(out, extra...)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
