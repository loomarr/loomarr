package filler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The split-review sweep (§10 V54). ⚠ Every assertion here is about NOT deleting something: this
// is the only code in Loomarr that removes an operator's media, so the tests are weighted toward
// the cases where it must keep its hands off.

type sweepMemStore struct {
	due      []SweepableProposal
	deleted  []string
	reaped   []string
	filed    []string
	failMark bool
}

func (m *sweepMemStore) ListSweepableSplitProposals(context.Context, time.Time) ([]SweepableProposal, error) {
	return m.due, nil
}
func (m *sweepMemStore) DeleteSplitProposal(_ context.Context, id string) error {
	m.deleted = append(m.deleted, id)
	return nil
}
func (m *sweepMemStore) MarkClipReaped(_ context.Context, hash string, _ time.Time) error {
	if m.failMark {
		return errors.New("boom")
	}
	m.reaped = append(m.reaped, hash)
	return nil
}
func (m *sweepMemStore) MarkPipelineFiled(_ context.Context, hash string, _ time.Time) error {
	m.filed = append(m.filed, hash)
	return nil
}

func sweeperFor(t *testing.T, st *sweepMemStore, window time.Duration) (*SplitSweeper, string) {
	t.Helper()
	dir := t.TempDir()
	return NewSplitSweeper(st, dir, func() time.Duration { return window }, time.Now, nil), dir
}

func writeReel(t *testing.T, dir, rel string) string {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("reel bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func TestSweep_RetiresAnExpiredReelAndReclaimsItsRecording(t *testing.T) {
	st := &sweepMemStore{due: []SweepableProposal{
		{ProposalID: "sp_1", ClipHash: "h1", ClipPath: "a1/b2/reel.mp4", Segments: 5},
	}}
	sw, dir := sweeperFor(t, st, 30*24*time.Hour)
	full := writeReel(t, dir, "a1/b2/reel.mp4")

	res, err := sw.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if res.Retired != 1 {
		t.Errorf("retired = %d, want 1", res.Retired)
	}
	if len(st.deleted) != 1 || st.deleted[0] != "sp_1" {
		t.Errorf("deleted = %v, want the proposal", st.deleted)
	}
	if len(st.reaped) != 1 {
		t.Errorf("reaped = %v, want the composite marked so sync does not prune its row", st.reaped)
	}
	if _, err := os.Stat(full); !os.IsNotExist(err) {
		t.Error("the recording is still on disk — the space was not reclaimed")
	}
}

// ⚠ **THE churn-loop guard.** Deleting the proposal alone does not finish the job: the composite is
// still `is_composite` and still on the belt, so the split rung re-detects it next pass — propose →
// partly confirm → leftovers → sweep → re-propose, burning a boundary scan every cycle forever. A
// sweep that skips this is worse than no sweep.
func TestSweep_TakesTheReelOffTheBeltSoItIsNotReproposed(t *testing.T) {
	st := &sweepMemStore{due: []SweepableProposal{
		{ProposalID: "sp_1", ClipHash: "h1", ClipPath: "a1/b2/reel.mp4"},
	}}
	sw, dir := sweeperFor(t, st, 30*24*time.Hour)
	writeReel(t, dir, "a1/b2/reel.mp4")

	if _, err := sw.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(st.filed) != 1 || st.filed[0] != "h1" {
		t.Fatalf("filed = %v — a swept reel left claimable is re-proposed every pass, forever", st.filed)
	}
}

// ⚠ 0s is OFF. An operator who has not chosen an expiry has not agreed to have recordings deleted.
func TestSweep_AZeroWindowDeletesNothing(t *testing.T) {
	st := &sweepMemStore{due: []SweepableProposal{{ProposalID: "sp_1", ClipHash: "h1", ClipPath: "r.mp4"}}}
	sw, dir := sweeperFor(t, st, 0)
	full := writeReel(t, dir, "r.mp4")

	res, err := sw.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if res.Retired != 0 || len(st.deleted) != 0 {
		t.Errorf("a zero window swept anyway: retired=%d deleted=%v", res.Retired, st.deleted)
	}
	if _, err := os.Stat(full); err != nil {
		t.Error("a zero window deleted a recording")
	}
}

// ⚠ **The ORDER is the correctness story.** The tombstone must be written BEFORE the unlink: a sync
// landing in between would find the file gone, prune the row, and dangle `parent_hash` on every
// clip cut out of that reel. So a failure to mark must abort before anything is removed.
func TestSweep_DoesNotDeleteTheRecordingIfTheTombstoneFails(t *testing.T) {
	st := &sweepMemStore{
		due:      []SweepableProposal{{ProposalID: "sp_1", ClipHash: "h1", ClipPath: "r.mp4"}},
		failMark: true,
	}
	sw, dir := sweeperFor(t, st, 30*24*time.Hour)
	full := writeReel(t, dir, "r.mp4")

	res, err := sw.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if res.Retired != 0 {
		t.Errorf("retired = %d, want 0 — the mark failed", res.Retired)
	}
	if _, err := os.Stat(full); err != nil {
		t.Fatal("the recording was deleted despite the tombstone failing — a sync would now dangle " +
			"parent_hash on every clip cut out of this reel")
	}
	if len(st.deleted) != 0 {
		t.Error("the proposal was deleted after the tombstone failed, losing the review work too")
	}
}

// A missing file is not a failure: the row is what the sweep is really retiring.
func TestSweep_ToleratesARecordingThatIsAlreadyGone(t *testing.T) {
	st := &sweepMemStore{due: []SweepableProposal{{ProposalID: "sp_1", ClipHash: "h1", ClipPath: "gone.mp4"}}}
	sw, _ := sweeperFor(t, st, 30*24*time.Hour)

	res, err := sw.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Retired != 1 {
		t.Errorf("retired = %d, want 1 — an already-missing file still leaves a row to retire", res.Retired)
	}
}
