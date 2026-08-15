-- +goose Up
-- Postgres mirror of sqlite 00057. Animation semantics describe the original; recipe and output
-- hash describe regenerable worker output and make an encoder change an explicit cache version.

ALTER TABLE images ADD COLUMN frame_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE images ADD COLUMN duration_ms BIGINT NOT NULL DEFAULT 0;
ALTER TABLE images ADD COLUMN loop_count INTEGER;

ALTER TABLE image_derivatives ADD COLUMN recipe TEXT NOT NULL DEFAULT 'legacy-go-v1';
ALTER TABLE image_derivatives ADD COLUMN output_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE image_derivatives ADD COLUMN animated BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE image_derivatives DROP CONSTRAINT image_derivatives_pkey;
ALTER TABLE image_derivatives ADD PRIMARY KEY (image_hash, recipe, format, width);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
