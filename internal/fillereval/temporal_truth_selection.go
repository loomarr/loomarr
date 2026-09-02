package fillereval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

const (
	TemporalTruthSelectionSchemaVersion   = 1
	TemporalTruthSelectionContractVersion = "filler-temporal-truth-selection-v1"
	TemporalTruthSelectionCases           = 48
	temporalTruthBucketCases              = 16
	temporalTruthRiskClassCases           = 4
)

const (
	TemporalTruthBucketAgreement    = "agreement"
	TemporalTruthBucketDisagreement = "disagreement"
	TemporalTruthBucketHighRisk     = "high_risk"

	TemporalTruthRiskProgrammeExcerpt = "programme_excerpt"
	TemporalTruthRiskCompilation      = "compilation"
	TemporalTruthRiskUnusableUnclear  = "unusable_or_unclear"
	TemporalTruthRiskShortBoundary    = "short_boundary"
)

// TemporalTruthCandidate is coordinator-private recovered history. Its labels
// are sampling signals only and never become truth.
type TemporalTruthCandidate struct {
	CaseID        string                             `json:"caseId"`
	ContentSHA256 string                             `json:"contentSha256"`
	SourceLane    string                             `json:"sourceLane"`
	DurationMS    int64                              `json:"durationMs"`
	Assessments   []TemporalTruthCandidateAssessment `json:"assessments"`
}

type TemporalTruthCandidateAssessment struct {
	Assessor string       `json:"assessor"`
	Unit     UnitKind     `json:"unit"`
	Role     TemporalRole `json:"role,omitempty"`
}

type TemporalTruthInputDigest struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type TemporalTruthSelection struct {
	SchemaVersion   int                          `json:"schemaVersion"`
	ContractVersion string                       `json:"contractVersion"`
	Seed            string                       `json:"seed"`
	Inputs          []TemporalTruthInputDigest   `json:"inputs"`
	Cases           []TemporalTruthSelectionCase `json:"cases"`
}

type TemporalTruthSelectionCase struct {
	CaseID        string   `json:"caseId"`
	ContentSHA256 string   `json:"contentSha256"`
	Bucket        string   `json:"bucket"`
	RiskClass     string   `json:"riskClass,omitempty"`
	RankSHA256    string   `json:"rankSha256"`
	Strata        []string `json:"strata"`
}

