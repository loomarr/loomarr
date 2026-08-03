package playout

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Per-program encode args — the child half of the concat mechanism (§9.1, prior-art §1).
//
// THE CORRECTNESS BURDEN LIVES HERE. The parent process concatenates these children with
// `-c copy`, which is only legal if every child produced byte-compatible output: same
// resolution, same framerate, same codec, same pixel format, same track count. A child that
// quietly differs produces a stream players reject partway through — and the symptom (a
// channel that dies a few minutes in) points nowhere near the cause, because the child that
// caused it exited cleanly and long ago.
//
// So every knob that could vary between programs is pinned by the Profile, and the things a
// real library item can spring on us — multiple audio tracks, subtitles, HDR, 10-bit, an
// unusual framerate — are normalized away rather than passed through.
//
// Every flag below is either verified in Tunarr's source or executed against the live dev
// Emby (prior-art §5a–§5c). None of it is guessed; the ones that look redundant are the ones
// that took a real failure to find.

// readrateInitialBurst is how many seconds of content ffmpeg may read flat-out before
// settling to realtime pacing.
//
// This is the TUNE-IN LATENCY FIX (prior-art §5a, Tunarr's ReadrateInputOption). Realtime
// pacing alone is correct but feels broken: a player joining a live stream has an empty
// buffer and must wait for it to fill at 1.0x before showing anything. The burst fills it
// immediately, then pacing settles down so we do not race ahead of wall-clock.
//
// Deliberately NOT applied to synthetic sources — pacing a lavfi generator with a burst
// stalls the pipeline (see TestCardArgs, which uses plain `-re`).
const readrateInitialBurst = 10

// ProgramArgs builds the args to encode ONE program, starting `offset` in, for `limit`.
//
// This is what the "what's on now" endpoint spawns per program. It streams finite MPEG-TS to
// stdout and then EXITS — that EOF is the sequencing signal the parent's concat demuxer acts
// on (prior-art §1). Nothing here loops; a child that never ended would pin the channel to
// one program forever.
//
// Audio track 0 — the file's first — which is the historical behaviour and correct only when
// the caller has no language preference to apply. ProgramArgsWithAudio is what the resolver
// uses; see audio.go for why the choice cannot be expressed in ffmpeg's args alone.
func ProgramArgs(p Profile, streamURL string, offset, limit time.Duration) []string {
	return ProgramArgsWithAudio(p, streamURL, offset, limit, 0)
}

// ProgramArgsWithAudio is ProgramArgs with an explicit audio track index (§9.1).
//
// `audioTrack` is an index among AUDIO streams — the `N` in `-map 0:a:N` — as returned by
// PickAudioTrack. Out-of-range values are not clamped here: ffprobe and ffmpeg enumerate the
// same file the same way, so a bad index means the two disagreed, and failing loudly at encode
// beats silently playing a track the operator did not ask for.
func ProgramArgsWithAudio(
	p Profile, streamURL string, offset, limit time.Duration, audioTrack int,
) []string {
	return ProgramArgsNormalised(p, streamURL, offset, limit, audioTrack, "")
}

