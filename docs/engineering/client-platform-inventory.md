# Shared client platform inventory

**Date:** 2026-08-23  
**Status:** Phase-0 baseline  
**Plan:** [`plans/shared-client-platform.md`](plans/shared-client-platform.md)

This inventory compares the supplied local design artifact with the current web and Android TV
implementations. It decides what the migration should keep, adapt, redesign, or retire; it does not
claim that the mock or current UI is already the finished visual language.

## Evidence inspected

- `/home/fictional/Downloads/Shared file archive(4).zip`, especially
  `Loomarr TV + iOS.dc.html`, rendered locally rather than inferred from the unavailable hosted
  artifact.
- Current Android TV sources under `android/app/src/main/java/loomarr/media/{design,guide,navigation,pairing,playback}`.
- Committed Android Roborazzi references for Watching, Surf, Guide, pairing, and their 4K-density
  variants under `android/app/src/test/screenshots/`.
- Current web Guide, Watch/player, artwork, channel-identity, token, Storybook, visual, and generated
  client modules.
- Current product behavior in `docs/design.md`, especially auth, Guide, Watch, player, and TV
  navigation contracts.

The current client surface is substantial: 213 production web TSX files, including 86 Loomarr
product modules and 25 UI primitives; 114 web stories and 159 web component tests; and 26 Android
production Kotlin files with 17 Kotlin test files. This is a staged migration of proven behavior,
not a greenfield demo.

## What the supplied artifact actually contains

| Platform | Rendered surfaces | Useful direction | Missing contract |
| --- | --- | --- | --- |
| Android TV / Shield | Watching chrome, grouped Surf rail, 54-row Guide | watching-first shell, lightweight bottom chrome, direct number tune, grouped Surf, focus-tracking Guide detail | pairing, failure/loading, artwork behavior, long metadata, actual focus restoration, playback failures, 4K proof |
| Apple TV | Surf overlay | same information model with a lift/brightness focus treatment | complete Guide, Watching controls, tvOS Back/Menu equivalents, playback integration |
| iPhone | Watch/channel list and approval queue | content-forward navigation, search, watch-anywhere, approve-on-the-go | Guide detail, player controls, setup/auth, safe areas, accessibility, error states |
| iPad | landscape rail + player + tonight schedule | useful wide-touch composition and persistent channel rail | portrait, multitasking widths, interaction and player-state behavior |

The mock labels are annotations, not application copy. Phrases such as “row 1 of 1,” “OK tune,” or
“Menu filters” do not enter the UI merely because a composite uses them to explain interaction.
Controls must communicate their purpose through the platform interaction and accessible semantics.

## Current implementation assessment

### 2026-08-26 Compose-to-React-Native presentation checkpoint

A rendered comparison against the shipping Compose references found that the current React Native
TV application does **not** preserve the shipping presentation. This is not a small token or spacing
drift: the visible information architecture and composition differ on every primary viewer surface.
The behavioral seams described below remain useful, but they are not evidence of visual parity.

| Surface | Shipping Compose reference | Current React Native presentation | Checkpoint result |
| --- | --- | --- | --- |
| Watching | Full-screen video with a compact transient Channel pill, direct number-entry card, and a two-tier bottom now/next/progress bar; remote hints are quiet text rather than controls | A large titled playback-controls panel containing Channel identity, state copy, and a row of Previous, Channel, Guide, Surf, pause, Go Live, and Retry actions | behavior carried; presentation replaced |
| Surf | A left-side translucent overlay over the still-visible player; grouped Channel rows expand the focused now/progress detail and Back cancels | A full application shell with brand/server controls, a large focused programme card, and persistent Watching/Guide/Surf navigation | mounted-player rule carried; overlay composition replaced |
| Guide | A dense edge-to-edge table with fixed Channel/time axes, compact filter strip, six visible rows, and a bottom focus-tracking detail band | A rounded shared surface with large filter actions, programme cards, and persistent destination navigation | selection/tune rules carried; table composition replaced |
| Navigation | Watching owns direct D-pad/number actions; Guide and Surf are temporary destinations; Back returns to Watching and then Android owns exit | The remote adapter retains those transitions, while shared destination buttons are also rendered as prominent application chrome | behavior mostly carried; navigation presentation replaced |
| Loading, empty, and failure | Surface-specific centered copy over the playback or Guide canvas | Shared `StatePanel` and playback-control compositions with explicit recovery actions | state meaning carried; presentation replaced |

