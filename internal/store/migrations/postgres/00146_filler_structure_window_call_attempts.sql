-- +goose Up
-- Postgres mirror of sqlite 00146; the reasoning lives there. The primary key becomes
-- (request_sha256, attempt) and every existing row is attempt 1.

ALTER TABLE filler_structure_window_call_ledger ADD COLUMN attempt INTEGER NOT NULL DEFAULT 1;
ALTER TABLE filler_structure_window_call_ledger ADD CONSTRAINT filler_structure_window_call_ledger_attempt_check CHECK (attempt >= 1);
ALTER TABLE filler_structure_window_call_ledger DROP CONSTRAINT filler_structure_window_call_ledger_pkey;
ALTER TABLE filler_structure_window_call_ledger ADD PRIMARY KEY (request_sha256, attempt);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
