package filler

import (
	"context"
	"testing"
	"time"
)

type structureShadowMemoryRepository struct {
	records []StructureSplitShadowDecision
}

func (r *structureShadowMemoryRepository) PutStructureSplitShadowDecision(_ context.Context, decision StructureSplitShadowDecision) error {
	r.records = append(r.records, decision)
	return nil
}

func (r *structureShadowMemoryRepository) GetStructureSplitShadowDecision(_ context.Context, id string) (StructureSplitShadowDecision, bool, error) {
	for _, record := range r.records {
		if record.ID == id {
			return record, true, nil
		}
	}
	return StructureSplitShadowDecision{}, false, nil
}

func TestStructureSplitShadowPersistsCompatibilityAndCompletePlanDecisions(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	proposal.ID = "proposal-1"
	proposal.ClipHash = proposal.Source.ClipHash
	proposal.CreatedAt = time.Date(2026, time.September, 3, 13, 0, 0, 0, time.UTC)
	auto := certifiedAutoPolicy()
	repository := &structureShadowMemoryRepository{}
	shadow, err := NewStructureSplitShadow(repository, auto, &StructureCertificationPolicy{
		ScreeningCertified: func(SegmentScreeningEvidence) bool { return false },
	}, func() time.Duration { return 10 * time.Second }, "production-shadow-no-certified-slices-v1")
	if err != nil {
		t.Fatal(err)
	}
	legacy := AutoConfirmable(proposal, auto, 10*time.Second)
	if err := shadow.ObserveStructureSplit(t.Context(), proposal, legacy); err != nil {
		t.Fatal(err)
	}
	if len(repository.records) != 1 {
		t.Fatalf("records = %d", len(repository.records))
	}
	record := repository.records[0]
	if len(record.Legacy.Confirm) != 2 || len(record.Legacy.Hold) != 0 || record.Legacy.Verdict != AutoSplitOK {
		t.Fatalf("legacy outcome = %+v", record.Legacy)
	}
	if len(record.Certified.Confirm) != 0 || len(record.Certified.Hold) != 2 || record.Certified.Verdict != RejectStructureUncertified {
		t.Fatalf("certified outcome = %+v", record.Certified)
	}
	wantObservedAt := proposal.Structure.AssessedAt
	if proposal.StructureDecision.DecidedAt.After(wantObservedAt) {
		wantObservedAt = proposal.StructureDecision.DecidedAt
	}
	if record.AssessmentSHA256 != proposal.Structure.SHA256 || record.StructureDecisionSHA256 != proposal.StructureDecision.SHA256 || record.SourceSHA256 != proposal.Source.SHA256 || record.ObservedAt != wantObservedAt || ValidateStructureSplitShadowDecision(record) != nil {
		t.Fatalf("record = %+v", record)
	}
	if err := shadow.ObserveStructureSplit(t.Context(), proposal, legacy); err != nil {
		t.Fatal(err)
	}
	if repository.records[1].ID != record.ID || repository.records[1].SHA256 != record.SHA256 {
		t.Fatalf("retry changed content identity: first=%+v second=%+v", record, repository.records[1])
	}
	pending, err := shadow.NeedsStructureSplitObservation(t.Context(), proposal)
	if err != nil || pending {
		t.Fatalf("persisted observation pending = %v, error = %v", pending, err)
	}
}

func TestStructureSplitShadowDetectsUnobservedAndChangedProposals(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	proposal.ID = "proposal-1"
	proposal.ClipHash = proposal.Source.ClipHash
	proposal.CreatedAt = time.Date(2026, time.September, 3, 13, 0, 0, 0, time.UTC)
	repository := &structureShadowMemoryRepository{}
	shadow, err := NewStructureSplitShadow(repository, certifiedAutoPolicy(), allowCertifiedStructure(), func() time.Duration { return 10 * time.Second }, "candidate-v1")
	if err != nil {
		t.Fatal(err)
	}
	pending, err := shadow.NeedsStructureSplitObservation(t.Context(), proposal)
	if err != nil || !pending {
		t.Fatalf("new observation pending = %v, error = %v", pending, err)
	}
	legacy := AutoConfirmable(proposal, certifiedAutoPolicy(), 10*time.Second)
	if err := shadow.ObserveStructureSplit(t.Context(), proposal, legacy); err != nil {
		t.Fatal(err)
	}
	proposal.Segments[0].BoundaryConfidence = 60
	pending, err = shadow.NeedsStructureSplitObservation(t.Context(), proposal)
	if err != nil || !pending {
		t.Fatalf("changed observation pending = %v, error = %v", pending, err)
	}
}

func TestStructureSplitShadowWholeProposalReasonOverridesStaleSegmentReason(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	proposal.ID = "proposal-1"
	proposal.ClipHash = proposal.Source.ClipHash
	proposal.CreatedAt = time.Date(2026, time.September, 3, 13, 0, 0, 0, time.UTC)
	proposal.Segments[0].HoldReason = string(RejectBoundaryUncertain)
	legacy := AutoConfirmable(proposal, certifiedAutoPolicy(), 10*time.Second)
	certified := SplitPartition{Reject: RejectStructureUncertified, Hold: proposal.Segments}
	decision, err := newStructureSplitShadowDecision(proposal, legacy, certified, "candidate-v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, span := range decision.Certified.Hold {
		if span.HoldReason != string(RejectStructureUncertified) {
			t.Fatalf("certified hold reason = %q", span.HoldReason)
		}
	}
}

func TestStructureSplitShadowRejectsIncompleteCompatibilityOutcome(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	proposal.ID = "proposal-1"
	proposal.ClipHash = proposal.Source.ClipHash
	proposal.CreatedAt = time.Date(2026, time.September, 3, 13, 0, 0, 0, time.UTC)
	repository := &structureShadowMemoryRepository{}
	shadow, err := NewStructureSplitShadow(repository, certifiedAutoPolicy(), allowCertifiedStructure(), func() time.Duration { return 10 * time.Second }, "candidate-v1")
	if err != nil {
		t.Fatal(err)
	}
	legacy := SplitPartition{Confirm: proposal.Segments[:1]}
	if err := shadow.ObserveStructureSplit(t.Context(), proposal, legacy); err == nil || len(repository.records) != 0 {
		t.Fatalf("incomplete outcome error = %v, records = %d", err, len(repository.records))
	}
}

func TestValidateStructureSplitShadowDecisionRejectsMutation(t *testing.T) {
	proposal := certifiedStructureProposal(t)
	proposal.ID = "proposal-1"
	proposal.ClipHash = proposal.Source.ClipHash
	proposal.CreatedAt = time.Date(2026, time.September, 3, 13, 0, 0, 0, time.UTC)
	legacy := AutoConfirmable(proposal, certifiedAutoPolicy(), 10*time.Second)
	decision, err := newStructureSplitShadowDecision(proposal, legacy, legacy, "candidate-v1")
	if err != nil {
		t.Fatal(err)
	}
	decision.Legacy.Confirm[0].EndMs--
	if err := ValidateStructureSplitShadowDecision(decision); err == nil {
		t.Fatal("mutated decision was accepted")
	}
}
