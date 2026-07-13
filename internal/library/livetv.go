package library

import (
	"context"
	"fmt"
	"net/http"
)

// Live TV wiring (§6): register Tunarr as an M3U tuner + XMLTV guide source in
// the media server so Loomarr's channels appear in the family guide. This is
// ONE-TIME wiring, never per-channel, and it is idempotent — enumerate existing
// tuners/providers first and no-op if Tunarr is already registered (§6, a
// Phase-10 gate). Duplicate tuners are a classic Emby mess.
//
// VERSION FRAGILITY (§6): the exact accepted payload fields and the guide-refresh
// task id drift across Emby/Jellyfin versions. These shapes are written against
// the maintainer-supervised Phase-10 live capture pinned in
// internal/testkit/fixtures/livetv/; if a capture contradicts these, update the
// design doc §6 first, then this file (CLAUDE.md: fixtures are pinned truth).

// LiveTV is the wiring capability the setup flow depends on (§6/§7). Implemented
// by the media-server Client; abstracted so the API layer and tests don't couple
// to the HTTP details.
type LiveTV interface {
	// TunerRegistered reports whether a tuner host already points at the given
	// Tunarr playlist URL (the idempotency check — §6 enumerate-first).
	TunerRegistered(ctx context.Context, tunarrM3U string) (bool, error)
	// ListingRegistered reports whether an XMLTV listing provider already points
	// at the given Tunarr guide URL.
	ListingRegistered(ctx context.Context, tunarrXMLTV string) (bool, error)
	// AddTuner registers Tunarr's M3U playlist as an m3u tuner host.
	AddTuner(ctx context.Context, tunarrM3U string) error
	// AddListingProvider registers Tunarr's XMLTV guide as an xmltv listing
	// provider.
	AddListingProvider(ctx context.Context, tunarrXMLTV string) error
	// RefreshGuide triggers the guide-refresh scheduled task (§9 guide freshness).
	// Best-effort; the task id is version-fragile (pinned via capture).
	RefreshGuide(ctx context.Context) error
}

// --- wire types (pinned to the Phase-10 capture; see fixtures/livetv/) ---

// tunerHost is a /LiveTv/TunerHosts entry. Only the fields the idempotency check
// and registration need are modeled.
type tunerHost struct {
	ID   string `json:"Id,omitempty"`
	Type string `json:"Type"` // "m3u"
	URL  string `json:"Url"`
}

// listingProvider is a /LiveTv/ListingProviders entry.
type listingProvider struct {
	ID   string `json:"Id,omitempty"`
	Type string `json:"Type"` // "xmltv"
	Path string `json:"Path"` // Emby/Jellyfin use Path for the xmltv URL
}

// TunerRegistered enumerates tuner hosts and reports whether one already targets
// the Tunarr M3U (§6 enumerate-first idempotency).
func (c *Client) TunerRegistered(ctx context.Context, tunarrM3U string) (bool, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/LiveTv/TunerHosts", nil)
	if err != nil {
		return false, err
	}
	c.flavor.applyTokenAuth(req, c.token, c.deviceID)
	var hosts []tunerHost
	if err := c.do(req, &hosts); err != nil {
		return false, err
	}
	for _, h := range hosts {
		if h.URL == tunarrM3U {
			return true, nil
		}
	}
	return false, nil
}

// ListingRegistered enumerates listing providers and reports whether one already
// targets the Tunarr XMLTV guide.
func (c *Client) ListingRegistered(ctx context.Context, tunarrXMLTV string) (bool, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/LiveTv/ListingProviders", nil)
	if err != nil {
		return false, err
	}
	c.flavor.applyTokenAuth(req, c.token, c.deviceID)
	var providers []listingProvider
	if err := c.do(req, &providers); err != nil {
		return false, err
	}
	for _, p := range providers {
		if p.Path == tunarrXMLTV {
			return true, nil
		}
	}
	return false, nil
}

// AddTuner registers Tunarr as an m3u tuner host (§6 — M3U preferred over
// HDHomeRun emulation).
func (c *Client) AddTuner(ctx context.Context, tunarrM3U string) error {
	body := tunerHost{Type: "m3u", URL: tunarrM3U}
	req, err := c.newJSONRequest(ctx, http.MethodPost, "/LiveTv/TunerHosts", body)
	if err != nil {
		return err
	}
	c.flavor.applyTokenAuth(req, c.token, c.deviceID)
	return c.do(req, nil)
}

// AddListingProvider registers Tunarr's XMLTV guide as an xmltv listing provider.
func (c *Client) AddListingProvider(ctx context.Context, tunarrXMLTV string) error {
	body := listingProvider{Type: "xmltv", Path: tunarrXMLTV}
	req, err := c.newJSONRequest(ctx, http.MethodPost, "/LiveTv/ListingProviders", body)
	if err != nil {
		return err
	}
	c.flavor.applyTokenAuth(req, c.token, c.deviceID)
	return c.do(req, nil)
}

// Client satisfies the LiveTV capability.
var _ LiveTV = (*Client)(nil)

// guideRefreshTaskID is the media server's "Refresh Guide" scheduled-task id.
// VERSION-FRAGILE (§6): this value is pinned from the Phase-10 live capture. Emby
// 4.10 / Jellyfin share the lineage; if a capture shows a different id, update
// the design doc first, then this constant.
const guideRefreshTaskID = "6432c1a6d4e2f8b90c1e5a7d3f2b8c4e"

// RefreshGuide triggers the guide-refresh scheduled task (§9). Best-effort — the
// caller treats any error as degraded freshness, never a hard failure.
func (c *Client) RefreshGuide(ctx context.Context) error {
	path := fmt.Sprintf("/ScheduledTasks/Running/%s", guideRefreshTaskID)
	req, err := c.newRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return err
	}
	c.flavor.applyTokenAuth(req, c.token, c.deviceID)
	return c.do(req, nil)
}
