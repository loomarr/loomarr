-- +goose Up
-- Holidays are data (#2030): one row per holiday, each country's dates and the words that show a
-- title is about it, edited in the app rather than shipped as Go. Like taxa, rows are SEEDED AT
-- BOOT from holidayvocab.Seed() (empty-guard plus numbered revisions), so this migration creates
-- the empty tables.
--
-- A holiday owns its seasonal taxon: the same slug, with a foreign key, so a clip's seasonal tag
-- always names a holiday. A seasonal taxon an operator made before this has no shipped holiday; boot
-- gives it an undated one (no dates, no words), whose clips air year-round.
--
-- words, kept, air_window and members are JSON validated by holidayvocab.New on every load: kept
-- is each country's day and window rules, members a group holiday's holidays. position is the
-- display order, which also breaks ties when two holidays are on at once.
CREATE TABLE holidays (
  id                       TEXT PRIMARY KEY REFERENCES taxa(slug),
  label                    TEXT NOT NULL,
  position                 INTEGER NOT NULL,
  words                    TEXT NOT NULL DEFAULT '{}',
  kept                     TEXT NOT NULL DEFAULT '[]',
  air_window               TEXT NOT NULL DEFAULT '',
  members                  TEXT NOT NULL DEFAULT '[]',
  name_needs_corroboration INTEGER NOT NULL DEFAULT 0,
  updated_at               INTEGER NOT NULL DEFAULT 0
);

-- Each shipped revision of the holiday seed applies once; an operator's later edit or retirement
-- is never undone by boot.
CREATE TABLE holiday_seed_revisions (
  version    INTEGER PRIMARY KEY,
  applied_at INTEGER NOT NULL
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
