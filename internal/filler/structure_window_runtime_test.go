package filler

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
	"github.com/loomarr/loomarr/internal/fillerstructurewindow"
)

type capturedWindowPreparer struct {
	prepared StructureAssessmentWindowMediaSet
	err      error
}

func (p *capturedWindowPreparer) PrepareWindows(_ context.Context, _ StructureAssessmentSource, _ fillerstructurewindow.Plan) (StructureAssessmentWindowMediaSet, error) {
	return p.prepared, p.err
}

type capturedWindowAssessor struct {
	profile        fillerstructure.AssessorProfile
	timeline       []fillerstructure.Segment
	events         *[]string
	failureOrdinal int
	driftProfile   bool
}

func (a *capturedWindowAssessor) Profile() fillerstructure.AssessorProfile { return a.profile }

func (a *capturedWindowAssessor) AssessWindow(_ context.Context, set fillerstructurewindow.MediaSet, media StructureAssessmentWindowMedia) (fillerstructurewindow.Assessment, error) {
	*a.events = append(*a.events, "call:"+a.profile.ID+":"+string(rune('0'+media.Window.Ordinal)))
	profile := a.profile
	if a.driftProfile {
		profile.ID += "-drift"
	}
	input := fillerstructurewindow.AssessmentInput{
		MediaSet: set, WindowOrdinal: media.Window.Ordinal, Assessor: profile,
		Segments:   clipStructureTimeline(a.timeline, media.Window),
		AssessedAt: time.Date(2026, time.September, 11, 10, 0, media.Window.Ordinal, 0, time.UTC),
	}
	if media.Window.Ordinal == a.failureOrdinal {
		input.Failure, input.Segments = "provider_timeout", nil
	}
	return fillerstructurewindow.NewAssessment(input)
}

type capturedWindowEvidence struct {
	events      *[]string
	failEvent   string
	assessments []fillerstructurewindow.Assessment
	stitches    []fillerstructurewindow.StitchResult
	decisions   []fillerstructure.Artifact
}

func (e *capturedWindowEvidence) PutStructureWindowAssessment(_ context.Context, _ fillerstructurewindow.MediaSet, assessment fillerstructurewindow.Assessment) error {
	event := "put:" + assessment.Assessor.ID + ":" + string(rune('0'+assessment.WindowOrdinal))
	*e.events = append(*e.events, event)
	if event == e.failEvent {
		return errors.New("persistence failed")
	}
	e.assessments = append(e.assessments, assessment)
	return nil
}

func (e *capturedWindowEvidence) PutStructureWindowStitch(_ context.Context, stitch fillerstructurewindow.StitchResult) error {
	event := "stitch:" + stitch.Assessor.ID
	*e.events = append(*e.events, event)
	if event == e.failEvent {
		return errors.New("persistence failed")
	}
	e.stitches = append(e.stitches, stitch)
	return nil
}

func (e *capturedWindowEvidence) PutStructureDecisionArtifact(_ context.Context, artifact fillerstructure.Artifact) error {
	*e.events = append(*e.events, "decision")
	if e.failEvent == "decision" {
		return errors.New("persistence failed")
	}
	e.decisions = append(e.decisions, artifact)
	return nil
}

