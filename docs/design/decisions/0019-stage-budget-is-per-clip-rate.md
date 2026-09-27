# 0019. A stage's budget is a per-clip rate

- **Date:** 2026-08-09
- **Status:** accepted

## Context

A long commercial-break recording split into dozens of segments, and the split rung classified every
segment with a model call inside one clip's time budget. The pass was killed at its deadline, the
failure looked like any other, and the rung retried the same doomed work indefinitely (V51g). A later
fix bounded split-time vision per pass, but it was read as a limit per reel (V54).

## Decision

The scheduler's unit is a clip. A rung may not spend per segment what the budget allows per clip:
splitting only cuts, and each segment pays its own stages as a new clip. Any per-segment model call
needs its own per-pass budget setting, and that budget is a rate across passes, not a ceiling on the
reel. Running out of time is a deferral that keeps progress, never a failure.

## Consequences

- Partly-grounded reels defer and resume from their persisted proposal instead of starting over.
- Every rung must be able to make progress within one pass, or it must persist partial work.
- The incident is archived in
  [`filler-v51g-budget-incident.md`](../../engineering/archive/design-2026-09/filler-v51g-budget-incident.md).
