package fillerrelease

import (
	"fmt"
	"strings"
)

const (
	HouseholdCohortSchemaVersion = 1
	HouseholdCohortSeedSize      = 50
	HouseholdCohortSize          = 32
)

// CohortCandidate is one frozen seed case observed through the shipping pipeline.
// Descriptive metadata is deliberately absent: eligibility comes from Ready,
// byte identities, duplicate-family control, and real Range playback.
type CohortCandidate struct {
	CaseID                   string `json:"case_id"`
	ClipHash                 string `json:"clip_hash"`
	SourceIdentity           string `json:"source_identity"`
	SourceMasterSHA256       string `json:"source_master_sha256"`
	LineageSHA256            string `json:"lineage_sha256"`
	PlaybackDerivativeSHA256 string `json:"playback_derivative_sha256"`
	SidecarSHA256            string `json:"sidecar_sha256"`
	DuplicateFamilyIdentity  string `json:"duplicate_family_identity,omitempty"`
	Disposition              string `json:"disposition"`
	RangePlayback            bool   `json:"range_playback"`
}

type CohortExclusion struct {
	CaseID string `json:"case_id"`
	Reason string `json:"reason"`
}

// CohortSelection is deterministic: selected and reserves retain frozen seed
// order, while every ineligible row remains visible with one stable reason.
type CohortSelection struct {
	SchemaVersion   int               `json:"schema_version"`
	SelectionPolicy string            `json:"selection_policy"`
	Selected        []CohortCandidate `json:"selected"`
	Reserves        []CohortCandidate `json:"reserves"`
	Excluded        []CohortExclusion `json:"excluded"`
}

// SelectHouseholdCohort chooses the first 32 eligible cases in the frozen
// 50-case seed order. It never backfills a second member of the same known
// duplicate family and never turns a held/rejected or unplayable case into a
// release candidate merely to reach the target.
func SelectHouseholdCohort(seedOrder []string, candidates []CohortCandidate) (CohortSelection, error) {
	selection := CohortSelection{
		SchemaVersion:   HouseholdCohortSchemaVersion,
		SelectionPolicy: "frozen-seed-order:first-32-ready-range-playable:one-per-duplicate-family",
		Selected:        []CohortCandidate{},
		Reserves:        []CohortCandidate{},
		Excluded:        []CohortExclusion{},
	}
	if len(seedOrder) != HouseholdCohortSeedSize {
		return selection, fmt.Errorf("frozen seed has %d cases, want %d", len(seedOrder), HouseholdCohortSeedSize)
	}
	byID := make(map[string]CohortCandidate, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.CaseID) == "" {
			return selection, fmt.Errorf("candidate case id is empty")
		}
		if _, exists := byID[candidate.CaseID]; exists {
			return selection, fmt.Errorf("candidate %q repeats", candidate.CaseID)
		}
		byID[candidate.CaseID] = candidate
	}
	if len(byID) != len(seedOrder) {
		return selection, fmt.Errorf("candidate inventory has %d cases, want %d", len(byID), len(seedOrder))
	}
	seenSeed := make(map[string]struct{}, len(seedOrder))
	selectedFamilies := map[string]struct{}{}
	selectedDigests := map[string]struct{}{}
	for _, caseID := range seedOrder {
		if _, exists := seenSeed[caseID]; exists {
			return selection, fmt.Errorf("frozen seed repeats %q", caseID)
		}
		seenSeed[caseID] = struct{}{}
		candidate, exists := byID[caseID]
		if !exists {
			return selection, fmt.Errorf("frozen seed case %q is missing from the pipeline inventory", caseID)
		}
		reason := cohortIneligibleReason(candidate, selectedFamilies, selectedDigests)
		if reason != "" {
			selection.Excluded = append(selection.Excluded, CohortExclusion{CaseID: caseID, Reason: reason})
			continue
		}
		if candidate.DuplicateFamilyIdentity != "" {
			selectedFamilies[candidate.DuplicateFamilyIdentity] = struct{}{}
		}
		selectedDigests[candidate.SourceMasterSHA256] = struct{}{}
		selectedDigests[candidate.LineageSHA256] = struct{}{}
		selectedDigests[candidate.PlaybackDerivativeSHA256] = struct{}{}
		selectedDigests[candidate.SidecarSHA256] = struct{}{}
		if len(selection.Selected) < HouseholdCohortSize {
			selection.Selected = append(selection.Selected, candidate)
		} else {
			selection.Reserves = append(selection.Reserves, candidate)
		}
	}
	if len(selection.Selected) != HouseholdCohortSize {
		return selection, fmt.Errorf("eligible cohort has %d clips, want %d", len(selection.Selected), HouseholdCohortSize)
	}
	return selection, nil
}

func cohortIneligibleReason(candidate CohortCandidate, selectedFamilies, selectedDigests map[string]struct{}) string {
	if candidate.Disposition != "ready" {
		return "not_ready"
	}
	if !candidate.RangePlayback {
		return "range_playback_failed"
	}
	if strings.TrimSpace(candidate.SourceIdentity) == "" || !validDigest(candidate.ClipHash) ||
		!validDigest(candidate.SourceMasterSHA256) || !validDigest(candidate.LineageSHA256) ||
		!validDigest(candidate.PlaybackDerivativeSHA256) || !validDigest(candidate.SidecarSHA256) {
		return "identity_invalid"
	}
	if candidate.DuplicateFamilyIdentity != "" {
		if _, exists := selectedFamilies[candidate.DuplicateFamilyIdentity]; exists {
			return "duplicate_family"
		}
	}
	digests := []string{candidate.SourceMasterSHA256, candidate.LineageSHA256, candidate.PlaybackDerivativeSHA256, candidate.SidecarSHA256}
	localDigests := make(map[string]struct{}, len(digests))
	for _, digest := range digests {
		if _, exists := localDigests[digest]; exists {
			return "identity_collision"
		}
		localDigests[digest] = struct{}{}
		if _, exists := selectedDigests[digest]; exists {
			return "identity_collision"
		}
	}
	return ""
}
