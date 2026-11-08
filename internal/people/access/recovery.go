package access

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Local-password recovery records and their bearer grants (§11): the second kind of grant this
// package issues, beside an invitation's.

const (
	RecoveryExpiry    = 30 * time.Minute
	RecoveryRetention = 30 * 24 * time.Hour
)

type RecoveryStatus string

const (
	RecoveryPending  RecoveryStatus = "pending"
	RecoveryRedeemed RecoveryStatus = "redeemed"
	RecoveryExpired  RecoveryStatus = "expired"
	RecoveryRevoked  RecoveryStatus = "revoked"
)

// RecoveryRecord is a local person's durable password-recovery lifecycle. It contains no credential or
// bearer material.
type RecoveryRecord struct {
	ID         string
	UserID     string
	Status     RecoveryStatus
	CreatedAt  time.Time
	ExpiresAt  time.Time
	TerminalAt time.Time
}

// RecoveryGrant is the durable half of a recovery bearer. TokenHash is the only token material stored.
type RecoveryGrant struct {
	TokenHash  string
	RecoveryID string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt time.Time
	RevokedAt  time.Time
}

func (r RecoveryRecord) Validate() error {
	if err := identifier("recovery id", r.ID); err != nil {
		return err
	}
	if err := identifier("user id", r.UserID); err != nil {
		return err
	}
	if r.CreatedAt.IsZero() || !r.ExpiresAt.Equal(r.CreatedAt.Add(RecoveryExpiry)) {
		return fmt.Errorf("password recovery requires the fixed thirty-minute expiry")
	}
	switch r.Status {
	case RecoveryPending:
		if !r.TerminalAt.IsZero() {
			return fmt.Errorf("pending password recovery cannot be terminal")
		}
	case RecoveryExpired:
		if !r.TerminalAt.Equal(r.ExpiresAt) {
			return fmt.Errorf("expired password recovery terminates at expiry")
		}
	case RecoveryRedeemed, RecoveryRevoked:
		if r.TerminalAt.IsZero() {
			return fmt.Errorf("terminal password recovery requires a terminal time")
		}
	default:
		return fmt.Errorf("invalid password recovery status %q", r.Status)
	}
	return nil
}

func (g RecoveryGrant) Validate() error {
	decoded, err := hex.DecodeString(g.TokenHash)
	if err != nil || len(decoded) != sha256.Size {
		return fmt.Errorf("password recovery grant requires a SHA-256 token hash")
	}
	if err := identifier("recovery id", g.RecoveryID); err != nil {
		return err
	}
	if g.CreatedAt.IsZero() || !g.ExpiresAt.After(g.CreatedAt) || g.ExpiresAt.After(g.CreatedAt.Add(RecoveryExpiry)) {
		return fmt.Errorf("password recovery grant expiry must be positive and no longer than thirty minutes")
	}
	if !g.ConsumedAt.IsZero() && !g.RevokedAt.IsZero() {
		return fmt.Errorf("password recovery grant cannot be consumed and revoked")
	}
	return nil
}

func (r RecoveryRecord) EffectiveStatus(now time.Time) RecoveryStatus {
	if r.Status == RecoveryPending && !now.Before(r.ExpiresAt) {
		return RecoveryExpired
	}
	return r.Status
}
