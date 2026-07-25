package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/store"
)

// Internal playout's HTTP surface (§9.1, §11 device auth).
//
// These are NOT typed Huma ops: they stream bytes (two of them forever), which Huma's
// typed-JSON model cannot express. They are plain mux handlers, like /v1/events and
// /v1/backup already are.
//
// The route family exists because a TELEVISION is the client. That single fact drives the
// whole design:
//
//   - It cannot hold a session cookie, so these routes authenticate a DEVICE by token rather
//     than a PERSON by session (§11). The token rides in the query string because the
//     consumers — a media server, and ffmpeg itself — are handed a URL and nothing else.
//   - It never re-requests anything on the tuner path, so a disconnect is only ever observable
//     via the request context. Nothing else will tell us the viewer left.
//   - It expects a stream that never ends, so no Content-Length, no ranges, and periodic
//     flushing.
//
// The route set mirrors what Tunarr already serves, because Emby/Jellyfin accept that shape
// today (prior-art §1) and reproducing a working contract beats inventing one:
//
//	GET /playout/tuner.m3u          the channel list the media server registers
//	GET /playout/stream/{id}        continuous MPEG-TS — what the TV actually plays
//	GET /playout/playlist/{id}      the 2-line ffconcat the parent ffmpeg reads

// playoutTokenParam is the query parameter carrying the device token.
//
// `token` rather than `api_key`: the latter is Emby's own parameter name, and these URLs are
// handed TO Emby. Reusing its name invites confusion about whose credential it is — and they
// are genuinely different secrets with different authority (§11: playout_token grants no API
// access at all, api_token is break-glass admin).
const playoutTokenParam = "token"

// PlayoutSessions is the session surface the handlers need (implemented by playout.Manager).
// Nil ⇒ the /playout/ routes still mount but report "not running", so a misconfigured install
// gets an explanation rather than a 404 that looks like a wiring mistake.
type PlayoutSessions interface {
	// Attach connects a viewer and returns its chunk channel plus a detach func. The caller
	// MUST call detach — it decrements the refcount that keeps the encoder alive.
	Attach(ctx context.Context, channelID string) (<-chan []byte, func(), error)
}

// authorizePlayout checks the device token, writing a response and returning false on failure.
//
// CONSTANT-TIME COMPARISON. The token is long and random so a timing oracle is a marginal
// threat, but this is a credential check on a route reachable by anything on the LAN, and
// subtle.ConstantTimeCompare costs nothing. Using == would be the kind of detail that is
// correct today and wrong after someone shortens the token.
//
// Returns 404, not 401 or 403. A wrong token must not reveal that the route EXISTS: these URLs
// are pasted into a media server's config and leak into logs and screenshots, so an enumerable
// "yes, that is a real channel, wrong password" tells an attacker where to aim. 404 is also
// what a media server handles most gracefully — it retries rather than prompting for
// credentials it does not have.
func (s *Server) authorizePlayout(w http.ResponseWriter, r *http.Request) bool {
	want := s.playoutToken()
	if want == "" {
		// No token configured means playout is not set up. Refusing is the safe default:
		// serving streams unauthenticated because a secret failed to mint would be a silent
		// downgrade of the only auth these routes have.
		http.NotFound(w, r)
		return false
	}
	got := r.URL.Query().Get(playoutTokenParam)
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		http.NotFound(w, r)
		return false
	}
	return true
}

// playoutToken reads the generated device secret (§15 `playout_token`).
func (s *Server) playoutToken() string {
	if s.playoutSecret == nil {
		return ""
	}
	return s.playoutSecret()
}

// playoutBaseURL is the operator-configured base every playout URL is built from.
//
// From `server.public_url`, NEVER from r.Host or X-Forwarded-Host — and the stakes here are
// higher than for the icon URLs that share this reasoning. The playlist URL is what the parent
// ffmpeg RE-OPENS FOREVER: a spoofed Host header would not merely poison one stored link, it
// would point a long-lived channel at an attacker's server for as long as it runs.
//
// Unlike icons there is no safe relative fallback. ffmpeg is a separate process resolving these
// URLs itself, with no notion of "the origin this came from", so a relative URL is not
// fetchable at all. An unset public_url therefore means playout cannot serve, and the handlers
// say so rather than emitting a URL that fails somewhere less obvious.
func (s *Server) playoutBaseURL() string {
	if s.liveConfig == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(s.liveConfig("server.public_url")), "/")
}

