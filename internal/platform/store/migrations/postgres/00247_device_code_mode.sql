-- +goose Up
-- Code Mode (#4486): an admin's paired device may run JavaScript against the API through the MCP
-- endpoint's execute tool. Off for every device until its owner turns it on, and only an admin's
-- device is ever offered the tools; a demoted owner's device loses them on the next request.
ALTER TABLE device_tokens ADD COLUMN code_mode BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
