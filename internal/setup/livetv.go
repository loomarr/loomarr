// Package setup owns the operator connection flows (§7, §13): the Live TV wiring
// (POST /v1/setup/livetv-connect) and the setup-status checklist. It composes the
// media-server LiveTV capability (§6) and derives Tunarr's playlist/guide URLs;
// it never talks to Tunarr directly (Tunarr publishes those URLs; the media
// server consumes them).
package setup

import (
	"context"
	"fmt"
	"strings"

	"github.com/mantonx/loomarr/internal/library"
)

// TunarrURLs are the M3U playlist + XMLTV guide URLs Tunarr publishes. Both flow
// from TUNARR_URL (§15); Tunarr serves the playlist at /api/channels.m3u and the
// guide at /api/xmltv.xml (Tunarr 1.3.8, pinned Phase 0).
type TunarrURLs struct {
	M3U   string
	XMLTV string
}

// TunarrURLsFrom derives the tuner + guide URLs from the Tunarr base URL.
func TunarrURLsFrom(tunarrBaseURL string) TunarrURLs {
	base := strings.TrimRight(tunarrBaseURL, "/")
	return TunarrURLs{
		M3U:   base + "/api/channels.m3u",
		XMLTV: base + "/api/xmltv.xml",
	}
}

// LiveTVConnector performs the one-time, idempotent Live TV wiring (§6). It is
// the POST /v1/setup/livetv-connect body and also backs the wizard's one-click
// connect (§13).
type LiveTVConnector struct {
	lib  library.LiveTV
	urls TunarrURLs
}

// NewLiveTVConnector wires the connector to the media-server LiveTV capability
// and the Tunarr URLs.
func NewLiveTVConnector(lib library.LiveTV, urls TunarrURLs) *LiveTVConnector {
	return &LiveTVConnector{lib: lib, urls: urls}
}

// ConnectResult reports what the connect did — so the API/UI can distinguish
// "just wired it" from "already wired" (§6 second-call-no-op) without guessing.
type ConnectResult struct {
	TunerAdded   bool // an m3u tuner host was registered this call
	ListingAdded bool // an xmltv listing provider was registered this call
}

// AlreadyWired reports whether nothing needed to change — the idempotent no-op
// case the Phase-10 gate asserts on the second call.
func (r ConnectResult) AlreadyWired() bool { return !r.TunerAdded && !r.ListingAdded }

// Connect wires Tunarr as an M3U tuner + XMLTV guide in the media server,
// idempotently (§6): it enumerates first and only registers what's missing, so a
// second call with Tunarr already registered is a no-op. This is the guard
// against duplicate tuners — a classic Emby mess.
func (c *LiveTVConnector) Connect(ctx context.Context) (ConnectResult, error) {
	var res ConnectResult

	tunerThere, err := c.lib.TunerRegistered(ctx, c.urls.M3U)
	if err != nil {
		return res, fmt.Errorf("enumerate tuner hosts: %w", err)
	}
	if !tunerThere {
		if err := c.lib.AddTuner(ctx, c.urls.M3U); err != nil {
			return res, fmt.Errorf("register tuner: %w", err)
		}
		res.TunerAdded = true
	}

	listingThere, err := c.lib.ListingRegistered(ctx, c.urls.XMLTV)
	if err != nil {
		return res, fmt.Errorf("enumerate listing providers: %w", err)
	}
	if !listingThere {
		if err := c.lib.AddListingProvider(ctx, c.urls.XMLTV); err != nil {
			return res, fmt.Errorf("register listing provider: %w", err)
		}
		res.ListingAdded = true
	}

	return res, nil
}

// Wired reports whether Tunarr is already registered as both a tuner and a guide
// — the "media server has Tunarr wired" check for GET /v1/setup/status (§7/§6).
func (c *LiveTVConnector) Wired(ctx context.Context) (bool, error) {
	tuner, err := c.lib.TunerRegistered(ctx, c.urls.M3U)
	if err != nil {
		return false, err
	}
	listing, err := c.lib.ListingRegistered(ctx, c.urls.XMLTV)
	if err != nil {
		return false, err
	}
	return tuner && listing, nil
}

// PokeGuideRefresh implements channels.GuidePoker (§9): trigger the media
// server's guide-refresh task after a channel-affecting reconcile. Best-effort.
func (c *LiveTVConnector) PokeGuideRefresh(ctx context.Context) error {
	return c.lib.RefreshGuide(ctx)
}
