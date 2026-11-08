package filler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/platform/llm"
)

// countingVision answers every call and records how much of the test's clock had passed when each
// call reached the model.
type countingVision struct {
	clock *time.Duration
	at    []time.Duration
}

func (v *countingVision) AskAboutImages(context.Context, string, [][]byte) (llm.Response, error) {
	v.at = append(v.at, *v.clock)
	return llm.Response{Content: "{}"}, nil
}

// Background vision yields its model call to interactive suggestion work (#1508): on a 2-slot AI
// server a vision prefill halved a suggestion's decode speed, and Add a channel took 52 s instead of
// ~25 s. The call waits while a suggestion is in flight, starts the moment it ends, and never waits
// past MaxWait, so a long run of suggestions defers the backlog rather than stalling it.
func TestYieldToInteractive_WaitsForInteractiveWorkBeforeEachModelCall(t *testing.T) {
	gate := &llm.InteractiveGate{}
	var clock time.Duration
	var endAt time.Duration
	var end func()
	sleep := func(_ context.Context, d time.Duration) error {
		clock += d
		if end != nil && clock >= endAt {
			end()
			end = nil
		}
		return nil
	}
	inner := &countingVision{clock: &clock}
	v := filler.YieldToInteractive(inner, gate, filler.InteractiveYield{Poll: time.Second, MaxWait: time.Minute, Sleep: sleep}, nil)

	// Idle: the call goes straight to the model.
	if _, err := v.AskAboutImages(t.Context(), "p", nil); err != nil || len(inner.at) != 1 || inner.at[0] != 0 {
		t.Fatalf("idle call: err %v, calls at %v; want one immediate call", err, inner.at)
	}

	// A suggestion in flight for 5 s: the call waits it out, and goes as soon as it ends.
	end, endAt = gate.Begin(), clock+5*time.Second
	if _, err := v.AskAboutImages(t.Context(), "p", nil); err != nil || len(inner.at) != 2 || inner.at[1] != 5*time.Second {
		t.Fatalf("busy call: err %v, calls at %v; want the second call once the suggestion ended at 5s", err, inner.at)
	}

	// Suggestions that never end: the call waits MaxWait, then runs anyway.
	stuck := gate.Begin()
	defer stuck()
	start := clock
	if _, err := v.AskAboutImages(t.Context(), "p", nil); err != nil || len(inner.at) != 3 || inner.at[2]-start != time.Minute {
		t.Fatalf("stuck call: err %v, calls at %v; want the call after MaxWait (1m)", err, inner.at)
	}
}

// A pass that ends while its call is waiting makes no call.
func TestYieldToInteractive_CancelledWhileWaitingMakesNoCall(t *testing.T) {
	gate := &llm.InteractiveGate{}
	defer gate.Begin()()
	var clock time.Duration
	inner := &countingVision{clock: &clock}
	ctx, cancel := context.WithCancel(t.Context())
	sleep := func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	v := filler.YieldToInteractive(inner, gate, filler.InteractiveYield{Poll: time.Second, MaxWait: time.Minute, Sleep: sleep}, nil)
	if _, err := v.AskAboutImages(ctx, "p", nil); !errors.Is(err, context.Canceled) || len(inner.at) != 0 {
		t.Fatalf("cancelled wait: err %v, calls %d; want context.Canceled and no call", err, len(inner.at))
	}
}