The comparison uses the committed Roborazzi references under `android/app/src/test/screenshots/`, the
shipping sources in `android/app/src/main/java/loomarr/media/{guide,navigation,playback}`, and the
production React Native sources in `web/apps/tv/src` and `web/packages/ui`. Device captures remain
acceptance evidence, but the committed renderer references make this mismatch reproducible without
requiring the same television or emulator session.

The maintainer selected screen-for-screen Compose fidelity first on 2026-08-26. The committed
Roborazzi references are therefore the Android TV presentation baseline for Watching, Surf, Guide,
pairing, loading, empty, and failure states. A later redesign remains possible only after the React
Native shipping path passes those parity gates; it is not part of this migration checkpoint.

### Behavioral parity checkpoint

Presentation choice does not reopen the contracts that already work. The migration keeps the
following ledger so a later visual correction cannot silently replace product or platform behavior.

| Contract | Compose evidence | React Native evidence | Status / remaining proof |
| --- | --- | --- | --- |
| Pairing, least-authority credential storage, confirmed disconnect, and remote revocation | `pairing/PairingClientTest.kt`, `pairing/DeviceStore.kt`, and the shipping paired shell | `core/src/pairing/pairing.test.ts`, `PairingShell`, paired transport tests, shared `DeviceDisconnectAction`, populated/unavailable TV Surf reachability tests, and physical-Shield pair/revoke/re-pair traversal | preserved; the Compose-parity Surf action still needs emulator/Shield D-pad confirmation and real-iPhone traversal remains |
| Watching Up/Channel Up, Down/Channel Down, OK Guide, Left/Menu Surf, and Back delegated to Android | `navigation/TvNavigation.kt` and `TvNavigationTest.kt` | `ui-tv/src/watching-remote` plus `watching-remote.test.ts`; the TV host returns `false` for Back from Watching | preserved; final physical Back exit remains |
| Exact number tune with a three-digit buffer and 1.2-second commit | `WatchScreen.kt` and `TvNavigationTest.kt` | `ui-tv/src/number-entry` and `number-entry.test.ts` | preserved by interface tests; physical number-key pass remains |
| Play, pause, resume-behind, and Go Live keep one player and one shared time-shift state | Compose did not expose the shared DVR contract | `player` live-state tests, native transport tests, Watching state tests, and the `ui-tv` Play/Pause remote adapter | implemented; real-device pause/resume/expiry traversal remains |
| One player remains mounted while Guide or Surf is visible | `TvHomeState` keeps Surf over Watching | `WatchingSurface` stays mounted and suppresses only its chrome; `apps/tv/tests/config.test.mjs` and `ui/tests/watching-surface.test.tsx` guard the seam | preserved; Surf must regain the Compose left-overlay composition |
| Authoritative Channel order, exact tune, previous Channel, and bounded recent history | `ChannelCatalogTest.kt` and `TuneHistory` | `player-controller.test.ts`, `surf-data.test.ts`, and `surf-navigation.test.ts` | preserved and extended to latest-request-wins reconciliation |
| Guide selection clamps to real rows/airings, retains a time anchor vertically, skips disabled filters, and tunes one explicit selection | `GuideCursor`, `GuideFocus`, `TimelineTest.kt`, and `GuideClientTest.kt` | `core/src/guide/*test.ts`, `ui-tv/tests/guide-navigation.test.ts`, `focus-registry.test.ts`, and `ui/tests/guide.test.tsx` | interface parity proven; populated remote-repeat and focus soak remain |
| Surf keeps Favorites, Recent, and All in stable order without inventing unavailable favorites | `channelSections` and `TvNavigationTest.kt` | `surf-data.test.ts`, `surf-rail.test.tsx`, and `surf-navigation.test.ts` | preserved; populated artwork/focus traversal remains |
| Loading, empty/dead-air, tuning, recoverable failure, and retry remain distinct | `WatchUiState`, `GuideUiState`, and committed screenshots | `state-panel.test.tsx`, `watching-surface.test.tsx`, Guide/Surf journeys, and deterministic native stories | state meaning proven; adopted presentation and device traversal remain |
| Device bearer never enters HLS or an external artwork origin; 401/403 revokes locally | shipping `PlaybackClient`, `GuideClient`, and `RemoteArtworkTest.kt` | `play-url-source.test.ts`, `native-transport.test.ts`, and `native-event-stream.test.ts` | preserved and fail-closed; final mobile admission remains |
| Backgrounding stops playback and releases the native player; foregrounding re-reads authority before retuning | Compose activity/player lifecycle | `native-player-lifecycle.test.ts`, `native-transport.test.ts`, and emulator HOME/foreground evidence | emulator proven; physical-Shield background/foreground remains |
| Android identity, signing, versioning, in-place update, and rollback survive the renderer migration | shipping `loomarr.media` application and Android beta contract | prototype identity remains separate; Compose is retained as rollback through P5; the generated release-like Shield Macrobenchmark harness enforces the P5 Guide p95/p99/frozen-frame limits | physical Shield benchmark report, migration build identity, signed AAB, in-place update, and rollback drill remain |

