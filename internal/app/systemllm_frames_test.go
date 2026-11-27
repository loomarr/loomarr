package app

import (
	"testing"
	"time"

	opsapi "github.com/loomarr/loomarr/internal/api/ops"
	"github.com/loomarr/loomarr/internal/platform/events"
)

// pullFrames drains the llm_pull frames published so far.
func pullFrames(t *testing.T, ch <-chan events.Event) []opsapi.LLMPullEvent {
	t.Helper()
	var out []opsapi.LLMPullEvent
	for {
		select {
		case ev := <-ch:
			if ev.Type != "llm_pull" {
				t.Fatalf("frame type %q, want llm_pull", ev.Type)
			}
			out = append(out, ev.Payload.(opsapi.LLMPullEvent))
		default:
			return out
		}
	}
}

// Ollama's many samples a second become one frame a second per status, while every change of
// status and the outcome go out at once.
func TestLLMPullEventsPaceProgressButNeverAStatusChange(t *testing.T) {
	bus := events.NewBus()
	ch, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	at := time.Unix(1_900_000_000, 0)
	pulls := newLLMPullEvents(bus)
	pulls.now = func() time.Time { return at }

	pulls.PullProgress("job-1", "qwen3:8b", "starting", 0, 0, 0, "")
	for i := range 20 {
		at = at.Add(20 * time.Millisecond)
		pulls.PullProgress("job-1", "qwen3:8b", "pulling layer", i, int64(i), 100, "")
	}
	if got := pullFrames(t, ch); len(got) != 2 || got[0].Status != "starting" || got[1].Percent != 0 {
		t.Fatalf("frames = %+v, want starting and the layer's first sample only", got)
	}

	at = at.Add(llmPullFrameGap)
	pulls.PullProgress("job-1", "qwen3:8b", "pulling layer", 40, 40, 100, "")
	pulls.PullProgress("job-1", "qwen3:8b", "verifying sha256 digest", 99, 100, 100, "")
	pulls.PullProgress("job-1", "qwen3:8b", "success", 100, 100, 100, "")
	got := pullFrames(t, ch)
	if len(got) != 3 || got[0].Percent != 40 || got[1].Status != "verifying sha256 digest" || got[2].Status != "success" {
		t.Fatalf("frames = %+v, want the paced sample, the new status and success", got)
	}
	if len(pulls.seen) != 0 {
		t.Fatalf("seen = %v after success, want the finished download forgotten", pulls.seen)
	}
}

// Two downloads are paced apart: one's samples never hold back the other's.
func TestLLMPullEventsPaceEachDownloadOnItsOwn(t *testing.T) {
	bus := events.NewBus()
	ch, unsubscribe := bus.Subscribe()
	defer unsubscribe()
	at := time.Unix(1_900_000_000, 0)
	pulls := newLLMPullEvents(bus)
	pulls.now = func() time.Time { return at }

	pulls.PullProgress("job-1", "a", "pulling layer", 1, 1, 10, "")
	pulls.PullProgress("job-2", "b", "pulling layer", 1, 1, 10, "")
	pulls.PullProgress("job-1", "a", "error", 1, 1, 10, "disk full")
	got := pullFrames(t, ch)
	if len(got) != 3 || got[1].JobID != "job-2" || got[2].Error != "disk full" {
		t.Fatalf("frames = %+v, want both downloads' samples and the error", got)
	}
}
