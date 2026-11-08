-- +goose Up
-- Schema to a 10 (#2793): every JSON column is jsonb, the twin of SQLite's TEXT with
-- CHECK (json_valid(col)). A text default cannot be cast to jsonb, so each column drops its
-- default, changes type and sets the jsonb default again. Five columns used '' for "no document",
-- which is not JSON: they become nullable and each '' becomes NULL. The ledger's settled/unsettled
-- CHECK compared record_json with '', which jsonb cannot do, so it is replaced by one that tests
-- for NULL. It was declared unnamed, so it is found by its definition.
-- +goose StatementBegin
DO $$
DECLARE c text;
BEGIN
    FOR c IN SELECT conname FROM pg_constraint
              WHERE conrelid = 'filler_structure_window_call_ledger'::regclass AND contype = 'c'
                AND pg_get_constraintdef(oid) LIKE '%record_json%'
    LOOP
        EXECUTE format('ALTER TABLE filler_structure_window_call_ledger DROP CONSTRAINT %I', c);
    END LOOP;
END $$;
-- +goose StatementEnd

ALTER TABLE channels
    ALTER COLUMN lineup_json DROP DEFAULT,
    ALTER COLUMN lineup_json TYPE jsonb USING lineup_json::jsonb,
    ALTER COLUMN lineup_json SET DEFAULT '[]'::jsonb,
    ALTER COLUMN desired_json DROP DEFAULT,
    ALTER COLUMN desired_json TYPE jsonb USING desired_json::jsonb,
    ALTER COLUMN desired_json SET DEFAULT '[]'::jsonb,
    ALTER COLUMN policy_json DROP DEFAULT,
    ALTER COLUMN policy_json TYPE jsonb USING policy_json::jsonb,
    ALTER COLUMN policy_json SET DEFAULT '{}'::jsonb,
    ALTER COLUMN intent_json DROP DEFAULT,
    ALTER COLUMN intent_json DROP NOT NULL,
    ALTER COLUMN intent_json TYPE jsonb USING NULLIF(intent_json, '')::jsonb;
ALTER TABLE client_crash_reports
    ALTER COLUMN breadcrumbs_json DROP DEFAULT,
    ALTER COLUMN breadcrumbs_json TYPE jsonb USING breadcrumbs_json::jsonb,
    ALTER COLUMN breadcrumbs_json SET DEFAULT '[]'::jsonb;
ALTER TABLE diagnostic_events
    ALTER COLUMN attributes_json DROP DEFAULT,
    ALTER COLUMN attributes_json TYPE jsonb USING attributes_json::jsonb,
    ALTER COLUMN attributes_json SET DEFAULT '{}'::jsonb;
ALTER TABLE filler_clip_defects
    ALTER COLUMN readings_json TYPE jsonb USING readings_json::jsonb,
    ALTER COLUMN assessment_json TYPE jsonb USING assessment_json::jsonb,
    ALTER COLUMN judgement_json DROP DEFAULT,
    ALTER COLUMN judgement_json DROP NOT NULL,
    ALTER COLUMN judgement_json TYPE jsonb USING NULLIF(judgement_json, '')::jsonb;
ALTER TABLE filler_clip_fingerprints
    ALTER COLUMN fingerprint_json TYPE jsonb USING fingerprint_json::jsonb;
ALTER TABLE filler_clip_pipeline
    ALTER COLUMN stages_json DROP DEFAULT,
    ALTER COLUMN stages_json TYPE jsonb USING stages_json::jsonb,
    ALTER COLUMN stages_json SET DEFAULT '[]'::jsonb;
ALTER TABLE filler_context_reports
    ALTER COLUMN report_json TYPE jsonb USING report_json::jsonb;
ALTER TABLE filler_enrichment_axes
    ALTER COLUMN value_json DROP DEFAULT,
    ALTER COLUMN value_json TYPE jsonb USING value_json::jsonb,
    ALTER COLUMN value_json SET DEFAULT '{}'::jsonb;
ALTER TABLE filler_inference_evaluations
    ALTER COLUMN modalities_json DROP DEFAULT,
    ALTER COLUMN modalities_json TYPE jsonb USING modalities_json::jsonb,
    ALTER COLUMN modalities_json SET DEFAULT '[]'::jsonb;
ALTER TABLE filler_pulls
    ALTER COLUMN plan_json DROP DEFAULT,
    ALTER COLUMN plan_json TYPE jsonb USING plan_json::jsonb,
    ALTER COLUMN plan_json SET DEFAULT '[]'::jsonb;
ALTER TABLE filler_split_proposals
    ALTER COLUMN segments_json TYPE jsonb USING segments_json::jsonb;
ALTER TABLE filler_split_shadow_decisions
    ALTER COLUMN decision_json TYPE jsonb USING decision_json::jsonb;
ALTER TABLE filler_structure_window_call_ledger
    ALTER COLUMN reservation_json TYPE jsonb USING reservation_json::jsonb,
    ALTER COLUMN record_json DROP DEFAULT,
    ALTER COLUMN record_json DROP NOT NULL,
    ALTER COLUMN record_json TYPE jsonb USING NULLIF(record_json, '')::jsonb,
    ADD CONSTRAINT filler_structure_window_call_ledger_settled_check CHECK (
        (state = 'settled' AND record_sha256 <> '' AND record_json IS NOT NULL AND assessed_at > 0) OR
        (state <> 'settled' AND record_sha256 = '' AND record_json IS NULL AND assessed_at = 0)
    );
ALTER TABLE inventory_item_origins
    ALTER COLUMN observation_json TYPE jsonb USING observation_json::jsonb;
ALTER TABLE inventory_source_analysis
    ALTER COLUMN breaks_json TYPE jsonb USING breaks_json::jsonb,
    ALTER COLUMN active_picture_json TYPE jsonb USING active_picture_json::jsonb;
ALTER TABLE inventory_source_measurements
    ALTER COLUMN observation_json TYPE jsonb USING observation_json::jsonb;
ALTER TABLE inventory_source_origins
    ALTER COLUMN locator_json TYPE jsonb USING locator_json::jsonb,
    ALTER COLUMN observation_json TYPE jsonb USING observation_json::jsonb;
ALTER TABLE notification_destinations
    ALTER COLUMN topics_json TYPE jsonb USING topics_json::jsonb,
    ALTER COLUMN configuration_json DROP DEFAULT,
    ALTER COLUMN configuration_json TYPE jsonb USING configuration_json::jsonb,
    ALTER COLUMN configuration_json SET DEFAULT '{}'::jsonb,
    ALTER COLUMN credential_keys_json TYPE jsonb USING credential_keys_json::jsonb;
ALTER TABLE notification_intents
    ALTER COLUMN template_json TYPE jsonb USING template_json::jsonb;
ALTER TABLE proposal_jobs
    ALTER COLUMN intent_json DROP DEFAULT,
    ALTER COLUMN intent_json TYPE jsonb USING intent_json::jsonb,
    ALTER COLUMN intent_json SET DEFAULT '{}'::jsonb,
    ALTER COLUMN failure_trace_json DROP DEFAULT,
    ALTER COLUMN failure_trace_json DROP NOT NULL,
    ALTER COLUMN failure_trace_json TYPE jsonb USING NULLIF(failure_trace_json, '')::jsonb;
ALTER TABLE proposals
    ALTER COLUMN proposal_json DROP DEFAULT,
    ALTER COLUMN proposal_json TYPE jsonb USING proposal_json::jsonb,
    ALTER COLUMN proposal_json SET DEFAULT '{}'::jsonb;
ALTER TABLE series_episodes
    ALTER COLUMN episodes_json DROP DEFAULT,
    ALTER COLUMN episodes_json TYPE jsonb USING episodes_json::jsonb,
    ALTER COLUMN episodes_json SET DEFAULT '[]'::jsonb;
ALTER TABLE titles
    ALTER COLUMN title_json TYPE jsonb USING title_json::jsonb;
ALTER TABLE taxa
    ALTER COLUMN synonyms DROP DEFAULT,
    ALTER COLUMN synonyms TYPE jsonb USING synonyms::jsonb,
    ALTER COLUMN synonyms SET DEFAULT '[]'::jsonb,
    ALTER COLUMN aliases DROP DEFAULT,
    ALTER COLUMN aliases TYPE jsonb USING aliases::jsonb,
    ALTER COLUMN aliases SET DEFAULT '[]'::jsonb;
ALTER TABLE holidays
    ALTER COLUMN words DROP DEFAULT,
    ALTER COLUMN words TYPE jsonb USING words::jsonb,
    ALTER COLUMN words SET DEFAULT '{}'::jsonb,
    ALTER COLUMN kept DROP DEFAULT,
    ALTER COLUMN kept TYPE jsonb USING kept::jsonb,
    ALTER COLUMN kept SET DEFAULT '[]'::jsonb,
    ALTER COLUMN air_window DROP DEFAULT,
    ALTER COLUMN air_window DROP NOT NULL,
    ALTER COLUMN air_window TYPE jsonb USING NULLIF(air_window, '')::jsonb,
    ALTER COLUMN members DROP DEFAULT,
    ALTER COLUMN members TYPE jsonb USING members::jsonb,
    ALTER COLUMN members SET DEFAULT '[]'::jsonb;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
