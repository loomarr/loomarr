package playout

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Browser HLS scratch lives under playout.hls_dir, on disk beside the database (#1512), in one
// process-owned root per origin (loomarr-hls-*, loomarr-packager-*). Stop removes a root; a crash
// does not, so a new process sweeps the roots no live process owns. Ownership is a lock the owner
// holds for its lifetime (the kernel drops it when the process dies), never a timestamp: an idle
// root of a live process looks exactly like a crashed one's (live, Air's next build swept the
// serving process's idle root and its next tune failed).

// scratchLock is the file in each root whose lock its owner holds.
const scratchLock = ".owner"

// unlockedQuiet is how long a root with no lock file (made before roots had one) must go
// unwritten before it is taken for a dead process's.
const unlockedQuiet = 24 * time.Hour

// newScratchRoot sweeps base of abandoned roots, then creates and locks this process's own. The
// returned release drops the lock; the caller removes the root with it.
func newScratchRoot(base, prefix string, log *slog.Logger) (root string, release func(), err error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if base != "" {
		// A fresh install may not have it yet; MkdirTemp below is the real check.
		_ = os.MkdirAll(base, 0o755)
	}
	sweepScratch(base, log)
	root, err = os.MkdirTemp(base, prefix)
	if err != nil {
		return "", nil, err
	}
	release, err = holdScratch(root)
	if err != nil {
		_ = os.RemoveAll(root)
		return "", nil, fmt.Errorf("lock %s: %w", root, err)
	}
	return root, release, nil
}

// sweepScratch removes the scratch roots under base that no live process owns.
func sweepScratch(base string, log *slog.Logger) {
	if base == "" {
		base = os.TempDir()
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || (!strings.HasPrefix(name, "loomarr-packager-") && !strings.HasPrefix(name, "loomarr-hls-")) {
			continue
		}
		root := filepath.Join(base, name)
		if !scratchAbandoned(root) {
			continue
		}
		if err := os.RemoveAll(root); err != nil {
			log.Warn("hls scratch: could not remove a previous process's root", "dir", root, "err", err)
			continue
		}
		log.Info("hls scratch: removed a previous process's root", "dir", root)
	}
}

// writtenSince reports whether dir or any directory directly inside it changed after t.
func writtenSince(dir string, t time.Time) bool {
	if fi, err := os.Stat(dir); err != nil || fi.ModTime().After(t) {
		return err == nil
	}
	children, _ := os.ReadDir(dir)
	for _, c := range children {
		if fi, err := c.Info(); err == nil && c.IsDir() && fi.ModTime().After(t) {
			return true
		}
	}
	return false
}
