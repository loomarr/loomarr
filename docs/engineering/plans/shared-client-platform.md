# Shared client platform migration

**Status:** P0a merged; P0b scaffold in progress
**Date:** 2026-08-23  
**Decision owner:** maintainer  
**Companion contract:** [`docs/frontend-design.md`](../../frontend-design.md)
**Current-state inventory:** [`docs/engineering/client-platform-inventory.md`](../client-platform-inventory.md)

## Outcome

Loomarr will replace the current Test Card/Tailwind/shadcn web design system and the separately
implemented Compose TV presentation with a Loomarr-owned client platform built on **Tamagui Core**.
The target serves the embedded web app, iOS and Android touch clients, and Android TV and Apple TV
clients without pretending that pointer, touch, and remote control are the same interaction.

This is not yet authority to rewrite every screen. The first delivery is a production-quality
Guide-to-Playback slice rendered from shared modules on the browser, an iPhone, and the physical
Nvidia Shield. A recorded go/no-go review authorizes or rejects the full migration. Until that
review passes, the existing web and Compose clients remain releasable and the new platform may not
remove their code, tests, distribution, or rollback path.

## Why the current approach is being replaced

The present contract deliberately shares tokens and logic but forbids shared component
implementations. That decision produced three costs that now outweigh its benefits:

1. web and TV encode the same visual rules independently, so the visual language drifts;
2. the token layer describes implementation colors and Tailwind mechanics more often than product
   meaning; and
3. a new mobile client would add a third implementation before the product has one design system
   worth reproducing.

The current palette, primitives, and screenshots are migration evidence, not presumed-good target
design. Existing behavior, accessibility, authorization, pairing, playout, and release guarantees
remain requirements even when their presentation is replaced.

## Non-negotiable product invariants

- `docs/design.md` continues to own behavior. A client may expose an existing capability differently
  but may not weaken roles, approvals, grounding, CSRF/session behavior, paired-device revocation,
  or fail-closed authorization.
- Wire types and query functions continue to come from committed OpenAPI and the generated
  `@loomarr/api` package. Shared UI does not invent DTOs.
- Web remains an offline-capable static build embedded in the Go binary. Native clients connect to
  the operator-configured public Loomarr URL and never bundle a household admin token.
- The physical Shield remains an acceptance device. An emulator or screenshot is useful evidence,
  not a substitute for D-pad, playback, restart, and update testing on hardware.
- TV navigation must work with five-way D-pad, OK, Back, and number keys. Menu may be a shortcut but
  never the only path to a function. Back ultimately returns to the platform home screen.
- Layout is authored in logical units. The same composition must fill 1920x1080 and 3840x2160
  output without hard-coded pixel coordinates, clipping, or a smaller centered canvas.
- Every migration PR leaves a shippable product and has a documented rollback. The Compose package
  `loomarr.media` is retired only after the replacement proves install and in-place update parity.

## Target modules and seams

The existing `web/` pnpm workspace remains the workspace root during the proof so the Go embed and
current frontend harness do not move at the same time as the UI architecture. Renaming it is a
separate post-adoption decision.

```text
web/
  apps/
    web/                 # Vite delivery adapter; current router may remain
    mobile/              # Expo Router; iOS and Android touch targets
    tv/                  # Expo + react-native-tvos; Android TV and Apple TV targets
  packages/
    api/                 # generated wire models and query functions
    core/                # platform-neutral domain logic and validation
    fixtures/            # deterministic domain scenarios
    design-system/       # Tamagui config, semantic tokens, fonts, icons, primitives
    ui/                  # shared viewer and product modules
    ui-tv/               # focus, remote, overscan, guide-virtualization adapters
    player/              # shared playback state interface plus platform adapters
```

Each package remains a deep module under [`web/packages/README.md`](../../../web/packages/README.md):
callers use root entry points, implementation stays private, tests exercise the same interface, and
the dependency graph remains acyclic.

### What is shared

| Shared implementation | Adapter-owned implementation |
| --- | --- |
| semantic tokens, themes, type roles, spacing, radii, motion contracts | font loading and platform font metrics |
| artwork, channel identity, metadata, badges, actions, loading/error states | web DOM semantics where React Native semantics are insufficient |
| Guide data shaping, time-axis math, selection intent, now/next state | router history, deep links, portals, safe areas, overscan |
| player state, overlay state machine, tune intent, previous-channel history | hls.js on web; native player transport on iOS/Android/TV |
| component states, deterministic fixtures, story contracts | pointer/keyboard, touch/gesture, and remote/focus behavior |