// ProgramArgsNormalised is ProgramArgsWithAudio with an optional loudness target (§10 V40).
//
// ⚠ **Only FILLER passes a target.** The caller decides, because only the caller knows what it is
// playing: `Airing.Source` is set for a resolved filler clip and empty for a library title, which
// is the discriminator — no new plumbing needed. Normalising a feature film to advert loudness
// would flatten its dynamic range, and the problem being solved (adverts recorded a decade apart
// at wildly different levels — a measured 11 dB spread) is a filler problem.
//
// `targetLUFS` empty ⇒ byte-identical to what shipped before V40, which is what keeps every
// existing caller and every existing test meaning what it meant.
func ProgramArgsNormalised(
	p Profile, streamURL string, offset, limit time.Duration, audioTrack int, targetLUFS string,
) []string {
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-progress", progressPipeArg(), "-nostats",
	}

	// HARDWARE DEVICE SETUP, and it must come before everything — it is a global option, and
	// placed after `-i` it silently applies to nothing.
	//
	// Reused from the capability prober rather than reimplemented. Writing a second version
	// of this reproduced a bug the prober had already found and fixed: without the device,
	// `hwupload` fails with "A hardware device reference is required to upload frames to",
	// which reads like a filter bug and sends you looking in the wrong place. The prober's
	// version is also per-encoder correct in ways a generic one is not (QSV needs an explicit
	// render node; Vulkan names its device differently).
	args = append(args, deviceInitArgs(p.Encoder)...)

	// HARDWARE DECODE. Measured on a 4K 10-bit HEVC film with an RTX 3080 Ti: the child went
	// from 341% CPU to ~0%, with the GPU decoder taking it instead.
	//
	// That number is the whole justification. Moving only the ENCODE to the GPU barely helped —
	// CPU actually ROSE from 260% to 341%, because the decode was the real cost and it stopped
	// being throttled by a slow software encoder. For 4K sources the decode dominates.
	args = append(args, hardwareDecodeArgs(p.Encoder)...)

	// --- Input options (before -i, so they apply to THIS input) ---

	// Reconnect flags, CHILD tier — and ONLY for an http input. See isHTTP.
	//
	// A child fetching a program from an HTTP media source should survive a network blip
	// mid-program rather than killing the slot. These must NOT include `-reconnect_at_eof`,
	// which belongs to the parent, where a child's EOF is the advance signal; on a child it
	// means the child tries to continue past the end of its own program, presenting as an
	// intermittent stall (prior-art §5a: "the two tiers must not get each other's flags").
	if isHTTP(streamURL) {
		args = append(args,
			"-reconnect", "1",
			"-reconnect_on_network_error", "1",
			"-reconnect_streamed", "1",
			"-multiple_requests", "1",
		)
	}

	// Realtime pacing with a burst. See readrateInitialBurst.
	args = append(args,
		"-readrate", "1.0",
		"-readrate_initial_burst", strconv.Itoa(readrateInitialBurst),
	)

	// THE SEEK, and its placement is load-bearing. `-ss` BEFORE `-i` makes ffmpeg seek —
	// over HTTP the server serves a byte range, verified at 2.9s wall-clock for a 40-minute
	// offset into a 4K remux (prior-art §5c). After `-i` it would decode and DISCARD from
	// the start of the file, which for the same offset takes minutes and burns a core
	// producing nothing.
	//
	// Sub-second precision is deliberate: a channel is a wall clock, and rounding every
	// tune-in to whole seconds would accumulate drift across a cycle.
	if offset > 0 {
		args = append(args, "-ss", seconds(offset))
	}

	args = append(args, "-i", streamURL)

	// --- Output options ---

	// EXPLICIT TRACK SELECTION, and it is mandatory rather than tidy (prior-art §5b). The
	// verified test item carried THREE audio tracks (dts, flac, ac3) plus subtitles. Without
	// maps, ffmpeg's default selection picks by its own heuristics — so the track count can
	// differ between programs, and a varying track count breaks the parent's `-c copy`
	// exactly like a varying resolution does.
	//
	// First video, ONE audio, nothing else. Subtitles are dropped: burning them in would
	// vary per item and there is no subtitle track in a normalized MPEG-TS profile.
	//
	// Which audio is chosen by the caller (see audio.go). It used to be hardcoded `0:a:0` —
	// the first track in the file — which is how a channel ended up playing a film in Russian:
	// the release simply carried its Russian dub first. Exactly one audio track either way,
	// because a varying track count breaks `-c copy` as surely as a varying resolution.
	args = append(args, "-map", "0:v:0", "-map", "0:a:"+strconv.Itoa(audioTrack))

	// `-t` bounds the child to its slot. This is what makes the child exit at the program
	// boundary rather than playing to the end of the file — which matters when the lineup
	// gives an item less time than its full duration (a rolling window, or a slot the
	// scheduler trimmed).
	if limit > 0 {
		args = append(args, "-t", seconds(limit))
	}

	args = append(args, p.scaleFilterArgs()...)
	args = append(args, p.videoEncodeArgs()...)
	args = append(args, p.audioEncodeArgsNormalised(targetLUFS)...)

	// `+initial_discontinuity` tells the downstream demuxer the first timestamps are not
	// necessarily zero — true for anything joining a live stream mid-flight, and true here
	// because we seeked.
	return append(args,
		"-f", "mpegts", "-mpegts_flags", "+initial_discontinuity", "pipe:1",
	)
}

