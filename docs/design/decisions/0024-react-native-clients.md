# 0024. React Native and Expo for client binaries

- **Date:** 2026-08-23 (migration authorized 2026-09-03, #970)
- **Status:** accepted
- **Supersedes:** the separate Kotlin/Compose Android TV client and per-platform component copies

## Context

Loomarr ships a web client, an Android TV client and phone clients. Each platform had its own
presentation code, and the same components were implemented more than once. Every product change
paid that cost per platform, and the copies drifted.

## Decision

React Native and Expo are approved runtimes for Loomarr's end-user client binaries, with
`react-native-tvos` for TVs and one shared, Loomarr-owned design system (Tamagui core behind
`design-system` and `ui`). They are client runtimes only: the Go server keeps the scheduler,
authorization, playout and all domain logic. The migration and legacy retirement are tracked in #970
under [`frontend-design.md`](../../frontend-design.md).

## Consequences

- One component and token implementation serves web, phones and TVs; platform seams remain for
  navigation, focus, overscan and the video player.
- The Android TV client keeps one permanent application identity across the replacement.
- Distribution covered by this decision is a signed sideload and Google Play Internal testing;
  public Play distribution and installed-credential migration are outside it.
