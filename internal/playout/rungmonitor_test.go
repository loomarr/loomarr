package playout

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// phase is a stretch of a scripted encoder: ffmpeg's realtime speed and the cores it burns.
type phase struct {
	dur   time.Duration
	speed float64
	cores float64 // 0: the caller has no CPU reading
}

// decision is one step the monitor asked for, and when (encoder wall time since the script began).
type decision struct {
	at   time.Duration
	rung SoftwareRung
}

// runScript renders the phases as ffmpeg's -progress protocol (a block every 500 ms, out_time_ms
// in microseconds as ffmpeg writes it), parses it with the production parser, and feeds each block
// to the monitor with the script's wall clock. A step restarts the encoder: its out_time and CPU
// counters start again from zero, exactly as a new process's do.
func runScript(t *testing.T, m *RungMonitor, phases ...phase) []decision {
	t.Helper()
	const period = 500 * time.Millisecond
	start := time.Unix(1_700_000_000, 0)
	var (
		got      []decision
		wall     time.Duration
		out, cpu float64 // seconds, since the current encoder started
		frame    int
	)
	for _, p := range phases {
		for n := time.Duration(0); n < p.dur; n += period {
			wall += period
			out += p.speed * period.Seconds()
			cpu += p.cores * period.Seconds()
			frame += int(p.speed * 12.5)
			block := fmt.Sprintf("frame=%d\nfps=25.00\nout_time_us=%d\nout_time_ms=%d\nout_time=00:00:00.000000\n"+
				"dup_frames=0\ndrop_frames=0\nspeed=%.3gx\nprogress=continue\n",
				frame, int64(out*1e6), int64(out*1e6), out/wall.Seconds())
			var parsed []Progress
			ReadProgress(io.NopCloser(strings.NewReader(block)), func(pr Progress) { parsed = append(parsed, pr) })
			if len(parsed) != 1 {
				t.Fatalf("the progress parser returned %d blocks for %q", len(parsed), block)
			}
			s := SpeedSample{At: start.Add(wall), OutTime: time.Duration(parsed[0].OutTimeMS) * time.Millisecond}
			if p.cores > 0 {
				s.CPU = time.Duration(cpu * float64(time.Second))
			}
			if d := m.Observe(s); d.Step {
				got = append(got, decision{wall, d.Rung})
				out, cpu, frame = 0, 0, 0 // the encoder restarted on the new rung
			}
		}
	}
	return got
}

func testMonitor(start SoftwareRung) *RungMonitor {
	return NewRungMonitor(start, RungMonitorConfig{CPUAllowance: 4})
}

func TestRungMonitor_HealthyPacedEncoderHolds(t *testing.T) {
	// Paced input (-readrate 1.0) wobbles around 1.0x; that is not a reason to degrade.
	var ph []phase
	for i := 0; i < 60; i++ {
		ph = append(ph, phase{time.Second, 0.97 + 0.06*float64(i%2), 0})
	}
	if got := runScript(t, testMonitor(RungFull), ph...); len(got) != 0 {
		t.Fatalf("a healthy encoder stepped: %+v", got)
	}
}

func TestRungMonitor_StepsDownWhenSustainedBelowRealtime(t *testing.T) {
	got := runScript(t, testMonitor(RungFull), phase{2 * time.Minute, 0.85, 0})
	if len(got) == 0 || got[0].rung != RungLight {
		t.Fatalf("want a step to %s, got %+v", RungLight, got)
	}
	// Settle 3 s, then the 4 s window, then 8 s continuously below.
	if got[0].at < 11*time.Second || got[0].at > 16*time.Second {
		t.Errorf("stepped at %s; want after settle + window + 8 s and promptly", got[0].at)
	}
}

func TestRungMonitor_ShortHiccupDoesNotStep(t *testing.T) {
	got := runScript(t, testMonitor(RungFull),
		phase{20 * time.Second, 1, 0}, phase{3 * time.Second, 0.3, 0}, phase{40 * time.Second, 1, 0})
	if len(got) != 0 {
		t.Fatalf("a 3 s read hiccup restarted the encoder: %+v", got)
	}
}

func TestRungMonitor_SlowDriftStepsDownOnLag(t *testing.T) {
	// 0.98x never looks slow in a window, but it falls a second behind every 50 s and stalls the
	// packager eventually. The accumulated lag catches it.
	got := runScript(t, testMonitor(RungFull), phase{5 * time.Minute, 0.98, 0})
	if len(got) == 0 || got[0].rung != RungLight {
		t.Fatalf("want a lag step to %s, got %+v", RungLight, got)
	}
	if got[0].at > 3*time.Minute {
		t.Errorf("stepped at %s; 3 s of lag at 0.98x accrues by ~2.5 min", got[0].at)
	}
}

func TestRungMonitor_WalksTheLadderToKeyframesAndStops(t *testing.T) {
	got := runScript(t, testMonitor(RungFull), phase{5 * time.Minute, 0.6, 0})
	want := []SoftwareRung{RungLight, RungNoRef, RungKeyframes}
	if len(got) != len(want) {
		t.Fatalf("want steps %v, got %+v", want, got)
	}
	for i := range want {
		if got[i].rung != want[i] {
			t.Errorf("step %d: %s, want %s", i, got[i].rung, want[i])
		}
	}
}

