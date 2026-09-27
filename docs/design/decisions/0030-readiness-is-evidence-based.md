# 0030. Readiness is evidence-based; confidence never publishes

- **Date:** 2026-09-15
- **Status:** accepted
- **Supersedes:** publishing a clip when its tagging confidence cleared a threshold

## Context

The tagger's grounding-capped confidence score was once an auto-file threshold: a clip whose tags
looked well grounded became airable. But a grounded taxonomy result is not proof that the bytes
play, that the content suits the household, or that anyone chose the source. It also turned optional
classifier gaps into approval chores for people who had already chosen the source.

## Decision

One Go-owned terminal-ready module owns the transition from conveyor completion to the playable
catalog. It needs the exact clip, durable enrollment authority (from enabling a source, opting a
folder in, or queueing an item) and completed required work, and commits placement, hold release and
a durable event in one transaction. Confidence stays as versioned diagnostic metadata and is never
read by readiness. Classification certification measures classifiers and never grants or withholds
readiness.

## Consequences

- The store has no generic "unhold" writer; only the terminal-ready transaction publishes.
- Missing optional enrichment never blocks an enrolled clip, and objective media failures still
  reject.
- Needs help holds only real choices; operational failures live in Diagnostics.
