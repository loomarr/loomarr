# 0015. Filler sources fetch on their own

- **Date:** 2026-08-03
- **Status:** accepted
- **Supersedes:** "there is no unattended crawler"

## Context

Adding a source did nothing until someone pasted a URL or approved a pull, so an operator had to
return by hand to keep a catalog growing. The earlier rule against an unattended crawler existed
for a real reason: unattended fetching can fill a stranger's disk.

## Decision

Registered, enabled sources are checked on their effective schedule (`filler.fetch.every`, with
nullable per-source overrides) and new items download without anyone asking. The concern survives
as limits rather than a prohibition: per-source and per-provider counts per check, a catalog
ceiling, and a storage governor with a soft allowance and a hard host reserve. Everything fetched
arrives held, and a limit that is reached is reported.

## Consequences

- Clips arrive because a source was added, not because someone keeps asking.
- Bulk backfill of a large collection stays a pull, where a human approves the plan.
- "Nothing new arrived" must always be explainable from live state.
