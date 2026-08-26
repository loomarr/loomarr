# Shared client design-system completeness ledger

**Status:** implementation and automated/rendered gates complete; P3.5 device evidence in progress
**Owner:** shared client platform  
**Parent plan:**
[`docs/engineering/plans/shared-client-platform.md`](plans/shared-client-platform.md)
**Shipping-surface inventory:**
[`docs/engineering/client-platform-inventory.md`](client-platform-inventory.md)

## Purpose

This ledger makes “the design system and building blocks are complete” a reviewable claim. It covers
the known embedded-web, mobile, and TV clients, including the viewer surfaces planned for P4/P6 and
the foundational controls required by the administrative web migration in P7. It does not require
inventing modules for hypothetical future features. A new product capability amends this ledger when
it introduces a genuinely new interaction or presentation contract.

Storybook is the executable workshop for deterministic states. It is necessary evidence, not a
substitute for rendering the production journey on browsers and real hardware.

The web workshop's official Themes toolbar drives the same `LoomarrProvider` used by clients. Dark
is the default; light and system-selected light/dark are interactive global modes, while a story may
pin a mode only when the mode itself is the state under review.

The official Themes addon declares React Native unsupported, so the on-device workshop consumes the
same `theme` story global through its native decorator instead of pretending the web manager addon
runs there. Dark is again the default, and every shared native story module publishes an explicit
light state; a source gate prevents a later module from silently becoming dark-only.

## Completion rules

P3.5 is complete only when all of these rules hold:

1. Every row below is **proven** or has a maintainer-approved exclusion with a named later owner.
2. Product rules are implemented once behind root interfaces in `@loomarr/design-system`,
   `@loomarr/ui`, `@loomarr/core`, or `@loomarr/player`; callers do not deep-import implementations.
3. Platform mechanics live in adapters only where at least two implementations prove the seam:
   browser semantics/keyboard, touch/safe-area/gesture, TV focus/remote/overscan/virtualization, and
   player transport.
4. Applications may compose shared modules but may not establish competing tokens, icon families,
   typography, focus treatments, loading vocabulary, or product-state rules.
5. Every supported theme, density, content state, interaction state, and motion mode is rendered in
   the web workshop. Shared native stories render the same interface on touch and TV hosts.
6. Tests cross the same root interface as callers. Visual, interaction, accessibility, asset drift,
   contrast, reduced-motion, import-boundary, and public-interface checks fail red when the contract
   regresses.
7. The deletion test holds: replacing Tamagui changes the design-system implementation and narrow
   adapters, not every product module or application screen.

Status vocabulary:

- **proven** — shared root interface plus required automated and rendered evidence exist;
- **partial** — a shared interface exists but required variants, states, or adapters are missing;
- **legacy** — only the shipping web or Compose implementation exists;
- **missing** — no implementation exists at the target seam;
- **claimed** — an active workstream owns the seam, but unmerged work is not completion evidence.

## Initial coverage audit

