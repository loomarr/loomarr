# TV Guide D-pad edges and auto-tune wiring: emulator evidence

Evidence for the PR that wires #1842's TV pieces ([#1817](https://github.com/loomarr/loomarr/issues/1817),
[#1659](https://github.com/loomarr/loomarr/issues/1659), maintainer decision Q-N4). All data is the
emulator journey's invented fixture (`web/scripts/tv-emulator-fixture-server.mjs`); nothing ran
against a real server.

## Device

Android 11 x86_64 emulator (`google_apis` image, AVD `loomarr-tv-x64`), 1920×1080 at 320 dpi, so the
960×540 logical TV canvas at 2×. It is not an Android TV system image: the only Android TV image
installed is 32-bit x86, which the build scripts reject. It is also not a Shield, so overscan, decode
and remote hardware are not covered.

The build is the embedded debug APK from `web/scripts/build-android-client.sh tv`, driven by
`adb shell input keyevent`. The Guide-edge section of `web/scripts/test-tv-emulator-journey.sh`
passed every checkpoint against the committed code. The full journey stops earlier, at its
pre-existing "initial Channel tune" assertion (the app also asks for the next channel's play URL),
which this change does not touch, so the Guide section was run on its own after pairing.

## What was observed

| Checkpoint | Screenshot |
| --- | --- |
| ◀ on the on-now programme: no page before now, focus stays | [guide-left-at-now.png](tv-guide-edges-2026-10-04/guide-left-at-now.png) |
| ▲ from the top row: focus on the "All · 2" filter (uiautomator `focused="true"`) | [guide-up-to-filters.png](tv-guide-edges-2026-10-04/guide-up-to-filters.png) |
| ▶ off "Star Trek", which spans the whole window: the window pages forward 4 h and focus stays on it | [guide-right-paged-forward.png](tv-guide-edges-2026-10-04/guide-right-paged-forward.png) |
| ◀ between the paged window's two programmes moves focus without paging; ◀ again pages back to the live window | [guide-left-back-to-now.png](tv-guide-edges-2026-10-04/guide-left-back-to-now.png) |
| Digit entry "77": the readout counts down ("auto-tunes in 0.4 s") at the default 1.2 s | [digit-entry-countdown.png](tv-guide-edges-2026-10-04/digit-entry-countdown.png) |

The fixture server recorded each requested Guide window. The journey asserts on that record, not only
on what the screen shows.

## Not verified

- **A non-default auto-tune duration on device.** Nothing writes the setting yet; the picker is a mock
  awaiting review ([tv-auto-tune-setting.html](../prototypes/tv-auto-tune-setting.html)). The stored
  duration reaching the reducer and countdown is unit-tested in `ui-tv/tests/auto-tune-setting.test.ts`.
- **TalkBack.** The invisible edge stops use `importantForAccessibility="no-hide-descendants"` but have
  not been checked with a screen reader.
- **A real Shield or remote.**

## Seen, not changed here

- In the digit-entry screenshot, the number readout overlaps the channel identity chip (from #1842's
  watching chrome).
- With focus on a filter, the Guide's selected programme keeps its amber ring, so two ambers show at
  once (the map's "focus is the only amber").
