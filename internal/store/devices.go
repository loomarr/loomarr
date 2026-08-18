package store

import (
	"context"
	"database/sql"
	"time"
)

// DevicePairing is an in-flight pairing: a code shown on a TV, waiting for a human to approve it in
// the web UI. It is short-lived by construction — see the migration for why the two halves of
// pairing have different lifetimes.
type DevicePairing struct {
	DeviceCodeHash string // SHA-256 of the device's polling secret; never the plaintext
	UserCode       string // the short code shown on screen and typed by a human
	UserID         string // empty while pending; set when approved
	DeviceName     string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	ApprovedAt     time.Time // zero while pending
}

// Approved reports whether a human has completed this pairing.
//
// Derived from ApprovedAt rather than stored separately so there is ONE source of truth: a boolean
// column alongside a timestamp is two facts that can disagree, and the disagreement would decide
// whether a credential is issued.
func (p DevicePairing) Approved() bool { return !p.ApprovedAt.IsZero() }

// DeviceToken is a paired device's durable credential. It mirrors Session (revocable row, hash at
// rest) with one deliberate difference: no expiry. A TV idle for a month must not log itself out;
// it is revoked explicitly instead.
type DeviceToken struct {
	TokenHash  string
	UserID     string
	DeviceName string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

// --- device pairings ---

func (s *sqlStore) CreateDevicePairing(ctx context.Context, p DevicePairing) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO device_pairings
		   (device_code, user_code, user_id, device_name, created_at, expires_at, approved_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`),
		p.DeviceCodeHash, p.UserCode, nullableUser(p.UserID), p.DeviceName,
		epoch(p.CreatedAt), epoch(p.ExpiresAt), epoch(p.ApprovedAt))
	return err
}

// GetDevicePairing resolves a pairing by its device-code hash, treating an expired row as absent on
// the same "expiry is immediate, purging is eventual" rule GetSession uses.
func (s *sqlStore) GetDevicePairing(ctx context.Context, codeHash string, now time.Time) (DevicePairing, error) {
	return s.scanPairing(s.db.QueryRowContext(ctx, s.ph(
		`SELECT device_code, user_code, user_id, device_name, created_at, expires_at, approved_at
		 FROM device_pairings WHERE device_code = ? AND expires_at > ?`), codeHash, epoch(now)))
}

// GetDevicePairingByUserCode resolves the pairing a human is approving. The user code is what they
// read off the TV, so this is the lookup the approval endpoint uses.
func (s *sqlStore) GetDevicePairingByUserCode(ctx context.Context, userCode string, now time.Time) (DevicePairing, error) {
	return s.scanPairing(s.db.QueryRowContext(ctx, s.ph(
		`SELECT device_code, user_code, user_id, device_name, created_at, expires_at, approved_at
		 FROM device_pairings WHERE user_code = ? AND expires_at > ?`), userCode, epoch(now)))
}

func (s *sqlStore) scanPairing(row *sql.Row) (DevicePairing, error) {
	var p DevicePairing
	var user sql.NullString
	var created, expires, approved int64
	err := row.Scan(&p.DeviceCodeHash, &p.UserCode, &user, &p.DeviceName, &created, &expires, &approved)
	if err == sql.ErrNoRows {
		return DevicePairing{}, ErrNotFound
	}
	if err != nil {
		return DevicePairing{}, err
	}
	p.UserID = user.String
	p.CreatedAt = fromEpoch(created)
	p.ExpiresAt = fromEpoch(expires)
	p.ApprovedAt = fromEpoch(approved)
	return p, nil
}

// ApproveDevicePairing records that a user approved a pending pairing.
//
// ⚠ The `approved_at = 0` predicate is the guard that makes approval single-use at the DATABASE, not
// merely in the handler. Two humans racing on the same code, or one clicking twice, must produce one
// credential — a check-then-write in Go would let both through.
func (s *sqlStore) ApproveDevicePairing(ctx context.Context, userCode, userID string, at time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE device_pairings SET user_id = ?, approved_at = ?
		 WHERE user_code = ? AND approved_at = 0 AND expires_at > ?`),
		userID, epoch(at), userCode, epoch(at))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// DeleteDevicePairing consumes a pairing once its token has been issued.
func (s *sqlStore) DeleteDevicePairing(ctx context.Context, codeHash string) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM device_pairings WHERE device_code = ?`), codeHash)
	return err
}

// PurgeExpiredDevicePairings sweeps codes nobody completed.
func (s *sqlStore) PurgeExpiredDevicePairings(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM device_pairings WHERE expires_at <= ?`), epoch(now))
	return err
}

// --- device tokens ---

func (s *sqlStore) CreateDeviceToken(ctx context.Context, t DeviceToken) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO device_tokens (token_hash, user_id, device_name, created_at, last_seen_at)
		 VALUES (?, ?, ?, ?, ?)`),
		t.TokenHash, t.UserID, t.DeviceName, epoch(t.CreatedAt), epoch(t.LastSeenAt))
	return err
}

// GetDeviceToken resolves a device credential. Unlike GetSession there is no expiry predicate: a
// device token lives until it is revoked.
func (s *sqlStore) GetDeviceToken(ctx context.Context, tokenHash string) (DeviceToken, error) {
	row := s.db.QueryRowContext(ctx, s.ph(
		`SELECT token_hash, user_id, device_name, created_at, last_seen_at
		 FROM device_tokens WHERE token_hash = ?`), tokenHash)
	var t DeviceToken
	var created, seen int64
	err := row.Scan(&t.TokenHash, &t.UserID, &t.DeviceName, &created, &seen)
	if err == sql.ErrNoRows {
		return DeviceToken{}, ErrNotFound
	}
	if err != nil {
		return DeviceToken{}, err
	}
	t.CreatedAt = fromEpoch(created)
	t.LastSeenAt = fromEpoch(seen)
	return t, nil
}

// TouchDeviceToken records that a device was seen, so the revocation UI can show what is actually in
// use. Best-effort by design: a failure here must never fail the request that carried the token.
func (s *sqlStore) TouchDeviceToken(ctx context.Context, tokenHash string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE device_tokens SET last_seen_at = ? WHERE token_hash = ?`), epoch(at), tokenHash)
	return err
}

// ListDeviceTokensForUser returns a user's paired devices, newest first — the revocation UI's read.
func (s *sqlStore) ListDeviceTokensForUser(ctx context.Context, userID string) ([]DeviceToken, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(
		`SELECT token_hash, user_id, device_name, created_at, last_seen_at
		 FROM device_tokens WHERE user_id = ? ORDER BY created_at DESC`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DeviceToken
	for rows.Next() {
		var t DeviceToken
		var created, seen int64
		if err := rows.Scan(&t.TokenHash, &t.UserID, &t.DeviceName, &created, &seen); err != nil {
			return nil, err
		}
		t.CreatedAt = fromEpoch(created)
		t.LastSeenAt = fromEpoch(seen)
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteDeviceToken revokes one device. Scoped by user so a caller can only revoke a device it owns
// — the authorisation is in the WHERE clause, not left to the handler.
func (s *sqlStore) DeleteDeviceToken(ctx context.Context, tokenHash, userID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM device_tokens WHERE token_hash = ? AND user_id = ?`), tokenHash, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// nullableUser keeps a pending pairing's user_id NULL rather than "", so the foreign key stays
// satisfiable while nobody has approved it yet.
func nullableUser(id string) any {
	if id == "" {
		return nil
	}
	return id
}
