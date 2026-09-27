# 0013. Filler pulls are the approval gate; removal is a tombstone

- **Date:** 2026-08-02
- **Status:** accepted

## Context

"The machine proposes, a human commits" applied to filler only as an intention: there was a listing
endpoint and a download button, and nothing a human could approve. Separately, removing a clip by
deleting its row did not work, because the catalog is a synced cache of the clip folder and the
next scan re-created the row.

## Decision

A **pull** is a persisted multi-source acquisition plan that sits in the approval queue beside
title proposals; nothing downloads until it is approved, and approval enqueues through the
ordinary ingest path. The gate binds bulk composition, not an admin's deliberate one-item action.
**Remove from catalog** writes a tombstone that the scan's upsert cannot clear; it deletes neither
the row nor the file.

## Consequences

- A bulk backfill is always something a human saw and agreed to.
- A removed clip stays removed across rescans, and restoring it is the same write reversed.
- Loomarr never deletes operator media to implement removal.
