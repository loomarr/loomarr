//go:build ffmpeg

// Tests that EXECUTE ffmpeg. Behind a build tag because unit tests must not depend on an
// external binary being present — the same reasoning CLAUDE.md applies to the network. Run
// with `make test-ffmpeg`.
//
// These exist because arg-shape tests assert my own stated invariants and cannot catch an
// invariant that is wrong. "Every rung has even dimensions" is checkable in Go; "every rung
// actually encodes" is only checkable by encoding.

package playout

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
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
	enc := Detect(context.Background(), bin, DefaultProfile()).Chosen
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
	c := Detect(context.Background(), bin, DefaultProfile())

	if c.Chosen == "" {
		t.Fatal("Detect returned no encoder — software is always a valid answer")
	}
	if c.MaxChannels < 1 {
		t.Errorf("MaxChannels = %d, want at least 1", c.MaxChannels)
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
	c := Detect(context.Background(), ffmpegBin(t), DefaultProfile())
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
	enc := Detect(context.Background(), bin, p).Chosen
	p.Encoder = enc
	t.Logf("normalizing real content with %s", enc)

	// Seek in, so this also exercises the mid-program tune-in path against HTTP.
	args := ProgramArgs(p, url, 60*time.Second, 3*time.Second)
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
	p.Encoder = Detect(context.Background(), bin, p).Chosen

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Encode two "programs" from different offsets, exactly as two children would.
	parts := []string{dir + "/a.ts", dir + "/b.ts"}
	for i, offset := range []time.Duration{30 * time.Second, 300 * time.Second} {
		args := replaceOutput(ProgramArgs(p, url, offset, 2*time.Second), parts[i])
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
