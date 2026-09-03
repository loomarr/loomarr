package fillerreview

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	TemporalStructureCertificationSchemaVersion     = 1
	TemporalStructureCertificationContractVersion   = "filler-temporal-structure-certification-v1"
	TemporalStructureCertificationPassed            = "passed"
	TemporalStructureCertificationFailed            = "failed"
	temporalStructureCertificationMinimumCases      = TemporalStructureHoldoutCases
	temporalStructureCertificationMinimumSliceCases = 6
)

var temporalStructureCertificationRequiredSlices = []string{
	TemporalStructureSliceAdjacentSameRole,
	TemporalStructureSliceMixedRoleJoins,
	TemporalStructureSliceProgrammeNearEnd,
	TemporalStructureSliceProgrammeNearStart,
	TemporalStructureSliceSpotEarly,
	TemporalStructureSliceSpotLate,
	TemporalStructureSliceThreeItemCompilation,
	TemporalStructureSliceTwoItemCompilation,
}

type TemporalStructureCertificationConfig struct {
	HoldoutAuthoringPath string
	HoldoutReceiptPath   string
	PublicManifestPath   string
	PrivateAuthorityPath string
	AssessmentPaths      []string
	ComparedAt           time.Time
	CertifiedAt          time.Time
	OutputPath           string
}

type TemporalStructureCertificationReport struct {
	SchemaVersion              int                                   `json:"schemaVersion"`
	ContractVersion            string                                `json:"contractVersion"`
	CertifiedAt                time.Time                             `json:"certifiedAt"`
	ChallengeID                string                                `json:"challengeId"`
	HoldoutAuthoringSHA256     string                                `json:"holdoutAuthoringSha256"`
	HoldoutReceiptSHA256       string                                `json:"holdoutReceiptSha256"`
	PublicManifestSHA256       string                                `json:"publicManifestSha256"`
	PrivateAuthoritySHA256     string                                `json:"privateAuthoritySha256"`
	ComparisonSHA256           string                                `json:"comparisonSha256"`
	Cases                      int                                   `json:"cases"`
	AssessorIDs                []string                              `json:"assessorIds"`
	BoundaryToleranceMS        int64                                 `json:"boundaryToleranceMs"`
	MinimumSliceCases          int                                   `json:"minimumSliceCases"`
	Slices                     []TemporalStructureSliceCertification `json:"slices"`
	CertifiedSlices            []string                              `json:"certifiedSlices"`
	FailureCodes               []string                              `json:"failureCodes,omitempty"`
	CertificationStatus        string                                `json:"certificationStatus"`
	TrainingAllowed            bool                                  `json:"trainingAllowed"`
	ProductionAdmissionAllowed bool                                  `json:"productionAdmissionAllowed"`
	NextAction                 string                                `json:"nextAction"`
}

type TemporalStructureSliceCertification struct {
	Slice        string   `json:"slice"`
	Cases        int      `json:"cases"`
	Assessors    int      `json:"assessors"`
	FailureCodes []string `json:"failureCodes,omitempty"`
	Passed       bool     `json:"passed"`
}

