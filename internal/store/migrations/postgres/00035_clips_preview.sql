-- +goose Up
-- V39: a clip's animated hover preview.
--
-- `preview` is a path RELATIVE to the preview cache directory, exactly like `thumbnail` (00017)
-- and for the same reason: the mount differs between host and container, so an absolute path
-- would invalidate every row the first time someone moves it. Empty = not generated yet, which
-- renders as the still thumbnail rather than as a broken image.
--
-- ⚠ A SEPARATE column rather than deriving the path from `thumbnail` in code. They are almost
-- always the same stem with a different extension, which is exactly what makes derivation
-- tempting and wrong: a derived path asserts the file EXISTS, and the two passes fail
-- independently. A clip whose still generated and whose preview did not would then render a
-- broken <img> on hover — a failure the operator sees and cannot explain, in place of a preview
-- that simply does not appear.
--
-- The bytes are NOT stored here, for the reason 00017 gives at length: these are regenerable
-- ffmpeg output, thousands of rows deep, and `clips` rides the §16 backup.
--
-- Safe as a plain ADD COLUMN: `clips` is a synced cache (00013), so the worst case is one sync
-- cycle with blank previews.
--
-- Forward-only (§16).
ALTER TABLE clips ADD COLUMN preview TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
