# 0029. Compilation structure is assessed before children are materialized

- **Date:** 2026-09-06
- **Status:** accepted
- **Supersedes:** treating every duration-quarantined source as a compilation

## Context

The duration quarantine answers "is this too long to air as one clip?", not "does this contain
several commercials?". A programme excerpt, a damaged capture, a long infomercial and a compilation
all cross the same threshold. The detector cut all of them, dropped time it could not explain, and
let children inherit the parent's generic kind.

## Decision

One structure-assessment module turns an exact evidence asset and independent observations into a
verdict (`single_unit`, `compilation_break`, `programme_with_spots`, `ambiguous`, `unusable`) and a
plan that covers the whole timeline, with an independent role per kept interval. Unattended
materialization needs agreement between independently locked producer families under a
deterministic reducer, inside a certified slice; anything else is held for review. Structure
authority only creates held children; it is never broadcast admission.

## Consequences

- No time disappears: every discarded span is explained in the plan.
- The legacy detector survives only as a proposal and shadow input.
- Automatic splitting expands slice by slice, as measurement certifies each one.
