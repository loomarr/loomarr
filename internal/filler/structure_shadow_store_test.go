package filler_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// realShadowStore is the production adapter: the decision crosses a JSON column, so empty span
// groups (omitempty) come back as nil slices, unlike the in-memory adapter.
func realShadowStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+filepath.Join(t.TempDir(), "shadow.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func shadowProposal(t *testing.T, id string) (filler.SplitProposal, *filler.StructureSplitShadow, store.Store) {
	t.Helper()
	proposal, auto, materialization := filler.CertifiedShadowFixture(t)
	proposal.ID = id
	proposal.ClipHash = proposal.Source.ClipHash
	proposal.CreatedAt = time.Date(2026, time.September, 3, 13, 0, 0, 0, time.UTC)
	st := realShadowStore(t)
	shadow, err := filler.NewStructureSplitShadow(st, auto, materialization, func() time.Duration { return 10 * time.Second }, "candidate-v1")
	if err != nil {
		t.Fatal(err)
	}
	return proposal, shadow, st
}

// The certified fixture confirms every span, so Hold and Discard are empty on both sides: the
// exact production shape that failed with a false conflict.
func TestStructureSplitShadow_StoredDecisionWithEmptyGroupsIsNotAConflict(t *testing.T) {
	proposal, shadow, _ := shadowProposal(t, "proposal-1")
	auto := func() *filler.AutoSplitPolicy { _, a, _ := filler.CertifiedShadowFixture(t); return a }()
	legacy := filler.AutoConfirmable(proposal, auto, 10*time.Second)
	if err := shadow.ObserveStructureSplit(t.Context(), proposal, legacy); err != nil {
		t.Fatal(err)
	}
	pending, err := shadow.NeedsStructureSplitObservation(t.Context(), proposal)
	if err != nil || pending {
		t.Fatalf("observed proposal: pending = %v, err = %v; want false, nil", pending, err)
	}
}

// tamperedRepository returns the stored decision with one span edited: same id, different content.
type tamperedRepository struct{ store.Store }

func (r tamperedRepository) GetStructureSplitShadowDecision(ctx context.Context, id string) (filler.StructureSplitShadowDecision, bool, error) {
	d, found, err := r.Store.GetStructureSplitShadowDecision(ctx, id)
	if found && len(d.Certified.Confirm) > 0 {
		d.Certified.Confirm = append([]filler.StructureSplitShadowSpan(nil), d.Certified.Confirm...)
		d.Certified.Confirm[0].EndMs++
	}
	return d, found, err
}

func TestStructureSplitShadow_DifferentStoredDecisionStillConflicts(t *testing.T) {
	proposal, shadow, st := shadowProposal(t, "proposal-1")
	_, auto, materialization := filler.CertifiedShadowFixture(t)
	if err := shadow.ObserveStructureSplit(t.Context(), proposal, filler.AutoConfirmable(proposal, auto, 10*time.Second)); err != nil {
		t.Fatal(err)
	}
	tampered, err := filler.NewStructureSplitShadow(tamperedRepository{st}, auto, materialization, func() time.Duration { return 10 * time.Second }, "candidate-v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tampered.NeedsStructureSplitObservation(t.Context(), proposal); !errors.Is(err, filler.ErrStructureSplitShadowConflict) {
		t.Fatalf("err = %v, want ErrStructureSplitShadowConflict", err)
	}
}

type reviewQueue struct{ proposals []filler.SplitProposal }

func (q reviewQueue) ListSplitProposals(context.Context) ([]filler.SplitProposal, error) {
	return q.proposals, nil
}

// One observed proposal (empty groups) must not abort the resume scan for the unobserved one.
func TestPipeline_SplitReviewResumeRequeuesPendingDespiteObservedEmptyGroups(t *testing.T) {
	observed, shadow, _ := shadowProposal(t, "proposal-observed")
	_, auto, _ := filler.CertifiedShadowFixture(t)
	if err := shadow.ObserveStructureSplit(t.Context(), observed, filler.AutoConfirmable(observed, auto, 10*time.Second)); err != nil {
		t.Fatal(err)
	}
	pending := observed
	pending.ID, pending.ClipHash = "proposal-pending", "clip-pending"

	st := newPipeMemStore()
	for _, hash := range []string{observed.ClipHash, pending.ClipHash} {
		st.put(filler.StoreClip{Clip: filler.Clip{Hash: hash, Path: hash + ".mp4"}})
		st.rows[hash] = filler.ClipPipeline{
			ClipHash: hash, Stage: filler.StageSplit, Status: filler.StatusDone, Disposition: filler.DispositionReview,
			Stages: []filler.StageRecord{{Stage: filler.StageSplit, Status: filler.StatusDone}},
		}
	}
	stage := filler.NewSplitStage(nil, reviewQueue{[]filler.SplitProposal{observed, pending}}).WithStructureShadow(shadow)
	result, err := newPipe(st, []filler.Stage{stage}, filler.Budget{}).RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := st.rows[pending.ClipHash]; result.Requeued != 1 || got.Disposition != filler.DispositionRunning || got.Status != filler.StatusQueued {
		t.Fatalf("pending proposal not requeued: result=%+v row=%+v", result, got)
	}
	if got := st.rows[observed.ClipHash]; got.Disposition != filler.DispositionReview {
		t.Fatalf("observed proposal was requeued: %+v", got)
	}
}
