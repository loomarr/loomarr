// Package contact owns person contact-address identity and normalization (§11).
// Contact data is deliberately separate from usernames and credential paths.
package contact

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
)

// Status is where an address sits in the possession-verification lifecycle.
type Status string

const (
	StatusPending  Status = "pending"
	StatusVerified Status = "verified"
)

// Provenance records how Loomarr first learned an address. It never implies verification.
type Provenance string

const (
	ProvenanceAdmin      Provenance = "admin"
	ProvenanceInvitation Provenance = "invitation"
	ProvenanceSelf       Provenance = "self"
)

// OwnerKind distinguishes an allowlisted person from a pending Invitation. Contact is never
// identity for either owner.
type OwnerKind string

const (
	OwnerUser       OwnerKind = "user"
	OwnerInvitation OwnerKind = "invitation"
)

// Address is one durable contact-address candidate. A person may have one verified address and
// one pending replacement; only the verified address is recovery-capable.
type Address struct {
	OwnerKind  OwnerKind
	OwnerID    string
	Email      string
	Normalized string
	Status     Status
	Provenance Provenance
	CreatedAt  time.Time
	VerifiedAt time.Time
}

// Set is the complete contact state for one person.
type Set struct {
	Verified *Address
	Pending  *Address
}

// Normalize parses exactly one mailbox and returns its address-only display value plus the
// case-folded uniqueness key. Provider-specific plus/dot rewriting is intentionally absent.
func Normalize(raw string) (email, normalized string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("contact address is empty")
	}
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil {
		return "", "", fmt.Errorf("parse contact address: %w", err)
	}
	if strings.ContainsAny(parsed.Address, "\r\n") {
		return "", "", fmt.Errorf("contact address contains a line break")
	}
	at := strings.LastIndexByte(parsed.Address, '@')
	if at <= 0 || at == len(parsed.Address)-1 {
		return "", "", fmt.Errorf("contact address requires a local and domain part")
	}
	return parsed.Address, strings.ToLower(parsed.Address), nil
}
