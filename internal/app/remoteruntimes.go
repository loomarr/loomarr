package app

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/loomarr/loomarr/internal/clipfetch"
)

// remoteRuntimesFile is the runtime cache's file inside the filler folder. Dot-prefixed like the
// artwork cache (filler.ThumbDirName) so scans and intake skip it, and kept beside the clips it
// is for: they share a lifecycle, and emptying the folder takes the cache with it.
const remoteRuntimesFile = ".loomarr-remote-runtimes.json"

// remoteRuntime is what one remote item's metadata said about it.
type remoteRuntime struct {
	DurationMS int `json:"durationMs"`
	Height     int `json:"height,omitempty"`
}

// remoteRuntimeCache remembers remote items' runtimes, keyed by provider and item id (#1773).
//
// Reading an Archive.org runtime costs one throttled metadata call per item (clipfetch.Enrich:
// ~25 items in 15–25 s), and while compilation reels are held back every planned pull and
// scheduled check needs them. A published item's runtime does not change, so each is read once
// and kept on disk across plans and restarts. Only known runtimes are kept: an item Archive.org
// has not probed yet is asked about again next time, since it may have been probed since.
//
// It is derived data. A missing or unreadable file starts empty, and a failed write keeps the
// runtimes in memory; either way the cost is re-reading, never a wrong runtime.
type remoteRuntimeCache struct {
	// path is the cache file; empty keeps runtimes in memory only.
	path string
	log  *slog.Logger

	mu       sync.Mutex
	loaded   bool
	runtimes map[string]remoteRuntime
}

func newRemoteRuntimeCache(path string, log *slog.Logger) *remoteRuntimeCache {
	return &remoteRuntimeCache{path: path, log: log}
}

func remoteRuntimeKey(provider, id string) string { return provider + "/" + id }

// load reads the file on first use. The caller holds mu.
func (c *remoteRuntimeCache) load() {
	if c.loaded {
		return
	}
	c.loaded = true
	c.runtimes = map[string]remoteRuntime{}
	if c.path == "" {
		return
	}
	raw, err := os.ReadFile(c.path)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err == nil {
		err = json.Unmarshal(raw, &c.runtimes)
	}
	if err != nil {
		c.runtimes = map[string]remoteRuntime{}
		c.log.Warn("remote runtime cache unreadable; starting empty", "path", c.path, "err", err)
	}
}

// lookup returns a remembered runtime.
func (c *remoteRuntimeCache) lookup(provider, id string) (remoteRuntime, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	r, ok := c.runtimes[remoteRuntimeKey(provider, id)]
	return r, ok
}

// remember keeps the known runtimes among learned (item id → runtime) and saves the file.
func (c *remoteRuntimeCache) remember(provider string, learned map[string]remoteRuntime) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	changed := false
	for id, r := range learned {
		if r.DurationMS > 0 {
			c.runtimes[remoteRuntimeKey(provider, id)] = r
			changed = true
		}
	}
	if !changed || c.path == "" {
		return
	}
	if err := c.save(); err != nil {
		c.log.Warn("remote runtime cache not saved; runtimes kept in memory", "path", c.path, "err", err)
	}
}

// save replaces the file atomically, so a crash mid-write leaves the previous cache. The caller
// holds mu.
func (c *remoteRuntimeCache) save() error {
	raw, err := json.Marshal(c.runtimes)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.path), remoteRuntimesFile+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after a successful rename
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close() //nolint:errcheck,gosec // the write error is the one reported
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), c.path)
}

// cachedArchiveCatalog reads each Archive.org item's runtime once (#1773): remembered items are
// filled from the cache and only the rest are asked of Archive.org.
type cachedArchiveCatalog struct {
	archiveCatalog
	runtimes *remoteRuntimeCache
}

func (c cachedArchiveCatalog) Enrich(ctx context.Context, items []clipfetch.DiscoveredItem) {
	var unknown []clipfetch.DiscoveredItem
	var at []int
	for i := range items {
		if r, ok := c.runtimes.lookup("archive", items[i].ID); ok {
			items[i].DurationMS, items[i].Height = r.DurationMS, r.Height
			continue
		}
		unknown = append(unknown, items[i])
		at = append(at, i)
	}
	if len(unknown) == 0 {
		return
	}
	c.archiveCatalog.Enrich(ctx, unknown)
	learned := make(map[string]remoteRuntime, len(unknown))
	for j, item := range unknown {
		items[at[j]].DurationMS, items[at[j]].Height = item.DurationMS, item.Height
		learned[item.ID] = remoteRuntime{DurationMS: item.DurationMS, Height: item.Height}
	}
	c.runtimes.remember("archive", learned)
}
