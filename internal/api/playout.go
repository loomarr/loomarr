package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// Internal playout's HTTP surface (§9.1, §11 device auth).
//
// These stream bytes (some forever), which Huma's typed-JSON MODEL cannot express — but that does
// not mean they live outside the framework. They register on the SAME Huma API as everything else
// (V47) and stream through the raw response writer via humago.Unwrap; auth is a shared middleware
// (playoutAuthMiddleware), not a hand-rolled inline check re-implemented per handler. This mounts
// them in the one router the dev proxy and every other guarantee already cover, and it ends the
// parallel plain-mux world that (a) duplicated auth and (b) put these routes outside the SPA's
// origin, so the in-app player's same-origin URLs did not resolve in dev.
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

// playoutPlanParam names the codec-audience query on the playlist and program URLs (§9.1 V48):
// `?plan=baseline|hevc8|hevc10|full`. It selects the copy/transcode plan; see playout.EncodePlan and
// clientPlan (the client-safe reader). ⚠ Replaces the retired `?target=browser|mediaserver` token
// (see scripts/check-retired.sh).
const playoutPlanParam = "plan"

// PlayoutSessions is the session surface the handlers need (implemented by playout.Manager).
// Nil ⇒ the /playout/ routes still mount but report "not running", so a misconfigured install
// gets an explanation rather than a 404 that looks like a wiring mistake.
type PlayoutSessions interface {
	// Attach connects a viewer to a (channel, target) and returns its chunk channel plus a detach
	// func (§9.1 V47). The caller MUST call detach — it decrements the refcount that keeps the
	// encoder alive. The tuner path attaches as TargetMediaServer; only the copy plan differs.
	Attach(ctx context.Context, channelID string, target playout.EncodePlan) (<-chan []byte, func(), error)

	// Stats snapshots every live encoder for the dashboard (§12, V16).
	Stats(now time.Time) []playout.SessionStat
	// Capacity is the admission bound — the denominator in "2 / 4".
	Capacity() int
	// ReportProgram records the CURRENT program's encoder + progress for a channel.
	//
	// Reported from the per-program path rather than captured at session start, because the
	// session's own ffmpeg is the `-c copy` parent and never encodes: its speed would measure
	// remuxing and its encoder would be copy. Encoding happens in the per-program children,
	// and the load-aware Resolve can legitimately pick differently between programs.
	ReportProgram(channelID string, target playout.EncodePlan, enc playout.Encoder, transcoding bool, p playout.Progress)
}

// PlayoutHLS is the HLS repackaging surface the Watch handlers need (§9.1, V46), implemented by
// playout.HLSManager. Nil ⇒ the /playout/hls routes report "not running", so an install without
// internal playout wired explains itself rather than 404ing.
type PlayoutHLS interface {
	// Playlist starts (or joins) a channel's HLS remux for `plan` and returns the on-disk MASTER
	// playlist path plus a detach func the caller MUST invoke — it releases the viewer refcount that
	// keeps the remux, and through it the channel's encoder, alive. The plan selects the copy plan: a
	// baseline client (h264) and an HEVC-capable client get SEPARATE remuxes/sessions of one channel,
	// so an HEVC channel copies its video for the capable client while transcoding for the baseline one.
	Playlist(channelID string, plan playout.EncodePlan) (playlistPath string, detach func(), err error)
	// AssetPath resolves an HLS asset under a channel's `plan` remux — a variant playlist or a
	// segment, by its path relative to the channel dir (e.g. "720p/seg-3.ts") — to its on-disk path
	// for a live channel; ok=false when no remux is running or the path would escape the dir.
	AssetPath(channelID string, plan playout.EncodePlan, rel string) (string, bool)
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
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1 {
		return true
	}
	// A browser/native player carries a SIGNED URL instead of the device token (§9.1 Watch,
	// playoutsign.go): a capability scoped to this channel and expiring on its own. Accepted
	// only when the request path names a channel to scope it to — the tuner list and other
	// channel-less routes take the device token alone. Same 404-on-failure as the token path,
	// so a bad or expired signature is indistinguishable from a wrong token.
	if sig := r.URL.Query().Get(signQueryParam); sig != "" {
		if id := r.PathValue("id"); id != "" && s.verifyPlayoutSignature(sig, id, time.Now()) {
			return true
		}
	}
	http.NotFound(w, r)
	return false
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
	return fmt.Sprintf("%s/v1/playout/%s/%s?%s", base, kind, url.PathEscape(channelID), q.Encode())
}

