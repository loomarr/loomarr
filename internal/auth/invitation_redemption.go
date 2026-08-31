package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/store"
)

var (
	ErrInvalidInvitation             = errors.New("auth: invitation is invalid or expired")
	ErrInvitationProviderUnavailable = errors.New("auth: invitation provider is unavailable")
)

// InvitationRedemptionStore is the complete durable authority needed by the
// public activation flow. Redemption itself remains one store transaction.
type InvitationRedemptionStore interface {
	GetInvitationByGrant(context.Context, string, time.Time) (invitation.Invitation, error)
	RedeemInvitation(context.Context, string, store.User, store.Session, time.Time) (invitation.Invitation, error)
}

// InvitationRedemptionService proves the invited credential before asking the
// store to atomically admit the identity and create its first session.
type InvitationRedemptionService struct {
	store InvitationRedemptionStore
	lib   Authenticator
	mgr   *Manager
	newID IDGen
	now   func() time.Time
}

func NewInvitationRedemptionService(
	st InvitationRedemptionStore,
	lib Authenticator,
	mgr *Manager,
	newID IDGen,
	now func() time.Time,
) *InvitationRedemptionService {
	if now == nil {
		now = time.Now
	}
	return &InvitationRedemptionService{store: st, lib: lib, mgr: mgr, newID: newID, now: now}
}

// Preview resolves a usable bearer without consuming it or creating allowlist
// state. Possession of the bearer authorizes seeing its reserved identity.
func (s *InvitationRedemptionService) Preview(ctx context.Context, bearer string) (invitation.Invitation, error) {
	grantHash, err := invitationGrantHash(bearer)
	if err != nil {
		return invitation.Invitation{}, err
	}
	value, err := s.store.GetInvitationByGrant(ctx, grantHash, s.now())
	if errors.Is(err, store.ErrNotFound) {
		return invitation.Invitation{}, ErrInvalidInvitation
	}
	return value, err
}

func (s *InvitationRedemptionService) RedeemLocal(
	ctx context.Context,
	bearer string,
	password string,
) (token string, expires time.Time, user store.User, err error) {
	value, err := s.Preview(ctx, bearer)
	if err != nil {
		return "", time.Time{}, store.User{}, err
	}
	if value.Kind != invitation.KindLocal {
		return "", time.Time{}, store.User{}, ErrInvalidInvitation
	}
	if len(password) < MinPasswordLen {
		return "", time.Time{}, store.User{}, ErrWeakPassword
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return "", time.Time{}, store.User{}, fmt.Errorf("hash invitation password: %w", err)
	}
	at := s.now()
	user = store.User{
		ID: s.newID(), Name: value.Username, Role: store.Role(value.Role), PasswordHash: passwordHash,
		CreatedAt: at, UpdatedAt: at,
	}
	token, session, err := s.mgr.prepare(user.ID)
	if err != nil {
		return "", time.Time{}, store.User{}, err
	}
	if _, err := s.store.RedeemInvitation(ctx, invitation.HashGrant(bearer), user, session, at); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return "", time.Time{}, store.User{}, ErrInvalidInvitation
		}
		return "", time.Time{}, store.User{}, err
	}
	return token, session.ExpiresAt, user, nil
}

func (s *InvitationRedemptionService) RedeemLibrary(
	ctx context.Context,
	bearer string,
	username string,
	password string,
) (token string, expires time.Time, user store.User, err error) {
	value, err := s.Preview(ctx, bearer)
	if err != nil {
		return "", time.Time{}, store.User{}, err
	}
	if value.Kind != invitation.KindLibrary {
		return "", time.Time{}, store.User{}, ErrInvalidInvitation
	}
	if s.lib == nil {
		return "", time.Time{}, store.User{}, ErrInvitationProviderUnavailable
	}
	providerUser, err := s.lib.AuthenticateByName(ctx, username, password)
	if err != nil {
		if errors.Is(err, library.ErrProviderUnavailable) {
			return "", time.Time{}, store.User{}, ErrInvitationProviderUnavailable
		}
		return "", time.Time{}, store.User{}, ErrInvalidCredentials
	}
	if providerUser.ID != value.LibraryUserID || providerUser.Disabled {
		return "", time.Time{}, store.User{}, ErrInvalidCredentials
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return "", time.Time{}, store.User{}, fmt.Errorf("hash invitation password: %w", err)
	}
	at := s.now()
	user = store.User{
		ID: providerUser.ID, Name: providerUser.Name, Role: store.Role(value.Role),
		MediaServerLinked: true, PasswordHash: passwordHash, CreatedAt: at, UpdatedAt: at,
	}
	token, session, err := s.mgr.prepare(user.ID)
	if err != nil {
		return "", time.Time{}, store.User{}, err
	}
	if _, err := s.store.RedeemInvitation(ctx, invitation.HashGrant(bearer), user, session, at); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return "", time.Time{}, store.User{}, ErrInvalidInvitation
		}
		return "", time.Time{}, store.User{}, err
	}
	return token, session.ExpiresAt, user, nil
}

func invitationGrantHash(bearer string) (string, error) {
	decoded, err := hex.DecodeString(bearer)
	if err != nil || len(decoded) != 32 {
		return "", ErrInvalidInvitation
	}
	return invitation.HashGrant(bearer), nil
}
