package cliplibrary

import (
	"context"
	"io/fs"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/media/tools"
)

// ProbeCache remembers what ffprobe reported for each file, so the 15-minute sync does not run
// ffprobe again over a library that has not changed (#3855). A file counts as unchanged when its
// size and modification time are what they were at the scan that probed it; anything else is
// probed afresh. Only a probe that produced a result is remembered, so a file that could not be
// read is tried again on every scan until it can.
//
// The cache lives in memory: after a restart the first scan probes every file once. It never
// stands in for the content hash or the sidecar, which every scan still reads.
type ProbeCache struct {
	mu      sync.Mutex
	entries map[string]probeEntry
}

type probeEntry struct {
	size    int64
	modTime time.Time
	probed  tools.Probed
}

// NewProbeCache returns an empty cache.
func NewProbeCache() *ProbeCache { return &ProbeCache{entries: map[string]probeEntry{}} }

// probePass is one scan's view of the cache: the files it saw, so the entries of files that are
// gone can be dropped when the walk completes.
type probePass struct {
	cache *ProbeCache
	seen  map[string]bool
}

// begin starts a scan's pass. A nil cache yields a pass that probes every file.
func (c *ProbeCache) begin() *probePass {
	return &probePass{cache: c, seen: map[string]bool{}}
}

// probe returns the remembered result for an unchanged file, else asks probe and remembers a
// result that has a duration.
func (p *probePass) probe(ctx context.Context, path string, info fs.FileInfo, probe tools.Prober) (tools.Probed, error) {
	if p.cache == nil || info == nil {
		return probe(ctx, path)
	}
	p.seen[path] = true
	p.cache.mu.Lock()
	entry, ok := p.cache.entries[path]
	p.cache.mu.Unlock()
	if ok && entry.size == info.Size() && entry.modTime.Equal(info.ModTime()) {
		return entry.probed, nil
	}
	result, err := probe(ctx, path)
	if err == nil && result.DurationMs > 0 {
		p.cache.mu.Lock()
		p.cache.entries[path] = probeEntry{size: info.Size(), modTime: info.ModTime(), probed: result}
		p.cache.mu.Unlock()
	}
	return result, err
}

// finish drops the entries of files this scan did not see. Called only for a walk that completed,
// so a scan cut short does not forget the files it never reached.
func (p *probePass) finish() {
	if p.cache == nil {
		return
	}
	p.cache.mu.Lock()
	defer p.cache.mu.Unlock()
	for path := range p.cache.entries {
		if !p.seen[path] {
			delete(p.cache.entries, path)
		}
	}
}
