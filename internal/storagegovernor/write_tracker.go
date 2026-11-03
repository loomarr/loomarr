package storagegovernor

import (
	"context"
	"errors"
	"os"
)

// WriteTracker turns one operation's lease into a monotonic actual-output ceiling. Each
// private writer is monitored before publication; Record commits its completed byte count so the
// next writer cannot reuse capacity already consumed earlier in the same operation.
//
// A nil *WriteTracker, or one built from a nil lease, is valid and imposes no ceiling. A tracker
// belongs to one operation and is not safe for concurrent use.
type WriteTracker struct {
	lease   *Lease
	written int64
}

// NewWriteTracker returns a tracker that charges the operation's writes against lease.
func NewWriteTracker(lease *Lease) *WriteTracker {
	return &WriteTracker{lease: lease}
}

// Monitor binds path to the lease for the next writer, counting the bytes already recorded.
func (t *WriteTracker) Monitor(ctx context.Context, path string) (context.Context, func() error) {
	if t == nil || t.lease == nil {
		return ctx, func() error { return nil }
	}
	return MonitorPath(ctx, t.lease, path, t.written)
}

// Record commits bytes of finished output to the operation's running total.
func (t *WriteTracker) Record(ctx context.Context, bytes int64) error {
	if t == nil || t.lease == nil || bytes == 0 {
		return nil
	}
	if bytes < 0 || t.written > int64(^uint64(0)>>1)-bytes {
		return errors.New("track filler storage (estimate_unknown): written byte count overflowed")
	}
	next := t.written + bytes
	decision := t.lease.Revalidate(ctx, next)
	if !decision.Allowed {
		return Pause("track filler storage", decision)
	}
	t.written = next
	return nil
}

// RecordFile commits the size of the regular file at path.
func (t *WriteTracker) RecordFile(ctx context.Context, path string) error {
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
