package filler

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSegmentScreeningCertificationReplaysEveryAxisAndRawEvidence(t *testing.T) {
	subject := screeningSubjectFixture(t)
	aggregate, certification, repository, records := screeningCertificationFixture(t, subject, true)
	if err := certification.Verify(t.Context(), aggregate); err != nil {
		t.Fatal(err)
	}
	for _, recorded := range records {
		loaded, err := repository.GetSegmentScreeningAxisEvidence(t.Context(), recorded.Evidence.SHA256)
		if err != nil || !slices.Equal(loaded.RawEvidence, recorded.RawEvidence) {
			t.Fatalf("loaded=%+v error=%v", loaded, err)
		}
	}
}

func TestSegmentScreeningCertificationFailsClosedOnReleaseAndEvidenceDrift(t *testing.T) {
	subject := screeningSubjectFixture(t)
	t.Run("production permission", func(t *testing.T) {
		aggregate, certification, _, _ := screeningCertificationFixture(t, subject, false)
		if err := certification.Verify(t.Context(), aggregate); err == nil {
			t.Fatal("non-authorizing release passed")
		}
	})
	t.Run("subject", func(t *testing.T) {
		aggregate, certification, _, _ := screeningCertificationFixture(t, subject, true)
		aggregate.SubjectSHA256 = strings.Repeat("f", 64)
		aggregate.SHA256 = SegmentScreeningEvidenceSHA256(aggregate)
		if err := certification.Verify(t.Context(), aggregate); err == nil {
			t.Fatal("subject drift passed")
		}
	})
	t.Run("axis projection", func(t *testing.T) {
		aggregate, certification, _, _ := screeningCertificationFixture(t, subject, true)
		aggregate.Results[0].ReasonCode = "different_reason"
		aggregate.SHA256 = SegmentScreeningEvidenceSHA256(aggregate)
		if err := certification.Verify(t.Context(), aggregate); err == nil {
			t.Fatal("axis projection drift passed")
		}
	})
	t.Run("raw evidence", func(t *testing.T) {
		aggregate, certification, repository, records := screeningCertificationFixture(t, subject, true)
		first := records[0].Evidence
		if err := os.WriteFile(repository.axisPath("screening-axis-raw", first.RawEvidenceSHA256), []byte("tampered"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := certification.Verify(t.Context(), aggregate); err == nil {
			t.Fatal("tampered raw evidence passed")
		}
	})
	t.Run("aggregate evidence", func(t *testing.T) {
		aggregate, certification, repository, _ := screeningCertificationFixture(t, subject, true)
		if err := os.Remove(repository.path(aggregate.SHA256)); err != nil {
			t.Fatal(err)
		}
		if err := certification.Verify(t.Context(), aggregate); err == nil {
			t.Fatal("missing aggregate evidence passed")
		}
	})
	t.Run("settled operation", func(t *testing.T) {
		aggregate, certification, repository, records := screeningCertificationFixture(t, subject, true)
		operationSHA256 := segmentScreeningOperationSHA256(subject.SHA256, records[0].Evidence.Profile)
		if err := os.Remove(repository.operationPath(operationSHA256)); err != nil {
			t.Fatal(err)
		}
		if err := certification.Verify(t.Context(), aggregate); err == nil {
			t.Fatal("axis without its settled operation passed")
		}
	})
	t.Run("profile", func(t *testing.T) {
		aggregate, _, repository, records := screeningCertificationFixture(t, subject, true)
		profiles := screeningProfiles(records)
		profiles[0].PolicySHA256 = strings.Repeat("f", 64)
		release := screeningReleaseFixture(profiles, true)
		certification, err := NewSegmentScreeningCertification(release, repository)
		if err != nil {
			t.Fatal(err)
		}
		if err := certification.Verify(t.Context(), aggregate); err == nil {
			t.Fatal("profile drift passed")
		}
	})
}

func screeningCertificationFixture(t *testing.T, subject SegmentScreeningSubject, production bool) (SegmentScreeningEvidence, *SegmentScreeningCertification, *FileSegmentScreeningEvidenceRepository, []RecordedSegmentScreeningAxisEvidence) {
	t.Helper()
	repository, err := NewFileSegmentScreeningEvidenceRepository(filepath.Join(t.TempDir(), "evidence"))
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.PutSegmentScreeningSubject(t.Context(), subject); err != nil {
		t.Fatal(err)
	}
	records := passingAxisEvidence(t, subject)
	results := make([]SegmentScreeningResult, 0, len(records))
	for _, recorded := range records {
		if err := repository.PutSegmentScreeningAxisEvidence(t.Context(), recorded); err != nil {
			t.Fatal(err)
		}
		results = append(results, recorded.Evidence.Result())
	}
	aggregate, err := NewSegmentScreeningEvidence(subject, results, time.Date(2026, time.September, 12, 7, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.PutSegmentScreeningEvidence(t.Context(), aggregate); err != nil {
		t.Fatal(err)
	}
	certification, err := NewSegmentScreeningCertification(screeningReleaseFixture(screeningProfiles(records), production), repository)
	if err != nil {
		t.Fatal(err)
	}
	return aggregate, certification, repository, records
}

func screeningProfiles(records []RecordedSegmentScreeningAxisEvidence) []SegmentScreeningAxisProfile {
	profiles := make([]SegmentScreeningAxisProfile, 0, len(records))
	for _, recorded := range records {
		profiles = append(profiles, recorded.Evidence.Profile)
	}
	canonicalizeSegmentScreeningProfiles(profiles)
	return profiles
}

func screeningReleaseFixture(profiles []SegmentScreeningAxisProfile, production bool) SegmentScreeningReleaseAuthority {
	release := SegmentScreeningReleaseAuthority{
		SchemaVersion: SegmentScreeningReleaseSchemaVersion, ContractVersion: SegmentScreeningReleaseContractVersion,
		CertificateSHA256: strings.Repeat("e", 64), AggregateContractVersion: SegmentScreeningContractVersion,
		Profiles: append([]SegmentScreeningAxisProfile(nil), profiles...), ProductionAdmissionAllowed: production,
	}
	release.SHA256 = SegmentScreeningReleaseAuthoritySHA256(release)
	return release
}
