-- +goose Up
-- The required Rust image worker makes animation semantics and encoder provenance durable. A
-- recipe is part of derivative identity: changing encoder settings must create a new cache entry,
-- never make old bytes masquerade under the new recipe.

ALTER TABLE images ADD COLUMN frame_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE images ADD COLUMN duration_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE images ADD COLUMN loop_count INTEGER;

CREATE TABLE image_derivatives_v57 (
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    recipe     TEXT NOT NULL,
    format     TEXT NOT NULL,
    width      INTEGER NOT NULL,
    bytes      INTEGER NOT NULL DEFAULT 0,
    output_hash TEXT NOT NULL DEFAULT '',
    path       TEXT NOT NULL,
    animated   INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (image_hash, recipe, format, width)
);

INSERT INTO image_derivatives_v57
    (image_hash, recipe, format, width, bytes, output_hash, path, animated, created_at)
SELECT image_hash, 'legacy-go-v1', format, width, bytes, '', path, 0, created_at
FROM image_derivatives;

DROP TABLE image_derivatives;
ALTER TABLE image_derivatives_v57 RENAME TO image_derivatives;
CREATE INDEX idx_image_derivatives_format ON image_derivatives (format, created_at);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
