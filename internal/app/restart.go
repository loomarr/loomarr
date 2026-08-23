package app

import (
	"sort"

	"github.com/loomarr/loomarr/internal/config"
	"github.com/loomarr/loomarr/internal/filler"
)

// restartAdapter is the composition root's bridge between the API's RestartService and
// main's generation loop (§9.2, V13).
//
// A func rather than a channel so the api package never learns how restarting works — it
// asks, main decides. The func must not block: the handler calls it while writing its
// response, and the drain cannot start until that response is on the wire.
type restartAdapter struct{ fn func() }

func (r restartAdapter) Restart() { r.fn() }

// restartDrift names bootstrap and app-managed restart-scoped settings whose desired
// value now differs from the value applied to this process generation (config-design §3).
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
func restartDrift(
	running *config.Config,
	applied map[string]string,
	current func(string) string,
) func() []string {
	return func() []string {
		var pending []string
		if running != nil {
			// A failed re-read is not drift. The boot config already parsed, so an error here
			// means the file changed to something invalid — worth a log elsewhere, but
			// reporting it as "restart required" would tell the operator to do the one thing
			// that cannot fix it.
			desired, err := config.Load()
			if err == nil {
				if desired.DatabaseURL != running.DatabaseURL {
					pending = append(pending, "DATABASE_URL")
				}
				if desired.ListenAddr != running.ListenAddr {
					pending = append(pending, "LISTEN_ADDR")
				}
			}
		}

		// Maps deliberately do not own presentation order. Sort restart-scoped registry
		// keys so the restart endpoint is stable across calls and replicas.
		keys := make([]string, 0, len(applied))
		for key := range applied {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if current != nil {
			for _, key := range keys {
				if current(key) != applied[key] {
					pending = append(pending, key)
				}
			}
		}
		return pending
	}
}

// canonicalFillerRestartBaseline records the topology the generation actually uses, not the raw
// spelling persisted in Settings. Equivalent edits such as `/data/x/../filler` must not ask for a
// restart, and an empty watch setting must track the derived `<clip>/_watch` path.
func canonicalFillerRestartBaseline(applied map[string]string, layout filler.Layout) {
	if _, ok := applied["filler.dir"]; ok {
		applied["filler.dir"] = layout.ClipDir()
	}
	if _, ok := applied["filler.watch_dir"]; ok {
		applied["filler.watch_dir"] = layout.WatchDir()
	}
}

// canonicalRestartCurrent resolves the desired filler pair through the same Layout seam used by
// Build. Invalid pair topology stays visibly different and remains pending until fixed.
func canonicalRestartCurrent(set resolved) func(string) string {
	return func(key string) string {
		if key != "filler.dir" && key != "filler.watch_dir" {
			return set.str(key)
		}
		layout, err := filler.NewLayout(set.str("filler.dir"), set.str("filler.watch_dir"))
		if err != nil {
			return set.str(key)
		}
		if key == "filler.dir" {
			return layout.ClipDir()
		}
		return layout.WatchDir()
	}
}
