package fillersafety

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestEvaluationOperationBudgetHoldPreventsHTTPAndReturnsDurableHold(t *testing.T) {
	t.Parallel()
	fixture := newOperationFixture(t, []proposedInterval{{StartMS: 100, EndMS: 800}})
	fixture.repository.budgetHeld = true

	report, err := fixture.operation.Evaluate(t.Context(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if report.Result.Outcome != OutcomeHold || fixture.audio.calls != 0 || fixture.video.calls != 0 ||
		len(fixture.repository.reservations) != 1 || len(fixture.repository.settlements) != 0 {
		t.Fatalf("report=%+v calls=%d/%d reservations=%d settlements=%d", report, fixture.audio.calls, fixture.video.calls, len(fixture.repository.reservations), len(fixture.repository.settlements))
	}
	if got := fixture.repository.events[len(fixture.repository.events)-1]; got.Terminal == nil {
		t.Fatalf("budget hold did not reach a durable terminal: %+v", got)
	}
}

func TestEvaluationOperationPersistsUnprojectablePresenceWithoutPromotingIt(t *testing.T) {
	t.Parallel()
	fixture := newOperationFixture(t, nil)
	fixture.video.state = VideoProhibitedUnprojectable
	fixture.video.err = errors.New("private malformed timing detail")

	report, err := fixture.operation.Evaluate(t.Context(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if report.Result.Outcome != OutcomeHold || !slices.Equal(report.Result.Reasons, []Reason{ReasonPresenceUnprojectable}) ||
		len(fixture.repository.settlements) != 1 || fixture.repository.settlements[0].Failure != FailureInvalidResponse {
		t.Fatalf("report=%+v settlements=%+v", report, fixture.repository.settlements)
	}
	raw, err := json.Marshal(fixture.repository.events)
	if err != nil || strings.Contains(string(raw), "private malformed timing detail") {
		t.Fatalf("ledger retained private provider detail: %s err=%v", raw, err)
	}
}

func TestEvaluationOperationReservationFailureLeavesRunForRecovery(t *testing.T) {
	t.Parallel()
	fixture := newOperationFixture(t, []proposedInterval{{StartMS: 100, EndMS: 800}})
	fixture.repository.reserveErr = errors.New("private persistence detail")

	if _, err := fixture.operation.Evaluate(t.Context(), fixture.request); err == nil {
		t.Fatal("expected reservation persistence failure")
	}
	if fixture.audio.calls != 0 || len(fixture.repository.events) != 2 || fixture.repository.events[1].Proposal == nil {
		t.Fatalf("calls=%d events=%+v", fixture.audio.calls, fixture.repository.events)
	}
	if _, err := fixture.operation.Evaluate(t.Context(), fixture.request); !errors.Is(err, ErrEvaluationIncomplete) {
		t.Fatalf("in-place retry=%v, want ErrEvaluationIncomplete", err)
	}
}

func TestEvaluationOperationUnknownChargeLeavesAcceptedReservationUnsettled(t *testing.T) {
	t.Parallel()
	fixture := newOperationFixture(t, []proposedInterval{{StartMS: 100, EndMS: 800}})
	fixture.audio.err = errors.New("private transport detail")
	fixture.audio.unknownCharge = true

	if _, err := fixture.operation.Evaluate(t.Context(), fixture.request); !errors.Is(err, ErrEvaluationIncomplete) {
		t.Fatalf("err=%v, want ErrEvaluationIncomplete", err)
	}
	if fixture.audio.calls != 1 || len(fixture.repository.settlements) != 0 ||
		len(fixture.repository.events) != 3 || fixture.repository.events[2].Reserve == nil {
		t.Fatalf("calls=%d settlements=%d events=%+v", fixture.audio.calls, len(fixture.repository.settlements), fixture.repository.events)
	}
}

func TestEvaluationOperationSettlementFailureCannotReturnUnrecordedEvidence(t *testing.T) {
	t.Parallel()
	fixture := newOperationFixture(t, []proposedInterval{{StartMS: 100, EndMS: 800}})
	fixture.repository.settleErr = errors.New("private settlement detail")

	if _, err := fixture.operation.Evaluate(t.Context(), fixture.request); err == nil {
		t.Fatal("expected settlement persistence failure")
	}
	if fixture.audio.calls != 1 || len(fixture.repository.events) != 3 ||
		fixture.repository.events[len(fixture.repository.events)-1].Reserve == nil {
		t.Fatalf("calls=%d events=%+v", fixture.audio.calls, fixture.repository.events)
	}
}
