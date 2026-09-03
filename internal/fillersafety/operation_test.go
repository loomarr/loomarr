package fillersafety

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestEvaluationOperationRecordsSerialCascadeBeforeReturningEvidence(t *testing.T) {
	t.Parallel()
	fixture := newOperationFixture(t, []proposedInterval{{StartMS: 100, EndMS: 800}})

	report, err := fixture.operation.Evaluate(t.Context(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if report.Result.Outcome != OutcomeCandidateRejected || report.Run.ID != fixture.request.RunID ||
		report.TerminalEventID == "" || !validSHA256(report.TerminalSHA256) ||
		fixture.proposer.calls != 1 || fixture.audio.calls != 1 || fixture.video.calls != 1 {
		t.Fatalf("report=%+v calls=%d/%d/%d", report, fixture.proposer.calls, fixture.audio.calls, fixture.video.calls)
	}
	kinds := make([]LedgerEventKind, 0, len(fixture.repository.events))
	for index, event := range fixture.repository.events {
		if event.Ordinal != index {
			t.Fatalf("event %d has ordinal %d", index, event.Ordinal)
		}
		kinds = append(kinds, event.Kind)
	}
	wantKinds := []LedgerEventKind{
		LedgerSourcePlanned, LedgerProposalCompleted,
		LedgerInferenceReserved, LedgerInferenceSettled,
		LedgerInferenceReserved, LedgerInferenceSettled, LedgerTerminal,
	}
	if !slices.Equal(kinds, wantKinds) {
		t.Fatalf("event kinds=%v", kinds)
	}
	terminalEvent := fixture.repository.events[len(fixture.repository.events)-1]
	terminal := terminalEvent.Terminal
	digest, digestErr := LedgerEventSHA256(terminalEvent)
	if terminal == nil || !sameResult(terminal.Result, report.Result) ||
		len(terminal.EventIDs) != len(fixture.repository.events)-1 || digestErr != nil ||
		report.TerminalEventID != terminalEvent.ID || report.TerminalSHA256 != digest {
		t.Fatalf("terminal=%+v digest=%s err=%v", terminalEvent, digest, digestErr)
	}
	if len(fixture.repository.reservations) != 2 ||
		!slices.Equal(fixture.repository.reservations[0].Modalities, []string{"audio"}) ||
		!slices.Equal(fixture.repository.reservations[1].Modalities, []string{"audio", "video"}) ||
		fixture.repository.events[2].Reserve.Role != "spoken-safety" || fixture.repository.events[2].Reserve.Rung != "native-audio" ||
		fixture.repository.events[2].Reserve.DerivativeBytes <= 0 || fixture.repository.events[2].Reserve.DerivativeDurationMS <= 0 ||
		fixture.repository.events[4].Reserve.Rung != "complete-video" ||
		fixture.repository.reservations[0].Versions.EvidenceSHA256 != report.Run.AuthoritySHA256 ||
		fixture.repository.reservations[0].Versions.CertificationSHA256 != report.Run.CertificationSHA256 {
		t.Fatalf("reservations=%+v", fixture.repository.reservations)
	}
	public, err := json.Marshal(report)
	if err != nil || strings.Contains(string(public), fixture.request.Source.Path) || strings.Contains(string(public), "complete operation source") {
		t.Fatalf("report leaked private source data: %s err=%v", public, err)
	}
}

func TestEvaluationOperationUsesWeightFreeWindowsForCompleteAudioCoverage(t *testing.T) {
	t.Parallel()
	fixture := newOperationFixture(t, nil)
	proposer, identity := newCompleteAudioWindowProposer()
	fixture.operation.cascade.proposer = proposer
	fixture.operation.cascade.proposerIdentity = identity

	report, err := fixture.operation.Evaluate(t.Context(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if report.Result.Outcome != OutcomeCandidateRejected || len(report.Evidence.Candidates) != 2 ||
		fixture.audio.calls != 2 || fixture.video.calls != 1 || len(fixture.repository.reservations) != 3 ||
		report.Evidence.Candidates[0].StartMS != 0 || report.Evidence.Candidates[0].EndMS != completeAudioWindowMS ||
		report.Evidence.Candidates[1].StartMS != completeAudioWindowMS || report.Evidence.Candidates[1].EndMS != report.Run.DurationMS {
		t.Fatalf("report=%+v calls=%d/%d reservations=%d", report, fixture.audio.calls, fixture.video.calls, len(fixture.repository.reservations))
	}
}

func TestEvaluationOperationReturnsCompletedRunWithoutRepeatingWork(t *testing.T) {
	t.Parallel()
	fixture := newOperationFixture(t, []proposedInterval{{StartMS: 100, EndMS: 800}})
	first, err := fixture.operation.Evaluate(t.Context(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	eventCount := len(fixture.repository.events)
	second, err := fixture.operation.Evaluate(t.Context(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflectEvaluationReport(first, second) || len(fixture.repository.events) != eventCount ||
		fixture.proposer.calls != 1 || fixture.audio.calls != 1 || fixture.video.calls != 1 {
		t.Fatalf("first=%+v second=%+v events=%d calls=%d/%d/%d", first, second, len(fixture.repository.events), fixture.proposer.calls, fixture.audio.calls, fixture.video.calls)
	}
}

func reflectEvaluationReport(first, second EvaluationReport) bool {
	return reflect.DeepEqual(first, second)
}
