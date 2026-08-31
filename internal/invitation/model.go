// Package invitation owns administrator admission decisions and their bearer grants (§11).
package invitation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	Expiry    = 7 * 24 * time.Hour
	Retention = 30 * 24 * time.Hour
)

type Kind string

const (
	KindLocal   Kind = "local"
	KindLibrary Kind = "library"
)

type Role string

const (
	RoleMember Role = "member"
	RoleAdmin  Role = "admin"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusRedeemed Status = "redeemed"
	StatusExpired  Status = "expired"
	StatusRevoked  Status = "revoked"
)

type GrantKind string

const GrantActivation GrantKind = "activation"

type Conveyance string

const (
	ConveyanceEmail Conveyance = "email"
	ConveyanceCopy  Conveyance = "copy"
	ConveyanceQR    Conveyance = "qr"
)

// Invitation is an administrator's durable admission decision, not a bearer link.
type Invitation struct {
	ID            string
	Kind          Kind
	Username      string
	LibraryUserID string
	DisplayName   string
	IdentityKey   string
	Role          Role
	Status        Status
	CreatedAt     time.Time
	ExpiresAt     time.Time
	TerminalAt    time.Time
	RedeemedBy    string
}

// Grant is the durable half of an Invitation grant. TokenHash is the only token material stored.
type Grant struct {
	TokenHash    string
	InvitationID string
	Kind         GrantKind
	Conveyance   Conveyance
	CreatedAt    time.Time
	ExpiresAt    time.Time
	ConsumedAt   time.Time
	RevokedAt    time.Time
}

func (i Invitation) Validate() error {
	if err := identifier("invitation id", i.ID); err != nil {
		return err
	}
	if i.Role != RoleMember && i.Role != RoleAdmin {
		return fmt.Errorf("invalid invitation role %q", i.Role)
	}
	if i.Status != StatusPending && i.Status != StatusRedeemed && i.Status != StatusRevoked {
		return fmt.Errorf("invalid durable invitation status %q", i.Status)
	}
	if i.CreatedAt.IsZero() || !i.ExpiresAt.Equal(i.CreatedAt.Add(Expiry)) {
		return fmt.Errorf("invitation requires the fixed seven-day expiry")
	}
	switch i.Kind {
	case KindLocal:
		username := strings.TrimSpace(i.Username)
		if username == "" || len(username) > 200 || i.LibraryUserID != "" || i.DisplayName != "" {
			return fmt.Errorf("local invitation requires only a reserved username")
		}
		if i.IdentityKey != NormalizeLocalIdentity(username) {
			return fmt.Errorf("local invitation identity key is not normalized")
		}
	case KindLibrary:
		if i.Username != "" {
			return fmt.Errorf("Library invitation cannot reserve a local username")
		}
		if err := identifier("Library user id", i.LibraryUserID); err != nil {
			return err
		}
		if i.IdentityKey != i.LibraryUserID {
			return fmt.Errorf("Library invitation identity key must be the exact Library id")
		}
		if err := safeText("display name", i.DisplayName, 200); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid invitation kind %q", i.Kind)
	}
	return nil
}

func (i Invitation) EffectiveStatus(now time.Time) Status {
	if i.Status == StatusPending && !now.Before(i.ExpiresAt) {
		return StatusExpired
	}
	return i.Status
}

func (g Grant) Validate() error {
	decoded, err := hex.DecodeString(g.TokenHash)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("invitation grant requires a SHA-256 token hash")
	}
	if err := identifier("invitation id", g.InvitationID); err != nil {
		return err
	}
	if g.Kind != GrantActivation {
		return fmt.Errorf("invalid invitation grant kind %q", g.Kind)
	}
	if g.Conveyance != ConveyanceEmail && g.Conveyance != ConveyanceCopy && g.Conveyance != ConveyanceQR {
		return fmt.Errorf("invalid invitation grant conveyance %q", g.Conveyance)
	}
	if g.CreatedAt.IsZero() || !g.ExpiresAt.After(g.CreatedAt) || g.ExpiresAt.After(g.CreatedAt.Add(Expiry)) {
		return fmt.Errorf("invitation grant expiry must be positive and no longer than seven days")
	}
	if !g.ConsumedAt.IsZero() && !g.RevokedAt.IsZero() {
		return fmt.Errorf("invitation grant cannot be consumed and revoked")
	}
	return nil
}

func HashGrant(plaintext string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext)))
}

func NormalizeLocalIdentity(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func identifier(name, value string) error {
	if value == "" || len(value) > 200 {
		return fmt.Errorf("%s must contain 1..200 characters", name)
	}
	return safeText(name, value, 200)
}

func safeText(name, value string, limit int) error {
	if len(value) > limit || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s contains unsafe or excessive text", name)
	}
	return nil
}
