package api

import (
	"context"
	"net/http"

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

	// Note: there is no standalone livetv-connect route. Live TV wiring is idempotent and
	// fully derived from the Tunarr connection, so it auto-runs on a Connections save
	// (settings.go autoWireAfterSave) — a separate manual endpoint would be a redundant
	// no-op. The wiring STATUS still surfaces via the `livetv` setup check above.

	huma.Register(api, huma.Operation{
		OperationID: "tunarr-connect", Method: http.MethodPost, Path: "/v1/setup/tunarr-connect",
		Summary: "Wire the library as Tunarr's media source", Description: "Admin only. Ensures Tunarr's Emby/Jellyfin source (Loomarr's admin token) + enables/scans the movie/show libraries. One-time, idempotent (§6).",
		Tags: []string{"setup"},
	}, s.tunarrConnectHandler)

	huma.Register(api, huma.Operation{
		OperationID: "livetv-reconnect", Method: http.MethodPost, Path: "/v1/setup/livetv-reconnect",
		Summary: "Force-re-wire the Tunarr tuner", Description: "Admin only. Removes + re-adds Loomarr's Tunarr tuner in the media server and re-scans it, clearing a STALE channel→stream binding (the media server streaming a since-deleted channel id — 'guide right, plays wrong'). Use when a channel plays the wrong content in the media server but is correct in Tunarr (§6/§9).",
		Tags: []string{"setup"},
	}, s.livetvReconnectHandler)
}

// SetupCheck is one integration's checklist result (§13).
type SetupCheck struct {
	Name string `json:"name" example:"livetv" doc:"Integration key"`
	OK   bool   `json:"ok"`
	Hint string `json:"hint,omitempty" doc:"Actionable hint when not OK (§13)"`
	// DocHref deep-links this check to its Troubleshooting section so a red check
	// is one click from the exact fix (§13 "failures never blame").
	DocHref string `json:"docHref,omitempty" doc:"Anchor into the embedded Troubleshooting docs"`
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
			check.Hint = "Couldn't reach the media server's Live TV settings. Check the media-server connection."
		} else if !wired {
			// Wiring is automatic on a Tunarr-connection save (config-design §6), so the fix
			// is to (re)save that connection, not to run a separate action.
			check.Hint = "Tunarr isn't registered as a tuner + guide yet. Re-save the Tunarr connection to wire it up."
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

	return out, nil
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
		return nil, errNotImplemented("Setup incomplete",
			"Connect Tunarr and your media server in Settings before wiring them together.",
			"troubleshooting#tunarr")
	}
	sourceID, enabled, err := s.tunarrConnect.Connect(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't wire Tunarr",
			"Loomarr couldn't set your media server up as Tunarr's source. Check that both are reachable and try again.", err)
	}
	out := &tunarrConnectOutput{}
	out.Body.SourceID, out.Body.LibrariesEnabled = sourceID, enabled
	return out, nil
}

type livetvReconnectOutput struct {
	Body struct {
		TunersReset int `json:"tunersReset" doc:"Loomarr tuners removed + re-added in the media server"`
	}
}

// livetvReconnectHandler force-re-wires the Tunarr tuner (§6): remove + re-add +
// re-scan, to drop a stale channel→stream binding the media server holds after a
// channel was deleted (its guide is fresh, but playback resolves to a dead/other
// stream). Distinct from the idempotent auto-wire, which leaves a current-URL tuner
// untouched and so can't fix this.
func (s *Server) livetvReconnectHandler(ctx context.Context, _ *struct{}) (*livetvReconnectOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.livetv == nil || s.unconfigured("tunarr.url") {
		return nil, errNotImplemented("Live TV isn't set up",
			"Connect Tunarr in Settings before re-wiring the tuner.", "troubleshooting#livetv")
	}
	reset, err := s.livetv.Reconnect(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't re-wire the tuner",
			"Loomarr couldn't re-register its Tunarr tuner in your media server. Check that both are reachable and try again.", err)
	}
	out := &livetvReconnectOutput{}
	out.Body.TunersReset = reset
	return out, nil
}

// configInt reads a live INTEGER setting. It deliberately does NOT parse configValue:
// settings.String panics on a non-string key, so routing an int setting through the
// string seam is a 500 waiting to happen — which is exactly what it was. Unwired ⇒ 0,
// which callers treat as "no configured value" rather than as a real limit of zero.
func (s *Server) configInt(key string) int {
	if s.liveConfigInt == nil {
		return 0
	}
	return s.liveConfigInt(key)
}