| Capability | Target module/interface | Current evidence on `main` | Status | P3.5 exit evidence |
| --- | --- | --- | --- | --- |
| Semantic color, type, spacing, radius, target, and motion roles | `design-system` tokens and provider | Dark/light themes, pointer/touch/TV densities, contrast and drift tests, browser/native foundation stories | proven | Preserve current gates while later rows consume semantic roles only |
| Loomarr mark, wordmark, lockups, chroma bar, favicons, launchers, TV banner, and launch motion | `design-system` brand | Canonical brand contract, generated derivatives, launch sequence, drift tests, light/dark stories | proven | Every shipping derivative remains generated and workshop-visible |
| Product iconography | `design-system` `Icon` | Loomarr-owned Lucide interface, named sizes/tones, browser/native stories | proven | No arbitrary icon family or local size vocabulary in migrated surfaces |
| Loading and progress | `design-system` loading interfaces | Activity, skeleton, determinate progress, and signal-acquisition treatments with reduced-motion tests/stories | proven | Add only missing product wait states discovered by the parity inventory |
| QR presentation | `design-system` `QrCode` | Protected-centre branded QR, decode tests, pairing use | proven | Preserve unbranded fallback and scanner-safe geometry |
| Layout, surfaces, typography, artwork, focus, and primitive actions | `design-system` primitives and layout | `Screen`, `Surface`, `Text`, `ArtworkFrame`, `FocusSurface`, `Action`, `Badge`, `Field`, and `ProgressTrack`; P3.5 now supplies provider-owned platform insets, a 48-unit TV overscan policy, an edge-to-edge viewport frame, semantic action states, and shared `AdaptiveSplit`, `ScrollFrame`, and `Disclosure` seams | proven | Preserve responsive wide/narrow composition, scrolling, progressive disclosure, and disabled/pressed/selected/error/focus contracts in product modules |
| Form and selection controls | `design-system` interaction and selection modules; host semantics adapters where required | Shared `Action`, `Field`, `Toggle`, `ChoiceGroup`, `Tabs`, `MenuList`, `SelectControl`, and `Hint` own their cross-platform presentation and state contracts with pointer/touch/TV stories; the browser adapter owns automatic tab traversal, menu traversal/dismissal, select focus return/Escape, and hint hover/focus triggers, with interaction and axe coverage; dialogs use the shared `ModalOverlay` | partial | Add anchored/portal placement where a production consumer requires it, implement native long-press/D-pad adapters, and prove native input behavior rather than duplicating product rules |
| Feedback and recovery | `ui` `StatePanel` | Shared loading, empty, error, offline, retry, and permission treatments have useful accessible copy, one decisive recovery action, pointer/touch/TV stories, native stories, and centered loading geometry proof | proven | Preserve state vocabulary and recovery behavior as product modules consume it |
| Channel and programme identity | `ui` identity and metadata modules | Shared `ChannelIdentity`, `ProgrammeIdentity`, and composed `ProgrammeCard` own title, channel number/logo and initials fallback, series/season/episode, design-compliant airing label, description, badge, artwork state, and progress across pointer/touch/TV stories | proven | Preserve authoritative server metadata and deterministic missing-content fallbacks in Guide, Surf, overlays, and player chrome |
| Pairing and device recovery | `ui` `PairingShell`; `core/pairing` | Shared generated-contract state machine and presentation; iPhone/Shield pair, disconnect, and revocation evidence | proven | Remains dark-first, touch/remote reachable, mobile-responsive, and QR plus typed-code capable |
| Application navigation shell | `ui` `ClientNavigation` and `ClientShell` plus host Back adapters | Shared Watching/Guide/Surf destination intent, labelled selected-button navigation, icon vocabulary, keyboard reachability, TV preferred focus, and the Guide/Surf → Watching → platform-home Back rule now run through production native surfaces over the still-mounted player; confirmed disconnect remains in the shell. Physical 4K Shield traversal proves empty Watching/Guide/Surf route selection and excludes hidden Watching chrome from sibling-route semantics | partial | Prove populated-route focus restoration, surrounding-control handoff, final Back exit, and touch/browser parity |
| Guide product rules | `core/guide`; `ui` `GuideSurface`, `GuideExperience`, and `GuideJourney` | Shared geometry, metadata formatting, selection, movement, latest-request-wins fetch, empty/error recovery, and authoritative generated `/v1/guide` window now feed the production mobile/TV Guide; tuning its selection enters the still-mounted player without inventing metadata. Native artwork/logo renderers consume the generated image paths through the paired origin-locked adapter, and the journey accepts a platform-owned channel window without learning TV mechanics | partial | Prove tune/back/focus restoration, populated artwork, and refresh with the real API journey |
| Guide TV mechanics | `ui-tv` Guide adapter | TV-only package now owns deterministic grid/filter D-pad movement, disabled-filter skipping, time-anchor retention, tune/filter activation intent, focus restoration after catalog change, and a bounded 100-channel row window with explicit position labels; the production TV Guide now renders that bounded row window, and a Storybook remote workshop proves the full controller through Arrow/Enter interaction in both themes | partial | Connect the controller to native focus refs, then prove remote repeat, overscan, focus restoration, and 1080p/4K viewport behavior in emulator and physical Shield |
| Overlay and modal composition | `ui` `ModalOverlay` and `TransientOverlay`; React Native host adapters | Shared blocking and non-modal interfaces now own scrim, web portal/focus trap/return, Escape/scrim dismissal, safe content padding, reduced motion, edge-to-edge playback composition, and bounded auto-dismiss across web/native stories; production disconnect consumes the modal | partial | Prove stacked modal ownership plus native Back/dismiss and real touch/TV focus behavior before marking proven |
| Player state and transport | `player` root interface plus web/native adapters | The dependency-stacked P4 branch adds a generated-contract-derived root interface, latest-request-wins controller, intent-sensitive initial tuning, exact/wrapped tuning, identity reconciliation, bounded previous/recent history, Loomarr-owned overlay visibility, signed-URL source, origin-locked paired transport, and one Expo Video native adapter with serialized replacement and synchronous pause/release. A tested first-party XHR event-stream adapter supplies authenticated `channel` invalidations, authoritative catalog/Guide re-reads, transient reconnect, background closure, and fail-closed 401/403 handling without introducing another runtime dependency. Android TV sends the existing server-approved closed playback lifecycle through the bounded shared diagnostics reporter without retaining arbitrary error prose; Expo Doctor and Android/iOS/TV bundles are green | partial | Add the browser adapter, live-time-shift state, mobile diagnostic admission, and real first-frame/background/recovery evidence before P4 completion |
| Player chrome and timeline | `ui` player presentation consuming `player`; `ui-tv` number-entry adapter | Shared `WatchingSurface` keeps one supplied player mounted behind Channel identity, authoritative generated-Guide now/next identity and accessible live progress, tuning/error/dead-air feedback, bounded transient chrome, pause/Play/Go Live, and explicit previous/channel/Guide/Surf/retry actions across touch/TV native stories; it can suppress only chrome while sibling journeys cover the still-mounted player, regression-pinning a leak found on the physical Shield. The TV adapter retains the exact three-digit/1.2-second number-entry contract and interface tests cover the observable states | partial | Keep timeline identity fresh through reconciliation and prove touch/keyboard/remote traversal at every required viewport |
| Surf rail | `ui` `SurfRail` and `SurfJourney`; `ui-tv` Surf adapter | The production mobile/TV rail now preserves the still-mounted player, filters the authoritative Guide against the server-declared playable catalog, keeps empty Favourites honest, derives Recent/All with now/next/progress, restores selection by identity, and tunes through the shared player; the TV host supplies its tested identity-restoration adapter through the shared journey seam. Confirmed disconnect and Back remain available in the surrounding shell. Generated server version plus the native app-config version remain visible in populated and unavailable states and are proven on the physical Shield; mapped artwork/logo paths use the same origin-locked native renderer as Guide | partial | Bind TV movement to native focus refs, then prove populated artwork, catalog refresh, tune, previous-channel, and viewport behavior on browser, touch device, emulator, and Shield |
| Browser application semantics | web adapters | Mature legacy DOM controls and accessibility coverage exist, but most do not implement shared root interfaces | legacy | Shared modules retain semantic HTML, keyboard, screen-reader, reduced-motion, and responsive behavior without product-rule duplication |
| Touch mechanics | mobile adapters | Pairing shell and destination shell run on iPhone; safe-area and gesture contracts are not general modules | partial | Real iPhone portrait/landscape evidence for insets, targets, scrolling, gestures, keyboard, and focus transfer |
| TV mechanics | `ui-tv` root interfaces | A production arm64 release APK pairs over plain-HTTP LAN on the physical 4K Shield, persists its credential across process restart, and accepts D-pad/OK traversal through empty Watching, Guide, and Surf; exact number-entry buffering is interface-tested. Guide/Surf focus-controller logic remains disconnected from rendered native rows | partial | Populated physical 4K Shield plus 1080p host evidence for Back, number keys, focus restoration, remote repeat, overscan, and ten-minute traversal |

