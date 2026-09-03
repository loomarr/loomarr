package filler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStructureScreeningEvidenceRepositoryRoundTripsAndRejectsTampering(t *testing.T) {
	repository, err := NewFileStructureScreeningEvidenceRepository(filepath.Join(t.TempDir(), "screening-evidence"))
	if err != nil {
		t.Fatal(err)
	}
	evidence := segmentScreeningFixture(t)
	if err := repository.PutSegmentScreeningEvidence(t.Context(), evidence); err != nil {
		t.Fatal(err)
	}
	if err := repository.PutSegmentScreeningEvidence(t.Context(), evidence); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	replayed, err := repository.GetSegmentScreeningEvidence(t.Context(), evidence.SHA256)
	if err != nil || replayed.SHA256 != evidence.SHA256 || !replayed.Passes() {
		t.Fatalf("replayed=%+v error=%v", replayed, err)
	}
	if err := os.WriteFile(repository.path(evidence.SHA256), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetSegmentScreeningEvidence(t.Context(), evidence.SHA256); err == nil {
		t.Fatal("tampered screening evidence was accepted")
	}
}

func TestFileStructureScreeningEvidenceRepositoryRejectsInvalidIdentity(t *testing.T) {
	repository, err := NewFileStructureScreeningEvidenceRepository(filepath.Join(t.TempDir(), "screening-evidence"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetSegmentScreeningEvidence(t.Context(), "not-a-digest"); err == nil {
		t.Fatal("invalid screening identity was accepted")
	}
	evidence := segmentScreeningFixture(t)
	evidence.Results[0].AuthoritySHA256 = ""
	if err := repository.PutSegmentScreeningEvidence(t.Context(), evidence); err == nil {
		t.Fatal("invalid screening evidence was stored")
	}
}
