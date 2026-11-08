-- +goose Up
-- ChannelPolicy (programming-design §2): the per-channel programming contract —
-- scope / audience ceiling / separation / ordering / seasonal + applied
-- relaxations — that the suggester extracts and the scheduler enforces. Stored as
-- a JSON blob alongside lineup_json/desired_json (the store never queries inside
-- it; identity + reconcile scheduling remain the only indexed concerns).
--
-- Forward-only (§16). Unlike the disposable `clips` cache (00006), `channels`
-- holds durable rows, so this is the FIRST true ALTER TABLE ADD COLUMN — it
-- establishes that convention. The DDL default '{}' means existing channels get an
-- empty (all-defaults) policy with no data migration.

ALTER TABLE channels ADD COLUMN policy_json TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE channels DROP COLUMN policy_json;
