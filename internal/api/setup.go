package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// registerSetup mounts /v1/setup/* (§7/§13). These power the operator wizard +
// Settings troubleshooting; all are admin-only.
func (s *Server) registerSetup(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "setup-status", Method: http.MethodGet, Path: "/v1/setup/status",
		Summary: "Connection checklist", Description: "Admin only. Per-integration pass/fail (§13).",
		Tags: []string{"setup"},
	}, RoleAdmin), s.setupStatus)

	// Note: there is no standalone livetv-connect route. Live TV wiring is idempotent and
	// fully derived from the selected playout backend and connection settings, so it auto-runs
	// when those settings are saved
	// (settings.go mutateLiveTVSettings) — a separate manual endpoint would be a redundant
	// no-op. The wiring STATUS still surfaces via the `livetv` setup check above.

	huma.Register(api, withRole(huma.Operation{
		OperationID: "tunarr-connect", Method: http.MethodPost, Path: "/v1/setup/tunarr-connect",
		Summary: "Wire the library as Tunarr's media source", Description: "Admin only. Ensures Tunarr's Emby/Jellyfin source (Loomarr's admin token) + enables/scans the movie/show libraries. One-time, idempotent (§6).",
		Tags: []string{"setup"},
	}, RoleAdmin), s.tunarrConnectHandler)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "livetv-reconnect", Method: http.MethodPost, Path: "/v1/setup/livetv-reconnect",
		Summary: "Force-re-wire the Live TV tuner", Description: "Admin only. Removes + re-adds Loomarr's currently applied tuner in the media server and re-scans it, clearing a stale channel→stream binding ('guide right, plays wrong'). Works for internal or Tunarr playout and is serialized with backend transitions (§6/§9).",
		Tags: []string{"setup"},
	}, RoleAdmin), s.livetvReconnectHandler)
}

// SetupCheck is one integration's checklist result (§13).
type SetupCheck struct {
	Name string `json:"name" example:"livetv" doc:"Integration key"`
	OK   bool   `json:"ok"`
	Hint string `json:"hint,omitempty" doc:"Actionable hint when not OK (§13)"`
	// DocHref deep-links this check to its Troubleshooting section so a red check
	// is one click from the exact fix (§13 "failures never blame").
	DocHref string `json:"docHref,omitempty" doc:"Anchor into the embedded Troubleshooting docs"`
	// Target is what this check connected TO — the Services panel shows it beside the state
	// (§12, V31), because "media server: FAIL" without an address makes an operator go
	// looking for which one. Empty when the probe has no configurable endpoint (TMDB) or the
	// setting is unset.
	Target string `json:"target,omitempty" doc:"The URL (without credentials/query) or path this check probed"`
}

// connectionChecklist is the ordered set of connection probes surfaced by
// setup/status, reusing the §8 setup/test registry (SettingsService.Test). Each
// maps a probe name to its Troubleshooting anchor. `filler` is optional — shown
// only when a filler dir is configured (§13 "if configured").
// targetKey names the setting holding what this probe connects TO, so the Services panel
// (§12, V31) can show `emby.lan:8096` beside a red dot. Empty where there is nothing to
// show — TMDB is a fixed vendor endpoint nobody configures, so a "target" there would be a
// constant dressed up as diagnostics.
var connectionChecklist = []struct{ name, docHref, targetKey string }{
	{"media_server", "troubleshooting#media-server", "library.url"},
	{"requester", "troubleshooting#seerr", "seerr.url"},
	{"tunarr", "troubleshooting#tunarr", "tunarr.url"},
	{"llm", "troubleshooting#llm", "llm.url"},
	{"tmdb", "troubleshooting#tmdb", ""},
	{"filler", "troubleshooting#filler", "filler.dir"},
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
	out := &setupStatusOutput{}
	out.Body.Checks = s.runConnectionChecks(ctx)
	return out, nil
}

// runConnectionChecks probes every configured service and returns one check each.
//
// ⚠ Extracted so `POST /v1/system/reload` (§9.2, V13) runs THIS implementation rather
// than a second copy. A reload that disagreed with the wizard's checklist would send an
// operator chasing a discrepancy that exists only in our code — and two copies of a
// probe list drift the moment one gains a check.
func (s *Server) runConnectionChecks(ctx context.Context) []SetupCheck {
	// ⚠ make(), not `var` — a nil slice marshals to JSON `null`, and since V53b the spec
	// declares this array non-nullable. The empty case is not hypothetical here: this function
	// deliberately contributes no check for services that aren't wired, so an UNCONFIGURED
	// INSTALL — the wizard's entire reason to exist — is exactly when it would be nil.
	// `TestResponses_ContainNoJSONNull` caught it; it was the only leak in 46 GET paths.
	checks := make([]SetupCheck, 0, len(connectionChecklist))

	// Connection probes (media_server, requester, tunarr, llm, tmdb, filler) via
	// the shared registry. filler is optional — shown only when configured.
	if s.settings != nil {
		for _, c := range connectionChecklist {
			if c.name == "filler" && s.configValue("filler.dir") == "" {
				continue
			}
			ok, hint := s.settings.Test(ctx, c.name)
			target := ""
			if c.targetKey != "" {
				target = safeConnectionTarget(c.targetKey, s.configValue(c.targetKey))
			}
			checks = append(checks, SetupCheck{Name: c.name, OK: ok, Hint: hint, DocHref: c.docHref, Target: target})
		}
	}

	// Live TV publication check (§6), followed by Tunarr's independent media-source scan.
	if s.livetv != nil {
		wired, err := s.livetv.Wired(ctx)
		check := SetupCheck{Name: "livetv", OK: wired, DocHref: "troubleshooting#livetv"}
		if err != nil {
			check.OK = false
			check.Hint = "Couldn't reach the media server's Live TV settings. Check the media-server connection."
		} else if !wired {
			check.Hint = "The selected playout backend isn't registered as a tuner + guide yet. Re-save its connection settings to retry publication."
		}
		checks = append(checks, check)
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
		checks = append(checks, check)
	}

	return checks
}

// safeConnectionTarget keeps the operationally useful endpoint identity without turning a
// health response into a credential echo. URL settings can technically contain basic-auth
// userinfo or token-bearing query parameters even though Loomarr normally stores credentials
// in separate secret fields; neither belongs in setup/status or the dashboard.
func safeConnectionTarget(key, raw string) string {
	if raw == "" || !strings.HasSuffix(key, ".url") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "configured target"
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	return u.String()
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

// livetvReconnectHandler force-rewires the durably applied tuner (§6): remove + re-add +
// re-scan, to drop a stale channel→stream binding. The transition coordinator owns the
// operation so a reconnect cannot interleave with a backend cutover on another replica.
func (s *Server) livetvReconnectHandler(ctx context.Context, _ *struct{}) (*livetvReconnectOutput, error) {
	if s.backendTransition == nil {
		return nil, errNotImplemented("Live TV isn't set up",
			"Connect your media server and configure the selected playout backend before re-wiring the tuner.", "troubleshooting#livetv")
	}
	reset, err := s.backendTransition.Reconnect(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't re-wire the tuner",
			"Loomarr couldn't re-register the selected playout tuner in your media server. Check the media server and backend settings, then try again.", err)
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
