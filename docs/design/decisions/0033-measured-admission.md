# 0033. Admission is one measured ledger

- **Date:** 2026-09-27
- **Status:** accepted
- **Supersedes:** static per-host stream capacity

## Context

Hosts differ by orders of magnitude: a GPU's encoder session cap, its throughput per stream class,
and a CPU-only host's cores. A static channel count was either unsafe or wasteful, and separate
checks in different places disagreed (#1505).

## Decision

One ledger, `playout.ResourceBudget`, admits every stream against what this host measured (#1520):
`min(GPU throughput at 1.2×, CPU allowance ÷ measured CPU per stream, encoder sessions)`, per stream
class and ladder rung. A background class probe measures each class through the live builder; live
encodes refine the CPU term. `playout.max_channels` can only lower the result. A new stream drops a
rung before it is refused, and a live session is never evicted.

## Consequences

- A packager is priced by the item airing now, so a heavy first item is demoted or refused (503)
  before any encoder starts.
- The probe runs at low priority and yields the moment a live transcode is admitted.
- Encoder sessions held by other applications count against the cap.
- The dashboard's capacity and the playout status read this one ledger.
