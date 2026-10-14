# 0028. Source evidence and playable media are separate assets

- **Date:** 2026-09-06
- **Status:** accepted
- **Supersedes:** a single mezzanine that replaced the acquired bytes

## Context

Transcode kept one H.264/AAC mezzanine and deleted the bytes it came from, and Archive acquisition
picked the smallest derivative. That is fine for playback and the wrong foundation for finding
where a commercial ends, reading small print, telling an upload defect from the source, or
reproducing a model's judgement later.

## Decision

Every clip that reaches transcode has three roles: an immutable **source master** (the exact
acquired bytes, identified by full SHA-256), a reproducible **evidence derivative** for inspection
and models, and a **playback derivative** for playout. Both derivatives are built from the master
under versioned recipes, and a manifest in the sidecar binds them. Archive acquisition picks the
best source representation, not the cheapest file.

## Consequences

- Reprocessing and recovery never depend on a lossy rendition.
- Storage grows; masters are not garbage-collected until an ownership graph can prove one
  disposable.
- Split children bind the parent's media-asset digest and source role, so a later playback-recipe
  change cannot change what a split meant.
