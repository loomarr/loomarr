-- +goose Up
-- V52 phase 6 (§22): a clip's still and hover loop become IMAGE-SERVICE images.
--
-- Before this, artwork lived only as files under FILLER_DIR with `thumbnail`/`preview` holding
-- paths relative to that cache, served by /v1/filler/thumb/{hash} and /v1/filler/hover/{hash}.
-- That predates the image service and carries none of what it provides: no srcset, no modern
-- format on the still (the only WebP in the product was the ANIMATED hover), no content
-- addressing, and therefore no honest immutable caching.
--
-- ⚠ **The pointer lives HERE, on the clip, and not in `image_refs`.** `image_refs` is the reverse
-- index the GC reads to answer "is anything still using this image"; it is not the forward lookup.
-- V52 phase 5 set the precedent — a channel's artwork pointer is `channels.logo`, and the Ref is
-- recorded ALONGSIDE it, not instead of it. Treating refs as the forward pointer would mean a
-- join (or a new store method) on every read of a surface that renders a grid of clips.
--
-- ⚠ **`thumbnail`/`preview` are deliberately NOT dropped here.** They still point at real files
-- that the existing routes still serve, and they are what the phase-8 backfill reads to find
-- artwork that predates this migration. Dropping them now would strand every clip on an install
-- that has not re-rendered, which is all of them at the moment this lands. Forward-only (§16):
-- the columns retire in phase 8, once nothing reads them.
--
-- Empty string, not NULL: it matches how `thumbnail`/`preview` already encode "not generated
-- yet", and V53b made the point that a nullable column invites a second empty value.
ALTER TABLE clips ADD COLUMN thumb_image_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE clips ADD COLUMN hover_image_hash TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE clips DROP COLUMN thumb_image_hash;
ALTER TABLE clips DROP COLUMN hover_image_hash;
