package fillerstructurewindow

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

func TestWindowAssessmentRequiresCompleteSourceRelativeCoverage(t *testing.T) {
	plan, err := NewPlan(sourceFixture(300_000))
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := NewAssessment(assessmentInputFixture(plan, 1, []fillerstructure.Segment{
		{StartMS: 105_000, EndMS: 120_000, Role: fillerstructure.RoleCommercial},
		{StartMS: 120_000, EndMS: 180_000, Role: fillerstructure.RolePromo},
		{StartMS: 180_000, EndMS: 255_000, Role: fillerstructure.RoleCommercial},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if assessment.State != AssessmentAccepted || assessment.WindowOrdinal != 1 || assessment.SHA256 != AssessmentSHA256(assessment) {
		t.Fatalf("assessment = %+v", assessment)
	}
}

func TestWindowAssessmentRetainsOperationalFailureWithoutSemanticClaim(t *testing.T) {
	plan, err := NewPlan(sourceFixture(300_000))
	if err != nil {
		t.Fatal(err)
	}
	input := assessmentInputFixture(plan, 1, nil)
	input.Failure = "provider_timeout"
	assessment, err := NewAssessment(input)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.State != AssessmentOperationalFailure || assessment.Failure != input.Failure || len(assessment.Segments) != 0 {
		t.Fatalf("failure assessment = %+v", assessment)
	}
}

func TestWindowAssessmentRejectsDriftAndIncompleteAnswers(t *testing.T) {
	plan, err := NewPlan(sourceFixture(300_000))
	if err != nil {
		t.Fatal(err)
	}
	valid, err := NewAssessment(assessmentInputFixture(plan, 1, []fillerstructure.Segment{
		{StartMS: 105_000, EndMS: 255_000, Role: fillerstructure.RoleCommercial},
	}))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Assessment)
	}{
		{name: "plan", mutate: func(value *Assessment) { value.PlanSHA256 = strings.Repeat("f", 64) }},
		{name: "source", mutate: func(value *Assessment) { value.Source.DurationMS++ }},
		{name: "ordinal", mutate: func(value *Assessment) { value.WindowOrdinal++ }},
		{name: "media profile", mutate: func(value *Assessment) { value.Media.ProfileSHA256 = strings.Repeat("e", 64) }},
		{name: "media lineage", mutate: func(value *Assessment) { value.Media.LineageSHA256 = "" }},
		{name: "media duration", mutate: func(value *Assessment) { value.Media.DurationMS += 1_001 }},
		{name: "coverage start", mutate: func(value *Assessment) { value.Segments[0].StartMS++ }},
		{name: "coverage end", mutate: func(value *Assessment) { value.Segments[0].EndMS-- }},
		{name: "role", mutate: func(value *Assessment) { value.Segments[0].Role = "invented" }},
		{name: "time", mutate: func(value *Assessment) { value.AssessedAt = value.AssessedAt.In(time.FixedZone("other", 3600)) }},
		{name: "digest", mutate: func(value *Assessment) { value.SHA256 = strings.Repeat("d", 64) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assessment := valid
			assessment.Segments = append([]fillerstructure.Segment(nil), valid.Segments...)
			test.mutate(&assessment)
			if err := ValidateAssessment(plan, assessment); err == nil {
				t.Fatal("drifted assessment was accepted")
			}
		})
	}
}

func assessmentInputFixture(plan Plan, ordinal int, segments []fillerstructure.Segment) AssessmentInput {
	window := plan.Windows[ordinal]
	return AssessmentInput{
		Plan: plan, WindowOrdinal: ordinal,
		Media: fillerstructure.AssessmentMedia{
			SHA256: strings.Repeat("b", 64), Bytes: 1_024,
			DurationMS:    window.MediaEndMS - window.MediaStartMS,
			ProfileSHA256: plan.Profile.AssessmentMediaProfileSHA256,
			LineageSHA256: strings.Repeat("c", 64),
		},
		Assessor: fillerstructure.AssessorProfile{
			ID: "assessor-a", ModelFamily: "family-a", Provider: "provider-a", Model: "model-a",
			ModelDigest: strings.Repeat("d", 64), CapabilitySHA256: strings.Repeat("e", 64),
			PromptVersion: "prompt-v1", EvidenceContract: "window-v1",
		},
		Segments: segments, AssessedAt: time.Date(2026, 9, 3, 19, 0, ordinal, 0, time.UTC),
	}
}
