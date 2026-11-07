-- +goose Up
-- Scan roots (#2151 decision 6): the local folders Loomarr walks for media.
--
-- `library_roots` is one row per local folder. `in_use` is the maintainer's confirmation; a
-- discovered folder starts unconfirmed. `readable` and `checked_at` (unix ms, 0 = never) record
-- the last readability check. `by_hand` marks a folder the maintainer typed rather than one a
-- media server or arr reported.
--
-- `library_root_paths` is where each authority (`emby`, `jellyfin`, `sonarr`, `radarr`) calls a
-- root: the generalized path map. `by_hand` marks a row the maintainer edited, which discovery
-- never rewrites.
CREATE TABLE library_roots (
    path TEXT PRIMARY KEY,
    in_use INTEGER NOT NULL DEFAULT 0,
    readable INTEGER NOT NULL DEFAULT 0,
    checked_at INTEGER NOT NULL DEFAULT 0,
    by_hand INTEGER NOT NULL DEFAULT 0,
    added_at INTEGER NOT NULL
);

CREATE TABLE library_root_paths (
    authority TEXT NOT NULL,
    authority_path TEXT NOT NULL,
    root_path TEXT NOT NULL DEFAULT '',
    by_hand INTEGER NOT NULL DEFAULT 0,
    seen_at INTEGER NOT NULL,
    PRIMARY KEY (authority, authority_path)
);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
