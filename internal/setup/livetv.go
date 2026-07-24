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
	TunerAdded     bool  // an m3u tuner host was registered this call
	ListingAdded   bool  // an xmltv listing provider was registered this call
	TunerRemoved   int   // stale Loomarr tuner hosts retired this call (URL change)
	ListingRemoved int   // stale Loomarr listing providers retired this call
	Poked          bool  // a rescan+refresh was attempted (something changed)
	PokeErr        error // best-effort poke failure; the connect still succeeded
}

// AlreadyWired reports whether nothing needed to change — the idempotent no-op
// case the Phase-10 gate asserts on the second call. A URL-change reconcile that
// retired a stale tuner is NOT a no-op, so the removals count too.
func (r ConnectResult) AlreadyWired() bool {
	return !r.TunerAdded && !r.ListingAdded && r.TunerRemoved == 0 && r.ListingRemoved == 0
}

// Connect wires Tunarr as an M3U tuner + XMLTV guide in the media server,
// idempotently and self-healing on a URL change (§6): it enumerates first and only
// registers what's missing, so a second call with the SAME Tunarr URL is a no-op.
// When the Tunarr URL has CHANGED, it also removes the Loomarr-owned tuner/listing
// left pointing at the old URL (identity = FriendlyName "loomarr") before adding
// the new — so a repoint moves the tuner instead of orphaning a dead one. A tuner
// the household added by hand is never touched (§9 ownership).
func (c *LiveTVConnector) Connect(ctx context.Context) (ConnectResult, error) {
	var res ConnectResult

	// Retire Loomarr-owned tuners pointing at a stale URL first, so we never leave a
	// dead tuner beside the live one (the "classic Emby mess"). Done before the add
	// so the desired tuner is the only Loomarr one when we finish.
	staleTuners, err := c.lib.StaleLoomarrTuners(ctx, c.urls.M3U)
	if err != nil {
		return res, fmt.Errorf("enumerate stale tuner hosts: %w", err)
	}
	for _, id := range staleTuners {
		if err := c.lib.RemoveTuner(ctx, id); err != nil {
			return res, fmt.Errorf("remove stale tuner %s: %w", id, err)
		}
		res.TunerRemoved++
	}

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

	staleListings, err := c.lib.StaleLoomarrListings(ctx, c.urls.XMLTV)
	if err != nil {
		return res, fmt.Errorf("enumerate stale listing providers: %w", err)
	}
	for _, id := range staleListings {
		if err := c.lib.RemoveListingProvider(ctx, id); err != nil {
			return res, fmt.Errorf("remove stale listing provider %s: %w", id, err)
		}
		res.ListingRemoved++
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

	// If anything changed (a fresh tuner was registered, or a stale one retired), poke
	// the media server so the new tuner's channels are discovered and their EPG filled
	// NOW rather than at the next nightly scan (§9). A newly-registered M3U tuner has
	// no channels in the media server's view until it re-reads the playlist — the
	// re-scan surfaces the channel LIST, the refresh fills the guide DATA. Both are
	// best-effort: freshness degrades on failure, but the wiring itself already
	// succeeded, so a poke error must not fail Connect (§6). A no-op connect skips
	// them — nothing new to discover.
	if !res.AlreadyWired() {
		res.Poked = true
		if err := c.lib.RescanTuner(ctx, c.urls.M3U); err != nil {
			res.PokeErr = fmt.Errorf("rescan tuner: %w", err)
		}
		if err := c.lib.RefreshGuide(ctx); err != nil {
			// Keep the first poke error if the rescan already failed; either way
			// the connect succeeds and the caller can log degraded freshness.
			if res.PokeErr == nil {
				res.PokeErr = fmt.Errorf("refresh guide: %w", err)
			}
		}
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
// server's guide-refresh task after an existing channel's lineup changed.
// Best-effort.
func (c *LiveTVConnector) PokeGuideRefresh(ctx context.Context) error {
	return c.lib.RefreshGuide(ctx)
}

// RescanTuner implements channels.GuidePoker (§9): make the media server re-read
// the tuner channel list so a newly-created channel is discovered. Best-effort.
func (c *LiveTVConnector) RescanTuner(ctx context.Context) error {
	return c.lib.RescanTuner(ctx, c.urls.M3U)
}
