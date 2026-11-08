-- +goose Up
-- Schema naming (#2323): an index is named `idx_<table>_<purpose>`, so its name says which table it
-- serves. Two indexes duplicated a key that already indexes the same leading column (the clip_tags
-- primary key and device_pairings' UNIQUE user_code), so they go.
DROP INDEX idx_clip_tags_clip;
DROP INDEX idx_device_pairings_user_code;

ALTER INDEX broadcast_station_call_signs_by_sign RENAME TO idx_broadcast_station_call_signs_sign;
ALTER INDEX idx_contact_normalized_status RENAME TO idx_contact_addresses_normalized_status;
ALTER INDEX discovery_feedback_scope_created RENAME TO idx_discovery_feedback_scope_created;
ALTER INDEX library_items_set RENAME TO idx_library_items_set;
ALTER INDEX idx_filler_enrichment_state_axis RENAME TO idx_filler_enrichment_axes_state_axis;
ALTER INDEX idx_filler_inference_clip_created RENAME TO idx_filler_inference_evaluations_clip_created;
ALTER INDEX idx_filler_inference_cache_key RENAME TO idx_filler_inference_evaluations_cache_key;
ALTER INDEX idx_filler_inference_run_created RENAME TO idx_filler_inference_evaluations_run_created;
ALTER INDEX idx_filler_inference_day_budget RENAME TO idx_filler_inference_evaluations_day_budget;
ALTER INDEX idx_filler_structure_window_call_source RENAME TO idx_filler_structure_window_call_ledger_source;
ALTER INDEX idx_filler_structure_window_call_open RENAME TO idx_filler_structure_window_call_ledger_open;
ALTER INDEX idx_notification_attempts_due RENAME TO idx_notification_delivery_attempts_due;
ALTER INDEX user_favourite_channels_channel RENAME TO idx_user_favourite_channels_channel;
ALTER INDEX user_recent_channels_channel RENAME TO idx_user_recent_channels_channel;
ALTER INDEX user_recent_channels_user_tuned RENAME TO idx_user_recent_channels_user_tuned;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
