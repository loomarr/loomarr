package records

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
)

// A call that produced no answer must be reservable again: a budget hold replaces itself, and a
// lost request keeps its reservation in the shared budget while the retry reserves under a new
// attempt. Every outcome that carries a response (or an open reservation) still refuses a second
// reservation, so a billed call is never repeated.
func testFillerStructureWindowCallLedgerRetry(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Date(2026, time.September, 12, 6, 0, 0, 0, time.UTC)
	roomy := InferenceBudget{PerClipNanoUSD: 200, PerDayNanoUSD: 20_000}
	tight := InferenceBudget{PerClipNanoUSD: 50, PerDayNanoUSD: 20_000}
	reserve := func(reservation window.CallReservation, budget InferenceBudget) window.CallReservationState {
		t.Helper()
		state, err := s.ReserveStructureWindowCall(ctx, reservation, budget)
		if err != nil {
			t.Fatalf("reserve: %v", err)
		}
		return state
	}
	settle := func(record window.CallRecord) {
		t.Helper()
		if err := s.SettleStructureWindowCall(ctx, record); err != nil {
			t.Fatalf("settle %s: %v", record.State, err)
		}
	}
	evaluation := func(id string) InferenceEvaluation {
		t.Helper()
		e, err := s.GetInferenceEvaluation(ctx, id)
		if err != nil {
			t.Fatalf("evaluation %s: %v", id, err)
		}
		return e
	}

	// Budget hold, then budget restored: the held row is replaced in place and nothing leaks.
	held := structureWindowCallReservationFixture(t, "1", "a", at)
	if state := reserve(held, tight); state != window.CallReservationHeldBudget {
		t.Fatalf("first reservation state = %q, want held_budget", state)
	}
	settle(structureWindowCallRecordFixture(t, held, structure.AssessmentRecordHeldBudget, 0))
	if state := reserve(held, roomy); state != window.CallReservationAccepted {
		t.Fatalf("reservation after the budget returned = %q, want accepted", state)
	}
	entry, err := s.GetStructureWindowCallLedgerEntry(ctx, held.RequestSHA256)
	if err != nil || entry.State != window.CallLedgerOpen || entry.Record != nil {
		t.Fatalf("entry after re-reservation = %+v error=%v, want one open entry", entry, err)
	}
	if e := evaluation("structure-window-" + held.RequestSHA256); e.State != InferenceReserved || e.ReservedNanoUSD != held.RequestedNanoUSD {
		t.Fatalf("replaced evaluation = %+v, want a fresh reservation", e)
	}
	settle(structureWindowCallRecordFixture(t, held, structure.AssessmentRecordAccepted, 40))
	if _, err := s.ReserveStructureWindowCall(ctx, held, roomy); !errors.Is(err, window.ErrCallLedgerConflict) {
		t.Fatalf("reservation after an accepted settlement error = %v, want conflict", err)
	}

	// A hold that never settled (the process stopped between reserve and settle) is replaced too.
	crashedHold := structureWindowCallReservationFixture(t, "5", "e", at)
	if state := reserve(crashedHold, tight); state != window.CallReservationHeldBudget {
		t.Fatalf("crashed hold state = %q", state)
	}
	if state := reserve(crashedHold, roomy); state != window.CallReservationAccepted {
		t.Fatalf("reservation after an unsettled hold = %q, want accepted", state)
	}

	// Transport failure: the lost request stays counted, the retry reserves a new attempt, and the
	// budget bounds how many attempts can be paid for.
	lost := structureWindowCallReservationFixture(t, "2", "b", at)
	lostID := "structure-window-" + lost.RequestSHA256
	if state := reserve(lost, roomy); state != window.CallReservationAccepted {
		t.Fatalf("lost call reservation = %q", state)
	}
	settle(structureWindowCallRecordFixture(t, lost, structure.AssessmentRecordUnsettled, 0))
	if state := reserve(lost, roomy); state != window.CallReservationAccepted {
		t.Fatalf("retry after transport = %q, want accepted", state)
	}
	if e := evaluation(lostID); e.State != InferenceFailed || e.ReservedNanoUSD != lost.RequestedNanoUSD {
		t.Fatalf("first attempt evaluation = %+v, want its reservation kept", e)
	}
	if e := evaluation(lostID + "-2"); e.State != InferenceReserved || e.ReservedNanoUSD != lost.RequestedNanoUSD {
		t.Fatalf("second attempt evaluation = %+v, want a new reservation", e)
	}
	entry, err = s.GetStructureWindowCallLedgerEntry(ctx, lost.RequestSHA256)
	if err != nil || entry.State != window.CallLedgerOpen {
		t.Fatalf("entry after the retry = %+v error=%v, want the open second attempt", entry, err)
	}
	settle(structureWindowCallRecordFixture(t, lost, structure.AssessmentRecordUnsettled, 0))
	if state := reserve(lost, roomy); state != window.CallReservationHeldBudget {
		t.Fatalf("third attempt = %q, want held_budget: two lost requests used the clip budget", state)
	}
	settle(structureWindowCallRecordFixture(t, lost, structure.AssessmentRecordHeldBudget, 0))
	if state := reserve(lost, InferenceBudget{PerClipNanoUSD: 1_000, PerDayNanoUSD: 20_000}); state != window.CallReservationAccepted {
		t.Fatalf("attempt after the budget returned = %q, want accepted", state)
	}
	if e := evaluation(lostID + "-3"); e.State != InferenceReserved {
		t.Fatalf("third attempt evaluation = %+v, want it reserved after replacing the hold", e)
	}

	// A response with an unknown charge may have been billed: it replays, never reserves again.
	billed := structureWindowCallReservationFixture(t, "3", "c", at)
	if state := reserve(billed, roomy); state != window.CallReservationAccepted {
		t.Fatalf("billed call reservation = %q", state)
	}
	settle(structureWindowUnsettledResponseRecord(t, billed))
	if _, err := s.ReserveStructureWindowCall(ctx, billed, roomy); !errors.Is(err, window.ErrCallLedgerConflict) {
		t.Fatalf("reservation after an unsettled response error = %v, want conflict", err)
	}

	// An open reservation is a recovery hold, never permission to resend.
	open := structureWindowCallReservationFixture(t, "4", "d", at)
	reserve(open, roomy)
	if _, err := s.ReserveStructureWindowCall(ctx, open, roomy); !errors.Is(err, window.ErrCallLedgerConflict) {
		t.Fatalf("second reservation while open error = %v, want conflict", err)
	}
}

func structureWindowUnsettledResponseRecord(t *testing.T, reservation window.CallReservation) window.CallRecord {
	t.Helper()
	recorded, err := window.NewRecordedAssessment(window.CallRecordInput{
		MediaSet: reservation.MediaSet, WindowOrdinal: reservation.WindowOrdinal, Assessor: reservation.Assessor,
		MetadataSnapshotSHA256: reservation.MetadataSnapshotSHA256,
		PromptSHA256:           reservation.PromptSHA256, SchemaSHA256: reservation.SchemaSHA256,
		RequestSHA256: reservation.RequestSHA256, UpstreamProvider: reservation.UpstreamProvider,
		UpstreamProviderSlug: reservation.UpstreamProviderSlug, RequestedNanoUSD: reservation.RequestedNanoUSD,
		RawResponse: []byte(`{"id":"generation"}`), ReservedNanoUSD: reservation.RequestedNanoUSD,
		AccountedNanoUSD: reservation.RequestedNanoUSD, AssessedAt: reservation.RequestedAt.Add(time.Second),
		State: structure.AssessmentRecordUnsettled, Failure: structure.AssessmentFailureUnsettled,
	})
	if err != nil {
		t.Fatal(err)
	}
	return recorded.Record
}
