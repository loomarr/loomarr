package fillerstructureassessment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

func TestFileStructureAssessmentEvidenceRepositoryRejectsSymlinkedRoot(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "evidence")
	if err := os.Symlink(target, root); err != nil {
		t.Fatal(err)
	}
	repository, err := NewFileStructureAssessmentEvidenceRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.PutStructureDecisionArtifact(t.Context(), structureDecisionArtifactFixture(t)); err == nil {
		t.Fatal("symlinked evidence root was accepted")
	}
}

func TestFileStructureAssessmentEvidenceRepositoryRejectsMissingOrDriftedDecision(t *testing.T) {
	repository := structureEvidenceRepositoryFixture(t)
	if _, err := repository.GetStructureDecisionArtifact(t.Context(), strings.Repeat("a", 64)); err == nil {
		t.Fatal("missing decision was accepted")
	}
	artifact := structureDecisionArtifactFixture(t)
	if err := repository.PutStructureDecisionArtifact(t.Context(), artifact); err != nil {
		t.Fatal(err)
	}
	path := repository.blobPath("decisions", artifact.SHA256)
	if err := os.WriteFile(path, []byte(`{"sha256":"drifted"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetStructureDecisionArtifact(t.Context(), artifact.SHA256); err == nil {
		t.Fatal("drifted decision was accepted")
	}
}

func structureDecisionArtifactFixture(t *testing.T) fillerstructure.Artifact {
	t.Helper()
	source, input := structureDecisionRepositoryInput(t)
	artifact, err := fillerstructure.NewArtifact(fillerstructure.Request{
		Source: source, Input: input,
		BoundaryToleranceMS: 2_000,
		Candidates: []fillerstructure.Candidate{
			structureDecisionRepositoryCandidate(source, input.SHA256, "a", "family-a", "2"),
			structureDecisionRepositoryCandidate(source, input.SHA256, "b", "family-b", "3"),
		},
	}, time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func structureDecisionRepositoryCandidate(source fillerstructure.Source, inputSHA256, id, family, assessment string) fillerstructure.Candidate {
	return fillerstructure.Candidate{
		Source: source, InputSHA256: inputSHA256,
		Assessor: fillerstructure.Assessor{
			ID: "assessor-" + id, ModelFamily: family, Provider: "captured", Model: "model-" + id,
			ModelDigest: strings.Repeat("4", 64), CapabilitySHA256: strings.Repeat("5", 64),
			PromptVersion: "prompt-v1", EvidenceContract: "assessment-v1", AssessmentSHA256: strings.Repeat(assessment, 64),
		},
		Unit: fillerstructure.UnitStandalone, Role: fillerstructure.RoleCommercial,
		Segments: []fillerstructure.Segment{{StartMS: 0, EndMS: 10_000, Role: fillerstructure.RoleCommercial}},
	}
}

func structureDecisionRepositoryInput(t *testing.T) (fillerstructure.Source, fillerstructure.AssessmentInput) {
	t.Helper()
	source := fillerstructure.Source{SHA256: strings.Repeat("1", 64), Bytes: 2_048, DurationMS: 10_000}
	media := fillerstructure.AssessmentMedia{SHA256: strings.Repeat("6", 64), Bytes: 1_024, DurationMS: 10_000, ProfileSHA256: strings.Repeat("7", 64), LineageSHA256: strings.Repeat("8", 64)}
	input, err := fillerstructure.NewCompleteVideoInput(source, media)
	if err != nil {
		t.Fatal(err)
	}
	return source, input
}

func structureEvidenceRepositoryFixture(t *testing.T) *FileStructureAssessmentEvidenceRepository {
	t.Helper()
	repository, err := NewFileStructureAssessmentEvidenceRepository(filepath.Join(t.TempDir(), "evidence"))
	if err != nil {
		t.Fatal(err)
	}
	return repository
}
