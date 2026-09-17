package filler

import (
	"context"
	"errors"
	"os"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

// storageWriteTracker turns one operation's lease into a monotonic actual-output ceiling. Each
// private writer is monitored before publication; Record commits its completed byte count so the
// next writer cannot reuse capacity already consumed earlier in the same operation.
type storageWriteTracker struct {
	lease   *storagegovernor.Lease
	written int64
}

func (t *storageWriteTracker) Monitor(ctx context.Context, path string) (context.Context, func() error) {
	if t == nil || t.lease == nil {
		return ctx, func() error { return nil }
	}
	return storagegovernor.MonitorPath(ctx, t.lease, path, t.written)
}

func (t *storageWriteTracker) Record(ctx context.Context, bytes int64) error {
	if t == nil || t.lease == nil || bytes == 0 {
		return nil
	}
	if bytes < 0 || t.written > int64(^uint64(0)>>1)-bytes {
		return errors.New("track filler storage (estimate_unknown): written byte count overflowed")
	}
	next := t.written + bytes
	decision := t.lease.Revalidate(ctx, next)
	if !decision.Allowed {
		return storageDecisionError("track filler storage", decision)
	}
	t.written = next
	return nil
}

func (t *storageWriteTracker) RecordFile(ctx context.Context, path string) error {
	if t == nil || t.lease == nil {
		return nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("track filler storage: output is not a regular file")
	}
	return t.Record(ctx, info.Size())
}
