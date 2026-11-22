-- +goose Up
-- A download failure's item_key separated its parts with NUL, which Postgres text rejects, so the
-- table could not be written there at all. The store now writes the unit separator (0x1F) instead
-- and turns it back into the in-memory key on read; this rewrites the rows already stored so they
-- still match the items they record.
--
-- SQLite's replace() stops at a NUL, so the key is rebuilt from the parts each row already holds,
-- as DownloadItem.Key() builds it: the provider lower-cased, then the source and the remote id,
-- or "url:" and the URL for an item with none, each trimmed.
UPDATE filler_download_failures
SET item_key = lower(trim(provider)) || char(31) || trim(source_id) || char(31) ||
    trim(CASE WHEN remote_id = '' THEN 'url:' || url ELSE remote_id END)
WHERE instr(item_key, char(0)) > 0;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
