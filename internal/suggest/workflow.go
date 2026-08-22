package suggest

import (
	"context"
	"time"
)

// WorkflowWork is the activity input issued by the durable proposal workflow.
// Version plus Attempt form the compare-and-swap token for terminal writes.
type WorkflowWork struct {
	Version   int
	JobID     string
	Attempt   int
	Kind      string
	CreatedBy string
	Intent    Intent
}

// WorkflowProposal is the durably committed Proposal returned to the optional
// approval grants after generation succeeds.
type WorkflowProposal struct {
	ID        string
	JobID     string
	CreatedBy string
	Proposal  Proposal
	CreatedAt time.Time
}

// DurableWorkflow is the worker-facing deep-module seam. The suggest package
// owns model/catalog activities; it does not own lifecycle transitions.
type DurableWorkflow interface {
	Claim(context.Context, time.Time, time.Duration, int) ([]WorkflowWork, error)
	Complete(context.Context, WorkflowWork, Proposal) (WorkflowProposal, error)
	Fail(context.Context, WorkflowWork, string, string) error
}
