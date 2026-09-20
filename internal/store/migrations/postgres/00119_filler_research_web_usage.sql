-- +goose Up
CREATE TABLE filler_research_web_usage (
    month TEXT PRIMARY KEY CHECK (length(month) = 7),
    request_count INTEGER NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    last_provider TEXT NOT NULL DEFAULT '',
    last_success_at BIGINT NOT NULL DEFAULT 0,
    last_failure_at BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE filler_research_web_attempts (
    clip_hash TEXT NOT NULL REFERENCES clips(hash) ON DELETE CASCADE,
    input_revision BIGINT NOT NULL CHECK (input_revision > 0),
    adapter_version TEXT NOT NULL CHECK (adapter_version <> ''),
    provider TEXT NOT NULL CHECK (provider IN ('brave', 'searxng')),
    reserved_at BIGINT NOT NULL CHECK (reserved_at > 0),
    PRIMARY KEY (clip_hash, input_revision, adapter_version)
);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
