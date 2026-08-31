package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/recovery"
)

const passwordRecoverySelect = `SELECT id, user_id, status, created_at, expires_at, terminal_at
 FROM password_recoveries`

const passwordRecoveryGrantSelect = `SELECT token_hash, recovery_id, created_at, expires_at,
 consumed_at, revoked_at FROM password_recovery_grants`

func (s *sqlStore) CreatePasswordRecovery(ctx context.Context, value recovery.Record) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("validate password recovery: %w", err)
	}
	if value.Status != recovery.StatusPending {
		return fmt.Errorf("new password recovery must be pending")
	}
	return s.withPasswordRecoveryLock(ctx, value.UserID, func(ctx context.Context) error {
		return s.createPasswordRecovery(ctx, value)
	})
}

func (s *sqlStore) createPasswordRecovery(ctx context.Context, value recovery.Record) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("create password recovery: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var eligible int
	err = tx.QueryRowContext(ctx, s.ph(`SELECT 1 FROM users
		WHERE id = ? AND disabled = ? AND media_server_linked = ? AND password_hash <> ''`),
		value.UserID, false, false).Scan(&eligible)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("create password recovery: check local person: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE password_recovery_grants SET revoked_at = ?
		WHERE recovery_id IN (SELECT id FROM password_recoveries
			WHERE user_id = ? AND status = 'pending')
		  AND consumed_at = 0 AND revoked_at = 0`), epoch(value.CreatedAt), value.UserID); err != nil {
		return fmt.Errorf("create password recovery: revoke earlier grants: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE password_recoveries
		SET status = CASE WHEN expires_at <= ? THEN 'expired' ELSE 'revoked' END,
		    terminal_at = CASE WHEN expires_at <= ? THEN expires_at ELSE ? END
		WHERE user_id = ? AND status = 'pending'`), epoch(value.CreatedAt), epoch(value.CreatedAt),
		epoch(value.CreatedAt), value.UserID); err != nil {
		return fmt.Errorf("create password recovery: supersede earlier request: %w", err)
	}
	_, err = tx.ExecContext(ctx, s.ph(`INSERT INTO password_recoveries
		(id, user_id, status, created_at, expires_at, terminal_at) VALUES (?, ?, ?, ?, ?, ?)`),
		value.ID, value.UserID, string(value.Status), epoch(value.CreatedAt), epoch(value.ExpiresAt), 0)
	if err != nil {
		return fmt.Errorf("create password recovery: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("create password recovery: commit: %w", err)
	}
	return nil
}