// playoutURL builds an absolute, token-bearing playout URL. Returns "" when the base is unset.
func (s *Server) playoutURL(kind, channelID string) string {
	base := s.playoutBaseURL()
	if base == "" {
		return ""
	}
	q := url.Values{}
	q.Set(playoutTokenParam, s.playoutToken())
	return fmt.Sprintf("%s/playout/%s/%s?%s", base, kind, url.PathEscape(channelID), q.Encode())
}

// streamHandler serves a channel as continuous MPEG-TS. This is what the television plays.
//
// Three properties make it a live stream rather than a file download, each with a failure mode
// if omitted:
//
//   - NO Content-Length and NO range support. A length would promise an end that never comes;
//     advertising ranges invites a client to seek, which is meaningless here and makes some
//     players issue a range request and then give up when it is refused.
//   - FLUSH after every chunk. Go buffers responses, so without this the first bytes can sit in
//     the buffer while the player times out waiting for a stream that is in fact flowing.
//   - The request context IS the disconnect signal. Nothing else reports it: the tuner path
//     never re-requests, so there is no next request whose absence we could notice.
func (s *Server) streamHandler(w http.ResponseWriter, r *http.Request) {
	if !s.authorizePlayout(w, r) {
		return
	}
	if s.playoutSessions == nil {
		s.writeProblem(w, r, http.StatusNotImplemented, "Playout unavailable",
			"Internal playout isn't running on this instance.")
		return
	}
	channelID := r.PathValue("id")
	if channelID == "" {
		http.NotFound(w, r)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Without flushing this cannot be a live stream at all, so failing loudly beats
		// serving something that will stall.
		s.writeProblem(w, r, http.StatusInternalServerError, "Playout unavailable",
			"This connection can't carry a live stream.")
		return
	}

	chunks, detach, err := s.playoutSessions.Attach(r.Context(), channelID)
	if err != nil {
		// At capacity is a real, actionable condition rather than a generic failure: the
		// operator can raise playout.max_channels or lower the quality tier so more channels
		// fit. 503 + Retry-After is also what makes a media server back off politely instead
		// of hammering.
		if errors.Is(err, playout.ErrAtCapacity) {
			w.Header().Set("Retry-After", "30")
			s.writeProblem(w, r, http.StatusServiceUnavailable, "All tuners are busy",
				"Loomarr is already encoding as many channels as it's configured to handle. "+
					"Raise the channel limit in Settings → Playout, or choose a lower quality tier.")
			return
		}
		s.log.Warn("playout: attach failed", "channel", channelID, "err", err)
		s.writeProblem(w, r, http.StatusBadGateway, "Couldn't start the channel",
			"Loomarr couldn't start encoding this channel. Check the playout log for details.")
		return
	}
	// MUST run: it is what decrements the refcount, and a leaked viewer keeps a channel
	// encoding forever (playout.Manager.Attach).
	defer detach()

	w.Header().Set("Content-Type", "video/mp2t")
	// A live stream must never be cached, by the client or anything between us.
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	// Explicitly refuse ranges rather than staying silent — see above.
	w.Header().Set("Accept-Ranges", "none")
	w.WriteHeader(http.StatusOK)
	flusher.Flush() // send headers now so the player knows it is connected

	for {
		select {
		case <-r.Context().Done():
			return // the viewer left; detach fires
		case chunk, ok := <-chunks:
			if !ok {
				// The session ended: the encoder exited, the channel was stopped, or this
				// viewer fell too far behind and was dropped (playout.broadcast).
				return
			}
			if _, err := w.Write(chunk); err != nil {
				return // the client went away mid-write
			}
			flusher.Flush()
		}
	}
}

// playlistHandler serves the two-line ffconcat playlist the parent ffmpeg reads.
//
// Both lines are the same program URL — that is the mechanism, not a mistake (prior-art §1).
// The concat demuxer needs a second entry to advance to when the first hits EOF, and
// `-stream_loop -1` cycles between them forever; each open of the program URL asks "what is
// airing now?" and gets whatever is current.
func (s *Server) playlistHandler(w http.ResponseWriter, r *http.Request) {
	if !s.authorizePlayout(w, r) {
		return
	}
	programURL := s.playoutURL("program", r.PathValue("id"))
	if programURL == "" {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "Playout isn't configured",
			"Set Loomarr's public address in Settings → Server so streams can be reached.")
		return
	}

	// text/plain: ffmpeg does not care about the type, and there is no registered one for
	// ffconcat. Nosniff so a browser that opens the URL cannot reinterpret the bytes.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(playout.Playlist(programURL)))
}

