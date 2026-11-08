-- +goose Up
-- V52 (§22): the image service — one pipeline for every image Loomarr shows.
--
-- Replaces four unrelated arrangements: uploaded channel icons as database blobs, clip stills and
-- hover loops on disk under FILLER_DIR, and TMDB posters hot-linked straight from the operator's
-- browser. One record per image, bytes on disk under a content hash.
--
-- ⚠ **No image bytes live here.** `channel_icons` put them in the database specifically so they
-- would ride the §16 backup, which worked but makes the database the wrong shape for a general
-- image service and would not survive ingesting remote artwork. §22 records the consequence
-- plainly: the application backup is a DATABASE backup, so an operator upload is the one thing
-- that does not come back if /data/images is lost — which is why the GC job counts missing
-- unrecoverable rows and raises a warning rather than letting them render as broken images.

CREATE TABLE images (
    -- sha256 of the ORIGINAL bytes, hex. Identity, filename, ETag and cache key at once, which is
    -- what makes `Cache-Control: immutable` an honest header rather than a hope.
    hash         TEXT PRIMARY KEY,

    -- upload | remote | extracted | generated. ⚠ This is the durability column: it decides what
    -- can be got back after a restore. Only `remote` and `extracted` are recoverable.
    origin       TEXT NOT NULL,

    -- Upstream URL for `remote`, empty otherwise. Load-bearing rather than informational: it is
    -- the difference between an image being cached and an image being recoverable.
    source_url   TEXT NOT NULL DEFAULT '',

    -- public | member. ⚠ Visibility is a property of the IMAGE, not of the route — a channel icon
    -- must be fetchable by Tunarr with no credentials while a clip still must not be, and one
    -- serve path with a per-row check is the only shape that serves both.
    visibility   TEXT NOT NULL DEFAULT 'member',

    -- poster | backdrop | icon | thumb. Decides the width ladder and the fallback aspect.
    role         TEXT NOT NULL DEFAULT 'icon',

    -- Describes the ORIGINAL, never a derivative. width/height are served to the frontend so an
    -- <img> can carry real dimensions and contribute zero layout shift.
    mime         TEXT NOT NULL,
    width        INTEGER NOT NULL DEFAULT 0,
    height       INTEGER NOT NULL DEFAULT 0,
    bytes        INTEGER NOT NULL DEFAULT 0,

    -- Motion in the original (the clip hover loop). Such images have one rendition and skip the
    -- ladder: resizing an animation per breakpoint costs far more than it saves.
    animated     INTEGER NOT NULL DEFAULT 0,

    -- ThumbHash, base64 (~25 bytes raw). ThumbHash rather than BlurHash because it carries ALPHA;
    -- channel logos are routinely transparent PNGs, which BlurHash renders as black.
    placeholder  TEXT NOT NULL DEFAULT '',
    dominant_hex TEXT NOT NULL DEFAULT '',

    -- Free-form JSON: attribution, licence, upstream ids, generation parameters. Deliberately
    -- opaque at this layer so the service does not grow a schema per producer.
    meta         TEXT NOT NULL DEFAULT '{}',

    -- ⚠ When the bytes were obtained upstream — a COMPLIANCE field, not a cache heuristic. TMDB's
    -- API terms forbid caching their content beyond six months, so remote images carry an expiry
    -- the GC enforces. It is here from the first migration because retrofitting expiry into a
    -- content-addressed store, after a year of artwork has accumulated, is not something adding a
    -- column later can fix.
    origin_fetched_at INTEGER NOT NULL DEFAULT 0,

    created_at   INTEGER NOT NULL DEFAULT 0,
    updated_at   INTEGER NOT NULL DEFAULT 0,
    -- Drives derivative eviction under the cache budget.
    last_used_at INTEGER NOT NULL DEFAULT 0
);

-- The fetch job's work queue: remote rows whose bytes have not landed yet. Partial-ish by
-- ordering on origin first, which is the selective column here.
CREATE INDEX idx_images_origin_fetched ON images (origin, origin_fetched_at);

-- What an image decorates. ⚠ A separate table rather than a column on each domain so the GC can
-- find orphans WITHOUT every domain knowing about images — the same reasoning that keeps clip
-- identity out of the channel schema. A domain adds a ref; it never gains an images import.
--
-- An image may be referenced more than once (the same poster as a channel's icon and a title's
-- art), so the key is the whole tuple.
CREATE TABLE image_refs (
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    owner_kind TEXT NOT NULL,
    owner_id   TEXT NOT NULL,
    role       TEXT NOT NULL DEFAULT 'icon',
    PRIMARY KEY (image_hash, owner_kind, owner_id, role)
);

-- The reverse lookup: "what image does this channel use?" is the question every surface asks.
CREATE INDEX idx_image_refs_owner ON image_refs (owner_kind, owner_id);

-- One encoded rendition on disk. Regenerable by construction — nothing here is worth backing up,
-- and deleting a row costs a re-encode rather than data.
CREATE TABLE image_derivatives (
    image_hash TEXT NOT NULL REFERENCES images(hash) ON DELETE CASCADE,
    format     TEXT NOT NULL,
    width      INTEGER NOT NULL,
    bytes      INTEGER NOT NULL DEFAULT 0,
    path       TEXT NOT NULL,
    created_at INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (image_hash, format, width)
);

-- The AVIF job's work queue is "images with a WebP rendition but no AVIF one", which is a
-- format-scoped scan; and the GC evicts by age across all formats.
CREATE INDEX idx_image_derivatives_format ON image_derivatives (format, created_at);

-- Forward-only (§16).

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
