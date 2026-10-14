package storagegovernor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const writeMonitorInterval = 50 * time.Millisecond

// MonitorPath binds a private output tree to an existing lease. The returned context is cancelled
// as soon as observed regular-file bytes exceed the reservation or live host capacity. finish must
// be called before the private tree is published or renamed; it performs one final synchronous
// measurement and returns the storage reason that stopped the writer.
func MonitorPath(parent context.Context, lease *Lease, root string, baseBytes int64) (context.Context, func() error) {
	return MonitorPaths(parent, lease, []string{root}, baseBytes)
}

// MonitorPaths is MonitorPath for a writer that produces several distinct private files in one
// pass. Their observed bytes are summed before the lease is revalidated, so individually small
// outputs cannot collectively cross one reservation.
func MonitorPaths(parent context.Context, lease *Lease, roots []string, baseBytes int64) (context.Context, func() error) {
	return monitorPaths(parent, lease, roots, baseBytes, false)
}

// MonitorGrowingPath is MonitorPath for output whose size cannot be forecast tightly (a
// quality-targeted encode). Instead of stopping a writer that outgrew its reservation, it
// re-estimates from the observed footprint and extends the lease, so a nearly finished publication
// is not discarded; it still stops when the extension is refused (the hard host reserve).
func MonitorGrowingPath(parent context.Context, lease *Lease, root string, baseBytes int64) (context.Context, func() error) {
	return monitorPaths(parent, lease, []string{root}, baseBytes, true)
}

func monitorPaths(parent context.Context, lease *Lease, roots []string, baseBytes int64, grow bool) (context.Context, func() error) {
	ctx, cancel := context.WithCancelCause(parent)
	stop := make(chan struct{})
	done := make(chan error, 1)
	var checkMu sync.Mutex
	maxObserved := baseBytes
	check := func() error {
		checkMu.Lock()
		defer checkMu.Unlock()
		var bytes int64
		for _, root := range roots {
			pathBytes, err := privatePathBytes(root)
			if err != nil {
				return err
			}
			bytes = saturatingAdd(bytes, pathBytes)
		}
		observed := saturatingAdd(baseBytes, bytes)
		// Writers may replace or remove temporary files while building a final output. A lease
		// reports cumulative progress monotonically, so retain the largest footprint observed.
		if observed < maxObserved {
			observed = maxObserved
		} else {
			maxObserved = observed
		}
		if grow && observed > lease.Estimated() {
			if extension := lease.Extend(parent, saturatingAdd(observed, max(observed/growthMarginDenom, growthFloorBytes))); !extension.Allowed {
				if extension.Err != nil {
					return fmt.Errorf("storage write paused (%s): %w", extension.Snapshot.Reason, extension.Err)
				}
				return fmt.Errorf("storage write paused (%s)", extension.Snapshot.Reason)
			}
		}
		decision := lease.Revalidate(parent, observed)
		if decision.Allowed {
			return nil
		}
		if decision.Err != nil {
			return fmt.Errorf("storage write paused (%s): %w", decision.Snapshot.Reason, decision.Err)
		}
		return fmt.Errorf("storage write paused (%s)", decision.Snapshot.Reason)
	}
	go func() {
		ticker := time.NewTicker(writeMonitorInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				done <- nil
				return
			case <-stop:
				done <- nil
				return
			case <-ticker.C:
				if parent.Err() != nil {
					done <- nil
					return
				}
				if err := check(); err != nil {
					cancel(err)
					done <- err
					return
				}
			}
		}
	}()
	var finishOnce sync.Once
	var finishErr error
	return ctx, func() error {
		finishOnce.Do(func() {
			if context.Cause(ctx) == nil {
				finishErr = check()
				if finishErr != nil {
					cancel(finishErr)
				}
			}
			close(stop)
			if monitorErr := <-done; monitorErr != nil {
				finishErr = monitorErr
			}
			// A parent cancellation belongs to the writer. Do not replace its more useful error
			// with a storage error unless this monitor was the component that stopped the work.
			if finishErr == nil {
				cancel(nil)
			}
		})
		return finishErr
	}
}

func privatePathBytes(root string) (int64, error) {
	info, err := os.Lstat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, errors.New("storage output is symlinked")
	}
	if info.Mode().IsRegular() {
		return info.Size(), nil
	}
	if !info.IsDir() {
		return 0, errors.New("storage output is not a private file or directory")
	}
	var total int64
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, fs.ErrNotExist) {
			return nil // a file the writer already replaced or removed is no longer output
		}
		if walkErr != nil {
			return walkErr
		}
		stat, infoErr := lstatEntry(path)
		if errors.Is(infoErr, fs.ErrNotExist) {
			return nil // written then removed between the listing and this stat (temp segment rename)
		}
		if infoErr != nil {
			return infoErr
		}
		if stat.Mode()&os.ModeSymlink != 0 {
			return errors.New("storage output contains a symlink")
		}
		if stat.IsDir() {
			return nil
		}
		if !stat.Mode().IsRegular() {
			return errors.New("storage output contains a non-regular file")
		}
		total = saturatingAdd(total, stat.Size())
		return nil
	})
	return total, err
}

// lstatEntry is a seam so tests can remove a file between the directory listing and its stat.
var lstatEntry = os.Lstat

// A re-estimate reserves a quarter beyond what was observed (at least 64 MiB) so a steadily growing
// output does not need an extension on every monitor tick.
const (
	growthMarginDenom = 4
	growthFloorBytes  = int64(64 << 20)
)
