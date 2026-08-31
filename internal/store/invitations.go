package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/contact"
	"github.com/loomarr/loomarr/internal/invitation"
)

const invitationSelect = `SELECT id, kind, username, library_user_id, display_name,
 identity_key, role, status, created_at, expires_at, terminal_at, redeemed_by FROM invitations`

const invitationGrantSelect = `SELECT token_hash, invitation_id, grant_kind, conveyance,
 created_at, expires_at, consumed_at, revoked_at FROM invitation_grants`

func (s *sqlStore) CreateInvitation(
	ctx context.Context,
	value invitation.Invitation,
	address *contact.Address,
) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("validate invitation: %w", err)
	}
	if value.Status != invitation.StatusPending || !value.TerminalAt.IsZero() || value.RedeemedBy != "" {
		return fmt.Errorf("new invitation must be pending")
	}
	if address != nil {
		if address.OwnerKind != contact.OwnerInvitation || address.OwnerID != value.ID ||
			address.Email == "" || address.Normalized == "" || address.Status != contact.StatusPending ||
			address.Provenance != contact.ProvenanceAdmin || address.CreatedAt.IsZero() || !address.VerifiedAt.IsZero() {
			return fmt.Errorf("invitation contact must be an unverified admin-supplied address for this invitation")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("create invitation: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if value.Kind == invitation.KindLocal {
		err = tx.QueryRowContext(ctx, s.ph(
			`SELECT 1 FROM users WHERE lower(trim(name)) = ? LIMIT 1`), value.IdentityKey).Scan(&exists)
	} else {
		err = tx.QueryRowContext(ctx, s.ph(
			`SELECT 1 FROM users WHERE id = ? LIMIT 1`), value.IdentityKey).Scan(&exists)
	}
	if err == nil {
		return ErrInvitationIdentityConflict
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("create invitation: check allowlist: %w", err)
	}

	_, err = tx.ExecContext(ctx, s.ph(`INSERT INTO invitations
		(id, kind, username, library_user_id, display_name, identity_key, role, status,
		 created_at, expires_at, terminal_at, redeemed_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		value.ID, string(value.Kind), value.Username, value.LibraryUserID, value.DisplayName,
		value.IdentityKey, string(value.Role), string(value.Status), epoch(value.CreatedAt),
		epoch(value.ExpiresAt), epoch(value.TerminalAt), value.RedeemedBy)
	if isConstraintViolation(err) {
		return ErrInvitationIdentityConflict
	}
	if err != nil {
		return fmt.Errorf("create invitation: insert: %w", err)
	}
	if address != nil {
		_, err = tx.ExecContext(ctx, s.ph(`INSERT INTO contact_addresses
			(owner_kind, owner_id, email, normalized, status, provenance, created_at, verified_at)
			VALUES ('invitation', ?, ?, ?, 'pending', 'admin', ?, NULL)`),
			address.OwnerID, address.Email, address.Normalized, epoch(address.CreatedAt))
		if isConstraintViolation(err) {
			return ErrContactAddressConflict
		}
		if err != nil {
			return fmt.Errorf("create invitation: insert contact: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("create invitation: commit: %w", err)
	}
	return nil
}

func (s *sqlStore) GetInvitationContactAddress(
	ctx context.Context,
	invitationID string,
) (contact.Address, error) {
	return scanContactAddress(s.db.QueryRowContext(ctx, s.ph(contactAddressSelect+
		` WHERE owner_kind = 'invitation' AND owner_id = ? AND status = 'pending'`), invitationID))
}

func (s *sqlStore) ReplaceInvitationGrant(
	ctx context.Context,
	invitationID string,
	grant invitation.Grant,
	at time.Time,
) error {
	if err := grant.Validate(); err != nil {
		return fmt.Errorf("validate invitation grant: %w", err)
	}
	if grant.InvitationID != invitationID || !grant.CreatedAt.Equal(at) {
		return fmt.Errorf("invitation grant ownership or creation time mismatch")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace invitation grant: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var expiresAt int64
	err = tx.QueryRowContext(ctx, s.ph(
		`SELECT expires_at FROM invitations WHERE id = ? AND status = 'pending' AND expires_at > ?`),
		invitationID, epoch(at)).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("replace invitation grant: load invitation: %w", err)
	}
	if grant.ExpiresAt.After(fromEpoch(expiresAt)) {
		return fmt.Errorf("invitation grant cannot outlive its invitation")
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE invitation_grants SET revoked_at = ?
		WHERE invitation_id = ? AND consumed_at = 0 AND revoked_at = 0`), epoch(at), invitationID); err != nil {
		return fmt.Errorf("replace invitation grant: revoke superseded: %w", err)
	}
	if err := insertInvitationGrant(ctx, tx, s.ph, grant); err != nil {
		return fmt.Errorf("replace invitation grant: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replace invitation grant: commit: %w", err)
	}
	return nil
}

func (s *sqlStore) AddInvitationGrant(
	ctx context.Context,
	invitationID string,
	grant invitation.Grant,
	at time.Time,
) error {
	if err := grant.Validate(); err != nil {
		return fmt.Errorf("validate invitation grant: %w", err)
	}
	if grant.InvitationID != invitationID || !grant.CreatedAt.Equal(at) {
		return fmt.Errorf("invitation grant ownership or creation time mismatch")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("add invitation grant: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var expiresAt int64
	err = tx.QueryRowContext(ctx, s.ph(
		`SELECT expires_at FROM invitations WHERE id = ? AND status = 'pending' AND expires_at > ?`),
		invitationID, epoch(at)).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("add invitation grant: load invitation: %w", err)
	}
	if grant.ExpiresAt.After(fromEpoch(expiresAt)) {
		return fmt.Errorf("invitation grant cannot outlive its invitation")
	}
	if err := insertInvitationGrant(ctx, tx, s.ph, grant); err != nil {
		return fmt.Errorf("add invitation grant: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("add invitation grant: commit: %w", err)
	}
	return nil
}

func (s *sqlStore) RevokeInvitationGrant(ctx context.Context, tokenHash string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, s.ph(`UPDATE invitation_grants SET revoked_at = ?
		WHERE token_hash = ? AND consumed_at = 0 AND revoked_at = 0`), epoch(at), tokenHash)
	if err != nil {
		return fmt.Errorf("revoke invitation grant: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *sqlStore) ListInvitationGrants(
	ctx context.Context,
	invitationID string,
) ([]invitation.Grant, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(invitationGrantSelect+
		` WHERE invitation_id = ? ORDER BY created_at, token_hash`), invitationID)
	if err != nil {
		return nil, fmt.Errorf("list invitation grants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var grants []invitation.Grant
	for rows.Next() {
		grant, err := scanInvitationGrant(rows)
		if err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func (s *sqlStore) RevokeInvitation(ctx context.Context, invitationID string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("revoke invitation: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, s.ph(`UPDATE invitations
		SET status = 'revoked', terminal_at = ?
		WHERE id = ? AND status = 'pending' AND expires_at > ?`), epoch(at), invitationID, epoch(at))
	if err != nil {
		return fmt.Errorf("revoke invitation: update: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE invitation_grants SET revoked_at = ?
		WHERE invitation_id = ? AND consumed_at = 0 AND revoked_at = 0`), epoch(at), invitationID); err != nil {
		return fmt.Errorf("revoke invitation: invalidate grants: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("revoke invitation: commit: %w", err)
	}
	return nil
}

func (s *sqlStore) PurgeTerminalInvitations(ctx context.Context, before time.Time) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("purge terminal invitations: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	terminal := `(terminal_at > 0 AND terminal_at <= ?) OR (status = 'pending' AND expires_at <= ?)`
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM contact_addresses
		WHERE owner_kind = 'invitation' AND owner_id IN (
			SELECT id FROM invitations WHERE `+terminal+`
		)`), epoch(before), epoch(before)); err != nil {
		return 0, fmt.Errorf("purge terminal invitations: contacts: %w", err)
	}
	result, err := tx.ExecContext(ctx, s.ph(`DELETE FROM invitations WHERE `+terminal),
		epoch(before), epoch(before))
	if err != nil {
		return 0, fmt.Errorf("purge terminal invitations: decisions: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purge terminal invitations: affected rows: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("purge terminal invitations: commit: %w", err)
	}
	return int(count), nil
}

func (s *sqlStore) RedeemInvitation(
	ctx context.Context,
	grantHash string,
	user User,
	session Session,
	at time.Time,
) (invitation.Invitation, error) {
	if grantHash == "" || user.ID == "" || user.Name == "" || user.PasswordHash == "" || at.IsZero() ||
		session.TokenHash == "" || session.UserID != user.ID || session.CreatedAt.IsZero() ||
		!session.ExpiresAt.After(session.CreatedAt) {
		return invitation.Invitation{}, fmt.Errorf("redeem invitation requires prepared user and session credentials")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return invitation.Invitation{}, fmt.Errorf("redeem invitation: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := `SELECT i.id, i.kind, i.username, i.library_user_id, i.display_name,
		i.identity_key, i.role, i.status, i.created_at, i.expires_at, i.terminal_at,
		i.redeemed_by, g.conveyance
		FROM invitations i JOIN invitation_grants g ON g.invitation_id = i.id
		WHERE g.token_hash = ? AND g.consumed_at = 0 AND g.revoked_at = 0 AND g.expires_at > ?
		  AND i.status = 'pending' AND i.expires_at > ?`
	if s.dialect == DialectPostgres {
		query += ` FOR UPDATE OF i, g`
	}
	var value invitation.Invitation
	var kind, role, status, conveyance string
	var createdAt, expiresAt, terminalAt int64
	err = tx.QueryRowContext(ctx, s.ph(query), grantHash, epoch(at), epoch(at)).Scan(
		&value.ID, &kind, &value.Username, &value.LibraryUserID, &value.DisplayName,
		&value.IdentityKey, &role, &status, &createdAt, &expiresAt, &terminalAt,
		&value.RedeemedBy, &conveyance)
	if err == sql.ErrNoRows {
		return invitation.Invitation{}, ErrNotFound
	}
	if err != nil {
		return invitation.Invitation{}, fmt.Errorf("redeem invitation: load grant: %w", err)
	}
	value.Kind = invitation.Kind(kind)
	value.Role = invitation.Role(role)
	value.Status = invitation.Status(status)
	value.CreatedAt, value.ExpiresAt = fromEpoch(createdAt), fromEpoch(expiresAt)
	if !redemptionIdentityMatches(value, user) {
		return invitation.Invitation{}, ErrNotFound
	}

	result, err := tx.ExecContext(ctx, s.ph(`UPDATE invitations
		SET status = 'redeemed', terminal_at = ?, redeemed_by = ?
		WHERE id = ? AND status = 'pending' AND expires_at > ?`),
		epoch(at), user.ID, value.ID, epoch(at))
	if err != nil {
		return invitation.Invitation{}, fmt.Errorf("redeem invitation: claim: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return invitation.Invitation{}, ErrNotFound
	}

	var exists int
	if value.Kind == invitation.KindLocal {
		err = tx.QueryRowContext(ctx, s.ph(
			`SELECT 1 FROM users WHERE lower(trim(name)) = ? LIMIT 1`), value.IdentityKey).Scan(&exists)
	} else {
		err = tx.QueryRowContext(ctx, s.ph(`SELECT 1 FROM users WHERE id = ? LIMIT 1`), value.IdentityKey).Scan(&exists)
	}
	if err == nil {
		return invitation.Invitation{}, ErrInvitationIdentityConflict
	}
	if err != sql.ErrNoRows {
		return invitation.Invitation{}, fmt.Errorf("redeem invitation: check allowlist: %w", err)
	}
	user.Role = Role(value.Role)
	user.Disabled = false
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO users
		(id, name, role, disabled, quota, auto_approve, media_server_linked, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), user.ID, user.Name, string(user.Role), user.Disabled,
		user.Quota, user.AutoApprove, user.MediaServerLinked, user.PasswordHash,
		epoch(user.CreatedAt), epoch(user.UpdatedAt)); err != nil {
		if isConstraintViolation(err) {
			return invitation.Invitation{}, ErrInvitationIdentityConflict
		}
		return invitation.Invitation{}, fmt.Errorf("redeem invitation: create allowlist row: %w", err)
	}

	result, err = tx.ExecContext(ctx, s.ph(`UPDATE invitation_grants SET consumed_at = ?
		WHERE token_hash = ? AND consumed_at = 0 AND revoked_at = 0`), epoch(at), grantHash)
	if err != nil {
		return invitation.Invitation{}, fmt.Errorf("redeem invitation: consume grant: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return invitation.Invitation{}, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE invitation_grants SET revoked_at = ?
		WHERE invitation_id = ? AND token_hash <> ? AND consumed_at = 0 AND revoked_at = 0`),
		epoch(at), value.ID, grantHash); err != nil {
		return invitation.Invitation{}, fmt.Errorf("redeem invitation: revoke sibling grants: %w", err)
	}

	contactUpdate := `UPDATE contact_addresses SET owner_kind = 'user', owner_id = ?
		WHERE owner_kind = 'invitation' AND owner_id = ?`
	args := []any{user.ID, value.ID}
	if invitation.Conveyance(conveyance) == invitation.ConveyanceEmail {
		contactUpdate = `UPDATE contact_addresses
			SET owner_kind = 'user', owner_id = ?, status = 'verified', verified_at = ?
			WHERE owner_kind = 'invitation' AND owner_id = ?`
		args = []any{user.ID, epoch(at), value.ID}
	}
	if _, err := tx.ExecContext(ctx, s.ph(contactUpdate), args...); err != nil {
		return invitation.Invitation{}, fmt.Errorf("redeem invitation: transfer contact: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`),
		session.TokenHash, session.UserID, epoch(session.CreatedAt), epoch(session.ExpiresAt)); err != nil {
		return invitation.Invitation{}, fmt.Errorf("redeem invitation: create session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return invitation.Invitation{}, fmt.Errorf("redeem invitation: commit: %w", err)
	}
	value.Status = invitation.StatusRedeemed
	value.TerminalAt = at
	value.RedeemedBy = user.ID
	return value, nil
}

func redemptionIdentityMatches(value invitation.Invitation, user User) bool {
	if value.Kind == invitation.KindLocal {
		return !user.MediaServerLinked && invitation.NormalizeLocalIdentity(user.Name) == value.IdentityKey
	}
	return user.MediaServerLinked && user.ID == value.LibraryUserID
}

func (s *sqlStore) GetInvitation(
	ctx context.Context,
	id string,
	now time.Time,
) (invitation.Invitation, error) {
	value, err := scanInvitation(s.db.QueryRowContext(ctx, s.ph(invitationSelect+` WHERE id = ?`), id))
	if err != nil {
		return invitation.Invitation{}, err
	}
	return effectiveInvitation(value, now), nil
}

func (s *sqlStore) ListInvitations(ctx context.Context, now time.Time) ([]invitation.Invitation, error) {
	rows, err := s.db.QueryContext(ctx, invitationSelect+` ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var values []invitation.Invitation
	for rows.Next() {
		value, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, effectiveInvitation(value, now))
	}
	return values, rows.Err()
}

func scanInvitation(sc scannable) (invitation.Invitation, error) {
	var value invitation.Invitation
	var kind, role, status string
	var createdAt, expiresAt, terminalAt int64
	err := sc.Scan(&value.ID, &kind, &value.Username, &value.LibraryUserID, &value.DisplayName,
		&value.IdentityKey, &role, &status, &createdAt, &expiresAt, &terminalAt, &value.RedeemedBy)
	if err == sql.ErrNoRows {
		return invitation.Invitation{}, ErrNotFound
	}
	if err != nil {
		return invitation.Invitation{}, err
	}
	value.Kind = invitation.Kind(kind)
	value.Role = invitation.Role(role)
	value.Status = invitation.Status(status)
	value.CreatedAt = fromEpoch(createdAt)
	value.ExpiresAt = fromEpoch(expiresAt)
	if terminalAt > 0 {
		value.TerminalAt = fromEpoch(terminalAt)
	}
	return value, nil
}

func effectiveInvitation(value invitation.Invitation, now time.Time) invitation.Invitation {
	if value.EffectiveStatus(now) == invitation.StatusExpired {
		value.Status = invitation.StatusExpired
		value.TerminalAt = value.ExpiresAt
	}
	return value
}

func insertInvitationGrant(
	ctx context.Context,
	tx *sql.Tx,
	placeholder func(string) string,
	grant invitation.Grant,
) error {
	_, err := tx.ExecContext(ctx, placeholder(`INSERT INTO invitation_grants
		(token_hash, invitation_id, grant_kind, conveyance, created_at, expires_at, consumed_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`), grant.TokenHash, grant.InvitationID, string(grant.Kind),
		string(grant.Conveyance), epoch(grant.CreatedAt), epoch(grant.ExpiresAt),
		epoch(grant.ConsumedAt), epoch(grant.RevokedAt))
	return err
}

func scanInvitationGrant(sc scannable) (invitation.Grant, error) {
	var grant invitation.Grant
	var kind, conveyance string
	var createdAt, expiresAt, consumedAt, revokedAt int64
	err := sc.Scan(&grant.TokenHash, &grant.InvitationID, &kind, &conveyance,
		&createdAt, &expiresAt, &consumedAt, &revokedAt)
	if err == sql.ErrNoRows {
		return invitation.Grant{}, ErrNotFound
	}
	if err != nil {
		return invitation.Grant{}, err
	}
	grant.Kind = invitation.GrantKind(kind)
	grant.Conveyance = invitation.Conveyance(conveyance)
	grant.CreatedAt = fromEpoch(createdAt)
	grant.ExpiresAt = fromEpoch(expiresAt)
	if consumedAt > 0 {
		grant.ConsumedAt = fromEpoch(consumedAt)
	}
	if revokedAt > 0 {
		grant.RevokedAt = fromEpoch(revokedAt)
	}
	return grant, nil
}
