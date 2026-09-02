package fillerreview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestBuildTemporalStructureHoldoutPlanBindsAuthoritiesAndBuildsBalancedConstructions(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	firstConfig := fixture.config(first)
	result, err := BuildTemporalStructureHoldoutPlan(firstConfig)
	if err != nil {
		t.Fatal(err)
	}
	if result.Cases != TemporalStructureHoldoutCases || !reviewSHA256(result.AuthoringSHA256) || !reviewSHA256(result.ReceiptSHA256) {
		t.Fatalf("result = %+v", result)
	}
	secondConfig := fixture.config(second)
	repeat, err := BuildTemporalStructureHoldoutPlan(secondConfig)
	if err != nil {
		t.Fatal(err)
	}
	if repeat.AuthoringSHA256 != result.AuthoringSHA256 || repeat.ReceiptSHA256 != result.ReceiptSHA256 {
		t.Fatalf("holdout plan is not reproducible: first=%+v repeat=%+v", result, repeat)
	}
	authoring := readStrictTestJSON[TemporalStructureChallengeAuthoring](t, filepath.Join(first, "authoring.json"))
	receipt := readStrictTestJSON[TemporalStructureHoldoutReceipt](t, filepath.Join(first, "receipt.json"))
	unitCounts := map[fillereval.UnitKind]int{}
	for _, item := range authoring.Cases {
		unitCounts[item.Unit]++
	}
	if unitCounts[fillereval.UnitStandalone] != 12 || unitCounts[fillereval.UnitCompilation] != 12 || unitCounts[fillereval.UnitProgrammeExcerpt] != 12 || len(authoring.Sources) != 18 || receipt.StandaloneRoleCounts[fillereval.TemporalRoleBumper] != 2 || receipt.StandaloneRoleCounts[fillereval.TemporalRoleCommercial] != 3 || receipt.StandaloneRoleCounts[fillereval.TemporalRolePromo] != 2 || receipt.StandaloneRoleCounts[fillereval.TemporalRolePSA] != 2 || receipt.StandaloneRoleCounts[fillereval.TemporalRoleTrailer] != 3 || receipt.TrainingAllowed || receipt.ProductionAdmissionAllowed {
		t.Fatalf("authoring counts=%v sources=%d receipt=%+v", unitCounts, len(authoring.Sources), receipt)
	}
	bands := map[string]int{}
	usedByBand := map[string]map[string]struct{}{"early": {}, "middle": {}, "late": {}}
	for _, item := range receipt.CompilationConstructions {
		bands[item.JoinBand]++
		for _, sourceID := range []string{item.FirstSourceID, item.SecondSourceID} {
			if _, duplicate := usedByBand[item.JoinBand][sourceID]; duplicate {
				t.Fatalf("join band %q reused source %q", item.JoinBand, sourceID)
			}
			usedByBand[item.JoinBand][sourceID] = struct{}{}
		}
	}
	patterns := map[string]int{}
	for _, item := range receipt.ProgrammeConstructions {
		patterns[item.Pattern]++
		if item.StartMS < 5_000 || item.StartMS+item.DurationMS > item.ParentEndMS-5_000 {
			t.Fatalf("programme cut lacks parent margins: %+v", item)
		}
	}
	if bands["early"] != 4 || bands["middle"] != 4 || bands["late"] != 4 || patterns["near_parent_start"] != 6 || patterns["near_parent_end"] != 6 {
		t.Fatalf("bands=%v patterns=%v", bands, patterns)
	}
	if _, err := BuildTemporalStructureHoldoutPlan(firstConfig); err == nil {
		t.Fatal("immutable holdout output was overwritten")
	}
}

func TestBuildTemporalStructureHoldoutPlanRejectsProhibitedRoleCoverage(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	report := readStrictTestJSON[TemporalSuitabilityComparisonReport](t, fixture.suitability)
	report.CaseComparisons[0].Disposition = "prohibited_hold"
	report.CaseComparisons[0].UnionFlags = []SuitabilityFlag{SuitabilityExplicitNudity}
	report.CaseComparisons[0].CorroboratedFlags = []SuitabilityFlag{SuitabilityExplicitNudity}
	report.CorroboratedProhibitedCases = 1
	report.FlaggedUnionCases = 1
	report.CoverageHoldCases--
	fixture.suitability = writeTemporalHumanJSON(t, t.TempDir(), "suitability.json", report)
	audit := readStrictTestJSON[temporalStructureHoldoutFamilyAudit](t, fixture.family)
	audit.Fingerprints = audit.Fingerprints[1:]
	audit.Summary.Cases--
	fixture.family = writeTemporalHumanJSON(t, t.TempDir(), "family.json", audit)
	_, err := BuildTemporalStructureHoldoutPlan(fixture.config(filepath.Join(t.TempDir(), "output")))
	if err == nil || !strings.Contains(err.Error(), "insufficient eligible bumper") {
		t.Fatalf("prohibited coverage error = %v", err)
	}
}

