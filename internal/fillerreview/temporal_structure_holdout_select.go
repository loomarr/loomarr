package fillerreview

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/loomarr/loomarr/internal/fillereval"
)

type temporalStructureHoldoutSelectedAnchor struct {
	receipt TemporalStructureHoldoutAnchor
	source  TemporalStructureChallengeSource
}

var temporalStructureHoldoutRoleQuotas = map[fillereval.TemporalRole]int{
	fillereval.TemporalRoleBumper:     2,
	fillereval.TemporalRoleCommercial: 3,
	fillereval.TemporalRolePromo:      2,
	fillereval.TemporalRolePSA:        2,
	fillereval.TemporalRoleTrailer:    3,
}

func selectTemporalStructureHoldoutAnchors(seed string, loaded temporalStructureHoldoutLoaded) ([]temporalStructureHoldoutSelectedAnchor, error) {
	evidenceByAlias := make(map[string]TemporalTruthEvidenceCase, len(loaded.evidence.Cases))
	for _, item := range loaded.evidence.Cases {
		evidenceByAlias[item.Alias] = item
	}
	mapByAlias := make(map[string]TemporalTruthEvidencePrivateEntry, len(loaded.privateMap.Entries))
	for _, item := range loaded.privateMap.Entries {
		mapByAlias[item.Alias] = item
	}
	qualityByAlias := make(map[string]TemporalMediaQualityCase, len(loaded.quality.CaseMeasurements))
	for _, item := range loaded.quality.CaseMeasurements {
		qualityByAlias[item.EvidenceAlias] = item
	}
	suitabilityByAlias := make(map[string]TemporalSuitabilityCaseComparison, len(loaded.suitability.CaseComparisons))
	for _, item := range loaded.suitability.CaseComparisons {
		suitabilityByAlias[item.EvidenceAlias] = item
	}
	selectionByCase := make(map[string]fillereval.TemporalTruthSelectionCase, len(loaded.selection.Cases))
	for _, item := range loaded.selection.Cases {
		selectionByCase[item.CaseID] = item
	}
	fingerprintByCase := make(map[string]temporalStructureHoldoutFingerprint, len(loaded.family.Fingerprints))
	for _, item := range loaded.family.Fingerprints {
		fingerprintByCase[item.CaseID] = item
	}
	familyByCase := make(map[string]string)
	for _, family := range loaded.family.Families {
		for _, member := range family.Members {
			familyByCase[member] = family.FamilyID
		}
	}
	byRole := make(map[fillereval.TemporalRole][]temporalStructureHoldoutSelectedAnchor)
	for _, assessment := range loaded.human.Assessments {
		if assessment.Unit != fillereval.UnitStandalone || assessment.Role == nil || temporalStructureHoldoutRoleQuotas[*assessment.Role] == 0 {
			continue
		}
		evidence := evidenceByAlias[assessment.EvidenceAlias]
		mapping := mapByAlias[assessment.EvidenceAlias]
		quality := qualityByAlias[assessment.EvidenceAlias]
		suitability := suitabilityByAlias[assessment.EvidenceAlias]
		selection, selectionExists := selectionByCase[mapping.CaseID]
		fingerprint, fingerprintExists := fingerprintByCase[mapping.CaseID]
		if evidence.Alias == "" || mapping.Alias == "" || quality.EvidenceAlias == "" || suitability.EvidenceAlias == "" || !selectionExists {
			return nil, fmt.Errorf("temporal structure holdout anchor %q lacks complete authority", assessment.EvidenceAlias)
		}
		if mapping.ContentSHA256 != selection.ContentSHA256 || quality.SourceMediaSHA256 != evidence.Video.SHA256 {
			return nil, fmt.Errorf("temporal structure holdout anchor %q authority bytes drift", assessment.EvidenceAlias)
		}
		if quality.OperationalFailure != "" || quality.PolicyVerdict != mediaQualityContinue || suitability.Disposition == "prohibited_hold" || suitability.Disposition == "operational_hold" {
			continue
		}
		if !fingerprintExists {
			continue
		}
		if fingerprint.ContentSHA256 != selection.ContentSHA256 {
			return nil, fmt.Errorf("temporal structure holdout anchor %q duplicate-family bytes drift", assessment.EvidenceAlias)
		}
		familyID := familyByCase[mapping.CaseID]
		if familyID == "" {
			familyID = "singleton-" + hashBytes([]byte(mapping.CaseID))[:24]
		}
		rank := hashBytes([]byte(seed + "\x00anchor\x00" + assessment.EvidenceAlias))
		sourceID := "bounded-" + hashBytes([]byte(assessment.EvidenceAlias))[:24]
		metadataSHA := temporalTruthJSONSHA(struct {
			Selection   fillereval.TemporalTruthSelectionCase `json:"selection"`
			Mapping     TemporalTruthEvidencePrivateEntry     `json:"mapping"`
			Assessment  TemporalHumanReviewAssessment         `json:"assessment"`
			Quality     TemporalMediaQualityCase              `json:"quality"`
			Suitability TemporalSuitabilityCaseComparison     `json:"suitability"`
			FamilyID    string                                `json:"familyId"`
		}{selection, mapping, assessment, quality, suitability, familyID})
		byRole[*assessment.Role] = append(byRole[*assessment.Role], temporalStructureHoldoutSelectedAnchor{
			receipt: TemporalStructureHoldoutAnchor{
				EvidenceAlias: assessment.EvidenceAlias, CaseID: mapping.CaseID, SourceID: sourceID,
				FamilyID: familyID, Role: *assessment.Role, DurationMS: evidence.Video.DurationMS, RankSHA256: rank,
			},
			source: TemporalStructureChallengeSource{
				ID: sourceID, Path: evidence.Video.Path, SHA256: evidence.Video.SHA256, DurationMS: evidence.Video.DurationMS,
				StandaloneRole: *assessment.Role,
				Provenance: TemporalStructureSourceProvenance{
					Kind: TemporalStructureSourceBoundedItem, Authority: "locked-temporal-human-review",
					Reference: mapping.CaseID, MetadataSHA256: metadataSHA, RetrievedAt: loaded.evidence.GeneratedAt,
				},
			},
		})
	}
	selected := make([]temporalStructureHoldoutSelectedAnchor, 0, temporalStructureHoldoutClassCases)
	seenFamilies := map[string]struct{}{}
	seenSourceSHA := map[string]struct{}{}
	for _, role := range []fillereval.TemporalRole{
		fillereval.TemporalRoleBumper, fillereval.TemporalRoleCommercial, fillereval.TemporalRolePromo,
		fillereval.TemporalRolePSA, fillereval.TemporalRoleTrailer,
	} {
		candidates := byRole[role]
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].receipt.RankSHA256 < candidates[j].receipt.RankSHA256 })
		for _, candidate := range candidates {
			if len(selectedRoleAnchors(selected, role)) == temporalStructureHoldoutRoleQuotas[role] {
				break
			}
			if _, duplicate := seenFamilies[candidate.receipt.FamilyID]; duplicate {
				continue
			}
			if _, duplicate := seenSourceSHA[candidate.source.SHA256]; duplicate {
				continue
			}
			selected = append(selected, candidate)
			seenFamilies[candidate.receipt.FamilyID] = struct{}{}
			seenSourceSHA[candidate.source.SHA256] = struct{}{}
		}
		if len(selectedRoleAnchors(selected, role)) != temporalStructureHoldoutRoleQuotas[role] {
			return nil, fmt.Errorf("temporal structure holdout has insufficient eligible %s anchors", role)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].receipt.RankSHA256 < selected[j].receipt.RankSHA256 })
	return selected, nil
}

