-- +goose Up
-- Schema to a 10 (#2793, item 1): a crash report names the paired device that sent it by foreign
-- key. client_crash_reports.device_id references device_tokens (the device's token hash) and is
-- cleared when that device is unpaired (SET NULL): the report outlives the device, as it outlives
-- the person (user_id).
--
-- '' (sent from a browser session, not a paired device) becomes NULL, as does a device already
-- unpaired.
--
-- SQLite adds a foreign key to an existing column by adding a keyed column, copying the values
-- across, and dropping the old one under its name: no table is rebuilt.
ALTER TABLE client_crash_reports ADD COLUMN device_ref TEXT REFERENCES device_tokens(token_hash) ON DELETE SET NULL;
UPDATE client_crash_reports SET device_ref = device_id
WHERE EXISTS (SELECT 1 FROM device_tokens d WHERE d.token_hash = client_crash_reports.device_id);
ALTER TABLE client_crash_reports DROP COLUMN device_id;
ALTER TABLE client_crash_reports RENAME COLUMN device_ref TO device_id;
CREATE INDEX idx_client_crash_reports_device ON client_crash_reports (device_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
