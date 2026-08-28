-- +goose Up
-- §10 V65: split confirmation publishes reviewed media across a filesystem and a durable store.
-- One expiring fencing token serializes that operation across processes; every partial or final
-- proposal mutation checks the token so a recovered owner fences a stalled predecessor.

ALTER TABLE filler_split_proposals
  ADD COLUMN claim_token TEXT NOT NULL DEFAULT '';

ALTER TABLE filler_split_proposals
  ADD COLUMN claim_expires_at BIGINT NOT NULL DEFAULT 0
  CHECK ((claim_token = '' AND claim_expires_at = 0) OR
         (claim_token <> '' AND claim_expires_at > 0));

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
