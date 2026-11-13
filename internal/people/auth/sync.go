package auth

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// UserLister lists media-server users (the library adapter).
type UserLister interface {
	ListUsers(ctx context.Context) ([]mediaserver.User, error)
}

// UserSync refreshes ALREADY-IMPORTED media-server users (§11): it updates the name from the
// source and disables, revoking sessions and paired devices, any user the server now reports
// disabled. It never re-enables a user: Loomarr owns the disabled flag. It NEVER adds users — import (Provisioner.Import) defines the
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
		if !existing.MediaServerLinked {
			continue // an id collision must never turn a local account into provider-owned state
		}
		existing.Name = ms.Name
		existing.UpdatedAt = now
		if err := s.store.UpsertUser(ctx, existing); err != nil {
			return n, err
		}
		// Newly disabled server-side → disable and revoke sessions and paired devices at once (§11).
		// Loomarr owns the flag (#1861): sync may set it, but a server reporting the user enabled
		// never clears it. Re-enabling is an admin action in Loomarr.
		if !existing.Disabled && ms.Disabled {
			if err := s.store.DisableUser(ctx, ms.ID, now); err != nil {
				return n, err
			}
		}
		n++
	}
	return n, nil
}
