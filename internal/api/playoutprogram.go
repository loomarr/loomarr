package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/store"
)

// GET /playout/program/{id} — "what is airing right now?" (§9.1, prior-art §1).
//
// THIS IS THE SEQUENCING LAYER, and it is worth being precise about why it looks so
// unremarkable. The parent ffmpeg reads a two-line ffconcat playlist whose entries both point
// here. Each time the concat demuxer opens this URL it gets a FINITE MPEG-TS stream of the one
// program currently on; when that program ends the child exits, the demuxer sees EOF, advances
// to the next (identical) entry, re-requests, and gets the NEXT program.
//
// So there is no splicing code, no scheduler loop, no "advance to the next item" state machine.
// The demuxer's EOF-and-advance IS the program boundary, and this handler is the whole of it.
//
// It also means this handler is called REPEATEDLY for one channel — once per program, forever.
// It must therefore be cheap, idempotent, and above all CONSISTENT: two calls at the same
// instant must resolve to the same program at the same offset, or a channel replaying a program
// it already showed would be indistinguishable from a scheduling bug.

// PlayoutResolver answers "what should this channel play right now" and turns it into an ffmpeg
// input. Implemented by an adapter over channels.Engine + library.Client; abstracted so the api
// package need not import either.
type PlayoutResolver interface {
	// AiringNow resolves the channel's current program and the URL ffmpeg should read.
	//
	// The returned Airing carries the seek offset and the remaining duration, which together
	// make a mid-program tune-in land in the right place. An Airing that is not Playable means
	// "nothing is airing" — an empty lineup, or one where nothing has landed yet — which the
	// caller renders as the offline card rather than as an error.
	AiringNow(ctx context.Context, channelID string) (playout.Airing, string, error)
	// Profile is the encode profile to normalize this program to, resolved against measured
	// capacity and current load (§9.1 quality ladder).
	Profile(ctx context.Context) playout.Profile
	// AudioTrackFor picks which audio track to play from a source — the `N` in `-map 0:a:N`,
	// honouring the operator's preferred language (§9.1). Best-effort: 0 (the file's first
	// track) whenever the preference cannot be resolved, so a probe failure costs the language
	// and never the programme.
	AudioTrackFor(ctx context.Context, streamURL string) int
	// Tracks probes the audio + subtitle tracks of the channel's CURRENTLY-AIRING program, for
	// the Watch surface's pickers (§9.1, V46). The options a viewer sees are what the airing file
	// actually carries — not a hardcoded list and not an app setting. Best-effort: empty tracks
	// when nothing is airing or the probe fails, so a UI request never blocks on a bad probe.
	Tracks(ctx context.Context, channelID string) (playout.MediaTracks, error)
	// PlanFor decides the copy/transcode plan for an input against a target (§9.1 direct play,
	// V47): copy the streams the target can play, transcode the rest. Fails safe toward transcode
	// (the zero CopyPlan) when the source cannot be probed.
	PlanFor(ctx context.Context, input string, target playout.Target) playout.CopyPlan
}

// PlayoutEncoder starts a supervised ffmpeg for the given args. Injected so the handlers can be
// tested without executing a binary; the composition root supplies playout.Start.
//
// `onProgress` carries ffmpeg's structured progress back (V16 telemetry). It was always
// available — `playout.Start` has taken this callback since the process supervisor was written
// — and every caller passed nil, so each sample was parsed and discarded. Threading it through
// here is what puts real encoder speed on the dashboard instead of a plausible-looking constant.
type PlayoutEncoder func(ctx context.Context, args []string, onProgress func(playout.Progress)) (*playout.Process, error)

