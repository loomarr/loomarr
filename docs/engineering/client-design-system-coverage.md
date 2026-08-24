# Shared client design-system completeness ledger

**Status:** initial audit; P3.5 in progress  
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
| Form and selection controls | `design-system` interaction module; web semantics adapter where required | Shared `Action`, `Field`, `Toggle`, and `ChoiceGroup` own the checkbox, switch, and single-choice contracts with pointer/touch/TV stories and browser interaction coverage; select, menu, tabs, tooltip, and dialog remain legacy-web implementations | partial | Complete the remaining known selection and disclosure controls; DOM-only semantics remain narrow adapters |
| Feedback and recovery | `ui` `StatePanel` | Shared loading, empty, error, offline, retry, and permission treatments have useful accessible copy, one decisive recovery action, pointer/touch/TV stories, native stories, and centered loading geometry proof | proven | Preserve state vocabulary and recovery behavior as product modules consume it |
| Channel and programme identity | `ui` identity and metadata modules | Shared `ChannelIdentity`, `ProgrammeIdentity`, and composed `ProgrammeCard` own title, channel number/logo and initials fallback, series/season/episode, design-compliant airing label, description, badge, artwork state, and progress across pointer/touch/TV stories | proven | Preserve authoritative server metadata and deterministic missing-content fallbacks in Guide, Surf, overlays, and player chrome |
| Pairing and device recovery | `ui` `PairingShell`; `core/pairing` | Shared generated-contract state machine and presentation; iPhone/Shield pair, disconnect, and revocation evidence | proven | Remains dark-first, touch/remote reachable, mobile-responsive, and QR plus typed-code capable |
| Application navigation shell | `ui` shell plus platform navigation adapters | Shared destination shell exists; production destinations beyond pairing are placeholders | partial | Responsive navigation, focus return, Back semantics, route intent, and surrounding-control handoff proven on all interaction modes |
| Guide product rules | `core/guide` | Shared geometry, metadata formatting, selection, and movement rules with focused tests; current web presentation remains legacy | partial | Shared Guide presentation and detail composition consume the core interface with artwork, empty/error/loading states, and density-specific disclosure |
| Guide TV mechanics | `ui-tv` Guide adapter | Target package does not exist | missing | D-pad boundaries, focus restoration, 100-channel virtualization, time-anchor retention, overscan, and 1080p/4K viewport evidence |
| Overlay and modal composition | `ui` `ModalOverlay` and `TransientOverlay`; React Native host adapters | Shared blocking and non-modal interfaces now own scrim, web portal/focus trap/return, Escape/scrim dismissal, safe content padding, reduced motion, edge-to-edge playback composition, and bounded auto-dismiss across web/native stories; production disconnect consumes the modal | partial | Prove stacked modal ownership plus native Back/dismiss and real touch/TV focus behavior before marking proven |
| Player state and transport | `player` root interface plus web/native adapters | `player-shared-modules` is separately claimed for P4; no merged target package exists on this baseline | claimed | Shared state/tune/history interface with hls.js and native transport adapters; real first-frame and recovery evidence remains P4-owned |
| Player chrome and timeline | `ui` player presentation consuming `player` | Mature controls exist only in legacy web; no shared touch/TV presentation | legacy | Edge-to-edge bottom anchoring, live/time-shift states, accessible controls, auto-dismiss, touch/keyboard/remote traversal, and all densities |
| Surf rail | `ui` Surf module plus TV/touch adapters | No target shared implementation | missing | Authoritative now/next identity, artwork, focus/gesture traversal, tune and previous-channel intent, overlay timing, and viewport-safe composition |
| Browser application semantics | web adapters | Mature legacy DOM controls and accessibility coverage exist, but most do not implement shared root interfaces | legacy | Shared modules retain semantic HTML, keyboard, screen-reader, reduced-motion, and responsive behavior without product-rule duplication |
| Touch mechanics | mobile adapters | Pairing shell and destination shell run on iPhone; safe-area and gesture contracts are not general modules | partial | Real iPhone portrait/landscape evidence for insets, targets, scrolling, gestures, keyboard, and focus transfer |
| TV mechanics | `ui-tv` root interfaces | Pairing/shell remote traversal exists; no general focus, remote, overscan, or virtualization package | partial | Physical 4K Shield plus 1080p host evidence for D-pad, OK, Back, number keys, focus restoration, overscan, and ten-minute traversal |

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

## Publication gate

During implementation, affected package tests and focused Storybook checks are the normal feedback
loop. Publication requires one stable pass of `make clients`, `make fe`, `make fe-visual`,
`make docs-lint`, and `make check`, plus the real-device evidence named above. Expensive matrices are
not rerun after changes that cannot invalidate them; protected CI owns the final platform matrix.
