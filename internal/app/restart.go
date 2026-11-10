package app

import (
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// restartAdapter is the composition root's bridge between the API's RestartService and
// main's generation loop (§9.2, V13).
//
// A func rather than a channel so the api package never learns how restarting works — it
// asks, main decides. The func must not block: the handler calls it while writing its
// response, and the drain cannot start until that response is on the wire.
type restartAdapter struct{ fn func() }

func (r restartAdapter) Restart() { r.fn() }

// canonicalFillerRestartBaseline records the topology the generation actually uses, not the raw
// spelling persisted in Settings. Equivalent edits such as `/data/x/../filler` must not ask for a
// restart, and an empty watch setting must track the derived `<clip>/_watch` path.
func canonicalFillerRestartBaseline(applied map[string]string, layout cliplibrary.Layout) {
	if _, ok := applied[settings.FillerDir.Key()]; ok {
		applied[settings.FillerDir.Key()] = layout.ClipDir()
	}
	if _, ok := applied[settings.FillerWatchDir.Key()]; ok {
		applied[settings.FillerWatchDir.Key()] = layout.WatchDir()
	}
}

// canonicalRestartCurrent resolves the desired filler pair through the same Layout seam used by
// Build. Invalid pair topology stays visibly different and remains pending until fixed.
func canonicalRestartCurrent(set resolved) func(string) string {
	return func(key string) string {
		if key != settings.FillerDir.Key() && key != settings.FillerWatchDir.Key() {
			return settings.ValueString(set.Value(key))
		}
		layout, err := cliplibrary.NewLayout(settings.FillerDir.Get(set), settings.FillerWatchDir.Get(set))
		if err != nil {
			return settings.ValueString(set.Value(key))
		}
		if key == settings.FillerDir.Key() {
			return layout.ClipDir()
		}
		return layout.WatchDir()
	}
}
