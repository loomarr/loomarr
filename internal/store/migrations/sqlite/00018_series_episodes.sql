-- +goose Up
-- Cached series episode lists (§5, §9 series expansion).
--
-- WHY. Expanding a series lineup entry into its episodes is one media-server call PER SHOW, and
-- it sat on the request path: `GET /v1/guide` re-expanded every series on every load. Measured
-- on the maintainer's install, a 4-show channel spent 232ms in enumeration while a 25-film
-- movie channel spent 1ms — roughly 90% of the guide's latency. (ComputeDesiredAt, the function
-- originally blamed, benchmarks at 45ms for TWO HUNDRED channels, so it was never the cost.)
--
-- This is a MATERIALIZED ANSWER, not a second source of truth. The library still owns what
-- episodes exist; a miss or a stale row falls back to the live call and writes the result back,
-- so a cold cache degrades to exactly today's behaviour rather than to an empty channel.
--
-- Keyed by the media server's SHOW item id, not by provision.Key: the same show can be reached
-- by a TMDB-keyed or a TVDB-keyed lineup entry (scan.go:84 documents that duality), and both
-- resolve to one library id. Keying on the library id means one row per show however it was
-- referenced, instead of two rows that could disagree.
--
-- `episodes_json` is the resolved list as the scheduler consumes it (item id, title, duration,
-- season/episode, multi-part span) — the same shape `desired_json` uses on `channels`, and for
-- the same reason: it is read whole, never queried into, so a normalized episodes table would
-- buy joins nobody performs and cost a migration nobody needs.
--
-- `fetched_at` drives staleness (`episodes.max_age`, default 24h) and is what
-- `series-episode-refresh` (§18.1) selects on. An empty list is a legitimate cached answer —
-- a show with no episodes present yet — so absence of a ROW is the only "unknown".
--
-- Forward-only (§16).
CREATE TABLE series_episodes (
    library_id    TEXT PRIMARY KEY,          -- media-server SHOW item id
    episodes_json TEXT   NOT NULL DEFAULT '[]',
    episode_count INTEGER NOT NULL DEFAULT 0, -- denormalized for cheap diagnostics/logging
    fetched_at    BIGINT NOT NULL DEFAULT 0   -- unix seconds; 0 = never (treated as stale)
);

-- The refresh job asks "which shows are stale?", so the index is on the ordering column.
CREATE INDEX idx_series_episodes_fetched_at ON series_episodes (fetched_at);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
