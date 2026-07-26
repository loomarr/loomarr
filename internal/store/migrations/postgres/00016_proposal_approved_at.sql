-- +goose Up
-- The approval TIMESTAMP (§7, V27's audit rows). `approved_by` records who approved and
-- `mod_summary`/`note` record what changed — but nothing recorded WHEN, so an approvals history
-- could list decisions in no verifiable order.
--
-- ⚠ A DEDICATED COLUMN, deliberately, rather than reusing `updated_at`. That looks equivalent —
-- the last write on the approve path IS the approval — and it is not: THREE callers update a
-- proposal row (suggest.Approve, the deny handler, and recurate's filtered rewrite), so a
-- re-curation touching an already-approved proposal would silently rewrite its approval time.
-- An audit field that another subsystem can move is not an audit field.
--
-- 0 = never approved, which `fromEpoch` already maps to the zero time — so every pre-existing
-- row, including ones approved before this column existed, reads as "no recorded approval time"
-- rather than 1970. That is honest: the timestamp genuinely was not recorded for them, and
-- back-filling it from `updated_at` would manufacture a record that was never taken.
--
-- BIGINT epoch seconds, matching created_at/updated_at on this table.
--
-- Forward-only (§16).
ALTER TABLE proposals ADD COLUMN approved_at BIGINT NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
