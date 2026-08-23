package api

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/prepared"
)

type fixedPreparedObserver struct{ status prepared.PlannerStatus }

func (f fixedPreparedObserver) Status() prepared.PlannerStatus { return f.status }

// The verdict is the doctor's whole point — mapping a raw speed to ok|degraded|stalled with a
// reason an operator can act on. These cases pin the thresholds and the copy-is-always-ok rule.
func TestVerdict(t *testing.T) {
	cases := []struct {
		name       string
		copying    bool
		speed      float64
		hardware   bool
		wantHealth PlayoutHealth
	}{
		// A copy does no encoding, so its speed is meaningless — always OK, even at a "slow" number.
		{"copy is always ok, ignores speed", true, 0.1, false, HealthOK},
		{"copy is ok at high speed too", true, 50, true, HealthOK},

		// A transcode with no sample yet is warming up, not stalled.
		{"transcode, no sample yet", false, 0, true, HealthOK},

		// Below realtime = stalled, on hardware OR software.
		{"transcode stalled on software", false, 0.7, false, HealthStalled},
		{"transcode stalled on hardware (starved)", false, 0.9, true, HealthStalled},

		// Exactly 1.0× is the knife-edge: the buffer neither fills nor drains, so it is precarious
		// (degraded) rather than losing (stalled). Below 1.0× is where it actually falls behind.
		{"transcode exactly realtime is degraded (knife-edge)", false, 1.0, true, HealthDegraded},

		// Between 1.0 and 1.3 = degraded (playing, little margin).
		{"transcode just above realtime is degraded", false, 1.1, true, HealthDegraded},
		{"transcode near the degraded ceiling", false, 1.29, true, HealthDegraded},

		// At or above 1.3 = ok.
		{"transcode with margin is ok", false, 1.3, true, HealthOK},
		{"transcode fast is ok", false, 3.0, true, HealthOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := verdict(c.copying, c.speed, c.hardware)
			if got != c.wantHealth {
				t.Errorf("verdict(copy=%v speed=%v hw=%v) = %q, want %q", c.copying, c.speed, c.hardware, got, c.wantHealth)
			}
			if reason == "" {
				t.Error("every verdict must carry an actionable reason, got empty")
			}
		})
	}
}

// A stalled HARDWARE transcode must point at the GPU/VRAM in its reason — that is the actionable
// difference from a software stall (the operator checks VRAM contention, not CPU load).
func TestVerdict_StalledHardwarePointsAtGPU(t *testing.T) {
	_, reason := verdict(false, 0.8, true)
	if !contains(reason, "GPU") && !contains(reason, "VRAM") {
		t.Errorf("a stalled hardware transcode should point at GPU/VRAM, got %q", reason)
	}
}

// channelHealthFrom maps a raw SessionStat to a row, deriving mode from the encoder.
func TestChannelHealthFrom_ModeFromEncoder(t *testing.T) {
	copyRow := channelHealthFrom(playout.SessionStat{ChannelID: "c", Encoder: "copy", Speed: 0})
	if copyRow.Mode != "direct-play" || copyRow.Health != HealthOK {
		t.Errorf("copy encoder → direct-play + ok, got mode=%q health=%q", copyRow.Mode, copyRow.Health)
	}
	txRow := channelHealthFrom(playout.SessionStat{ChannelID: "c", Encoder: "h264_vulkan", Hardware: true, Speed: 2.0})
	if txRow.Mode != "transcode" || txRow.Health != HealthOK {
		t.Errorf("hardware encoder at 2× → transcode + ok, got mode=%q health=%q", txRow.Mode, txRow.Health)
	}
}

func TestPlayoutStatusProjectsPreparedPlannerSnapshot(t *testing.T) {
	completed := time.Unix(50_000, 0)
	s := &Server{preparedObserver: fixedPreparedObserver{status: prepared.PlannerStatus{
		Available: true, Running: true, LastRunAt: completed, LastError: "one source failed",
		Readiness: prepared.ReadinessSummary{
			Channels: 100, ReadyChannels: 84,
			ScheduledBindings: 300, ReadyBindings: 260, MissingBindings: 40,
			QueuedPublications: 16,
		},
		Retention: prepared.RetentionStatus{
			RemainingBytes: 700, BudgetBytes: 1_000, ProtectedBytes: 600,
		},
	}}}

	got := s.playoutStatus(t.Context(), completed)
	if !got.Prepared.Available || !got.Prepared.Running || got.Prepared.LastRunAt == nil ||
		!got.Prepared.LastRunAt.Equal(completed) || got.Prepared.LastError != "one source failed" {
		t.Fatalf("prepared lifecycle = %+v", got.Prepared)
	}
	if got.Prepared.Channels != 100 || got.Prepared.ReadyChannels != 84 ||
		got.Prepared.ScheduledBindings != 300 || got.Prepared.ReadyBindings != 260 ||
		got.Prepared.MissingBindings != 40 || got.Prepared.QueuedPublications != 16 {
		t.Fatalf("prepared readiness = %+v", got.Prepared)
	}
	if got.Prepared.RemainingBytes != 700 || got.Prepared.BudgetBytes != 1_000 ||
		got.Prepared.ProtectedBytes != 600 {
		t.Fatalf("prepared retention = %+v", got.Prepared)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