// withPlan appends `&plan=<p>` to a playout URL that already carries a query (playoutURL always
// sets the token, so there is always a `?`). It is the one place the plan is written on the internal
// playlist→program hop, keyed off EncodePlan.String, so a program child inherits the session's served
// plan (§9.1 V50: PlanBaseline=h264/TS, PlanHEVC8/10=HEVC/fMP4) and its copy/transcode decision
// cannot drift from the parent's. Written explicitly (including PlanBaseline) so the served plan is
// self-documenting in a log line and unambiguous about how the child must encode.
func withPlan(rawURL string, t playout.EncodePlan) string {
	return rawURL + "&" + playoutPlanParam + "=" + url.QueryEscape(t.String())
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

	// The raw MPEG-TS tuner audience is a media server (Emby/Jellyfin), which ingests HEVC/AC3 and
	// adapts per-client downstream — so it attaches as TargetMediaServer and keeps HEVC as a copy
	// (§9.1 V47). The browser's HLS remux attaches as TargetBrowser separately.
	chunks, detach, err := s.playoutSessions.Attach(r.Context(), channelID, playout.PlanFull)
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

	// Propagate the session's PLAN (§9.1 V48) down to the program URL: a baseline session's parent
	// fetches `playlist?plan=baseline`, so every program it requests must also carry `baseline` and
	// plan its copy for that bucket. The playlist and its programs are one codec audience; the plan is
	// what makes them so. This is an INTERNAL parent→program hop — the plan was already fixed when the
	// session was created (clientPlan enforced the safe default at the client edge), so ParseEncodePlan
	// here round-trips the canonical token; an absent one degrades to PlanBaseline, the safe floor.
	plan := playout.ParseEncodePlan(r.URL.Query().Get(playoutPlanParam))
	programURL = withPlan(programURL, plan)

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
// The precedence rule itself lives in schedule.PlaysInternally — ONE copy, because it decides
// which subsystem answers for a channel and is consulted from several surfaces. See the note
// there on why a drifted second copy fails silently rather than loudly.
//
// No liveConfig ⇒ false: with no way to read the global, the safe answer is "not ours", so the
// tuner advertises nothing rather than advertising channels it may not be serving.
func (s *Server) playsInternally(ch store.Channel) bool {
	if s.liveConfig == nil {
		if p := ch.Policy.Playout; p != nil && p.Backend != "" {
			return p.Backend == schedule.PlayoutBackendInternal
		}
		return false
	}
	return schedule.PlaysInternally(ch.Policy, s.liveConfig("playout.backend"))
}

// hlsPlaylistHandler serves a channel's live HLS media playlist for the in-app player (§9.1
// Watch, V46). Authorized by EITHER the device token OR a signed URL (authorizePlayout), so the
// same route serves a browser (signed) that the tuner list points a media server at (token).
//
// It reads the playlist ffmpeg is writing to disk and returns it, holding a viewer refcount for
// the duration of… no — the opposite. A playlist fetch is a SNAPSHOT: hls.js re-fetches the
// playlist every few seconds to discover new segments, so holding the refcount for one fetch
// would drop the remux between polls. The refcount must span the WHOLE viewing session, which no
// single stateless request can bracket. So the playlist fetch itself keeps the remux alive for a
// grace window after the last fetch (the remux's own idle timer), and the refcount is taken-and-
// released per fetch: each poll re-arms the grace timer, and when the polls stop, the timer fires.
func (s *Server) hlsPlaylistHandler(w http.ResponseWriter, r *http.Request) {
	if !s.authorizePlayout(w, r) {
		return
	}
	if s.playoutHLS == nil {
		s.writeProblem(w, r, http.StatusNotImplemented, "Playout unavailable",
			"Internal playout isn't running on this instance.")
		return
	}
	channelID := r.PathValue("id")
	if channelID == "" {
		http.NotFound(w, r)
		return
	}

	path, detach, err := s.playoutHLS.Playlist(channelID, clientPlan(r))
	if err != nil {
		if errors.Is(err, playout.ErrAtCapacity) {
			w.Header().Set("Retry-After", "30")
			s.writeProblem(w, r, http.StatusServiceUnavailable, "All tuners are busy",
				"Loomarr is already encoding as many channels as it's configured to handle. "+
					"Raise the channel limit in Settings → Playout, or choose a lower quality tier.")
			return
		}
		s.log.Warn("playout: hls playlist failed", "channel", channelID, "err", err)
		s.writeProblem(w, r, http.StatusBadGateway, "Couldn't start the channel",
			"Loomarr couldn't start this channel's in-app stream.")
		return
	}
	// Release THIS fetch's refcount as it returns — the remux's grace timer keeps it alive
	// between the client's playlist polls (see the handler doc).
	defer detach()

	body, rerr := os.ReadFile(path)
	if rerr != nil {
		http.NotFound(w, r) // the playlist vanished between the readiness wait and now (teardown)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(rewritePlaylistAuth(body, r.URL.RawQuery))
}

// clientPlan reads the EncodePlan for a Watch HLS request (a browser OR a native app) from `?plan=`.
// It is the CLIENT-FACING edge, and its default is the safety invariant of the whole V48 model:
//
//   - An absent, empty, or unrecognized `?plan=` resolves to PlanBaseline (h264/aac) — NEVER a
//     richer plan. Copying HEVC (or 10-bit, or surround) to a client that did not prove it can decode
//     it is a black frame, so only an explicit, recognized plan token unlocks richer copy.
//   - The token itself was minted from the client's DeviceProfile at play-url time (resolve →
//     `?plan=`), and resolve already rounded DOWN to what the profile fully satisfies. So there are
//     two independent guards: resolve at mint, clientPlan here at read. Either alone keeps a client
//     from receiving an undecodable stream; both is defence in depth.
//
// ParseEncodePlan already defaults unknown→PlanBaseline, so this is a thin, intention-naming wrapper
// over it — but the wrapper (and this comment) is where the client-edge invariant is asserted, so it
// is not inlined.
func clientPlan(r *http.Request) playout.EncodePlan {
	return playout.ParseEncodePlan(r.URL.Query().Get(playoutPlanParam))
}

// rewritePlaylistAuth appends the request's auth query (`?sig=…` or `?token=…`) to every SEGMENT
// URI in a media playlist, so each segment fetch is self-authenticating.
//
// ⚠ This is REQUIRED, not a nicety, and it is why token-authed HLS origins all rewrite playlists.
// ffmpeg writes bare segment URIs (`seg-0.ts`); a client resolves them relative to the playlist's
// PATH but does NOT carry the playlist's query string onto them — so a bare segment request arrives
// with no credential and 404s (authorizePlayout). Native players (iOS/Android/Roku) will not
// re-append a query param either, so fixing it client-side (hls.js xhrSetup) would leave every
// native player broken. Rewriting the URIs here is the one fix that works for every client.
//
// Only lines that are segment/asset references get the query — most tag lines (starting with `#`)
// and blank lines are left untouched. The ONE exception is `#EXT-X-MAP:URI="…"`, the fMP4 init
// segment (§9.1 V48 HEVC): it is a tag but its URI is a fetch that must self-authenticate too, or the
// init segment 404s and an HEVC stream black-screens. So its quoted URI is rewritten in place.
func rewritePlaylistAuth(body []byte, rawQuery string) []byte {
	if rawQuery == "" {
		return body
	}
	appendQuery := func(uri string) string {
		sep := "?"
		if strings.Contains(uri, "?") {
			sep = "&"
		}
		return uri + sep + rawQuery
	}
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		// EXT-X-MAP carries the fMP4 init segment as a quoted URI="…" — rewrite that URI, not the
		// whole tag. The rest of the tag (BYTERANGE etc.) is left intact.
		if strings.HasPrefix(t, "#EXT-X-MAP:") {
			const marker = `URI="`
			if start := strings.Index(t, marker); start != -1 {
				uStart := start + len(marker)
				if end := strings.Index(t[uStart:], `"`); end != -1 {
					uri := t[uStart : uStart+end]
					lines[i] = t[:uStart] + appendQuery(uri) + t[uStart+end:]
				}
			}
			continue
		}
		if strings.HasPrefix(t, "#") {
			continue // any other tag — not a URI
		}
		// A URI line. Append the query, preserving any it already carries (it never does today,
		// but a `?` in the name must not be doubled).
		lines[i] = appendQuery(t)
	}
	return []byte(strings.Join(lines, "\n"))
}

// hlsAssetHandler serves an HLS asset under a channel — a variant media playlist
// (`720p/stream.m3u8`) or a segment (`720p/seg-12.ts`) — for the ABR ladder (§9.1 Watch, V46).
// Same dual auth as the master playlist. The asset path (captured as a trailing wildcard) is
// validated against traversal here AND again in AssetPath (defence in depth).
func (s *Server) hlsAssetHandler(w http.ResponseWriter, r *http.Request) {
	if !s.authorizePlayout(w, r) {
		return
	}
	if s.playoutHLS == nil {
		http.NotFound(w, r)
		return
	}
	channelID := r.PathValue("id")
	rel := r.PathValue("asset") // the trailing "<variant>/<file>" path
	// Reject a parent ref up front — the asset paths are all "<variant>/<file>" and never need
	// to climb. AssetPath re-checks containment after cleaning, so this is the outer of two gates.
	if channelID == "" || rel == "" || strings.Contains(rel, "..") {
		http.NotFound(w, r)
		return
	}

	path, ok := s.playoutHLS.AssetPath(channelID, clientPlan(r), rel)
	if !ok {
		http.NotFound(w, r)
		return
	}
	// Content type by suffix — the only discriminator the asset carries. MPEG-TS segments (.ts) are
	// the baseline plan; fMP4 segments (.m4s) and their init segment (init.mp4) are the HEVC plans
	// (§9.1 V48 — HEVC HLS must be fMP4). An unknown suffix falls through to the TS default, harmless
	// because AssetPath already proved the file exists under the channel's own remux dir.
	switch {
	case strings.HasSuffix(rel, ".m3u8"):
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	case strings.HasSuffix(rel, ".m4s"), strings.HasSuffix(rel, ".mp4"):
		w.Header().Set("Content-Type", "video/mp4")
	default:
		w.Header().Set("Content-Type", "video/mp2t")
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}

// registerPlayout mounts the playout streaming routes on the Huma API (§9.1, V47).
//
// ⚠ These stream bytes, which Huma's typed-JSON model cannot express — but they mount on the SAME
// Huma API as every other route, via huma.StreamResponse (the framework's streaming primitive) and
// the raw response writer (humago.Unwrap). This replaced a parallel plain-mux registration that
// duplicated auth per handler and, because it lived outside the app's router surface, put these
// routes outside the SPA's origin — which is why the in-app player's same-origin URLs did not
// resolve in dev. Auth is ONE shared middleware (playoutAuthMiddleware), not re-derived per handler.
//
// ⚠ **They are now IN the OpenAPI spec, and that is a documentation claim, not a stability
// promise.** The previous text here said they were "deliberately NOT under /v1 and hidden",
// which had gone stale twice over: the paths have been `/v1/playout/…` since V47, and hiding
// them meant `api/openapi.yaml` silently under-reported the served surface — the same class of
// gap that let `GET /v1/playout/sessions` run live and undocumented. Their shape is still
// dictated by what media servers accept (prior-art §1), NOT by us, so appearing in the spec
// describes what we serve without implying we are free to change it or that a generated client
// should drive it. Read the response schemas as "these bytes, this media type" only.
//
// The handler BODIES (streamHandler, hlsPlaylistHandler, …) are unchanged — they still take (w, r)
// and stream; this only changes how they are mounted, guarded and described.
func (s *Server) registerPlayout(api huma.API) {
	// The tuner list + XMLTV the media server registers.
	streamOp[struct{}](s, api, bytesResponse(huma.Operation{
		OperationID: "playout-tuner", Method: http.MethodGet, Path: "/v1/playout/tuner.m3u",
		Summary: "Playout tuner M3U (device-authed)", Tags: []string{"playout"},
	}, "The M3U tuner list a media server registers as a Live TV source.",
		"application/vnd.apple.mpegurl"), s.tunerHandler)
	// The XMLTV listings (playoutguide.go). Without it a channel tunes with an empty EPG.
	streamOp[struct{}](s, api, bytesResponse(huma.Operation{
		OperationID: "playout-guide", Method: http.MethodGet, Path: "/v1/playout/guide.xml",
		Summary: "Playout XMLTV guide (device-authed)", Tags: []string{"playout"},
	}, "XMLTV listings for every internally-played channel.", "application/xml"), s.guideHandler)

	// The continuous MPEG-TS a TV/media server plays, plus the ffconcat + program the parent reads.
	streamOp[playoutChannelInput](s, api, bytesResponse(huma.Operation{
		OperationID: "playout-stream", Method: http.MethodGet, Path: "/v1/playout/stream/{id}",
		Summary: "Channel MPEG-TS stream (device-authed)", Tags: []string{"playout"},
	}, "A continuous transport stream of whatever the channel is playing now.",
		"video/mp2t"), s.streamHandler)
	streamOp[playoutChannelInput](s, api, bytesResponse(huma.Operation{
		OperationID: "playout-playlist", Method: http.MethodGet, Path: "/v1/playout/playlist/{id}",
		Summary: "Channel ffconcat playlist (device-authed)", Tags: []string{"playout"},
	}, "The ffconcat playlist the parent ffmpeg reads to sequence programs.",
		"text/plain"), s.playlistHandler)
	// The sequencing layer (playoutprogram.go): the concat demuxer re-opens this once per program.
	streamOp[playoutChannelInput](s, api, bytesResponse(huma.Operation{
		OperationID: "playout-program", Method: http.MethodGet, Path: "/v1/playout/program/{id}",
		Summary: "One program's MPEG-TS (device-authed)", Tags: []string{"playout"},
	}, "One program's transport stream; the concat demuxer re-opens this per program.",
		"video/mp2t"), s.programHandler)

	// The in-app browser/native HLS surface (§9.1 Watch). Master playlist + its segments, authed by
	// the signed URL (or the device token). Same-origin under the Huma API now, so the dev proxy and
	// the SPA origin cover them — which fixed the in-app player. The signed URL is kept because the
	// native players (iOS/Android/Roku) fetch HLS with their own networking and carry no session —
	// Roku especially forces a credential in the URL, so a URL-borne credential is the one mechanism
	// that works across every player + the browser.
	streamOp[playoutChannelInput](s, api, bytesResponse(huma.Operation{
		OperationID: "playout-hls-master", Method: http.MethodGet, Path: "/v1/playout/hls/{id}/master.m3u8",
		Summary: "Channel HLS master playlist (signed-URL authed)", Tags: []string{"playout"},
	}, "The HLS master playlist for the in-app and native players.",
		"application/vnd.apple.mpegurl"), s.hlsPlaylistHandler)
	// A segment is a bare file beside the master (`seg-N.ts`) — direct play is one playlist, no
	// variant subdirs, so a single `{asset}` segment (not a trailing wildcard) is enough.
	//
	// Two content types because this one route genuinely serves both: `{asset}` is a segment
	// (video/mp2t) or the media playlist beside the master (vnd.apple.mpegurl), decided by the
	// filename asked for — see hlsAssetHandler.
	streamOp[playoutAssetInput](s, api, bytesResponse(huma.Operation{
		OperationID: "playout-hls-asset", Method: http.MethodGet, Path: "/v1/playout/hls/{id}/{asset}",
		Summary: "Channel HLS segment or media playlist (signed-URL authed)", Tags: []string{"playout"},
	}, "A segment, or the media playlist beside the master.",
		"video/mp2t", "application/vnd.apple.mpegurl"), s.hlsAssetHandler)
}

// playoutChannelInput addresses one channel's playout by its Loomarr channel id.
//
// Declared for the SPEC, not for the handlers — they still read r.PathValue("id"). Huma builds
// the parameter list from this struct and never cross-checks it against the `{placeholders}` in
// Path, so an op registered with struct{} emits a path template whose parameter is defined
// nowhere. That was invisible while these routes were Hidden (see rawop.go).
type playoutChannelInput struct {
	ID string `path:"id" example:"ch_abc123" doc:"Loomarr channel id"`
}

// playoutAssetInput is playoutChannelInput plus the file beside the master playlist.
type playoutAssetInput struct {
	ID    string `path:"id" example:"ch_abc123" doc:"Loomarr channel id"`
	Asset string `path:"asset" example:"seg-7.ts" doc:"A file beside the master playlist — a segment, or the media playlist"`
}

// streamOp registers one playout streaming route on the Huma API: method + path, the shared playout
// auth middleware, and a StreamResponse body that hands the existing (w, r) handler control. `body`
// is the current stdlib handler — reused verbatim.
//
// A thin specialisation of rawOp (rawop.go) pinning the two things every playout route shares.
// It is a package-level function rather than a method because it takes a type parameter for the
// path params, and Go methods cannot.
func streamOp[I any](s *Server, api huma.API, op huma.Operation, body func(http.ResponseWriter, *http.Request)) {
	// RolePublic tells the GLOBAL session-auth middleware (registerMiddleware) to skip these — they
	// do not authenticate a PERSON by session (which defaults to admin-required, a 401). Their real
	// auth is the device token / signed URL, enforced by playoutAuthMiddleware below. Public to the
	// session layer, guarded by the playout layer — the correct two-layer split (§11).
	rawOp[I](api, op, RolePublic, body, s.playoutAuthMiddleware)
}

// playoutAuthMiddleware is the ONE authorization point for playout streaming routes — the device
// token OR a signed URL, exactly what authorizePlayout checked inline, now shared across every route
// instead of re-derived per handler. On failure it writes the 404 (see authorizePlayout for why
// 404) and does NOT call next, so the stream handler never runs.
func (s *Server) playoutAuthMiddleware(hctx huma.Context, next func(huma.Context)) {
	r, w := humago.Unwrap(hctx)
	if !s.authorizePlayout(w, r) {
		return // authorizePlayout already wrote the 404
	}
	next(hctx)
}
