package storagegovernor

import (
	"io/fs"
	"os"
	"time"
)

// SetWalkStatHooks replaces the stat seams used while walking a private tree and returns a restore.
func SetWalkStatHooks(
	info func(fs.DirEntry) (fs.FileInfo, error), lstat func(string) (fs.FileInfo, error),
) (restore func()) {
	oldInfo, oldLstat := entryInfo, lstatEntry
	if info != nil {
		entryInfo = info
	}
	if lstat != nil {
		lstatEntry = lstat
	}
	return func() { entryInfo, lstatEntry = oldInfo, oldLstat }
}

var _ = os.Lstat

// SetClock replaces the governor's clock so tests cross the re-walk interval without sleeping.
func SetClock(g *Governor, now func() time.Time) {
	g.mu.Lock()
	g.now = now
	g.mu.Unlock()
}

// WaitForRefresh blocks until every background re-walk started so far has finished.
func WaitForRefresh(g *Governor) { g.refreshes.Wait() }

// UsageRefreshInterval is how long a measured total is trusted before a background re-walk.
const UsageRefreshInterval = usageRefreshInterval