// BuildTemporalTruthSelection creates the private, deterministic 16/16/16
// sampling ledger. It does not accept or emit final labels.
func BuildTemporalTruthSelection(seed string, inputs []TemporalTruthInputDigest, candidates []TemporalTruthCandidate) (TemporalTruthSelection, error) {
	seed = strings.TrimSpace(seed)
	if seed == "" {
		return TemporalTruthSelection{}, fmt.Errorf("temporal truth selection seed is required")
	}
	canonicalInputs, err := validateTemporalTruthInputs(inputs)
	if err != nil {
		return TemporalTruthSelection{}, err
	}
	if err := validateTemporalTruthCandidates(candidates); err != nil {
		return TemporalTruthSelection{}, err
	}

	selected := make(map[string]struct{}, TemporalTruthSelectionCases)
	result := TemporalTruthSelection{
		SchemaVersion: TemporalTruthSelectionSchemaVersion, ContractVersion: TemporalTruthSelectionContractVersion,
		Seed: seed, Inputs: canonicalInputs,
	}
	for _, riskClass := range []string{
		TemporalTruthRiskProgrammeExcerpt,
		TemporalTruthRiskCompilation,
		TemporalTruthRiskUnusableUnclear,
		TemporalTruthRiskShortBoundary,
	} {
		pool := filterTemporalTruthCandidates(candidates, selected, func(candidate TemporalTruthCandidate) bool {
			return temporalTruthRiskClass(candidate) == riskClass
		})
		picked, pickErr := pickTemporalTruthDiverse(seed, TemporalTruthBucketHighRisk+":"+riskClass, pool, temporalTruthRiskClassCases, func(candidate TemporalTruthCandidate) string {
			return strings.Join([]string{candidate.SourceLane, temporalTruthDurationBand(candidate.DurationMS), temporalTruthOutcomeSignature(candidate)}, "\x00")
		})
		if pickErr != nil {
			return TemporalTruthSelection{}, pickErr
		}
		for _, candidate := range picked {
			selected[candidate.CaseID] = struct{}{}
			result.Cases = append(result.Cases, temporalTruthSelectedCase(seed, TemporalTruthBucketHighRisk, riskClass, candidate))
		}
	}

	agreements := filterTemporalTruthCandidates(candidates, selected, func(candidate TemporalTruthCandidate) bool {
		return temporalTruthRiskClass(candidate) == "" && temporalTruthExactAgreement(candidate)
	})
	pickedAgreements, err := pickTemporalTruthDiverse(seed, TemporalTruthBucketAgreement, agreements, temporalTruthBucketCases, func(candidate TemporalTruthCandidate) string {
		return strings.Join([]string{temporalTruthOutcomeSignature(candidate), temporalTruthDurationBand(candidate.DurationMS), candidate.SourceLane}, "\x00")
	})
	if err != nil {
		return TemporalTruthSelection{}, err
	}
	for _, candidate := range pickedAgreements {
		selected[candidate.CaseID] = struct{}{}
		result.Cases = append(result.Cases, temporalTruthSelectedCase(seed, TemporalTruthBucketAgreement, "", candidate))
	}

	disagreements := filterTemporalTruthCandidates(candidates, selected, func(candidate TemporalTruthCandidate) bool {
		return temporalTruthRiskClass(candidate) == "" && !temporalTruthExactAgreement(candidate)
	})
	pickedDisagreements, err := pickTemporalTruthDiverse(seed, TemporalTruthBucketDisagreement, disagreements, temporalTruthBucketCases, func(candidate TemporalTruthCandidate) string {
		return strings.Join([]string{temporalTruthOutcomeSignature(candidate), temporalTruthDurationBand(candidate.DurationMS), candidate.SourceLane}, "\x00")
	})
	if err != nil {
		return TemporalTruthSelection{}, err
	}
	for _, candidate := range pickedDisagreements {
		result.Cases = append(result.Cases, temporalTruthSelectedCase(seed, TemporalTruthBucketDisagreement, "", candidate))
	}

	sort.Slice(result.Cases, func(i, j int) bool {
		a, b := result.Cases[i], result.Cases[j]
		if a.Bucket != b.Bucket {
			return a.Bucket < b.Bucket
		}
		if a.RiskClass != b.RiskClass {
			return a.RiskClass < b.RiskClass
		}
		return a.RankSHA256 < b.RankSHA256
	})
	return result, nil
}

func DecodeTemporalTruthSelection(data []byte) (TemporalTruthSelection, error) {
	var selection TemporalTruthSelection
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&selection); err != nil {
		return TemporalTruthSelection{}, fmt.Errorf("decode temporal truth selection: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return TemporalTruthSelection{}, fmt.Errorf("decode temporal truth selection: trailing JSON value")
		}
		return TemporalTruthSelection{}, fmt.Errorf("decode temporal truth selection trailing value: %w", err)
	}
	if err := ValidateTemporalTruthSelection(selection); err != nil {
		return TemporalTruthSelection{}, err
	}
	return selection, nil
}

func ValidateTemporalTruthSelection(selection TemporalTruthSelection) error {
	if selection.SchemaVersion != TemporalTruthSelectionSchemaVersion || selection.ContractVersion != TemporalTruthSelectionContractVersion || strings.TrimSpace(selection.Seed) == "" {
		return fmt.Errorf("temporal truth selection identity is invalid")
	}
	inputs, err := validateTemporalTruthInputs(selection.Inputs)
	if err != nil || !slices.Equal(inputs, selection.Inputs) {
		return fmt.Errorf("temporal truth selection input digests are invalid or not canonical")
	}
	if len(selection.Cases) != TemporalTruthSelectionCases {
		return fmt.Errorf("temporal truth selection has %d cases; requires exactly %d", len(selection.Cases), TemporalTruthSelectionCases)
	}
	buckets := map[string]int{}
	risks := map[string]int{}
	caseIDs := map[string]struct{}{}
	content := map[string]struct{}{}
	previousOrder := ""
	for index, item := range selection.Cases {
		if strings.TrimSpace(item.CaseID) == "" || !validSHA256(item.ContentSHA256) || !validSHA256(item.RankSHA256) || len(item.Strata) < 3 || !strictSortedUnique(item.Strata) {
			return fmt.Errorf("temporal truth selection case %d is invalid", index)
		}
		if _, duplicate := caseIDs[item.CaseID]; duplicate {
			return fmt.Errorf("temporal truth selection repeats case %q", item.CaseID)
		}
		if _, duplicate := content[item.ContentSHA256]; duplicate {
			return fmt.Errorf("temporal truth selection repeats content %q", item.ContentSHA256)
		}
		caseIDs[item.CaseID] = struct{}{}
		content[item.ContentSHA256] = struct{}{}
		order := item.Bucket + "\x00" + item.RiskClass + "\x00" + item.RankSHA256
		if index > 0 && order <= previousOrder {
			return fmt.Errorf("temporal truth selection cases are not canonically ordered")
		}
		previousOrder = order
		buckets[item.Bucket]++
		if item.Bucket == TemporalTruthBucketHighRisk {
			risks[item.RiskClass]++
		} else if item.RiskClass != "" {
			return fmt.Errorf("temporal truth selection non-risk case carries a risk class")
		}
	}
	for _, bucket := range []string{TemporalTruthBucketAgreement, TemporalTruthBucketDisagreement, TemporalTruthBucketHighRisk} {
		if buckets[bucket] != temporalTruthBucketCases {
			return fmt.Errorf("temporal truth selection bucket %q has %d cases; requires %d", bucket, buckets[bucket], temporalTruthBucketCases)
		}
	}
	if len(buckets) != 3 {
		return fmt.Errorf("temporal truth selection contains an unknown bucket")
	}
	for _, riskClass := range []string{TemporalTruthRiskProgrammeExcerpt, TemporalTruthRiskCompilation, TemporalTruthRiskUnusableUnclear, TemporalTruthRiskShortBoundary} {
		if risks[riskClass] != temporalTruthRiskClassCases {
			return fmt.Errorf("temporal truth selection risk class %q has %d cases; requires %d", riskClass, risks[riskClass], temporalTruthRiskClassCases)
		}
	}
	if len(risks) != 4 {
		return fmt.Errorf("temporal truth selection contains an unknown risk class")
	}
	return nil
}

