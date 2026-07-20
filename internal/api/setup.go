package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
)

// registerSetup mounts /v1/setup/* (§7/§13). These power the operator wizard +
// Settings troubleshooting; all are admin-only.
func (s *Server) registerSetup(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "setup-status", Method: http.MethodGet, Path: "/v1/setup/status",
		Summary: "Connection checklist", Description: "Admin only. Per-integration pass/fail (§13).",
		Tags: []string{"setup"},
	}, s.setupStatus)

	huma.Register(api, huma.Operation{
		OperationID: "livetv-connect", Method: http.MethodPost, Path: "/v1/setup/livetv-connect",
		Summary: "Wire Tunarr as a tuner + guide", Description: "Admin only. One-time, idempotent (§6).",
		Tags: []string{"setup"},
	}, s.livetvConnect)

	huma.Register(api, huma.Operation{
		OperationID: "tunarr-connect", Method: http.MethodPost, Path: "/v1/setup/tunarr-connect",
		Summary: "Wire the library as Tunarr's media source", Description: "Admin only. Ensures Tunarr's Emby/Jellyfin source (Loomarr's admin token) + enables/scans the movie/show libraries. One-time, idempotent (§6).",
		Tags: []string{"setup"},
	}, s.tunarrConnectHandler)
}

// SetupCheck is one integration's checklist result (§13).
type SetupCheck struct {
	Name string `json:"name" example:"livetv" doc:"Integration key"`
	OK   bool   `json:"ok"`
	Hint string `json:"hint,omitempty" doc:"Actionable hint when not OK (§13)"`
	// DocHref deep-links this check to its Troubleshooting section so a red check
	// is one click from the exact fix (§13 "failures never blame").
	DocHref string `json:"docHref,omitempty" doc:"Anchor into the embedded Troubleshooting docs"`
	// LastReceived carries per-app (sonarr/radarr) webhook receipt timestamps
	// (RFC3339). Set only on the `webhook` check — powers the wizard's live
	// handshake ("listening… → green on receipt", §13).
	LastReceived map[string]string `json:"lastReceived,omitempty" doc:"Per-app webhook last-received (webhook check only)"`
}

// connectionChecklist is the ordered set of connection probes surfaced by
// setup/status, reusing the §8 setup/test registry (SettingsService.Test). Each
// maps a probe name to its Troubleshooting anchor. `filler` is optional — shown
// only when a filler dir is configured (§13 "if configured").
var connectionChecklist = []struct{ name, docHref string }{
	{"media_server", "troubleshooting#media-server"},
	{"requester", "troubleshooting#seerr"},
	{"tunarr", "troubleshooting#tunarr"},
	{"llm", "troubleshooting#llm"},
	{"tmdb", "troubleshooting#tmdb"},
	{"filler", "troubleshooting#filler"},
}

type setupStatusOutput struct {
	Body struct {
		Checks []SetupCheck `json:"checks"`
	}
}

// setupStatus runs the full connection checklist (§13): the connection probes
// (reusing the §8 setup/test registry), the Tunarr wiring checks, and the webhook
// handshake. Every dependency the wizard needs is one structured call — each
// result carries an actionable hint + a Troubleshooting deep-link. Services that
// aren't wired (unit tests, unconfigured installs) contribute no check rather than
// a false failure.
func (s *Server) setupStatus(ctx context.Context, _ *struct{}) (*setupStatusOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	out := &setupStatusOutput{}

	// Connection probes (media_server, requester, tunarr, llm, tmdb, filler) via
	// the shared registry. filler is optional — shown only when configured.
	if s.settings != nil {
		for _, c := range connectionChecklist {
			if c.name == "filler" && s.configValue("filler.dir") == "" {
				continue
			}
			ok, hint := s.settings.Test(ctx, c.name)
			out.Body.Checks = append(out.Body.Checks, SetupCheck{Name: c.name, OK: ok, Hint: hint, DocHref: c.docHref})
		}
	}

	// Tunarr wiring checks (§6): tuner+guide, and the media-source scan.
	if s.livetv != nil {
		wired, err := s.livetv.Wired(ctx)
		check := SetupCheck{Name: "livetv", OK: wired, DocHref: "troubleshooting#livetv"}
		if err != nil {
			check.OK = false
			check.Hint = "could not reach the media server's Live TV settings: " + err.Error()
		} else if !wired {
			check.Hint = "Tunarr is not wired as a tuner + guide — run Connect (POST /v1/setup/livetv-connect)"
		}
		out.Body.Checks = append(out.Body.Checks, check)
	}
	if s.tunarrConnect != nil {
		ready, err := s.tunarrConnect.LibrariesReady(ctx)
		// Its own anchor, not the general Tunarr one: this check fails for a specific and
		// silent reason (Tunarr's media source isn't scanned, so every slot degrades to
		// dead air while everything else reports healthy), and it deserves the section
		// that explains that rather than generic connectivity advice.
		check := SetupCheck{Name: "tunarr_library", OK: ready, DocHref: "troubleshooting#tunarr-library"}
		if err != nil {
			check.OK = false
			check.Hint = "could not reach Tunarr: " + err.Error()
		} else if !ready {
			check.Hint = "Tunarr can't see your library yet — run Connect (POST /v1/setup/tunarr-connect); otherwise channels air flex/dead-air"
		}
		out.Body.Checks = append(out.Body.Checks, check)
	}

	// Webhook handshake (§13): green once Sonarr/Radarr have each delivered at least
	// once, carrying per-app last-received so the wizard shows "listening…".
	out.Body.Checks = append(out.Body.Checks, s.webhookCheck(ctx))

	return out, nil
}

