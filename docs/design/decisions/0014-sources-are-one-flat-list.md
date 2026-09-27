# 0014. Filler sources are one flat list, any number of folders and libraries

- **Date:** 2026-08-03
- **Status:** accepted
- **Supersedes:** the derived-folder/registered-collection split; the one-folder, one-library
  singleton rule

## Context

The drop folder was derived from configuration and switched by a setting, while remote collections
were rows with their own switch. That asymmetry kept one source from appearing twice, but it could
not express several folders or libraries, which is an ordinary household situation. A singleton
rule added to preserve "no source appears twice" forbade legitimate second folders without
preventing the real problem.

## Decision

Every source is one row in one list, whatever backs it (`folder`, `library`, `archive`,
`youtube`). Uniqueness is on the target: one row per distinct path or library id. The
config-backed rows are always present, even unset, so "not configured" stays expressible. Provider
grouping is derived from `kind`, never stored.

## Consequences

- The list is the whole answer to "where does filler come from?".
- `filler.dir` is the first folder and the default download target, not the only one scanned.
- A media-server library is one acquisition source among several and never the catalog's only
  route.
