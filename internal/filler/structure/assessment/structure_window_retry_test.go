package assessment

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
)

type windowOutcome int

const (
	outcomeAccepted windowOutcome = iota
	outcomeHeldBudget
	outcomeTransport
	outcomeUnsettledWithResponse
)

// scriptedWindowAssessor answers each window from a per-ordinal queue of outcomes and accepts once
// the queue is empty, so a test can break one window and then restore it for the next pass.
type scriptedWindowAssessor struct {
	*capturedWindowAssessor
	script map[int][]windowOutcome
	calls  []int
}

func (a *scriptedWindowAssessor) AssessWindow(ctx context.Context, set window.MediaSet, media StructureAssessmentWindowMedia) (window.RecordedAssessment, error) {
	ordinal := media.Window.Ordinal
	outcome := outcomeAccepted
	if queue := a.script[ordinal]; len(queue) > 0 {
		outcome, a.script[ordinal] = queue[0], queue[1:]
	}
	a.calls = append(a.calls, ordinal)
	if outcome == outcomeAccepted {
		return a.capturedWindowAssessor.AssessWindow(ctx, set, media)
	}
	duration := media.Window.MediaEndMS - media.Window.MediaStartMS
	input := window.CallRecordInput{
		MediaSet: set, WindowOrdinal: ordinal, Assessor: a.profile,
		MetadataSnapshotSHA256: strings.Repeat("f", 64),
		PromptSHA256:           window.DirectVideoPromptSHA256(duration),
		SchemaSHA256:           window.DirectVideoSchemaSHA256(duration),
		RequestSHA256:          windowRequestDigest(a.profile.ID, ordinal),
		UpstreamProvider:       "Provider", UpstreamProviderSlug: "provider", RequestedNanoUSD: 2_000,
		AssessedAt: time.Date(2026, time.September, 11, 10, 0, ordinal, 0, time.UTC),
	}
	switch outcome {
	case outcomeHeldBudget:
		input.State, input.Failure = structure.AssessmentRecordHeldBudget, structure.AssessmentFailureBudget
	case outcomeTransport:
		input.State, input.Failure = structure.AssessmentRecordUnsettled, structure.AssessmentFailureTransport
		input.ReservedNanoUSD, input.AccountedNanoUSD = 2_000, 2_000
	case outcomeUnsettledWithResponse:
		input.State, input.Failure = structure.AssessmentRecordUnsettled, structure.AssessmentFailureUnsettled
		input.RawResponse = []byte("provider response without a charge")
		input.ReservedNanoUSD, input.AccountedNanoUSD = 2_000, 2_000
	}
	return window.NewRecordedAssessment(input)
}

func retryFamilyFixture(t *testing.T, script map[int][]windowOutcome) (*StructureWindowFamilyRuntime, *scriptedWindowAssessor, *capturedWindowEvidence, StructureAssessmentWindowMediaSet) {
	t.Helper()
	_, prepared := structureWindowRuntimeFixture(t)
	events := []string{}
	timeline := []structure.Segment{{StartMS: 0, EndMS: 300_000, Role: structure.RoleCommercial}}
	assessor := &scriptedWindowAssessor{
		capturedWindowAssessor: windowAssessorFixture("assessor-a", "family-a", "a", timeline, &events), script: script,
	}
	evidence := &capturedWindowEvidence{events: &events}
	runtime, err := NewStructureWindowFamilyRuntime(assessor, evidence, 2_000)
	if err != nil {
		t.Fatal(err)
	}
	return runtime, assessor, evidence, prepared
}

// A held budget and a lost request say nothing about the window. They are not published, so the
// next pass calls the same window again and every earlier window replays.
func TestStructureWindowFamilyRuntimeRetriesBudgetHoldAndTransportOnTheNextPass(t *testing.T) {
	for name, outcome := range map[string]windowOutcome{"budget hold": outcomeHeldBudget, "transport failure": outcomeTransport} {
		t.Run(name, func(t *testing.T) {
			runtime, assessor, evidence, prepared := retryFamilyFixture(t, map[int][]windowOutcome{1: {outcome}})
			_, err := runtime.Assess(t.Context(), prepared)
			if !errors.Is(err, ErrStructureWindowRetryable) {
				t.Fatalf("first pass error = %v, want ErrStructureWindowRetryable", err)
			}
			if !slices.Equal(assessor.calls, []int{0, 1}) || len(evidence.assessments) != 1 || len(evidence.stitches) != 0 {
				t.Fatalf("first pass calls=%v published=%d stitches=%d, want window 0 only", assessor.calls, len(evidence.assessments), len(evidence.stitches))
			}
			assessor.calls = nil
			stitch, err := runtime.Assess(t.Context(), prepared)
			if err != nil {
				t.Fatalf("second pass: %v", err)
			}
			if !slices.Equal(assessor.calls, []int{1, 2}) || stitch.Status != window.StitchComplete {
				t.Fatalf("second pass calls=%v status=%q, want windows 1 and 2 assessed to a complete stitch", assessor.calls, stitch.Status)
			}
		})
	}
}

// A response with an unknown charge may have been billed, so it replays and is never called twice.
func TestStructureWindowFamilyRuntimeReplaysUnsettledResponseWithoutASecondCall(t *testing.T) {
	runtime, assessor, _, prepared := retryFamilyFixture(t, map[int][]windowOutcome{1: {outcomeUnsettledWithResponse}})
	first, err := runtime.Assess(t.Context(), prepared)
	if err != nil || first.Status != window.StitchHeld || first.HoldReason != window.HoldOperationalFailure {
		t.Fatalf("first pass status=%q reason=%q error=%v", first.Status, first.HoldReason, err)
	}
	assessor.calls = nil
	second, err := runtime.Assess(t.Context(), prepared)
	if err != nil || len(assessor.calls) != 0 || second.SHA256 != first.SHA256 {
		t.Fatalf("second pass calls=%v error=%v: an unsettled response must replay", assessor.calls, err)
	}
}

// The repository enforces the rule itself, so no caller can pin a source with a no-answer outcome.
func TestFileStructureWindowEvidenceRepositoryRefusesToPublishNoAnswerOutcomes(t *testing.T) {
	_, assessor, _, prepared := retryFamilyFixture(t, map[int][]windowOutcome{0: {outcomeHeldBudget, outcomeTransport}})
	repository := structureEvidenceRepositoryFixture(t)
	for _, name := range []string{"budget hold", "transport failure"} {
		recorded, err := assessor.AssessWindow(t.Context(), prepared.Authority, prepared.Windows[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := repository.PutStructureWindowAssessmentEvidence(t.Context(), recorded); err == nil {
			t.Fatalf("%s: repository published a no-answer outcome", name)
		}
		_, found, err := repository.FindStructureWindowAssessmentEvidence(t.Context(), prepared.Authority, 0, assessor.Profile())
		if err != nil || found {
			t.Fatalf("%s: found=%v error=%v, want nothing published", name, found, err)
		}
	}
}