func TestStructureWindowRuntimePersistsFamilyMajorSerialEvidenceBeforeReduction(t *testing.T) {
	input, prepared := structureWindowRuntimeFixture(t)
	events := []string{}
	timeline := []fillerstructure.Segment{
		{StartMS: 0, EndMS: 120_000, Role: fillerstructure.RoleCommercial},
		{StartMS: 120_000, EndMS: 300_000, Role: fillerstructure.RolePromo},
	}
	assessors := []CompleteWindowStructureAssessor{
		windowAssessorFixture("assessor-a", "family-a", "a", timeline, &events),
		windowAssessorFixture("assessor-b", "family-b", "b", timeline, &events),
	}
	evidence := &capturedWindowEvidence{events: &events}
	runtime, err := NewStructureWindowAssessmentRuntime(assessors, &capturedWindowPreparer{prepared: prepared}, evidence, 2_000, func() time.Time {
		return time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := runtime.Assess(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{
		"call:assessor-a:0", "put:assessor-a:0", "call:assessor-a:1", "put:assessor-a:1", "call:assessor-a:2", "put:assessor-a:2", "stitch:assessor-a",
		"call:assessor-b:0", "put:assessor-b:0", "call:assessor-b:1", "put:assessor-b:1", "call:assessor-b:2", "put:assessor-b:2", "stitch:assessor-b", "decision",
	}
	if !reflect.DeepEqual(events, wantEvents) || len(evidence.assessments) != 6 || len(evidence.stitches) != 2 || len(evidence.decisions) != 1 ||
		artifact.Decision.Status != fillerstructure.StatusConfirmed || artifact.Decision.Unit != fillerstructure.UnitCompilation ||
		artifact.Decision.Input.Kind != fillerstructure.AssessmentInputWindowMediaSet {
		t.Fatalf("events=%v evidence=%+v artifact=%+v", events, evidence, artifact)
	}
}

func TestStructureWindowRuntimeRetainsOperationalFailureAndCompletesEveryWindow(t *testing.T) {
	input, prepared := structureWindowRuntimeFixture(t)
	events := []string{}
	timeline := []fillerstructure.Segment{{StartMS: 0, EndMS: 300_000, Role: fillerstructure.RoleCommercial}}
	left := windowAssessorFixture("assessor-a", "family-a", "a", timeline, &events)
	left.failureOrdinal = 1
	evidence := &capturedWindowEvidence{events: &events}
	runtime, err := NewStructureWindowAssessmentRuntime([]CompleteWindowStructureAssessor{
		left, windowAssessorFixture("assessor-b", "family-b", "b", timeline, &events),
	}, &capturedWindowPreparer{prepared: prepared}, evidence, 2_000, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := runtime.Assess(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Decision.Status != fillerstructure.StatusHeld ||
		!reflect.DeepEqual(artifact.Decision.ReasonCodes, []string{fillerstructure.ReasonOperationalFailure}) ||
		len(evidence.assessments) != 6 || len(evidence.stitches) != 2 || evidence.stitches[0].Status != fillerstructurewindow.StitchHeld {
		t.Fatalf("events=%v evidence=%+v artifact=%+v", events, evidence, artifact)
	}
}

func TestStructureWindowRuntimeStopsBeforeNextCallOnDriftOrPersistenceFailure(t *testing.T) {
	for _, test := range []struct {
		name      string
		drift     bool
		failEvent string
	}{
		{name: "profile drift", drift: true},
		{name: "assessment persistence", failEvent: "put:assessor-a:0"},
		{name: "stitch persistence", failEvent: "stitch:assessor-a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, prepared := structureWindowRuntimeFixture(t)
			events := []string{}
			timeline := []fillerstructure.Segment{{StartMS: 0, EndMS: 300_000, Role: fillerstructure.RoleCommercial}}
			left := windowAssessorFixture("assessor-a", "family-a", "a", timeline, &events)
			left.driftProfile = test.drift
			evidence := &capturedWindowEvidence{events: &events, failEvent: test.failEvent}
			runtime, err := NewStructureWindowAssessmentRuntime([]CompleteWindowStructureAssessor{
				left, windowAssessorFixture("assessor-b", "family-b", "b", timeline, &events),
			}, &capturedWindowPreparer{prepared: prepared}, evidence, 2_000, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.Assess(t.Context(), input); err == nil {
				t.Fatal("invalid or unpersisted evidence reached reduction")
			}
			for _, event := range events {
				if strings.HasPrefix(event, "call:assessor-b") {
					t.Fatalf("second family ran after failure: %v", events)
				}
			}
		})
	}
}

func TestStructureWindowRuntimeRejectsInvalidPolicyBeforePreparation(t *testing.T) {
	input, prepared := structureWindowRuntimeFixture(t)
	events := []string{}
	timeline := []fillerstructure.Segment{{StartMS: 0, EndMS: 300_000, Role: fillerstructure.RoleCommercial}}
	assessors := []CompleteWindowStructureAssessor{
		windowAssessorFixture("assessor-a", "family-a", "a", timeline, &events),
		windowAssessorFixture("assessor-b", "family-b", "b", timeline, &events),
	}
	preparer := &capturedWindowPreparer{prepared: prepared}
	if _, err := NewStructureWindowAssessmentRuntime(assessors, preparer, &capturedWindowEvidence{events: &events}, fillerstructurewindow.ContextOverlapMS, time.Now); err == nil {
		t.Fatal("seam tolerance as wide as the context was accepted")
	}
	prepared.Windows[0].FullPath = input.FullPath
	runtime, err := NewStructureWindowAssessmentRuntime(assessors, preparer, &capturedWindowEvidence{events: &events}, 2_000, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Assess(t.Context(), input); err == nil || len(events) != 0 {
		t.Fatalf("source path reused as normalized window: events=%v error=%v", events, err)
	}
}

func TestStructureWindowRuntimeFreezesConfiguredProfiles(t *testing.T) {
	input, prepared := structureWindowRuntimeFixture(t)
	events := []string{}
	timeline := []fillerstructure.Segment{{StartMS: 0, EndMS: 300_000, Role: fillerstructure.RoleCommercial}}
	left := windowAssessorFixture("assessor-a", "family-a", "a", timeline, &events)
	runtime, err := NewStructureWindowAssessmentRuntime([]CompleteWindowStructureAssessor{
		left, windowAssessorFixture("assessor-b", "family-b", "b", timeline, &events),
	}, &capturedWindowPreparer{prepared: prepared}, &capturedWindowEvidence{events: &events}, 2_000, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	left.profile.ID = "assessor-a-mutated"
	if _, err := runtime.Assess(t.Context(), input); err == nil {
		t.Fatal("profile mutation after construction was accepted")
	}
}

func structureWindowRuntimeFixture(t *testing.T) (StructureAssessmentSource, StructureAssessmentWindowMediaSet) {
	t.Helper()
	root := t.TempDir()
	source := SplitSourceAsset{
		Role: SplitSourceLegacyPlayback, SHA256: strings.Repeat("1", 64), Bytes: 2_048,
		ClipHash: strings.Repeat("2", 64), Path: "source.mp4", DurationMs: 300_000,
	}
	input := StructureAssessmentSource{Source: source, FullPath: filepath.Join(root, source.Path)}
	core := fillerstructure.Source{SHA256: source.SHA256, Bytes: source.Bytes, DurationMS: source.DurationMs}
	plan, err := fillerstructurewindow.NewPlan(core)
	if err != nil {
		t.Fatal(err)
	}
	media := make([]fillerstructure.AssessmentMedia, len(plan.Windows))
	for ordinal, window := range plan.Windows {
		media[ordinal] = fillerstructure.AssessmentMedia{
			SHA256: strings.Repeat(string(rune('3'+ordinal)), 64), Bytes: 1_024,
			DurationMS:    window.MediaEndMS - window.MediaStartMS,
			ProfileSHA256: plan.Profile.AssessmentMediaProfileSHA256,
			LineageSHA256: strings.Repeat(string(rune('6'+ordinal)), 64),
		}
	}
	set, err := fillerstructurewindow.NewMediaSet(plan, media)
	if err != nil {
		t.Fatal(err)
	}
	prepared := StructureAssessmentWindowMediaSet{Source: source, Authority: set}
	for ordinal, window := range plan.Windows {
		prepared.Windows = append(prepared.Windows, StructureAssessmentWindowMedia{
			Window: window, Media: set.Windows[ordinal], FullPath: filepath.Join(root, "window-"+string(rune('0'+ordinal))+".mp4"),
		})
	}
	return input, prepared
}

func windowAssessorFixture(id, family, digest string, timeline []fillerstructure.Segment, events *[]string) *capturedWindowAssessor {
	return &capturedWindowAssessor{
		profile: fillerstructure.AssessorProfile{
			ID: id, ModelFamily: family, Provider: "provider", Model: "model",
			ModelDigest: strings.Repeat(digest, 64), CapabilitySHA256: strings.Repeat("f", 64),
			PromptVersion: "window-prompt-v1", EvidenceContract: "window-assessment-v2",
		},
		timeline: timeline, events: events, failureOrdinal: -1,
	}
}

func clipStructureTimeline(timeline []fillerstructure.Segment, window fillerstructurewindow.Window) []fillerstructure.Segment {
	var clipped []fillerstructure.Segment
	for _, segment := range timeline {
		start, end := max(segment.StartMS, window.MediaStartMS), min(segment.EndMS, window.MediaEndMS)
		if start < end {
			clipped = append(clipped, fillerstructure.Segment{StartMS: start, EndMS: end, Role: segment.Role})
		}
	}
	return clipped
}
