package testkit

import (
	"context"
	"sync"
)

// CancelAfterErrChecks reports a live context for the requested number of Err
// checks, then cancels it. It lets tests exercise cancellation at a specific
// downstream context boundary without relying on scheduling or sleeps.
func CancelAfterErrChecks(parent context.Context, checks int) context.Context {
	ctx, cancel := context.WithCancel(parent)
	return &cancelAfterErrChecks{Context: ctx, cancel: cancel, checks: checks}
}

type cancelAfterErrChecks struct {
	context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	checks int
}

func (ctx *cancelAfterErrChecks) Err() error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if err := ctx.Context.Err(); err != nil {
		return err
	}
	if ctx.checks == 0 {
		ctx.cancel()
		return ctx.Context.Err()
	}
	ctx.checks--
	return nil
}
