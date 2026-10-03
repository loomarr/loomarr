# 0042. The iPhone app has one permanent identity, selected only by its release build

- **Date:** 2026-10-03
- **Status:** accepted

## Context

The Expo touch client has shipped only as an isolated prototype (`media.loomarr.mobile.prototype`).
TestFlight needs an App Store Connect record, and that record is bound to its bundle identifier for
good: changing it later makes a different app that testers must reinstall.

## Decision

`media.loomarr.mobile` is the iPhone app's one permanent bundle identifier, displayed as "Loomarr"
with the `loomarr` URL scheme. As with the TV client ([0023](0023-android-tv-watching-first.md)),
development, Storybook and simulator builds keep the prototype identity; only an explicit release
configuration (`LOOMARR_IOS_RELEASE_CHANNEL=testflight` with a validated version and build number)
selects the permanent one. The Android touch build keeps its prototype identity until it has its own
release decision.

## Consequences

- A simulator or development build can never be uploaded as, or replace, the TestFlight app.
- Release mechanics for this identity live with the iPhone beta work (#1816).
