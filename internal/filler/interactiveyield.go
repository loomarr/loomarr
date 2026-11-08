package filler

import (
	"context"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/platform/llm"
)

// InteractiveYield bounds how long a background model call waits for interactive LLM work to clear
// (#1508), as PlaybackYield does for media rungs and live playback.
type InteractiveYield struct {
	// Poll is the wait between questions to the gate.
	Poll time.Duration
	// MaxWait is the most one call waits. Past it the call runs anyway: the backlog slows during a
	// long run of suggestions, but never stalls behind them.
	MaxWait time.Duration
	// Sleep is the wait itself; nil uses a context-aware timer. A seam for tests.
	Sleep func(context.Context, time.Duration) error
}

// DefaultInteractiveYield polls every second for up to 2 minutes: a suggestion takes 20-60 s.
func DefaultInteractiveYield() InteractiveYield {
	return InteractiveYield{Poll: time.Second, MaxWait: 2 * time.Minute}
}

func (y InteractiveYield) sleep(ctx context.Context, d time.Duration) error {
	return PlaybackYield{Sleep: y.Sleep}.sleep(ctx, d)
}

// YieldToInteractive makes every call to provider wait, bounded by y, while gate has interactive
// work in flight. Background vision (the vision rung and split-time grounding) goes through it, so a
// household member's Add a channel gets the AI server's slots first: on a small server one slot's
// vision prefill slows every other slot's decode. A nil gate never waits.
func YieldToInteractive(provider llm.VisionProvider, gate *llm.InteractiveGate, y InteractiveYield, log *slog.Logger) llm.VisionProvider {
	if provider == nil || gate == nil {
		return provider
	}
	if y.Poll <= 0 {
		y.Poll = DefaultInteractiveYield().Poll
	}
	if log == nil {
		log = slog.Default()
	}
	return interactiveYielding{provider: provider, gate: gate, yield: y, log: log}
}

type interactiveYielding struct {
	provider llm.VisionProvider
	gate     *llm.InteractiveGate
	yield    InteractiveYield
	log      *slog.Logger
}

func (v interactiveYielding) AskAboutImages(ctx context.Context, prompt string, jpegs [][]byte) (llm.Response, error) {
	if err := v.wait(ctx); err != nil {
		return llm.Response{}, err
	}
	return v.provider.AskAboutImages(ctx, prompt, jpegs)
}

// wait returns once the gate is idle or MaxWait has passed, or the context's error.
func (v interactiveYielding) wait(ctx context.Context) error {
	if !v.gate.Busy() {
		return nil
	}
	v.log.Debug("filler vision: yielding the model to interactive work", "max_wait", v.yield.MaxWait.String())
	for waited := time.Duration(0); waited < v.yield.MaxWait; {
		if err := v.yield.sleep(ctx, v.yield.Poll); err != nil {
			return err
		}
		waited += v.yield.Poll
		if !v.gate.Busy() {
			return nil
		}
	}
	v.log.Info("filler vision: interactive work stayed busy; running the model call anyway",
		"waited", v.yield.MaxWait.String())
	return ctx.Err()
}