func TestRungMonitor_SkipsNoRefWhereTheSourceDoesNotGain(t *testing.T) {
	// H1 in phase 0b: noref decodes every frame anyway (no gain), so the ladder records that and
	// the next descent from rung 1 goes straight to keyframes-only.
	m := testMonitor(RungLight)
	got := runScript(t, m,
		phase{20 * time.Second, 0.9, 3.9},  // rung 1: slow → noref
		phase{20 * time.Second, 0.92, 3.9}, // rung 2: no measurable gain → keyframes
		phase{70 * time.Second, 1, 0.3},    // rung 3: 0.3 cores of 4 → room for rung 1 (up at ~102 s, after the 60 s dwell)
		phase{40 * time.Second, 0.9, 3.9})  // rung 1 again: slow → straight to keyframes
	want := []SoftwareRung{RungNoRef, RungKeyframes, RungLight, RungKeyframes}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %+v", want, got)
	}
	for i := range want {
		if got[i].rung != want[i] {
			t.Errorf("step %d: %s, want %s (all %+v)", i, got[i].rung, want[i], got)
		}
	}
}

func TestRungMonitor_KeepsNoRefWhereTheSourceGains(t *testing.T) {
	// H2 in phase 0b: noref cut the cores 3.23 → 1.28.
	got := runScript(t, testMonitor(RungLight),
		phase{20 * time.Second, 0.9, 3.9}, phase{3 * time.Minute, 1, 2.6})
	if len(got) != 1 || got[0].rung != RungNoRef {
		t.Fatalf("want one step to %s and a hold, got %+v", RungNoRef, got)
	}
}

func TestRungMonitor_StepsUpOnlyWithHeadroomForTheNextRung(t *testing.T) {
	// 4K HDR at keyframes-only: rung 1 costs 0.95/0.12 = 7.9x as much, so the bar is 9.5x headroom
	// (7.9 × 1.2). 0.5 cores of 4 is 8x: hold. 0.3 cores is 13.3x: up, after the 60 s dwell.
	if got := runScript(t, testMonitor(RungKeyframes), phase{3 * time.Minute, 1, 0.5}); len(got) != 0 {
		t.Fatalf("stepped up without headroom for the next rung: %+v", got)
	}
	got := runScript(t, testMonitor(RungKeyframes), phase{3 * time.Minute, 1, 0.3})
	if len(got) == 0 || got[0].rung != RungLight {
		t.Fatalf("want a step up to %s, got %+v", RungLight, got)
	}
	if got[0].at < 60*time.Second {
		t.Errorf("stepped up at %s; want the 60 s minimum dwell first", got[0].at)
	}
}

// TestRungMonitor_StepUpBarIsTheClassCostRatio: the same 8x headroom holds a 4K HDR item on
// keyframes-only (rung 1 costs 7.9x as much) but lifts a 1080p SDR item (4.3x: bar 5.2x).
func TestRungMonitor_StepUpBarIsTheClassCostRatio(t *testing.T) {
	hdr := NewRungMonitor(RungKeyframes, RungMonitorConfig{CPUAllowance: 4, Costs: RungCostsFor(testSources()["hevc-4k-hdr-dv"])})
	if got := runScript(t, hdr, phase{3 * time.Minute, 1, 0.5}); len(got) != 0 {
		t.Fatalf("4K HDR stepped up on 8x headroom: %+v", got)
	}
	sdr := NewRungMonitor(RungKeyframes, RungMonitorConfig{CPUAllowance: 4, Costs: RungCostsFor(testSources()["hevc10-1080p"])})
	if got := runScript(t, sdr, phase{3 * time.Minute, 1, 0.5}); len(got) == 0 || got[0].rung != RungLight {
		t.Fatalf("1080p SDR held keyframes-only on 8x headroom: %+v", got)
	}
}

// TestRungMonitor_HysteresisAtTheBar: headroom that dips under the bar restarts the dwell, so
// capacity hovering at the threshold never steps.
func TestRungMonitor_HysteresisAtTheBar(t *testing.T) {
	var ph []phase
	for i := 0; i < 8; i++ { // 50 s above the bar (0.3 cores), 10 s under it (0.5 cores), repeated
		ph = append(ph, phase{50 * time.Second, 1, 0.3}, phase{10 * time.Second, 1, 0.5})
	}
	if got := runScript(t, testMonitor(RungKeyframes), ph...); len(got) != 0 {
		t.Fatalf("stepped up without 60 s continuously above the bar: %+v", got)
	}
}

func TestRungMonitor_NoCPUReadingNeverStepsUp(t *testing.T) {
	// Paced input caps speed= at 1.0x, so without CPU time headroom is invisible: hold, don't guess.
	if got := runScript(t, testMonitor(RungKeyframes), phase{5 * time.Minute, 1, 0}); len(got) != 0 {
		t.Fatalf("stepped up blind: %+v", got)
	}
}

func TestRungMonitor_BacksOffAfterAFailedStepUp(t *testing.T) {
	// The projection is a prior; when the upper rung proves too slow the next attempt waits longer.
	got := runScript(t, testMonitor(RungLight),
		phase{70 * time.Second, 1, 0.7},  // rung 1 with room → rung 0 after the 60 s dwell
		phase{20 * time.Second, 0.8, 4},  // rung 0 too slow → back to 1
		phase{100 * time.Second, 1, 0.7}) // room again, but the wait has doubled to 120 s
	want := []SoftwareRung{RungFull, RungLight}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %+v", want, got)
	}
}

func TestRungMonitor_FloorAndCeiling(t *testing.T) {
	if got := runScript(t, testMonitor(RungKeyframes), phase{2 * time.Minute, 0.5, 4}); len(got) != 0 {
		t.Fatalf("stepped below the floor: %+v", got)
	}
	if got := runScript(t, testMonitor(RungFull), phase{2 * time.Minute, 1, 0.2}); len(got) != 0 {
		t.Fatalf("stepped above the ceiling: %+v", got)
	}
}