“Preserved” in this table means the named automated evidence covers the interface contract. It does
not upgrade missing hardware, distribution, visual, or adoption evidence into completion.

### Keep as product truth

| Existing asset | Why it survives | Target home |
| --- | --- | --- |
| `api/openapi.yaml` and generated `@loomarr/api` modules | authoritative names, types, query functions, and compile-time drift detection | `packages/api` |
| `@loomarr/core` validation, SSE invalidation, formatters, and domain contracts | already platform-neutral; prevents client-specific wire/domain inventions | `packages/core` |
| deterministic fixtures and story scenarios | one reproducible state vocabulary for web/native stories and tests | `packages/fixtures` |
| pairing and revocable device-token behavior | correct least-authority native auth; cannot regress to an admin token | native transport adapter + shared auth state |
| signed play-URL and prepared-playback contract | the real playback seam shared by browser and native clients | `packages/player` interface + transport adapters |
| programme identity and guide endpoints | server remains authoritative for title, episode, time, artwork, and channel | shared Guide data module |
| TV navigation behavior in `TvNavigation.kt` | exact number matching, bounded recent history, grouped channels, explicit focus graph | shared pure TypeScript behavior with TV adapter tests |
| web `Image` behavior and Android `RemoteArtwork` behavior | both encode valuable placeholder, geometry, failure, and remote-loading requirements | shared Artwork interface + web/native adapters |
| release identity `loomarr.media`, Play signing, versioning, and Shield update gates | distribution is independent from presentation technology | adapted Android/Expo release pipeline |

“Keep” means keep the behavior or interface. Kotlin or Tailwind implementation does not survive
automatically.

### Adapt behind explicit seams

| Existing implementation | Adaptation |
| --- | --- |
| TanStack Router and browser cookie/CSRF transport | remain web adapters; Expo Router and paired bearer transport satisfy separate interfaces |
| `useHlsPlayer` and the web `VideoPlayer` state | split transport from player/overlay state; retain hls.js only in the web transport adapter |
| Android `PlaybackClient`, `GuideClient`, `PairingClient`, and `DeviceStore` | port the contracts and negative tests to generated TypeScript/native adapters; do not copy private DTOs |
| Android Guide cursor and focus graph | move pure selection math into `ui-tv`; bind it to `react-native-tvos` focus primitives in the TV app |
| Android channel catalog SSE/refetch behavior | express as shared query/invalidation behavior and retain GET as authoritative after reconnect |
| web and Android story/screenshot fixtures | consolidate the domain scenario; keep separate renderer baselines at platform dimensions |
| existing launcher icon, TV banner, and store metadata | preserve identity requirements, then regenerate presentation assets from the adopted visual language |

### Redesign only after Android TV parity

