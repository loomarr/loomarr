-- +goose Up
-- V52 (§22): the image service. Postgres mirror of the sqlite 00044; the full reasoning lives
-- there and is not duplicated here beyond what the schema itself needs to justify.
--
-- ⚠ In short: one record per image with the bytes on disk under a content hash, replacing four
-- unrelated arrangements (icons as DB blobs, clip artwork under FILLER_DIR, TMDB hot-linked). No
-- image bytes live in the database — see the sqlite file for what that costs on restore and why
-- it is accepted.
--
-- Dialect differences from the sqlite file, and only these: BIGINT for epoch columns, BOOLEAN for
-- `animated` (sqlite carries it as INTEGER), and JSONB for `meta` so an operator can query
-- attribution without parsing text. The COLUMN SET and every constraint are otherwise identical —
-- the conformance suite runs one set of assertions against both, so a divergence here surfaces as
-- a failing shared test rather than as a Postgres-only surprise in production.

CREATE TABLE images (
    hash         TEXT PRIMARY KEY,
    origin       TEXT NOT NULL,
    source_url   TEXT NOT NULL DEFAULT '',
    visibility   TEXT NOT NULL DEFAULT 'member',
    role         TEXT NOT NULL DEFAULT 'icon',
    mime         TEXT NOT NULL,
    width        INTEGER NOT NULL DEFAULT 0,
    height       INTEGER NOT NULL DEFAULT 0,
    bytes        BIGINT NOT NULL DEFAULT 0,
    animated     BOOLEAN NOT NULL DEFAULT FALSE,
    placeholder  TEXT NOT NULL DEFAULT '',
    dominant_hex TEXT NOT NULL DEFAULT '',
    meta         JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- ⚠ Compliance, not caching: TMDB's terms cap retention of their content at six months.
    origin_fetched_at BIGINT NOT NULL DEFAULT 0,
    created_at   BIGINT NOT NULL DEFAULT 0,
    updated_at   BIGINT NOT NULL DEFAULT 0,
    last_used_at BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_images_origin_fetched ON images (origin, origin_fetched_at);

CREATE TABLE image_refs (
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    owner_kind TEXT NOT NULL,
    owner_id   TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'icon',
    PRIMARY KEY (image_hash, owner_kind, owner_id, role)
);

CREATE INDEX idx_image_refs_owner ON image_refs (owner_kind, owner_id);

CREATE TABLE image_derivatives (
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    format     TEXT NOT NULL,
    width      INTEGER NOT NULL,
    bytes      BIGINT NOT NULL DEFAULT 0,
    path       TEXT NOT NULL,
    created_at BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (image_hash, format, width)
);

CREATE INDEX idx_image_derivatives_format ON image_derivatives (format, created_at);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