func TestBuildTemporalStructureHoldoutPlanRejectsDuplicateFamilyCoverage(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	privateMap := readStrictTestJSON[TemporalTruthEvidencePrivateMap](t, fixture.privateMap)
	audit := readStrictTestJSON[temporalStructureHoldoutFamilyAudit](t, fixture.family)
	audit.Families = []temporalStructureHoldoutDuplicateFamily{{
		FamilyID: "same-bumper-family", Members: []string{privateMap.Entries[0].CaseID, privateMap.Entries[1].CaseID}, CompleteClique: true,
	}}
	audit.Summary.DuplicateFamilies = 1
	fixture.family = writeTemporalHumanJSON(t, t.TempDir(), "family.json", audit)
	_, err := BuildTemporalStructureHoldoutPlan(fixture.config(filepath.Join(t.TempDir(), "output")))
	if err == nil || !strings.Contains(err.Error(), "insufficient eligible bumper") {
		t.Fatalf("duplicate-family coverage error = %v", err)
	}
}

func TestBuildTemporalStructureHoldoutPlanRejectsMediaQualitySummaryDrift(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	report := readStrictTestJSON[TemporalMediaQualityReport](t, fixture.quality)
	report.PolicyContinueCases--
	fixture.quality = writeTemporalHumanJSON(t, t.TempDir(), "quality.json", report)
	_, err := BuildTemporalStructureHoldoutPlan(fixture.config(filepath.Join(t.TempDir(), "output")))
	if err == nil || !strings.Contains(err.Error(), "summary does not match") {
		t.Fatalf("media-quality summary error = %v", err)
	}
}

type temporalStructureHoldoutFixture struct {
	temporalHumanReviewFixture
	humanAssessment  string
	humanAttestation string
	quality          string
	suitability      string
	family           string
	inventory        string
	plannedAt        time.Time
}

