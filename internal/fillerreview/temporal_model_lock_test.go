package fillerreview

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestLockTemporalModelAssessmentBindsPostHumanResult(t *testing.T) {
	fixture := newTemporalModelLockFixture(t)
	output := filepath.Join(t.TempDir(), "locked-model")
	locked, err := LockTemporalModelAssessment(TemporalModelAssessmentLockConfig{
		PackagePath: fixture.packagePath, PrivateMapPath: fixture.mapPath, ResultPath: fixture.resultPath,
		SnapshotPath: fixture.snapshotPath, HumanAssessmentPath: fixture.humanAssessmentPath,
		HumanAttestationPath: fixture.humanAttestationPath, ExpectedCases: 1,
		ReleasedAt: fixture.modelTime.Add(time.Minute), OutputDir: output,
	})
	if err != nil {
		t.Fatal(err)
	}
	if locked.Assessments != 1 || !reviewSHA256(locked.AssessmentSetSHA256) || !reviewSHA256(locked.AttestationSHA256) {
		t.Fatalf("lock result = %+v", locked)
	}
	set, err := readStrictJSON[TemporalModelAssessmentSet](filepath.Join(output, "assessment-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	if set.PanelSlot != "panel-a" || set.Assessor.ModelFamily != "qwen3.8" || len(set.Assessments) != 1 || set.Assessments[0].EvidenceAlias != "evidence-01" || len(set.Assessments[0].UnitDecisiveAtMS) != 1 || set.Assessments[0].UnitDecisiveAtMS[0] != 100 || len(set.Assessments[0].RoleDecisiveAtMS) != 1 || set.Assessments[0].RoleDecisiveAtMS[0] != 200 {
		t.Fatalf("canonical model set = %+v", set)
	}
	attestation, err := readStrictJSON[TemporalModelAssessmentAttestation](filepath.Join(output, "attestation.json"))
	if err != nil {
		t.Fatal(err)
	}
	if attestation.AssessmentSetSHA256 != locked.AssessmentSetSHA256 || temporalModelAssessmentAttestationSHA256(attestation) != locked.AttestationSHA256 || attestation.HumanAttestationSHA256 != fixture.humanAttestation.AttestationSHA256 {
		t.Fatalf("model attestation = %+v", attestation)
	}
	for _, name := range []string{"assessment-set.json", "attestation.json"} {
		info, statErr := os.Stat(filepath.Join(output, name))
		if statErr != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode=%v err=%v", name, info.Mode(), statErr)
		}
	}
}

