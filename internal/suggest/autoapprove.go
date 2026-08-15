package suggest

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mantonx/loomarr/internal/store"
)

// AutoApprovedBy is the audit value recorded when the grant, not a human, approved a
// proposal (§11: approvals are audited). Deliberately not a user id: the trail must be
// able to answer "did a person decide this?" without inferring it.
const AutoApprovedBy = "auto"

// AutoCuratedBy is the audit value recorded when the §8.2 scheduled re-curation, authorized
// by a channel's AutoCurate opt-in, approved a proposal. Distinct from AutoApprovedBy so the
// trail separates "a user's auto-approve grant" from "the channel's auto-curate loop".
const AutoCuratedBy = "auto-curate"

// GrantStore reads the requester's grant + cap.
type GrantStore interface {
	GetUser(ctx context.Context, id string) (store.User, error)
}

// AutoApprover decides whether a freshly-produced proposal may skip the admin queue.
//
// §8 makes the rule explicit: `auto_approve` is a per-user grant HARD-GATED by the
// pending-acquisition cap. So a holder's proposal auto-approves only while it keeps them
// within quota; over it, the proposal stays `submitted` and waits for an admin. It is
// never DENIED for quota — a cap is a bound on unattended spending, not a judgement about
// the request, and denying would lose work a human might well approve.
type AutoApprover struct {
	store    autoStore
	approver *Approver
	defaults func(ctx context.Context) int // suggest.max_acquisitions
	log      *slog.Logger
}

type autoStore interface {
	TitleReader
	QuotaStore
	GrantStore
}

// NewAutoApprover wires the grant. `defaultLimit` supplies suggest.max_acquisitions,
// read per call so a settings change takes effect without a restart (config-design §3).
func NewAutoApprover(
	st autoStore,
	approver *Approver,
	defaultLimit func(ctx context.Context) int,
	log *slog.Logger,
) *AutoApprover {
	return &AutoApprover{store: st, approver: approver, defaults: defaultLimit, log: log}
}

// Decision records what the grant did, for the caller to log and for tests to assert on.
type Decision struct {
	Approved bool
	// Reason is why it did NOT auto-approve, empty when it did. Recorded rather than
	// dropped so "why is this still in the queue?" is answerable.
	Reason   string
	Enqueued int
}

// Consider applies the grant to a just-created proposal.
func (a *AutoApprover) Consider(ctx context.Context, p store.Proposal) (Decision, error) {
	if a == nil || p.CreatedBy == "" {
		return Decision{Reason: "no requester"}, nil
	}

	user, err := a.store.GetUser(ctx, p.CreatedBy)
	if err != nil {
		// An unreadable user is not a reason to auto-approve. Fail CLOSED: the proposal
		// waits for an admin, which is the safe direction for a gate that spends money.
		return Decision{Reason: "requester unavailable"}, nil
	}
	if !user.AutoApprove {
		return Decision{Reason: "no auto-approve grant"}, nil
	}
	if user.Disabled {
		return Decision{Reason: "requester is disabled"}, nil
	}

	limit := user.Quota
	if limit <= 0 && a.defaults != nil {
		limit = a.defaults(ctx) // §11: 0 ⇒ the suggest.max_acquisitions default
	}

	usage, err := PendingFor(ctx, a.store, p.CreatedBy, limit)
	if err != nil {
		return Decision{Reason: "quota unavailable"}, err
	}
	// Only NEW acquisitions count: a title someone else already caused costs this user
	// nothing, and charging for it would make the cap depend on unrelated activity.
	wanted, err := NewAcquisitions(ctx, a.store, p)
	if err != nil {
		return Decision{Reason: "proposal unreadable"}, err
	}
	if usage.Exceeded(wanted) {
		return Decision{Reason: "over the pending-acquisition cap"}, nil
	}

	// nil edit: an auto-approval takes the proposal exactly as the model produced it. There is
	// no approver to make a judgement, which is the point of the grant — and it runs the SAME
	// Approve as the manual path, so the two cannot drift on what approving means (§8).
	if a.approver == nil {
		return Decision{Reason: "approval unavailable"}, errors.New("auto-approve: approval gate is not configured")
	}
	result, err := a.approver.Approve(ctx, p, nil, AutoApprovedBy)
	if err != nil {
		return Decision{Reason: "approval failed"}, err
	}
	return Decision{Approved: true, Enqueued: result.Enqueued}, nil
}
