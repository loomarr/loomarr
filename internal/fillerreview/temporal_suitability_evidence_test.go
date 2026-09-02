package fillerreview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunOpenRouterTemporalSuitabilityAcceptsAuthorityBoundStructureVideo(t *testing.T) {
	now := time.Date(2026, 9, 2, 4, 0, 0, 0, time.UTC)
	structure := newTemporalStructureFixture(t)
	root, challengeResult := structure.build(t, "suitability-structure")
	manifestPath := filepath.Join(root, "public", "manifest.json")
	authorityPath := filepath.Join(root, "private", "authority.json")
	manifest := readStrictTestJSON[TemporalStructureChallengeManifest](t, manifestPath)
	alias := manifest.Cases[0].Alias
	server := newTemporalSuitabilityServer(t, nil, `{"visualAssessment":"completed","spokenLanguageAssessment":"completed","flags":[]}`)
	defer server.Close()
	config := temporalSuitabilityTestConfig(manifestPath, filepath.Join(t.TempDir(), "private"), alias, server, now)
	config.StructureAuthorityPath = authorityPath
	result, err := RunOpenRouterTemporalSuitability(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	if result.EvidenceManifestSHA256 != challengeResult.PublicManifestSHA256 || result.Requests != 1 || len(result.Assessments) != 1 || result.Assessments[0].EvidenceAlias != alias || result.Assessments[0].Outcome != SuitabilityOutcomeNoSignalObserved || result.ProductionAdmissionAllowed {
		t.Fatalf("structure suitability result = %+v", result)
	}
}

func TestLoadTemporalSuitabilityEvidenceProjectsOnlyThePaidTransportSurface(t *testing.T) {
	structure := newTemporalStructureFixture(t)
	root, result := structure.build(t, "suitability-projection")
	manifestPath := filepath.Join(root, "public", "manifest.json")
	authorityPath := filepath.Join(root, "private", "authority.json")
	projected, digest, err := loadTemporalSuitabilityEvidence(manifestPath, authorityPath)
	if err != nil {
		t.Fatal(err)
	}
	if digest != result.PublicManifestSHA256 || len(projected.Cases) != 3 || !reviewSHA256(projected.SelectionSHA256) {
		t.Fatalf("projected identity = %+v digest=%s", projected, digest)
	}
	for _, item := range projected.Cases {
		if item.Alias == "" || item.DurationMS <= 0 || item.Video.Path == "" || len(item.Frames) != 0 || len(item.TranscriptSegments) != 0 {
			t.Fatalf("projection added or lost evidence: %+v", item)
		}
	}
}

func TestLoadTemporalSuitabilityEvidenceRejectsAuthorityModeDrift(t *testing.T) {
	structure := newTemporalStructureFixture(t)
	root, _ := structure.build(t, "suitability-mode")
	manifestPath := filepath.Join(root, "public", "manifest.json")
	authorityPath := filepath.Join(root, "private", "authority.json")
	if _, _, err := loadTemporalSuitabilityEvidence(manifestPath, ""); err == nil || !strings.Contains(err.Error(), "requires private construction authority") {
		t.Fatalf("missing structure authority error = %v", err)
	}

	truth := newTemporalHumanReviewFixture(t)
	if _, _, err := loadTemporalSuitabilityEvidence(truth.manifest, authorityPath); err == nil || !strings.Contains(err.Error(), "cannot carry structure authority") {
		t.Fatalf("extra structure authority error = %v", err)
	}

	unsupported := filepath.Join(t.TempDir(), "unsupported.json")
	if err := os.WriteFile(unsupported, []byte(`{"contractVersion":"future-v9"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadTemporalSuitabilityEvidence(unsupported, ""); err == nil || !strings.Contains(err.Error(), "unsupported suitability evidence contract") {
		t.Fatalf("unsupported contract error = %v", err)
	}
}
