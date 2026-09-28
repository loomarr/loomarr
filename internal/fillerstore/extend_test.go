package fillerstore

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/store"
)

// TestExtendHidesNoCoreMethod guards the wrapper Extend returns. It embeds the store.Store
// interface, so it promotes only that interface's methods: any other exported method on the core
// adapter disappears behind it, and a caller that type-asserts for one silently takes its fallback
// (#1752 lost the Incoming count per source that way). Every core method must either be reachable
// on the wrapper or be listed below with the call that still reaches it.
func TestExtendHidesNoCoreMethod(t *testing.T) {
	s, err := openExtended(t.Context(), "sqlite://"+filepath.Join(t.TempDir(), "loomarr.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// The backup methods are capability probes: callers reach them through helpers that unwrap
	// the wrapper to the core adapter, never by asserting on the store they hold.
	reachedThroughCore := map[string]bool{
		"StreamBackup": store.SQLiteBackuper(s) != nil,
		"WriteBackup":  store.BackupWriter(s) != nil,
	}

	wrapper, core := reflect.TypeOf(s), reflect.TypeOf(s.Core())
	for i := range core.NumMethod() {
		name := core.Method(i).Name
		if _, ok := wrapper.MethodByName(name); ok {
			continue
		}
		reached, listed := reachedThroughCore[name]
		switch {
		case !listed:
			t.Errorf("%s is on the core adapter but hidden by the Extend wrapper; add it to a store interface", name)
		case !reached:
			t.Errorf("%s is hidden by the Extend wrapper and its unwrapping helper no longer reaches it", name)
		}
	}
}
