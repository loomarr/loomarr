package storagegovernor

import (
	"context"
	"time"
)

// usageRefreshInterval is how long a measured managed total is trusted before a background
// re-walk corrects drift from writes the governor never saw.
const usageRefreshInterval = 10 * time.Minute

// usageKey identifies one running total: managed bytes are counted per real filesystem and domain.
type usageKey struct {
	filesystemID string
	domain       Domain
}

// usage is the in-memory managed-bytes total for one usageKey. The first observation walks the
// tree once; reservations then adjust the total and a background re-walk replaces it every
// usageRefreshInterval. Every field is guarded by Governor.mu, and no walk runs while it is held.
type usage struct {
	scope      string // roots the total covers; a change discards the total
	epoch      int    // bumped when the total is discarded so an in-flight re-walk cannot restore it
	bytes      int64
	loaded     bool
	loading    chan struct{} // closed when the first walk finishes; non-nil while it runs
	loadErr    error
	refreshing bool
	checkedAt  time.Time // when the last walk (successful or not) was started
}

// prepareUsage guarantees the running total for the path's filesystem and domain exists before
// the caller takes g.mu. The first call per key walks the tree (callers wait on one shared walk);
// later calls return immediately and, once the total is stale, start one background re-walk.
// Measurement problems are left for snapshotLocked to report so there is a single error shape.
func (g *Governor) prepareUsage(ctx context.Context, path string, domain Domain) error {
	if ctx.Err() != nil || g.meter == nil || path == "" {
		return nil
	}
	measurement, err := g.meter.Measure(ctx, path)
	if err != nil || measurement.ID == "" {
		return nil
	}
	key := usageKey{filesystemID: measurement.ID, domain: domain}
	var scope string
	if scoped, ok := g.meter.(interface {
		UsageScope(Domain) (string, error)
	}); ok {
		if scope, err = scoped.UsageScope(domain); err != nil {
			return err
		}
	}
	for {
		g.mu.Lock()
		entry := g.usage[key]
		if entry == nil {
			entry = &usage{scope: scope}
			g.usage[key] = entry
		}
		if entry.loaded && entry.scope != scope {
			entry.loaded, entry.scope = false, scope
			entry.epoch++
		}
		switch {
		case entry.loaded:
			g.refreshIfStaleLocked(key, entry)
			g.mu.Unlock()
			return nil
		case entry.loading != nil:
			wait := entry.loading
			g.mu.Unlock()
			select {
			case <-wait:
			case <-ctx.Done():
				return ctx.Err()
			}
			g.mu.Lock()
			err, loaded := entry.loadErr, entry.loaded
			g.mu.Unlock()
			if !loaded && err != nil {
				return err
			}
		default:
			entry.loading = make(chan struct{})
			entry.loadErr = nil
			entry.scope = scope
			entry.checkedAt = g.now()
			g.mu.Unlock()

			bytes, walkErr := g.meter.ManagedBytes(ctx, key.filesystemID, key.domain)
			if walkErr == nil && bytes < 0 {
				walkErr = errManagedInvalid
			}
			g.mu.Lock()
			if walkErr == nil {
				entry.bytes, entry.loaded = bytes, true
			}
			entry.loadErr = walkErr
			close(entry.loading)
			entry.loading = nil
			g.mu.Unlock()
			return walkErr
		}
	}
}

// refreshIfStaleLocked starts at most one background re-walk per key once the total is older than
// usageRefreshInterval. The walk runs without g.mu; a failed walk keeps the previous total and is
// retried after the next interval.
func (g *Governor) refreshIfStaleLocked(key usageKey, entry *usage) {
	if entry.refreshing || g.now().Sub(entry.checkedAt) < usageRefreshInterval {
		return
	}
	entry.refreshing = true
	entry.checkedAt = g.now()
	epoch := entry.epoch
	g.refreshes.Add(1)
	go func() {
		defer g.refreshes.Done()
		ctx, cancel := context.WithTimeout(context.Background(), usageRefreshInterval)
		defer cancel()
		bytes, err := g.meter.ManagedBytes(ctx, key.filesystemID, key.domain)
		g.mu.Lock()
		if err == nil && bytes >= 0 && entry.loaded && entry.epoch == epoch {
			entry.bytes = bytes
		}
		entry.refreshing = false
		g.mu.Unlock()
	}()
}

// addUsageLocked folds bytes a lease reports as written into the running total.
func (g *Governor) addUsageLocked(key usageKey, written int64) {
	if entry := g.usage[key]; entry != nil && entry.loaded {
		entry.bytes = saturatingAdd(entry.bytes, written)
	}
}
