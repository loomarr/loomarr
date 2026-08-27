# Shared client source-sharing audit

**Status:** shared native product rules are consolidated; P5 adoption blockers remain

**Audit head:** `android-react-native-migration`

**Companion plan:**
[`shared-client-platform.md`](plans/shared-client-platform.md)

## Decision

The Android mobile and TV applications share product rules through deep Loomarr modules and keep
touch, remote, safe-area, focus, and transport mechanics in adapters. No duplicated product rule was
found between the two React Native applications. The remaining duplicate product implementations
are the explicitly retained legacy web and Compose clients; they remain the rollback path until the
P5 adoption decision and cannot be retired yet.

This audit does not make P5 complete. Real-iPhone evidence, populated TV focus/time-shift soak, the
generated Guide Macrobenchmark's physical-Shield result, physical-device background/foreground
recovery, distribution upgrade, and the maintainer's recorded adoption decision remain open.

## Module and adapter ownership

| Concern | Shared module and interface | Adapter-owned implementation | Finding |
| --- | --- | --- | --- |
| Semantic presentation | `@loomarr/design-system` | Tamagui host implementation, platform insets, native/web font metrics | Shared. Product applications import Loomarr roles rather than Tamagui. |
| Pairing and authorization | `@loomarr/core/pairing`, `@loomarr/ui` `PairingShell` | SecureStore, browser storage, paired Bearer transport, cookie/CSRF transport | Shared product state and fail-closed recovery; storage and authentication mechanics vary at a real seam. |
| Guide rules | `@loomarr/core/guide`, `@loomarr/ui` `GuideJourney` | Browser keyboard/DOM, touch, and `@loomarr/ui-tv` focus/window adapters | Shared geometry, identity, selection, refresh, and tune intent. TV row windows and focus refs do not leak into product rules. |
| Watching and Surf | `@loomarr/player`, `@loomarr/ui` `WatchingSurface` and `SurfJourney` | Touch chrome, Compose-parity TV presentation, D-pad/number entry | Shared catalog, history, tune ordering, now/next, time-shift, and recovery. Presentation adapters differ because distance and input differ. |
| Playback transport | `@loomarr/player` controller interface | `@loomarr/player/native` Expo Video adapter; `@loomarr/player/browser` hls.js/native-HLS adapter | Both shipping transports sit behind public root entries with private implementations. The web Watch route supplies only its generated signed-URL, diagnostic, error-projection, and tune-timing ports. |
| Diagnostics | `@loomarr/core/client-diagnostics`, `@loomarr/player/native` lifecycle vocabulary | Web keepalive/CSRF sender and paired Android sender identities | Shared bounded queue and event projection. Server admission remains a closed source/platform pair and derives the actor. |
| Application composition | Shared roots above | Expo Router/safe area on touch; react-native-tvos/remote/overscan on TV | Legitimate adapter composition. Neither application defines Channel, Guide, playback, or pairing rules locally. |

## Duplication classification

### Shared product rules

- Pairing transitions, revocation, credential validation, and authenticated fetch behavior live in
  `core/pairing`.
- Guide shaping, household-time formatting, selection, latest-request-wins reads, and tune intent
  live in `core/guide` and `ui`.
- Playable catalog reconciliation, signed-URL replacement, previous/recent history, overlay state,
  live/paused/behind state, and expiry recovery live in `player`.
- Watching, Surf, Guide, Channel/programme identity, loading, empty, error, artwork, and action
  vocabulary live in `ui` and `design-system`.
- The closed diagnostic queue and playback lifecycle vocabulary are shared; applications supply
  only authenticated transport and an admitted runtime identity.

### Legitimate adapter duplication

`apps/mobile` and `apps/tv` each assemble a pairing session, authenticated transport, Guide
controller, player, and artwork renderer. That composition is deliberately local because the hosts
own different startup destinations, routing, Back behavior, safe-area/overscan policy, diagnostics
identity, and focus mechanics. These files do not reimplement the rules supplied by those modules.

### Deliberate legacy duplication

- The Compose TV application remains releasable under the permanent Play identity until React
  Native passes P5 and in-place update acceptance.
- The Tailwind/shadcn web viewer remains the browser rollback path after adopting the shared browser
  player adapter; its remaining presentation migration and P5/P8 adoption gates still prevent
  retirement.

These are named migration states, not accepted end-state duplication. P8 deletes each legacy
consumer and records its retired identifiers after the replacement passes its full contract.

## Deletion tests

- Replacing Tamagui changes `packages/design-system` and narrow host adapters; applications and
  product modules do not import `@tamagui/core`.
- Replacing Expo Video changes the private native player adapter; mobile/TV applications continue
  consuming `@loomarr/player/native`.
- Replacing TV focus mechanics changes `ui-tv` and TV host bindings; Guide and Surf selection rules
  remain unchanged.
- Removing either legacy client before adoption would remove rollback, so those deletion tests are
  intentionally deferred to P8.

## Verification

The following evidence is required on the publication head:

```sh
cd web
pnpm lint:boundaries
pnpm --filter @loomarr/player test
pnpm --filter @loomarr/player typecheck
pnpm --filter @loomarr/web exec vitest run src/channels/use-hls-player/use-hls-player.test.ts
pnpm --filter @loomarr/web build
pnpm --filter @loomarr/mobile typecheck
pnpm --filter @loomarr/tv typecheck
```

The import-boundary gate rejects application or sibling-package deep imports, dependency cycles,
and product imports of Tamagui. Tests cross the same package entry points as callers.
