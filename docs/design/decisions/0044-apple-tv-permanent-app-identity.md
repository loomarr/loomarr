# 0044. The Apple TV app is its own App Store Connect app, selected only by its release build

- **Date:** 2026-10-13
- **Status:** accepted

## Context

The Expo TV client ships to the Shield as `loomarr.media` ([0023](0023-android-tv-watching-first.md)),
and its Apple TV build has only run in the Simulator under the prototype identity
(`media.loomarr.tv.prototype`). Internal testers need it through TestFlight, as the iPhone app
already is ([0042](0042-iphone-permanent-app-identity.md)). An App Store Connect record is bound to
its bundle identifier for good.

## Decision

The Apple TV app is a separate App Store Connect app, "Loomarr TV", with the permanent bundle
identifier `media.loomarr.tv` (registered as a universal App ID), the same Household internal
testers, and its own build numbers. It does not share the iPhone record: the two apps are different
clients with different screens, and each uploads on its own schedule.

As for the iPhone, development, Storybook and Simulator builds keep the prototype identity; only an
explicit release configuration (`LOOMARR_APPLE_RELEASE_CHANNEL=testflight` with a validated version
and build number) selects the permanent one, and only for the Apple TV build. The Shield identity is
untouched, and one build cannot select both. Both Apple apps release through one workflow and one
script, which differ only in a table of per-app values.

## Consequences

- A push to `main` that changes the TV client uploads an Apple TV build, as one that changes the
  iPhone client uploads an iPhone build.
- The Apple TV build needs tvOS brand assets (home-screen icon and Top Shelf images), drawn from the
  brand contract by `make brand-assets`.
- Release mechanics live in [Releasing](../../contributing/releasing.md#apple-testflight) (#1500).
