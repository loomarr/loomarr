package playout

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// playout.hls_dir is disk-backed beside the database (#1512), so a crash (which skips Stop) would
// leave its segments there for good. A new process sweeps the roots no live process owns, by the
// owner's lock, not by age: live, a new build swept the still-serving process's root because it
// had been idle for ten minutes, and that process's next tune failed.
func TestNewScratchRootSweepsOnlyUnownedRoots(t *testing.T) {
	base := t.TempDir()
	mk := func(rel string, age time.Duration, lock bool) string {
		p := filepath.Join(base, rel)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if lock {
			if err := os.WriteFile(filepath.Join(p, scratchLock), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		mtime := time.Now().Add(-age)
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		return p
	}
	crashed := mk("loomarr-packager-1", 0, true) // fresh, but its owner is gone
	crashedRemux := mk("loomarr-hls-2", 0, true)
	idle := mk("loomarr-packager-3", time.Hour, true) // a live process with no viewers for an hour
	release, err := holdScratch(idle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	legacy := mk("loomarr-hls-4", time.Hour, false) // no lock file: spared until long quiet
	legacyDead := mk("loomarr-hls-5", 25*time.Hour, false)
	other := mk("operator-files", 25*time.Hour, false)

	root, unlock, err := NewScratchRoot(base, "loomarr-packager-", slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unlock)
	for p, want := range map[string]bool{
		crashed: false, crashedRemux: false, legacyDead: false, idle: true, legacy: true, other: true, root: true,
	} {
		if _, err := os.Stat(p); (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", filepath.Base(p), err == nil, want)
		}
	}
}
