package playoutcertfixture

// ScopedFaultController is a cycle-free test double for interfaces whose only
// contract is a stable, explicitly named disposable-target scope.
type ScopedFaultController struct {
	ScopeName string
}

func (c ScopedFaultController) Scope() string { return c.ScopeName }
