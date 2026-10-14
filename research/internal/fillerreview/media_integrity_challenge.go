package fillerreview

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	MediaIntegrityChallengeSchemaVersion   = 1
	MediaIntegrityChallengeContractVersion = "filler-media-integrity-challenge-v1"
	MediaIntegrityPolicyVersion            = "filler-media-quality-policy-v1"

	IntegrityNoVideo          = "no_video"
	IntegrityNoAudio          = "no_audio"
	IntegrityDecodeFailure    = "decode_failure"
	IntegrityNearTotalBlack   = "near_total_black"
	IntegrityNearTotalSilence = "near_total_silence"
	IntegrityStuckLowMotion   = "stuck_or_low_motion"
	IntegrityCleanAssessable  = "clean_assessable"

	PresentationScreenRecapture  = "screen_recapture"
	PresentationPlayerChrome     = "player_or_browser_chrome"
	PresentationRecordingOverlay = "timecode_or_recording_overlay"
	PresentationStockWatermark   = "third_party_stock_watermark"

	IntegrityExpectedReject   = "reject"
	IntegrityExpectedReview   = "review"
	IntegrityExpectedContinue = "continue"
	IntegrityExpectedHold     = "hold"

	LowMotionNotApplicable = "not_applicable"
	LowMotionDetected      = "detected"
	LowMotionMeasuredGap   = "measured_gap"
)

type MediaIntegrityChallengeAuthoring struct {
	SchemaVersion            int                                 `json:"schemaVersion"`
	ContractVersion          string                              `json:"contractVersion"`
	AuthoredAt               time.Time                           `json:"authoredAt"`
	MediaQualityReportSHA256 string                              `json:"mediaQualityReportSha256"`
	PolicyVersion            string                              `json:"policyVersion"`
	Cases                    []MediaIntegrityChallengeAuthorCase `json:"cases"`
}

type MediaIntegrityChallengeAuthorCase struct {
	CaseID               string   `json:"caseId"`
	EvidenceAlias        string   `json:"evidenceAlias"`
	IntegritySlice       string   `json:"integritySlice"`
	ExpectedOutcome      string   `json:"expectedOutcome"`
	PresentationDefects  []string `json:"presentationDefects"`
	LowMotionDisposition string   `json:"lowMotionDisposition"`
}

type MediaIntegrityChallengePackage struct {
	SchemaVersion            int                                 `json:"schemaVersion"`
	ContractVersion          string                              `json:"contractVersion"`
	PreparedAt               time.Time                           `json:"preparedAt"`
	MediaQualityReportSHA256 string                              `json:"mediaQualityReportSha256"`
	PolicyVersion            string                              `json:"policyVersion"`
	SeedSHA256               string                              `json:"seedSha256"`
	Cases                    []MediaIntegrityChallengePublicCase `json:"cases"`
}

type MediaIntegrityChallengePublicCase struct {
	Alias             string `json:"alias"`
	SourceMediaSHA256 string `json:"sourceMediaSha256"`
	MeasurementSHA256 string `json:"measurementSha256"`
}

type MediaIntegrityChallengeMap struct {
	SchemaVersion            int                               `json:"schemaVersion"`
	ContractVersion          string                            `json:"contractVersion"`
	PreparedAt               time.Time                         `json:"preparedAt"`
	MediaQualityReportSHA256 string                            `json:"mediaQualityReportSha256"`
	PolicyVersion            string                            `json:"policyVersion"`
	Seed                     string                            `json:"seed"`
	PackageSHA256            string                            `json:"packageSha256"`
	Entries                  []MediaIntegrityChallengeMapEntry `json:"entries"`
}

type MediaIntegrityChallengeMapEntry struct {
	Alias                string   `json:"alias"`
	CaseID               string   `json:"caseId"`
	EvidenceAlias        string   `json:"evidenceAlias"`
	IntegritySlice       string   `json:"integritySlice"`
	ExpectedOutcome      string   `json:"expectedOutcome"`
	PresentationDefects  []string `json:"presentationDefects"`
	LowMotionDisposition string   `json:"lowMotionDisposition"`
}

type MediaIntegrityChallengeBuildConfig struct {
	AuthoringPath    string
	MediaQualityPath string
	Seed             string
	PreparedAt       time.Time
	OutputDir        string
}

