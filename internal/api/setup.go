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
}

type setupStatusOutput struct {
	Body struct {
		Checks []SetupCheck `json:"checks"`
	}
}

// setupStatus runs the connection checklist (§13). Phase 10 contributes the Live
// TV "wired?" check; other integrations' checks are added by their phases.
func (s *Server) setupStatus(ctx context.Context, _ *struct{}) (*setupStatusOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	out := &setupStatusOutput{}
	if s.livetv != nil {
		wired, err := s.livetv.Wired(ctx)
		check := SetupCheck{Name: "livetv", OK: wired}
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
		check := SetupCheck{Name: "tunarr_library", OK: ready}
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
