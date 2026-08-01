-- +goose Up
-- V35: filler acquisition gets the approval gate that title acquisition already has.
--
-- §10 has stated the principle since the starter pack shipped — "the machine proposes, a human
-- commits" — with nothing to hang it on: the only things that existed were a listing endpoint
-- and a download button. A PULL is that object. It is a plan Loomarr composed across sources
-- ("fill the 1990s kids gap" resolving to several collections, each with a reason and an
-- estimate), and NOTHING DOWNLOADS UNTIL IT IS APPROVED.
--
-- ⚠ **The gate binds composed plans, not an admin's own hands** (maintainer decision,
-- 2026-08-01). An admin searching one source and queueing one clip stays direct, mirroring §7
-- where an admin may POST /v1/titles because the admin IS the gate. Requiring a proposal for a
-- single deliberate click would make the gate ceremony, and ceremony is what teaches people to
-- click through it.

CREATE TABLE IF NOT EXISTS filler_pulls (
  id           TEXT PRIMARY KEY,
  -- Operator-facing summary of what this pull is for.
  title        TEXT NOT NULL,
  -- Why Loomarr composed it — the gap it is trying to close. Rendered verbatim above the plan,
  -- because "approve this" without a reason is a button, not a decision.
  reason       TEXT NOT NULL DEFAULT '',
  -- Who or what proposed it. Free text: a user id today, a job name when a schedule composes one.
  proposed_by  TEXT NOT NULL DEFAULT '',
  -- pending | approved | dismissed. A decided pull is KEPT rather than deleted: the queue's
  -- History tab answers "what did we agree to download, and when", which a delete erases.
  status       TEXT NOT NULL DEFAULT 'pending',
  -- The operator's narrowing note, captured at approval ("no local dealers, no PSAs").
  note         TEXT NOT NULL DEFAULT '',
  -- The plan rows as JSON, including any the operator dropped before approving.
  --
  -- ⚠ Dropped rows are RETAINED with a flag rather than removed, and that is a property of the
  -- gate rather than bookkeeping: the record has to show what was proposed AND what was agreed
  -- to, or "we approved this" loses the half that matters. Same reason §7 keeps deny reasons.
  plan_json    TEXT NOT NULL DEFAULT '[]',
  created_at   INTEGER NOT NULL,
  -- Epoch seconds; 0 = still pending.
  decided_at   INTEGER NOT NULL DEFAULT 0,
  decided_by   TEXT NOT NULL DEFAULT ''
);

-- The queue reads pending pulls constantly and history rarely, so status leads.
CREATE INDEX IF NOT EXISTS idx_filler_pulls_status ON filler_pulls(status, created_at);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
