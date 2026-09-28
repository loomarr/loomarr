package filler_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// #1684: the household's readiness said channels MATCHED 14-16 clips, while the forecast aired 1-5
// distinct ones. Readiness reports the WIDEST rung (`Total`) and the TIGHTEST non-empty rung
// (`Level`); assembly drew only from that tightest rung. With the LLM off most clips have no
// grounded audience, so they sit on the bottom rung, and a channel with one grounded in-era clip
// played that one clip in every break.

// supplyCatalog is the household shape: `grounded` clips on the exact rung (grounded audience,
// in-era) and `untagged` clips that only the bottom rung admits.
func supplyCatalog(grounded, untagged int) []filler.Clip {
	var out []filler.Clip
	for i := 0; i < grounded; i++ {
		id := fmt.Sprintf("grounded-%02d", i)
		out = append(out, filler.Clip{Hash: id, Path: id + ".mp4", Kind: filler.Commercial,
			Era: 1994, Audience: filler.General, DurationMs: 30_000})
	}
	for i := 0; i < untagged; i++ {
		id := fmt.Sprintf("untagged-%02d", i)
		out = append(out, filler.Clip{Hash: id, Path: id + ".mp4", Kind: filler.Commercial,
			Era: 1994, DurationMs: 30_000})
	}
	return out
}

// supplyWindow is a 1990s general channel's break, sized the way PodAdapter sizes it: the
// 10-minute pool gap and the pod_max a 5-minute break of 30-second clips derives (10).
func supplyWindow(at time.Time, exposures map[string]filler.Exposure) filler.Window {
	return filler.Window{
		ChannelID: "channel", Seed: at.UnixMilli(), Era: filler.EraRange{From: 1990, To: 1999},
		Audience: filler.General, GapMs: 600_000, PodMax: 10, Exposures: exposures, SnapshotAt: at,
	}
}

// A day of breaks, two an hour, watched throughout so exposure is fed exactly as playout feeds it.
func TestLadder_OneGroundedClipDoesNotStarveTheChannel(t *testing.T) {
	cat := supplyCatalog(1, 15)
	policy := filler.Policy{Cooldown: 30 * time.Minute}
	start := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)
	exposures := map[string]filler.Exposure{}
	distinct := map[string]bool{}
	for at := start; at.Before(start.Add(24 * time.Hour)); at = at.Add(30 * time.Minute) {
		snapshot := make(map[string]filler.Exposure, len(exposures))
		for id, e := range exposures {
			snapshot[id] = e
		}
		pod := filler.Assemble(cat, supplyWindow(at, snapshot), policy, nil)
		ids := commercialIDs(pod)
		if len(ids) < 5 {
			t.Fatalf("%s: break aired %d commercials %v from a 16-clip pool — the pod stopped at the exact rung",
				at.Format(time.Kitchen), len(ids), ids)
		}
		for _, id := range ids {
			distinct[id] = true
			exposures[id] = filler.Exposure{PlayCount: exposures[id].PlayCount + 1, LastPlayedAt: at}
		}
	}
	if len(distinct) != len(cat) {
		t.Fatalf("a day aired %d distinct clips of the %d the channel matches", len(distinct), len(cat))
	}
}

// Topping up keeps the ladder's order: a grounded exact-era clip still opens the pod, and the
// untagged clips follow it rather than displacing it.
func TestLadder_TopUpKeepsTheGroundedClipFirst(t *testing.T) {
	cat := supplyCatalog(1, 3)
	at := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	pod := filler.Assemble(cat, supplyWindow(at, nil), filler.Policy{}, nil)
	ids := commercialIDs(pod)
	if len(ids) != 4 || ids[0] != "grounded-00" {
		t.Fatalf("pod = %v, want the grounded clip first and all three untagged clips after it", ids)
	}
	if pod.MatchLevel != filler.MatchAudience {
		t.Fatalf("match level = %s, want audience: the pod reached the bottom rung", pod.MatchLevel)
	}
}

// The V29 gate on the household shape: readiness read "exact" while every break was one clip. Now
// breaks reach the bottom rung, and the meter has to say so rather than report the rung that leads.
func TestLadder_CoverageLevelIsTheRungBreaksReach(t *testing.T) {
	cat := supplyCatalog(1, 15)
	w := supplyWindow(time.Time{}, nil)
	report := filler.Coverage(cat, w, filler.Policy{})
	pod := filler.Assemble(cat, w, filler.Policy{}, nil)
	if report.Level != filler.MatchAudience || pod.MatchLevel != report.Level {
		t.Fatalf("coverage %q, pod %q: want both audience", report.Level, pod.MatchLevel)
	}
	if report.Rungs[0].Clips != 1 || report.Total != 16 {
		t.Fatalf("rungs %+v total %d, want 1 exact of 16", report.Rungs, report.Total)
	}
}

// When the tight rung is resting inside its cooldown, a fresh clip from a wider rung plays first.
// Cooldown relaxes only when nothing else can fill the break, and a wider rung CAN.
func TestLadder_WidensBeforeRepeatingInsideCooldown(t *testing.T) {
	cat := supplyCatalog(1, 2)
	at := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	w := supplyWindow(at, map[string]filler.Exposure{
		"grounded-00": {PlayCount: 5, LastPlayedAt: at.Add(-10 * time.Minute)},
	})
	w.PodMax = 1
	pod := filler.Assemble(cat, w, filler.Policy{Cooldown: 30 * time.Minute}, nil)
	ids := commercialIDs(pod)
	if len(ids) != 1 || ids[0] == "grounded-00" {
		t.Fatalf("pod = %v, want a fresh untagged clip ahead of the grounded one inside its cooldown", ids)
	}
	if pod.CooldownRelaxed {
		t.Fatal("a wider rung filled the break, so no cooldown was relaxed")
	}

	// Every rung resting: the cooldown relaxes predictably, tight rung first.
	w.Exposures["untagged-00"] = filler.Exposure{PlayCount: 1, LastPlayedAt: at.Add(-5 * time.Minute)}
	w.Exposures["untagged-01"] = filler.Exposure{PlayCount: 1, LastPlayedAt: at.Add(-5 * time.Minute)}
	pod = filler.Assemble(cat, w, filler.Policy{Cooldown: 30 * time.Minute}, nil)
	if got := commercialIDs(pod); !reflect.DeepEqual(got, []string{"grounded-00"}) || !pod.CooldownRelaxed {
		t.Fatalf("all resting: pod = %v relaxed=%v, want the grounded clip with the cooldown relaxed", got, pod.CooldownRelaxed)
	}
}
