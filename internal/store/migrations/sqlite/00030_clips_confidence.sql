-- +goose Up
-- §10 tagging confidence + auto-filing (V38). Two columns, one job each.
--
-- `confidence` (0-100) is what decides whether a clip is filed automatically or surfaced to a
-- human in Incoming. ⚠ **It is grounding-CAPPED, never the model's self-assessment.** This
-- tagger has a measured history of confident fabrication — 00024 exists because it invented an
-- era on 2 of 10 real clips, inferred from tone — so a self-reported score would be the same
-- failure one level up: the model that fabricated the era also grading how sure it is. The
-- grounding facts (era literally present? audience/category matched an enum? was there any text
-- to check?) set a CEILING the model may lower but never lift.
--
-- 0 = never scored. ⚠ That is deliberately the same as "scored zero" and it is safe in one
-- direction only: an unscored clip can never be auto-filed, which is the failure mode we want.
-- A default of 100 would auto-file the entire existing catalog on upgrade.
--
-- `auto_filed` records that NO HUMAN LOOKED AT THIS CLIP before it entered the catalog and
-- became playable on a channel. It is not telemetry: it is what makes an unattended decision
-- reversible. An operator who did not expect auto-filing must be able to find exactly what was
-- filed without them and undo it, and a boolean on the row is the only thing that can answer
-- "which of these did I never see?" after the fact.
--
-- ⚠ Both default to the SAFE value rather than the common one. Every clip already in the catalog
-- was put there by a human or by a pre-V38 scan, so `auto_filed = 0` is true of all of them, and
-- `confidence = 0` keeps them out of any threshold comparison.
--
-- Forward-only (§16). Safe as plain ADD COLUMNs: `clips` is a synced CACHE (see 00013), so the
-- worst case is one sync cycle before the tagger scores a clip.
ALTER TABLE clips ADD COLUMN confidence INTEGER NOT NULL DEFAULT 0;
ALTER TABLE clips ADD COLUMN auto_filed INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
