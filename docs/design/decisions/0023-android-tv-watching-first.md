# 0023. Android TV is watching-first with one permanent app identity

- **Date:** 2026-08-22
- **Status:** accepted

## Context

The TV is where a household actually watches channels, with a remote rather than a pointer. A TV
client that mirrored the admin web app would be hard to use from a couch and would widen what a
living-room device can do.

## Decision

The Android TV client opens on Watching and keeps one tuned Channel as its home state, with Surf and
Guide as the only other surfaces. It is a member-scoped adapter over the same Channel, Guide and
signed-HLS contracts as Web, authenticated by a revocable device credential; it never gets an admin
session or the media server's token. `loomarr.media` is its one permanent production application
id; development and Storybook builds keep an isolated identity, and only an explicit release
configuration selects the production one.

## Consequences

- Admin work stays on the web UI; the TV cannot change configuration.
- A server is found by local discovery and chosen explicitly; discovery grants no trust.
- Release mechanics for that identity live with the Android beta docs (#1572).