// scaleFilterArgs normalizes any input geometry to the profile's.
//
// A REAL filter-graph failure, and exactly the class ErsatzTV warns about (prior-art §5b):
// `-vf scale=1280:720` straight into `h264_vulkan` fails with "Impossible to convert between
// the formats supported by the filter 'Parsed_scale_0' and the filter 'auto_scale_0'" and a
// 40-line pixel-format dump. The cause is not the scale — it is that `scale` emits CPU
// frames while a hardware encoder wants GPU frames, and nothing uploads between them.
//
// So hardware gets `scale=W:H,format=nv12,hwupload`: scale on the CPU, convert to a format
// the uploader accepts, THEN upload. Software needs no upload step and gets the bare scale
// plus an explicit pixel format.
//
// `force_original_aspect_ratio=decrease` + `pad` letterboxes rather than stretching, so a
// 4:3 episode in a 16:9 profile keeps its geometry. The pad is what preserves the profile's
// exact output dimensions, which `-c copy` requires — a bare aspect-preserving scale would
// emit 960x720 for 4:3 content and break concatenation.
func (p Profile) scaleFilterArgs() []string {
	if p.Width <= 0 || p.Height <= 0 {
		return nil
	}
	scale := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
		p.Width, p.Height, p.Width, p.Height)

	// Framerate is pinned too: a 24fps film and a 25fps episode must not produce different
	// output rates, or `-c copy` on the parent is invalid.
	fps := fmt.Sprintf("fps=%d", p.Framerate)

	parts := []string{scale, fps}

	// The upload step comes from the capability prober's own helper, not a local copy. It is
	// per-encoder correct in ways a generic "format=nv12,hwupload" is not — QSV needs
	// `extra_hw_frames=64` or its lookahead intermittently fails to allocate frames, and the
	// families that accept CPU frames directly (nvenc, amf, videotoolbox, rkmpp, v4l2m2m)
	// must get NO upload at all. A hand-rolled version here got QSV wrong and drifted from
	// the prober within one commit of being written.
	if up := hardwareUploadFilter(p.Encoder); up != "" {
		// These families upload to GPU memory, and their upload filter already pins the
		// pixel format (nv12) on the way.
		parts = append(parts, up)
	} else {
		// EVERY OTHER FAMILY gets an explicit 8-bit pixel format — software AND the hardware
		// encoders that take CPU frames directly (nvenc, amf, videotoolbox, rkmpp, v4l2m2m).
		//
		// This `else` used to be `else if p.Encoder == EncoderSoftware`, which left exactly
		// those hardware families with NO pixel-format normalization. A 10-bit source then
		// reached the encoder as yuv420p10le, and h264_nvenc — which encodes 8-bit H.264 only
		// — rejected it:
		//
		//	[h264_nvenc] No capable devices found
		//	[out#0/mpegts] Nothing was written into output file
		//
		// That message names the DEVICE, not the pixel format, so it reads as "your GPU is
		// missing" while the GPU is fine. Found on a live channel: a 4K 10-bit HEVC film
		// played on libx264 (which had this filter) and died the moment nvenc was selected.
		//
		// Every prior live test used software, so every prior live test had the fix.
		parts = append(parts, "format=yuv420p")
	}
	return []string{"-vf", strings.Join(parts, ",")}
}

