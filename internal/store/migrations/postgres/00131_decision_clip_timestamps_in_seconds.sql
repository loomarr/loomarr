-- +goose Up
-- An applied admission decision stamped the clip and its pipeline row with the decision ledger's
-- Unix-NANOSECOND codec, while both tables hold Unix seconds: those rows read back millions of years
-- in the future (#1747). The writes now use seconds; this repairs the rows already written.
--
-- A seconds value stays below 10^11 until the year 5138, and a nanosecond value is above it for any
-- date after 1973, so the threshold separates the two exactly. A value repaired once falls below it,
-- so running this again changes nothing.
UPDATE clips SET updated_at = updated_at / 1000000000 WHERE updated_at > 100000000000;
UPDATE clips SET removed_at = removed_at / 1000000000 WHERE removed_at > 100000000000;
UPDATE filler_clip_pipeline SET updated_at = updated_at / 1000000000 WHERE updated_at > 100000000000;

-- +goose Down
SELECT 1;
