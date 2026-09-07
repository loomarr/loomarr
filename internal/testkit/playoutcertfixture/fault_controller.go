package playoutcertfixture

import "context"

// ScopedFaultController is a cycle-free test double for interfaces whose only
// contract is a stable, explicitly named disposable-target scope.
type ScopedFaultController struct {
	ScopeName string
}

func (c ScopedFaultController) Scope() string { return c.ScopeName }

// CleanupFailureTarget is a bounded isolated-target port for command cleanup
// tests. It makes no process or network calls.
type CleanupFailureTarget struct {
	Err error
}

func (c CleanupFailureTarget) Close(context.Context) error { return c.Err }
