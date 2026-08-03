-- +goose Up
-- §10 tagging confidence + auto-filing (V38). Postgres mirror of the sqlite 00030; the full
-- reasoning lives there and in §10 ("Tagging confidence, and auto-filing").
--
-- The short version of both columns:
--
-- `confidence` (0-100) decides whether a clip is filed automatically or surfaced to a human.
-- ⚠ Grounding-CAPPED, never the model's self-assessment — 00024 exists because this tagger
-- invented an era on 2 of 10 real clips, so a self-reported score would be the same failure one
-- level up. 0 = never scored, which can never clear a threshold; a default of 100 would
-- auto-file the entire existing catalog on upgrade.
--
-- `auto_filed` records that NO HUMAN LOOKED at this clip before it became playable. It is what
-- makes an unattended decision reversible — the only thing that can answer "which of these did I
-- never see?" after the fact.
--
-- ⚠ `auto_filed` is BOOLEAN here where sqlite has INTEGER. V37's 00029 shipped a literal `1`
-- into a Postgres BOOLEAN column and took a hard 42804 at migrate time; the two files look
-- identical when read side by side, and only `test-pg` tells them apart. Same trap, one
-- migration later.
--
-- Forward-only (§16). Safe as plain ADD COLUMNs: `clips` is a synced CACHE (see 00013).
ALTER TABLE clips ADD COLUMN confidence INTEGER NOT NULL DEFAULT 0;
ALTER TABLE clips ADD COLUMN auto_filed BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