One source file is not a success when it is littered with platform branches. Shared modules expose a
small product interface; platform adapters satisfy the seams that genuinely vary. A second adapter
justifies a seam. Hypothetical abstractions do not.

## Tamagui decision

Use **`@tamagui/core`**, not the predesigned Tamagui UI kit, as the candidate styling and primitive
implementation. Loomarr owns its semantic interface and visual language. Application code imports
Loomarr modules rather than Tamagui directly; only `packages/design-system` and narrowly documented
adapter code may import Tamagui.

The optimizing compiler is **not** enabled in the scaffold PR. All features work at runtime, and
adding the compiler before representative code exists would add build complexity without measurable
evidence. Benchmark the uncompiled slice first, then test the compiler as an isolated optimization;
retain it only when it improves the production artifacts without changing behavior or source files.

NativeWind, Gluestack, Tailwind, shadcn, Base UI, and Compose are not co-foundations. Tailwind/shadcn
and Compose remain legacy implementations during migration; they receive fixes needed to keep the
shipping clients healthy but no new shared design authority. They are removed only after adoption
and parity.

## New visual direction

The replacement has no retro-theme name. It is simply Loomarr's product language:

- **Content first.** Programme artwork, channel identity, title, episode information, and time are
  visually primary; application chrome recedes.
- **Watching first on TV.** Playback is the home state. Guide and Surf are transient, edge-to-edge
  layers over a still-mounted player and dismiss without losing the tuned channel.
- **Calm, dimensional dark surfaces.** Black is reserved for playback and transparent overlays;
  ordinary surfaces use distinguishable semantic layers rather than one undifferentiated black.
- **Focus is a first-class state.** Focus is not hover with a larger scale. It has a visible ring or
  surface treatment, predictable movement, restored position, and no layout jump.
- **Artwork has a contract.** Programmes use 16:9 stills/backdrops with complete-image treatment;
  placeholder, missing, loading, and error states preserve geometry.
- **Density follows distance.** The information hierarchy is shared, while type sizes, hit targets,
  gutters, and disclosure differ for desktop, touch, and ten-foot viewing.
- **Motion explains state.** Overlay entrance/exit, focus movement, and tuning transitions may
  animate; decoration does not compete with playback, and reduced-motion is honored everywhere.

### Semantic token interface

The new token vocabulary names intent, not a CSS technique or a historical palette. The first slice
must cover at least:

```text
color.surface.{canvas,raised,overlay,focus}
color.content.{primary,secondary,muted,inverse}
color.state.{live,success,warning,danger,info}
color.action.{primary,secondary,focus,disabled}
space.{screen,section,control,inline}
type.{display,title,body,label,metadata,time,channelNumber}
radius.{control,card,overlay,round}
motion.{instant,focus,overlay,tune}
size.target.{pointer,touch,tv}
```

Platform scales map these roles to concrete values. Raw palette values remain private
implementation. The token generator continues to publish machine-readable artifacts while legacy
consumers exist; Tamagui configuration becomes the target source and generated CSS, JSON, and Kotlin
are adapters rather than competing sources.

## First vertical slice

The slice is one continuous user journey, not a gallery-only proof:

1. pair or authenticate without broadening authority;
2. enter the edge-to-edge Guide and load real guide data and artwork;
3. move through channels and airings with platform-correct navigation;
4. inspect the focused airing's title, series/season/episode, description, time, and artwork;
5. tune the focused channel and reach first frame;
6. reveal and dismiss the player overlay;
7. open Surf, browse channels, tune another channel, and return to the previous channel;
8. leave playback using platform-correct Back behavior.

The slice uses real generated client contracts and a configured Loomarr server. Storybook/MSW
fixtures cover deterministic states, but a mock-only demo does not pass.

## Acceptance evidence and adoption gate

### Visual and interaction

- Maintainer-approved captures for desktop web, mobile web, iPhone, 1920x1080 TV, and 3840x2160 TV.
- Guide and player surfaces fill the viewport. Safe-area/overscan padding belongs inside the
  composition and never creates an outer frame.
