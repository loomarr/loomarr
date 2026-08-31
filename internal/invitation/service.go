package invitation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/contact"
)

type Repository interface {
	CreateInvitation(context.Context, Invitation, *contact.Address) error
	GetInvitation(context.Context, string, time.Time) (Invitation, error)
	ListInvitations(context.Context, time.Time) ([]Invitation, error)
	GetInvitationContactAddress(context.Context, string) (contact.Address, error)
	ReplaceInvitationGrant(context.Context, string, Grant, time.Time) error
	AddInvitationGrant(context.Context, string, Grant, time.Time) error
	RevokeInvitationGrant(context.Context, string, time.Time) error
	RevokeInvitation(context.Context, string, time.Time) error
}

type LibraryAccount struct {
	ID       string
	Name     string
	Disabled bool
}

type LibraryAccountResolver interface {
	ResolveLibraryAccount(context.Context, string) (LibraryAccount, error)
}

type CreateCommand struct {
	Kind          Kind
	Username      string
	LibraryUserID string
	ContactEmail  string
	Role          Role
}

type IssuedGrant struct {
	Invitation Invitation
	Plaintext  string
	ExpiresAt  time.Time
}

type Service struct {
	repository Repository
	library    LibraryAccountResolver
	newID      func() string
	newToken   func() (string, error)
	now        func() time.Time
}

func NewService(
	repository Repository,
	library LibraryAccountResolver,
	newID func() string,
	newToken func() (string, error),
	now func() time.Time,
) *Service {
	if newToken == nil {
		newToken = NewGrantToken
	}
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, library: library, newID: newID, newToken: newToken, now: now}
}

func (s *Service) Create(ctx context.Context, command CreateCommand) (Invitation, error) {
	now := s.now().UTC().Truncate(time.Second)
	role := command.Role
	if role == "" {
		role = RoleMember
	}
	value := Invitation{
		ID: s.newID(), Kind: command.Kind, Role: role, Status: StatusPending,
		CreatedAt: now, ExpiresAt: now.Add(Expiry),
	}
	switch command.Kind {
	case KindLocal:
		if strings.TrimSpace(command.LibraryUserID) != "" {
			return Invitation{}, fmt.Errorf("local invitation cannot select a Library account")
		}
		value.Username = strings.TrimSpace(command.Username)
		value.IdentityKey = NormalizeLocalIdentity(value.Username)
	case KindLibrary:
		if strings.TrimSpace(command.Username) != "" {
			return Invitation{}, fmt.Errorf("library invitation cannot reserve a local username")
		}
		if s.library == nil {
			return Invitation{}, fmt.Errorf("library invitation creation requires a connected Library")
		}
		requestedID := strings.TrimSpace(command.LibraryUserID)
		account, err := s.library.ResolveLibraryAccount(ctx, requestedID)
		if err != nil {
			return Invitation{}, fmt.Errorf("resolve Library account: %w", err)
		}
		if account.ID != requestedID || account.Disabled {
			return Invitation{}, fmt.Errorf("selected Library account is unavailable")
		}
		value.LibraryUserID = account.ID
		value.IdentityKey = account.ID
		value.DisplayName = strings.TrimSpace(account.Name)
	default:
		return Invitation{}, fmt.Errorf("invalid invitation kind %q", command.Kind)
	}
	if err := value.Validate(); err != nil {
		return Invitation{}, err
	}
	var address *contact.Address
	if strings.TrimSpace(command.ContactEmail) != "" {
		email, normalized, err := contact.Normalize(command.ContactEmail)
		if err != nil {
			return Invitation{}, err
		}
		address = &contact.Address{
			OwnerKind: contact.OwnerInvitation, OwnerID: value.ID, Email: email, Normalized: normalized,
			Status: contact.StatusPending, Provenance: contact.ProvenanceAdmin, CreatedAt: now,
		}
	}
	if err := s.repository.CreateInvitation(ctx, value, address); err != nil {
		return Invitation{}, err
	}
	return value, nil
}

