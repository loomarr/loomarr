package fillerreference

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/mediatools"
)

type InspectionSeed struct {
	SchemaVersion        int                     `json:"schemaVersion"`
	Kind                 string                  `json:"kind"`
	Status               string                  `json:"status"`
	ContractVersion      string                  `json:"contractVersion"`
	SourceAuditSHA256    string                  `json:"sourceAuditSha256"`
	DuplicateAuditSHA256 string                  `json:"duplicateAuditSha256"`
	TriageEvidence       TriageEvidence          `json:"triageEvidence"`
	SelectionPolicy      InspectionSelection     `json:"selectionPolicy"`
	SelectedCaseIDs      []string                `json:"selectedCaseIds"`
	ExplicitFindings     []ExplicitTriageFinding `json:"explicitTriageFindings"`
	RequiredNextGate     string                  `json:"requiredNextGate"`
}

type TriageEvidence struct {
	Method                 string `json:"method"`
	SampledFramesPerSource int    `json:"sampledFramesPerSource"`
	Limitations            string `json:"limitations"`
}

type InspectionSelection struct {
	TargetSources      int      `json:"targetSources"`
	FinalAcceptedClips int      `json:"finalAcceptedClips"`
	Prefer             []string `json:"prefer"`
	DoNotBackfill      []string `json:"doNotBackfill"`
	CoverageShortfall  string   `json:"coverageShortfall"`
}

type ExplicitTriageFinding struct {
	CaseID          string `json:"caseId,omitempty"`
	CaseIDPattern   string `json:"caseIdPattern,omitempty"`
	Disposition     string `json:"disposition"`
	PreferredCaseID string `json:"preferredCaseId,omitempty"`
	Evidence        string `json:"evidence"`
}

type MeasurementInputIdentity struct {
	AuditSHA256  string `json:"auditSha256"`
	FamilySHA256 string `json:"familyAuditSha256"`
	SeedSHA256   string `json:"inspectionSeedSha256"`
}

type RawInspectionInputs struct {
	Audit    []byte
	Families []byte
	Seed     []byte
}

type InspectionPlan struct {
	Inputs    MeasurementInputIdentity
	Cases     []Case
	NotBefore time.Time
}

type InspectionMeasurement struct {
	CaseID           string                              `json:"caseId"`
	ContentSHA256    string                              `json:"contentSha256"`
	LocalFile        string                              `json:"localFile"`
	Status           string                              `json:"status"`
	Measurement      *mediatools.ConditioningMeasurement `json:"measurement,omitempty"`
	MeasurementError string                              `json:"measurementError,omitempty"`
}

type MeasurementSummary struct {
	Cases          int `json:"cases"`
	Measured       int `json:"measured"`
	TechnicalHolds int `json:"technicalHolds"`
}

type MeasurementArtifact struct {
	SchemaVersion int                      `json:"schemaVersion"`
	GeneratedAt   time.Time                `json:"generatedAt"`
	Inputs        MeasurementInputIdentity `json:"inputs"`
	Summary       MeasurementSummary       `json:"summary"`
	Cases         []InspectionMeasurement  `json:"cases"`
}

