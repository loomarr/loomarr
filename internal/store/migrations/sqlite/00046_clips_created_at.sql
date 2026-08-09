-- +goose Up
-- V51d (§10): when a clip ENTERED the catalog, so "recently added" can be a sort order.
--
-- ⚠ **`updated_at` cannot stand in, and that is the whole reason this column exists.** A folder
-- re-sync upserts every file it finds and bumps `updated_at` on each one, so an "added" sort
-- backed by it would reshuffle the entire catalog after a routine scan — the newest clips would
-- be whichever ones the last pass happened to touch. `created_at` is written ONCE, at insert.
--
-- ⚠ Existing rows backfill from `updated_at` as a STATED ESTIMATE. For a clip that predates this
-- column there is no better answer on disk, and the alternative — leaving them at 0 — would sort
-- every pre-V51d clip into one undifferentiated block at the far end of the order. An estimate
-- that is right for a never-re-synced clip and approximately right otherwise beats a sentinel.
--
-- ⚠ INTEGER here, BIGINT on Postgres — the per-column dialect split every epoch column in this
-- schema follows (00033 made last_played_at / removed_at / updated_at BIGINT there). sqlite's
-- INTEGER is already 64-bit, so the two agree on values while differing in spelling.
--
-- ⚠ INSERTed by `UpsertClip` but deliberately OMITTED from its `DO UPDATE` list — the fourth
-- column with that rule, alongside held/removed_at/confidence and the play counters. The scan
-- supplies a fresh timestamp on every pass; letting it ride the update list would mark every
-- clip "just added" after each sync, which is exactly the failure the column exists to avoid.

ALTER TABLE clips ADD COLUMN created_at INTEGER NOT NULL DEFAULT 0;

UPDATE clips SET created_at = updated_at WHERE created_at = 0;

-- +goose Down
-- Forward-only (§16): the Down is recorded for goose's benefit, never run in anger.
ALTER TABLE clips DROP COLUMN created_at;
