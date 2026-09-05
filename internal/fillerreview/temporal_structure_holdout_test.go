package fillerreview

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
	"github.com/loomarr/loomarr/internal/fillerreference"
)

func TestTemporalStructureHoldoutPlanReproducesCompleteBlindedChallenge(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	planRoot := filepath.Join(t.TempDir(), "plan")
	if _, err := BuildTemporalStructureHoldoutPlan(fixture.config(planRoot)); err != nil {
		t.Fatal(err)
	}
	authoringPath := filepath.Join(planRoot, "authoring.json")
	authoring := readStrictTestJSON[TemporalStructureChallengeAuthoring](t, authoringPath)
	media := &fakeTemporalStructureMedia{durationByPath: make(map[string]int64, len(authoring.Sources))}
	for _, source := range authoring.Sources {
		media.durationByPath[filepath.Join(fixture.root, filepath.FromSlash(source.Path))] = source.DurationMS
	}
	build := func(output string) TemporalStructureChallengeResult {
		t.Helper()
		result, err := BuildTemporalStructureChallenge(context.Background(), TemporalStructureChallengeConfig{
			AuthoringPath: authoringPath,
			SourceRoot:    fixture.root,
			OutputDir:     output,
			ChallengeID:   "complete-holdout-reproduction",
			Seed:          "complete-holdout-blinding-seed",
			GeneratedAt:   fixture.plannedAt.Add(time.Hour),
			Media:         media,
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	firstRoot := filepath.Join(t.TempDir(), "first")
	secondRoot := filepath.Join(t.TempDir(), "second")
	first := build(firstRoot)
	second := build(secondRoot)
	if first.Cases != TemporalStructureHoldoutCases || first != second {
		t.Fatalf("complete holdout results differ: first=%+v second=%+v", first, second)
	}
	if !bytes.Equal(readTree(t, firstRoot), readTree(t, secondRoot)) {
		t.Fatal("complete 36-case plan did not reproduce byte-identical public and private trees")
	}
}

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
	sameRoleByBand := map[string]int{}
	usedByBand := map[string]map[string]struct{}{"early": {}, "middle": {}, "late": {}}
	for _, item := range receipt.CompilationConstructions {
		bands[item.JoinBand]++
		if item.Roles[0] == item.Roles[1] {
			sameRoleByBand[item.JoinBand]++
		}
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
	if bands["early"] != 4 || bands["middle"] != 4 || bands["late"] != 4 || sameRoleByBand["early"] != 2 || sameRoleByBand["middle"] != 2 || sameRoleByBand["late"] != 2 || patterns["near_parent_start"] != 6 || patterns["near_parent_end"] != 6 {
		t.Fatalf("bands=%v same-role=%v patterns=%v", bands, sameRoleByBand, patterns)
	}
	if _, err := BuildTemporalStructureHoldoutPlan(firstConfig); err == nil {
		t.Fatal("immutable holdout output was overwritten")
	}
}

func TestBuildTemporalStructureHoldoutPlanRejectsRepeatedProgrammeProvenanceParent(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	inventory := readStrictTestJSON[TemporalStructureHoldoutProgrammeInventory](t, fixture.inventory)
	inventory.Sources[1].Provenance.Authority = inventory.Sources[0].Provenance.Authority
	inventory.Sources[1].Provenance.Reference = inventory.Sources[0].Provenance.Reference
	fixture.inventory = writeTemporalHumanJSON(t, t.TempDir(), "programme-inventory.json", inventory)
	_, err := BuildTemporalStructureHoldoutPlan(fixture.config(filepath.Join(t.TempDir(), "output")))
	if err == nil || !strings.Contains(err.Error(), "repeats a provenance parent") {
		t.Fatalf("repeated programme provenance error = %v", err)
	}
}

func TestBuildTemporalStructureHoldoutPlanRejectsProgrammeParentDerivedFromFiller(t *testing.T) {
	tests := map[string]func(*testing.T, *temporalStructureHoldoutFixture, *TemporalStructureHoldoutProgrammeInventory){
		"same bytes": func(t *testing.T, fixture *temporalStructureHoldoutFixture, inventory *TemporalStructureHoldoutProgrammeInventory) {
			manifest := readStrictTestJSON[TemporalTruthEvidenceManifest](t, fixture.manifest)
			path, err := filepath.Rel(fixture.root, filepath.Join(filepath.Dir(fixture.manifest), manifest.Cases[0].Video.Path))
			if err != nil {
				t.Fatal(err)
			}
			inventory.Sources[0].Path = filepath.ToSlash(path)
			inventory.Sources[0].SHA256 = manifest.Cases[0].Video.SHA256
		},
		"same provenance": func(t *testing.T, fixture *temporalStructureHoldoutFixture, inventory *TemporalStructureHoldoutProgrammeInventory) {
			privateMap := readStrictTestJSON[TemporalTruthEvidencePrivateMap](t, fixture.privateMap)
			inventory.Sources[0].Provenance.Authority = "locked-temporal-human-review"
			inventory.Sources[0].Provenance.Reference = privateMap.Entries[0].CaseID
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newTemporalStructureHoldoutFixture(t)
			inventory := readStrictTestJSON[TemporalStructureHoldoutProgrammeInventory](t, fixture.inventory)
			mutate(t, &fixture, &inventory)
			fixture.inventory = writeTemporalHumanJSON(t, t.TempDir(), "programme-inventory.json", inventory)
			_, err := BuildTemporalStructureHoldoutPlan(fixture.config(filepath.Join(t.TempDir(), "output")))
			if err == nil || !strings.Contains(err.Error(), "repeats bounded filler") {
				t.Fatalf("programme parent derived from filler error = %v", err)
			}
		})
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
	_, err := BuildTemporalStructureHoldoutPlan(fixture.config(filepath.Join(t.TempDir(), "output")))
	if err == nil || !strings.Contains(err.Error(), "insufficient eligible bumper") {
		t.Fatalf("prohibited coverage error = %v", err)
	}
}

func TestBuildTemporalStructureHoldoutPlanRejectsDuplicateFamilyCoverage(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	privateMap := readStrictTestJSON[TemporalTruthEvidencePrivateMap](t, fixture.privateMap)
	audit := readStrictTestJSON[temporalStructureHoldoutFamilyAudit](t, fixture.family)
	for index := range audit.Fingerprints {
		if audit.Fingerprints[index].CaseID == privateMap.Entries[0].CaseID || audit.Fingerprints[index].CaseID == privateMap.Entries[1].CaseID {
			audit.Fingerprints[index].FrameHashes = []uint64{
				0x0f0f0f0f0f0f0f0f, 0x0f0f0f0f0f0f0f0f, 0x0f0f0f0f0f0f0f0f, 0x0f0f0f0f0f0f0f0f,
				0x0f0f0f0f0f0f0f0f, 0x0f0f0f0f0f0f0f0f, 0x0f0f0f0f0f0f0f0f, 0x0f0f0f0f0f0f0f0f,
				0x0f0f0f0f0f0f0f0f, 0x0f0f0f0f0f0f0f0f, 0x0f0f0f0f0f0f0f0f, 0x0f0f0f0f0f0f0f0f,
			}
			audit.Fingerprints[index].AudioRMS = make([]uint32, 50)
		}
	}
	fixture.family = rebuildTemporalStructureHoldoutFamily(t, fixture.referenceAudit, audit)
	_, err := BuildTemporalStructureHoldoutPlan(fixture.config(filepath.Join(t.TempDir(), "output")))
	if err == nil || !strings.Contains(err.Error(), "insufficient eligible bumper") {
		t.Fatalf("duplicate-family coverage error = %v", err)
	}
}

func TestBuildTemporalStructureHoldoutPlanRejectsInventedFamilyGraph(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	audit := readStrictTestJSON[temporalStructureHoldoutFamilyAudit](t, fixture.family)
	audit.Families = []temporalStructureHoldoutDuplicateFamily{{
		FamilyID: "invented-family", Members: []string{audit.Fingerprints[0].CaseID, audit.Fingerprints[1].CaseID},
		CompleteClique: false,
	}}
	audit.Summary.DuplicateFamilies = 1
	audit.Summary.NonCliqueFamilies = 1
	fixture.family = writeTemporalHumanJSON(t, t.TempDir(), "family.json", audit)
	_, err := BuildTemporalStructureHoldoutPlan(fixture.config(filepath.Join(t.TempDir(), "output")))
	if err == nil || !strings.Contains(err.Error(), "canonical duplicate graph") {
		t.Fatalf("invented family graph error = %v", err)
	}
}

func TestBuildTemporalStructureHoldoutPlanRejectsMissingReferenceFamilyFingerprint(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	audit := readStrictTestJSON[temporalStructureHoldoutFamilyAudit](t, fixture.family)
	audit.Fingerprints = audit.Fingerprints[:len(audit.Fingerprints)-1]
	audit.Summary.Cases--
	fixture.family = writeTemporalHumanJSON(t, t.TempDir(), "family.json", audit)
	_, err := BuildTemporalStructureHoldoutPlan(fixture.config(filepath.Join(t.TempDir(), "output")))
	if err == nil || !strings.Contains(err.Error(), "does not cover the reference audit") {
		t.Fatalf("missing family fingerprint error = %v", err)
	}
}

func TestBuildTemporalStructureHoldoutPlanAllowsFamilyAuthoritySupersetOfSelection(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	if _, err := BuildTemporalStructureHoldoutPlan(fixture.config(filepath.Join(t.TempDir(), "output"))); err != nil {
		t.Fatalf("reference family superset was rejected: %v", err)
	}
}

func TestTemporalStructureHoldoutAcceptsBoundLegacyReferenceAudit(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	reference := readStrictTestJSON[fillerreference.Audit](t, fixture.referenceAudit)
	reference.SchemaVersion = 2
	reference.Contract = temporalStructureHoldoutLegacyReferenceContract
	reference.Summary.Contract = temporalStructureHoldoutLegacyReferenceContract
	reference.Inputs.ContentReviewSHA256 = ""
	path := writeTemporalHumanJSON(t, t.TempDir(), "legacy-reference-audit.json", reference)
	if _, _, err := loadTemporalStructureHoldoutReferenceAudit(path, fixture.plannedAt); err != nil {
		t.Fatalf("bound legacy reference audit was rejected: %v", err)
	}
}

func TestTemporalStructureHoldoutRejectsIncompleteReferenceAudit(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	reference := readStrictTestJSON[fillerreference.Audit](t, fixture.referenceAudit)
	reference.Cases = reference.Cases[:len(reference.Cases)-1]
	reference.Summary.Cases--
	reference.Summary.Candidates--
	path := writeTemporalHumanJSON(t, t.TempDir(), "incomplete-reference-audit.json", reference)
	if _, _, err := loadTemporalStructureHoldoutReferenceAudit(path, fixture.plannedAt); err == nil || !strings.Contains(err.Error(), "reference audit is invalid") {
		t.Fatalf("incomplete reference audit error = %v", err)
	}
}

func TestTemporalStructureHoldoutAllowsSelectedReferenceExclusionWithoutFingerprint(t *testing.T) {
	fixture := newTemporalStructureHoldoutFixture(t)
	reference := readStrictTestJSON[fillerreference.Audit](t, fixture.referenceAudit)
	excludedID := reference.Cases[len(reference.Cases)-1].CaseID
	reference.Cases[len(reference.Cases)-1].Disposition = fillerreference.DispositionExclude
	reference.Summary.Candidates--
	reference.Summary.Excluded++
	referencePath := writeTemporalHumanJSON(t, t.TempDir(), "reference-audit.json", reference)
	reference, referenceSHA, err := loadTemporalStructureHoldoutReferenceAudit(referencePath, fixture.plannedAt)
	if err != nil {
		t.Fatal(err)
	}
	family := readStrictTestJSON[temporalStructureHoldoutFamilyAudit](t, fixture.family)
	family.SourceAudit = referenceSHA
	filtered := family.Fingerprints[:0]
	for _, fingerprint := range family.Fingerprints {
		if fingerprint.CaseID != excludedID {
			filtered = append(filtered, fingerprint)
		}
	}
	family.Fingerprints = filtered
	familyPath := rebuildTemporalStructureHoldoutFamily(t, referencePath, family)
	selectionRaw, err := os.ReadFile(fixture.selection)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := fillereval.DecodeTemporalTruthSelection(selectionRaw)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadTemporalStructureHoldoutFamily(familyPath, selection, reference, referenceSHA, fixture.plannedAt); err != nil {
		t.Fatalf("selected reference exclusion was not kept ineligible: %v", err)
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
	referenceAudit   string
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
	reference := fillerreference.Audit{
		SchemaVersion: fillerreference.AuditSchemaVersion, Contract: fillerreference.ContractVersion,
		GeneratedAt: suitability.ComparedAt.Add(30 * time.Minute),
		Inputs: fillerreference.InputIdentity{
			ManifestSHA256: strings.Repeat("1", 64), PacketsSHA256: strings.Repeat("2", 64),
			MappingSHA256: strings.Repeat("3", 64), DownloadLedgerSHA256: strings.Repeat("4", 64),
			ContentReviewSHA256: strings.Repeat("5", 64),
		},
		Summary: fillerreference.Summary{
			Cases: len(privateMap.Entries), Candidates: len(privateMap.Entries),
			Mapping: "fixture-mapping-v1", Contract: fillerreference.ContractVersion,
		},
	}
	for _, item := range privateMap.Entries {
		reference.Cases = append(reference.Cases, fillerreference.Case{
			CaseID: item.CaseID, ContentSHA256: item.ContentSHA256, SourceLocalFile: item.SourceLocalFile,
			Disposition: fillerreference.DispositionCandidate,
		})
	}
	for index := len(reference.Cases); index < 300; index++ {
		caseID := fmt.Sprintf("reference-only-%03d", index)
		reference.Cases = append(reference.Cases, fillerreference.Case{
			CaseID: caseID, ContentSHA256: hashBytes([]byte(caseID)), SourceLocalFile: caseID + ".mp4",
			Disposition: fillerreference.DispositionCandidate,
		})
	}
	reference.Summary.Cases = len(reference.Cases)
	reference.Summary.Candidates = len(reference.Cases)
	referencePath := writeTemporalHumanJSON(t, base.root, "reference-audit.json", reference)
	referenceRaw, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatal(err)
	}
	referenceSHA, err := hashFile(referencePath)
	if err != nil {
		t.Fatal(err)
	}
	fingerprints := make([]fillerreference.FamilyFingerprint, 0, len(reference.Cases))
	for _, item := range reference.Cases {
		fingerprints = append(fingerprints, fillerreference.FamilyFingerprint{
			CaseID: item.CaseID, ContentSHA256: item.ContentSHA256, LocalFile: item.SourceLocalFile,
			FrameHashes: []uint64{1}, AudioRMS: []uint32{1},
		})
	}
	family, err := fillerreference.BuildFamilyAudit(referenceRaw, fingerprints, suitability.ComparedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if family.SourceAudit != referenceSHA {
		t.Fatal("family fixture did not bind reference audit bytes")
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
		quality: qualityPath, suitability: suitabilityPath, referenceAudit: referencePath, family: familyPath, inventory: inventoryPath,
		plannedAt: inventory.GeneratedAt.Add(time.Hour),
	}
}

func (fixture temporalStructureHoldoutFixture) config(output string) TemporalStructureHoldoutConfig {
	return TemporalStructureHoldoutConfig{
		SelectionPath: fixture.selection, EvidenceManifestPath: fixture.manifest, EvidencePrivateMapPath: fixture.privateMap,
		HumanAssessmentPath: fixture.humanAssessment, HumanAttestationPath: fixture.humanAttestation,
		MediaQualityPath: fixture.quality, SuitabilityPath: fixture.suitability, FamilyAuditPath: fixture.family,
		ReferenceAuditPath:     fixture.referenceAudit,
		ProgrammeInventoryPath: fixture.inventory, SourceRoot: fixture.root, Seed: "holdout-seed",
		PlannedAt: fixture.plannedAt, OutputDir: output,
	}
}

func rebuildTemporalStructureHoldoutFamily(t *testing.T, referencePath string, audit temporalStructureHoldoutFamilyAudit) string {
	t.Helper()
	referenceRaw, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := fillerreference.BuildFamilyAudit(referenceRaw, audit.Fingerprints, audit.GeneratedAt)
	if err != nil {
		t.Fatal(err)
	}
	return writeTemporalHumanJSON(t, t.TempDir(), "family.json", rebuilt)
}
