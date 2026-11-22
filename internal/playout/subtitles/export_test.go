package subtitles

import (
	"context"
	"time"
)

// Step transcribes the most urgent wanted block, if any, and reports whether it ran one: the
// worker's loop body, driven one block at a time by the tests.
func (g *Generator) Step(ctx context.Context) bool { return g.step(ctx) }

// SetClock replaces the Generator's clock.
func (g *Generator) SetClock(now func() time.Time) { g.now = now }

// SetHostedTimeout replaces the bound on one block's hosted call.
func (g *Generator) SetHostedTimeout(d time.Duration) { g.hostedTimeout = d }
