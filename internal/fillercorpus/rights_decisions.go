package fillercorpus

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

// DecodeRightsDecisions accepts strict JSONL and rejects blank, unknown,
// repeated, or trailing values. Profile authority is validated separately.
func DecodeRightsDecisions(raw []byte) ([]RightsDecision, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	var decisions []RightsDecision
	seen := map[string]struct{}{}
	for line := 1; scanner.Scan(); line++ {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			return nil, fmt.Errorf("decode rights decisions: blank line %d", line)
		}
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		var decision RightsDecision
		if err := decoder.Decode(&decision); err != nil {
			return nil, fmt.Errorf("decode rights decision line %d: %w", line, err)
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return nil, fmt.Errorf("decode rights decision line %d: trailing JSON value", line)
		}
		if _, duplicate := seen[decision.CaseID]; duplicate {
			return nil, fmt.Errorf("rights decisions repeat case %q", decision.CaseID)
		}
		seen[decision.CaseID] = struct{}{}
		decisions = append(decisions, decision)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("decode rights decisions: %w", err)
	}
	if len(decisions) == 0 {
		return nil, fmt.Errorf("decode rights decisions: no decisions")
	}
	return decisions, nil
}

// ValidateRightsDecision proves one decision is a current profile-specific
// lock for one exact inventory case. Quarantine report reproduction remains
// with fillerquarantine.
func ValidateRightsDecision(candidate InventoryCase, inventorySHA256 string, decision RightsDecision, profile string, at time.Time) error {
	if !IsSHA256(inventorySHA256) || decision.InventorySHA256 != inventorySHA256 || decision.CaseID != candidate.CaseID ||
		!slices.Equal(decision.CaptureIDs, candidate.CaptureIDs) || decision.Authority != candidate.Authority ||
		decision.ItemID != candidate.ItemID || decision.MetadataSHA256 != candidate.MetadataSHA256 ||
		strings.TrimSpace(decision.ReviewerID) == "" || decision.ReviewedAt.IsZero() || decision.ReviewedAt.Location() != time.UTC ||
		decision.ReviewedAt.Before(candidate.MetadataRetrievedAt) || decision.ReviewedAt.After(at) || strings.TrimSpace(decision.Basis) == "" ||
		(decision.Decision != "approved" && decision.Decision != "held") {
		return fmt.Errorf("rights decision for %q does not bind the current inventory and review", candidate.CaseID)
	}
	if decision.Decision == "held" {
		if decision.Redistributable {
			return fmt.Errorf("held rights decision for %q grants redistribution", candidate.CaseID)
		}
		return nil
	}
	switch profile {
	case RightsProfileDevelopment:
		if !decision.Redistributable || decision.HoldoutContract != nil || decision.QuarantineContract != nil {
			return fmt.Errorf("approved development decision for %q has invalid grants", candidate.CaseID)
		}
	case RightsProfileCertification:
		if decision.QuarantineContract != nil || len(HoldoutRightsHoldReasons(decision.HoldoutContract, at)) != 0 || len(decision.HoldoutContract.HoldReasons) != 0 {
			return fmt.Errorf("approved certification decision for %q lacks current holdout authority", candidate.CaseID)
		}
	default:
		return fmt.Errorf("rights decision for %q has unsupported profile %q", candidate.CaseID, profile)
	}
	return nil
}