// PublishTemporalStructureCertification reproduces comparison from the locked
// assessment sets and binds it to the source-family-disjoint holdout receipt.
// Passing this development gate permits only a shadow comparison.
func PublishTemporalStructureCertification(config TemporalStructureCertificationConfig) (TemporalStructureCertificationReport, string, error) {
	paths := []string{
		config.HoldoutAuthoringPath, config.HoldoutReceiptPath, config.PublicManifestPath,
		config.PrivateAuthorityPath, config.OutputPath,
	}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			return TemporalStructureCertificationReport{}, "", fmt.Errorf("temporal structure certification requires every authority and output path")
		}
	}
	if config.ComparedAt.IsZero() || config.CertifiedAt.IsZero() || config.CertifiedAt.Before(config.ComparedAt) {
		return TemporalStructureCertificationReport{}, "", fmt.Errorf("temporal structure certification requires ordered comparison and certification times")
	}

	authoringRaw, err := os.ReadFile(config.HoldoutAuthoringPath)
	if err != nil {
		return TemporalStructureCertificationReport{}, "", fmt.Errorf("read temporal structure holdout authoring: %w", err)
	}
	receiptRaw, err := os.ReadFile(config.HoldoutReceiptPath)
	if err != nil {
		return TemporalStructureCertificationReport{}, "", fmt.Errorf("read temporal structure holdout receipt: %w", err)
	}
	authoring, err := readStrictJSON[TemporalStructureChallengeAuthoring](config.HoldoutAuthoringPath)
	if err != nil {
		return TemporalStructureCertificationReport{}, "", fmt.Errorf("decode temporal structure holdout authoring: %w", err)
	}
	receipt, err := readStrictJSON[TemporalStructureHoldoutReceipt](config.HoldoutReceiptPath)
	if err != nil {
		return TemporalStructureCertificationReport{}, "", fmt.Errorf("decode temporal structure holdout receipt: %w", err)
	}
	if hashBytes(authoringRaw) != receipt.AuthoringSHA256 {
		return TemporalStructureCertificationReport{}, "", fmt.Errorf("temporal structure holdout receipt does not bind authoring bytes")
	}
	if err := validateTemporalStructureHoldoutReceipt(receipt, authoring); err != nil {
		return TemporalStructureCertificationReport{}, "", err
	}

	loaded, err := loadTemporalStructureComparison(TemporalStructureComparisonConfig{
		PublicManifestPath: config.PublicManifestPath, PrivateAuthorityPath: config.PrivateAuthorityPath,
		AssessmentPaths: config.AssessmentPaths, ExpectedCases: TemporalStructureHoldoutCases,
		ComparedAt: config.ComparedAt,
	})
	if err != nil {
		return TemporalStructureCertificationReport{}, "", err
	}
	if loaded.authority.AuthoringSHA256 != receipt.AuthoringSHA256 || loaded.authority.SeedSHA256 != receipt.SeedSHA256 || loaded.authority.GeneratedAt.Before(receipt.PlannedAt) {
		return TemporalStructureCertificationReport{}, "", fmt.Errorf("temporal structure challenge does not descend from the certified holdout")
	}
	comparison := buildTemporalStructureComparison(loaded, config.ComparedAt.UTC())
	comparisonRaw, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		return TemporalStructureCertificationReport{}, "", err
	}
	comparisonRaw = append(comparisonRaw, '\n')
	report := scoreTemporalStructureCertification(comparison, config.CertifiedAt)
	report.HoldoutAuthoringSHA256 = hashBytes(authoringRaw)
	report.HoldoutReceiptSHA256 = hashBytes(receiptRaw)
	report.PublicManifestSHA256 = loaded.publicSHA
	report.PrivateAuthoritySHA256 = loaded.authoritySHA
	report.ComparisonSHA256 = hashBytes(comparisonRaw)
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return TemporalStructureCertificationReport{}, "", err
	}
	raw = append(raw, '\n')
	if err := writeTemporalTruthNew(config.OutputPath, raw, 0o600); err != nil {
		return TemporalStructureCertificationReport{}, "", fmt.Errorf("publish temporal structure certification: %w", err)
	}
	return report, hashBytes(raw), nil
}

