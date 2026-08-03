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
		args := playout.OfflineCardArgs(profile, font,
			"Nothing scheduled", channelID, offlineCardDuration)
		s.streamChild(w, r, channelID, "offline card", args, profile.Encoder)
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

	args := playout.ProgramArgsNormalised(
		profile, streamURL, airing.Offset, airing.Remaining, audioTrack, targetLUFS)
	s.streamChild(w, r, channelID, airing.Title, args, profile.Encoder)
}

// offlineCardDuration is how long one offline-card request lasts before the demuxer re-asks.
//
// Short enough that a channel starts playing promptly once content lands (the operator approves
// a title, an acquisition completes, the next re-request finds it), long enough that an empty
// channel is not re-encoding every second.
const offlineCardDuration = 30 * time.Second

// streamChild spawns one encoder and pipes it to the response until it ends.
//
// Deliberately NOT going through the session Manager. A session is a SHARED encoder fanned out to
// N viewers; this is the opposite — one private child per demuxer request, whose whole job is to
// END so the parent advances. Routing it through the session map would collide on the channel key
// and, worse, the grace-period teardown would keep a finished program's encoder alive.
func (s *Server) streamChild(
	w http.ResponseWriter, r *http.Request, channelID, what string, args []string, enc playout.Encoder,
) {
	// The child dies with the request. If the parent ffmpeg goes away — the channel was stopped,
	// the last viewer left, the process was killed — this context cancels and takes the encoder
	// with it. Without that binding a child outlives its parent and becomes an orphan nobody
	// will ever reap.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Progress from THIS child is the channel's real encoder telemetry (V16). Reported to the
	// session by channel id, because the child is per-program and short-lived while the
	// dashboard asks about the channel. A report for a channel with no live session is dropped
	// by the manager — the child is bound to its request and can briefly outlive a teardown.
	onProgress := func(p playout.Progress) {
		if s.playoutSessions != nil {
			s.playoutSessions.ReportProgram(channelID, enc, p)
		}
	}
	proc, err := s.playoutEncoder(ctx, args, onProgress)
	if err != nil {
		s.log.Warn("playout: could not start the program encoder",
			"channel", channelID, "program", what, "err", err)
		s.writeProblem(w, r, http.StatusBadGateway, "Couldn't start the program",
			"Loomarr couldn't start encoding this program.")
		return
	}

	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	// No Content-Length: this IS finite, unlike the tuner stream, but finite at a length not
	// known until the encode is done.
	w.Header().Set("Accept-Ranges", "none")
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}

	n, copyErr := copyAndFlush(w, proc.Stdout, flusher)

	// Reap it. The deferred cancel would also kill it, but waiting HERE means the process is
	// gone before the response ends, so the encoder count is accurate the moment the demuxer
	// re-requests.
	cancel()
	_ = proc.Wait()

	// ZERO BYTES IS ALWAYS WRONG, whether or not the copy reported an error.
	//
	// This condition was `copyErr != nil && n == 0` and it never fired for the case that
	// matters most: an encoder that dies at startup closes its stdout, which the copy sees as
	// a clean EOF — so copyErr is NIL and n is 0. The channel silently produced nothing while
	// the viewer's player sat buffering, and the only clue (ffmpeg's stderr) was logged at
	// DEBUG. Found the hard way: a misconfigured hardware encoder in a container with no GPU
	// took a live channel down with not one line in the log at INFO.
	//
	// ffmpeg's own last stderr line is the useful part — "Device creation failed", "No such
	// file or directory" — so it is surfaced here rather than left to a debug-level sink.
	if n == 0 {
		s.log.Warn("playout: the encoder produced NO OUTPUT — this channel will not play",
			"channel", channelID, "program", what,
			"ffmpeg", proc.LastError(), "copyErr", copyErr)
	}
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
