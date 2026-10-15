-- +goose Up
-- Where a clip's catalogue `name` came from (#1988): source | sidecar | grounded | curated |
-- filename, or 'unknown'. A clip with no source, sidecar or grounded title is named after its
-- file, and viewer surfaces must withhold that name. Its shape cannot tell it apart from copy:
-- `Widgetmax Ad 1991` reads like a title. Writers record the provenance from now on.
--
-- Backfill uses only what a row already records, because a migration cannot run the scan. Each
-- rule below matches a name to the exact text the app would have produced from the row's own
-- columns. Anything else stays 'unknown'. The rules mirror the sqlite file one for one.
--
-- * filename: the name is the stem of the row's path, or an opaque 40/64-hex hash, or a split
--   segment's fallback `Clip N from <recording>` (built from the parent's filename).
-- * grounded: the name is a filler.GroundedName rendering of the row's own facts: the brand
--   (with ` — <era>`), a quoted transcript or on-screen excerpt (“…”), `<Category> commercial`,
--   `<era> <kind>`, or the neutral `Untitled <kind>`.
--
-- NOT backfillable in SQL: a sidecar or source title, and a filename stem taken from the sidecar's
-- `originalName`. Intake files clips as `<hash>.<ext>`, so most filename-named clips have a path
-- that no longer carries the name. Those rows stay 'unknown', and viewer surfaces fall back to the
-- #1980 shape heuristic for them. The next folder scan reads each clip's sidecar and rewrites
-- name_source for every clip it finds, so 'unknown' remains only for rows whose file is gone.
ALTER TABLE clips ADD COLUMN name_source TEXT NOT NULL DEFAULT 'unknown';

-- Comparisons are exact (left/strpos, not LIKE): SQLite's LIKE ignores ASCII case and Postgres's
-- does not, and the two dialects must classify the same row the same way.
UPDATE clips SET name_source = 'filename'
WHERE name <> '' AND (
  left(path, length(name) + 1) = name || '.'
  OR strpos(path, '/' || name || '.') > 0
  OR name ~ '^([0-9a-f]{40}|[0-9a-f]{64})$'
  OR (parent_hash <> '' AND left(name, 5) = 'Clip ' AND strpos(name, ' from ') > 0)
);

UPDATE clips SET name_source = 'grounded'
WHERE name_source = 'unknown' AND name <> '' AND (
  (brand <> '' AND (name = brand OR (era > 0 AND name = brand || ' — ' || CAST(era AS TEXT))))
  OR (left(name, 1) = '“' AND (right(name, 1) = '”' OR strpos(name, '” — ') > 0))
  OR (category <> '' AND (
    name = upper(left(replace(category, '_', ' '), 1)) || substr(replace(category, '_', ' '), 2) || ' commercial'
    OR (era > 0 AND name = upper(left(replace(category, '_', ' '), 1)) || substr(replace(category, '_', ' '), 2)
      || ' commercial — ' || CAST(era AS TEXT))))
  OR (era > 0 AND name = CAST(era AS TEXT) || ' ' || replace(kind, '_', ' '))
  OR left(name, 9) = 'Untitled '
);

-- Forward-only (§16).

-- +goose Down
SELECT 1;
