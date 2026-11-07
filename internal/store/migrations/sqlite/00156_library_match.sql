-- +goose Up
-- Library → Wrong title? (#2151 step 11, decision 26): what said which title each movie or series
-- is, so the title page can send the maintainer to the app that decides, or let them pick.
--
-- `match_basis` is the evidence (manager, embedded_id, sidecar, link; '' nothing), `match_app` the
-- provider that said it ('' for evidence Loomarr finds itself), `match_page` the title's page in
-- that app ('' none). The scanner writes them with the title; `grounded` stays the Needs you test.
ALTER TABLE library_items ADD COLUMN match_basis TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN match_app TEXT NOT NULL DEFAULT '';
ALTER TABLE library_items ADD COLUMN match_page TEXT NOT NULL DEFAULT '';

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
