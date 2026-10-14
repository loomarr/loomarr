-- +goose Up
-- #955: uniquely bind new atomic approvals without erasing historical duplicate-run audit.
CREATE TABLE filler_pull_commits (
  pull_id TEXT PRIMARY KEY REFERENCES filler_pulls(id),
  acquisition_id TEXT NOT NULL UNIQUE REFERENCES filler_acquisition_runs(id)
);

-- Forward-only (§16).
-- +goose Down
SELECT 1;
