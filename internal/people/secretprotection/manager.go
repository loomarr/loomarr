package secretprotection

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// ErrInstallationKeyMismatch means this database was initialized with a
// different external installation key. Starting with the supplied key is unsafe.
var ErrInstallationKeyMismatch = errors.New("secret protection: installation key does not match database")

// DataKeyRepository is the durable wrapped-key seam. Implementations never see
// raw installation-key or data-key material.
type DataKeyRepository interface {
	EnsureInstallationKeyFingerprint(context.Context, string) error
	EnsureSecretDataKey(context.Context, WrappedDataKey) (WrappedDataKey, error)
	RotateSecretDataKey(context.Context, WrappedDataKey) error
	ListSecretDataKeys(context.Context) ([]WrappedDataKey, error)
	ReplaceWrappedDataKeys(context.Context, string, string, []WrappedDataKey) error
}

// ManagerOptions supplies the installation key and deterministic test seams.
type ManagerOptions struct {
	InstallationKey InstallationKey
	Random          io.Reader
	Now             func() time.Time
}

// Manager owns the complete in-memory keyring and field-protection interface.
type Manager struct {
	mu          sync.RWMutex
	protector   *Protector
	repository  DataKeyRepository
	wrapper     *KeyWrapper
	random      io.Reader
	now         func() time.Time
	fingerprint string
	active      DataKey
	retired     []DataKey
	stored      []WrappedDataKey
}

// NewManager establishes the first wrapped DEK when needed, then loads and
// authenticates the complete readable keyring.
func NewManager(ctx context.Context, repository DataKeyRepository, options ManagerOptions) (*Manager, error) {
	if repository == nil {
		return nil, errors.New("secret protection: data-key repository unavailable")
	}
	random := options.Random
	if random == nil {
		random = defaultRandom
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	wrapper, err := NewKeyWrapper(options.InstallationKey, random)
	if err != nil {
		return nil, err
	}
	if err := repository.EnsureInstallationKeyFingerprint(ctx, options.InstallationKey.Fingerprint()); err != nil {
		return nil, fmt.Errorf("secret protection: verify installation key: %w", err)
	}
	stored, err := repository.ListSecretDataKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("secret protection: load data keys: %w", err)
	}
	if len(stored) == 0 {
		candidate, err := generateDataKey(random)
		if err != nil {
			return nil, err
		}
		wrapped, err := wrapper.Wrap(candidate)
		if err != nil {
			return nil, err
		}
		if _, err := repository.EnsureSecretDataKey(ctx, WrappedDataKey{
			ID: candidate.ID, Wrapped: wrapped, Active: true, CreatedAt: now().UTC().Truncate(time.Second),
		}); err != nil {
			return nil, fmt.Errorf("secret protection: ensure data key: %w", err)
		}
		stored, err = repository.ListSecretDataKeys(ctx)
		if err != nil {
			return nil, fmt.Errorf("secret protection: load initialized data keys: %w", err)
		}
	}
	var active DataKey
	retired := make([]DataKey, 0, len(stored))
	for _, row := range stored {
		key, err := wrapper.Unwrap(row.ID, row.Wrapped)
		if err != nil {
			return nil, fmt.Errorf("secret protection: unwrap data key %q: %w", row.ID, err)
		}
		if row.Active {
			if active.ID != "" {
				return nil, errors.New("secret protection: multiple active data keys")
			}
			active = key
		} else {
			retired = append(retired, key)
		}
	}
	if active.ID == "" {
		return nil, errors.New("secret protection: no active data key")
	}
	protector, err := New(active, retired, random)
	if err != nil {
		return nil, err
	}
	return &Manager{
		protector: protector, repository: repository, wrapper: wrapper, random: random, now: now,
		fingerprint: options.InstallationKey.Fingerprint(), active: active, retired: retired,
		stored: append([]WrappedDataKey(nil), stored...),
	}, nil
}

