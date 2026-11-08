-- +goose Up
-- Edit-before-approve (§7, decision D-K): an admin can drop titles, add others, and leave a
-- note before approving. These two columns are the AUDIT half of that.
--
-- `mod_summary` is what actually changed, generated server-side ("dropped 2, added 1") rather
-- than typed by the approver — a summary the approver writes is a claim, one the code writes is
-- a record. `note` is the human message to the requester ("swapped Con Air for Face/Off — we
-- already have it"), which is why a member's request coming back altered is explicable instead
-- of mysterious.
--
-- REAL COLUMNS, not fields inside proposal_json. They sit with `approved_by` and `deny_reason`,
-- which are the other audit fields, and the trail has to be queryable: "what did admins change
-- at approval time" is a question about the gate, and folding it into an opaque blob would make
-- it unaskable.
--
-- Empty default, so every existing row reads as "approved unmodified" — which is what those
-- approvals were.
--
-- Forward-only (§16).
ALTER TABLE proposals ADD COLUMN mod_summary TEXT NOT NULL DEFAULT '';
ALTER TABLE proposals ADD COLUMN note TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
