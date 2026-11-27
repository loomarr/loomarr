-- +goose Up
-- Schema to a 10 (#2793, item 1): a crash report names the paired device that sent it by foreign
-- key. client_crash_reports.device_id references device_tokens (the device's token hash) and is
-- cleared when that device is unpaired (SET NULL): the report outlives the device, as it outlives
-- the person (user_id).
--
-- '' (sent from a browser session, not a paired device) becomes NULL, as does a device already
-- unpaired.
ALTER TABLE client_crash_reports ALTER COLUMN device_id DROP NOT NULL, ALTER COLUMN device_id DROP DEFAULT;
UPDATE client_crash_reports r
SET device_id = (SELECT d.token_hash FROM device_tokens d WHERE d.token_hash = r.device_id);
ALTER TABLE client_crash_reports ADD CONSTRAINT client_crash_reports_device_id_fkey
    FOREIGN KEY (device_id) REFERENCES device_tokens(token_hash) ON DELETE SET NULL;
CREATE INDEX idx_client_crash_reports_device ON client_crash_reports (device_id);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