// programHandler streams ONE program as finite MPEG-TS, then exits.
//
// "Then exits" is the contract, not an implementation detail: the child's EOF is what advances
// the channel. A handler that looped, or that held the connection open after the program ended,
// would pin the channel to one program forever.
func (s *Server) programHandler(w http.ResponseWriter, r *http.Request) {
	if !s.authorizePlayout(w, r) {
		return
	}
	if s.playoutResolver == nil || s.playoutEncoder == nil {
		s.writeProblem(w, r, http.StatusNotImplemented, "Playout unavailable",
			"Internal playout isn't running on this instance.")
		return
	}
	channelID := r.PathValue("id")
	if channelID == "" {
		http.NotFound(w, r)
		return
	}

	// The codec audience this program is for (§9.1 V47) — set on the URL by the session parent that
	// requested it, so a browser session's programs plan for the browser and a tuner session's for
	// the media server. It drives both the copy plan below and which session gets this child's
	// telemetry. Parsed once here so the offline-card path (which also spawns a child) carries it too.
	target := playout.ParseTarget(r.URL.Query().Get(playoutTargetParam))

	airing, streamURL, err := s.playoutResolver.AiringNow(r.Context(), channelID)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		s.log.Warn("playout: could not resolve what is airing", "channel", channelID, "err", err)
		// 502, not 500: the usual cause is the media server being unreachable, which is
		// upstream of us. It also matters that this is RETRYABLE — the demuxer will
		// re-request, so a transient library outage heals itself.
		s.writeProblem(w, r, http.StatusBadGateway, "Couldn't work out what's on",
			"Loomarr couldn't resolve this channel's current program.")
		return
	}

	profile := s.playoutResolver.Profile(r.Context())

	// Nothing airing ⇒ the offline card, NOT an error and NOT an empty body.
	//
	// An empty 200 would make the demuxer EOF instantly and re-request in a tight loop,
	// spinning a core on a channel with no content. The card is a real encode occupying real
	// time, so the loop paces itself — and the viewer sees "nothing scheduled" rather than a
	// channel that fails to tune.
	if !airing.Playable() || streamURL == "" {
		// The font comes from the composition root, not playout.FindFont(), because the
		// question is not "does this host have a font file?" but "can this ffmpeg BUILD draw
		// one?" — a build without drawtext dies at graph-init and takes the channel with it.
		// See playout.CardFontFor. Nil-guarded: "" is an unlabelled card, a valid rendering.
		font := ""
		if s.playoutFont != nil {
			font = s.playoutFont()
		}
		// The card is a synthetic lavfi source, so it does not contend for VRAM the way a decode+
		// transcode does — but it still uses the profile's encoder, so it gets the SOFTWARE fallback
		// (not the VRAM eviction step): a card that cannot hardware-encode should still render rather
		// than leave the channel with no offline frame either.
		card := func(enc playout.Encoder) []string {
			p := profile
			p.Encoder = enc
			return playout.OfflineCardArgs(p, font, "Nothing scheduled", channelID, offlineCardDuration)
		}
		if c := s.startChild(r.Context(), channelID, target, profile.Encoder, card(profile.Encoder)); c != nil {
			s.pipeChild(w, r, channelID, "offline card", c)
			return
		}
		if profile.Encoder != playout.EncoderSoftware {
			if c := s.startChild(r.Context(), channelID, target, playout.EncoderSoftware, card(playout.EncoderSoftware)); c != nil {
				s.pipeChild(w, r, channelID, "offline card", c)
				return
			}
		}
		s.failProgram(w, r, channelID, "offline card", "the offline card could not be rendered")
		return
	}

	// Which audio track, honouring the operator's language preference (§9.1). Resolved here
	// rather than inside ProgramArgs because it needs a probe of the source, and the args
	// builder is deliberately a pure function.
	audioTrack := s.playoutResolver.AudioTrackFor(r.Context(), streamURL)

	// Loudness normalisation, FILLER ONLY (§10 V40).
	//
	// ⚠ `airing.Source` is the discriminator, and it needs no new plumbing: it is set for a
	// resolved filler clip and empty for a library title (see Airing.Source). Normalising a
	// feature film to advert loudness would flatten its dynamic range — the problem this solves is
	// adverts recorded a decade apart at wildly different levels, measured at an 11 dB spread
	// across real fetched clips.
	//
	// ⚠ Read LIVE rather than captured at wiring, so `filler.target_lufs` hot-applies like every
	// other setting (config-design §3). Empty (or no liveConfig, as in unit tests that build a
	// bare Server) ⇒ no filter, byte-identical to what shipped before V40.
	targetLUFS := ""
	if airing.Source != "" && s.liveConfig != nil {
		targetLUFS = s.liveConfig("filler.target_lufs")
	}

	// The copy/transcode plan (§9.1 direct play, V47): copy the video when the target can play it
	// (the common case — instant, no GPU), transcode only what it cannot. The TARGET comes from the
	// request (`?target=`), because a channel's stream is not one thing — a media-server tuner and a
	// browser have different codec tolerance, so each attaches its own session with its own target,
	// and this handler feeds that session's parent. ParseTarget defaults to the media server (the
	// broader set, and the historical behaviour), so an un-parameterised request is unchanged.
	plan := s.playoutResolver.PlanFor(r.Context(), streamURL, target)

	spec := playout.ProgramSpec{
		Profile:    profile,
		Input:      streamURL,
		Offset:     airing.Offset,
		Limit:      airing.Remaining,
		AudioTrack: audioTrack,
		TargetLUFS: targetLUFS,
		Plan:       plan,
	}
	// The retry ladder (§9.1 V47) lives in streamChild: it runs the hardware encode, and only if it
	// produces NO output does it reclaim VRAM + retry, then fall back to software. Passing the spec
	// (not pre-built args) is what lets the ladder rebuild the SAME program with a software encoder.
	// A copy plan (no video transcode) has no ladder — a copy that produces nothing is a bad file.
	s.streamProgram(w, r, channelID, target, airing.Title, spec)
}

