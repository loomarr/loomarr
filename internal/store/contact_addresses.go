package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/contact"
)

const contactAddressSelect = `SELECT owner_kind, owner_id, email, normalized, status, provenance, created_at, verified_at
 FROM contact_addresses`

// GetContactAddresses returns the verified recovery address and pending replacement for one person.
// A contactless person returns an empty Set; callers establish user existence independently.
func (s *sqlStore) GetContactAddresses(ctx context.Context, userID string) (contact.Set, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(contactAddressSelect+
		` WHERE owner_kind = 'user' AND owner_id = ? ORDER BY status`), userID)
	if err != nil {
		return contact.Set{}, err
	}
	defer func() { _ = rows.Close() }()
	return scanContactSet(rows)
}

// ListContactAddresses returns all current contact state in stable person/status order. It lets the
// People roster hydrate contact data with one bounded query instead of one query per person.
func (s *sqlStore) ListContactAddresses(ctx context.Context) ([]contact.Address, error) {
	rows, err := s.db.QueryContext(ctx, contactAddressSelect+
		` WHERE owner_kind = 'user' ORDER BY owner_id, status`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []contact.Address
	for rows.Next() {
		address, err := scanContactAddress(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, address)
	}
	return out, rows.Err()
}

func (s *sqlStore) GetVerifiedContactAddressByNormalized(ctx context.Context, normalized string) (contact.Address, error) {
	return scanContactAddress(s.db.QueryRowContext(ctx, s.ph(contactAddressSelect+
		` WHERE owner_kind = 'user' AND normalized = ? AND status = 'verified'`), normalized))
}

// PutPendingContactAddress creates or replaces the one unverified candidate. A case-only/display
// correction of the verified mailbox preserves verification and cancels any pending replacement.
func (s *sqlStore) PutPendingContactAddress(ctx context.Context, address contact.Address) error {
	if address.OwnerKind != contact.OwnerUser || address.OwnerID == "" || address.Email == "" || address.Normalized == "" {
		return fmt.Errorf("contact address requires user, email, and normalized key")
	}
	if address.Provenance != contact.ProvenanceAdmin &&
		address.Provenance != contact.ProvenanceInvitation && address.Provenance != contact.ProvenanceSelf {
		return fmt.Errorf("invalid contact provenance %q", address.Provenance)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRowContext(ctx, s.ph(`SELECT 1 FROM users WHERE id = ?`), address.OwnerID).Scan(&exists); err == sql.ErrNoRows {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	var verifiedNormalized string
	err = tx.QueryRowContext(ctx, s.ph(
		`SELECT normalized FROM contact_addresses
		 WHERE owner_kind = 'user' AND owner_id = ? AND status = 'verified'`),
		address.OwnerID).Scan(&verifiedNormalized)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && verifiedNormalized == address.Normalized {
		if _, err = tx.ExecContext(ctx, s.ph(
			`UPDATE contact_addresses SET email = ?
			 WHERE owner_kind = 'user' AND owner_id = ? AND status = 'verified'`),
			address.Email, address.OwnerID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.ph(
			`DELETE FROM contact_addresses
			 WHERE owner_kind = 'user' AND owner_id = ? AND status = 'pending'`), address.OwnerID); err != nil {
			return err
		}
		return tx.Commit()
	}

	_, err = tx.ExecContext(ctx, s.ph(
		`INSERT INTO contact_addresses
		 (owner_kind, owner_id, email, normalized, status, provenance, created_at, verified_at)
		 VALUES ('user', ?, ?, ?, 'pending', ?, ?, NULL)
		 ON CONFLICT(owner_kind, owner_id, status) DO UPDATE SET
		 email=excluded.email, normalized=excluded.normalized, provenance=excluded.provenance,
		 created_at=excluded.created_at, verified_at=NULL`),
		address.OwnerID, address.Email, address.Normalized, string(address.Provenance), epoch(address.CreatedAt))
	if isConstraintViolation(err) {
		return ErrContactAddressConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// VerifyPendingContactAddress atomically promotes the matching pending candidate and retires the
// previous verified address. A stale verification proof cannot promote a newer replacement.
func (s *sqlStore) VerifyPendingContactAddress(
	ctx context.Context, userID, normalized string, at time.Time,
) (contact.Address, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contact.Address{}, err
	}
	defer func() { _ = tx.Rollback() }()

	pending, err := scanContactAddress(tx.QueryRowContext(ctx, s.ph(contactAddressSelect+
		` WHERE owner_kind = 'user' AND owner_id = ? AND status = 'pending' AND normalized = ?`), userID, normalized))
	if err != nil {
		return contact.Address{}, err
	}
	if _, err = tx.ExecContext(ctx, s.ph(
		`DELETE FROM contact_addresses
		 WHERE owner_kind = 'user' AND owner_id = ? AND status = 'verified'`), userID); err != nil {
		return contact.Address{}, err
	}
	res, err := tx.ExecContext(ctx, s.ph(
		`UPDATE contact_addresses SET status = 'verified', verified_at = ?
		 WHERE owner_kind = 'user' AND owner_id = ? AND status = 'pending' AND normalized = ?`),
		epoch(at), userID, normalized)
	if err != nil {
		return contact.Address{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return contact.Address{}, ErrNotFound
	}
	if err = tx.Commit(); err != nil {
		return contact.Address{}, err
	}
	pending.Status = contact.StatusVerified
	pending.VerifiedAt = at
	return pending, nil
}

func (s *sqlStore) DeletePendingContactAddress(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM contact_addresses
		 WHERE owner_kind = 'user' AND owner_id = ? AND status = 'pending'`), userID)
	return err
}

func (s *sqlStore) DeleteContactAddresses(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM contact_addresses WHERE owner_kind = 'user' AND owner_id = ?`), userID)
	return err
}

func scanContactSet(rows *sql.Rows) (contact.Set, error) {
	var set contact.Set
	for rows.Next() {
		address, err := scanContactAddress(rows)
		if err != nil {
			return contact.Set{}, err
		}
		switch address.Status {
		case contact.StatusVerified:
			set.Verified = &address
		case contact.StatusPending:
			set.Pending = &address
		default:
			return contact.Set{}, fmt.Errorf("invalid contact status %q", address.Status)
		}
	}
	return set, rows.Err()
}

func scanContactAddress(sc scannable) (contact.Address, error) {
	var address contact.Address
	var status, provenance string
	var created int64
	var verified sql.NullInt64
	var ownerKind string
	err := sc.Scan(&ownerKind, &address.OwnerID, &address.Email, &address.Normalized,
		&status, &provenance, &created, &verified)
	if err == sql.ErrNoRows {
		return contact.Address{}, ErrNotFound
	}
	if err != nil {
		return contact.Address{}, err
	}
	address.Status = contact.Status(status)
	address.OwnerKind = contact.OwnerKind(ownerKind)
	address.Provenance = contact.Provenance(provenance)
	address.CreatedAt = fromEpoch(created)
	if verified.Valid {
		address.VerifiedAt = fromEpoch(verified.Int64)
	}
	return address, nil
}
