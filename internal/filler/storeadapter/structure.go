package storeadapter

import (
	"context"

	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
)

// StructureWindowLedger bridges the store → the long-reel window assessor's call ledger, under the
// per-source and per-day budget the deployment declares.
type StructureWindowLedger struct {
	store  records.FillerStructureAssessmentStore
	budget records.InferenceBudget
}

// NewStructureWindowLedger returns the window call ledger over store and budget.
func NewStructureWindowLedger(store records.FillerStructureAssessmentStore, budget records.InferenceBudget) StructureWindowLedger {
	return StructureWindowLedger{store: store, budget: budget}
}

func (l StructureWindowLedger) Reserve(ctx context.Context, reservation window.CallReservation) (window.CallReservationState, error) {
	return l.store.ReserveStructureWindowCall(ctx, reservation, l.budget)
}

func (l StructureWindowLedger) Settle(ctx context.Context, record window.CallRecord) error {
	return l.store.SettleStructureWindowCall(ctx, record)
}