func TestLockTemporalModelAssessmentFailsClosed(t *testing.T) {
	fixture := newTemporalModelLockFixture(t)
	base := TemporalModelAssessmentLockConfig{
		PackagePath: fixture.packagePath, PrivateMapPath: fixture.mapPath, ResultPath: fixture.resultPath,
		SnapshotPath: fixture.snapshotPath, HumanAssessmentPath: fixture.humanAssessmentPath,
		HumanAttestationPath: fixture.humanAttestationPath, ExpectedCases: 1,
		ReleasedAt: fixture.modelTime.Add(time.Minute),
	}

	t.Run("release predates result", func(t *testing.T) {
		config := base
		config.ReleasedAt = fixture.modelTime.Add(-time.Second)
		config.OutputDir = filepath.Join(t.TempDir(), "locked")
		assertTemporalModelLockFails(t, config)
	})

	t.Run("model predates human lock", func(t *testing.T) {
		attestation := fixture.humanAttestation
		attestation.LockedAt = fixture.modelTime.Add(time.Minute)
		attestation.AttestationSHA256 = temporalHumanAttestationSHA256(attestation)
		attestationPath := writeTemporalHumanJSON(t, t.TempDir(), "attestation.json", attestation)
		config := base
		config.HumanAttestationPath = attestationPath
		config.ReleasedAt = fixture.modelTime.Add(2 * time.Minute)
		config.OutputDir = filepath.Join(t.TempDir(), "locked")
		assertTemporalModelLockFails(t, config)
	})

	t.Run("snapshot drift", func(t *testing.T) {
		snapshot, err := readStrictJSON[fillerbakeoff.OpenRouterSnapshot](fixture.snapshotPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot.RetrievedAt = snapshot.RetrievedAt.Add(time.Second)
		snapshotPath := writeTemporalHumanJSON(t, t.TempDir(), "snapshot.json", snapshot)
		config := base
		config.SnapshotPath = snapshotPath
		config.OutputDir = filepath.Join(t.TempDir(), "locked")
		assertTemporalModelLockFails(t, config)
	})

	t.Run("map drift", func(t *testing.T) {
		mapping, err := readStrictJSON[TemporalModelReviewMap](fixture.mapPath)
		if err != nil {
			t.Fatal(err)
		}
		mapping.Entries[0].EvidenceAlias = "evidence-other"
		mapPath := writeTemporalHumanJSON(t, t.TempDir(), "map.json", mapping)
		config := base
		config.PrivateMapPath = mapPath
		config.OutputDir = filepath.Join(t.TempDir(), "locked")
		assertTemporalModelLockFails(t, config)
	})
}

func assertTemporalModelLockFails(t *testing.T, config TemporalModelAssessmentLockConfig) {
	t.Helper()
	if _, err := LockTemporalModelAssessment(config); err == nil {
		t.Fatal("invalid model lock authority was accepted")
	}
	if _, err := os.Lstat(config.OutputDir); !os.IsNotExist(err) {
		t.Fatalf("failed lock published output: %v", err)
	}
}

type temporalModelLockFixture struct {
	packagePath, mapPath, resultPath, snapshotPath string
	humanAssessmentPath, humanAttestationPath      string
	humanAttestation                               TemporalHumanReviewAttestation
	modelTime                                      time.Time
}

func newTemporalModelLockFixture(t *testing.T) temporalModelLockFixture {
	t.Helper()
	const (
		model    = "review/vendor-model"
		provider = "Provider Route"
		slug     = "provider/route"
	)
	packagePath := writeTemporalModelInferenceFixture(t)
	pack, _, packageSHA, err := LoadTemporalModelReviewPackage(packagePath, 1)
	if err != nil {
		t.Fatal(err)
	}
	mapping := TemporalModelReviewMap{
		SchemaVersion: TemporalModelReviewSchemaVersion, ContractVersion: TemporalModelReviewContractVersion,
		PanelSlot: pack.PanelSlot, BatchID: pack.BatchID, PreparedAt: pack.PreparedAt, Seed: "model-seed",
		EvidenceManifestSHA256: pack.EvidenceManifestSHA256, SelectionSHA256: pack.SelectionSHA256, PackageSHA256: packageSHA,
		Entries: []TemporalModelReviewMapEntry{{Alias: pack.Cases[0].Alias, EvidenceAlias: "evidence-01"}},
	}
	mapPath := writeTemporalHumanJSON(t, t.TempDir(), "map.json", mapping)

	modelTime := time.Unix(20_000, 0).UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openRouterStructuredRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		content := `{"kind":"standalone","decisiveSignalIds":["frame-01"]}`
		if request.ResponseFormat.JSONSchema.Name == "filler_temporal_role" {
			content = `{"kind":"commercial","decisiveSignalIds":["transcript-01"]}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "generation", "model": model,
			"choices": []any{map[string]any{"message": map[string]any{"content": content, "reasoning": ""}}},
			"usage":   map[string]any{"prompt_tokens": 100, "completion_tokens": 20, "cost": 0.001},
			"openrouter_metadata": map[string]any{
				"attempt": 1, "attempts": []any{map[string]any{"provider": provider, "model": model, "status": 200}},
				"endpoints": map[string]any{"available": []any{map[string]any{"provider": provider, "model": model, "selected": true}}},
			},
		})
	}))
	defer server.Close()
	snapshot := openRouterReviewSnapshot(server.URL, modelTime)
	result, err := RunOpenRouterTemporalModelAssessment(t.Context(), OpenRouterTemporalConfig{
		PackagePath: packagePath, CheckpointDir: filepath.Join(t.TempDir(), "checkpoint"), BaseURL: server.URL,
		APIKey: "test-key", Snapshot: snapshot, Model: model, ModelFamily: "qwen3.8",
		UpstreamProvider: provider, UpstreamProviderSlug: slug, AssessorID: "panel-a-model",
		ExpectedPackageCases: 1, ExpectedCalibrationCases: 1, PerCaseTimeout: time.Second,
		MaxRequests: 2, MaxSpendNanoUSD: 4_000_000, MaxChargeNanoUSD: 2_000_000,
		AllowInsecureTestURL: true, Now: func() time.Time { return modelTime },
	})
	if err != nil {
		t.Fatal(err)
	}
	resultPath := writeTemporalHumanJSON(t, t.TempDir(), "result.json", result)
	snapshotPath := writeTemporalHumanJSON(t, t.TempDir(), "snapshot.json", snapshot)

	role := fillereval.TemporalRoleCommercial
	humanSet := TemporalHumanAssessmentSet{
		SchemaVersion: TemporalHumanReviewSchemaVersion, ContractVersion: TemporalHumanReviewContractVersion,
		BatchID: "human-batch", ReviewerID: "human", EvidenceManifestSHA256: pack.EvidenceManifestSHA256,
		SelectionSHA256: pack.SelectionSHA256, PackageSHA256: strings.Repeat("1", 64), SubmissionSHA256: strings.Repeat("2", 64),
		PreparedAt: modelTime.Add(-2 * time.Hour), CompletedAt: modelTime.Add(-70 * time.Minute),
		Assessments: []TemporalHumanReviewAssessment{{EvidenceAlias: "evidence-01", Unit: fillereval.UnitStandalone, Role: &role, DecisiveAtMS: 100}},
	}
	humanAssessmentPath := writeTemporalHumanJSON(t, t.TempDir(), "assessment-set.json", humanSet)
	humanSetSHA, err := hashFile(humanAssessmentPath)
	if err != nil {
		t.Fatal(err)
	}
	humanAttestation := TemporalHumanReviewAttestation{
		SchemaVersion: TemporalHumanReviewSchemaVersion, ContractVersion: TemporalHumanReviewContractVersion,
		BatchID: humanSet.BatchID, ReviewerID: humanSet.ReviewerID, LockedAt: modelTime.Add(-time.Hour),
		PackageSHA256: humanSet.PackageSHA256, MapSHA256: strings.Repeat("3", 64), SubmissionSHA256: humanSet.SubmissionSHA256,
		AssessmentSetSHA256: humanSetSHA,
	}
	humanAttestation.AttestationSHA256 = temporalHumanAttestationSHA256(humanAttestation)
	humanAttestationPath := writeTemporalHumanJSON(t, t.TempDir(), "attestation.json", humanAttestation)
	return temporalModelLockFixture{
		packagePath: packagePath, mapPath: mapPath, resultPath: resultPath, snapshotPath: snapshotPath,
		humanAssessmentPath: humanAssessmentPath, humanAttestationPath: humanAttestationPath,
		humanAttestation: humanAttestation, modelTime: modelTime,
	}
}