type MediaIntegrityChallengeBuildResult struct {
	Cases         int
	PackageSHA256 string
	MapSHA256     string
}

func BuildMediaIntegrityChallenge(config MediaIntegrityChallengeBuildConfig) (MediaIntegrityChallengeBuildResult, error) {
	if strings.TrimSpace(config.AuthoringPath) == "" || strings.TrimSpace(config.MediaQualityPath) == "" || strings.TrimSpace(config.Seed) == "" || config.PreparedAt.IsZero() || strings.TrimSpace(config.OutputDir) == "" {
		return MediaIntegrityChallengeBuildResult{}, fmt.Errorf("media integrity challenge requires authoring, media quality, secret seed, prepared time, and output")
	}
	authoring, err := readStrictJSON[MediaIntegrityChallengeAuthoring](config.AuthoringPath)
	if err != nil {
		return MediaIntegrityChallengeBuildResult{}, fmt.Errorf("read media integrity authority: %w", err)
	}
	report, reportRaw, reportSHA, err := loadMediaIntegrityQualityReport(config.MediaQualityPath)
	if err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	if err := validateMediaIntegrityAuthoring(authoring, report, reportSHA, config.PreparedAt); err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	measurements := make(map[string]TemporalMediaQualityCase, len(report.CaseMeasurements))
	for _, item := range report.CaseMeasurements {
		measurements[item.EvidenceAlias] = item
	}
	pack := MediaIntegrityChallengePackage{SchemaVersion: MediaIntegrityChallengeSchemaVersion, ContractVersion: MediaIntegrityChallengeContractVersion, PreparedAt: config.PreparedAt.UTC(), MediaQualityReportSHA256: reportSHA, PolicyVersion: MediaIntegrityPolicyVersion, SeedSHA256: hashBytes([]byte(config.Seed))}
	mapping := MediaIntegrityChallengeMap{SchemaVersion: MediaIntegrityChallengeSchemaVersion, ContractVersion: MediaIntegrityChallengeContractVersion, PreparedAt: config.PreparedAt.UTC(), MediaQualityReportSHA256: reportSHA, PolicyVersion: MediaIntegrityPolicyVersion, Seed: config.Seed}
	for _, item := range authoring.Cases {
		measurement := measurements[item.EvidenceAlias]
		measurementRaw, marshalErr := json.Marshal(measurement)
		if marshalErr != nil {
			return MediaIntegrityChallengeBuildResult{}, marshalErr
		}
		alias := mediaIntegrityAlias(config.Seed, item.CaseID, item.EvidenceAlias)
		pack.Cases = append(pack.Cases, MediaIntegrityChallengePublicCase{Alias: alias, SourceMediaSHA256: measurement.SourceMediaSHA256, MeasurementSHA256: hashBytes(measurementRaw)})
		mapping.Entries = append(mapping.Entries, MediaIntegrityChallengeMapEntry{Alias: alias, CaseID: item.CaseID, EvidenceAlias: item.EvidenceAlias, IntegritySlice: item.IntegritySlice, ExpectedOutcome: item.ExpectedOutcome, PresentationDefects: append([]string(nil), item.PresentationDefects...), LowMotionDisposition: item.LowMotionDisposition})
	}
	sort.Slice(pack.Cases, func(i, j int) bool { return pack.Cases[i].Alias < pack.Cases[j].Alias })
	sort.Slice(mapping.Entries, func(i, j int) bool { return mapping.Entries[i].Alias < mapping.Entries[j].Alias })
	packageRaw, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	packageRaw = append(packageRaw, '\n')
	if err := auditMediaIntegrityPublic(packageRaw, authoring, config.Seed); err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	mapping.PackageSHA256 = hashBytes(packageRaw)
	mapRaw, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	mapRaw = append(mapRaw, '\n')
	stage, err := beginTemporalHumanReviewStage(config.OutputDir)
	if err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	defer stage.Cleanup()
	if err := os.MkdirAll(filepath.Join(stage.path, "public"), 0o750); err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(stage.path, "private"), 0o700); err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	if err := writeTemporalTruthNew(filepath.Join(stage.path, "public", "manifest.json"), packageRaw, 0o640); err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	if err := writeTemporalTruthNew(filepath.Join(stage.path, "private", "map.json"), mapRaw, 0o600); err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	if err := writeTemporalTruthNew(filepath.Join(stage.path, "private", "media-quality.json"), reportRaw, 0o600); err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	if err := stage.Publish(); err != nil {
		return MediaIntegrityChallengeBuildResult{}, err
	}
	return MediaIntegrityChallengeBuildResult{Cases: len(pack.Cases), PackageSHA256: mapping.PackageSHA256, MapSHA256: hashBytes(mapRaw)}, nil
}

