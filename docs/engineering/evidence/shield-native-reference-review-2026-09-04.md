# Shield native reference review — 2026-09-04

## Scope and method

This review covers GitHub issue #970 item 34 at base commit
`8eb769556b1e95c5b31facf037828ae46d08da88`. The embedded React Native Storybook ran as an Android
TV application on Android 36 Google TV emulator profiles with native framebuffers at:

- 1920×1080, 320 dpi (`1080p`)
- 3840×2160, 640 dpi (`4k`)

Both profiles therefore expose the same 960×540 dp composition. The capture command is
`pnpm --filter @loomarr/tv references:capture`; it detects the native framebuffer, selects each
Storybook state through a localhost-only controller bridged with `adb reverse`, and rejects logical
`wm size` overrides. The checked-in artifact test proves all 30 PNGs exist at their claimed physical
dimensions.

## Captured replacement contract

| Surface | Ready/focused | Loading | Empty | Error |
| --- | --- | --- | --- | --- |
| Pairing | [1080p](../../../web/apps/tv/tests/native-references/1080p/pairing.png) · [4K](../../../web/apps/tv/tests/native-references/4k/pairing.png) | [1080p](../../../web/apps/tv/tests/native-references/1080p/pairing-loading.png) · [4K](../../../web/apps/tv/tests/native-references/4k/pairing-loading.png) | Not applicable | [1080p](../../../web/apps/tv/tests/native-references/1080p/pairing-error.png) · [4K](../../../web/apps/tv/tests/native-references/4k/pairing-error.png) |
| Watching | [1080p](../../../web/apps/tv/tests/native-references/1080p/watching.png) · [4K](../../../web/apps/tv/tests/native-references/4k/watching.png) | [1080p](../../../web/apps/tv/tests/native-references/1080p/watching-loading.png) · [4K](../../../web/apps/tv/tests/native-references/4k/watching-loading.png) | [1080p](../../../web/apps/tv/tests/native-references/1080p/watching-empty.png) · [4K](../../../web/apps/tv/tests/native-references/4k/watching-empty.png) | [1080p](../../../web/apps/tv/tests/native-references/1080p/watching-error.png) · [4K](../../../web/apps/tv/tests/native-references/4k/watching-error.png) |
| Surf | [1080p](../../../web/apps/tv/tests/native-references/1080p/surf-focused.png) · [4K](../../../web/apps/tv/tests/native-references/4k/surf-focused.png) | [1080p](../../../web/apps/tv/tests/native-references/1080p/surf-loading.png) · [4K](../../../web/apps/tv/tests/native-references/4k/surf-loading.png) | [1080p](../../../web/apps/tv/tests/native-references/1080p/surf-empty.png) · [4K](../../../web/apps/tv/tests/native-references/4k/surf-empty.png) | [1080p](../../../web/apps/tv/tests/native-references/1080p/surf-error.png) · [4K](../../../web/apps/tv/tests/native-references/4k/surf-error.png) |
| Guide | [1080p](../../../web/apps/tv/tests/native-references/1080p/guide-focused.png) · [4K](../../../web/apps/tv/tests/native-references/4k/guide-focused.png) | [1080p](../../../web/apps/tv/tests/native-references/1080p/guide-loading.png) · [4K](../../../web/apps/tv/tests/native-references/4k/guide-loading.png) | [1080p](../../../web/apps/tv/tests/native-references/1080p/guide-empty.png) · [4K](../../../web/apps/tv/tests/native-references/4k/guide-empty.png) | [1080p](../../../web/apps/tv/tests/native-references/1080p/guide-error.png) · [4K](../../../web/apps/tv/tests/native-references/4k/guide-error.png) |

## Kotlin side-by-side findings

The committed Kotlin references are:

- [Pairing](../../../android/app/src/test/screenshots/loomarr.media.design.DesignScreenshotTest.pairing%20with%20a%20real%20hostname%20stays%20on%20one%20line.png)
- [Watching 1080p](../../../android/app/src/test/screenshots/loomarr.media.design.DesignScreenshotTest.android%20tv%20watching.png) and [4K](../../../android/app/src/test/screenshots/loomarr.media.design.DesignScreenshotTest.android%20tv%20watching%20at%204k%20density.png)
- [Surf 1080p](../../../android/app/src/test/screenshots/loomarr.media.design.DesignScreenshotTest.android%20tv%20surf.png) and [4K](../../../android/app/src/test/screenshots/loomarr.media.design.DesignScreenshotTest.android%20tv%20surf%20at%204k%20density.png)
- [Guide 1080p](../../../android/app/src/test/screenshots/loomarr.media.design.DesignScreenshotTest.android%20tv%20guide.png) and [4K](../../../android/app/src/test/screenshots/loomarr.media.design.DesignScreenshotTest.android%20tv%20guide%20at%204k%20density.png)

The review found that the replacement contract is complete and resolution-independent, but the
current React Native ready/focused surfaces are **not yet acceptable as 1:1 Kotlin visual parity**.
These are product-level geometry differences, not merely Compose-versus-React-Native rasterization:

- **Pairing:** both versions preserve QR and website paths, code, expiry, and focused refresh action.
  React Native adds the Loomarr wordmark and uses a materially wider/taller card with different
  column, divider, and action placement.
- **Watching:** both preserve number entry, Channel identity, now/next information, progress, and
  remote hints over a full-screen player. The replacement changes the scale and placement of both
  top indicators and materially changes the height, inset, information order, and progress geometry
  of the bottom programme bar.
- **Surf:** both preserve the translucent left rail, Favorites/Recent/All grouping, current marker,
  focus ring, progress, row position, version identity, and tune/cancel hint. The checked-in fixtures
  differ in Channel count and scroll position, and the replacement row typography, vertical rhythm,
  and rail content density do not yet reproduce the Kotlin reference closely enough for acceptance.
- **Guide:** both preserve filters, two-hour grid, current-time rail, programme focus, position rail,
  and bottom detail/artwork fallback. The replacement uses substantially taller Channel rows and a
  larger header/detail composition, exposing far fewer rows than Kotlin on the same 960×540 dp
  canvas. This is a direct geometry mismatch.

React Native 1080p and 4K captures do preserve the same logical composition; the only observed
cross-density difference is the expected one-second expiry countdown movement. All loading, empty,
error, and focused variants render the intended state without Storybook chrome or emulator scaling.

## Acceptance decision

Item 34's capture and review evidence is complete. Visual parity is **not accepted** by this review.
The Pairing, Watching, Surf, and Guide geometry gaps above must be remediated and recaptured before
issue #970 item 36 can record maintainer acceptance or item 38 can delete the Kotlin reference
implementation. Physical Shield behavior and real playback remain explicitly untested here and are
owned by item 36.
