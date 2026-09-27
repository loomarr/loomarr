# 0005. Proposals need human approval by default

- **Date:** 2026-07-13
- **Status:** accepted

## Context

An approved Proposal spends real resources: it requests downloads and creates or rewrites a channel.
The Proposal comes from a model, and members can submit them.

## Decision

Proposals are never auto-executed. A user with the approve permission confirms before anything is
acquired or scheduled, and `approved_by` records who. `auto_approve` is an optional per-user grant,
hard-gated by that requester's pending-acquisition cap: over quota, the Proposal falls back to the
admin queue instead of being denied, and `approved_by` records `auto`.

## Consequences

- Approval is the only authority that creates acquisitions and materialises the intent-bound
  Channel, and it is one atomic decision.
- The audit trail distinguishes machine decisions from human ones.
- Approvals for one requester are ordered by the store so the quota check cannot race; only
  automatic approval can be refused by the cap, because a manual admin approval *is* the gate.
