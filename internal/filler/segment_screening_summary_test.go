package filler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSegmentScreeningSummaryReproducesBrowserSafeFiveAxisEvidence(t *testing.T) {
	service, mediaPath, subject, aggregate := segmentScreeningSummaryFixture(t, true)
	summary, err := service.ReadSegmentScreeningSummary(t.Context(), subject.CatalogHash, mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateSegmentScreeningSummary(summary) != nil || summary.State != ScreeningSummaryAvailable ||
		summary.SubjectSHA256 != subject.SHA256 || summary.EvidenceSHA256 != aggregate.SHA256 ||
		summary.Outcome != ScreenPass || summary.Airworthiness == nil ||
		!reflect.DeepEqual(summary.Airworthiness, &aggregate.Airworthiness) {
		t.Fatalf("summary = %+v", summary)
	}
	wantAxes := []SegmentScreeningAxis{ScreenVisualSafety, ScreenSpokenSafety, ScreenWrittenSafety, ScreenRights, ScreenPlayback}
	for index, axis := range summary.Axes {
		if axis.Axis != wantAxes[index] || axis.EvidenceSHA256 == "" || axis.ReasonCode == "" {
			t.Fatalf("axis %d = %+v", index, axis)
		}
	}
}

func TestSegmentScreeningSummaryDistinguishesNotScreenedFromUnavailable(t *testing.T) {
	t.Run("not screened", func(t *testing.T) {
		service, mediaPath, subject, _ := segmentScreeningSummaryFixture(t, false)
		summary, err := service.ReadSegmentScreeningSummary(t.Context(), subject.CatalogHash, mediaPath)
		if err != nil || summary.State != ScreeningSummaryNotScreened ||
			summary.ReasonCode != ScreeningSummaryReasonNotAttached || ValidateSegmentScreeningSummary(summary) != nil {
			t.Fatalf("summary=%+v err=%v", summary, err)
		}
	})

	t.Run("attached evidence missing", func(t *testing.T) {
		service, mediaPath, subject, aggregate := segmentScreeningSummaryFixture(t, true)
		missing, err := NewFileSegmentScreeningEvidenceRepository(filepath.Join(t.TempDir(), "missing"))
		if err != nil {
			t.Fatal(err)
		}
		service, err = NewSegmentScreeningSummaryService(missing)
		if err != nil {
			t.Fatal(err)
		}
		summary, readErr := service.ReadSegmentScreeningSummary(t.Context(), subject.CatalogHash, mediaPath)
		if readErr == nil || summary.State != ScreeningSummaryUnavailable ||
			summary.ReasonCode != ScreeningSummaryReasonEvidenceUnavailable ||
			summary.SubjectSHA256 != subject.SHA256 || summary.EvidenceSHA256 != aggregate.SHA256 ||
			ValidateSegmentScreeningSummary(summary) != nil {
			t.Fatalf("summary=%+v err=%v", summary, readErr)
		}
	})

	t.Run("current clip identity drift", func(t *testing.T) {
		service, mediaPath, _, _ := segmentScreeningSummaryFixture(t, true)
		summary, err := service.ReadSegmentScreeningSummary(t.Context(), strings.Repeat("9", 64), mediaPath)
		if err == nil || summary.State != ScreeningSummaryUnavailable ||
			summary.ReasonCode != ScreeningSummaryReasonEvidenceDrift || ValidateSegmentScreeningSummary(summary) != nil {
			t.Fatalf("summary=%+v err=%v", summary, err)
		}
	})

	t.Run("current playback bytes drift", func(t *testing.T) {
		service, mediaPath, subject, aggregate := segmentScreeningSummaryFixture(t, true)
		service.inspect = func(context.Context, string, string, int64, string) (segmentScreeningArtifactObservation, bool, error) {
			return segmentScreeningArtifactObservation{State: "observed", SHA256: strings.Repeat("f", 64)}, false, nil
		}
		summary, err := service.ReadSegmentScreeningSummary(t.Context(), subject.CatalogHash, mediaPath)
		if err == nil || summary.State != ScreeningSummaryUnavailable ||
			summary.ReasonCode != ScreeningSummaryReasonEvidenceDrift ||
			summary.SubjectSHA256 != subject.SHA256 || summary.EvidenceSHA256 != aggregate.SHA256 ||
			ValidateSegmentScreeningSummary(summary) != nil {
			t.Fatalf("summary=%+v err=%v", summary, err)
		}
	})

	t.Run("current playback cannot be inspected", func(t *testing.T) {
		service, mediaPath, subject, aggregate := segmentScreeningSummaryFixture(t, true)
		service.inspect = func(context.Context, string, string, int64, string) (segmentScreeningArtifactObservation, bool, error) {
			return segmentScreeningArtifactObservation{}, false, errors.New("read denied")
		}
		summary, err := service.ReadSegmentScreeningSummary(t.Context(), subject.CatalogHash, mediaPath)
		if err == nil || summary.State != ScreeningSummaryUnavailable ||
			summary.ReasonCode != ScreeningSummaryReasonEvidenceUnavailable ||
			summary.SubjectSHA256 != subject.SHA256 || summary.EvidenceSHA256 != aggregate.SHA256 ||
			ValidateSegmentScreeningSummary(summary) != nil {
			t.Fatalf("summary=%+v err=%v", summary, err)
		}
	})
}

func TestSegmentScreeningSummaryRejectsAnOverallOutcomeThatDisagreesWithItsAxes(t *testing.T) {
	service, mediaPath, subject, _ := segmentScreeningSummaryFixture(t, true)
	summary, err := service.ReadSegmentScreeningSummary(t.Context(), subject.CatalogHash, mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	summary.Axes[0].Outcome = ScreenHold
	if err := ValidateSegmentScreeningSummary(summary); err == nil {
		t.Fatal("summary accepted overall pass with a held visual axis")
	}
}

func segmentScreeningSummaryFixture(t *testing.T, attach bool) (*SegmentScreeningSummaryService, string, SegmentScreeningSubject, SegmentScreeningEvidence) {
	t.Helper()
	repository, err := NewFileSegmentScreeningEvidenceRepository(filepath.Join(t.TempDir(), "evidence"))
	if err != nil {
		t.Fatal(err)
	}
	tags := screeningChildTagsFixture(t)
	subject, err := NewSegmentScreeningSubject(tags.MediaAssets.Playback.Asset.ClipHash, tags)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := screeningAggregateFixture(t, subject)
	if attach {
		if err := repository.PutSegmentScreeningSubject(t.Context(), subject); err != nil {
			t.Fatal(err)
		}
		if err := repository.PutSegmentScreeningEvidence(t.Context(), aggregate); err != nil {
			t.Fatal(err)
		}
		reference, err := NewSegmentScreeningReference(subject, aggregate)
		if err != nil {
			t.Fatal(err)
		}
		tags.SegmentScreening = &reference
	}
	mediaPath := filepath.Join(t.TempDir(), "child.mp4")
	if err := os.WriteFile(mediaPath, []byte("screened child"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSidecarTags(mediaPath, tags, false); err != nil {
		t.Fatal(err)
	}
	service, err := NewSegmentScreeningSummaryService(repository)
	if err != nil {
		t.Fatal(err)
	}
	service.inspect = func(_ context.Context, path, expectedSHA256 string, expectedBytes int64, expectedClipHash string) (segmentScreeningArtifactObservation, bool, error) {
		if path != mediaPath || expectedSHA256 != subject.PlaybackSHA256 || expectedBytes != subject.PlaybackBytes || expectedClipHash != subject.CatalogHash {
			return segmentScreeningArtifactObservation{}, false, errors.New("unexpected playback identity")
		}
		return segmentScreeningArtifactObservation{
			State: "observed", SHA256: expectedSHA256, Bytes: expectedBytes, ClipHash: expectedClipHash,
		}, true, nil
	}
	return service, mediaPath, subject, aggregate
}
