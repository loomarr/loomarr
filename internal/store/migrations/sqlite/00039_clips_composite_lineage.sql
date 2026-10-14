-- +goose Up
-- V45: composites and split lineage (§10).
--
-- A COMPOSITE is a recorded break — many adverts in one file, like "KCPQ/Fox commercials,
-- 5/28/1996". Two columns give a clip its place in the composite → segment relationship:
--
--   is_composite   the clip IS a recorded break, NOT a single advert. NOT airable — pod assembly
--                  excludes it exactly like held/removed_at (a 16-min block aired as one
--                  "commercial" is the bug this removes). Its segments are the airable clips.
--                  BOOLEAN, matching held/auto_filed/ai_tagged/vision_tagged after 00033/00038 —
--                  the dialect split is per column and this table's bools live on the BOOLEAN side.
--
--   parent_hash    the hash of the composite this clip was split OUT of, or '' for a clip with no
--                  parent (a hand-dropped single advert, or a composite itself). This is the
--                  lineage V45 KEEPS that V34 threw away: V34 deleted the compilation on confirm,
--                  V45 keeps it and points each segment back — for provenance ("which break did this
--                  air in?"), re-splitting (detection improves), and broadcast-context inheritance.
--
-- ⚠ is_composite is written by the intake/detection path and by the split Confirm (which now marks
-- the parent composite instead of deleting it); parent_hash is written by Confirm when it cuts each
-- segment. Neither rides UpsertClip's DO UPDATE list — the folder scan does not know a file is a
-- composite or whose segment it is, so letting them ride the upsert would blank the lineage on the
-- next sync. Same single-writer discipline as language/removed_at/transcript (00036/00038).
--
-- Safe as plain ADD COLUMNs: `clips` is a synced cache (00013). Every existing row gets
-- is_composite=FALSE (correct — nothing was a composite before V45) and parent_hash='' (correct —
-- no existing clip is a split segment under the new model).
--
-- Forward-only (§16).
ALTER TABLE clips ADD COLUMN is_composite BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE clips ADD COLUMN parent_hash TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
