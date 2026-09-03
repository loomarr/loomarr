package filler

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
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

func structureEvidenceRepositoryFixture(t *testing.T) *FileStructureAssessmentEvidenceRepository {
	t.Helper()
	repository, err := NewFileStructureAssessmentEvidenceRepository(filepath.Join(t.TempDir(), "evidence"))
	if err != nil {
		t.Fatal(err)
	}
	return repository
}
