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
	// RescanTuner forces the media server to re-read the M3U tuner's channel list
	// so a newly-created channel is discovered (§9). RefreshGuide alone won't
	// surface a new channel. Best-effort; no-op if the tuner isn't registered.
	RescanTuner(ctx context.Context, tunarrM3U string) error
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

// liveTVConfigPath is the ONE read that enumerates both tuners and listing
// providers, and the only one that works on both flavors.
//
// The Emby-lineage `GET /LiveTv/TunerHosts` / `GET /LiveTv/ListingProviders` are
// WRITE-ONLY on Jellyfin: POST works, GET returns 405 (verified against Jellyfin
// 10.10.3 — see fixtures/livetv/FINDINGS.md). Reading through them meant the
// enumerate-first idempotency check errored on every Jellyfin install, so a connect
// either failed outright or re-registered the tuner on each attempt. §6 has always
// claimed both flavors; the Phase-10 capture was Emby-only, which is how it went
// unnoticed until the maintainer smoke wired a real Jellyfin.
const liveTVConfigPath = "/System/Configuration/livetv"

// liveTVConfig is the media server's Live TV configuration. Both flavors answer 200
// here and both return these two collections; everything else in the payload is
// recording/padding settings we neither read nor write.
type liveTVConfig struct {
	TunerHosts       []tunerHost       `json:"TunerHosts"`
	ListingProviders []listingProvider `json:"ListingProviders"`
}

// liveTVConfigRaw is the same read kept as generic maps, for RescanTuner — it must
// re-POST a host VERBATIM (Id, DeviceId, and any field we don't model) or the media
// server treats the write as a new registration rather than an update.
type liveTVConfigRaw struct {
	TunerHosts []map[string]any `json:"TunerHosts"`
}

func (c *Client) liveTVConfig(ctx context.Context, into any) error {
	req, err := c.newRequest(ctx, http.MethodGet, liveTVConfigPath, nil)
	if err != nil {
		return err
	}
	c.flavor.applyTokenAuth(req, c.token(), c.deviceID)
	return c.do(req, into)
}

// TunerRegistered enumerates tuner hosts and reports whether one already targets
// the Tunarr M3U (§6 enumerate-first idempotency).
func (c *Client) TunerRegistered(ctx context.Context, tunarrM3U string) (bool, error) {
	var cfg liveTVConfig
	if err := c.liveTVConfig(ctx, &cfg); err != nil {
		return false, err
	}
	for _, h := range cfg.TunerHosts {
		if h.URL == tunarrM3U {
			return true, nil
		}
	}
	return false, nil
}

// ListingRegistered enumerates listing providers and reports whether one already
// targets the Tunarr XMLTV guide.
func (c *Client) ListingRegistered(ctx context.Context, tunarrXMLTV string) (bool, error) {
	var cfg liveTVConfig
	if err := c.liveTVConfig(ctx, &cfg); err != nil {
		return false, err
	}
	for _, p := range cfg.ListingProviders {
		if p.Path == tunarrXMLTV {
			return true, nil
		}
	}
	return false, nil
}

// RescanTuner forces the media server to re-read the M3U tuner's channel list so
// a newly-created (or removed) Loomarr channel is discovered (§6/§9). A guide
// refresh alone updates EPG for KNOWN channels and won't surface a new one; on
// Emby/Jellyfin, re-POSTing the existing M3U tuner host triggers the re-read.
// Best-effort: no matching host (not wired yet) is a no-op, not an error.
//
// The full host object is fetched and re-POSTed verbatim (as a generic map, to
// preserve fields we don't model — Id, DeviceId, etc.) so the media server treats
// it as an update of the same host rather than a new registration.
func (c *Client) RescanTuner(ctx context.Context, tunarrM3U string) error {
	var cfg liveTVConfigRaw
	if err := c.liveTVConfig(ctx, &cfg); err != nil {
		return err
	}
	for _, h := range cfg.TunerHosts {
		if url, _ := h["Url"].(string); url == tunarrM3U {
			post, err := c.newJSONRequest(ctx, http.MethodPost, "/LiveTv/TunerHosts", h)
			if err != nil {
				return err
			}
			c.flavor.applyTokenAuth(post, c.token(), c.deviceID)
			return c.do(post, nil)
		}
	}
	return nil // tuner not registered (not wired) → nothing to re-scan
}

// AddTuner registers Tunarr as an m3u tuner host (§6 — M3U preferred over
// HDHomeRun emulation).
func (c *Client) AddTuner(ctx context.Context, tunarrM3U string) error {
	body := tunerHost{Type: "m3u", URL: tunarrM3U}
	req, err := c.newJSONRequest(ctx, http.MethodPost, "/LiveTv/TunerHosts", body)
	if err != nil {
		return err
	}
	c.flavor.applyTokenAuth(req, c.token(), c.deviceID)
	return c.do(req, nil)
}

// AddListingProvider registers Tunarr's XMLTV guide as an xmltv listing provider.
func (c *Client) AddListingProvider(ctx context.Context, tunarrXMLTV string) error {
	body := listingProvider{Type: "xmltv", Path: tunarrXMLTV}
	req, err := c.newJSONRequest(ctx, http.MethodPost, "/LiveTv/ListingProviders", body)
	if err != nil {
		return err
	}
	c.flavor.applyTokenAuth(req, c.token(), c.deviceID)
	return c.do(req, nil)
}

// Client satisfies the LiveTV capability.
var _ LiveTV = (*Client)(nil)

// guideRefreshTaskKey is the media server's "Refresh Guide" scheduled-task Key.
// Phase-10 live capture (Emby 4.10.0.17) finding: the task *Id* is per-install
// and version-fragile, but the *Key* is the stable "RefreshGuide". The run
// endpoint takes the Id (the Key form 404s), so RefreshGuide resolves the id by
// this Key at runtime. See fixtures/livetv/FINDINGS.md.
const guideRefreshTaskKey = "RefreshGuide"

// scheduledTask is the slice of a /ScheduledTasks entry we need to resolve the
// guide-refresh task id from its stable Key (Phase-10 finding 4).
type scheduledTask struct {
	ID  string `json:"Id"`
	Key string `json:"Key"`
}

// RefreshGuide triggers the guide-refresh scheduled task (§9). Best-effort — the
// caller treats any error as degraded freshness, never a hard failure. It first
// resolves the per-install task id by the stable Key "RefreshGuide" (Phase-10
// finding 4), then POSTs /ScheduledTasks/Running/<id>.
func (c *Client) RefreshGuide(ctx context.Context) error {
	id, err := c.guideRefreshTaskID(ctx)
	if err != nil {
		return err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/ScheduledTasks/Running/"+id, nil)
	if err != nil {
		return err
	}
	c.flavor.applyTokenAuth(req, c.token(), c.deviceID)
	return c.do(req, nil)
}

// guideRefreshTaskID resolves the guide-refresh task's per-install id by its
// stable Key (Phase-10 finding 4).
func (c *Client) guideRefreshTaskID(ctx context.Context) (string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/ScheduledTasks", nil)
	if err != nil {
		return "", err
	}
	c.flavor.applyTokenAuth(req, c.token(), c.deviceID)
	var tasks []scheduledTask
	if err := c.do(req, &tasks); err != nil {
		return "", err
	}
	for _, t := range tasks {
		if t.Key == guideRefreshTaskKey {
			return t.ID, nil
		}
	}
	return "", fmt.Errorf("guide-refresh task (Key=%q) not found on the media server", guideRefreshTaskKey)
}
