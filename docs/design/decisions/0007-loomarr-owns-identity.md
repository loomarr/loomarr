# 0007. Loomarr owns identity; every credential path lands on one users table

- **Date:** 2026-07-13
- **Status:** accepted
- **Supersedes:** "the first media-server admin to sign in claims the instance; users are created lazily"

## Context

The first design let media-server accounts sign in and created users on first login. That made access
a side effect of who held an account somewhere else, and an install could not work without a media
server.

## Decision

The local `users` table is the allowlist and source of truth. A credential proves who someone is; the
row decides whether they may enter and what they may do. Local, imported media-server and SSO
credentials all resolve to that one table. A media-server account grants nothing until an admin
explicitly imports it.

## Consequences

- An install works with zero media-server configuration; first run creates a local admin.
- A login for a name with no row is rejected even with valid external credentials.
- Roles, quotas and disabled state are Loomarr's decisions on every path.
