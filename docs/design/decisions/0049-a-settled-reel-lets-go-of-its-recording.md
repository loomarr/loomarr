# 0049. A split reel Loomarr fetched lets go of its recording once its pieces are settled

- **Date:** 2026-11-19
- **Status:** accepted
- **Amends:** [0017](0017-composites-keep-the-parent.md)'s "in the store and on disk" for a reel
  Loomarr fetched

## Context

0017 keeps a split reel on disk so it can be re-split when detection improves. Since the automatic
split (#3838 step 6) cuts every reel Loomarr fetches with nobody reviewing, kept reels became the
largest thing in filler that nothing could free: eviction looks only at clips on air, and a reel
never airs. On 2026-11-18 the maintainer's install held 86 of them, 43% of its 50 GB allowance,
and every fetch was refused for room. The maintainer chose to let them go and fetch again on a
re-split (#4421).

## Decision

- **Once every piece cut from a reel Loomarr fetched is settled, upkeep lets the reel's recording
  go.** Settled means each piece still in the catalog is Ready, rejected, dismissed or complete; a
  piece still running, waiting on a person or not yet enrolled keeps its reel. A reel still on the
  belt, or with a split proposal left, is kept too.
- **The row stays.** In one transaction it is reaped and its pipeline row records
  `files_deleted_at`, then its files go to the 24-hour quarantine like any other upkeep release.
  The row stays present: every piece's `parent_hash` resolves, provenance and inherited context
  survive, and the sync's prune spares a reaped row.
- **A reel added by hand is never let go.** It may be the operator's only copy.
- **A re-split fetches the reel again** from its recorded acquisition. Until that lands, a split of
  a let-go reel is refused with a 409 saying its recording is gone, and a rewind is refused as for
  any clip whose files were deleted.

## Consequences

- A fetched reel costs disk only while its pieces are being worked.
- Re-splitting a fetched reel costs a download, not a disk reservation held for good.
- Split review of a let-go reel has nothing to play; it is refused before any job starts.
