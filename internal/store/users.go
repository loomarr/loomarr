package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/invitation"
)

// Role is a user's authorization level (§11).
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// User is a Loomarr account with local role/quota (§11). Loomarr owns identity:
// the row IS the allowlist. Keyed by user id — the media-server user id for an
// imported user, a Loomarr-minted id for a local one.
type User struct {
	ID          string
	Name        string
	Role        Role
	Disabled    bool
	Quota       int // pending-acquisition cap; 0 = default
	AutoApprove bool
	// MediaServerLinked is independent from PasswordHash: an imported user may
	// carry an offline verifier after a successful provider login (§11).
	MediaServerLinked bool
	// PasswordHash is an Argon2id PHC verifier. It is never exposed by the API.
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Session is a revocable session row (§11); the token is SHA-256-hashed at rest.
type Session struct {
	TokenHash string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
	// LastSeenAt and ClientLabel ("Firefox on macOS") are the last authenticated use (#1667);
	// zero and "" until the session is first used after the column arrived.
	LastSeenAt  time.Time
	ClientLabel string
}

// --- users ---

func (s *sqlStore) GetUser(ctx context.Context, id string) (User, error) {
	row := s.db.QueryRowContext(ctx, s.ph(
		`SELECT id, name, role, disabled, quota, auto_approve, media_server_linked, password_hash, created_at, updated_at
		 FROM users WHERE id = ?`), id)
	return scanUser(row)
}

// UpsertUser writes a user. A sync of an imported user passes an empty
// PasswordHash; COALESCE preserves any existing hash so a re-sync never wipes a
// local user's credentials (defense in depth — imported and local users are
// distinct rows, but the COALESCE makes the invariant structural).
// GetUserByName returns a user by name (§11: local login resolves the username to
// the allowlist row). Names are unique per install by convention; the first match
// wins deterministically (ORDER BY id). ErrNotFound when absent.
func (s *sqlStore) GetUserByName(ctx context.Context, name string) (User, error) {
	row := s.db.QueryRowContext(ctx, s.ph(
		`SELECT id, name, role, disabled, quota, auto_approve, media_server_linked, password_hash, created_at, updated_at
		 FROM users WHERE name = ? ORDER BY id LIMIT 1`), name)
	return scanUser(row)
}

func (s *sqlStore) UpsertUser(ctx context.Context, u User) error {
	var hash any
	if u.PasswordHash != "" {
		hash = u.PasswordHash
	}
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO users (id, name, role, disabled, quota, auto_approve, media_server_linked, password_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name=excluded.name, role=excluded.role, disabled=excluded.disabled,
		   quota=excluded.quota, auto_approve=excluded.auto_approve,
		   media_server_linked=excluded.media_server_linked,
		   password_hash=COALESCE(excluded.password_hash, users.password_hash),
		   updated_at=excluded.updated_at`),
		u.ID, u.Name, string(u.Role), u.Disabled, u.Quota, u.AutoApprove, u.MediaServerLinked, hash,
		epoch(u.CreatedAt), epoch(u.UpdatedAt))
	return err
}

func (s *sqlStore) CreateUserUnlessInvited(ctx context.Context, u User, now time.Time) error {
	kind := invitation.KindLocal
	identityKey := invitation.NormalizeLocalIdentity(u.Name)
	if u.MediaServerLinked {
		kind = invitation.KindLibrary
		identityKey = u.ID
	}
	return s.withInvitationIdentityLock(ctx, kind, identityKey, func(ctx context.Context) error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("create user: begin: %w", err)
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.ExecContext(ctx, s.ph(`UPDATE invitations
			SET status = 'expired', terminal_at = expires_at
			WHERE kind = ? AND identity_key = ? AND status = 'pending' AND expires_at <= ?`),
			string(kind), identityKey, epoch(now)); err != nil {
			return fmt.Errorf("create user: expire reservation: %w", err)
		}
		var exists int
		err = tx.QueryRowContext(ctx, s.ph(`SELECT 1 FROM invitations
			WHERE kind = ? AND identity_key = ? AND status = 'pending' LIMIT 1`),
			string(kind), identityKey).Scan(&exists)
		if err == nil {
			return ErrInvitationIdentityConflict
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("create user: check reservation: %w", err)
		}
		var hash any
		if u.PasswordHash != "" {
			hash = u.PasswordHash
		}
		_, err = tx.ExecContext(ctx, s.ph(`INSERT INTO users
			(id, name, role, disabled, quota, auto_approve, media_server_linked, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			u.ID, u.Name, string(u.Role), u.Disabled, u.Quota, u.AutoApprove, u.MediaServerLinked,
			hash, epoch(u.CreatedAt), epoch(u.UpdatedAt))
		if err != nil {
			return fmt.Errorf("create user: insert: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("create user: commit: %w", err)
		}
		return nil
	})
}

func (s *sqlStore) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(
		`SELECT id, name, role, disabled, quota, auto_approve, media_server_linked, password_hash, created_at, updated_at
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

// CreateSession writes a session for an enabled user. It returns ErrNotFound when the user is unknown
// or disabled, which login maps to the same refusal as a disabled login. The check is part of the
// INSERT (see enabledUserSQL), so a disable that lands between login's Disabled check and this write
// cannot leave a session behind for a later re-enable to revive.
func (s *sqlStore) CreateSession(ctx context.Context, sess Session) error {
	res, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at)
		 SELECT CAST(? AS TEXT), CAST(? AS TEXT), CAST(? AS BIGINT), CAST(? AS BIGINT)
		 WHERE EXISTS (`+s.enabledUserSQL()+`)`),
		sess.TokenHash, sess.UserID, epoch(sess.CreatedAt), epoch(sess.ExpiresAt), sess.UserID, false)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// GetSession returns the session for a token hash if it exists and is unexpired
