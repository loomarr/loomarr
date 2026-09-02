package fillerreview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestMediaIntegrityChallengeBuildsLabelFreePackageAndScoresSeparateAxes(t *testing.T) {
	root := t.TempDir()
	qualityPath := filepath.Join(root, "quality.json")
	authorityPath := filepath.Join(root, "authority.json")
	outputDir := filepath.Join(root, "challenge")
	preparedAt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	report := mediaIntegrityFixtureReport(preparedAt.Add(-time.Hour))
	qualityRaw := writeIntegrityJSON(t, qualityPath, report)
	authority := MediaIntegrityChallengeAuthoring{
		SchemaVersion: MediaIntegrityChallengeSchemaVersion, ContractVersion: MediaIntegrityChallengeContractVersion,
		AuthoredAt: preparedAt.Add(-time.Minute), MediaQualityReportSHA256: hashBytes(qualityRaw), PolicyVersion: MediaIntegrityPolicyVersion,
		Cases: []MediaIntegrityChallengeAuthorCase{
			{CaseID: "private/no-video", EvidenceAlias: "evidence-no-video", IntegritySlice: IntegrityNoVideo, ExpectedOutcome: IntegrityExpectedReject, LowMotionDisposition: LowMotionNotApplicable},
			{CaseID: "private/decode", EvidenceAlias: "evidence-decode", IntegritySlice: IntegrityDecodeFailure, ExpectedOutcome: IntegrityExpectedHold, LowMotionDisposition: LowMotionNotApplicable},
			{CaseID: "private/presentation", EvidenceAlias: "evidence-presentation", IntegritySlice: IntegrityCleanAssessable, ExpectedOutcome: IntegrityExpectedContinue, PresentationDefects: []string{PresentationScreenRecapture}, LowMotionDisposition: LowMotionNotApplicable},
			{CaseID: "private/static", EvidenceAlias: "evidence-static", IntegritySlice: IntegrityStuckLowMotion, ExpectedOutcome: IntegrityExpectedReview, LowMotionDisposition: LowMotionMeasuredGap},
		},
	}
	writeIntegrityJSON(t, authorityPath, authority)
	result, err := BuildMediaIntegrityChallenge(MediaIntegrityChallengeBuildConfig{AuthoringPath: authorityPath, MediaQualityPath: qualityPath, Seed: "private-seed-value", PreparedAt: preparedAt, OutputDir: outputDir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cases != 4 || !reviewSHA256(result.PackageSHA256) || !reviewSHA256(result.MapSHA256) {
		t.Fatalf("build result = %+v", result)
	}
	publicRaw, err := os.ReadFile(filepath.Join(outputDir, "public", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private/no-video", "evidence-no-video", IntegrityNoVideo, IntegrityExpectedReject, "private-seed-value"} {
		if strings.Contains(string(publicRaw), secret) {
			t.Fatalf("public package leaked %q", secret)
		}
	}
	scorePath := filepath.Join(root, "score.json")
	score, digest, err := ScoreMediaIntegrityChallenge(MediaIntegrityChallengeScoreConfig{PackagePath: filepath.Join(outputDir, "public", "manifest.json"), MapPath: filepath.Join(outputDir, "private", "map.json"), MediaQualityPath: qualityPath, LockedAt: preparedAt.Add(time.Hour), OutputPath: scorePath})
	if err != nil {
		t.Fatal(err)
	}
	if score.Cases != 4 || score.Correct != 3 || score.OperationalHolds != 1 || score.MeasuredGaps != 1 || score.PresentationObservations[PresentationScreenRecapture] != 1 || score.ProductionAdmissionAllowed || !reviewSHA256(digest) {
		t.Fatalf("score = %+v", score)
	}
	for _, metric := range score.Slices {
		if metric.Cases != 1 || metric.AccuracyLower < 0 || metric.AccuracyLower > metric.Accuracy {
			t.Fatalf("slice metric = %+v", metric)
		}
	}
}

func TestMediaIntegrityChallengeFailsClosedOnVocabularyAndIdentityDrift(t *testing.T) {
	preparedAt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	report := mediaIntegrityFixtureReport(preparedAt.Add(-time.Hour))
	authority := MediaIntegrityChallengeAuthoring{SchemaVersion: MediaIntegrityChallengeSchemaVersion, ContractVersion: MediaIntegrityChallengeContractVersion, AuthoredAt: preparedAt.Add(-time.Minute), PolicyVersion: MediaIntegrityPolicyVersion}
	reportRaw, _ := json.Marshal(report)
	authority.MediaQualityReportSHA256 = hashBytes(append(reportRaw, '\n'))
	authority.Cases = []MediaIntegrityChallengeAuthorCase{{CaseID: "one", EvidenceAlias: report.CaseMeasurements[0].EvidenceAlias, IntegritySlice: PresentationScreenRecapture, ExpectedOutcome: IntegrityExpectedReject, LowMotionDisposition: LowMotionNotApplicable}}
	if err := validateMediaIntegrityAuthoring(authority, report, authority.MediaQualityReportSHA256, preparedAt); err == nil {
		t.Fatal("presentation vocabulary was accepted as an integrity slice")
	}
	pack := MediaIntegrityChallengePackage{SchemaVersion: MediaIntegrityChallengeSchemaVersion, ContractVersion: MediaIntegrityChallengeContractVersion, PreparedAt: preparedAt, MediaQualityReportSHA256: authority.MediaQualityReportSHA256, PolicyVersion: MediaIntegrityPolicyVersion, SeedSHA256: hashBytes([]byte("seed")), Cases: []MediaIntegrityChallengePublicCase{{Alias: "mi-one", SourceMediaSHA256: strings.Repeat("f", 64), MeasurementSHA256: strings.Repeat("a", 64)}}}
	mapping := MediaIntegrityChallengeMap{SchemaVersion: pack.SchemaVersion, ContractVersion: pack.ContractVersion, PreparedAt: pack.PreparedAt, MediaQualityReportSHA256: pack.MediaQualityReportSHA256, PolicyVersion: pack.PolicyVersion, Seed: "seed", PackageSHA256: "different", Entries: []MediaIntegrityChallengeMapEntry{{Alias: "mi-one", CaseID: "one", EvidenceAlias: report.CaseMeasurements[0].EvidenceAlias, IntegritySlice: IntegrityNoVideo, ExpectedOutcome: IntegrityExpectedReject, LowMotionDisposition: LowMotionNotApplicable}}}
	if err := validateMediaIntegrityJoin(pack, mapping, "package", report, authority.MediaQualityReportSHA256, preparedAt.Add(time.Hour)); err == nil {
		t.Fatal("package/map identity drift was accepted")
	}
}

func mediaIntegrityFixtureReport(measuredAt time.Time) TemporalMediaQualityReport {
	tool := TemporalTruthToolIdentity{Path: "/usr/bin/fixture", Version: "fixture-v1", BinarySHA256: strings.Repeat("a", 64)}
	return TemporalMediaQualityReport{
		SchemaVersion: TemporalMediaQualitySchemaVersion, ContractVersion: TemporalMediaQualityContractVersion, MeasuredAt: measuredAt,
		PolicyVersion: MediaIntegrityPolicyVersion,
		MediaTools:    TemporalTruthMediaIdentity{FFmpeg: tool, FFprobe: tool}, Cases: 4, ProductionAdmissionAllowed: false,
		CaseMeasurements: []TemporalMediaQualityCase{
			{EvidenceAlias: "evidence-no-video", SourceMediaSHA256: strings.Repeat("b", 64), DurationMS: 10_000, PolicyVerdict: mediaQualityReject, PolicyReason: filler.ReasonNoVideo},
			{EvidenceAlias: "evidence-decode", SourceMediaSHA256: strings.Repeat("c", 64), DurationMS: 10_000, OperationalFailure: "synthetic decode failure"},
			{EvidenceAlias: "evidence-presentation", SourceMediaSHA256: strings.Repeat("d", 64), DurationMS: 10_000, PolicyVerdict: mediaQualityContinue},
			{EvidenceAlias: "evidence-static", SourceMediaSHA256: strings.Repeat("e", 64), DurationMS: 10_000, PolicyVerdict: mediaQualityContinue},
		},
	}
}

func writeIntegrityJSON(t *testing.T, path string, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return raw
}