func loadMediaIntegrityQualityReport(path string) (TemporalMediaQualityReport, []byte, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return TemporalMediaQualityReport{}, nil, "", err
	}
	report, err := readStrictJSON[TemporalMediaQualityReport](path)
	if err != nil {
		return TemporalMediaQualityReport{}, nil, "", err
	}
	if report.SchemaVersion != TemporalMediaQualitySchemaVersion || report.ContractVersion != TemporalMediaQualityContractVersion || report.PolicyVersion != MediaIntegrityPolicyVersion || report.ProductionAdmissionAllowed || len(report.CaseMeasurements) == 0 || report.Cases != len(report.CaseMeasurements) || !reviewSHA256(report.MediaTools.FFmpeg.BinarySHA256) || !reviewSHA256(report.MediaTools.FFprobe.BinarySHA256) {
		return TemporalMediaQualityReport{}, nil, "", fmt.Errorf("media integrity challenge requires the exact complete non-authorizing full-decode report")
	}
	seen := map[string]struct{}{}
	for _, item := range report.CaseMeasurements {
		if strings.TrimSpace(item.EvidenceAlias) == "" || !reviewSHA256(item.SourceMediaSHA256) {
			return TemporalMediaQualityReport{}, nil, "", fmt.Errorf("media quality case has incomplete source identity")
		}
		if _, duplicate := seen[item.EvidenceAlias]; duplicate {
			return TemporalMediaQualityReport{}, nil, "", fmt.Errorf("media quality report repeats evidence alias")
		}
		seen[item.EvidenceAlias] = struct{}{}
	}
	return report, raw, hashBytes(raw), nil
}

func validateMediaIntegrityAuthoring(authoring MediaIntegrityChallengeAuthoring, report TemporalMediaQualityReport, reportSHA string, preparedAt time.Time) error {
	if authoring.SchemaVersion != MediaIntegrityChallengeSchemaVersion || authoring.ContractVersion != MediaIntegrityChallengeContractVersion || authoring.PolicyVersion != MediaIntegrityPolicyVersion || authoring.MediaQualityReportSHA256 != reportSHA || authoring.AuthoredAt.IsZero() || preparedAt.Before(authoring.AuthoredAt) || len(authoring.Cases) == 0 || len(authoring.Cases) > len(report.CaseMeasurements) {
		return fmt.Errorf("media integrity authority does not bind the exact report and policy")
	}
	reportAliases := make(map[string]struct{}, len(report.CaseMeasurements))
	for _, item := range report.CaseMeasurements {
		reportAliases[item.EvidenceAlias] = struct{}{}
	}
	seenCases, seenAliases := map[string]struct{}{}, map[string]struct{}{}
	for _, item := range authoring.Cases {
		if strings.TrimSpace(item.CaseID) == "" || !validIntegritySlice(item.IntegritySlice) || !validIntegrityOutcome(item.ExpectedOutcome) || !validLowMotionDisposition(item.IntegritySlice, item.LowMotionDisposition) {
			return fmt.Errorf("media integrity authority contains invalid closed fields")
		}
		if _, exists := reportAliases[item.EvidenceAlias]; !exists {
			return fmt.Errorf("media integrity authority references an unknown measurement")
		}
		if _, duplicate := seenCases[item.CaseID]; duplicate {
			return fmt.Errorf("media integrity authority repeats a case id")
		}
		if _, duplicate := seenAliases[item.EvidenceAlias]; duplicate {
			return fmt.Errorf("media integrity authority repeats an evidence alias")
		}
		seenCases[item.CaseID], seenAliases[item.EvidenceAlias] = struct{}{}, struct{}{}
		seenDefects := map[string]struct{}{}
		for _, defect := range item.PresentationDefects {
			if !validPresentationDefect(defect) {
				return fmt.Errorf("media integrity authority contains unknown presentation defect")
			}
			if _, duplicate := seenDefects[defect]; duplicate {
				return fmt.Errorf("media integrity authority repeats a presentation defect")
			}
			seenDefects[defect] = struct{}{}
		}
	}
	return nil
}