func (s *Service) Regenerate(ctx context.Context, invitationID string, conveyance Conveyance) (IssuedGrant, error) {
	now := s.now().UTC().Truncate(time.Second)
	value, err := s.repository.GetInvitation(ctx, invitationID, now)
	if err != nil {
		return IssuedGrant{}, err
	}
	if value.Status != StatusPending {
		return IssuedGrant{}, fmt.Errorf("invitation is not pending")
	}
	plaintext, err := s.newToken()
	if err != nil {
		return IssuedGrant{}, fmt.Errorf("generate invitation grant: %w", err)
	}
	if len(plaintext) != 64 {
		return IssuedGrant{}, fmt.Errorf("generate invitation grant: expected 256-bit hexadecimal token")
	}
	if _, err := hex.DecodeString(plaintext); err != nil {
		return IssuedGrant{}, fmt.Errorf("generate invitation grant: expected 256-bit hexadecimal token")
	}
	grant := Grant{
		TokenHash: HashGrant(plaintext), InvitationID: value.ID, Kind: GrantActivation,
		Conveyance: conveyance, CreatedAt: now, ExpiresAt: value.ExpiresAt,
	}
	if err := s.repository.ReplaceInvitationGrant(ctx, value.ID, grant, now); err != nil {
		return IssuedGrant{}, err
	}
	return IssuedGrant{Invitation: value, Plaintext: plaintext, ExpiresAt: grant.ExpiresAt}, nil
}

// IssueSibling adds a grant without invalidating an earlier one. Email resends need this shape:
// mail may already have been accepted, and invalidating its link would make a delivered notice lie.
func (s *Service) IssueSibling(
	ctx context.Context,
	invitationID string,
	conveyance Conveyance,
) (IssuedGrant, error) {
	now := s.now().UTC().Truncate(time.Second)
	value, err := s.repository.GetInvitation(ctx, invitationID, now)
	if err != nil {
		return IssuedGrant{}, err
	}
	if value.Status != StatusPending {
		return IssuedGrant{}, fmt.Errorf("invitation is not pending")
	}
	plaintext, err := s.newToken()
	if err != nil {
		return IssuedGrant{}, fmt.Errorf("generate invitation grant: %w", err)
	}
	if len(plaintext) != 64 {
		return IssuedGrant{}, fmt.Errorf("generate invitation grant: expected 256-bit hexadecimal token")
	}
	if _, err := hex.DecodeString(plaintext); err != nil {
		return IssuedGrant{}, fmt.Errorf("generate invitation grant: expected 256-bit hexadecimal token")
	}
	grant := Grant{
		TokenHash: HashGrant(plaintext), InvitationID: value.ID, Kind: GrantActivation,
		Conveyance: conveyance, CreatedAt: now, ExpiresAt: value.ExpiresAt,
	}
	if err := s.repository.AddInvitationGrant(ctx, value.ID, grant, now); err != nil {
		return IssuedGrant{}, err
	}
	return IssuedGrant{Invitation: value, Plaintext: plaintext, ExpiresAt: grant.ExpiresAt}, nil
}

func (s *Service) Revoke(ctx context.Context, invitationID string) error {
	return s.repository.RevokeInvitation(ctx, invitationID, s.now().UTC().Truncate(time.Second))
}

// RevokeIssuedGrant invalidates one grant known to have remained local because delivery failed
// before remote acceptance. Callers retain the plaintext only in memory; persistence receives its
// one-way hash.
func (s *Service) RevokeIssuedGrant(ctx context.Context, plaintext string) error {
	if len(plaintext) != 64 {
		return fmt.Errorf("revoke invitation grant: expected 256-bit hexadecimal token")
	}
	if _, err := hex.DecodeString(plaintext); err != nil {
		return fmt.Errorf("revoke invitation grant: expected 256-bit hexadecimal token")
	}
	return s.repository.RevokeInvitationGrant(
		ctx, HashGrant(plaintext), s.now().UTC().Truncate(time.Second),
	)
}

func (s *Service) Get(ctx context.Context, invitationID string) (Invitation, error) {
	return s.repository.GetInvitation(ctx, invitationID, s.now().UTC().Truncate(time.Second))
}

// Contact returns the optional address attached to a pending Invitation. Keeping
// this read on the module prevents HTTP and notification adapters from reaching
// through to its persistence representation.
func (s *Service) Contact(ctx context.Context, invitationID string) (contact.Address, error) {
	return s.repository.GetInvitationContactAddress(ctx, invitationID)
}

func (s *Service) List(ctx context.Context) ([]Invitation, error) {
	return s.repository.ListInvitations(ctx, s.now().UTC().Truncate(time.Second))
}

func NewGrantToken() (string, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}
