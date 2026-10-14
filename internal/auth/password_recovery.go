package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/contact"
	"github.com/loomarr/loomarr/internal/recovery"
	"github.com/loomarr/loomarr/internal/store"
)

var ErrInvalidPasswordRecovery = errors.New("auth: password recovery is invalid or expired")

type PasswordRecoveryStore interface {
	GetUserByName(context.Context, string) (store.User, error)
	GetContactAddresses(context.Context, string) (contact.Set, error)
	CreatePasswordRecovery(context.Context, recovery.Record) error
	GetPasswordRecovery(context.Context, string, time.Time) (recovery.Record, error)
	GetPasswordRecoveryByGrant(context.Context, string, time.Time) (recovery.Record, error)
	AddPasswordRecoveryGrant(context.Context, string, recovery.Grant, time.Time) error
	RevokePasswordRecoveryGrant(context.Context, string, time.Time) error
	RedeemPasswordRecovery(context.Context, string, string, time.Time) (recovery.Record, error)
}

// PasswordRecoveryRequest is returned only inside the composition root. Public callers always
// receive the same accepted response and never learn whether this value existed.
type PasswordRecoveryRequest struct {
	Recovery      recovery.Record
	RecipientName string
}

type IssuedRecoveryGrant struct {
	Plaintext string
	ExpiresAt time.Time
}

type PasswordRecoveryService struct {
	store    PasswordRecoveryStore
	newID    IDGen
	newToken func() (string, error)
	now      func() time.Time
}

func NewPasswordRecoveryService(
	st PasswordRecoveryStore,
	newID IDGen,
	newToken func() (string, error),
	now func() time.Time,
) *PasswordRecoveryService {
	if newToken == nil {
		newToken = recoveryGrantToken
	}
	if now == nil {
		now = time.Now
	}
	return &PasswordRecoveryService{store: st, newID: newID, newToken: newToken, now: now}
}

// Request returns nil without error for every ineligible identity. The API deliberately discards
// even the eligible result so unknown, disabled, imported, contactless, and local people are
// indistinguishable to the requester.
func (s *PasswordRecoveryService) Request(
	ctx context.Context,
	username string,
) (*PasswordRecoveryRequest, error) {
	user, err := s.store.GetUserByName(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if user.Disabled || user.MediaServerLinked || user.PasswordHash == "" {
		return nil, nil
	}
	addresses, err := s.store.GetContactAddresses(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if addresses.Verified == nil {
		return nil, nil
	}
	now := s.now().UTC().Truncate(time.Second)
	value := recovery.Record{
		ID: s.newID(), UserID: user.ID, Status: recovery.StatusPending,
		CreatedAt: now, ExpiresAt: now.Add(recovery.Expiry),
	}
	if err := s.store.CreatePasswordRecovery(ctx, value); err != nil {
		// Eligibility can change between the reads and atomic create. Preserve the public no-op
		// contract if the person was disabled or changed credential paths during that window.
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &PasswordRecoveryRequest{Recovery: value, RecipientName: user.Name}, nil
}

func (s *PasswordRecoveryService) Contact(ctx context.Context, userID string) (contact.Address, error) {
	addresses, err := s.store.GetContactAddresses(ctx, userID)
	if err != nil {
		return contact.Address{}, err
	}
	if addresses.Verified == nil {
		return contact.Address{}, store.ErrNotFound
	}
	return *addresses.Verified, nil
}

func (s *PasswordRecoveryService) IssueGrant(
	ctx context.Context,
	recoveryID string,
) (IssuedRecoveryGrant, error) {
	now := s.now().UTC().Truncate(time.Second)
	value, err := s.store.GetPasswordRecovery(ctx, recoveryID, now)
	if err != nil || value.Status != recovery.StatusPending {
		return IssuedRecoveryGrant{}, ErrInvalidPasswordRecovery
	}
	plaintext, err := s.newToken()
	if err != nil {
		return IssuedRecoveryGrant{}, fmt.Errorf("generate password recovery grant: %w", err)
	}
	if _, err := recoveryGrantHash(plaintext); err != nil {
		return IssuedRecoveryGrant{}, err
	}
	grant := recovery.Grant{
		TokenHash: recovery.HashGrant(plaintext), RecoveryID: value.ID,
		CreatedAt: now, ExpiresAt: value.ExpiresAt,
	}
	if err := s.store.AddPasswordRecoveryGrant(ctx, value.ID, grant, now); err != nil {
		return IssuedRecoveryGrant{}, err
	}
	return IssuedRecoveryGrant{Plaintext: plaintext, ExpiresAt: grant.ExpiresAt}, nil
}

func (s *PasswordRecoveryService) RevokeIssuedGrant(ctx context.Context, plaintext string) error {
	hash, err := recoveryGrantHash(plaintext)
	if err != nil {
		return err
	}
	return s.store.RevokePasswordRecoveryGrant(
		ctx, hash, s.now().UTC().Truncate(time.Second),
	)
}

func (s *PasswordRecoveryService) Preview(ctx context.Context, plaintext string) (recovery.Record, error) {
	hash, err := recoveryGrantHash(plaintext)
	if err != nil {
		return recovery.Record{}, err
	}
	value, err := s.store.GetPasswordRecoveryByGrant(ctx, hash, s.now())
	if errors.Is(err, store.ErrNotFound) {
		return recovery.Record{}, ErrInvalidPasswordRecovery
	}
	return value, err
}

func (s *PasswordRecoveryService) Redeem(
	ctx context.Context,
	plaintext string,
	password string,
) error {
	hash, err := recoveryGrantHash(plaintext)
	if err != nil {
		return err
	}
	if len(password) < MinPasswordLen {
		return ErrWeakPassword
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash recovered password: %w", err)
	}
	if _, err := s.store.RedeemPasswordRecovery(ctx, hash, passwordHash, s.now()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrInvalidPasswordRecovery
		}
		return err
	}
	return nil
}

func recoveryGrantHash(plaintext string) (string, error) {
	decoded, err := hex.DecodeString(plaintext)
	if err != nil || len(decoded) != 32 {
		return "", ErrInvalidPasswordRecovery
	}
	return recovery.HashGrant(plaintext), nil
}

func recoveryGrantToken() (string, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}
