-- +goose Up
-- Scheduler domain (§9): Loomarr-managed channels + their approved lineup and
-- last-computed desired lineup. Forward-only (§16). Epoch BIGINT timestamps (§3
-- note) so the schema stays dialect-neutral. The lineup shapes are JSON blobs —
-- the store round-trips the schedule.Channel/LineupEntry/Slot structs and never
-- queries inside them (identity + reconcile scheduling are the only indexed
-- concerns), mirroring titles.title_json.

CREATE TABLE channels (
    id                 TEXT PRIMARY KEY,
    intent_ref         TEXT NOT NULL DEFAULT '',
    name               TEXT NOT NULL,
    number             INTEGER NOT NULL,
    grp                TEXT NOT NULL DEFAULT '',   -- Tunarr groupTitle ("group" is reserved)
    logo               TEXT NOT NULL DEFAULT '',
    strategy           TEXT NOT NULL,
    filler_ref         TEXT NOT NULL DEFAULT '',
    tunarr_id          TEXT NOT NULL DEFAULT '',   -- server-assigned; '' until first reconcile
    status             TEXT NOT NULL,
    shuffle_seed       BIGINT NOT NULL DEFAULT 0,
    lineup_json        TEXT NOT NULL DEFAULT '[]', -- approved []LineupEntry
    desired_json       TEXT NOT NULL DEFAULT '[]', -- last-computed []Slot (desired lineup)
    -- reconcile_deadline drives ClaimDueChannels for the periodic sweep (§9/§18):
    -- a channel is "due" when its deadline is at/before now; claiming leases it
    -- (deadline := now+lease) so two replicas don't reconcile one channel at once.
    reconcile_deadline BIGINT NOT NULL DEFAULT 0,
    updated_at         BIGINT NOT NULL DEFAULT 0
);

-- Channel number is the guide position; it must be unique among managed channels.
CREATE UNIQUE INDEX idx_channels_number ON channels (number);
-- ClaimDueChannels filters by status + reconcile_deadline; index the hot path.
CREATE INDEX idx_channels_status_deadline ON channels (status, reconcile_deadline);

-- +goose Down
DROP TABLE channels;
