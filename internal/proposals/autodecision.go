package proposals

// AutoDecision records what an auto-approval grant did with one proposal, for the caller to log and
// for tests to assert on. The per-user grant and the channel auto-curate grant return the same shape.
type AutoDecision struct {
	Approved bool
	// Reason is why the grant did NOT approve, empty when it did. Recorded rather than
	// dropped so "why is this still in the queue?" is answerable.
	Reason   string
	Enqueued int
}
