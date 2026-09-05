package fillerreview

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const temporalStructureHoldoutFamilyAuditSchemaVersion = 3

type temporalStructureHoldoutFamilyAudit struct {
	SchemaVersion int                                       `json:"schemaVersion"`
	Algorithm     string                                    `json:"algorithm"`
	GeneratedAt   time.Time                                 `json:"generatedAt"`
	SourceAudit   string                                    `json:"sourceAuditSha256"`
	Summary       temporalStructureHoldoutFamilySummary     `json:"summary"`
	Fingerprints  []temporalStructureHoldoutFingerprint     `json:"fingerprints"`
	Pairs         []json.RawMessage                         `json:"relatedPairs"`
	ClosestPairs  []json.RawMessage                         `json:"closestNonMatches"`
	Families      []temporalStructureHoldoutDuplicateFamily `json:"families"`
}

type temporalStructureHoldoutFamilySummary struct {
	Cases             int `json:"cases"`
	RelatedPairs      int `json:"relatedPairs"`
	ClosestNonMatches int `json:"closestNonMatches"`
	DuplicateFamilies int `json:"duplicateFamilies"`
	NonCliqueFamilies int `json:"nonCliqueFamilies"`
}

type temporalStructureHoldoutFingerprint struct {
	CaseID        string   `json:"caseId"`
	ContentSHA256 string   `json:"contentSha256"`
	LocalFile     string   `json:"localFile"`
	FrameHashes   []uint64 `json:"frameHashes"`
	AudioRMS      []uint32 `json:"audioRms100ms"`
}

type temporalStructureHoldoutDuplicateFamily struct {
	FamilyID       string   `json:"familyId"`
	Members        []string `json:"members"`
	CompleteClique bool     `json:"completeClique"`
	PreferredCase  string   `json:"preferredCase,omitempty"`
}

func loadTemporalStructureHoldoutFamily(path string, selection fillereval.TemporalTruthSelection) (temporalStructureHoldoutFamilyAudit, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return temporalStructureHoldoutFamilyAudit{}, "", err
	}
	audit, err := readStrictJSON[temporalStructureHoldoutFamilyAudit](path)
	if err != nil {
		return temporalStructureHoldoutFamilyAudit{}, "", err
	}
	if audit.SchemaVersion != temporalStructureHoldoutFamilyAuditSchemaVersion || strings.TrimSpace(audit.Algorithm) == "" || audit.GeneratedAt.IsZero() || !reviewSHA256(audit.SourceAudit) || audit.Summary.Cases != len(audit.Fingerprints) || audit.Summary.RelatedPairs != len(audit.Pairs) || audit.Summary.ClosestNonMatches != len(audit.ClosestPairs) || audit.Summary.DuplicateFamilies != len(audit.Families) {
		return temporalStructureHoldoutFamilyAudit{}, "", fmt.Errorf("temporal structure holdout family authority is invalid")
	}
	selected := make(map[string]string, len(selection.Cases))
	for _, item := range selection.Cases {
		selected[item.CaseID] = item.ContentSHA256
	}
	seen := make(map[string]struct{}, len(audit.Fingerprints))
	for _, item := range audit.Fingerprints {
		if strings.TrimSpace(item.CaseID) == "" || !reviewSHA256(item.ContentSHA256) || strings.TrimSpace(item.LocalFile) == "" || len(item.FrameHashes) == 0 {
			return temporalStructureHoldoutFamilyAudit{}, "", fmt.Errorf("temporal structure holdout family fingerprint is invalid")
		}
		expected, selectedCase := selected[item.CaseID]
		if !selectedCase {
			return temporalStructureHoldoutFamilyAudit{}, "", fmt.Errorf("temporal structure holdout family authority case set does not match selection")
		}
		if expected != item.ContentSHA256 {
			return temporalStructureHoldoutFamilyAudit{}, "", fmt.Errorf("temporal structure holdout family content drift for %q", item.CaseID)
		}
		if _, duplicate := seen[item.CaseID]; duplicate {
			return temporalStructureHoldoutFamilyAudit{}, "", fmt.Errorf("temporal structure holdout family repeats a case")
		}
		seen[item.CaseID] = struct{}{}
	}
	if len(seen) != len(selected) {
		return temporalStructureHoldoutFamilyAudit{}, "", fmt.Errorf("temporal structure holdout family authority case set does not match selection")
	}
	memberFamily, seenFamilyIDs := map[string]string{}, map[string]struct{}{}
	nonCliqueFamilies := 0
	for _, family := range audit.Families {
		if strings.TrimSpace(family.FamilyID) == "" || len(family.Members) < 2 {
			return temporalStructureHoldoutFamilyAudit{}, "", fmt.Errorf("temporal structure holdout duplicate family is invalid")
		}
		if _, duplicate := seenFamilyIDs[family.FamilyID]; duplicate {
			return temporalStructureHoldoutFamilyAudit{}, "", fmt.Errorf("temporal structure holdout repeats a duplicate family id")
		}
		seenFamilyIDs[family.FamilyID] = struct{}{}
		if !family.CompleteClique {
			nonCliqueFamilies++
		}
		preferredSeen := family.PreferredCase == ""
		for _, member := range family.Members {
			if _, exists := seen[member]; !exists || memberFamily[member] != "" {
				return temporalStructureHoldoutFamilyAudit{}, "", fmt.Errorf("temporal structure holdout duplicate family membership is invalid")
			}
			memberFamily[member] = family.FamilyID
			preferredSeen = preferredSeen || member == family.PreferredCase
		}
		if !preferredSeen {
			return temporalStructureHoldoutFamilyAudit{}, "", fmt.Errorf("temporal structure holdout duplicate family preferred case is not a member")
		}
	}
	if nonCliqueFamilies != audit.Summary.NonCliqueFamilies {
		return temporalStructureHoldoutFamilyAudit{}, "", fmt.Errorf("temporal structure holdout duplicate family summary drift")
	}
	return audit, hashBytes(raw), nil
}