// offlineCardDuration is how long one offline-card request lasts before the demuxer re-asks.
//
// Short enough that a channel starts playing promptly once content lands (the operator approves
// a title, an acquisition completes, the next re-request finds it), long enough that an empty
// channel is not re-encoding every second.
const offlineCardDuration = 30 * time.Second

// streamProgram runs ONE program down the retry ladder (§9.1 V47), then streams it to the response.
//
// The ladder exists because a hardware encode can fail SILENTLY — most often the shared GPU is out
// of VRAM (the suggester's model is resident, §8.2), where the encoder cannot allocate its device
// context and emits zero frames with no error. A silent zero-byte encode is a black channel. So:
//
//  1. Try the spec as-is (hardware, if that is what the Profile resolved).
//  2. Produced nothing AND this is a video transcode? Reclaim VRAM (evict the LLM) and retry the
//     SAME encoder — the freed VRAM is often all it needed.
//  3. Still nothing? Rebuild the spec with the software encoder (libx264) and run that — slower, but
//     the channel PLAYS instead of going black. Software is the floor.
//
// A `-c copy` program is NOT laddered: a copy that produces nothing is a bad source file, which no
// encoder change fixes — so it runs once and fails through (attempt still surfaces the stderr).
//
// The ladder is only possible because startChild PEEKS the first chunk before committing the HTTP
// response: with nothing yet written, a retry is free. Once bytes are flowing, there is no retry —
// a mid-stream death is a normal program boundary, handled by the demuxer re-requesting.
func (s *Server) streamProgram(
	w http.ResponseWriter, r *http.Request, channelID string, target playout.Target, what string, spec playout.ProgramSpec,
) {
	// Attempt 1: as resolved (hardware when available).
	if c := s.startChild(r.Context(), channelID, target, spec.Profile.Encoder, playout.ProgramArgs(spec)); c != nil {
		s.pipeChild(w, r, channelID, what, c)
		return
	}

	// Only a VIDEO TRANSCODE can be rescued by the ladder — a copy failing is a bad file.
	transcoding := !spec.Plan.CopyVideo
	if !transcoding {
		s.failProgram(w, r, channelID, what, "the source could not be read")
		return
	}

	// Attempt 2: reclaim VRAM (evict the resident LLM), then retry the SAME hardware encoder. The
	// zero-byte first attempt is the "VRAM tight" signal — no polling, no guess (§8.2 Evictor).
	if s.reclaimVRAM != nil && spec.Profile.Encoder != playout.EncoderSoftware {
		s.log.Info("playout: hardware encode produced nothing — reclaiming GPU memory and retrying",
			"channel", channelID, "program", what, "encoder", spec.Profile.Encoder)
		s.reclaimVRAM(r.Context())
		if c := s.startChild(r.Context(), channelID, target, spec.Profile.Encoder, playout.ProgramArgs(spec)); c != nil {
			s.pipeChild(w, r, channelID, what, c)
			return
		}
	}

	// Attempt 3: software fallback. The channel plays (slower) rather than going black.
	softSpec := spec
	softSpec.Profile.Encoder = playout.EncoderSoftware
	s.log.Warn("playout: falling back to software encoding for this program",
		"channel", channelID, "program", what, "from", spec.Profile.Encoder)
	if c := s.startChild(r.Context(), channelID, target, playout.EncoderSoftware, playout.ProgramArgs(softSpec)); c != nil {
		s.pipeChild(w, r, channelID, what, c)
		return
	}

	// Even software produced nothing — genuinely unplayable (a corrupt file, a missing mount).
	s.failProgram(w, r, channelID, what, "no encoder could produce this program")
}

// liveChild is an encoder that has PROVEN it produces output — its peeked first chunk plus the
// still-running process. Returned by startChild only on success, so a caller holding one can commit
// the HTTP response knowing bytes will flow.
type liveChild struct {
	proc   *playout.Process
	first  []byte             // the first chunk, already read — must be written before draining proc
	enc    playout.Encoder    // for telemetry
	target playout.Target     // for telemetry
	cancel context.CancelFunc // ties the child to the request; pipeChild owns calling it
}