- TV focus survives a ten-minute traversal across rows, airings, Guide, Surf, and player controls;
  every focusable control is reachable and focus returns to the initiating item.
- The 100-channel by four-hour Guide virtualizes without blank rows, time-axis drift, or focus loss.
- Browser keyboard and screen-reader behavior remain valid; touch targets and safe areas pass on an
  actual iPhone.

### Correctness and safety

- Member/admin authorization negatives remain green, disabling a user or paired device kills the
  next authenticated request, and no native client stores the break-glass API token.
- Pairing, signed-play-URL refresh, URL redaction, and session expiry are exercised end to end.
- Programme identity in Guide, Surf, overlay, and playback agrees with the authoritative guide and
  now/next responses; the client never substitutes fixture or cached metadata for live identity.
- Back, OK, D-pad, number-key, and tune behavior match the TV contract; no function depends solely on
  a Menu key.

### Performance and build health

- Existing web budgets remain: no JavaScript chunk above 500 KiB and no entry plus module preloads
  above 1 MiB uncompressed.
- A Shield Macrobenchmark of repeated Guide navigation records p95 frame duration no greater than
  32 ms, p99 no greater than 50 ms, and zero frozen frames (700 ms or longer).
- Prepared-channel first frame stays within the existing playout tune budget and repeated surfing
  does not start encoders for prepared hits.
- The production slice is measured both with and without the Tamagui compiler. The compiler is
  adopted only if it improves bundle or render evidence and leaves all gates green.
- Local and CI tasks are affected-aware: native jobs do not run for an unrelated Go-only edit, and
  web-only story changes do not build both native applications unless a shared input changed.

### Maintainability and reuse

- `design-system`, `ui`, and `player` present documented root interfaces with no consumer deep
  imports and no dependency cycles.
- The same production source implements the shared primitives and Guide detail content on all three
  targets; platform adapters contain navigation, focus, and player transport differences.
- A source-sharing report identifies shared, adapter-specific, and duplicated code. Duplicated
  product rules block adoption; duplicated platform mechanics do not.
- Removing Tamagui would require replacing the design-system implementation, not editing every
  screen. This deletion test is proved by import-graph enforcement.

The maintainer records **adopt**, **revise and repeat**, or **reject** against every item above. Only
`adopt` authorizes the remaining migration and retirement phases.

## Delivery sequence

One row is one PR-sized phase. Later phases may be refined after evidence, but they may not collapse
the adoption gate.

| Phase | Deliverable | Required proof |
| --- | --- | --- |
| P0a | This contract and dependency decision | docs and generated-doc gates |
| P0b | pnpm/Turborepo/Expo/Tamagui scaffold; no migrated production screen | web build unchanged; Expo iOS/Android/TV dev builds; affected-task tests |
| P1 | semantic tokens, fonts, primitive interfaces, fixtures, web/native Storybooks | token drift, contrast, story coverage, web/native renders |
| P2 | shared Guide data/view modules and web integration | real API + deterministic visual/a11y gates; current web behavior retained |
| P3 | mobile/TV shells, pairing, transport, and navigation adapters | iPhone and Shield login/pair/revocation evidence |
| P4 | playback, overlay, Surf, tuning, and previous-channel behavior | real-server first frame and remote/touch/browser traversal |
| P5 | full vertical-slice evidence and go/no-go decision | every acceptance item above recorded |
| P6 | remaining viewer surfaces | route-by-route parity; current clients still releasable |
| P7 | administrative web surfaces | authorization, forms, accessibility, responsive and visual parity |
| P8 | retire Tailwind/shadcn and Compose presentation; finish distribution | clean retired identifiers, Play/iOS beta builds, in-place update, complete gates |

## Rollback and retirement

Before P5 adoption, reverting a phase removes only the new workspace applications and shared
packages. The Go embed, current web assets, Compose app, Play identity, and paired-device records
remain untouched.

After adoption, migration uses route/surface ownership rather than two implementations mounted for
the same user journey. A surface switches only when its replacement passes its full contract. The
old implementation is then deleted in the same PR or the next explicitly paired retirement PR; it
does not linger as an unowned fallback. Retiring framework identifiers adds them to
`scripts/check-retired.sh` as required by `AGENTS.md`.

