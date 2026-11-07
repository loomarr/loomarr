-- +goose Up
-- A window call that produced no answer (the budget held it, or the request got no response) is
-- called again on the next pass. A lost request keeps its reservation in the shared budget, so the
-- retry is a new ATTEMPT of the same request digest: the journal key is (request, attempt) and the
-- latest attempt is the one that governs replay. A budget hold has reserved nothing, so its retry
-- replaces the held row instead of adding one.
--
-- SQLite cannot change a primary key in place, so the journal is rebuilt with every row kept as
-- attempt 1.

CREATE TABLE filler_structure_window_call_ledger_attempts (
  request_sha256      TEXT NOT NULL,
  attempt             INTEGER NOT NULL DEFAULT 1,
  evaluation_id       TEXT NOT NULL UNIQUE REFERENCES filler_inference_evaluations(id),
  source_sha256       TEXT NOT NULL,
  media_set_sha256    TEXT NOT NULL,
  window_ordinal      INTEGER NOT NULL,
  assessor_id         TEXT NOT NULL,
  state               TEXT NOT NULL,
  reservation_json    TEXT NOT NULL,
  record_sha256       TEXT NOT NULL DEFAULT '',
  record_json         TEXT NOT NULL DEFAULT '',
  requested_at        INTEGER NOT NULL,
  assessed_at         INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (request_sha256, attempt),
  CHECK (attempt >= 1),
  CHECK (window_ordinal >= 0),
  CHECK (state IN ('open', 'held_budget', 'settled')),
  CHECK (
    (state = 'settled' AND record_sha256 <> '' AND record_json <> '' AND assessed_at > 0) OR
    (state <> 'settled' AND record_sha256 = '' AND record_json = '' AND assessed_at = 0)
  )
);

INSERT INTO filler_structure_window_call_ledger_attempts (
  request_sha256, attempt, evaluation_id, source_sha256, media_set_sha256, window_ordinal,
  assessor_id, state, reservation_json, record_sha256, record_json, requested_at, assessed_at
) SELECT
  request_sha256, 1, evaluation_id, source_sha256, media_set_sha256, window_ordinal,
  assessor_id, state, reservation_json, record_sha256, record_json, requested_at, assessed_at
FROM filler_structure_window_call_ledger;

DROP TABLE filler_structure_window_call_ledger;
ALTER TABLE filler_structure_window_call_ledger_attempts RENAME TO filler_structure_window_call_ledger;

CREATE INDEX IF NOT EXISTS idx_filler_structure_window_call_source
  ON filler_structure_window_call_ledger(source_sha256, media_set_sha256, requested_at DESC, request_sha256);
CREATE INDEX IF NOT EXISTS idx_filler_structure_window_call_open
  ON filler_structure_window_call_ledger(state, requested_at ASC, request_sha256);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
