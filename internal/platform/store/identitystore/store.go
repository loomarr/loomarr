package identitystore

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// ErrNotFound is storedb.ErrNotFound, the kernel's value under this package's name, so errors.Is
// matches an error whichever of internal/platform/store, identitystore or storedb produced it.
var ErrNotFound = storedb.ErrNotFound

// ErrContactAddressConflict means the normalized mailbox is already attached to another person
// or pending replacement. Public recovery must never resolve one address ambiguously.
var ErrContactAddressConflict = errors.New("store: contact address conflict")

// ErrInvitationIdentityConflict means an allowlist row or active Invitation already owns the
// reserved local username or exact Library account id.
var ErrInvitationIdentityConflict = errors.New("store: invitation identity conflict")

// Store is the identity cluster's persistence: users, sessions, devices, invitations, contact
// addresses, password recovery and notifications, over one storedb.Handle.
//
// A Store holds locks and must not be copied; build one with New and share the pointer.
type Store struct {
	h storedb.Handle
	// invitationLock serializes admission checks per identity (SQLite mutex, Postgres advisory
	// lock). One KeyedLock per lock domain, so the domains never share a SQLite mutex.
	invitationLock storedb.KeyedLock
	// recoveryLock serializes replacement of a person's pending recovery, the same way.
	recoveryLock storedb.KeyedLock
}

// New returns the identity store for h.
func New(h storedb.Handle) *Store { return &Store{h: h} }

// The kernel's codecs and scan shape under the names this cluster's statements already use.
type scannable = storedb.Scannable

func epoch(t time.Time) int64            { return storedb.Epoch(t) }
func fromEpoch(n int64) time.Time        { return storedb.FromEpoch(n) }
func isConstraintViolation(e error) bool { return storedb.IsConstraintViolation(e) }

// GetUserIn reads one user through q, which is the pool or a caller's transaction, so a
// transaction that must see the user inside its own snapshot (proposal approval's quota check)
// reads through the same statement as Store.GetUser. It returns ErrNotFound when absent.
func GetUserIn(ctx context.Context, q storedb.Querier, rebind storedb.Placeholder, id string) (User, error) {
	return scanUser(q.QueryRowContext(ctx, rebind(
		`SELECT id, name, role, disabled, quota, auto_approve, media_server_linked, password_hash, created_at, updated_at
		 FROM users WHERE id = ?`), id))
}

// InsertNotificationReferenceRecipient records that personID asked for the title or channel
// referenceID names, so a later notification about it reaches them. It runs on q (a caller's
// transaction, which has already written the title or channel: the reference is a foreign key)
// and is idempotent per (kind, id, person). A person who has since been deleted is not recorded:
// news could never reach them, and their proposal can still be approved.
func InsertNotificationReferenceRecipient(
	ctx context.Context,
	q storedb.Querier,
	rebind storedb.Placeholder,
	kind notifications.ReferenceKind, referenceID, personID string,
	createdAt int64,
) error {
	t, err := requesterTable(kind)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, rebind(`INSERT INTO `+t.table+` (`+t.column+`, person_id, created_at)
		SELECT ?, u.id, ? FROM users u WHERE u.id = ?
		ON CONFLICT (`+t.column+`, person_id) DO NOTHING`),
		referenceID, createdAt, personID)
	return err
}
