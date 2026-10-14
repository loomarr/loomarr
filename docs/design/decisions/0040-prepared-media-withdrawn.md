# 0040. Prepared media is withdrawn; nothing encodes until a viewer tunes

- **Date:** 2026-09-27
- **Status:** accepted
- **Supersedes:** V55–V56 prepared publications and the continuous-stream design

## Context

Prepared media pre-encoded channels ahead of time so a tune could start from finished files. On a
household server it starved the CPU and wedged the store. The maintainer ruled it the wrong
architecture on 2026-09-26 (#1509, #1510), along with the continuous-stream design (#1460, #1507).

## Decision

A channel encodes only while someone watches it. With no viewer, no encoder process runs, and there
is no boot warm-up (#1542). Phase 4 of #1512 removed the prepared-media code (#1547).

## Consequences

- Idle channels cost nothing; the first tune pays the cold start, which 1 s segments keep short
  ([0038](0038-one-second-segments.md)).
- Channel stills for a cold channel are one CPU-decoded frame of the airing on now, not a
  pre-encoded asset.
