# 0017. Composites keep the parent; lineage is first-class

- **Date:** 2026-08-07
- **Status:** accepted
- **Supersedes:** deleting the compilation on split confirm

## Context

A recorded ad break arrives as one file holding dozens of adverts. The first splitter deleted the
compilation's row and file on confirm, because its identity then "meant twenty clips". That threw
away provenance (which break did this advert air in?), any chance to re-split when detection
improved, and the broadcast context every child shares with its tape.

## Decision

Confirm keeps the compilation as a **composite**: in the store and on disk, marked composite and
never airable. Each child carries `parent_hash`. `IsComposite` is its own axis, separate from
`Kind`, so pods exclude containers once. A completed re-split replaces the prior generation of
children rather than stacking beside it.

## Consequences

- Split review can play each proposed cut as a byte range of the retained parent.
- Re-splitting is a re-run, and a retired cut stays restorable.
- Retained reels cost disk; the bounded split sweep may delete a reel's recording after its review
  window, which is the one place Loomarr deletes operator media, and it keeps the catalog row.
