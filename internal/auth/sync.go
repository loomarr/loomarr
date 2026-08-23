package auth

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/store"
)

// UserLister lists media-server users (the Phase-5 library adapter).
type UserLister interface {
	ListUsers(ctx context.Context) ([]library.User, error)
}

// UserSync refreshes ALREADY-IMPORTED media-server users (§11): it updates name +
// disabled from the source and revokes sessions for any user the server now
// reports disabled. It NEVER adds users — import (Provisioner.Import) defines the
// allowlist, sync only reconciles it (§11: "sync refreshes but never adds").
// Local role/quota/auto_approve are preserved (admin-managed).
type UserSync struct {
	lib   UserLister
	store store.UserStore
	now   func() time.Time
}

// NewUserSync builds the user-sync service.
func NewUserSync(lib UserLister, st store.UserStore, now func() time.Time) *UserSync {
	if now == nil {
		now = time.Now
	}
	return &UserSync{lib: lib, store: st, now: now}
}

// Sync runs one refresh pass over already-imported users, returning how many were
// updated. A media-server user with no local row is IGNORED — sync never adds to
// the allowlist (§11); use Provisioner.Import for that.
func (s *UserSync) Sync(ctx context.Context) (int, error) {
	msUsers, err := s.lib.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	now := s.now()
	n := 0
	for _, ms := range msUsers {
		existing, err := s.store.GetUser(ctx, ms.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue // not imported → not our concern (the allowlist is import-defined)
		}
		if err != nil {
			return n, err
		}
		wasEnabled := !existing.Disabled
		existing.Name = ms.Name
		existing.Disabled = ms.Disabled
		existing.UpdatedAt = now
		if err := s.store.UpsertUser(ctx, existing); err != nil {
			return n, err
		}
		// Newly disabled server-side → revoke sessions immediately (§11).
		if wasEnabled && ms.Disabled {
			if err := s.store.RevokeSessionsForUser(ctx, ms.ID); err != nil {
				return n, err
			}
		}
		n++
	}
	return n, nil
}