// as of now. Expired sessions are treated as not found (the janitor deletes them
// eventually; this makes expiry immediate for auth).
func (s *sqlStore) GetSession(ctx context.Context, tokenHash string, now time.Time) (Session, error) {
	row := s.db.QueryRowContext(ctx, s.ph(
		`SELECT `+sessionColumns+` FROM sessions WHERE token_hash = ? AND expires_at > ?`), tokenHash, epoch(now))
	sess, err := scanSession(row)
	if err == sql.ErrNoRows {
		return Session{}, ErrNotFound
	}
	return sess, err
}

const sessionColumns = `token_hash, user_id, created_at, expires_at, last_seen_at, client_label`

func scanSession(sc scannable) (Session, error) {
	var sess Session
	var created, expires, seen int64
	if err := sc.Scan(&sess.TokenHash, &sess.UserID, &created, &expires, &seen, &sess.ClientLabel); err != nil {
		return Session{}, err
	}
	sess.CreatedAt, sess.ExpiresAt, sess.LastSeenAt = fromEpoch(created), fromEpoch(expires), fromEpoch(seen)
	return sess, nil
}

// SessionSeen is one authenticated use of a session: when, the slid expiry, and the client's
// coarse label ("" when the request didn't say, which keeps the label the session has).
type SessionSeen struct {
	At          time.Time
	ExpiresAt   time.Time
	ClientLabel string
}

// lastSeenGranularity throttles the person-level write: "Last seen" reads "now", "2h ago", so a
// minute is plenty and most requests skip the users write.
const lastSeenGranularity = time.Minute

// TouchSession records a use of a session: it slides the expiry (§11) and notes when and from what
// client it was used, plus the person's last-seen (#1667).
func (s *sqlStore) TouchSession(ctx context.Context, tokenHash string, seen SessionSeen) error {
	if _, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE sessions SET expires_at = ?, last_seen_at = ?,
		   client_label = CASE WHEN ? <> '' THEN ? ELSE client_label END
		 WHERE token_hash = ?`),
		epoch(seen.ExpiresAt), epoch(seen.At), seen.ClientLabel, seen.ClientLabel, tokenHash); err != nil {
		return err
	}
	return s.bumpUserLastSeen(ctx, `SELECT user_id FROM sessions WHERE token_hash = ?`, tokenHash, seen.At)
}

// bumpUserLastSeen moves the owner of a session or device token (ownerQuery) to `at`, at most
// once per lastSeenGranularity.
func (s *sqlStore) bumpUserLastSeen(ctx context.Context, ownerQuery, tokenHash string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE users SET last_seen_at = ? WHERE id = (`+ownerQuery+`) AND last_seen_at < ?`),
		epoch(at), tokenHash, epoch(at.Add(-lastSeenGranularity)))
	return err
}

// UserLastSeen maps each person who has been seen to when (#1667, the People roster). One query;
// someone never seen since the column arrived has no entry.
func (s *sqlStore) UserLastSeen(ctx context.Context) (map[string]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, last_seen_at FROM users WHERE last_seen_at > 0`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]time.Time{}
	for rows.Next() {
		var id string
		var at int64
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out[id] = fromEpoch(at)
	}
	return out, rows.Err()
}

// ListSessionsForUser returns a user's live sessions, newest first. Expired rows are
// excluded on the same "expiry is immediate, purging is eventual" rule GetSession uses
// (§11) — an admin reviewing who is signed in must not see sessions that can no longer
// authenticate, or they would revoke things that were already dead and mistrust the list.
func (s *sqlStore) ListSessionsForUser(ctx context.Context, userID string, now time.Time) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(
		`SELECT `+sessionColumns+` FROM sessions WHERE user_id = ? AND expires_at > ?
		 ORDER BY created_at DESC`), userID, epoch(now))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
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
	var hash sql.NullString
	var created, updated int64
	err := sc.Scan(&u.ID, &u.Name, &role, &u.Disabled, &u.Quota, &u.AutoApprove, &u.MediaServerLinked, &hash, &created, &updated)
	if err == sql.ErrNoRows {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.Role = Role(role)
	u.PasswordHash = hash.String
	u.CreatedAt = fromEpoch(created)
	u.UpdatedAt = fromEpoch(updated)
	return u, nil
}
