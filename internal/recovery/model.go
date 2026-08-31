// Package recovery owns local-password recovery records and their bearer grants (§11).
package recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	Expiry    = 30 * time.Minute
	Retention = 30 * 24 * time.Hour
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusRedeemed Status = "redeemed"
	StatusExpired  Status = "expired"
	StatusRevoked  Status = "revoked"
)

// Record is a local person's durable password-recovery lifecycle. It contains no credential or
// bearer material.
type Record struct {
	ID         string
	UserID     string
	Status     Status
	CreatedAt  time.Time
	ExpiresAt  time.Time
	TerminalAt time.Time
}

// Grant is the durable half of a recovery bearer. TokenHash is the only token material stored.
type Grant struct {
	TokenHash  string
	RecoveryID string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt time.Time
	RevokedAt  time.Time
}

func (r Record) Validate() error {
	if err := identifier("recovery id", r.ID); err != nil {
		return err
	}
	if err := identifier("user id", r.UserID); err != nil {
		return err
	}
	if r.CreatedAt.IsZero() || !r.ExpiresAt.Equal(r.CreatedAt.Add(Expiry)) {
		return fmt.Errorf("password recovery requires the fixed thirty-minute expiry")
	}
	switch r.Status {
	case StatusPending:
		if !r.TerminalAt.IsZero() {
			return fmt.Errorf("pending password recovery cannot be terminal")
		}
	case StatusExpired:
		if !r.TerminalAt.Equal(r.ExpiresAt) {
			return fmt.Errorf("expired password recovery terminates at expiry")
		}
	case StatusRedeemed, StatusRevoked:
		if r.TerminalAt.IsZero() {
			return fmt.Errorf("terminal password recovery requires a terminal time")
		}
	default:
		return fmt.Errorf("invalid password recovery status %q", r.Status)
	}
	return nil
}

func (g Grant) Validate() error {
	decoded, err := hex.DecodeString(g.TokenHash)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("password recovery grant requires a SHA-256 token hash")
	}
	if err := identifier("recovery id", g.RecoveryID); err != nil {
		return err
	}
	if g.CreatedAt.IsZero() || !g.ExpiresAt.After(g.CreatedAt) || g.ExpiresAt.After(g.CreatedAt.Add(Expiry)) {
		return fmt.Errorf("password recovery grant expiry must be positive and no longer than thirty minutes")
	}
	if !g.ConsumedAt.IsZero() && !g.RevokedAt.IsZero() {
		return fmt.Errorf("password recovery grant cannot be consumed and revoked")
	}
	return nil
}

func (r Record) EffectiveStatus(now time.Time) Status {
	if r.Status == StatusPending && !now.Before(r.ExpiresAt) {
		return StatusExpired
	}
	return r.Status
}

func HashGrant(plaintext string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext)))
}

func identifier(name, value string) error {
	if value == "" || len(value) > 200 || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s must contain 1..200 safe characters", name)
	}
	return nil
}
