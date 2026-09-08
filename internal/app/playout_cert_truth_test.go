package app

import (
	"context"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playoutcert"
	"testing"
	"time"
)

func TestSyntheticLiveTruthRejectsUnownedInitialAsset(t *testing.T) {
	now := time.Now().UTC()
	source := &syntheticProgrammeEvidence{schedule: syntheticProgrammeSchedule{epoch: now.Add(-time.Second), duration: 6 * time.Second}, prepared: map[string]bool{"live": false}, signatures: map[string][2]playoutcert.ProgrammeSignature{"live": defaultCertificationSignatures()}, clocks: make(map[string]syntheticLiveClock)}
	source.recordClock(t.Context(), "live", &playout.Process{}, 2, now.Add(-time.Second))
	evidence, err := source.Freeze("live", now, now.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	_, err = evidence.ResolveAsset(context.Background(), playoutcert.ProgrammeAsset{Reference: "nonexistent-stale-generation-segment.ts", Epoch: "retired-generation", StartedAt: now.Add(-time.Hour), Duration: 4 * time.Second})
	if err == nil {
		t.Fatal("unowned initial asset accepted with current source clock")
	}
}

func TestSyntheticLiveClockCannotBeReplacedByRetiredSource(t *testing.T) {
	source := &syntheticProgrammeEvidence{clocks: make(map[string]syntheticLiveClock)}
	now := time.Now().UTC()
	current, old := &playout.Process{}, &playout.Process{}
	source.recordClock(t.Context(), "live", current, 2, now)
	source.recordClock(t.Context(), "live", old, 1, now.Add(-time.Hour))
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	source.recordClock(canceled, "live", old, 3, now.Add(time.Hour))
	state := source.clocks["live"]
	if state.source != current || state.clock.Generation != "2" || !state.clock.Origin.Equal(now) {
		t.Fatal("a retired request replaced the current source clock")
	}
}