// ConcatArgs builds the PARENT process: one long-lived `-c copy` ffmpeg that reads an HTTP
// ffconcat playlist and never exits (prior-art §1).
//
// It re-muxes and never re-encodes. All the encoding happens in the children, which is what
// makes one channel cost one encode regardless of how many programs it plays — and why the
// children's output must be byte-compatible.
func ConcatArgs(playlistURL string) []string {
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-progress", progressPipeArg(), "-nostats",
	}

	// Reconnect flags, PARENT tier — and ONLY for an http playlist. See isHTTP.
	//
	// `-reconnect_at_eof 1` is THE MECHANISM, not a resilience nicety: it makes a child
	// program's EOF non-fatal so the demuxer advances to the next playlist entry instead of
	// ending the channel. Without it the channel plays exactly one program and stops.
	if isHTTP(playlistURL) {
		args = append(args, "-reconnect", "1", "-reconnect_at_eof", "1")
	}

	return append(args,
		// The concat demuxer must be told which protocols its ENTRIES may use, or it
		// refuses to open an http:// entry — a playlist of local files is its default
		// assumption. This is about the entries, not the playlist itself, so it is
		// unconditional.
		"-protocol_whitelist", "file,http,https,tcp,tls,pipe",
		// Both playlist entries are the same URL; looping forever is what makes the channel
		// continuous (prior-art §1).
		"-stream_loop", "-1",
		"-f", "concat",
		// Entries are absolute URLs, so the demuxer's relative-path safety check must be
		// off or it rejects them.
		"-safe", "0",
		"-i", playlistURL,

		// COPY, never encode. See above.
		"-c", "copy",
		"-f", "mpegts", "-mpegts_flags", "+initial_discontinuity", "pipe:1",
	)
}

// isHTTP reports whether a URL uses a protocol that accepts ffmpeg's `-reconnect*` options.
//
// THIS CONDITION IS LOAD-BEARING, and omitting it is a hard failure rather than a missed
// optimization. `-reconnect*` are private options of ffmpeg's HTTP protocol, not global ones:
// against a local file input, `-reconnect 1` produces "Option reconnect not found" and
// ffmpeg exits 8 before opening anything. Tunarr applies them conditionally on
// `protocol === 'http'` for exactly this reason (prior-art §5a).
//
// It matters in production, not just in tests: filler clips are local files (§10 FILLER_DIR),
// so an unconditional flag list means every commercial break fails to start — as a channel
// that dies at the first break, with a message that names an option rather than a file.
func isHTTP(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// Playlist renders the two-line ffconcat playlist the parent reads (prior-art §1).
//
// Two identical entries, not one, and this is the trick that makes the whole mechanism work.
// The concat demuxer needs at least two entries to have something to ADVANCE TO when the
// first hits EOF; combined with `-stream_loop -1` on the parent it cycles between them
// forever. Each open of the URL asks "what is airing right now?" and gets the current
// program — so looping over two static lines yields an unbounded sequence of different
// programs.
//
// A one-line playlist would EOF the demuxer at the end of the first program and end the
// channel, which is the bug this shape exists to avoid.
func Playlist(programURL string) string {
	return "ffconcat version 1.0\n" +
		"file '" + escapePlaylistURL(programURL) + "'\n" +
		"file '" + escapePlaylistURL(programURL) + "'\n"
}

// escapePlaylistURL neutralises the quote that would terminate a `file '…'` directive.
//
// The URL carries a token (§11 device auth) and channel id, so it is not arbitrary operator
// text — but it is assembled from configuration (`server.public_url`) and a channel id, and
// a stray apostrophe there would produce a playlist that parses as something else entirely
// rather than failing cleanly.
func escapePlaylistURL(s string) string {
	// ffconcat's escape for a quote inside a single-quoted string is to close, escape, and
	// reopen: it's → 'it'\''s'. Newlines are dropped rather than escaped — there is no valid
	// escape for them in a line-oriented directive.
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.ReplaceAll(s, "'", `'\''`)
}

// seconds formats a duration for ffmpeg's -ss / -t, keeping millisecond precision.
//
// Not strconv on a float: %g would emit exponent notation for small values ("1e-06") which
// ffmpeg parses as a filename-ish token, and %f would pad to six decimals. Milliseconds are
// as fine as a seek is meaningfully accurate.
func seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}
