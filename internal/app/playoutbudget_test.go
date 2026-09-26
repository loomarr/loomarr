package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/playout"
)

func TestPlayoutBudgetFacts(t *testing.T) {
	host := playout.HostCPU{Cores: 4, Source: "cgroup-v2"}
	measured := playoutBudgetFacts(host, true, 12, nil, nil, playout.TierBalanced, 1000, 1000)
	if measured.CPUAllowance != 1 || measured.CPUSource != "cgroup-v2" || measured.FirstRung != 0 {
		t.Fatalf("GPU host facts = %+v, want a 1-core allowance from the cgroup and the top rung", measured)
	}
	software := playoutBudgetFacts(host, false, 3, nil, nil, playout.TierBalanced, 1000, 1000)
	if software.CPUAllowance != 3 {
		t.Fatalf("software allowance = %v, want 4 cores less the 1-core app reserve", software.CPUAllowance)
	}
	// An unmeasured host must not be treated as unlimited: its first session takes the bottom rung.
	unmeasured := playoutBudgetFacts(host, false, 1, nil, nil, playout.TierBalanced, 1000, 1000)
	lease, err := playout.NewResourceBudget(func() playout.BudgetFacts { return unmeasured }).
		Admit(context.Background(), playout.AdmitRequest{Class: playout.ClassSDR})
	if err != nil {
		t.Fatal(err)
	}
	if got := playout.Resolve(playout.TierBalanced, playout.EncoderSoftware, lease.Rung()); got.Height != 720 || got.VideoBitrate != 1800 {
		t.Fatalf("unmeasured host profile = %dp @%dk, want the bottom rung 720p @1800k", got.Height, got.VideoBitrate)
	}
}

// Measured class costs replace the whole-stream count: the probed session limit applies, the first
// rung is the top one, and the operator cap and VRAM shading apply to the measured ceiling.
func TestPlayoutBudgetFacts_MeasuredCostsDriveAdmission(t *testing.T) {
	host := playout.HostCPU{Cores: 4, Source: "cgroup-v2"}
	costs := &playout.MeasuredCosts{Encoder: playout.EncoderNVENC, SessionLimit: 12, Costs: map[playout.CostKey]playout.ClassCost{
		{Class: playout.ClassSDR, Height: 1080}: {Speed: 19.18, CPUCores: 0.026},
	}}
	capAt := func(n int) int { return min(n, 5) } // playout.max_channels = 5
	f := playoutBudgetFacts(host, true, 1, costs, capAt, playout.TierBalanced, 1000, 1000)
	if f.SessionLimit != 12 || f.FirstRung != 0 || len(f.Costs) != 1 {
		t.Fatalf("facts = %+v, want the measured table, session limit 12 and the top rung", f)
	}
	if f.OperatorCap != 5 {
		t.Fatalf("OperatorCap = %d, want the operator cap applied to the measured ceiling (12)", f.OperatorCap)
	}
}

// Filler's media rungs wait on the ResourceBudget (#1512 G5, after #1514): busy only while a live
// transcode holds a lease. A copy holds no encoder and costs almost no CPU, so it does not hold
// filler back; without internal playout there is nothing to wait for.
func TestPlaybackHeadroom_IsTheResourceBudget(t *testing.T) {
	if playbackHeadroomFor(nil) != nil {
		t.Fatal("no internal playout must mean no filler headroom gate")
	}
	budget := playout.NewResourceBudget(nil)
	headroom := playbackHeadroomFor(budget)
	if busy, _ := headroom.PlaybackBusy(); busy {
		t.Fatal("an idle budget reported playback busy")
	}
	copyLease, err := budget.Reserve(playout.AdmitRequest{Class: playout.ClassCopy})
	if err != nil {
		t.Fatal(err)
	}
	defer copyLease.Release()
	if busy, _ := headroom.PlaybackBusy(); busy {
		t.Fatal("a copy session held filler back")
	}
	live, err := budget.Reserve(playout.AdmitRequest{Class: playout.ClassSDR})
	if err != nil {
		t.Fatal(err)
	}
	if busy, reason := headroom.PlaybackBusy(); !busy || reason == "" {
		t.Fatalf("a live transcode did not hold filler back (busy=%v, reason %q)", busy, reason)
	}
	live.Release()
	if busy, _ := headroom.PlaybackBusy(); busy {
		t.Fatal("filler stayed blocked after the transcode ended")
	}
}
