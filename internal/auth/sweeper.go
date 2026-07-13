package auth

import (
	"context"
	"time"

	"github.com/mantonx/loomarr/internal/store"
)

// SessionSweeper purges expired sessions (§5 retention). It implements the
// reconcile.Sweeper interface so the janitor (Phase 7) runs it on the ticker.
type SessionSweeper struct{ store store.Store }

// NewSessionSweeper builds the sessions retention sweeper.
func NewSessionSweeper(st store.Store) *SessionSweeper { return &SessionSweeper{store: st} }

func (s *SessionSweeper) Name() string { return "sessions" }

func (s *SessionSweeper) Sweep(ctx context.Context, now time.Time) (int, error) {
	return s.store.PurgeExpiredSessions(ctx, now)
}
