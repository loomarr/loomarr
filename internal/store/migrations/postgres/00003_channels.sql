-- +goose Up
-- Postgres mirror of the channels schema (§9). See the sqlite variant for
-- commentary; kept ANSI-compatible. Epoch BIGINT timestamps (§3). ClaimDueChannels
-- uses FOR UPDATE SKIP LOCKED against reconcile_deadline for multi-replica
-- single-leader-per-channel reconcile (§18).

CREATE TABLE channels (
    id                 TEXT PRIMARY KEY,
    intent_ref         TEXT NOT NULL DEFAULT '',
    name               TEXT NOT NULL,
    number             INTEGER NOT NULL,
    grp                TEXT NOT NULL DEFAULT '',
    logo               TEXT NOT NULL DEFAULT '',
    strategy           TEXT NOT NULL,
    filler_ref         TEXT NOT NULL DEFAULT '',
    tunarr_id          TEXT NOT NULL DEFAULT '',
    status             TEXT NOT NULL,
    shuffle_seed       BIGINT NOT NULL DEFAULT 0,
    lineup_json        TEXT NOT NULL DEFAULT '[]',
    desired_json       TEXT NOT NULL DEFAULT '[]',
    reconcile_deadline BIGINT NOT NULL DEFAULT 0,
    updated_at         BIGINT NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX idx_channels_number ON channels (number);
CREATE INDEX idx_channels_status_deadline ON channels (status, reconcile_deadline);

-- +goose Down
DROP TABLE channels;