func validIntegritySlice(value string) bool {
	switch value {
	case IntegrityNoVideo, IntegrityNoAudio, IntegrityDecodeFailure, IntegrityNearTotalBlack, IntegrityNearTotalSilence, IntegrityStuckLowMotion, IntegrityCleanAssessable:
		return true
	default:
		return false
	}
}

func validIntegrityOutcome(value string) bool {
	return value == IntegrityExpectedReject || value == IntegrityExpectedReview || value == IntegrityExpectedContinue || value == IntegrityExpectedHold
}

func validPresentationDefect(value string) bool {
	return value == PresentationScreenRecapture || value == PresentationPlayerChrome || value == PresentationRecordingOverlay || value == PresentationStockWatermark
}

func validLowMotionDisposition(slice, value string) bool {
	if slice == IntegrityStuckLowMotion {
		return value == LowMotionDetected || value == LowMotionMeasuredGap
	}
	return value == LowMotionNotApplicable
}

func mediaIntegrityAlias(seed, caseID, evidenceAlias string) string {
	mac := hmac.New(sha256.New, []byte(seed))
	_, _ = mac.Write([]byte(caseID + "\x00" + evidenceAlias))
	return "mi-" + hex.EncodeToString(mac.Sum(nil)[:16])
}

func auditMediaIntegrityPublic(raw []byte, authoring MediaIntegrityChallengeAuthoring, seed string) error {
	private := []string{seed}
	for _, item := range authoring.Cases {
		private = append(private, item.CaseID, item.EvidenceAlias, item.IntegritySlice, item.ExpectedOutcome, item.LowMotionDisposition)
		private = append(private, item.PresentationDefects...)
	}
	text := string(raw)
	for _, value := range private {
		if strings.TrimSpace(value) != "" && strings.Contains(text, value) {
			return fmt.Errorf("media integrity public package leaks private authority")
		}
	}
	return nil
}

type MediaIntegrityChallengeScoreConfig struct {
	PackagePath      string
	MapPath          string
	MediaQualityPath string
	LockedAt         time.Time
	OutputPath       string
}

type MediaIntegrityChallengeReport struct {
	SchemaVersion              int                                 `json:"schemaVersion"`
	ContractVersion            string                              `json:"contractVersion"`
	LockedAt                   time.Time                           `json:"lockedAt"`
	PackageSHA256              string                              `json:"packageSha256"`
	MapSHA256                  string                              `json:"mapSha256"`
	MediaQualityReportSHA256   string                              `json:"mediaQualityReportSha256"`
	PolicyVersion              string                              `json:"policyVersion"`
	Cases                      int                                 `json:"cases"`
	Correct                    int                                 `json:"correct"`
	OperationalHolds           int                                 `json:"operationalHolds"`
	MeasuredGaps               int                                 `json:"measuredGaps"`
	PresentationObservations   map[string]int                      `json:"presentationObservations"`
	Slices                     []MediaIntegritySliceMetric         `json:"slices"`
	CaseResults                []MediaIntegrityChallengeCaseResult `json:"caseResults"`
	ProductionAdmissionAllowed bool                                `json:"productionAdmissionAllowed"`
}

type MediaIntegritySliceMetric struct {
	Slice         string         `json:"slice"`
	Cases         int            `json:"cases"`
	Correct       int            `json:"correct"`
	Accuracy      float64        `json:"accuracy"`
	AccuracyLower float64        `json:"accuracyLower"`
	Confusion     map[string]int `json:"confusion"`
}

type MediaIntegrityChallengeCaseResult struct {
	Alias                string   `json:"alias"`
	IntegritySlice       string   `json:"integritySlice"`
	ExpectedOutcome      string   `json:"expectedOutcome"`
	ObservedOutcome      string   `json:"observedOutcome"`
	Correct              bool     `json:"correct"`
	PresentationDefects  []string `json:"presentationDefects,omitempty"`
	LowMotionDisposition string   `json:"lowMotionDisposition"`
}

