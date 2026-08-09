-- +goose Up
-- V52 phase 7 (§22): make a remote image findable by the URL it came from.
--
-- `Adopt` records a remote image under a hash of its SOURCE URL, because the content hash cannot
-- be known before the bytes exist. The fetch job then RE-KEYS that row onto the real content hash
-- and deletes the URL-keyed placeholder — which is required for `Cache-Control: immutable` to be
-- true, and is not in question here.
--
-- ⚠ **The consequence was that an adopted URL became unfindable the moment it succeeded.** After
-- the re-key, `GetImage(hashOfURL(src))` misses forever, so a second `Adopt` of the same URL minted
-- a fresh placeholder and the fetch job downloaded bytes already sitting on disk. Nothing noticed
-- while `Adopt` had no production caller. Phase 7 gave it one on an interactive surface: the icon
-- picker adopts a dozen posters every time an operator opens it, against an origin that caps a
-- client at 20 simultaneous connections (§22). The steady state was a re-download per open.
--
-- The fix is a lookup by `source_url`, which needs this index to not be a table scan on a table
-- that grows with every poster, still and icon the instance has ever seen.
--
-- ⚠ **Deliberately NOT unique.** Two rows legitimately share a source URL for the width of the
-- re-key: `fetchOne` writes the content-hash row BEFORE deleting the placeholder, so no ref ever
-- points at a hash with no row. A unique index would turn that ordering — which exists to keep
-- referential integrity — into a constraint violation. The lookup resolves the ambiguity by asking
-- only for FETCHED rows, of which there is at most one.
CREATE INDEX idx_images_source_url ON images (source_url);

-- +goose Down
DROP INDEX IF EXISTS idx_images_source_url;