func scoreTemporalStructureCertification(comparison TemporalStructureComparisonReport, certifiedAt time.Time) TemporalStructureCertificationReport {
	report := TemporalStructureCertificationReport{
		SchemaVersion: TemporalStructureCertificationSchemaVersion, ContractVersion: TemporalStructureCertificationContractVersion,
		CertifiedAt: certifiedAt.UTC(), ChallengeID: comparison.ChallengeID, Cases: comparison.Cases,
		BoundaryToleranceMS: TemporalStructureNearBoundaryMS, MinimumSliceCases: temporalStructureCertificationMinimumSliceCases,
		TrainingAllowed: false, ProductionAdmissionAllowed: false,
	}
	for _, assessor := range comparison.Assessors {
		report.AssessorIDs = append(report.AssessorIDs, assessor.Assessor.ID)
	}
	sort.Strings(report.AssessorIDs)
	report.AssessorIDs = slices.Compact(report.AssessorIDs)
	if comparison.Cases < temporalStructureCertificationMinimumCases {
		report.FailureCodes = append(report.FailureCodes, "insufficient_cases")
	}
	if len(report.AssessorIDs) < 2 || len(comparison.AssessorSummaries) != len(report.AssessorIDs) {
		report.FailureCodes = append(report.FailureCodes, "insufficient_independent_assessors")
	}
	knownAssessors := make(map[string]struct{}, len(report.AssessorIDs))
	for _, assessorID := range report.AssessorIDs {
		knownAssessors[assessorID] = struct{}{}
	}
	seenSummaries := make(map[string]struct{}, len(comparison.AssessorSummaries))
	for _, summary := range comparison.AssessorSummaries {
		if _, known := knownAssessors[summary.AssessorID]; !known {
			report.FailureCodes = append(report.FailureCodes, "unknown_assessor_summary")
		}
		if _, duplicate := seenSummaries[summary.AssessorID]; duplicate {
			report.FailureCodes = append(report.FailureCodes, "duplicate_assessor_summary")
		}
		seenSummaries[summary.AssessorID] = struct{}{}
		if summary.Cases != comparison.Cases {
			report.FailureCodes = append(report.FailureCodes, "assessor_case_count")
		}
		report.FailureCodes = append(report.FailureCodes, temporalStructureSummaryFailures(summary)...)
	}
	report.FailureCodes = sortedUniqueStrings(report.FailureCodes)

	bySlice := make(map[string][]TemporalStructureConstructionSummary)
	for _, summary := range comparison.SliceSummaries {
		bySlice[summary.Slice] = append(bySlice[summary.Slice], summary)
	}
	for _, slice := range temporalStructureCertificationRequiredSlices {
		metric := TemporalStructureSliceCertification{Slice: slice}
		seenAssessors := map[string]struct{}{}
		for index, summary := range bySlice[slice] {
			if index == 0 || summary.Cases < metric.Cases {
				metric.Cases = summary.Cases
			}
			seenAssessors[summary.AssessorID] = struct{}{}
			if _, known := knownAssessors[summary.AssessorID]; !known {
				metric.FailureCodes = append(metric.FailureCodes, "unknown_slice_assessor")
			}
			metric.FailureCodes = append(metric.FailureCodes, temporalStructureConstructionFailures(summary)...)
		}
		metric.Assessors = len(seenAssessors)
		if len(bySlice[slice]) == 0 {
			metric.Cases = 0
		}
		if metric.Cases < temporalStructureCertificationMinimumSliceCases {
			metric.FailureCodes = append(metric.FailureCodes, "insufficient_slice_cases")
		}
		if metric.Assessors != len(report.AssessorIDs) || len(bySlice[slice]) != len(report.AssessorIDs) {
			metric.FailureCodes = append(metric.FailureCodes, "missing_slice_assessor")
		}
		metric.FailureCodes = sortedUniqueStrings(metric.FailureCodes)
		metric.Passed = len(report.FailureCodes) == 0 && len(metric.FailureCodes) == 0
		if metric.Passed {
			report.CertifiedSlices = append(report.CertifiedSlices, slice)
		}
		report.Slices = append(report.Slices, metric)
	}
	if len(report.CertifiedSlices) == len(temporalStructureCertificationRequiredSlices) {
		report.CertificationStatus = TemporalStructureCertificationPassed
		report.NextAction = "run_locked_shadow_comparison"
	} else {
		report.CertificationStatus = TemporalStructureCertificationFailed
		report.NextAction = "diagnose_failed_source_and_signal_slices"
	}
	return report
}

func temporalStructureSummaryFailures(summary TemporalStructureAssessorSummary) []string {
	var failures []string
	if summary.OperationalFailures != 0 {
		failures = append(failures, "operational_failure")
	}
	if summary.ExactUnitCorrect != summary.Cases {
		failures = append(failures, "unit_error")
	}
	if summary.CoverageComplete != summary.Cases {
		failures = append(failures, "timeline_gap")
	}
	if summary.UnderSplits != 0 {
		failures = append(failures, "under_split")
	}
	if summary.OverSplits != 0 {
		failures = append(failures, "over_split")
	}
	if summary.ExactSegmentPlans != summary.Cases {
		failures = append(failures, "segment_plan_error")
	}
	if summary.SegmentRoleCorrect != summary.SegmentRoleTargets {
		failures = append(failures, "segment_role_error")
	}
	if summary.Boundary.ComparableTargets != summary.Boundary.TruthTargets || summary.Boundary.Within2000MS != summary.Boundary.TruthTargets {
		failures = append(failures, "boundary_miss")
	}
	return failures
}

func temporalStructureConstructionFailures(summary TemporalStructureConstructionSummary) []string {
	return temporalStructureSummaryFailures(TemporalStructureAssessorSummary{
		Cases: summary.Cases, OperationalFailures: summary.OperationalFailures,
		ExactUnitCorrect: summary.ExactUnitCorrect, ExactSegmentPlans: summary.ExactSegmentPlans,
		CoverageComplete: summary.CoverageComplete, UnderSplits: summary.UnderSplits, OverSplits: summary.OverSplits,
		SegmentRoleTargets: summary.SegmentRoleTargets, SegmentRoleCorrect: summary.SegmentRoleCorrect, Boundary: summary.Boundary,
	})
}

func sortedUniqueStrings(values []string) []string {
	sort.Strings(values)
	return slices.Compact(values)
}