func ScoreMediaIntegrityChallenge(config MediaIntegrityChallengeScoreConfig) (MediaIntegrityChallengeReport, string, error) {
	if strings.TrimSpace(config.PackagePath) == "" || strings.TrimSpace(config.MapPath) == "" || strings.TrimSpace(config.MediaQualityPath) == "" || config.LockedAt.IsZero() || strings.TrimSpace(config.OutputPath) == "" {
		return MediaIntegrityChallengeReport{}, "", fmt.Errorf("media integrity score requires package, private map, quality report, lock time, and output")
	}
	pack, err := readStrictJSON[MediaIntegrityChallengePackage](config.PackagePath)
	if err != nil {
		return MediaIntegrityChallengeReport{}, "", err
	}
	mapping, err := readStrictJSON[MediaIntegrityChallengeMap](config.MapPath)
	if err != nil {
		return MediaIntegrityChallengeReport{}, "", err
	}
	quality, _, qualitySHA, err := loadMediaIntegrityQualityReport(config.MediaQualityPath)
	if err != nil {
		return MediaIntegrityChallengeReport{}, "", err
	}
	packageRaw, err := os.ReadFile(config.PackagePath)
	if err != nil {
		return MediaIntegrityChallengeReport{}, "", err
	}
	mapRaw, err := os.ReadFile(config.MapPath)
	if err != nil {
		return MediaIntegrityChallengeReport{}, "", err
	}
	if err := validateMediaIntegrityJoin(pack, mapping, hashBytes(packageRaw), quality, qualitySHA, config.LockedAt); err != nil {
		return MediaIntegrityChallengeReport{}, "", err
	}
	measurements := map[string]TemporalMediaQualityCase{}
	for _, item := range quality.CaseMeasurements {
		measurements[item.EvidenceAlias] = item
	}
	report := MediaIntegrityChallengeReport{SchemaVersion: MediaIntegrityChallengeSchemaVersion, ContractVersion: MediaIntegrityChallengeContractVersion, LockedAt: config.LockedAt.UTC(), PackageSHA256: hashBytes(packageRaw), MapSHA256: hashBytes(mapRaw), MediaQualityReportSHA256: qualitySHA, PolicyVersion: MediaIntegrityPolicyVersion, Cases: len(pack.Cases), PresentationObservations: map[string]int{}, ProductionAdmissionAllowed: false}
	type count struct {
		cases, correct int
		confusion      map[string]int
	}
	counts := map[string]*count{}
	for _, entry := range mapping.Entries {
		measurement := measurements[entry.EvidenceAlias]
		observed := measurement.PolicyVerdict
		if measurement.OperationalFailure != "" || observed == "" {
			observed = IntegrityExpectedHold
			report.OperationalHolds++
		}
		correct := observed == entry.ExpectedOutcome
		if correct {
			report.Correct++
		}
		if entry.LowMotionDisposition == LowMotionMeasuredGap {
			if observed != IntegrityExpectedContinue {
				return MediaIntegrityChallengeReport{}, "", fmt.Errorf("measured low-motion gap was not continued by the measured policy")
			}
			report.MeasuredGaps++
		}
		for _, defect := range entry.PresentationDefects {
			report.PresentationObservations[defect]++
		}
		bucket := counts[entry.IntegritySlice]
		if bucket == nil {
			bucket = &count{confusion: map[string]int{}}
			counts[entry.IntegritySlice] = bucket
		}
		bucket.cases++
		if correct {
			bucket.correct++
		}
		bucket.confusion[entry.ExpectedOutcome+"->"+observed]++
		report.CaseResults = append(report.CaseResults, MediaIntegrityChallengeCaseResult{Alias: entry.Alias, IntegritySlice: entry.IntegritySlice, ExpectedOutcome: entry.ExpectedOutcome, ObservedOutcome: observed, Correct: correct, PresentationDefects: append([]string(nil), entry.PresentationDefects...), LowMotionDisposition: entry.LowMotionDisposition})
	}
	for slice, bucket := range counts {
		report.Slices = append(report.Slices, MediaIntegritySliceMetric{Slice: slice, Cases: bucket.cases, Correct: bucket.correct, Accuracy: float64(bucket.correct) / float64(bucket.cases), AccuracyLower: mediaIntegrityWilsonLower(bucket.correct, bucket.cases), Confusion: bucket.confusion})
	}
	sort.Slice(report.Slices, func(i, j int) bool { return report.Slices[i].Slice < report.Slices[j].Slice })
	sort.Slice(report.CaseResults, func(i, j int) bool { return report.CaseResults[i].Alias < report.CaseResults[j].Alias })
	outputRaw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return MediaIntegrityChallengeReport{}, "", err
	}
	outputRaw = append(outputRaw, '\n')
	if err := writeTemporalTruthNew(config.OutputPath, outputRaw, 0o600); err != nil {
		return MediaIntegrityChallengeReport{}, "", err
	}
	return report, hashBytes(outputRaw), nil
}

