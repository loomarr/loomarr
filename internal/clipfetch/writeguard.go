package clipfetch

import (
	"context"
	"sync"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

const writeGuardRevalidateStep = 8 << 20

// writeGuard is shared by every file one provider item produces. The byte
// ceiling therefore applies to aggregate staging, not independently to media,
// sidecars, and provider bookkeeping.
type writeGuard struct {
	lease   *storagegovernor.Lease
	ceiling int64

	mu             sync.Mutex
	written        int64
	nextRevalidate int64
}

func newWriteGuard(lease *storagegovernor.Lease, ceiling int64) *writeGuard {
	return &writeGuard{lease: lease, ceiling: ceiling, nextRevalidate: writeGuardRevalidateStep}
}

func (g *writeGuard) Add(ctx context.Context, bytes int64) error {
	if g == nil || bytes <= 0 {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.written > int64(^uint64(0)>>1)-bytes {
		return ErrWriteCeilingExceeded
	}
	g.written += bytes
	if g.ceiling > 0 && g.written > g.ceiling {
		return ErrWriteCeilingExceeded
	}
	if g.lease != nil && g.written >= g.nextRevalidate {
		decision := g.lease.Revalidate(ctx, g.written)
		if !decision.Allowed {
			return &CapacityError{Decision: decision}
		}
		g.nextRevalidate = g.written + writeGuardRevalidateStep
	}
	return nil
}

func (g *writeGuard) Finish(ctx context.Context) error {
	if g == nil || g.lease == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	decision := g.lease.Revalidate(ctx, g.written)
	if !decision.Allowed {
		return &CapacityError{Decision: decision}
	}
	return nil
}

func (g *writeGuard) Written() int64 {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.written
}

func (g *writeGuard) CheckTotal(ctx context.Context, total int64) error {
	if g == nil {
		return nil
	}
	current := g.Written()
	if total <= current {
		return nil
	}
	return g.Add(ctx, total-current)
}
