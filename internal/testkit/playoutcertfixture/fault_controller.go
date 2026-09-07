package playoutcertfixture

import "context"

// ScopedFaultController is a cycle-free test double for interfaces whose only
// contract is a stable, explicitly named disposable-target scope.
type ScopedFaultController struct {
	ScopeName string
}

func (c ScopedFaultController) Scope() string { return c.ScopeName }

// ParentFaultTarget supplies the neutral mechanics for a public-stream parent
// fault test. Package users map their own request and receipt types at their
// boundary, avoiding an import cycle with the certification package.
type ParentFaultTarget struct {
	Fixture             *Fixture
	Peer                string
	WaitForExpiry       bool
	RetainAfterRecovery bool
}

func (c ParentFaultTarget) Current(ctx context.Context) (uint64, error) {
	return 1, ctx.Err()
}

func (c ParentFaultTarget) Fail(ctx context.Context, channelID string) error {
	_ = c.Fixture.FailSession(channelID)
	_ = c.Fixture.ContinueSession(c.Peer)
	if c.RetainAfterRecovery {
		c.Fixture.RetainAfterRecovery(channelID)
	}
	if c.WaitForExpiry {
		<-ctx.Done()
	}
	return nil
}

// CleanupFailureTarget is a bounded isolated-target port for command cleanup
// tests. It makes no process or network calls.
type CleanupFailureTarget struct {
	Err error
}

func (c CleanupFailureTarget) Close(context.Context) error { return c.Err }