func newTemporalStructureHoldoutFixture(t *testing.T) temporalStructureHoldoutFixture {
	t.Helper()
	base := newTemporalHumanReviewFixture(t)
	manifest := readStrictTestJSON[TemporalTruthEvidenceManifest](t, base.manifest)
	privateMap := readStrictTestJSON[TemporalTruthEvidencePrivateMap](t, base.privateMap)
	for index := range manifest.Cases {
		durationMS := int64(index+1) * 10_000
		raw := []byte("distinct bounded source " + manifest.Cases[index].Alias)
		name := "bounded-" + manifest.Cases[index].Alias + ".mp4"
		path := filepath.Join(filepath.Dir(base.manifest), name)
		if err := os.WriteFile(path, raw, 0o640); err != nil {
			t.Fatal(err)
		}
		manifest.Cases[index].DurationMS = durationMS
		manifest.Cases[index].Plan.SourceEndMS = durationMS
		manifest.Cases[index].Plan.EvidenceEndMS = durationMS
		manifest.Cases[index].Video.Path = name
		manifest.Cases[index].Video.SHA256 = hashBytes(raw)
		manifest.Cases[index].Video.Bytes = int64(len(raw))
		manifest.Cases[index].Video.DurationMS = durationMS
	}
	writeTemporalHumanJSON(t, filepath.Dir(base.manifest), filepath.Base(base.manifest), manifest)
	evidenceSHA, err := hashFile(base.manifest)
	if err != nil {
		t.Fatal(err)
	}
	roles := []fillereval.TemporalRole{
		fillereval.TemporalRoleBumper, fillereval.TemporalRoleBumper,
		fillereval.TemporalRoleCommercial, fillereval.TemporalRoleCommercial, fillereval.TemporalRoleCommercial,
		fillereval.TemporalRolePromo, fillereval.TemporalRolePromo,
		fillereval.TemporalRolePSA, fillereval.TemporalRolePSA,
		fillereval.TemporalRoleTrailer, fillereval.TemporalRoleTrailer, fillereval.TemporalRoleTrailer,
	}
	completedAt := base.preparedAt.Add(time.Hour)
	set := TemporalHumanAssessmentSet{
		SchemaVersion: TemporalHumanReviewSchemaVersion, ContractVersion: TemporalHumanReviewContractVersion,
		BatchID: "holdout-human", ReviewerID: "reviewer", EvidenceManifestSHA256: evidenceSHA,
		SelectionSHA256: manifest.SelectionSHA256, PackageSHA256: strings.Repeat("1", 64), SubmissionSHA256: strings.Repeat("2", 64),
		PreparedAt: base.preparedAt, CompletedAt: completedAt,
	}
	for index, item := range manifest.Cases {
		assessment := TemporalHumanReviewAssessment{EvidenceAlias: item.Alias, Unit: fillereval.UnitUnusable, DecisiveAtMS: 0}
		if index < len(roles) {
			role := roles[index]
			assessment.Unit, assessment.Role = fillereval.UnitStandalone, &role
		}
		set.Assessments = append(set.Assessments, assessment)
	}
	humanAssessment := writeTemporalHumanJSON(t, base.root, "human-assessment.json", set)
	humanSHA, err := hashFile(humanAssessment)
	if err != nil {
		t.Fatal(err)
	}
	attestation := TemporalHumanReviewAttestation{
		SchemaVersion: TemporalHumanReviewSchemaVersion, ContractVersion: TemporalHumanReviewContractVersion,
		BatchID: set.BatchID, ReviewerID: set.ReviewerID, LockedAt: completedAt.Add(time.Hour),
		PackageSHA256: set.PackageSHA256, MapSHA256: strings.Repeat("3", 64), SubmissionSHA256: set.SubmissionSHA256,
		AssessmentSetSHA256: humanSHA,
	}
	attestation.AttestationSHA256 = temporalHumanAttestationSHA256(attestation)
	humanAttestation := writeTemporalHumanJSON(t, base.root, "human-attestation.json", attestation)
	attestationFileSHA, err := hashFile(humanAttestation)
	if err != nil {
		t.Fatal(err)
	}
	tool := TemporalTruthToolIdentity{Path: "/tool", Version: "v1", BinarySHA256: strings.Repeat("a", 64)}
	quality := TemporalMediaQualityReport{
		SchemaVersion: TemporalMediaQualitySchemaVersion, ContractVersion: TemporalMediaQualityContractVersion,
		PolicyVersion: MediaIntegrityPolicyVersion, MeasuredAt: attestation.LockedAt.Add(time.Hour),
		HumanPackageSHA256: strings.Repeat("4", 64), HumanPrivateMapSHA256: strings.Repeat("5", 64),
		HumanAssessmentSetSHA256: humanSHA, HumanAttestationFileSHA256: attestationFileSHA,
		EvidenceManifestSHA256: evidenceSHA, SelectionSHA256: manifest.SelectionSHA256,
		MediaTools: TemporalTruthMediaIdentity{FFmpeg: tool, FFprobe: tool}, Cases: len(manifest.Cases),
		ProductionAdmissionAllowed: false,
	}
	for index, item := range manifest.Cases {
		measurement := TemporalMediaQualityCase{
			EvidenceAlias: item.Alias, SourceMediaSHA256: item.Video.SHA256, HumanUnit: set.Assessments[index].Unit,
			DurationMS: item.Video.DurationMS, HadAudio: true, PolicyVerdict: mediaQualityContinue,
		}
		quality.CaseMeasurements = append(quality.CaseMeasurements, measurement)
		accumulateTemporalMediaQuality(&quality, measurement)
	}
	qualityPath := writeTemporalHumanJSON(t, base.root, "quality.json", quality)
	suitabilityAliases := make([]string, 0, len(manifest.Cases))
	for _, item := range manifest.Cases {
		suitabilityAliases = append(suitabilityAliases, item.Alias)
	}
	sort.Strings(suitabilityAliases)
	suitability := TemporalSuitabilityComparisonReport{
		SchemaVersion: TemporalSuitabilityComparisonSchemaVersion, ContractVersion: TemporalSuitabilityComparisonContractVersion,
		ComparedAt: quality.MeasuredAt.Add(time.Hour), EvidenceManifestSHA256: evidenceSHA,
		SelectionSHA256: temporalTruthJSONSHA(suitabilityAliases), FirstResultSHA256: strings.Repeat("8", 64), SecondResultSHA256: strings.Repeat("9", 64),
		FirstAssessor: fillereval.TemporalAssessorIdentity{
			ID: "first", Provider: "openrouter", Model: "model-one", ModelFamily: "one",
			ModelDigest: "digest-one", PromptVersion: "prompt-v1",
		},
		SecondAssessor: fillereval.TemporalAssessorIdentity{
			ID: "second", Provider: "openrouter", Model: "model-two", ModelFamily: "two",
			ModelDigest: "digest-two", PromptVersion: "prompt-v1",
		},
		Cases: len(manifest.Cases), CoverageHoldCases: len(manifest.Cases), ProductionAdmissionAllowed: false,
	}
	for _, item := range manifest.Cases {
		suitability.CaseComparisons = append(suitability.CaseComparisons, TemporalSuitabilityCaseComparison{
			EvidenceAlias: item.Alias, FirstOutcome: string(SuitabilityOutcomeCoverageHold),
			SecondOutcome: string(SuitabilityOutcomeCoverageHold), Disposition: "coverage_hold",
		})
	}
	suitabilityPath := writeTemporalHumanJSON(t, base.root, "suitability.json", suitability)
	family := temporalStructureHoldoutFamilyAudit{
		SchemaVersion: temporalStructureHoldoutFamilyAuditSchemaVersion, Algorithm: "test-family-v1",
		GeneratedAt: suitability.ComparedAt.Add(time.Hour), SourceAudit: strings.Repeat("a", 64),
		Summary: temporalStructureHoldoutFamilySummary{Cases: len(privateMap.Entries)},
		Pairs:   []json.RawMessage{}, ClosestPairs: []json.RawMessage{}, Families: []temporalStructureHoldoutDuplicateFamily{},
	}
	for _, item := range privateMap.Entries {
		family.Fingerprints = append(family.Fingerprints, temporalStructureHoldoutFingerprint{
			CaseID: item.CaseID, ContentSHA256: item.ContentSHA256, LocalFile: item.SourceLocalFile,
			FrameHashes: []uint64{1}, AudioRMS: []uint32{1},
		})
	}
	familyPath := writeTemporalHumanJSON(t, base.root, "family.json", family)
	programmeRoot := filepath.Join(base.root, "programmes")
	if err := os.MkdirAll(programmeRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	inventory := TemporalStructureHoldoutProgrammeInventory{
		SchemaVersion: TemporalStructureHoldoutSchemaVersion, ContractVersion: TemporalStructureHoldoutProgrammeInventoryContract,
		GeneratedAt: family.GeneratedAt.Add(time.Hour),
	}
	for index := 0; index < temporalStructureHoldoutParentSources; index++ {
		raw := []byte(strings.Repeat(string(rune('a'+index)), index+1))
		path := filepath.Join(programmeRoot, "parent-"+string(rune('a'+index))+".mp4")
		if err := os.WriteFile(path, raw, 0o640); err != nil {
			t.Fatal(err)
		}
		inventory.Sources = append(inventory.Sources, TemporalStructureChallengeSource{
			ID: "programme-" + string(rune('a'+index)), Path: filepath.ToSlash(path[len(base.root)+1:]),
			SHA256: hashBytes(raw), DurationMS: 180_000 + int64(index)*10_000,
			Provenance: TemporalStructureSourceProvenance{
				Kind: TemporalStructureSourceProgrammeParent, Authority: "test-programme-authority",
				Reference: "test-programme-" + string(rune('a'+index)), MetadataSHA256: hashBytes([]byte("metadata-" + string(rune('a'+index)))),
				RetrievedAt: inventory.GeneratedAt.Add(-time.Hour),
			},
		})
	}
	inventoryPath := writeTemporalHumanJSON(t, base.root, "programme-inventory.json", inventory)
	return temporalStructureHoldoutFixture{
		temporalHumanReviewFixture: base, humanAssessment: humanAssessment, humanAttestation: humanAttestation,
		quality: qualityPath, suitability: suitabilityPath, family: familyPath, inventory: inventoryPath,
		plannedAt: inventory.GeneratedAt.Add(time.Hour),
	}
}

func (fixture temporalStructureHoldoutFixture) config(output string) TemporalStructureHoldoutConfig {
	return TemporalStructureHoldoutConfig{
		SelectionPath: fixture.selection, EvidenceManifestPath: fixture.manifest, EvidencePrivateMapPath: fixture.privateMap,
		HumanAssessmentPath: fixture.humanAssessment, HumanAttestationPath: fixture.humanAttestation,
		MediaQualityPath: fixture.quality, SuitabilityPath: fixture.suitability, FamilyAuditPath: fixture.family,
		ProgrammeInventoryPath: fixture.inventory, SourceRoot: fixture.root, Seed: "holdout-seed",
		PlannedAt: fixture.plannedAt, OutputDir: output,
	}
}
