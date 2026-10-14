# 0016. Clip identity is a content hash

- **Date:** 2026-08-03
- **Status:** accepted
- **Supersedes:** path identity (and, before it, the Tunarr program id)

## Context

Clip identity had already moved from Tunarr's program id to the path under the clip folder. Once
several watched folders were allowed, a path was unique only within its folder: two folders each
holding the same relative filename produced the same identity, and one silently overwrote the
other. A path also cannot answer "is this the same advert?".

## Decision

A clip's id is a sparse content hash (first 64 KB, last 64 KB and size); its path is where it
lives. The API's wire identity is the hash, and byte routes resolve the path server-side. Clips
live in the clip folder as `<hash>.<ext>`, and duplicates are catalogued once.

## Consequences

- Deleting one copy of a duplicate lets the survivor return with its tags.
- The sparse hash can collide on crafted or truncated files; conditioning and reuse boundaries
  compare full bytes.
- Changing identity drops the catalog once; sidecars restore tags, but play counts and pins are
  lost and the app says so.
