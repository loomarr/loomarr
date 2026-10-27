-- +goose Up
-- A filler ad's identity is its creative (#2033), matched by fingerprint, never by file or title.
--
-- The fingerprint cache now holds a whole creative fingerprint (video and audio), so its payload
-- column is renamed. Every cached row was a dHash under the retired 1/3 fps key; the cache is
-- rebuildable work, so it is cleared rather than reinterpreted.
ALTER TABLE filler_clip_fingerprints RENAME COLUMN frames_json TO fingerprint_json;
DELETE FROM filler_clip_fingerprints;

-- Linked variants: separate ads cut from the same footage, such as a 15 s cut-down of a 30 s spot.
-- A break never carries two of them, and they rest together. One row per pair, smaller hash first.
CREATE TABLE filler_clip_variants (
  a_hash TEXT NOT NULL,
  b_hash TEXT NOT NULL,
  PRIMARY KEY (a_hash, b_hash),
  CHECK (a_hash < b_hash)
);

CREATE INDEX idx_filler_clip_variants_b ON filler_clip_variants(b_hash);

-- Inferior copies: a copy of an ad that lost to a better one and was deleted. source_hash is the
-- sparse identity of the bytes as they were admitted, so the same file offered again by a folder
-- or a re-download is skipped at intake. No foreign keys: the loser's clip row is gone, and the
-- survivor may later be removed without reviving its copies.
CREATE TABLE filler_inferior_copies (
  clip_hash     TEXT NOT NULL PRIMARY KEY,
  source_hash   TEXT NOT NULL,
  survivor_hash TEXT NOT NULL,
  decided_at    BIGINT NOT NULL
);

CREATE INDEX idx_filler_inferior_copies_source ON filler_inferior_copies(source_hash);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
