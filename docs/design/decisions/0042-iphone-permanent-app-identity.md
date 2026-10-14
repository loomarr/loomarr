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

The release is iPhone-only: App Store Connect lets an app add iPad support later but never remove
it. Every prerelease of `x.y.z` (for example `0.2.0-beta.9`) ships in the `x.y.z` TestFlight version,
because Apple's version string is exactly three integers. The workflow's run number is the build
number that orders builds within it.

## Consequences

- Development and simulator builds carry a different bundle identifier, so they cannot be uploaded
  to, or installed over, the TestFlight app.
- Release metadata supplied without the release channel fails the build instead of producing a
  prototype-identity artifact.
- Re-running a release workflow run reuses its build number, which App Store Connect rejects once
  that number has been uploaded; upload again from a new run.
- Release mechanics for this identity live with the iPhone beta work (#1816).