func (s *sqlStore) withPasswordRecoveryLock(
	ctx context.Context,
	userID string,
	fn func(context.Context) error,
) error {
	if s.dialect == DialectPostgres {
		return s.withPostgresAdvisoryLock(ctx, "password recovery lock", "password-recovery:", userID, fn)
	}
	value, _ := s.passwordRecoveryLocks.LoadOrStore(userID, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	return fn(ctx)
}

func (s *sqlStore) GetPasswordRecovery(
	ctx context.Context,
	id string,
	now time.Time,
) (recovery.Record, error) {
	value, err := scanPasswordRecovery(s.db.QueryRowContext(ctx, s.ph(passwordRecoverySelect+
		` WHERE id = ?`), id))
	if err != nil {
		return recovery.Record{}, err
	}
	return effectivePasswordRecovery(value, now), nil
}

// GetPasswordRecoveryByGrant is read-only: merely opening a recovery link cannot consume it.
func (s *sqlStore) GetPasswordRecoveryByGrant(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) (recovery.Record, error) {
	const byGrant = `SELECT r.id, r.user_id, r.status, r.created_at, r.expires_at, r.terminal_at
		FROM password_recoveries r
		JOIN password_recovery_grants g ON g.recovery_id = r.id
		JOIN users u ON u.id = r.user_id
		WHERE g.token_hash = ? AND g.consumed_at = 0 AND g.revoked_at = 0
		  AND g.expires_at > ? AND r.status = 'pending' AND r.expires_at > ?
		  AND u.disabled = ? AND u.media_server_linked = ? AND u.password_hash <> ''`
	return scanPasswordRecovery(s.db.QueryRowContext(ctx, s.ph(byGrant),
		tokenHash, epoch(now), epoch(now), false, false))
}

func (s *sqlStore) AddPasswordRecoveryGrant(
	ctx context.Context,
	recoveryID string,
	grant recovery.Grant,
	at time.Time,
) error {
	if err := grant.Validate(); err != nil {
		return fmt.Errorf("validate password recovery grant: %w", err)
	}
	if grant.RecoveryID != recoveryID || !grant.CreatedAt.Equal(at) {
		return fmt.Errorf("password recovery grant ownership or creation time mismatch")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("add password recovery grant: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var expiresAt int64
	err = tx.QueryRowContext(ctx, s.ph(`SELECT expires_at FROM password_recoveries
		WHERE id = ? AND status = 'pending' AND expires_at > ?`), recoveryID, epoch(at)).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("add password recovery grant: load recovery: %w", err)
	}
	if grant.ExpiresAt.After(fromEpoch(expiresAt)) {
		return fmt.Errorf("password recovery grant cannot outlive its recovery")
	}
	_, err = tx.ExecContext(ctx, s.ph(`INSERT INTO password_recovery_grants
		(token_hash, recovery_id, created_at, expires_at, consumed_at, revoked_at)
		VALUES (?, ?, ?, ?, ?, ?)`), grant.TokenHash, grant.RecoveryID, epoch(grant.CreatedAt),
		epoch(grant.ExpiresAt), epoch(grant.ConsumedAt), epoch(grant.RevokedAt))
	if err != nil {
		return fmt.Errorf("add password recovery grant: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("add password recovery grant: commit: %w", err)
	}
	return nil
}

func (s *sqlStore) RevokePasswordRecoveryGrant(ctx context.Context, tokenHash string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, s.ph(`UPDATE password_recovery_grants SET revoked_at = ?
		WHERE token_hash = ? AND consumed_at = 0 AND revoked_at = 0`), epoch(at), tokenHash)
	if err != nil {
		return fmt.Errorf("revoke password recovery grant: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *sqlStore) ListPasswordRecoveryGrants(
	ctx context.Context,
	recoveryID string,
) ([]recovery.Grant, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(passwordRecoveryGrantSelect+
		` WHERE recovery_id = ? ORDER BY created_at, token_hash`), recoveryID)
	if err != nil {
		return nil, fmt.Errorf("list password recovery grants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var values []recovery.Grant
	for rows.Next() {
		value, err := scanPasswordRecoveryGrant(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// RedeemPasswordRecovery changes the local verifier, revokes every session, consumes the winning
// grant, and revokes every sibling recovery grant in one transaction. A caller therefore cannot
// observe the new password while an old session or alternate recovery bearer remains usable.
func (s *sqlStore) RedeemPasswordRecovery(
	ctx context.Context,
	grantHash string,
	passwordHash string,
	at time.Time,
) (recovery.Record, error) {
	if grantHash == "" || passwordHash == "" || at.IsZero() {
		return recovery.Record{}, fmt.Errorf("redeem password recovery requires prepared credentials")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return recovery.Record{}, fmt.Errorf("redeem password recovery: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := `SELECT r.id, r.user_id, r.status, r.created_at, r.expires_at, r.terminal_at
		FROM password_recoveries r
		JOIN password_recovery_grants g ON g.recovery_id = r.id
		JOIN users u ON u.id = r.user_id
		WHERE g.token_hash = ? AND g.consumed_at = 0 AND g.revoked_at = 0
		  AND g.expires_at > ? AND r.status = 'pending' AND r.expires_at > ?
		  AND u.disabled = ? AND u.media_server_linked = ? AND u.password_hash <> ''`
	if s.dialect == DialectPostgres {
		query += ` FOR UPDATE OF r, g, u`
	}
	value, err := scanPasswordRecovery(tx.QueryRowContext(ctx, s.ph(query),
		grantHash, epoch(at), epoch(at), false, false))
	if err != nil {
		if err == ErrNotFound {
			return recovery.Record{}, ErrNotFound
		}
		return recovery.Record{}, fmt.Errorf("redeem password recovery: load grant: %w", err)
	}

	result, err := tx.ExecContext(ctx, s.ph(`UPDATE password_recoveries
		SET status = 'redeemed', terminal_at = ?
		WHERE id = ? AND status = 'pending' AND expires_at > ?`), epoch(at), value.ID, epoch(at))
	if err != nil {
		return recovery.Record{}, fmt.Errorf("redeem password recovery: claim: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return recovery.Record{}, ErrNotFound
	}
	result, err = tx.ExecContext(ctx, s.ph(`UPDATE users SET password_hash = ?, updated_at = ?
		WHERE id = ? AND disabled = ? AND media_server_linked = ?`),
		passwordHash, epoch(at), value.UserID, false, false)
	if err != nil {
		return recovery.Record{}, fmt.Errorf("redeem password recovery: update verifier: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return recovery.Record{}, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, s.ph(`DELETE FROM sessions WHERE user_id = ?`), value.UserID); err != nil {
		return recovery.Record{}, fmt.Errorf("redeem password recovery: revoke sessions: %w", err)
	}
	result, err = tx.ExecContext(ctx, s.ph(`UPDATE password_recovery_grants SET consumed_at = ?
		WHERE token_hash = ? AND consumed_at = 0 AND revoked_at = 0`), epoch(at), grantHash)
	if err != nil {
		return recovery.Record{}, fmt.Errorf("redeem password recovery: consume grant: %w", err)
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return recovery.Record{}, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, s.ph(`UPDATE password_recovery_grants SET revoked_at = ?
		WHERE recovery_id IN (SELECT id FROM password_recoveries WHERE user_id = ?)
		  AND token_hash <> ? AND consumed_at = 0 AND revoked_at = 0`),
		epoch(at), value.UserID, grantHash); err != nil {
		return recovery.Record{}, fmt.Errorf("redeem password recovery: revoke outstanding grants: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return recovery.Record{}, fmt.Errorf("redeem password recovery: commit: %w", err)
	}
	value.Status = recovery.StatusRedeemed
	value.TerminalAt = at
	return value, nil
}

// PurgeTerminalPasswordRecoveries removes retained audit rows only after their fixed window.
// Pending records are eligible only when their expiry itself is older than the horizon; grants
// follow through ON DELETE CASCADE.
func (s *sqlStore) PurgeTerminalPasswordRecoveries(
	ctx context.Context,
	before time.Time,
) (int, error) {
	result, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM password_recoveries WHERE
		(terminal_at > 0 AND terminal_at < ?) OR
		(status = 'pending' AND expires_at < ?)`), epoch(before), epoch(before))
	if err != nil {
		return 0, fmt.Errorf("purge terminal password recoveries: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purge terminal password recoveries: affected rows: %w", err)
	}
	return int(count), nil
}

func effectivePasswordRecovery(value recovery.Record, now time.Time) recovery.Record {
	if value.EffectiveStatus(now) == recovery.StatusExpired {
		value.Status = recovery.StatusExpired
		value.TerminalAt = value.ExpiresAt
	}
	return value
}

func scanPasswordRecovery(sc scannable) (recovery.Record, error) {
	var value recovery.Record
	var status string
	var createdAt, expiresAt, terminalAt int64
	err := sc.Scan(&value.ID, &value.UserID, &status, &createdAt, &expiresAt, &terminalAt)
	if err == sql.ErrNoRows {
		return recovery.Record{}, ErrNotFound
	}
	if err != nil {
		return recovery.Record{}, err
	}
	value.Status = recovery.Status(status)
	value.CreatedAt = fromEpoch(createdAt)
	value.ExpiresAt = fromEpoch(expiresAt)
	if terminalAt > 0 {
		value.TerminalAt = fromEpoch(terminalAt)
	}
	return value, nil
}

func scanPasswordRecoveryGrant(sc scannable) (recovery.Grant, error) {
	var value recovery.Grant
	var createdAt, expiresAt, consumedAt, revokedAt int64
	err := sc.Scan(&value.TokenHash, &value.RecoveryID, &createdAt, &expiresAt, &consumedAt, &revokedAt)
	if err == sql.ErrNoRows {
		return recovery.Grant{}, ErrNotFound
	}
	if err != nil {
		return recovery.Grant{}, err
	}
	value.CreatedAt = fromEpoch(createdAt)
	value.ExpiresAt = fromEpoch(expiresAt)
	if consumedAt > 0 {
		value.ConsumedAt = fromEpoch(consumedAt)
	}
	if revokedAt > 0 {
		value.RevokedAt = fromEpoch(revokedAt)
	}
	return value, nil
}
