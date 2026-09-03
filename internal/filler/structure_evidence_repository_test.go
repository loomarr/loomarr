package filler

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

func TestFileStructureAssessmentEvidenceRepositoryRoundTripsAndReplays(t *testing.T) {
	repository := structureEvidenceRepositoryFixture(t)
	recorded := runtimeAssessorFixtures(structureSource(10_000), &[]string{})[0].(*capturedStructureAssessor).recorded
	if err := repository.PutStructureAssessmentEvidence(t.Context(), recorded); err != nil {
		t.Fatal(err)
	}
	if err := repository.PutStructureAssessmentEvidence(t.Context(), recorded); err != nil {
		t.Fatalf("idempotent put: %v", err)
	}
	loaded, err := repository.GetStructureAssessmentEvidence(t.Context(), recorded.Record.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded.RawResponse, recorded.RawResponse) || loaded.StructuredOutput != recorded.StructuredOutput || loaded.Record.SHA256 != recorded.Record.SHA256 {
		t.Fatalf("loaded=%+v", loaded)
	}
	for _, path := range []string{
		repository.blobPath("records", recorded.Record.SHA256),
		repository.blobPath("responses", recorded.Record.ResponseSHA256),
		repository.blobPath("outputs", recorded.Record.StructuredOutputSHA256),
	} {
		info, statErr := os.Lstat(path)
		if statErr != nil {
			t.Fatalf("evidence path %s: %v", path, statErr)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("evidence path %s mode=%v", path, info.Mode())
		}
	}
}

func TestFileStructureAssessmentEvidenceRepositoryRejectsConflictingOrMissingBlobs(t *testing.T) {
	recorded := runtimeAssessorFixtures(structureSource(10_000), &[]string{})[0].(*capturedStructureAssessor).recorded
	t.Run("conflicting output prevents record publication", func(t *testing.T) {
		repository := structureEvidenceRepositoryFixture(t)
		path := repository.blobPath("outputs", recorded.Record.StructuredOutputSHA256)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("different"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := repository.PutStructureAssessmentEvidence(t.Context(), recorded); err == nil {
			t.Fatal("conflicting evidence was accepted")
		}
		if _, err := os.Lstat(repository.blobPath("records", recorded.Record.SHA256)); !os.IsNotExist(err) {
			t.Fatalf("record published after blob conflict: %v", err)
		}
	})
	t.Run("missing response invalidates replay", func(t *testing.T) {
		repository := structureEvidenceRepositoryFixture(t)
		if err := repository.PutStructureAssessmentEvidence(t.Context(), recorded); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(repository.blobPath("responses", recorded.Record.ResponseSHA256)); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.GetStructureAssessmentEvidence(t.Context(), recorded.Record.SHA256); err == nil {
			t.Fatal("record replayed without its raw response")
		}
	})
}

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
	recorded := runtimeAssessorFixtures(structureSource(10_000), &[]string{})[0].(*capturedStructureAssessor).recorded
	if err := repository.PutStructureAssessmentEvidence(t.Context(), recorded); err == nil {
		t.Fatal("symlinked evidence root was accepted")
	}
}

func TestFileStructureAssessmentEvidenceRepositoryRejectsMissingOrDriftedDecision(t *testing.T) {
	repository := structureEvidenceRepositoryFixture(t)
	if _, err := repository.GetStructureDecisionArtifact(t.Context(), strings.Repeat("a", 64)); err == nil {
		t.Fatal("missing decision was accepted")
	}
	artifact, err := fillerstructure.NewArtifact(fillerstructure.Request{
		Source:              fillerstructure.Source{SHA256: strings.Repeat("1", 64), Bytes: 2_048, DurationMS: 10_000},
		Media:               fillerstructure.AssessmentMedia{SHA256: strings.Repeat("6", 64), Bytes: 1_024, DurationMS: 10_000, ProfileSHA256: strings.Repeat("7", 64), LineageSHA256: strings.Repeat("8", 64)},
		BoundaryToleranceMS: 2_000,
		Candidates: []fillerstructure.Candidate{
			structureDecisionRepositoryCandidate("a", "family-a", "2"),
			structureDecisionRepositoryCandidate("b", "family-b", "3"),
		},
	}, time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
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

func structureDecisionRepositoryCandidate(id, family, assessment string) fillerstructure.Candidate {
	return fillerstructure.Candidate{
		Source: fillerstructure.Source{SHA256: strings.Repeat("1", 64), Bytes: 2_048, DurationMS: 10_000},
		Media:  fillerstructure.AssessmentMedia{SHA256: strings.Repeat("6", 64), Bytes: 1_024, DurationMS: 10_000, ProfileSHA256: strings.Repeat("7", 64), LineageSHA256: strings.Repeat("8", 64)},
		Assessor: fillerstructure.Assessor{
			ID: "assessor-" + id, ModelFamily: family, Provider: "captured", Model: "model-" + id,
			ModelDigest: strings.Repeat("4", 64), CapabilitySHA256: strings.Repeat("5", 64),
			PromptVersion: "prompt-v1", EvidenceContract: "assessment-v1", AssessmentSHA256: strings.Repeat(assessment, 64),
		},
		Unit: fillerstructure.UnitStandalone, Role: fillerstructure.RoleCommercial,
		Segments: []fillerstructure.Segment{{StartMS: 0, EndMS: 10_000, Role: fillerstructure.RoleCommercial}},
	}
}

func structureEvidenceRepositoryFixture(t *testing.T) *FileStructureAssessmentEvidenceRepository {
	t.Helper()
	repository, err := NewFileStructureAssessmentEvidenceRepository(filepath.Join(t.TempDir(), "evidence"))
	if err != nil {
		t.Fatal(err)
	}
	return repository
}
