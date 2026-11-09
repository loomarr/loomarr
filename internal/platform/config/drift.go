package config

import (
	"maps"
	"reflect"
	"slices"
)

// ChangedKeys names the boot keys whose value in desired differs from c, in field order. Every
// env-tagged field counts: a generation reads all of them when it starts, so each one waits on a
// restart. DataDir has no env name and is recorded once, so it never drifts. A nil side has no
// baseline and reports nothing.
func (c *Config) ChangedKeys(desired *Config) []string {
	if c == nil || desired == nil {
		return nil
	}
	var changed []string
	running, want := reflect.ValueOf(c).Elem(), reflect.ValueOf(desired).Elem()
	for _, f := range reflect.VisibleFields(running.Type()) {
		key := f.Tag.Get("env")
		if key == "" || key == "-" {
			continue
		}
		if !running.FieldByIndex(f.Index).Equal(want.FieldByIndex(f.Index)) {
			changed = append(changed, key)
		}
	}
	return changed
}

// RestartDrift returns a func naming the bootstrap and app-managed restart-scoped settings whose
// desired value now differs from the value applied to this process generation (config-design §3).
//
// ⚠ DERIVED on every call, never a sticky flag someone remembers to set. A boolean written at the
// moment of an edit is wrong as soon as the operator edits it back, and would nag about a restart
// that is no longer needed. Re-resolving through the same Load the app itself boots from means this
// cannot drift from reality — and it names the specific key, so the UI can say WHICH setting is
// waiting rather than "something changed".
//
// ⚠ It names ONLY what truly cannot apply live (#1659 Q-S1): every bootstrap key (a generation
// reads them all once, when it starts; ChangedKeys walks the struct, so a new boot key is covered
// with no edit here), and the registry's `apply=restart` keys, which are just the filler folders
// (the watcher, the storage roots and the clip layout are all built on them). Everything else
// hot-applies; a key that goes live drops out of this list by losing `ApplyRestart`, with no edit
// here. The route that serves it is admin-only.
//
// running is captured ONCE, when the handler is built — that is what this generation started with,
// and re-reading it per call would compare the file against itself and never report drift. applied
// maps each restart-scoped registry key to the value this generation applied; current resolves a
// key's desired value.
func RestartDrift(running *Config, applied map[string]string, current func(string) string) func() []string {
	return func() []string {
		var pending []string
		if running != nil {
			// A failed re-read is not drift. The boot config already parsed, so an error here
			// means the file changed to something invalid — worth a log elsewhere, but
			// reporting it as "restart required" would tell the operator to do the one thing
			// that cannot fix it.
			if desired, err := Load(); err == nil {
				pending = running.ChangedKeys(desired)
			}
		}

		// Maps deliberately do not own presentation order. Sort restart-scoped registry keys so
		// the restart endpoint is stable across calls and replicas.
		if current != nil {
			for _, key := range slices.Sorted(maps.Keys(applied)) {
				if current(key) != applied[key] {
					pending = append(pending, key)
				}
			}
		}
		return pending
	}
}