// BuildInspectionPlan owns the exact Gate A, family, and seed bytes and returns
// selected cases only after recomputing every derived family relationship.
func BuildInspectionPlan(raw RawInspectionInputs) (InspectionPlan, error) {
	audit, err := decodeStrictJSON[Audit](raw.Audit)
	if err != nil {
		return InspectionPlan{}, fmt.Errorf("source audit: %w", err)
	}
	families, err := decodeStrictJSON[FamilyAudit](raw.Families)
	if err != nil {
		return InspectionPlan{}, fmt.Errorf("duplicate inventory: %w", err)
	}
	seed, err := decodeStrictJSON[InspectionSeed](raw.Seed)
	if err != nil {
		return InspectionPlan{}, fmt.Errorf("inspection seed: %w", err)
	}
	inputs := MeasurementInputIdentity{AuditSHA256: SHA256(raw.Audit), FamilySHA256: SHA256(raw.Families), SeedSHA256: SHA256(raw.Seed)}
	if err := validateFamilyAuditBinding(audit, families, inputs.AuditSHA256); err != nil {
		return InspectionPlan{}, fmt.Errorf("duplicate inventory binding: %w", err)
	}
	if seed.SchemaVersion != 1 || seed.Kind != "filler_reference_inspection_seed" || seed.Status != "preliminary_requires_full_playback" || seed.ContractVersion != ContractVersion {
		return InspectionPlan{}, fmt.Errorf("inspection seed identity is invalid")
	}
	if !validSHA256(inputs.AuditSHA256) || !validSHA256(inputs.FamilySHA256) || !validSHA256(inputs.SeedSHA256) || seed.SourceAuditSHA256 != inputs.AuditSHA256 || seed.DuplicateAuditSHA256 != inputs.FamilySHA256 || families.SourceAudit != inputs.AuditSHA256 {
		return InspectionPlan{}, fmt.Errorf("inspection seed input binding is invalid")
	}
	if seed.SelectionPolicy.TargetSources != 50 || seed.SelectionPolicy.FinalAcceptedClips != 32 || len(seed.SelectedCaseIDs) != seed.SelectionPolicy.TargetSources || seed.TriageEvidence.SampledFramesPerSource != 4 || strings.TrimSpace(seed.TriageEvidence.Method) == "" || strings.TrimSpace(seed.TriageEvidence.Limitations) == "" || len(seed.SelectionPolicy.Prefer) == 0 || len(seed.SelectionPolicy.DoNotBackfill) == 0 || strings.TrimSpace(seed.SelectionPolicy.CoverageShortfall) == "" || strings.TrimSpace(seed.RequiredNextGate) == "" {
		return InspectionPlan{}, fmt.Errorf("inspection seed size, evidence, policy, or gate is invalid")
	}
	byID := make(map[string]Case, len(audit.Cases))
	for _, item := range audit.Cases {
		byID[item.CaseID] = item
	}
	seen := map[string]struct{}{}
	selected := make([]Case, 0, len(seed.SelectedCaseIDs))
	for _, id := range seed.SelectedCaseIDs {
		item, ok := byID[id]
		if !ok || item.Disposition == DispositionExclude || item.ContentSHA256 == "" || item.SourceLocalFile == "" || item.Media.SourceDurationMS <= 0 || item.Media.SourceDurationMS > mediatools.ConditioningMaxDurationMs {
			return InspectionPlan{}, fmt.Errorf("inspection case %q is absent, excluded, or outside the measurement contract", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return InspectionPlan{}, fmt.Errorf("inspection case %q repeats", id)
		}
		seen[id] = struct{}{}
		selected = append(selected, item)
	}
	if err := validateTriageFindings(seed.ExplicitFindings, byID); err != nil {
		return InspectionPlan{}, err
	}
	if selectedFamilyCollision(families, seen) && !hasFamilyInspectionHold(seed.ExplicitFindings) {
		return InspectionPlan{}, fmt.Errorf("inspection seed selects related renditions without an explicit family hold")
	}
	return InspectionPlan{Inputs: inputs, Cases: selected, NotBefore: maxTime(audit.GeneratedAt, families.GeneratedAt)}, nil
}

func validateFamilyAuditBinding(audit Audit, families FamilyAudit, auditSHA256 string) error {
	if families.SchemaVersion != 3 || families.Algorithm != DuplicateAlgorithm || families.GeneratedAt.IsZero() || families.SourceAudit != auditSHA256 {
		return fmt.Errorf("duplicate inventory identity is invalid")
	}
	if err := validateFamilyInputs(audit, families.Fingerprints, families.GeneratedAt); err != nil {
		return err
	}
	if !slices.IsSortedFunc(families.Fingerprints, func(a, b FamilyFingerprint) int { return strings.Compare(a.CaseID, b.CaseID) }) || families.Summary.Cases != len(families.Fingerprints) || families.Summary.RelatedPairs != len(families.Pairs) || families.Summary.ClosestNonMatches != len(families.ClosestPairs) || families.Summary.DuplicateFamilies != len(families.Families) {
		return fmt.Errorf("duplicate inventory ordering or summary is invalid")
	}
	known := make(map[string]int, len(families.Fingerprints))
	parents := make([]int, len(families.Fingerprints))
	for index, fingerprint := range families.Fingerprints {
		known[fingerprint.CaseID] = index
		parents[index] = index
	}
	var find func(int) int
	find = func(index int) int {
		if parents[index] != index {
			parents[index] = find(parents[index])
		}
		return parents[index]
	}
	pairSet := make(map[string]struct{}, len(families.Pairs))
	for _, pair := range families.Pairs {
		left, leftOK := known[pair.CaseA]
		right, rightOK := known[pair.CaseB]
		key := pair.CaseA + "\x00" + pair.CaseB
		if !leftOK || !rightOK || pair.CaseA >= pair.CaseB || !validFamilyPair(pair, true) {
			return fmt.Errorf("duplicate inventory contains an invalid related pair %q", key)
		}
		if _, duplicate := pairSet[key]; duplicate {
			return fmt.Errorf("duplicate inventory repeats related pair %q", key)
		}
		pairSet[key] = struct{}{}
		left, right = find(left), find(right)
		if left != right {
			parents[right] = left
		}
	}
	closestSet := make(map[string]struct{}, len(families.ClosestPairs))
	for _, pair := range families.ClosestPairs {
		_, leftOK := known[pair.CaseA]
		_, rightOK := known[pair.CaseB]
		key := pair.CaseA + "\x00" + pair.CaseB
		if !leftOK || !rightOK || pair.CaseA >= pair.CaseB || !validFamilyPair(pair, false) {
			return fmt.Errorf("duplicate inventory contains an invalid closest non-match %q", key)
		}
		if _, related := pairSet[key]; related {
			return fmt.Errorf("duplicate inventory pair %q is both related and unrelated", key)
		}
		if _, duplicate := closestSet[key]; duplicate {
			return fmt.Errorf("duplicate inventory repeats closest non-match %q", key)
		}
		closestSet[key] = struct{}{}
	}
	components := make(map[int][]string)
	for id, index := range known {
		components[find(index)] = append(components[find(index)], id)
	}
	expectedFamilies := make(map[string][]string)
	for _, members := range components {
		if len(members) < 2 {
			continue
		}
		slices.Sort(members)
		expectedFamilies[duplicateFamilyID(members)] = members
	}
	if len(expectedFamilies) != len(families.Families) {
		return fmt.Errorf("duplicate inventory families do not match the related-pair graph")
	}
	nonClique := 0
	seenFamilies := make(map[string]struct{}, len(families.Families))
	for _, family := range families.Families {
		expected, ok := expectedFamilies[family.FamilyID]
		if !ok || !slices.Equal(family.Members, expected) || family.PreferredCase != "" {
			return fmt.Errorf("duplicate family %q does not match its related-pair component", family.FamilyID)
		}
		if _, duplicate := seenFamilies[family.FamilyID]; duplicate {
			return fmt.Errorf("duplicate inventory repeats family %q", family.FamilyID)
		}
		seenFamilies[family.FamilyID] = struct{}{}
		clique := true
		for left := range family.Members {
			for right := left + 1; right < len(family.Members); right++ {
				if _, ok := pairSet[family.Members[left]+"\x00"+family.Members[right]]; !ok {
					clique = false
				}
			}
		}
		if family.CompleteClique != clique {
			return fmt.Errorf("duplicate family %q has an invalid clique claim", family.FamilyID)
		}
		if !clique {
			nonClique++
		}
	}
	if families.Summary.NonCliqueFamilies != nonClique {
		return fmt.Errorf("duplicate inventory non-clique summary is invalid")
	}
	return nil
}

func validFamilyPair(pair FamilyPair, related bool) bool {
	basis := make([]string, 0, 2)
	if pair.Comparison.Related {
		basis = append(basis, "visual")
	}
	if pair.Audio.Related {
		basis = append(basis, "audio")
	}
	if related != (len(basis) > 0) || !slices.Equal(pair.Basis, basis) {
		return false
	}
	return pair.Comparison.MatchedFrames >= 0 && pair.Comparison.ComparedFramesA >= 0 && pair.Comparison.ComparedFramesB >= 0 && pair.Comparison.Coverage >= 0 && pair.Comparison.Coverage <= 1 && pair.Comparison.MeanDistance >= 0 && pair.Comparison.MaximumDistance >= 0 && pair.Audio.ComparedBins >= 0 && pair.Audio.Coverage >= 0 && pair.Audio.Coverage <= 1 && pair.Audio.Correlation >= -1 && pair.Audio.Correlation <= 1
}

func validateTriageFindings(findings []ExplicitTriageFinding, cases map[string]Case) error {
	if len(findings) == 0 {
		return fmt.Errorf("inspection seed has no explicit triage findings")
	}
	seen := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		if (finding.CaseID == "") == (finding.CaseIDPattern == "") || strings.TrimSpace(finding.Evidence) == "" {
			return fmt.Errorf("inspection seed has an incomplete triage finding")
		}
		if finding.CaseID != "" {
			if _, ok := cases[finding.CaseID]; !ok {
				return fmt.Errorf("inspection finding names unknown case %q", finding.CaseID)
			}
		}
		if finding.CaseIDPattern != "" && finding.CaseIDPattern != "*" && finding.CaseIDPattern != "duplicate-family-*" {
			return fmt.Errorf("inspection finding has unsupported pattern %q", finding.CaseIDPattern)
		}
		if finding.Disposition != "inspection_hold" && finding.Disposition != "no_editorial_acceptance" {
			return fmt.Errorf("inspection finding has unsupported disposition %q", finding.Disposition)
		}
		key := finding.CaseID + "\x00" + finding.CaseIDPattern
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("inspection seed repeats triage finding %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func selectedFamilyCollision(families FamilyAudit, selected map[string]struct{}) bool {
	for _, family := range families.Families {
		count := 0
		for _, id := range family.Members {
			if _, ok := selected[id]; ok {
				count++
			}
		}
		if count > 1 {
			return true
		}
	}
	return false
}

func hasFamilyInspectionHold(findings []ExplicitTriageFinding) bool {
	for _, finding := range findings {
		if finding.CaseIDPattern == "duplicate-family-*" && finding.Disposition == "inspection_hold" && strings.TrimSpace(finding.Evidence) != "" {
			return true
		}
	}
	return false
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
