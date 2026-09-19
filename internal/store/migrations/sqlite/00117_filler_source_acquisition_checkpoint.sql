-- +goose Up
-- #1239: YouTube source acquisition resumes by stable item identity rather than restarting at
-- the newest per-source prefix. Outcome counts explain a completed/continuing bounded sweep
-- without retaining extractor logs or exposing cursor machinery as settings.

ALTER TABLE filler_sources ADD COLUMN scan_cursor TEXT NOT NULL DEFAULT '';
ALTER TABLE filler_sources ADD COLUMN scan_watermark TEXT NOT NULL DEFAULT '';
ALTER TABLE filler_sources ADD COLUMN scan_pending_watermark TEXT NOT NULL DEFAULT '';

CREATE TABLE filler_source_check_outcomes (
    source_id TEXT NOT NULL REFERENCES filler_sources(id) ON DELETE CASCADE,
    disposition TEXT NOT NULL CHECK (disposition IN (
        'queued', 'already_known', 'too_short', 'too_long', 'live', 'upcoming',
        'private', 'unavailable', 'metadata_incomplete'
    )),
    item_count INTEGER NOT NULL CHECK (item_count >= 0),
    PRIMARY KEY (source_id, disposition)
);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
