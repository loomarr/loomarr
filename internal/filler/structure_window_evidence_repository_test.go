package filler

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
	"github.com/loomarr/loomarr/internal/fillerstructurewindow"
)

func TestFileStructureWindowEvidenceRepositoryRoundTripsValidatedArtifacts(t *testing.T) {
	_, prepared := structureWindowRuntimeFixture(t)
	set := prepared.Authority
	timeline := []fillerstructure.Segment{
		{StartMS: 0, EndMS: 120_000, Role: fillerstructure.RoleCommercial},
		{StartMS: 120_000, EndMS: 300_000, Role: fillerstructure.RolePromo},
	}
	profile := windowAssessorFixture("assessor-a", "family-a", "a", timeline, &[]string{}).Profile()
	assessments := structureWindowAssessmentFixtures(t, set, profile, timeline)
	stitch, err := fillerstructurewindow.Stitch(set, assessments, 2_000)
	if err != nil {
		t.Fatal(err)
	}
	repository := structureEvidenceRepositoryFixture(t)
	for _, assessment := range assessments {
		if err := repository.PutStructureWindowAssessment(t.Context(), set, assessment); err != nil {
			t.Fatal(err)
		}
		if err := repository.PutStructureWindowAssessment(t.Context(), set, assessment); err != nil {
			t.Fatalf("idempotent assessment persistence: %v", err)
		}
		loaded, err := repository.GetStructureWindowAssessment(t.Context(), set, assessment.SHA256)
		if err != nil || !reflect.DeepEqual(loaded, assessment) {
			t.Fatalf("loaded assessment=%+v error=%v", loaded, err)
		}
	}
	if err := repository.PutStructureWindowStitch(t.Context(), stitch); err != nil {
		t.Fatal(err)
	}
	if err := repository.PutStructureWindowStitch(t.Context(), stitch); err != nil {
		t.Fatalf("idempotent stitch persistence: %v", err)
	}
	loaded, err := repository.GetStructureWindowStitch(t.Context(), stitch.SHA256)
	if err != nil || !reflect.DeepEqual(loaded, stitch) {
		t.Fatalf("loaded stitch=%+v error=%v", loaded, err)
	}
}

func TestFileStructureWindowEvidenceRepositoryRejectsDrift(t *testing.T) {
	_, prepared := structureWindowRuntimeFixture(t)
	set := prepared.Authority
	timeline := []fillerstructure.Segment{{StartMS: 0, EndMS: 300_000, Role: fillerstructure.RoleCommercial}}
	profile := windowAssessorFixture("assessor-a", "family-a", "a", timeline, &[]string{}).Profile()
	assessments := structureWindowAssessmentFixtures(t, set, profile, timeline)
	stitch, err := fillerstructurewindow.Stitch(set, assessments, 2_000)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"assessment", "stitch"} {
		t.Run(target, func(t *testing.T) {
			repository := structureEvidenceRepositoryFixture(t)
			var path string
			if target == "assessment" {
				if err := repository.PutStructureWindowAssessment(t.Context(), set, assessments[0]); err != nil {
					t.Fatal(err)
				}
				path = repository.blobPath("window-assessments", assessments[0].SHA256)
			} else {
				if err := repository.PutStructureWindowStitch(t.Context(), stitch); err != nil {
					t.Fatal(err)
				}
				path = repository.blobPath("window-stitches", stitch.SHA256)
			}
			if err := os.WriteFile(path, []byte(`{"tampered":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if target == "assessment" {
				if _, err := repository.GetStructureWindowAssessment(t.Context(), set, assessments[0].SHA256); err == nil {
					t.Fatal("tampered assessment was accepted")
				}
			} else if _, err := repository.GetStructureWindowStitch(t.Context(), stitch.SHA256); err == nil {
				t.Fatal("tampered stitch was accepted")
			}
		})
	}
}

func structureWindowAssessmentFixtures(t *testing.T, set fillerstructurewindow.MediaSet, profile fillerstructure.AssessorProfile, timeline []fillerstructure.Segment) []fillerstructurewindow.Assessment {
	t.Helper()
	assessments := make([]fillerstructurewindow.Assessment, len(set.Plan.Windows))
	for ordinal, window := range set.Plan.Windows {
		assessment, err := fillerstructurewindow.NewAssessment(fillerstructurewindow.AssessmentInput{
			MediaSet: set, WindowOrdinal: ordinal, Assessor: profile,
			Segments:   clipStructureTimeline(timeline, window),
			AssessedAt: time.Date(2026, time.September, 11, 11, 0, ordinal, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		assessments[ordinal] = assessment
	}
	return assessments
}
