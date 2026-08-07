-- +goose Up
-- V44: richer clip metadata — the persisted transcript, the advertiser brand, and what a vision
-- pass read off the frame.
--
-- Four columns, and the state each carries mirrors the language column (00036) it sits beside:
--
--   transcript    ''  NOT YET TRANSCRIBED (the default, every existing row) — the transcribe job
--                     has not reached this clip, OR the clip is wordless and was never a candidate.
--                     The clip row does not need to tell those apart; a wordless clip is simply
--                     tagged from its other signals. Persisted (not discarded after tagging, as the
--                     splitter did pre-V44) so it is BOTH a searchable field AND the richest input
--                     the tagger gets — a cereal advert with no description still SAYS "Kellogg's".
--   brand         ''  NO GROUNDED ADVERTISER. The honest common case, not a failure. A brand is
--                     written only when it appears literally in a text signal or the visible_text a
--                     vision pass read — the era grounding rule (00024) generalised.
--   visible_text  ''  NO VISION PASS YET, or the pass read nothing. This is what makes vision
--                     AUDITABLE: a brand/era a vision model asserts is grounded only if the text it
--                     claims to have SEEN is here to support it. Never a display field alone.
--   vision_tagged     whether a VISION pass (keyframes → multimodal model) contributed tags,
--                     distinct from ai_tagged (text-only). Separate trust, separate cost: a human
--                     wants to know a tag came from pixels, and a re-run avoids paying twice.
--
-- ⚠ vision_tagged is BOOLEAN, matching ai_tagged/held/auto_filed after 00033 rebuilt this table —
-- NOT an INTEGER. The dialect split is per column and this table's bools live on the BOOLEAN side.
--
-- ⚠ All four are written ONLY by their dedicated job methods (SetClipTranscript, SetClipVisionTags),
-- never by UpsertClip's DO UPDATE list — exactly like language, removed_at, and the play counters.
-- The folder scan knows none of these, so letting them ride the upsert would blank a transcribed or
-- vision-tagged clip on the next sync and re-trigger ~341s of Whisper (or a paid vision call) over
-- work already done. This is the same failure 00036's language column documents.
--
-- Safe as plain ADD COLUMNs: `clips` is a synced cache (00013), so the worst case is one cycle
-- where nothing has been transcribed or vision-tagged yet — the honest state anyway.
--
-- Forward-only (§16).
ALTER TABLE clips ADD COLUMN transcript TEXT NOT NULL DEFAULT '';
ALTER TABLE clips ADD COLUMN brand TEXT NOT NULL DEFAULT '';
ALTER TABLE clips ADD COLUMN visible_text TEXT NOT NULL DEFAULT '';
ALTER TABLE clips ADD COLUMN vision_tagged BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
-- No down: forward-only (§16).
SELECT 1;