// Seal protects one contextual database secret.
func (m *Manager) Seal(record Record, plaintext []byte) (string, error) {
	if m == nil {
		return "", errors.New("secret protection: manager unavailable")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.protector.Seal(record, plaintext)
}

// Open authenticates and recovers one contextual database secret.
func (m *Manager) Open(record Record, envelope string) ([]byte, error) {
	if m == nil {
		return nil, errors.New("secret protection: manager unavailable")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.protector.Open(record, envelope)
}

// OpenLatest retries once after refreshing when another replica wrote with a
// newly rotated DEK. Ordinary reads using an already loaded key need no DB query.
func (m *Manager) OpenLatest(ctx context.Context, record Record, envelope string) ([]byte, error) {
	plain, err := m.Open(record, envelope)
	if !errors.Is(err, ErrDataKeyUnavailable) {
		return plain, err
	}
	if err := m.Refresh(ctx); err != nil {
		return nil, err
	}
	return m.Open(record, envelope)
}

// RotateDataKey makes a fresh DEK active for new writes while retaining every
// prior key for reads. The durable rotation commits before memory advances.
func (m *Manager) RotateDataKey(ctx context.Context) error {
	if m == nil {
		return errors.New("secret protection: manager unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	next, err := generateDataKey(m.random)
	if err != nil {
		return err
	}
	wrapped, err := m.wrapper.Wrap(next)
	if err != nil {
		return err
	}
	row := WrappedDataKey{ID: next.ID, Wrapped: wrapped, Active: true, CreatedAt: m.now().UTC().Truncate(time.Second)}
	if err := m.repository.RotateSecretDataKey(ctx, row); err != nil {
		return fmt.Errorf("secret protection: rotate data key: %w", err)
	}
	retired := append(append([]DataKey(nil), m.retired...), m.active)
	protector, err := New(next, retired, m.random)
	if err != nil {
		return err
	}
	for i := range m.stored {
		m.stored[i].Active = false
	}
	m.stored = append(m.stored, row)
	m.active, m.retired, m.protector = next, retired, protector
	return nil
}

// ReplaceInstallationKey rewraps every DEK under a deliberately supplied new
// installation key. Secret ciphertext itself is unchanged.
func (m *Manager) ReplaceInstallationKey(ctx context.Context, next InstallationKey) error {
	if m == nil {
		return errors.New("secret protection: manager unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	nextWrapper, err := NewKeyWrapper(next, m.random)
	if err != nil {
		return err
	}
	keys := make(map[string]DataKey, len(m.retired)+1)
	keys[m.active.ID] = m.active
	for _, key := range m.retired {
		keys[key.ID] = key
	}
	rewrapped := make([]WrappedDataKey, len(m.stored))
	for i, row := range m.stored {
		wrapped, err := nextWrapper.Wrap(keys[row.ID])
		if err != nil {
			return err
		}
		rewrapped[i] = row
		rewrapped[i].Wrapped = wrapped
	}
	nextFingerprint := next.Fingerprint()
	if err := m.repository.ReplaceWrappedDataKeys(ctx, m.fingerprint, nextFingerprint, rewrapped); err != nil {
		return fmt.Errorf("secret protection: replace installation key: %w", err)
	}
	m.wrapper, m.fingerprint, m.stored = nextWrapper, nextFingerprint, rewrapped
	return nil
}

// Fingerprint is the current non-secret installation-key identity.
func (m *Manager) Fingerprint() string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fingerprint
}

// DataKeyCount reports the active and retained readable DEKs.
func (m *Manager) DataKeyCount() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.stored)
}

// Refresh reloads the wrapped keyring. Persistence adapters call it at operation
// boundaries so PostgreSQL replicas observe rotations performed elsewhere.
func (m *Manager) Refresh(ctx context.Context) error {
	if m == nil {
		return errors.New("secret protection: manager unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.repository.EnsureInstallationKeyFingerprint(ctx, m.fingerprint); err != nil {
		return fmt.Errorf("secret protection: verify installation key: %w", err)
	}
	stored, err := m.repository.ListSecretDataKeys(ctx)
	if err != nil {
		return fmt.Errorf("secret protection: refresh data keys: %w", err)
	}
	if sameWrappedKeys(m.stored, stored) {
		return nil
	}
	var active DataKey
	retired := make([]DataKey, 0, len(stored))
	for _, row := range stored {
		key, err := m.wrapper.Unwrap(row.ID, row.Wrapped)
		if err != nil {
			return fmt.Errorf("secret protection: unwrap refreshed data key %q: %w", row.ID, err)
		}
		if row.Active {
			if active.ID != "" {
				return errors.New("secret protection: multiple active data keys")
			}
			active = key
		} else {
			retired = append(retired, key)
		}
	}
	if active.ID == "" {
		return errors.New("secret protection: no active data key")
	}
	protector, err := New(active, retired, m.random)
	if err != nil {
		return err
	}
	m.active, m.retired, m.protector = active, retired, protector
	m.stored = append([]WrappedDataKey(nil), stored...)
	return nil
}

func sameWrappedKeys(a, b []WrappedDataKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func generateDataKey(random io.Reader) (DataKey, error) {
	var identity [16]byte
	if _, err := io.ReadFull(random, identity[:]); err != nil {
		return DataKey{}, fmt.Errorf("secret protection: generate data-key identity: %w", err)
	}
	key := DataKey{ID: "dek-" + hex.EncodeToString(identity[:])}
	if _, err := io.ReadFull(random, key.Material[:]); err != nil {
		return DataKey{}, fmt.Errorf("secret protection: generate data key: %w", err)
	}
	return key, nil
}