## Required workshop matrix

Every shared visual module declares the applicable cells rather than relying on one showcase story:

| Dimension | Required values |
| --- | --- |
| Theme | dark default, light, system-selected dark/light where supported |
| Density | pointer, touch, TV |
| Content | representative, long, short, missing, loading, empty, error, offline |
| Interaction | rest, hover where meaningful, pressed, selected, focused, disabled, invalid |
| Motion | normal and reduced |
| Viewport | mobile portrait, mobile landscape, desktop, 1920x1080 TV, 3840x2160 TV |
| Input | pointer, keyboard, screen reader, touch, D-pad/OK/Back, number key where applicable |

Story applicability is machine-readable and checked against each module's declared interface. A
meaningless combination may be excluded, but the story metadata records why; absence is not an
implicit exclusion.

## P3.5 evidence to date

The protected client matrix, including Apple mobile, Apple TV, the four Playwright visual and
accessibility shards, and the aggregate CI job, passed on PR #581 before its final rebase. The pinned
local visual run rendered 1,081 cases successfully with one declared skip, and a strict rerun passed
against the reviewed baselines.

On 2026-08-26, the native TV workshop was generated and compiled as both arm64 and x86 React Native
Android TV development builds. The arm64 build installed on the physical Nvidia Shield and rendered
at its 3840x2160 output; the x86 build installed on the dedicated Android TV emulator and rendered
at its native 1920x1080 host resolution. In both environments, the TV-specific workshop rail showed
the canonical lockup and focus treatment, D-pad Down moved focus between story cards, and OK selected
the focused story. Logcat contained no React Native or Android fatal exception. The local captures
are retained under
`.artifacts/primary/design-system-device-evidence/`.

This is not the complete P3.5 exit claim. The real-iPhone portrait/landscape workshop evidence
remains required.

## Publication gate

During implementation, affected package tests and focused Storybook checks are the normal feedback
loop. Publication requires one stable pass of `make clients`, `make fe`, `make fe-visual`,
`make docs-lint`, and `make check`, plus the real-device evidence named above. Expensive matrices are
not rerun after changes that cannot invalidate them; protected CI owns the final platform matrix.
