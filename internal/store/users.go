package store

import (
	"context"
	"database/sql"
	"time"
)

// Role is a user's authorization level (§11).
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// User is a media-server-derived account with local role/quota (§11). Keyed by
// media-server user id.
type User struct {
	ID          string
	Name        string
	Role        Role
	Disabled    bool
	Quota       int // pending-acquisition cap; 0 = default
	AutoApprove bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Session is a revocable session row (§11); the token is SHA-256-hashed at rest.
type Session struct {
	TokenHash string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// --- users ---

func (s *sqlStore) GetUser(ctx context.Context, id string) (User, error) {
	row := s.db.QueryRowContext(ctx, s.ph(
		`SELECT id, name, role, disabled, quota, auto_approve, created_at, updated_at
		 FROM users WHERE id = ?`), id)
	return scanUser(row)
}

func (s *sqlStore) UpsertUser(ctx context.Context, u User) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO users (id, name, role, disabled, quota, auto_approve, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name=excluded.name, role=excluded.role, disabled=excluded.disabled,
		   quota=excluded.quota, auto_approve=excluded.auto_approve, updated_at=excluded.updated_at`),
		u.ID, u.Name, string(u.Role), u.Disabled, u.Quota, u.AutoApprove,
		epoch(u.CreatedAt), epoch(u.UpdatedAt))
	return err
}

func (s *sqlStore) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(
		`SELECT id, name, role, disabled, quota, auto_approve, created_at, updated_at
		 FROM users ORDER BY name`))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountAdmins reports how many enabled admins exist — powers the first-admin
// bootstrap decision (§11).
func (s *sqlStore) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.ph(
		`SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = ?`), false).Scan(&n)
	return n, err
}

// --- sessions ---

func (s *sqlStore) CreateSession(ctx context.Context, sess Session) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`),
		sess.TokenHash, sess.UserID, epoch(sess.CreatedAt), epoch(sess.ExpiresAt))
	return err
}

// GetSession returns the session for a token hash if it exists and is unexpired
// as of now. Expired sessions are treated as not found (the janitor deletes them
// eventually; this makes expiry immediate for auth).
func (s *sqlStore) GetSession(ctx context.Context, tokenHash string, now time.Time) (Session, error) {
	row := s.db.QueryRowContext(ctx, s.ph(
		`SELECT token_hash, user_id, created_at, expires_at
		 FROM sessions WHERE token_hash = ? AND expires_at > ?`), tokenHash, epoch(now))
	var sess Session
	var created, expires int64
	err := row.Scan(&sess.TokenHash, &sess.UserID, &created, &expires)
	if err == sql.ErrNoRows {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	sess.CreatedAt = fromEpoch(created)
	sess.ExpiresAt = fromEpoch(expires)
	return sess, nil
}

// TouchSession extends a session's sliding expiry (§11).
func (s *sqlStore) TouchSession(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE sessions SET expires_at = ? WHERE token_hash = ?`), epoch(expiresAt), tokenHash)
	return err
}

// RevokeSession deletes one session (logout).
func (s *sqlStore) RevokeSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM sessions WHERE token_hash = ?`), tokenHash)
	return err
}

// RevokeSessionsForUser deletes all of a user's sessions — called when a user is
// disabled (§11: disabling a user kills their sessions immediately).
func (s *sqlStore) RevokeSessionsForUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM sessions WHERE user_id = ?`), userID)
	return err
}

// PurgeExpiredSessions removes sessions past their expiry (janitor, §5).
func (s *sqlStore) PurgeExpiredSessions(ctx context.Context, now time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM sessions WHERE expires_at <= ?`), epoch(now))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanUser(sc scannable) (User, error) {
	var u User
	var role string
	var created, updated int64
	err := sc.Scan(&u.ID, &u.Name, &role, &u.Disabled, &u.Quota, &u.AutoApprove, &created, &updated)
	if err == sql.ErrNoRows {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.Role = Role(role)
	u.CreatedAt = fromEpoch(created)
	u.UpdatedAt = fromEpoch(updated)
	return u, nil
}
