package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// /v1/system/restart* + /v1/system/reload — the restart control (§9.2, §7, V13).
//
// Loomarr restarts by rebuilding itself in the same process: no re-exec, no supervisor,
// and identical behaviour on Windows (where `syscall.Exec` is a stub that compiles and
// fails at runtime). The mechanism lives in main's generation loop; this package only
// asks for it through a func, so the API never learns how restarting works.
//
// ⚠ **The GET exists so the confirm dialog states consequences instead of guessing.** A
// restart is not free once Loomarr owns the encoder (§9.1): internally-played channels
// drop for a few seconds while Tunarr-backed ones keep playing, and only the server knows
// how many of each are live right now.

// RestartService backs the restart routes. Implemented in the composition root over the
// generation loop; nil ⇒ the routes report 501, which is the honest answer for a handler
// built without a loop behind it (tests, the embedded harness).
type RestartService interface {
	// Restart asks the process to rebuild itself. It must return promptly — the handler
	// calls it while writing the response, and the drain cannot start until that
	// response is on the wire.
	Restart()
}

// RestartCost is what a restart would actually cost right now.
type RestartCost struct {
	// StreamingChannels is how many channels Loomarr is encoding itself. These drop for
	// a few seconds; Tunarr-backed channels do not (§9.1). Reported as a live count
	// rather than a static warning because an install can have both at once.
	StreamingChannels int `json:"streamingChannels" doc:"Channels Loomarr is streaming that will drop"`
	// RestartRequired reports a restart-scoped setting whose saved value differs from the one
	// this application generation is running (config-design §3).
	RestartRequired bool `json:"restartRequired" doc:"A restart-scoped setting has changed and needs a restart to apply"`
	// PendingKeys names the changed restart-scoped settings, so the UI can say WHICH one
	// rather than "something changed".
	PendingKeys []string `json:"pendingKeys,omitempty" doc:"Restart-scoped settings waiting on a restart, e.g. DATABASE_URL or filler.dir"`
	// Available is false when this build has no restart loop behind it. The UI needs it
	// to explain the absence rather than offer a button that cannot work.
	Available bool `json:"available" doc:"Whether this process can restart itself"`
}

// registerSystemRestart mounts the restart + reload routes. Admin-only: restarting
// interrupts playback for every internally-streamed channel.
func (s *Server) registerSystemRestart(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-restart-cost", Method: http.MethodGet, Path: "/v1/system/restart",
		Summary: "What a restart would cost right now",
		Description: "Admin only. The live count of channels Loomarr is streaming (which drop for a few " +
			"seconds; Tunarr-backed channels keep playing), plus any restart-scoped setting waiting on a " +
			"restart. Read-only — this does not restart anything.",
		Tags: []string{"system"},
	}, RoleAdmin), s.systemRestartCost)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-restart", Method: http.MethodPost, Path: "/v1/system/restart",
		Summary: "Restart Loomarr in place",
		Description: "Admin only. Drains connections, tears down playout, and rebuilds every subsystem in " +
			"the same process (§9.2) — no supervisor required. Responds before the drain begins, because " +
			"a client that never gets a reply cannot tell 'restarting' from 'crashed'.",
		Tags:          []string{"system"},
		DefaultStatus: http.StatusAccepted,
	}, RoleAdmin), s.systemRestart)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-reload", Method: http.MethodPost, Path: "/v1/system/reload",
		Summary: "Re-probe every configured service",
		Description: "Admin only. Re-runs the connection checks without restarting or tearing anything " +
			"down — no downtime. Uses the same probe implementation as the wizard's checklist, so the two " +
			"can never disagree.",
		Tags: []string{"system"},
	}, RoleAdmin), s.systemReload)
}

type systemRestartCostOutput struct {
	Body RestartCost
}

func (s *Server) systemRestartCost(ctx context.Context, _ *struct{}) (*systemRestartCostOutput, error) {
	out := &systemRestartCostOutput{}
	out.Body.Available = s.restart != nil

	// The live encoder count, from the same Stats V16 built for the dashboard — it already
	// filters CLOSED sessions, so a channel nobody is watching is not counted as a
	// casualty. Zero on a Tunarr-backed install, where nothing Loomarr owns is streaming:
	// the correct answer, not a missing one.
	if s.playoutObserver != nil {
		out.Body.StreamingChannels = len(s.playoutObserver.Stats(time.Now()))
	}

	// ⚠ DERIVED, never a sticky flag. A boolean written when the operator saves is wrong
	// the moment they change it back, and would nag about a restart no longer needed
	// (config-design §3).
	if s.restartDrift != nil {
		out.Body.PendingKeys = s.restartDrift()
		out.Body.RestartRequired = len(out.Body.PendingKeys) > 0
	}
	return out, nil
}

func (s *Server) systemRestart(ctx context.Context, _ *struct{}) (*struct{}, error) {
	if s.restart == nil {
		return nil, errNotImplemented("Restart unavailable",
			"This build can't restart itself. Restart the container or service the way you started it.")
	}
	// Returns immediately; the drain happens after this response is written.
	s.restart.Restart()
	return &struct{}{}, nil
}

type systemReloadOutput struct {
	Body struct {
		// Checks reuses the setup-check shape, so reload and the wizard checklist render
		// through the same component and cannot drift apart.
		Checks []SetupCheck `json:"checks"`
	}
}

func (s *Server) systemReload(ctx context.Context, _ *struct{}) (*systemReloadOutput, error) {
	// ⚠ The SAME probe the wizard's checklist runs, not a second implementation — a
	// reload that disagreed with it would send an operator chasing a discrepancy that
	// exists only in our code.
	out := &systemReloadOutput{}
	out.Body.Checks = s.runConnectionChecks(ctx)
	// A nil slice serializes as `null`, which a client must special-case before mapping.
	// An unwired build legitimately has no checks to report.
	if out.Body.Checks == nil {
		out.Body.Checks = []SetupCheck{}
	}
	return out, nil
}
