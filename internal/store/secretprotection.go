package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/secretprotection"
)

const secretDataKeyLockName = "loomarr:secret-data-key"

func (s *sqlStore) EnsureInstallationKeyFingerprint(ctx context.Context, fingerprint string) error {
	if fingerprint == "" {
		return errors.New("secret protection: empty installation-key fingerprint")
	}
	s.secretKeyLock.Lock()
	defer s.secretKeyLock.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin installation-key verification: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.lockSecretDataKeys(ctx, tx); err != nil {
		return err
	}
	var stored string
	err = tx.QueryRowContext(ctx, `SELECT installation_key_fingerprint
		FROM secret_protection_metadata WHERE singleton = 1`).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO secret_protection_metadata
			(singleton, installation_key_fingerprint) VALUES (1, ?)`), fingerprint); err != nil {
			return fmt.Errorf("store installation-key fingerprint: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("read installation-key fingerprint: %w", err)
	} else if stored != fingerprint {
		return secretprotection.ErrInstallationKeyMismatch
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit installation-key verification: %w", err)
	}
	return nil
}

func (s *sqlStore) EnsureSecretDataKey(ctx context.Context, candidate secretprotection.WrappedDataKey) (result secretprotection.WrappedDataKey, retErr error) {
	if err := validateWrappedDataKey(candidate); err != nil {
		return result, err
	}
	s.secretKeyLock.Lock()
	defer s.secretKeyLock.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin secret data-key initialization: %w", err)
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, tx.Rollback())
		}
	}()
	if err := s.lockSecretDataKeys(ctx, tx); err != nil {
		return result, err
	}
	result, err = scanSecretDataKey(tx.QueryRowContext(ctx, `SELECT id, wrapped_key, active, created_at
		FROM secret_data_keys WHERE active = 1 LIMIT 1`))
	if err == nil {
		if err := tx.Commit(); err != nil {
			return secretprotection.WrappedDataKey{}, fmt.Errorf("commit secret data-key read: %w", err)
		}
		return result, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	candidate.Active = true
	if err := insertSecretDataKey(ctx, tx, s.ph, candidate); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit initial secret data key: %w", err)
	}
	return candidate, nil
}

func (s *sqlStore) RotateSecretDataKey(ctx context.Context, next secretprotection.WrappedDataKey) (retErr error) {
	if err := validateWrappedDataKey(next); err != nil {
		return err
	}
	s.secretKeyLock.Lock()
	defer s.secretKeyLock.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin secret data-key rotation: %w", err)
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, tx.Rollback())
		}
	}()
	if err := s.lockSecretDataKeys(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE secret_data_keys SET active = 0 WHERE active = 1`); err != nil {
		return fmt.Errorf("retire active secret data key: %w", err)
	}
	next.Active = true
	if err := insertSecretDataKey(ctx, tx, s.ph, next); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit secret data-key rotation: %w", err)
	}
	return nil
}

func (s *sqlStore) ListSecretDataKeys(ctx context.Context) ([]secretprotection.WrappedDataKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, wrapped_key, active, created_at
		FROM secret_data_keys ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list secret data keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	keys := make([]secretprotection.WrappedDataKey, 0)
	for rows.Next() {
		key, err := scanSecretDataKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list secret data keys: %w", err)
	}
	return keys, nil
}

func (s *sqlStore) ReplaceWrappedDataKeys(
	ctx context.Context, expectedFingerprint, nextFingerprint string, keys []secretprotection.WrappedDataKey,
) (retErr error) {
	if expectedFingerprint == "" || nextFingerprint == "" || len(keys) == 0 {
		return errors.New("secret protection: replacement requires fingerprints and wrapped keys")
	}
	for _, key := range keys {
		if err := validateWrappedDataKey(key); err != nil {
			return err
		}
	}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, exists := seen[key.ID]; exists {
			return fmt.Errorf("secret protection: duplicate replacement data key %q", key.ID)
		}
		seen[key.ID] = struct{}{}
	}
	s.secretKeyLock.Lock()
	defer s.secretKeyLock.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin installation-key replacement: %w", err)
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, tx.Rollback())
		}
	}()
	if err := s.lockSecretDataKeys(ctx, tx); err != nil {
		return err
	}
	var storedCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM secret_data_keys`).Scan(&storedCount); err != nil {
		return fmt.Errorf("count data keys for replacement: %w", err)
	}
	if storedCount != len(keys) {
		return fmt.Errorf("secret protection: replacement keyring is incomplete")
	}
	result, err := tx.ExecContext(ctx, s.ph(`UPDATE secret_protection_metadata
		SET installation_key_fingerprint = ? WHERE singleton = 1 AND installation_key_fingerprint = ?`),
		nextFingerprint, expectedFingerprint)
	if err != nil {
		return fmt.Errorf("replace installation-key fingerprint: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return secretprotection.ErrInstallationKeyMismatch
	}
	for _, key := range keys {
		result, err := tx.ExecContext(ctx, s.ph(`UPDATE secret_data_keys SET wrapped_key = ? WHERE id = ?`), key.Wrapped, key.ID)
		if err != nil {
			return fmt.Errorf("rewrap data key %q: %w", key.ID, err)
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return fmt.Errorf("rewrap data key %q: row disappeared", key.ID)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit installation-key replacement: %w", err)
	}
	return nil
}

func (s *sqlStore) lockSecretDataKeys(ctx context.Context, tx *sql.Tx) error {
	if s.dialect != DialectPostgres {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(current_database() || ':' || $1, 0))`, secretDataKeyLockName); err != nil {
		return fmt.Errorf("lock secret data keys: %w", err)
	}
	return nil
}

func insertSecretDataKey(ctx context.Context, tx *sql.Tx, ph placeholder, key secretprotection.WrappedDataKey) error {
	active := 0
	if key.Active {
		active = 1
	}
	_, err := tx.ExecContext(ctx, ph(`INSERT INTO secret_data_keys
		(id, wrapped_key, active, created_at) VALUES (?, ?, ?, ?)`), key.ID, key.Wrapped, active, key.CreatedAt.Unix())
	if err != nil {
		return fmt.Errorf("insert secret data key: %w", err)
	}
	return nil
}

func scanSecretDataKey(scanner scannable) (secretprotection.WrappedDataKey, error) {
	var key secretprotection.WrappedDataKey
	var active int
	var createdAt int64
	if err := scanner.Scan(&key.ID, &key.Wrapped, &active, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return key, sql.ErrNoRows
		}
		return key, fmt.Errorf("scan secret data key: %w", err)
	}
	key.Active = active == 1
	key.CreatedAt = time.Unix(createdAt, 0).UTC()
	return key, nil
}

func validateWrappedDataKey(key secretprotection.WrappedDataKey) error {
	if key.ID == "" || key.Wrapped == "" || key.CreatedAt.IsZero() {
		return errors.New("secret data key requires id, wrapped material, and creation time")
	}
	return nil
}
