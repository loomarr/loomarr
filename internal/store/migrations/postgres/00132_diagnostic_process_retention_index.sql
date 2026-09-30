-- +goose Up
-- Retention pages must walk terminal runs in ending order instead of scanning and sorting every
-- run on every page. The predicate matches both expiry and retained-byte candidate queries.
CREATE INDEX idx_diagnostic_process_runs_retention
    ON diagnostic_process_runs (ended_at, id)
    WHERE status <> 'running' AND ended_at > 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
