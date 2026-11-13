# 0048. A Ready clip Loomarr fetched keeps only its airing copy

- **Date:** 2026-11-12
- **Status:** accepted
- **Supersedes:** [0028](0028-source-evidence-and-playable-media.md)'s "an admitted ad keeps its
  master" (its 2026-10-27 amendment) and "masters are not garbage-collected"

## Context

0028 kept every acquired original for good, so reprocessing and recovery never depended on a lossy
rendition. On a household install that made filler a collection that only grows: on 2026-11-12 the
maintainer's install had a full 50 GB budget, every fetch refused, and a large share of the bytes in
originals of clips already on air. The maintainer decided filler is a cache sized to the channels,
not a collection to keep (#3838, decision 14, grilled 2026-11-12).

## Decision

- **Once a clip Loomarr fetched is Ready, its original download is released.** The file goes to
  the 24-hour quarantine, then away. The sidecar keeps the original's SHA-256, length and sparse id
  with no path: what was acquired stays on record, and the playback copy is what airs.
- **A clip added by hand keeps its original.** So does a clip the library re-encode still has to
  move onto the current profile; its original is released after.
- **An original another clip still names stays** until the last clip naming it is released.
- **A clip that needs reprocessing is fetched again, or retired and replaced.** Loomarr does not
  re-run the transcode, split or screening of a clip whose original is released, and refuses a
  rewind to those rungs. Later rungs read the playback copy and still run.

## Consequences

- Disk per airable minute falls to about the size of the playback copy.
- A playback-recipe change after release reaches only clips fetched afterwards; the library
  re-encode skips a clip with no original.
- The evidence copy was already dropped at admission (0028's amendment), so no rung after Ready
  read the original; nothing that runs on its own loses an input.
- The sidecar's media manifest accepts a source master with no path; its authority digest ignores
  paths, so no screening identity moves when an original is released.