// webhookCheck reports the Sonarr/Radarr webhook handshake from the per-app
// last-received timestamps the ingest handler records (§13). OK once at least one
// app has delivered — the operator has proven the URL + secret are pasted right.
func (s *Server) webhookCheck(ctx context.Context) SetupCheck {
	check := SetupCheck{Name: "webhook", DocHref: "troubleshooting#webhooks"}
	if s.store == nil {
		check.Hint = "not configured"
		return check
	}
	last := map[string]string{}
	for _, app := range []string{"sonarr", "radarr"} {
		if v, err := s.store.GetSetting(ctx, "webhook_last_received:"+app); err == nil && v != "" {
			last[app] = v
		}
	}
	if len(last) > 0 {
		check.LastReceived = last
	}
	if len(last) == 0 {
		check.Hint = "waiting for a test webhook — click Test in Sonarr and Radarr; the checklist flips green on receipt"
		return check
	}
	check.OK = true
	return check
}

// configValue reads a live setting for gating optional checks; empty when the
// composition root didn't wire liveConfig (unit tests) — then optional checks are
// simply omitted.
func (s *Server) configValue(key string) string {
	if s.liveConfig == nil {
		return ""
	}
	return s.liveConfig(key)
}

type tunarrConnectOutput struct {
	Body struct {
		SourceID         string `json:"sourceId" doc:"Tunarr media-source id (existing or newly created)"`
		LibrariesEnabled int    `json:"librariesEnabled" doc:"Movie/show libraries now enabled + scanning"`
	}
}

// tunarrConnectHandler wires the media server as Tunarr's media source + scans it
// (§6). Idempotent — re-running reuses the source and skips already-enabled libraries.
func (s *Server) tunarrConnectHandler(ctx context.Context, _ *struct{}) (*tunarrConnectOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.tunarrConnect == nil || s.unconfigured("tunarr.url", "library.url") {
		return nil, huma.Error501NotImplemented("Tunarr + media server must be configured first")
	}
	sourceID, enabled, err := s.tunarrConnect.Connect(ctx)
	if err != nil {
		return nil, huma.Error502BadGateway("wiring Tunarr's media source failed", err)
	}
	out := &tunarrConnectOutput{}
	out.Body.SourceID, out.Body.LibrariesEnabled = sourceID, enabled
	return out, nil
}

type livetvConnectOutput struct {
	Body struct {
		TunerAdded   bool `json:"tunerAdded" doc:"An m3u tuner was registered this call"`
		ListingAdded bool `json:"listingAdded" doc:"An xmltv guide was registered this call"`
		AlreadyWired bool `json:"alreadyWired" doc:"Nothing changed — Tunarr was already wired (§6 idempotent)"`
	}
}

// livetvConnect performs the one-time, idempotent Live TV wiring (§6). A second
// call with Tunarr already registered is a no-op (alreadyWired=true).
func (s *Server) livetvConnect(ctx context.Context, _ *struct{}) (*livetvConnectOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.livetv == nil || s.unconfigured("tunarr.url") {
		return nil, huma.Error501NotImplemented("Live TV wiring not configured")
	}
	tunerAdded, listingAdded, err := s.livetv.Connect(ctx)
	if err != nil {
		return nil, huma.Error502BadGateway("Live TV wiring failed", err)
	}
	out := &livetvConnectOutput{}
	out.Body.TunerAdded = tunerAdded
	out.Body.ListingAdded = listingAdded
	out.Body.AlreadyWired = !tunerAdded && !listingAdded
	return out, nil
}

// configInt reads a live integer setting. Unparseable or unwired ⇒ 0, which callers
// treat as "no configured value" rather than as a real limit of zero.
func (s *Server) configInt(key string) int {
	n, err := strconv.Atoi(s.configValue(key))
	if err != nil {
		return 0
	}
	return n
}