## P0b scaffold evidence

P0b uses separate `mobile` and `tv` Expo applications because navigation and release identity are
real platform seams. Mobile owns Expo Router; TV owns a minimal root registration because the TV
config plugin does not support Expo Router. Both applications resolve the exact same
`react-native-tvos` version, which supports phone and TV targets, and both render the same
`ClientPlatformProof` source through the Loomarr-owned `design-system` interface.

The scaffold uses prototype-only bundle and package identifiers. It cannot overwrite the shipping
Compose application or claim the permanent mobile identity before P5 adoption. Runtime Tamagui is
proven through Android touch, iOS, Android TV, and Apple TV production JS bundles and a dedicated
Vite browser entry. After web-adapter React deduplication, the browser proof produces one 277.41 kB JavaScript
chunk (92.94 kB gzip) without mounting or changing a shipping route. No compiler or production
screen has been introduced.

Turborepo runs beneath `make clients`. Expo, Metro, and Turbo outputs are excluded from task inputs,
so a warmed unchanged graph restores all bundle tasks instead of rebuilding itself because of its
own logs. CI has a dedicated client gate: native-only source changes select clients without
selecting the legacy frontend, Playwright, tuner, or production-image families; workspace-root
dependency and tool changes fail wider because both graphs consume them.

Native Android proof builds are arm64-only and serialized. `make client-android-debug` defaults to
the mobile app; `CLIENT_APP=tv` selects TV. On Linux the command places the whole Gradle, Kotlin,
CMake, and Ninja process tree under a 3.75 GiB soft limit and 4 GiB hard limit, pins that tree to four
CPUs, uses one Gradle worker, keeps Kotlin compilation in-process, and injects one-slot CMake compile
and link pools through an Expo config plugin. Third-party native modules can own separate Ninja
graphs, so CPU affinity and the process-tree limit remain the fail-safe boundaries. Mobile and TV
native builds are never run concurrently.

The clean native proof is green for both generated Android targets: mobile produced a 76 MiB
`media.loomarr.mobile.prototype` APK in 5m35s and TV produced a 57 MiB
`media.loomarr.tv.prototype` APK in 2m15s. Both contain only `arm64-v8a`; the TV manifest is a
required Leanback application, marks touchscreen and faketouch optional, and exposes a Leanback
launcher activity. The proof also caught an optional-peer mismatch that Expo Doctor and production
JS bundles did not: Expo SDK 57 supports Reanimated 4.5.1 with Worklets 0.10.1, while pnpm had
auto-selected incompatible 4.6.0 and 0.12.1 releases. Both app manifests now pin Expo's supported
pair directly.

The Linux proof also generates both native Apple projects cleanly: the mobile project targets
iPhone and iPad (`TARGETED_DEVICE_FAMILY = "1,2"`, `SDKROOT = iphoneos`) while the TV project targets
Apple TV (`TARGETED_DEVICE_FAMILY = 3`, `SDKROOT = appletvos`). Xcode compilation and launch still
require macOS and remain explicit P0b acceptance evidence; a successful Metro bundle or Linux
prebuild is not recorded as a native Apple build. The `client-apple-simulator` target is the shared
local/CI verifier: its mobile and TV matrix legs generate the native project, install pods, make a
Release simulator build, boot the matching iOS or tvOS runtime, install and launch the application,
assert that its process remains alive, and retain a screenshot. A bundle-only result cannot make
P0b ready.

The browser proof is also rendered, not bundle-only. At a 1440x900 viewport, the shared screen fills
the viewport and the 760x126 proof panel is centered at x=340, y=387 with no horizontal or vertical
overflow and no page exceptions. A server-render test runs through the same adapter aliases so a
second React runtime in a linked universal package fails the client gate instead of producing a
blank-but-successfully-bundled page.

## Open evidence, not open architecture

The architecture above is decided for the slice. These facts must be measured rather than guessed:

- whether Tamagui's runtime-only path meets the web and Shield budgets;
- whether the compiler improves the representative slice enough to justify its build seam; and
- which current tokens or assets deserve migration after side-by-side visual review.

Those measurements can change an adapter or reject Tamagui. They do not weaken the Loomarr-owned
interfaces or bypass the adoption gate.