func validateMediaIntegrityJoin(pack MediaIntegrityChallengePackage, mapping MediaIntegrityChallengeMap, packageSHA string, quality TemporalMediaQualityReport, qualitySHA string, lockedAt time.Time) error {
	if pack.SchemaVersion != MediaIntegrityChallengeSchemaVersion || pack.ContractVersion != MediaIntegrityChallengeContractVersion || mapping.SchemaVersion != pack.SchemaVersion || mapping.ContractVersion != pack.ContractVersion || mapping.PackageSHA256 != packageSHA || pack.MediaQualityReportSHA256 != qualitySHA || mapping.MediaQualityReportSHA256 != qualitySHA || pack.PolicyVersion != MediaIntegrityPolicyVersion || mapping.PolicyVersion != pack.PolicyVersion || pack.PreparedAt != mapping.PreparedAt || lockedAt.Before(pack.PreparedAt) || hashBytes([]byte(mapping.Seed)) != pack.SeedSHA256 || len(pack.Cases) == 0 || len(pack.Cases) != len(mapping.Entries) || len(pack.Cases) > len(quality.CaseMeasurements) {
		return fmt.Errorf("media integrity package, map, report, and policy identities do not match")
	}
	publicByAlias := map[string]MediaIntegrityChallengePublicCase{}
	qualityByAlias := map[string]TemporalMediaQualityCase{}
	for _, item := range quality.CaseMeasurements {
		qualityByAlias[item.EvidenceAlias] = item
	}
	for _, item := range pack.Cases {
		if _, duplicate := publicByAlias[item.Alias]; duplicate || !reviewSHA256(item.SourceMediaSHA256) || !reviewSHA256(item.MeasurementSHA256) {
			return fmt.Errorf("media integrity public package contains invalid or duplicate cases")
		}
		publicByAlias[item.Alias] = item
	}
	seenEvidence := map[string]struct{}{}
	for _, entry := range mapping.Entries {
		public, exists := publicByAlias[entry.Alias]
		measurement, measured := qualityByAlias[entry.EvidenceAlias]
		measurementRaw, _ := json.Marshal(measurement)
		if !exists || !measured || public.SourceMediaSHA256 != measurement.SourceMediaSHA256 || public.MeasurementSHA256 != hashBytes(measurementRaw) || mediaIntegrityAlias(mapping.Seed, entry.CaseID, entry.EvidenceAlias) != entry.Alias || !validIntegritySlice(entry.IntegritySlice) || !validIntegrityOutcome(entry.ExpectedOutcome) || !validLowMotionDisposition(entry.IntegritySlice, entry.LowMotionDisposition) {
			return fmt.Errorf("media integrity private authority does not match public measurements")
		}
		if _, duplicate := seenEvidence[entry.EvidenceAlias]; duplicate {
			return fmt.Errorf("media integrity private map repeats evidence")
		}
		seenEvidence[entry.EvidenceAlias] = struct{}{}
		for _, defect := range entry.PresentationDefects {
			if !validPresentationDefect(defect) {
				return fmt.Errorf("unknown presentation defect")
			}
		}
	}
	return nil
}

func mediaIntegrityWilsonLower(successes, trials int) float64 {
	if trials == 0 || successes == 0 {
		return 0
	}
	z := 1.6448536269514722
	n := float64(trials)
	p := float64(successes) / n
	denominator := 1 + z*z/n
	centre := p + z*z/(2*n)
	margin := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n))
	return (centre - margin) / denominator
}
