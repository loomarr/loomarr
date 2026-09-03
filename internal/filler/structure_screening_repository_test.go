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

func TestFileStructureScreeningEvidenceRepositoryPublishesAxisRawBytesBeforeRecord(t *testing.T) {
	repository, err := NewFileStructureScreeningEvidenceRepository(filepath.Join(t.TempDir(), "screening-evidence"))
	if err != nil {
		t.Fatal(err)
	}
	recorded := passingAxisEvidence(t, structureSource(30_000), 0, 30_000)[0]
	if err := repository.PutSegmentScreeningAxisEvidence(t.Context(), recorded); err != nil {
		t.Fatal(err)
	}
	if err := repository.PutSegmentScreeningAxisEvidence(t.Context(), recorded); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	for _, path := range []string{
		repository.axisPath("screening-axis-raw", recorded.Evidence.RawEvidenceSHA256),
		repository.axisPath("screening-axis-records", recorded.Evidence.SHA256),
	} {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("axis evidence path %s info=%v error=%v", path, info, statErr)
		}
	}

	conflicting, err := NewFileStructureScreeningEvidenceRepository(filepath.Join(t.TempDir(), "screening-evidence"))
	if err != nil {
		t.Fatal(err)
	}
	rawPath := conflicting.axisPath("screening-axis-raw", recorded.Evidence.RawEvidenceSHA256)
	if err := os.MkdirAll(filepath.Dir(rawPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rawPath, []byte("different"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := conflicting.PutSegmentScreeningAxisEvidence(t.Context(), recorded); err == nil {
		t.Fatal("conflicting raw evidence was accepted")
	}
	if _, err := os.Lstat(conflicting.axisPath("screening-axis-records", recorded.Evidence.SHA256)); !os.IsNotExist(err) {
		t.Fatalf("axis record published after raw conflict: %v", err)
	}
}
