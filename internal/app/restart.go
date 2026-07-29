package app

import (
	"github.com/mantonx/loomarr/internal/config"
)

// restartAdapter is the composition root's bridge between the API's RestartService and
// main's generation loop (§9.2, V13).
//
// A func rather than a channel so the api package never learns how restarting works — it
// asks, main decides. The func must not block: the handler calls it while writing its
// response, and the drain cannot start until that response is on the wire.
type restartAdapter struct{ fn func() }

func (r restartAdapter) Restart() { r.fn() }

// bootstrapDrift names the boot-time settings whose value on disk now differs from the
// one this process is running (config-design §3).
//
// ⚠ DERIVED on every call, never a sticky flag someone remembers to set. A boolean
// written at the moment of an edit is wrong as soon as the operator edits it back, and
// would nag about a restart that is no longer needed. Re-resolving through the same
// config.Load the app itself boots from means this cannot drift from reality — and it
// names the specific key, so the UI can say WHICH setting is waiting rather than
// "something changed".
//
// `running` is captured ONCE, when the handler is built — that is what this generation
// started with, and re-reading it per call would compare the file against itself and
// never report drift.
func bootstrapDrift(running *config.Config) func() []string {
	if running == nil {
		// No baseline ⇒ nothing to compare. Reporting "no drift" is the safe direction:
		// a false "restart required" nags an operator toward an action that fixes nothing.
		return func() []string { return nil }
	}
	return func() []string {
		// A failed re-read is not drift. The boot config already parsed, so an error here
		// means the file changed to something invalid — worth a log elsewhere, but
		// reporting it as "restart required" would tell the operator to do the one thing
		// that cannot fix it.
		current, err := config.Load()
		if err != nil {
			return nil
		}
		var pending []string
		// Only the closed bootstrap set (config/bootstrapfile.go). Every other setting
		// hot-applies, so listing one here would ask for a restart nothing needs.
		if current.DatabaseURL != running.DatabaseURL {
			pending = append(pending, "DATABASE_URL")
		}
		if current.ListenAddr != running.ListenAddr {
			pending = append(pending, "LISTEN_ADDR")
		}
		return pending
	}
}
