// Package setup owns the operator connection flows (§7, §13): the Live TV wiring
// (POST /v1/setup/livetv-connect) and the setup-status checklist. It composes the
// media-server LiveTV capability (§6) and derives Tunarr's playlist/guide URLs;
// it never talks to Tunarr directly (Tunarr publishes those URLs; the media
// server consumes them).
package setup

import (
	"context"
	"fmt"
	"net/url"
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

// InternalPlayoutURLs derives the tuner + guide URLs Loomarr serves ITSELF (§9.1).
//
// ⚠ WHICH BACKEND STREAMS DECIDES WHICH URLs THE MEDIA SERVER GETS. `playout.backend`
// defaults to `internal`, meaning Loomarr encodes the channels — but the Live TV wiring
// registered Tunarr's `/api/channels.m3u` unconditionally, so the media server was pointed at
// a backend that was not serving those channels. Reported symptom: the channels appear in
// Emby's guide and will not play, and a `livetv-reconnect` "fixes" it by re-registering the
// same wrong URLs. design.md §9.1 item 3 called for exactly this and the code never did it.
//
// Both URLs carry the device token as a query parameter, because the media server fetches them
// unauthenticated from a background job — the same reason /playout/* takes `?token=` at all
// (§11: this authenticates a DEVICE, not a person).
//
// An empty publicURL yields empty URLs, and the caller MUST treat that as "not wireable" rather
// than registering a relative path: the media server resolves the URL from its own host, so a
// blank or relative base silently points it at itself. That is the `server.public_url` failure
// mode the playout handlers already guard.
func InternalPlayoutURLs(publicURL, deviceToken string) TunarrURLs {
	base := strings.TrimRight(strings.TrimSpace(publicURL), "/")
	if base == "" {
		return TunarrURLs{}
	}
	q := ""
	if deviceToken != "" {
		q = "?token=" + url.QueryEscape(deviceToken)
	}
	return TunarrURLs{
		M3U:   base + "/playout/tuner.m3u" + q,
		XMLTV: base + "/playout/guide.xml" + q,
	}
}

// LiveTVURLsFor picks the URLs for the backend that will actually stream (§9.1).
//
// `internal` (the default) ⇒ Loomarr's own endpoints; `tunarr` ⇒ Tunarr's. Anything else falls
// back to Tunarr, which is the pre-§9.1 behaviour and the safer default for an unrecognised
// value: an install that has not opted into internal playout keeps working exactly as it did.
func LiveTVURLsFor(backend, tunarrBaseURL, publicURL, deviceToken string) TunarrURLs {
	if strings.TrimSpace(backend) == "internal" {
		return InternalPlayoutURLs(publicURL, deviceToken)
	}
	return TunarrURLsFrom(tunarrBaseURL)
}

// LiveTVConnector performs the one-time, idempotent Live TV wiring (§6). It is
// the POST /v1/setup/livetv-connect body and also backs the wizard's one-click
// connect (§13).
type LiveTVConnector struct {
	lib library.LiveTV
	// urls is resolved PER CALL, not captured at construction: `playout.backend`,
	// `tunarr.url` and `server.public_url` all hot-apply (config-design §3), and a connector
	// holding a snapshot would keep wiring the media server to the backend that was configured
	// at boot. A closure is what makes "switch to internal playout, save, re-connect" work
	// without a restart.
	urls func() TunarrURLs
}

// NewLiveTVConnector wires the connector to the media-server LiveTV capability
// and the Tunarr URLs.
func NewLiveTVConnector(lib library.LiveTV, urls func() TunarrURLs) *LiveTVConnector {
	return &LiveTVConnector{lib: lib, urls: urls}
}

// NewLiveTVConnectorFixed wires a connector to a fixed URL pair — for tests and any caller with
// no live settings to read. Production uses NewLiveTVConnector so a settings change applies
// without a restart.
func NewLiveTVConnectorFixed(lib library.LiveTV, urls TunarrURLs) *LiveTVConnector {
	return NewLiveTVConnector(lib, func() TunarrURLs { return urls })
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
	// Resolve ONCE per call, so every check below reasons about the same target even if a
	// setting changes mid-flight.
	urls := c.urls()
	if urls.M3U == "" || urls.XMLTV == "" {
		// Nothing wireable — internal playout with no server.public_url set. Registering a
		// relative path here would point the media server at ITSELF (it resolves the URL from
		// its own host), which looks wired and never plays.
		return ConnectResult{}, fmt.Errorf("live tv: no reachable playout URLs — set Loomarr's public address in Settings")
	}

	staleTuners, err := c.lib.StaleLoomarrTuners(ctx, urls.M3U)
	if err != nil {
		return res, fmt.Errorf("enumerate stale tuner hosts: %w", err)
	}
	for _, id := range staleTuners {
		if err := c.lib.RemoveTuner(ctx, id); err != nil {
			return res, fmt.Errorf("remove stale tuner %s: %w", id, err)
		}
		res.TunerRemoved++
	}

	tunerThere, err := c.lib.TunerRegistered(ctx, urls.M3U)
	if err != nil {
		return res, fmt.Errorf("enumerate tuner hosts: %w", err)
	}
	if !tunerThere {
		if err := c.lib.AddTuner(ctx, urls.M3U); err != nil {
			return res, fmt.Errorf("register tuner: %w", err)
		}
		res.TunerAdded = true
	}

	staleListings, err := c.lib.StaleLoomarrListings(ctx, urls.XMLTV)
	if err != nil {
		return res, fmt.Errorf("enumerate stale listing providers: %w", err)
	}
	for _, id := range staleListings {
		if err := c.lib.RemoveListingProvider(ctx, id); err != nil {
			return res, fmt.Errorf("remove stale listing provider %s: %w", id, err)
		}
		res.ListingRemoved++
	}

	listingThere, err := c.lib.ListingRegistered(ctx, urls.XMLTV)
	if err != nil {
		return res, fmt.Errorf("enumerate listing providers: %w", err)
	}
	if !listingThere {
		if err := c.lib.AddListingProvider(ctx, urls.XMLTV); err != nil {
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
		if err := c.lib.RescanTuner(ctx, urls.M3U); err != nil {
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

// Reconnect FORCE-re-wires the tuner: it removes every Loomarr-owned tuner (even at
// the current URL, which Connect leaves alone) and re-adds it, then pokes a re-scan +
// guide refresh. This is the repair for a stale channel→stream binding — the media
// server keeps streaming a channel id Loomarr has since DELETED (its guide is fresh,
// but playback resolves to a dead/other stream: "guide right, plays wrong"). A normal
// Connect can't fix it (the tuner URL is unchanged, so nothing is stale); only a
// remove+re-add makes the media server re-read the M3U and rebind. Returns how many
// tuners were reset. Best-effort pokes, like Connect.
func (c *LiveTVConnector) Reconnect(ctx context.Context) (ConnectResult, error) {
	var res ConnectResult

	// Same live resolution as Connect — a reconnect after switching backends must re-wire to
	// the NEW backend, which is the repair an operator reaches for when channels won't play.
	urls := c.urls()
	if urls.M3U == "" || urls.XMLTV == "" {
		return res, fmt.Errorf("live tv: no reachable playout URLs — set Loomarr's public address in Settings")
	}

	tuners, err := c.lib.LoomarrTuners(ctx)
	if err != nil {
		return res, fmt.Errorf("enumerate loomarr tuners: %w", err)
	}
	for _, id := range tuners {
		if err := c.lib.RemoveTuner(ctx, id); err != nil {
			return res, fmt.Errorf("remove tuner %s: %w", id, err)
		}
		res.TunerRemoved++
	}
	if err := c.lib.AddTuner(ctx, urls.M3U); err != nil {
		return res, fmt.Errorf("re-add tuner: %w", err)
	}
	res.TunerAdded = true

	// Also re-wire the guide provider so both halves are freshly bound.
	staleListings, err := c.lib.StaleLoomarrListings(ctx, urls.XMLTV)
	if err == nil {
		for _, id := range staleListings {
			if rerr := c.lib.RemoveListingProvider(ctx, id); rerr == nil {
				res.ListingRemoved++
			}
		}
	}
	if there, lerr := c.lib.ListingRegistered(ctx, urls.XMLTV); lerr == nil && !there {
		if aerr := c.lib.AddListingProvider(ctx, urls.XMLTV); aerr == nil {
			res.ListingAdded = true
		}
	}

	// Poke so the re-added tuner's channels are re-read now (the whole point).
	res.Poked = true
	if err := c.lib.RescanTuner(ctx, urls.M3U); err != nil {
		res.PokeErr = fmt.Errorf("rescan tuner: %w", err)
	}
	if err := c.lib.RefreshGuide(ctx); err != nil && res.PokeErr == nil {
		res.PokeErr = fmt.Errorf("refresh guide: %w", err)
	}
	return res, nil
}

// Wired reports whether Tunarr is already registered as both a tuner and a guide
// — the "media server has Tunarr wired" check for GET /v1/setup/status (§7/§6).
func (c *LiveTVConnector) Wired(ctx context.Context) (bool, error) {
	urls := c.urls()
	if urls.M3U == "" || urls.XMLTV == "" {
		return false, nil // nothing wireable ⇒ not wired; the checklist reports it as such
	}
	tuner, err := c.lib.TunerRegistered(ctx, urls.M3U)
	if err != nil {
		return false, err
	}
	listing, err := c.lib.ListingRegistered(ctx, urls.XMLTV)
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
	urls := c.urls()
	if urls.M3U == "" {
		return nil // nothing to rescan; the poke is best-effort by contract (§9)
	}
	return c.lib.RescanTuner(ctx, urls.M3U)
}
