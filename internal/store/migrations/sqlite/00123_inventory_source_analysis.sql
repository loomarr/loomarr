-- +goose Up
-- Loomarr's own per-revision measurement of a source beyond its stream facts (beta.8 G7): the
-- keyframe index (varint-packed), EBU R128 loudness and natural break candidates. One row per
-- source; it is replaced when the source is re-measured and deleted with the source or when the
-- source revision changes.
CREATE TABLE inventory_source_analysis (
    source_id TEXT PRIMARY KEY REFERENCES inventory_sources(id) ON DELETE CASCADE,
    source_revision TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    keyframes BLOB NOT NULL,
    keyframe_count INTEGER NOT NULL,
    integrated_lufs DOUBLE PRECISION,
    true_peak_dbtp DOUBLE PRECISION,
    breaks_json TEXT NOT NULL,
    analyzed_at BIGINT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS inventory_source_analysis;
