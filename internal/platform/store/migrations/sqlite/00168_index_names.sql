-- +goose Up
-- Schema naming (#2323): an index is named `idx_<table>_<purpose>`, so its name says which table it
-- serves. Two indexes duplicated a key that already indexes the same leading column (the clip_tags
-- primary key and device_pairings' UNIQUE user_code), so they go. SQLite cannot rename an index, so
-- each is dropped and created again under its new name with the same definition.
DROP INDEX idx_clip_tags_clip;
DROP INDEX idx_device_pairings_user_code;

DROP INDEX broadcast_station_call_signs_by_sign;
CREATE INDEX idx_broadcast_station_call_signs_sign ON broadcast_station_call_signs (call_sign);

DROP INDEX idx_contact_normalized_status;
CREATE INDEX idx_contact_addresses_normalized_status ON contact_addresses (normalized, status);

DROP INDEX discovery_feedback_scope_created;
CREATE INDEX idx_discovery_feedback_scope_created
    ON discovery_feedback (scope, scope_id, created_at_ns DESC, id DESC);

DROP INDEX library_items_set;
CREATE INDEX idx_library_items_set ON library_items (set_key);

DROP INDEX idx_filler_enrichment_state_axis;
CREATE INDEX idx_filler_enrichment_axes_state_axis ON filler_enrichment_axes (state, axis);

DROP INDEX idx_filler_inference_clip_created;
CREATE INDEX idx_filler_inference_evaluations_clip_created
    ON filler_inference_evaluations (clip_hash, created_at DESC, id DESC);
DROP INDEX idx_filler_inference_cache_key;
CREATE INDEX idx_filler_inference_evaluations_cache_key
    ON filler_inference_evaluations (cache_key, created_at DESC, id DESC);
DROP INDEX idx_filler_inference_run_created;
CREATE INDEX idx_filler_inference_evaluations_run_created
    ON filler_inference_evaluations (run_id, created_at DESC, id DESC);
DROP INDEX idx_filler_inference_day_budget;
CREATE INDEX idx_filler_inference_evaluations_day_budget
    ON filler_inference_evaluations (created_at, reserved_nano_usd);

DROP INDEX idx_filler_structure_window_call_source;
CREATE INDEX idx_filler_structure_window_call_ledger_source
    ON filler_structure_window_call_ledger (source_sha256, media_set_sha256, requested_at DESC, request_sha256);
DROP INDEX idx_filler_structure_window_call_open;
CREATE INDEX idx_filler_structure_window_call_ledger_open
    ON filler_structure_window_call_ledger (state, requested_at ASC, request_sha256);

DROP INDEX idx_notification_attempts_due;
CREATE INDEX idx_notification_delivery_attempts_due
    ON notification_delivery_attempts (available_at, id) WHERE status = 'queued';

DROP INDEX user_favourite_channels_channel;
CREATE INDEX idx_user_favourite_channels_channel ON user_favourite_channels (channel_id);
DROP INDEX user_recent_channels_channel;
CREATE INDEX idx_user_recent_channels_channel ON user_recent_channels (channel_id);
DROP INDEX user_recent_channels_user_tuned;
CREATE INDEX idx_user_recent_channels_user_tuned ON user_recent_channels (user_id, tuned_at_ms DESC);

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