// startChild spawns one encoder and PEEKS its first chunk to prove it produces output before any
// response is committed. Returns the live child on success, or nil when the encoder produced nothing
// (the silent-failure case the ladder rescues) — logging the stderr either way. Deliberately NOT
// through the session Manager: this is one private child per demuxer request whose whole job is to
// END so the parent advances (routing it through the session map would collide on the channel key).
func (s *Server) startChild(
	ctx context.Context, channelID string, target playout.Target, enc playout.Encoder, args []string,
) *liveChild {
	// The child dies with the request. cancel is handed to the caller (pipeChild) which owns the
	// stream's lifetime; on a nil return here we cancel immediately so a failed attempt leaves no
	// orphan before the next ladder step.
	cctx, cancel := context.WithCancel(ctx)

	enc2 := enc // capture for the progress closure
	onProgress := func(p playout.Progress) {
		if s.playoutSessions != nil {
			s.playoutSessions.ReportProgram(channelID, target, enc2, p)
		}
	}
	proc, err := s.playoutEncoder(cctx, args, onProgress)
	if err != nil {
		s.log.Warn("playout: could not start the program encoder", "channel", channelID, "err", err)
		cancel()
		return nil
	}

	// PEEK the first chunk. A hardware-init failure closes stdout with no bytes — read returns
	// EOF/zero, which is exactly the silent failure the ladder exists to catch. A real encode
	// returns a chunk within a moment (the readrate burst fills the buffer immediately).
	buf := make([]byte, 64*1024)
	n, readErr := proc.Stdout.Read(buf)
	if n == 0 {
		// Nothing produced. Surface ffmpeg's own last stderr line — "Device creation failed",
		// "No capable devices found" — the actionable reason, not a generic message.
		s.log.Warn("playout: encoder produced NO OUTPUT",
			"channel", channelID, "encoder", enc, "ffmpeg", proc.LastError(), "readErr", readErr)
		cancel()
		_ = proc.Wait()
		return nil
	}
	first := make([]byte, n)
	copy(first, buf[:n])
	return &liveChild{proc: proc, first: first, enc: enc, target: target, cancel: cancel}
}

// pipeChild commits the HTTP response and streams the live child to it: the peeked first chunk, then
// the rest of the encoder's output, until the program ends (EOF) or the client disconnects.
func (s *Server) pipeChild(w http.ResponseWriter, r *http.Request, channelID, what string, c *liveChild) {
	defer c.cancel()

	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	// No Content-Length: this IS finite, unlike the tuner stream, but finite at a length not
	// known until the encode is done.
	w.Header().Set("Accept-Ranges", "none")
	w.WriteHeader(http.StatusOK)

	// The peeked first chunk goes out FIRST — it was read off the pipe before the header, so
	// skipping it would drop the start of the program.
	if _, err := w.Write(c.first); err == nil && flusher != nil {
		flusher.Flush()
	}

	_, _ = copyAndFlush(w, c.proc.Stdout, flusher)

	// Reap before returning so the encoder count is accurate the moment the demuxer re-requests.
	c.cancel()
	_ = c.proc.Wait()
}

// failProgram writes the 502 for a program that could not be produced at all — every ladder step
// exhausted. Retryable by the demuxer (it re-requests), so a transient library outage self-heals.
func (s *Server) failProgram(w http.ResponseWriter, r *http.Request, channelID, what, reason string) {
	s.log.Warn("playout: program could not be produced — this channel will not play",
		"channel", channelID, "program", what, "reason", reason)
	s.writeProblem(w, r, http.StatusBadGateway, "Couldn't start the program",
		"Loomarr couldn't start encoding this program.")
}

// copyAndFlush streams src to dst, flushing so the demuxer sees bytes promptly.
//
// Not plain io.Copy: Go buffers the response, and the concat demuxer does not treat the stream as
// started until data arrives. Flushing per chunk keeps the handoff between programs from stalling
// at the boundary — the point where a stall is most visible to a viewer.
//
// io.EOF is the SUCCESS case here: the encoder finishing is exactly what advances the channel.
func copyAndFlush(dst io.Writer, src io.Reader, flusher http.Flusher) (int64, error) {
	buf := make([]byte, 64*1024)
	var total int64
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			total += int64(written)
			if flusher != nil {
				flusher.Flush()
			}
			if writeErr != nil {
				// The demuxer went away. Normal (a channel being stopped), not a problem.
				return total, writeErr
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
}