// tunerHandler serves the M3U channel list the media server registers as a tuner.
//
// `#EXTINF` + the tvg-* attributes are how a media server correlates a stream with its guide
// entry: tvg-id must match the XMLTV channel id exactly, or the channel appears with no
// listings. That is the most common Live TV wiring failure and it is SILENT — the channel
// plays, the guide is just empty.
func (s *Server) tunerHandler(w http.ResponseWriter, r *http.Request) {
	if !s.authorizePlayout(w, r) {
		return
	}
	if s.playoutBaseURL() == "" {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "Playout isn't configured",
			"Set Loomarr's public address in Settings → Server so your media server can reach the streams.")
		return
	}

	channels, err := s.playoutChannels(r.Context())
	if err != nil {
		s.log.Warn("playout: tuner list failed", "err", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "Couldn't build the channel list",
			"Something went wrong reading your channels.")
		return
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, ch := range channels {
		// tvg-id ties this entry to its XMLTV <channel id>. tvg-chno gives the media server
		// the channel NUMBER; without it the server assigns its own ordering and the
		// operator's numbering is lost.
		fmt.Fprintf(&b, "#EXTINF:-1 tvg-id=%q tvg-name=%q tvg-chno=%q", ch.ID, ch.Name, ch.Number)
		if ch.Logo != "" {
			fmt.Fprintf(&b, " tvg-logo=%q", ch.Logo)
		}
		fmt.Fprintf(&b, ",%s\n%s\n", ch.Name, s.playoutURL("stream", ch.ID))
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	// No caching: a channel added in the UI must appear on the next tuner refresh.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(b.String()))
}

// playoutChannel is one row of the tuner list.
type playoutChannel struct {
	ID     string
	Name   string
	Number string
	Logo   string
}

// playoutChannels lists the channels internal playout serves.
//
// ONLY channels actually on the internal backend. A channel Tunarr is playing must not appear
// in Loomarr's tuner, or the media server has two tuners offering the same channel and picks
// between them unpredictably — which presents as a channel that plays fine sometimes and not
// others. `playout.backend` is per-channel overridable via policy_json (§15), so this is a real
// filter and not a global on/off.
func (s *Server) playoutChannels(ctx context.Context) ([]playoutChannel, error) {
	chans, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]playoutChannel, 0, len(chans))
	for _, ch := range chans {
		if !s.playsInternally(ch) {
			continue
		}
		out = append(out, playoutChannel{
			ID:     ch.ID,
			Name:   ch.Name,
			Number: strconv.Itoa(ch.Number),
			Logo:   ch.Logo,
		})
	}
	return out, nil
}

// playsInternally reports whether a channel is served by internal playout.
//
// The precedence is the nil-means-inherit shape §15 defines: a channel's own
// `policy.playout.backend` wins when set, otherwise the global `playout.backend`. Both a nil
// *PlayoutPolicy and an empty Backend mean "inherit", so a hand-edited policy_json cannot mean
// something surprising.
func (s *Server) playsInternally(ch store.Channel) bool {
	if p := ch.Policy.Playout; p != nil && p.Backend != "" {
		return p.Backend == playoutBackendInternal
	}
	if s.liveConfig == nil {
		return false
	}
	return strings.TrimSpace(s.liveConfig("playout.backend")) == playoutBackendInternal
}

// playoutBackendInternal is the `playout.backend` enum value meaning "Loomarr streams it".
const playoutBackendInternal = "internal"

// registerPlayout mounts the device-authenticated playout routes (§9.1).
//
// Deliberately NOT under /v1: these are not API operations and not part of the versioned
// contract the generated client consumes. They are a machine-to-machine surface whose shape is
// dictated by what media servers accept (prior-art §1), so versioning it alongside the JSON API
// would imply a freedom to change it that we do not have.
func (s *Server) registerPlayout(mux *http.ServeMux) {
	mux.HandleFunc("GET /playout/tuner.m3u", s.tunerHandler)
	mux.HandleFunc("GET /playout/stream/{id}", s.streamHandler)
	mux.HandleFunc("GET /playout/playlist/{id}", s.playlistHandler)
}
