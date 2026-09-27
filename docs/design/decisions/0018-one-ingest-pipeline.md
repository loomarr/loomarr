# 0018. Ingest is one ordered, watchable pipeline

- **Date:** 2026-08-09
- **Status:** accepted
- **Supersedes:** one cron sweep per capability

## Context

Filler grew one cron job per capability (sync, fetch, language, split, transcribe, vision, taxonomy),
each sweeping the whole catalog on its own schedule. A clip's progress was invisible for up to an
hour, nothing owned the order (tagging could run before transcription and never re-run), cheap and
expensive work competed with no budget, and failures retried at full cost forever.

## Decision

Replace the sweeps with one ordered per-clip pipeline and one driver job (V51b). Stages run in a fixed
order, one clip at a time, within per-run budgets, with backoff and a defined resolution for each
failure. Stage state is persisted beside the clip cache and served by the API; SSE only speeds it up.

## Consequences

- An operator can see which stage each clip is in, and the Incoming view is one conveyor.
- Media work never starves live channels, because the pipeline is sequential.
- Later work (enrichment, preparation progress, recovery rewinds) extends the same driver instead of
  adding new sweeps.
