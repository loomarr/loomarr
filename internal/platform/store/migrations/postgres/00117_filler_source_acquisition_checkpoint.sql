-- +goose Up
-- PostgreSQL mirror of SQLite 00117; the full rationale lives there.

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
