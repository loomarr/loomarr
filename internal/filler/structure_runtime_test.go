package filler

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

type capturedStructureAssessor struct {
	profile   fillerstructure.AssessorProfile
	candidate fillerstructure.Candidate
	err       error
	order     *[]string
}

func (a *capturedStructureAssessor) Profile() fillerstructure.AssessorProfile { return a.profile }

func (a *capturedStructureAssessor) AssessCompleteTimeline(_ context.Context, media StructureAssessmentMedia) (fillerstructure.Candidate, error) {
	*a.order = append(*a.order, a.profile.ID+":"+media.Source.SHA256+":"+media.FullPath)
	return a.candidate, a.err
}

func TestStructureAssessmentRuntimeCallsIndependentAssessorsSerially(t *testing.T) {
	source := structureSource(10_000)
	media := StructureAssessmentMedia{Source: source, FullPath: "/tmp/conditioned-source.mp4"}
	order := []string{}
	assessors := runtimeAssessorFixtures(source, &order)
	runtime, err := NewStructureAssessmentRuntime(assessors, 2_000, func() time.Time {
		return time.Date(2026, time.September, 9, 1, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := runtime.Assess(t.Context(), media)
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{
		"assessor-a:" + source.SHA256 + ":" + media.FullPath,
		"assessor-b:" + source.SHA256 + ":" + media.FullPath,
	}
	if !slices.Equal(order, wantOrder) || artifact.Decision.Status != fillerstructure.StatusConfirmed || len(artifact.Decision.Candidates) != 2 {
		t.Fatalf("order=%v artifact=%+v", order, artifact)
	}
}

func TestStructureAssessmentRuntimeRejectsMissingOrDriftedAuthority(t *testing.T) {
	source := structureSource(10_000)
	order := []string{}
	tests := []struct {
		name   string
		mutate func([]CompleteTimelineStructureAssessor)
	}{
		{name: "adapter error", mutate: func(items []CompleteTimelineStructureAssessor) {
			items[1].(*capturedStructureAssessor).err = errors.New("response was not persisted")
		}},
		{name: "profile drift", mutate: func(items []CompleteTimelineStructureAssessor) {
			items[1].(*capturedStructureAssessor).candidate.Assessor.Model = "drifted"
		}},
		{name: "source drift", mutate: func(items []CompleteTimelineStructureAssessor) {
			items[1].(*capturedStructureAssessor).candidate.Source.SHA256 = strings.Repeat("f", 64)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			items := runtimeAssessorFixtures(source, &order)
			test.mutate(items)
			runtime, err := NewStructureAssessmentRuntime(items, 2_000, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.Assess(t.Context(), StructureAssessmentMedia{Source: source, FullPath: "/tmp/source.mp4"}); err == nil {
				t.Fatal("invalid adapter result was reduced")
			}
		})
	}
}

func TestStructureAssessmentRuntimeRejectsNonIndependentConfigurationBeforeCalls(t *testing.T) {
	source := structureSource(10_000)
	order := []string{}
	assessors := runtimeAssessorFixtures(source, &order)
	second := assessors[1].(*capturedStructureAssessor)
	second.profile.ModelFamily = "family-a"
	second.candidate.Assessor.ModelFamily = "family-a"
	if _, err := NewStructureAssessmentRuntime(assessors, 2_000, time.Now); err == nil || len(order) != 0 {
		t.Fatalf("non-independent runtime error=%v calls=%v", err, order)
	}
}

func runtimeAssessorFixtures(source SplitSourceAsset, order *[]string) []CompleteTimelineStructureAssessor {
	coreSource := fillerstructure.Source{SHA256: source.SHA256, DurationMS: source.DurationMs}
	build := func(id, family, assessmentDigest string) *capturedStructureAssessor {
		assessor := fillerstructure.Assessor{
			ID: id, ModelFamily: family, Provider: "captured", Model: "video-model",
			ModelDigest: strings.Repeat("b", 64), CapabilitySHA256: strings.Repeat("c", 64),
			PromptVersion: "prompt-v1", EvidenceContract: "assessment-v1",
			AssessmentSHA256: strings.Repeat(assessmentDigest, 64),
		}
		candidate := fillerstructure.Candidate{
			Source: coreSource, Assessor: assessor, Unit: fillerstructure.UnitCompilation,
			Segments: []fillerstructure.Segment{
				{StartMS: 0, EndMS: 5_000, Role: fillerstructure.RoleCommercial},
				{StartMS: 5_000, EndMS: 10_000, Role: fillerstructure.RolePromo},
			},
		}
		return &capturedStructureAssessor{profile: fillerstructure.Profile(assessor), candidate: candidate, order: order}
	}
	return []CompleteTimelineStructureAssessor{build("assessor-a", "family-a", "1"), build("assessor-b", "family-b", "2")}
}