func selectedRoleAnchors(anchors []temporalStructureHoldoutSelectedAnchor, role fillereval.TemporalRole) []temporalStructureHoldoutSelectedAnchor {
	var result []temporalStructureHoldoutSelectedAnchor
	for _, anchor := range anchors {
		if anchor.receipt.Role == role {
			result = append(result, anchor)
		}
	}
	return result
}

func selectTemporalStructureHoldoutParents(seed string, inventory TemporalStructureHoldoutProgrammeInventory) ([]TemporalStructureChallengeSource, error) {
	parents := append([]TemporalStructureChallengeSource(nil), inventory.Sources...)
	sort.Slice(parents, func(i, j int) bool {
		left := hashBytes([]byte(seed + "\x00programme-parent\x00" + parents[i].ID))
		right := hashBytes([]byte(seed + "\x00programme-parent\x00" + parents[j].ID))
		return left < right
	})
	if len(parents) < temporalStructureHoldoutParentSources {
		return nil, fmt.Errorf("temporal structure holdout needs six programme parents")
	}
	return parents[:temporalStructureHoldoutParentSources], nil
}

func temporalStructureHoldoutRelativeEvidencePath(sourceRoot, manifestPath, evidencePath string) (string, error) {
	absolute := filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(evidencePath))
	relative, err := filepath.Rel(sourceRoot, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("temporal structure holdout evidence source escapes the common source root")
	}
	return filepath.ToSlash(relative), nil
}
