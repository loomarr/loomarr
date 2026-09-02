package fillerreview

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestBuildTemporalHumanReviewPackageCreatesDistinctBlindedHardlinkBatches(t *testing.T) {
	fixture := newTemporalHumanReviewFixture(t)
	first := fixture.build(t, "batch-one", "seed-one")
	repeat := fixture.build(t, "batch-one", "seed-one")
	second := fixture.build(t, "batch-two", "seed-two")

	firstPack, firstSHA, err := LoadTemporalHumanReviewPackage(filepath.Join(first, "public", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	secondPack, secondSHA, err := LoadTemporalHumanReviewPackage(filepath.Join(second, "public", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if firstSHA == secondSHA || firstPack.Cases[0].Alias == secondPack.Cases[0].Alias || slices.Equal(temporalHumanAliases(firstPack), temporalHumanAliases(secondPack)) {
		t.Fatal("independent batches did not receive distinct identities and order")
	}
	repeatPackageSHA, _ := hashFile(filepath.Join(repeat, "public", "manifest.json"))
	firstViewerSHA, _ := hashFile(filepath.Join(first, "public", "index.html"))
	repeatViewerSHA, _ := hashFile(filepath.Join(repeat, "public", "index.html"))
	firstMapSHA, _ := hashFile(filepath.Join(first, "private", "map.json"))
	repeatMapSHA, _ := hashFile(filepath.Join(repeat, "private", "map.json"))
	if repeatPackageSHA != firstSHA || repeatViewerSHA != firstViewerSHA || repeatMapSHA != firstMapSHA {
		t.Fatal("same declared seed and batch did not reproduce byte-identical authority files")
	}

	sourceInfo, err := os.Stat(filepath.Join(fixture.root, "public", "review.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	linkedInfo, err := os.Stat(filepath.Join(first, "public", filepath.FromSlash(firstPack.Cases[0].Video.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(sourceInfo, linkedInfo) {
		t.Fatal("default batch media is not a hard link to the sealed evidence")
	}

	publicRaw := readTree(t, filepath.Join(first, "public"))
	for _, secret := range fixture.secrets {
		if bytes.Contains(publicRaw, []byte(secret)) {
			t.Fatalf("public batch leaks coordinator-only value %q", secret)
		}
	}
	if !bytes.Contains(publicRaw, []byte("Temporal filler review")) || !bytes.Contains(publicRaw, []byte("No transcript is available")) {
		t.Fatal("offline reviewer was not embedded in the public batch")
	}
}

func TestTemporalHumanReviewLeakageAuditFailsWithoutPublishing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "visible.txt"), []byte("private-case-id"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := auditTemporalHumanReviewStrings(root, []string{"private-case-id"}); err == nil || !strings.Contains(err.Error(), "leaks") {
		t.Fatalf("leakage error = %v", err)
	}
	output := filepath.Join(t.TempDir(), "batch")
	stage, err := beginTemporalHumanReviewStage(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage.path, "partial"), []byte("private-case-id"), 0o640); err != nil {
		t.Fatal(err)
	}
	stage.Cleanup()
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("failed audit published output: %v", err)
	}
}

func TestTemporalLeakageMatcherFindsSecretsAcrossReadChunks(t *testing.T) {
	matcher := newTemporalLeakageMatcher([]string{"private-prefix", "private-secret", "selection-history"})
	match, err := matcher.Find(iotest.OneByteReader(strings.NewReader("public bytes then private-secret then more")))
	if err != nil || match != "private-secret" {
		t.Fatalf("cross-chunk leakage match = %q, %v", match, err)
	}
	match, err = matcher.Find(strings.NewReader("entirely safe public bytes"))
	if err != nil || match != "" {
		t.Fatalf("safe leakage match = %q, %v", match, err)
	}
}

func TestLockTemporalHumanReviewEmitsCanonicalBoundArtifacts(t *testing.T) {
	fixture := newTemporalHumanReviewFixture(t)
	batch := fixture.build(t, "batch-lock", "seed-lock")
	packPath := filepath.Join(batch, "public", "manifest.json")
	mapPath := filepath.Join(batch, "private", "map.json")
	pack, packageSHA, err := LoadTemporalHumanReviewPackage(packPath)
	if err != nil {
		t.Fatal(err)
	}
	submission := completeTemporalHumanSubmission(pack, packageSHA, "reviewer-one")
	submissionPath := writeTemporalHumanJSON(t, t.TempDir(), "submission.json", submission)
	output := filepath.Join(t.TempDir(), "locked")
	result, err := LockTemporalHumanReview(TemporalHumanReviewLockConfig{
		PackagePath: packPath, PrivateMapPath: mapPath, SubmissionPath: submissionPath,
		ExpectedReviewerID: "reviewer-one", LockedAt: fixture.preparedAt.Add(2 * time.Hour), MaximumAge: 24 * time.Hour, OutputDir: output,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Assessments != fillereval.TemporalTruthSelectionCases || !reviewSHA256(result.AssessmentSetSHA256) || !reviewSHA256(result.AttestationSHA256) {
		t.Fatalf("lock result = %+v", result)
	}
	set, err := readStrictJSON[TemporalHumanAssessmentSet](filepath.Join(output, "assessment-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	for index, assessment := range set.Assessments {
		if !strings.HasPrefix(assessment.EvidenceAlias, "evidence-") || (index > 0 && set.Assessments[index-1].EvidenceAlias >= assessment.EvidenceAlias) {
			t.Fatalf("canonical assessments are not stably unblinded and sorted: %+v", set.Assessments)
		}
	}
	attestation, err := readStrictJSON[TemporalHumanReviewAttestation](filepath.Join(output, "attestation.json"))
	if err != nil {
		t.Fatal(err)
	}
	mapSHA, _ := hashFile(mapPath)
	submissionSHA, _ := hashFile(submissionPath)
	if attestation.PackageSHA256 != packageSHA || attestation.MapSHA256 != mapSHA || attestation.SubmissionSHA256 != submissionSHA || attestation.AssessmentSetSHA256 != result.AssessmentSetSHA256 || temporalHumanAttestationSHA256(attestation) != result.AttestationSHA256 {
		t.Fatalf("attestation does not bind all inputs and outputs: %+v", attestation)
	}
}

func TestTemporalHumanReviewAttestationChangesWithEveryAcceptedAuthority(t *testing.T) {
	fixture := newTemporalHumanReviewFixture(t)
	batch := fixture.build(t, "batch-attestation", "seed-attestation")
	packPath := filepath.Join(batch, "public", "manifest.json")
	mapPath := filepath.Join(batch, "private", "map.json")
	pack, packageSHA, err := LoadTemporalHumanReviewPackage(packPath)
	if err != nil {
		t.Fatal(err)
	}
	baselineSubmission := completeTemporalHumanSubmission(pack, packageSHA, "reviewer-one")
	lock := func(t *testing.T, packagePath, privateMapPath string, submission TemporalHumanReviewSubmission, reviewer string) string {
		t.Helper()
		submissionPath := writeTemporalHumanJSON(t, t.TempDir(), "submission.json", submission)
		result, err := LockTemporalHumanReview(TemporalHumanReviewLockConfig{
			PackagePath: packagePath, PrivateMapPath: privateMapPath, SubmissionPath: submissionPath,
			ExpectedReviewerID: reviewer, LockedAt: fixture.preparedAt.Add(2 * time.Hour), MaximumAge: 24 * time.Hour, OutputDir: filepath.Join(t.TempDir(), "locked"),
		})
		if err != nil {
			t.Fatal(err)
		}
		return result.AttestationSHA256
	}
	baseline := lock(t, packPath, mapPath, baselineSubmission, "reviewer-one")

	answerChanged := cloneTemporalHumanSubmission(t, baselineSubmission)
	changedRole := fillereval.TemporalRolePromo
	answerChanged.Answers[0].Role = &changedRole
	timestampChanged := cloneTemporalHumanSubmission(t, baselineSubmission)
	timestampChanged.Answers[0].DecisiveAtMS++
	reviewerChanged := cloneTemporalHumanSubmission(t, baselineSubmission)
	reviewerChanged.ReviewerID = "reviewer-two"
	for index := range reviewerChanged.Answers {
		reviewerChanged.Answers[index].ReviewerID = "reviewer-two"
	}

	secondBatch := fixture.build(t, "batch-attestation-two", "seed-attestation-two")
	secondPackPath := filepath.Join(secondBatch, "public", "manifest.json")
	secondMapPath := filepath.Join(secondBatch, "private", "map.json")
	secondPack, secondPackageSHA, err := LoadTemporalHumanReviewPackage(secondPackPath)
	if err != nil {
		t.Fatal(err)
	}
	secondSubmission := completeTemporalHumanSubmission(secondPack, secondPackageSHA, "reviewer-one")

	mapping, err := readStrictJSON[TemporalHumanReviewMap](mapPath)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(mapping.Entries)
	reorderedMap := writeTemporalHumanJSON(t, t.TempDir(), "map.json", mapping)

	variants := map[string]string{
		"answer":    lock(t, packPath, mapPath, answerChanged, "reviewer-one"),
		"timestamp": lock(t, packPath, mapPath, timestampChanged, "reviewer-one"),
		"reviewer":  lock(t, packPath, mapPath, reviewerChanged, "reviewer-two"),
		"package":   lock(t, secondPackPath, secondMapPath, secondSubmission, "reviewer-one"),
		"map":       lock(t, packPath, reorderedMap, baselineSubmission, "reviewer-one"),
	}
	for authority, digest := range variants {
		if digest == baseline {
			t.Errorf("changing accepted %s did not change the attestation digest", authority)
		}
	}
}

func TestLockTemporalHumanReviewFailsClosed(t *testing.T) {
	fixture := newTemporalHumanReviewFixture(t)
	batch := fixture.build(t, "batch-fail-closed", "seed-fail-closed")
	packPath := filepath.Join(batch, "public", "manifest.json")
	mapPath := filepath.Join(batch, "private", "map.json")
	pack, packageSHA, err := LoadTemporalHumanReviewPackage(packPath)
	if err != nil {
		t.Fatal(err)
	}
	valid := completeTemporalHumanSubmission(pack, packageSHA, "reviewer-one")
	tests := []struct {
		name       string
		mutate     func(*TemporalHumanReviewSubmission)
		reviewer   string
		lockedAt   time.Time
		maximumAge time.Duration
	}{
		{name: "incomplete", mutate: func(value *TemporalHumanReviewSubmission) { value.Answers = value.Answers[:len(value.Answers)-1] }},
		{name: "duplicate", mutate: func(value *TemporalHumanReviewSubmission) { value.Answers[1] = value.Answers[0] }},
		{name: "unknown alias", mutate: func(value *TemporalHumanReviewSubmission) {
			value.Answers[0].Alias = "review-00000000000000000000000000000000"
		}},
		{name: "mixed reviewer", mutate: func(value *TemporalHumanReviewSubmission) { value.Answers[0].ReviewerID = "reviewer-two" }},
		{name: "wrong batch", mutate: func(value *TemporalHumanReviewSubmission) { value.BatchID = "other-batch" }},
		{name: "wrong package", mutate: func(value *TemporalHumanReviewSubmission) { value.PackageSHA256 = strings.Repeat("f", 64) }},
		{name: "invalid role", mutate: func(value *TemporalHumanReviewSubmission) {
			invalid := fillereval.TemporalRole("advert")
			value.Answers[0].Role = &invalid
		}},
		{name: "role on nonstandalone", mutate: func(value *TemporalHumanReviewSubmission) {
			role := fillereval.TemporalRoleCommercial
			value.Answers[1].Role = &role
		}},
		{name: "invalid timestamp", mutate: func(value *TemporalHumanReviewSubmission) { value.Answers[0].DecisiveAtMS = pack.Cases[0].DurationMS }},
		{name: "stale completion", mutate: func(value *TemporalHumanReviewSubmission) { value.CompletedAt = fixture.preparedAt.Add(25 * time.Hour) }},
		{name: "unexpected reviewer", reviewer: "reviewer-two"},
		{name: "stale lock", lockedAt: fixture.preparedAt.Add(25 * time.Hour)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneTemporalHumanSubmission(t, valid)
			if test.mutate != nil {
				test.mutate(&candidate)
			}
			reviewer := test.reviewer
			if reviewer == "" {
				reviewer = "reviewer-one"
			}
			lockedAt := test.lockedAt
			if lockedAt.IsZero() {
				lockedAt = fixture.preparedAt.Add(2 * time.Hour)
			}
			maximumAge := test.maximumAge
			if maximumAge == 0 {
				maximumAge = 24 * time.Hour
			}
			submissionPath := writeTemporalHumanJSON(t, t.TempDir(), "submission.json", candidate)
			output := filepath.Join(t.TempDir(), "locked")
			_, err := LockTemporalHumanReview(TemporalHumanReviewLockConfig{
				PackagePath: packPath, PrivateMapPath: mapPath, SubmissionPath: submissionPath,
				ExpectedReviewerID: reviewer, LockedAt: lockedAt, MaximumAge: maximumAge, OutputDir: output,
			})
			if err == nil {
				t.Fatal("invalid submission was accepted")
			}
			if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
				t.Fatalf("failed lock published output: %v", statErr)
			}
		})
	}

	t.Run("unknown JSON field", func(t *testing.T) {
		raw, err := json.Marshal(valid)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw[:len(raw)-1], []byte(`,"priorLabel":"commercial"}`)...)
		submissionPath := filepath.Join(t.TempDir(), "submission.json")
		if err := os.WriteFile(submissionPath, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = LockTemporalHumanReview(TemporalHumanReviewLockConfig{
			PackagePath: packPath, PrivateMapPath: mapPath, SubmissionPath: submissionPath,
			ExpectedReviewerID: "reviewer-one", LockedAt: fixture.preparedAt.Add(2 * time.Hour), MaximumAge: 24 * time.Hour, OutputDir: filepath.Join(t.TempDir(), "locked"),
		})
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("unknown-field error = %v", err)
		}
	})

	for _, malformed := range []struct {
		name string
		raw  []byte
	}{
		{name: "blank submission", raw: nil},
		{name: "trailing JSON value", raw: []byte("{}\n{}\n")},
	} {
		t.Run(malformed.name, func(t *testing.T) {
			submissionPath := filepath.Join(t.TempDir(), "submission.json")
			if err := os.WriteFile(submissionPath, malformed.raw, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LockTemporalHumanReview(TemporalHumanReviewLockConfig{
				PackagePath: packPath, PrivateMapPath: mapPath, SubmissionPath: submissionPath,
				ExpectedReviewerID: "reviewer-one", LockedAt: fixture.preparedAt.Add(2 * time.Hour), MaximumAge: 24 * time.Hour, OutputDir: filepath.Join(t.TempDir(), "locked"),
			})
			if err == nil {
				t.Fatal("malformed submission was accepted")
			}
		})
	}
}

type temporalHumanReviewFixture struct {
	root       string
	manifest   string
	privateMap string
	selection  string
	preparedAt time.Time
	secrets    []string
}

func newTemporalHumanReviewFixture(t *testing.T) temporalHumanReviewFixture {
	t.Helper()
	root := t.TempDir()
	publicRoot := filepath.Join(root, "public")
	privateRoot := filepath.Join(root, "private")
	if err := os.MkdirAll(publicRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	videoRaw := []byte("tiny review video")
	frameRaw := []byte("tiny review frame")
	if err := os.WriteFile(filepath.Join(publicRoot, "review.mp4"), videoRaw, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(publicRoot, "frame.jpg"), frameRaw, 0o640); err != nil {
		t.Fatal(err)
	}
	identity := TemporalTruthToolIdentity{Path: "/reviewed/tool", Version: "v1", BinarySHA256: strings.Repeat("a", 64)}
	manifest := TemporalTruthEvidenceManifest{
		SchemaVersion: TemporalTruthEvidenceSchemaVersion, ContractVersion: TemporalTruthEvidenceContractVersion,
		EvidenceVersion: TemporalTruthEvidenceVersion, GeneratedAt: time.Unix(1, 0).UTC(), SelectionSHA256: strings.Repeat("b", 64),
		MediaTools: TemporalTruthMediaIdentity{FFmpeg: identity, FFprobe: identity}, OCR: TemporalTruthOCRStatus{Status: "unavailable"},
		Config: TemporalTruthEvidenceSettings{SceneThreshold: .3, MaximumFramesPerCase: TemporalEvidenceMaxFrames, MaximumVideoBytes: TemporalTruthMaximumVideoBytes, PerCaseTimeoutMS: 1_000},
	}
	private := TemporalTruthEvidencePrivateMap{
		SchemaVersion: TemporalTruthEvidenceSchemaVersion, ContractVersion: TemporalTruthEvidenceContractVersion,
		SelectionSHA256: manifest.SelectionSHA256, DraftSHA256: strings.Repeat("c", 64), DownloadLedgerSHA256: strings.Repeat("d", 64), PacketsSHA256: strings.Repeat("e", 64), TranscriptSetSHA256: strings.Repeat("f", 64),
	}
	secrets := []string{private.DraftSHA256, private.DownloadLedgerSHA256, private.PacketsSHA256, private.TranscriptSetSHA256}
	selection := fillereval.TemporalTruthSelection{
		SchemaVersion: fillereval.TemporalTruthSelectionSchemaVersion, ContractVersion: fillereval.TemporalTruthSelectionContractVersion,
		Seed: "coordinator-private-selection-seed", Inputs: []fillereval.TemporalTruthInputDigest{{Name: "private-model-history", SHA256: hashBytes([]byte("private model history"))}},
	}
	for index := 0; index < fillereval.TemporalTruthSelectionCases; index++ {
		alias := fmt.Sprintf("evidence-%02d-private", index)
		manifest.Cases = append(manifest.Cases, TemporalTruthEvidenceCase{
			Alias: alias, DurationMS: 1_000,
			Plan:   TemporalEvidencePlan{SchemaVersion: TemporalEvidencePlanSchemaVersion, SourceEndMS: 1_000, EvidenceEndMS: 1_000, FrameTimesMS: []int64{0}},
			Video:  TemporalTruthEvidenceFile{Path: "review.mp4", SHA256: hashBytes(videoRaw), Bytes: int64(len(videoRaw)), DurationMS: 1_000, Width: 320, Height: 240},
			Frames: []TemporalTruthEvidenceFrame{{ID: "frame-01", Path: "frame.jpg", SHA256: hashBytes(frameRaw), Bytes: int64(len(frameRaw)), Width: 320, Height: 240, AtMS: 0}},
		})
		entry := TemporalTruthEvidencePrivateEntry{
			Alias: alias, CaseID: fmt.Sprintf("private-case-%02d", index), ContentSHA256: hashBytes([]byte(fmt.Sprintf("content-%d", index))),
			SourceLocalFile: fmt.Sprintf("private/source-%02d.mp4", index), SourceSHA256: hashBytes([]byte(fmt.Sprintf("source-%d", index))), PacketSHA256: hashBytes([]byte(fmt.Sprintf("packet-%d", index))),
		}
		private.Entries = append(private.Entries, entry)
		bucket, riskClass := temporalHumanSelectionBucket(index)
		selection.Cases = append(selection.Cases, fillereval.TemporalTruthSelectionCase{
			CaseID: entry.CaseID, ContentSHA256: entry.ContentSHA256, Bucket: bucket, RiskClass: riskClass,
			RankSHA256: fmt.Sprintf("%064x", index+1), Strata: []string{"duration:private-band", fmt.Sprintf("history:private-label-%02d", index), "source:private-lane"},
		})
		secrets = append(secrets, entry.Alias, entry.CaseID, entry.ContentSHA256, entry.SourceLocalFile, entry.SourceSHA256, entry.PacketSHA256)
	}
	selectionPath := writeTemporalHumanJSON(t, privateRoot, "selection.json", selection)
	selectionRaw, err := os.ReadFile(selectionPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest.SelectionSHA256 = hashBytes(selectionRaw)
	private.SelectionSHA256 = manifest.SelectionSHA256
	manifestPath := writeTemporalHumanJSON(t, publicRoot, "manifest.json", manifest)
	privatePath := writeTemporalHumanJSON(t, privateRoot, "map.json", private)
	secrets = append(secrets, selection.Seed, selection.Inputs[0].Name, selection.Inputs[0].SHA256)
	for _, item := range selection.Cases {
		secrets = append(secrets, item.RankSHA256)
		secrets = append(secrets, item.Strata...)
	}
	return temporalHumanReviewFixture{root: root, manifest: manifestPath, privateMap: privatePath, selection: selectionPath, preparedAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), secrets: secrets}
}

func (fixture temporalHumanReviewFixture) build(t *testing.T, batch, seed string) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), batch)
	result, err := BuildTemporalHumanReviewPackage(TemporalHumanReviewPackageConfig{
		EvidenceManifestPath: fixture.manifest, EvidencePrivateMapPath: fixture.privateMap,
		SelectionPath: fixture.selection,
		BatchID:       batch, Seed: seed, PreparedAt: fixture.preparedAt, OutputDir: output, Materialization: TemporalHumanReviewHardlink,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cases != fillereval.TemporalTruthSelectionCases || result.Files != 2*fillereval.TemporalTruthSelectionCases {
		t.Fatalf("package result = %+v", result)
	}
	return output
}

func temporalHumanSelectionBucket(index int) (string, string) {
	if index < 16 {
		return fillereval.TemporalTruthBucketAgreement, ""
	}
	if index < 32 {
		return fillereval.TemporalTruthBucketDisagreement, ""
	}
	risks := []string{
		fillereval.TemporalTruthRiskCompilation, fillereval.TemporalTruthRiskProgrammeExcerpt,
		fillereval.TemporalTruthRiskShortBoundary, fillereval.TemporalTruthRiskUnusableUnclear,
	}
	return fillereval.TemporalTruthBucketHighRisk, risks[(index-32)/4]
}

func completeTemporalHumanSubmission(pack TemporalHumanReviewPackage, packageSHA, reviewer string) TemporalHumanReviewSubmission {
	role := fillereval.TemporalRoleCommercial
	result := TemporalHumanReviewSubmission{
		SchemaVersion: TemporalHumanReviewSchemaVersion, ContractVersion: TemporalHumanReviewContractVersion,
		BatchID: pack.BatchID, PackageSHA256: packageSHA, ReviewerID: reviewer, PreparedAt: pack.PreparedAt, CompletedAt: pack.PreparedAt.Add(time.Hour),
	}
	for index, item := range pack.Cases {
		answer := TemporalHumanReviewAnswer{Alias: item.Alias, ReviewerID: reviewer, Unit: fillereval.UnitCompilation, DecisiveAtMS: int64(index % int(item.DurationMS))}
		if index%2 == 0 {
			answer.Unit = fillereval.UnitStandalone
			answer.Role = &role
		}
		result.Answers = append(result.Answers, answer)
	}
	return result
}

func cloneTemporalHumanSubmission(t *testing.T, source TemporalHumanReviewSubmission) TemporalHumanReviewSubmission {
	t.Helper()
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var result TemporalHumanReviewSubmission
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func writeTemporalHumanJSON(t *testing.T, root, name string, value any) string {
	t.Helper()
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func temporalHumanAliases(pack TemporalHumanReviewPackage) []string {
	aliases := make([]string, 0, len(pack.Cases))
	for _, item := range pack.Cases {
		aliases = append(aliases, item.Alias)
	}
	return aliases
}

func readTree(t *testing.T, root string) []byte {
	t.Helper()
	var result bytes.Buffer
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		raw, err := os.ReadFile(path)
		if err == nil {
			_, _ = result.Write(raw)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}