| Current area | Migration direction | Later option after parity |
| --- | --- | --- |
| palette and token names | Keep semantic shared roles internally, but the TV adapter must resolve to the committed Compose colors, contrast, opacity, and focus treatment | Evolve semantic values after a separately approved visual change |
| typography scale | Preserve the Compose ten-foot hierarchy and truncation at both committed 1080p and 4K densities | Revisit platform scales with new reviewed TV captures |
| Watching chrome | Match the compact top-right Channel pill, top-left number entry, two-tier bottom now/next/progress bar, quiet remote hints, and five-second fade | Explore another chrome composition after parity ships |
| Surf | Match the left-side translucent rail over the still-visible player, stable group order, focused-row expansion, progress, count, and Back cancel | Rebalance rail width or artwork only with new acceptance references |
| Guide | Match the dense edge-to-edge table, fixed Channel/time axes, compact filter strip, visible row density, focus treatment, and bottom detail band | Introduce a different Guide hierarchy only as a later redesign |
| channel identity | Keep one shared identity interface while the TV adapter preserves Compose number/name/live geometry; touch may use its own density adapter | Unify more presentation after both adapters are proven |
| administrative web chrome | Not governed by the Android TV parity baseline | Continue the planned quieter web shell independently |
| empty/loading/error treatment | Preserve state meaning through shared interfaces and match the corresponding Compose TV canvas composition | Refine shared state presentation after TV parity |

### Retire only after parity

| Legacy asset | Retirement condition |
| --- | --- |
| Tailwind theme/preset, shadcn copies, CVA styling, and Base UI presentation | final web consumer migrated with keyboard, screen-reader, find-in-page, forms, menus, slider, tooltip, and visual gates green |
| Test Card palette/name as the design authority | semantic Tamagui token gallery approved and every legacy adapter generated from it or deleted |
| Compose `design/`, `guide/`, and Watching/Surf presentation | Expo TV build passes physical Shield parity, Play install, process/device restart, and an in-place update |
| Kotlin client/network state that duplicates shared TypeScript | equivalent generated transport and shared state tests are green on real server behavior |
| Roborazzi presentation baselines | replacement native visual and device gates are committed and capable of failing on a real regression |
| legacy Android token generator | no Kotlin presentation consumer remains |

Release scripts, package identity, signing credentials, and historical acceptance evidence are
adapted, not deleted with Compose.

## First-slice module map

| Product rule | Shared module | Web adapter | Mobile adapter | TV adapter |
| --- | --- | --- | --- | --- |
| channel identity and artwork states | `design-system` / `ui` | DOM image, responsive sources, browser alt semantics | native image loading and safe area | native image loading and distance scale |
| guide window, airing width, focused detail | `ui` | pointer/keyboard + virtual rows | touch list/detail | D-pad focus graph + virtual rows/time axis |
| tune selection and previous-channel history | `player` | browser controller/hls.js | native controller/player | remote keys, number buffer, native player |
| overlay visibility and fade | `player` | pointer/keyboard activity | touch activity | remote activity and focus ownership |
| Surf grouping and selection | `ui` | responsive overlay | sheet/rail | overscan-safe transparent rail |
| authentication state | `core` interface | cookie + CSRF | paired bearer | paired bearer + TV code flow |

## Gaps the slice must expose honestly

- The mocks show favorite groups, but the current backend has no durable per-user favorites
  contract. The client may show the group only when authoritative favorite data exists; it may not
  fabricate persistence.
- The supplied TV Guide composite does not visibly demonstrate rich artwork even though the product
  and current Android detail surface support it. The replacement must include actual episode stills
  or backdrops and explicit missing/loading/error geometry.
- The iPhone and iPad composites are not a complete mobile application specification. Setup,
  authentication, error recovery, accessibility, portrait/landscape, and safe-area behavior come
  from the product contract and platform evidence.
- The current Compose player uses Media3 directly. The React Native adapter is now selected as Expo
  SDK 57's `expo-video`, private to `@loomarr/player/native`: it supplies Media3/AVPlayer source and
  view mechanics while Loomarr retains signed-URL, latest-request-wins, live-edge, overlay, history,
  diagnostics, and lifecycle policy. Background playback and Picture in Picture stay disabled, and
  pause/release plus real-device background traversal are mandatory because SDK 57 has a reported
  audio-after-unmount regression.
- Tamagui's compiler and TV behavior are evidence questions. Runtime-only Tamagui is the baseline;
  compiler adoption and full migration wait for the recorded P5 gate.

## Inventory conclusion

The reusable asset includes both the server/authorization/navigation contracts and the committed
Android TV presentation. React Native may replace the implementation, but its TV adapter must first
preserve the shipping composition while concentrating product rules behind shared Loomarr
interfaces. Touch and web adapters may use their approved density-specific compositions. The Guide-to-Playback slice is
large enough to test every risky seam—data, artwork, focus, navigation, playback, overlay state,
responsive layout, and actual hardware—before the project commits to migrating 213 web view files
and the shipping TV client.
