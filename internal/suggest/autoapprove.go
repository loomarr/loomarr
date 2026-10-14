package suggest

import (
	"context"
	"errors"
	"log/slog"

	"github.com/loomarr/loomarr/internal/store"
)

// AutoApprovedBy is the audit value recorded when the grant, not a human, approved a
// proposal (§11: approvals are audited). Deliberately not a user id: the trail must be
// able to answer "did a person decide this?" without inferring it.
const AutoApprovedBy = "auto"

// AutoCuratedBy is the audit value recorded when the §8.2 scheduled re-curation, authorized
// by a channel's AutoCurate opt-in, approved a proposal. Distinct from AutoApprovedBy so the
// trail separates "a user's auto-approve grant" from "the channel's auto-curate loop".
const AutoCuratedBy = "auto-curate"

// guardedApprovalStore is the transaction-bound seam for one requester's
// unattended-spend decision. The store supplies quota reads from the same
// transaction/connection that performs the durable approval commit.
type guardedApprovalStore interface {
	CommitProposalApprovalGuarded(context.Context, store.ProposalApproval, store.ProposalApprovalGuard) (int, error)
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
	guardedApprovalStore
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
	if a.approver == nil {
		return Decision{Reason: "approval unavailable"}, errors.New("auto-approve: approval gate is not configured")
	}

	decision := Decision{}
	guardRan := false
	guard := func(guardCtx context.Context, view store.ProposalApprovalReader) error {
		guardRan = true
		user, err := view.GetUser(guardCtx, p.CreatedBy)
		if err != nil {
			// An unreadable user is not a reason to auto-approve. Fail CLOSED: the proposal
			// waits for an admin, which is the safe direction for a gate that spends money.
			decision.Reason = "requester unavailable"
			return errAutoApprovalDeclined
		}
		if !user.AutoApprove {
			decision.Reason = "no auto-approve grant"
			return errAutoApprovalDeclined
		}
		if user.Disabled {
			decision.Reason = "requester is disabled"
			return errAutoApprovalDeclined
		}

		limit := user.Quota
		if limit <= 0 && a.defaults != nil {
			limit = a.defaults(guardCtx) // §11: 0 ⇒ the suggest.max_acquisitions default
		}

		usage, err := PendingFor(guardCtx, view, p.CreatedBy, limit)
		if err != nil {
			decision.Reason = "quota unavailable"
			return err
		}
		// Only NEW acquisitions count: a title someone else already caused costs this user
		// nothing, and charging for it would make the cap depend on unrelated activity.
		wanted, err := NewAcquisitions(guardCtx, view, p)
		if err != nil {
			decision.Reason = "proposal unreadable"
			return err
		}
		if usage.Exceeded(wanted) {
			decision.Reason = "over the pending-acquisition cap"
			return errAutoApprovalDeclined
		}
		return nil
	}

	// nil edit: an auto-approval takes the proposal exactly as the model produced it. There is
	// no approver to make a judgement, which is the point of the grant — and it runs the SAME
	// durable approval planner as the manual path (§8). Only the commit adapter differs: it adds
	// the transaction-bound quota guard.
	result, err := a.approver.approveDurably(ctx, p, nil, AutoApprovedBy,
		func(commitCtx context.Context, commit store.ProposalApproval) (int, error) {
			return a.store.CommitProposalApprovalGuarded(commitCtx, commit, guard)
		})
	if errors.Is(err, errAutoApprovalDeclined) {
		return decision, nil
	}
	if err != nil {
		if decision.Reason == "" {
			if guardRan {
				decision.Reason = "approval failed"
			} else {
				decision.Reason = "quota unavailable"
			}
		}
		return decision, err
	}

	decision.Approved = true
	decision.Enqueued = result.Enqueued
	a.approver.afterApprovalCommitted(ctx, result.ChannelID)
	return decision, nil
}

var errAutoApprovalDeclined = errors.New("auto-approval safely declined")