func validateTemporalTruthInputs(inputs []TemporalTruthInputDigest) ([]TemporalTruthInputDigest, error) {
	result := slices.Clone(inputs)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	for index, input := range result {
		if strings.TrimSpace(input.Name) == "" || !validSHA256(input.SHA256) || index > 0 && input.Name <= result[index-1].Name {
			return nil, fmt.Errorf("temporal truth selection input digests must be named, unique, and valid")
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("temporal truth selection requires input digests")
	}
	return result, nil
}

func validateTemporalTruthCandidates(candidates []TemporalTruthCandidate) error {
	caseIDs := make(map[string]struct{}, len(candidates))
	content := make(map[string]struct{}, len(candidates))
	for index, candidate := range candidates {
		if strings.TrimSpace(candidate.CaseID) == "" || !validSHA256(candidate.ContentSHA256) || strings.TrimSpace(candidate.SourceLane) == "" || candidate.DurationMS <= 0 || len(candidate.Assessments) != 3 {
			return fmt.Errorf("temporal truth candidate %d has invalid identity, source, duration, or assessment count", index)
		}
		if _, duplicate := caseIDs[candidate.CaseID]; duplicate {
			return fmt.Errorf("temporal truth candidates repeat case %q", candidate.CaseID)
		}
		if _, duplicate := content[candidate.ContentSHA256]; duplicate {
			return fmt.Errorf("temporal truth candidates repeat content %q", candidate.ContentSHA256)
		}
		caseIDs[candidate.CaseID] = struct{}{}
		content[candidate.ContentSHA256] = struct{}{}
		assessors := make(map[string]struct{}, 3)
		for _, assessment := range candidate.Assessments {
			if strings.TrimSpace(assessment.Assessor) == "" || !validUnitKind(assessment.Unit) || assessment.Unit == UnitStandalone && !validTemporalRoleKind(assessment.Role) || assessment.Unit != UnitStandalone && assessment.Role != "" {
				return fmt.Errorf("temporal truth candidate %q has invalid normalized assessment", candidate.CaseID)
			}
			if _, duplicate := assessors[assessment.Assessor]; duplicate {
				return fmt.Errorf("temporal truth candidate %q repeats assessor %q", candidate.CaseID, assessment.Assessor)
			}
			assessors[assessment.Assessor] = struct{}{}
		}
	}
	return nil
}

func validTemporalRoleKind(role TemporalRole) bool {
	switch role {
	case TemporalRoleCommercial, TemporalRolePromo, TemporalRoleBumper, TemporalRolePSA, TemporalRoleStationID, TemporalRoleTrailer, TemporalRoleInterstitial, TemporalRoleUnclear:
		return true
	default:
		return false
	}
}

func temporalTruthRiskClass(candidate TemporalTruthCandidate) string {
	if temporalTruthHasUnit(candidate, UnitProgrammeExcerpt) {
		return TemporalTruthRiskProgrammeExcerpt
	}
	if temporalTruthHasUnit(candidate, UnitCompilation) {
		return TemporalTruthRiskCompilation
	}
	if temporalTruthHasUnit(candidate, UnitUnusable) || temporalTruthHasUnit(candidate, UnitUnclear) {
		return TemporalTruthRiskUnusableUnclear
	}
	if candidate.DurationMS < 15_000 {
		return TemporalTruthRiskShortBoundary
	}
	return ""
}

func temporalTruthHasUnit(candidate TemporalTruthCandidate, unit UnitKind) bool {
	return slices.ContainsFunc(candidate.Assessments, func(assessment TemporalTruthCandidateAssessment) bool {
		return assessment.Unit == unit
	})
}

func temporalTruthExactAgreement(candidate TemporalTruthCandidate) bool {
	first := candidate.Assessments[0]
	for _, assessment := range candidate.Assessments[1:] {
		if assessment.Unit != first.Unit || assessment.Role != first.Role {
			return false
		}
	}
	return true
}

func temporalTruthOutcomeSignature(candidate TemporalTruthCandidate) string {
	values := make([]string, 0, len(candidate.Assessments))
	for _, assessment := range candidate.Assessments {
		value := string(assessment.Unit)
		if assessment.Role != "" {
			value += "/" + string(assessment.Role)
		}
		values = append(values, value)
	}
	slices.Sort(values)
	return strings.Join(values, "+")
}

func temporalTruthDurationBand(durationMS int64) string {
	switch {
	case durationMS < 15_000:
		return "under_15s"
	case durationMS < 60_000:
		return "15s_to_60s"
	default:
		return "over_60s"
	}
}

func temporalTruthSelectedCase(seed, bucket, riskClass string, candidate TemporalTruthCandidate) TemporalTruthSelectionCase {
	strata := []string{
		"duration:" + temporalTruthDurationBand(candidate.DurationMS),
		"history:" + temporalTruthOutcomeSignature(candidate),
		"source:" + candidate.SourceLane,
	}
	if riskClass != "" {
		strata = append(strata, "risk:"+riskClass)
	}
	slices.Sort(strata)
	return TemporalTruthSelectionCase{
		CaseID: candidate.CaseID, ContentSHA256: candidate.ContentSHA256, Bucket: bucket,
		RiskClass: riskClass, RankSHA256: temporalTruthRank(seed, bucket+":"+riskClass, candidate.ContentSHA256), Strata: strata,
	}
}

func filterTemporalTruthCandidates(candidates []TemporalTruthCandidate, selected map[string]struct{}, include func(TemporalTruthCandidate) bool) []TemporalTruthCandidate {
	result := make([]TemporalTruthCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, exists := selected[candidate.CaseID]; !exists && include(candidate) {
			result = append(result, candidate)
		}
	}
	return result
}

func pickTemporalTruthDiverse(seed, bucket string, pool []TemporalTruthCandidate, count int, group func(TemporalTruthCandidate) string) ([]TemporalTruthCandidate, error) {
	if len(pool) < count {
		return nil, fmt.Errorf("temporal truth selection bucket %q has %d candidates; requires %d with no fallback borrowing", bucket, len(pool), count)
	}
	groups := make(map[string][]TemporalTruthCandidate)
	for _, candidate := range pool {
		key := group(candidate)
		groups[key] = append(groups[key], candidate)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
		sort.Slice(groups[key], func(i, j int) bool {
			return temporalTruthRank(seed, bucket, groups[key][i].ContentSHA256) < temporalTruthRank(seed, bucket, groups[key][j].ContentSHA256)
		})
	}
	sort.Slice(keys, func(i, j int) bool {
		return temporalTruthRank(seed, bucket+":group", keys[i]) < temporalTruthRank(seed, bucket+":group", keys[j])
	})
	result := make([]TemporalTruthCandidate, 0, count)
	for depth := 0; len(result) < count; depth++ {
		added := false
		for _, key := range keys {
			if depth < len(groups[key]) {
				result = append(result, groups[key][depth])
				added = true
				if len(result) == count {
					break
				}
			}
		}
		if !added {
			return nil, fmt.Errorf("temporal truth selection bucket %q could not fill its declared quota", bucket)
		}
	}
	return result, nil
}

func temporalTruthRank(seed, bucket, identity string) string {
	sum := sha256.Sum256([]byte(seed + "\x00" + bucket + "\x00" + identity))
	return hex.EncodeToString(sum[:])
}
