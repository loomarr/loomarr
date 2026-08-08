package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/mantonx/loomarr/internal/store"
)

// POST /v1/channels/{id}/play-url — mint a signed HLS URL for the in-app player (§9.1 Watch).
//
// This is the bridge between "a person is logged in" and "a device plays a stream". The caller
// is session-authenticated (any member — watching is not an admin action); the response is a
// short-lived signed URL their `<video>`/hls.js loads. The device token never crosses the wire;
// see playoutsign.go for the signature and why it is keyed by `playout_token`.
//
// Member, not admin: a viewer can watch. The stream itself exposes only what the household's
// media server already serves, and the signed URL is scoped to one channel and expires — so
// this grants strictly less than the media-server client every viewer already has.

// registerChannelPlayURL mounts the play-url op. Split from registerChannels only so the Watch
// surface's routes live beside their handler; it is called from the same Register sweep.
func (s *Server) registerChannelPlayURL(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "channel-play-url", Method: http.MethodPost, Path: "/v1/channels/{id}/play-url",
		Summary: "Mint a signed URL to watch this channel in the browser",
		Description: "Any signed-in user. Returns a short-lived, signed HLS URL the in-app player (or a native app) " +
			"loads directly — the device playout token never reaches the client (§9.1, §11). The URL is scoped to " +
			"this channel and expires; request a fresh one when it lapses. `quality` picks a per-viewer tier " +
			"(auto/720p/480p) — the only per-viewer control, since one encoder serves every viewer of a channel.",
		Tags: []string{"channels"},
	}, RoleMember), s.channelPlayURL)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "channel-tracks", Method: http.MethodGet, Path: "/v1/channels/{id}/tracks",
		Summary: "Audio and subtitle tracks of what's airing now",
		Description: "Any signed-in user. The audio and subtitle tracks the CURRENTLY-AIRING programme actually " +
			"carries — what the Watch tab's Audio/Subtitle pickers offer. These come from the media (an ffprobe of the " +
			"airing file), not a fixed list and not a setting, so the choices are real. Best-effort: an empty list when " +
			"nothing is airing or the probe can't run, never an error — a viewer still gets the channel's current default.",
		Tags: []string{"channels"},
	}, RoleMember), s.channelTracks)
}

type playURLInput struct {
	ID string `path:"id" example:"ch_abc123"`
	// Quality is the per-viewer tier the player asks for. Optional; empty = auto. Deliberately
	// a small closed set the player offers, not the operator's `playout.quality_tier` names —
	// this is the viewer's session preference, resolved into a variant at request time.
	Quality string `query:"quality" enum:"auto,720,480" example:"auto"`
}

type playURLOutput struct {
	Body struct {
		// URL is the ABSOLUTE signed HLS URL — for a NATIVE app or media server, which fetches it
		// cold with no origin of its own. Built from server.public_url; empty if that is unset.
		URL string `json:"url" doc:"Absolute signed HLS (.m3u8) URL for native/off-origin clients; empty if the server's public address is unset"`
		// RelativeURL is the SAME-ORIGIN signed HLS URL — for the in-app WEB player. A browser is
		// already on Loomarr's origin, so a relative URL needs no CORS (hls.js fetches by XHR) and
		// works even when public_url is unset. Web clients should prefer this.
		RelativeURL string `json:"relativeUrl" doc:"Same-origin signed HLS (.m3u8) URL for the in-app web player — prefer this in a browser (no CORS)"`
		// ExpiresAt is when the URLs stop verifying — the client re-mints before then.
		ExpiresAt time.Time `json:"expiresAt" doc:"When the signed URLs expire (RFC 3339)"`
	}
}

func (s *Server) channelPlayURL(ctx context.Context, in *playURLInput) (*playURLOutput, error) {
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	}
	if err != nil {
		return nil, err
	}

	// A channel that Loomarr does not stream itself has no in-app HLS to serve — that stream
	// lives in Tunarr, reached through the media server, not here. Say so rather than mint a
	// URL whose segments would 404, so the UI can point the viewer at the right client.
	if !s.playsInternally(ch) {
		return nil, errConflict(
			"This channel isn't streamed by Loomarr",
			"Tunarr streams this channel — watch it in your media server. In-app playback is available "+
				"only for channels Loomarr plays out itself (Settings → Playout).")
	}

	exp := time.Now().Add(playURLTTL)
	quality := normalizeQuality(in.Quality)
	rel := s.playoutHLSPathURL(ch.ID, quality, exp)
	if rel == "" {
		// The RELATIVE URL fails only when the signature can't be minted (no playout token) —
		// which means playout is genuinely unconfigured. The web player can use the relative URL
		// even with no public_url set, so the wall is keyed on the signature, not the base.
		return nil, errServiceUnavailable(
			"Playout isn't configured",
			"Loomarr's playout secret isn't set up yet, so channels can't be watched.")
	}

	out := &playURLOutput{}
	// Absolute may be empty (public_url unset) — that only strands NATIVE clients, and the message
	// for them is different (set the public address), so it is not an error for the web player.
	out.Body.URL = s.playoutHLSURL(ch.ID, quality, exp)
	out.Body.RelativeURL = rel
	out.Body.ExpiresAt = exp
	return out, nil
}

// normalizeQuality maps the viewer's requested tier to the value carried in the HLS URL.
// "auto" and "" both mean "let playout decide", carried as an empty query param so the HLS
// handler applies its own ladder; an explicit tier is passed through for the handler to honour.
func normalizeQuality(q string) string {
	switch q {
	case "720", "480":
		return q
	default:
		return "" // auto / unset / anything unexpected → let playout choose
	}
}

// TrackDTO is one selectable audio or subtitle track, as the Watch pickers show it.
type TrackDTO struct {
	Index    int    `json:"index" doc:"Position among tracks of its own type — the N in -map 0:a:N / 0:s:N."`
	Language string `json:"language,omitempty" doc:"ISO 639-2 code, lowercased; empty when the media left it untagged."`
	Title    string `json:"title,omitempty" doc:"Free-text stream title when the media carries one (e.g. 'Commentary')."`
}

type channelTracksOutput struct {
	Body struct {
		// Audio and Subtitles are what the AIRING programme actually carries — the real menu the
		// pickers offer, from an ffprobe of the media, not a hardcoded list.
		Audio     []TrackDTO `json:"audio"`
		Subtitles []TrackDTO `json:"subtitles"`
	}
}

func (s *Server) channelTracks(ctx context.Context, in *channelIDInput) (*channelTracksOutput, error) {
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	out := &channelTracksOutput{}
	// Always non-nil slices so the JSON is `[]` not `null` — the FE maps over them directly.
	out.Body.Audio = []TrackDTO{}
	out.Body.Subtitles = []TrackDTO{}

	// No resolver wired (unit installs, no media server) ⇒ empty lists, not an error: the pickers
	// then show only the channel's current default, which is a valid degraded state.
	if s.playoutResolver == nil {
		return out, nil
	}

	tracks, err := s.playoutResolver.Tracks(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		// A resolve error that isn't "not found" (a Tunarr/library hiccup) must not blank the
		// Watch tab: degrade to empty, exactly like the probe-failure path inside Tracks.
		return out, nil
	}
	for _, t := range tracks.Audio {
		out.Body.Audio = append(out.Body.Audio, TrackDTO{Index: t.Index, Language: t.Language, Title: t.Title})
	}
	for _, t := range tracks.Subtitles {
		out.Body.Subtitles = append(out.Body.Subtitles, TrackDTO{Index: t.Index, Language: t.Language, Title: t.Title})
	}
	return out, nil
}
