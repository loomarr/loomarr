# Web phone-width bottom bar — draft for maintainer mock

Draft checkpoint for the supervisor's part of decision **X1**
([redesign map](../redesign-map-2026-09.md#decisions)): "Web gets a bottom bar at phone width. The
supervisor drafts a brief; the maintainer mocks it." This is that brief, built interactively so it
can be reviewed in a browser rather than read as prose. It does not change production code, and it
does not itself approve a bottom bar — the map still holds **"Don't build the phone layout until
the mock exists"** until the maintainer signs off on (or redraws) what is here. Tracking:
[#1659](https://github.com/loomarr/loomarr/issues/1659),
[#1785](https://github.com/loomarr/loomarr/issues/1785), map [PR 18](../redesign-map-2026-09.md#pr-sequence).

Open [the interactive prototype](../prototypes/phone-web-nav.html) directly in a browser (works
offline, no server calls). Its controls switch role, whether a channel exists yet, the bar's
overflow composition, and a 390/320 px frame. Every fixture is invented; no real titles, no remote
images.

## What this replaces

Today, below the `md` breakpoint (768 px, `usePhoneWidth`'s threshold) `AppShell` keeps its 56 px
icon-only rail (`web/apps/web/src/components/loomarr/shell/app-shell/app-shell.tsx`) rather than
hiding it — the map's row 1 blocker ("no phone or narrow layout") recommended that as the floor
until a mock exists, and the maintainer's decision kept it that way. The phone Watch (#1793,
`ChannelWatch`) and Guide (#1795, `GuidePage` → `GuideCompact`) already ship beside that rail today.
This draft proposes removing the rail at phone width in favour of a bottom bar, and shows that both
existing phone surfaces still fit above it.

## Destinations, in the source's own order

Pulled verbatim from the map's `### Shell` section, which reads the mock's nav:

> Nav: admin gets Home, Watch, Guide, Requests (with a badge), Filler, People, Settings, Help.
> Member gets Home, Watch, Guide, Requests (with a badge), Notifications, Help.

That is 8 admin destinations and 6 member destinations once Watch exists — more than a bottom bar
can hold at a 44 pt+ target floor on a 320–390 px screen. Per **decision X2**, Watch is the
last-tuned channel and is **hidden** until one exists (drops to 7/5). Per **decision X3**, the
mini-player is dropped — nothing stands in for it here. Per **decision H6**, there is no kids or
restricted role in beta.9, so the role switch is only admin/member, matching
`web/apps/web/src/components/loomarr/shell/app-shell/app-shell.type.ts`'s `isAdmin` boolean; H2
(named viewing is admin-only) is a Home-content rule and has no bearing on the bar itself.

## The open question: which destinations sit in the bar

The map decides *that* there is a bottom bar; it does not decide which of the up-to-8 destinations
get primary billing versus a **More** sheet. The prototype implements both candidates behind a
toggle, since the inputs don't settle this and inventing a silent answer would be a design
decision dressed as a technical one:

- **Alt A — fixed primary set (recommended default in the draft).** The bar's first positions are
  always Home, Watch, Guide, Requests (filtered by whether each currently applies); everything
  else — Filler, People, Settings, Help for admin, Notifications, Help for member — lives behind
  **More**. When Watch is hidden (X2), its slot is removed rather than held empty or backfilled, so
  the remaining tabs widen instead of leaving a gap. The bar's shape never reshuffles based on role
  or channel state, which is the point: a returning viewer's muscle memory for "second tab is
  Watch" either holds or the tab is absent, never a different destination.
- **Alt B — always-fill-five.** The first five applicable destinations (in the same source order)
  go in the bar; **More** appears only past five. This removes the overflow sheet entirely for a
  member with no channel yet (Home, Guide, Requests, Notifications, Help — exactly five, verified
  in the prototype check below), but the fifth tab's identity changes with role and with X2's
  Watch visibility (admin's fifth slot is Filler; member's is Notifications or Help depending on
  whether Watch is showing), which is the trade-off against Alt A's stability.

Neither is a recorded maintainer rejection; the prototype's toggle exists so the maintainer's own
mock can pick one (or a third shape) with both laid out side by side rather than described.

## Visual language and components

- **Bar chrome** follows `@loomarr/design-system`'s `TabBar` (`web/packages/design-system/src/tab-bar/tab-bar.tsx`,
  built in #1776 for the native client): canvas background, a 1 px top hairline, a 49 pt row, each
  item a whole-column target, no selection pill (that's the iOS idiom; Android's Material idiom
  with the pill belongs to the native app, not this draft — web is one renderer, not OS-mimicking,
  so it only needs one idiom). Native's `ClientNavigation`
  (`web/packages/ui/src/client-navigation/client-navigation.tsx`) is a different product's nav (the
  dedicated phone client, 3 destinations: watching/guide/surf) — this draft is for `web/apps/web`'s
  own browser app, which keeps its full admin/member destination set, but borrows the same bar
  component's visual language so the two apps read as one product.
- **More** follows the same PR's `BottomSheet` docked-strip variant (no grabber — that variant is
  for content with no drag affordance, which matches a plain list of links). Opens on tap or
  <kbd>Enter</kbd>/<kbd>Space</kbd> on the More button, closes on <kbd>Esc</kbd>, backdrop click, or
  its own close button, and returns focus to the More button on close (checked below).
- **Tokens**: canvas `#0B0C0E`, raised `#131519`, elevated `#1B1E24`, hairline `#2A2E37`, signal
  (selected tab, focus ring) `#FFB020`, suggest/badge `#D6409F` — all from the map's "Tokens and
  type" section, which found **no new tokens needed** for the mock. This draft needs none either.
- **Requests' badge** (map: "Requests (with a badge)") renders as a small numeral inside the More
  sheet and a plain dot on the tab itself, matching today's rail
  (`app-shell.tsx`'s icon-only-rail dot vs. labelled-rail numeral) rather than inventing a new badge
  shape. Requests sits in the bar's primary set under both alternatives, so a badged item never
  actually lands behind More in this nav list — the dot-on-More affordance exists in the markup for
  a future destination that might, but isn't exercised by today's 8/6 items.

## Fitting the existing phone surfaces under the bar

- **Watch** (#1793): the prototype's Watch panel reuses the same shape — edge-to-edge picture, a
  live badge, the channel/programme line, the four transport controls, "next", favourites —
  without the `-mx-6 -mt-6` gutter-cancel class list `channel-watch.tsx` currently applies relative
  to a *side* rail. Once the rail is gone, that cancellation is simpler (no left offset to undo),
  which is a build-time simplification, not a design change.
- **Guide** (#1795): the prototype shows the compact grid, the All/Favorites/Recent chips,
  and — on tapping a programme — the docked strip with its own **Watch** action appearing **above**
  the bar, never overlapping it (asserted in the check below). Today, with no bottom bar yet, that
  docked strip is the bottom-most fixed element on a phone and presumably owns whatever bottom
  inset it needs on its own. Once the bar exists, the bar becomes the outermost docked element (the
  native `Screen`'s `footer` pattern: the outermost chrome carries the safe-area inset,
  `web/packages/design-system/src/viewport/viewport.tsx`'s `useViewportInsets`), and the docked
  strip sits flush above it with no inset of its own. That's a straightforward follow-on change in
  the implementation PR (map PR 18), not an open question — flagging it here so it isn't
  rediscovered as a bug once the bar ships.
- **Safe-area inset**: a real device's `env(safe-area-inset-bottom)` can't be produced by a static
  file opened in a desktop browser, so the prototype substitutes a checkbox that adds a fixed 20 px
  to the bar's bottom padding, standing in for a home-indicator gap, and says so in the control's
  label rather than silently faking a device value.

## State contract covered

- Role: admin / member (switches the destination set and its order).
- Channel existence (X2): toggles Watch in and out of every destination list and bar composition.
- Bar composition: Alt A / Alt B, live-recomputed for the current role and channel state.
- Frame: 390 px and 320 px, both at the current composition and role.
- Destination selection: every bar item and every More-sheet item is clickable and swaps the
  content pane, including Guide's own docked-strip demo.
- Keyboard: the bar is a real `role="tablist"` of `role="tab"` buttons in visual left-to-right DOM
  order, so native Tab traversal already matches reading order; More is a disclosure button
  (`aria-haspopup="dialog"`, `aria-expanded`) rather than a tab, since it opens a sheet instead of
  switching the active destination.
- Targets: every bar button is 49 pt tall (`TabBar`'s own iOS measurement), comfortably over the
  44 pt floor; More-sheet rows are 48 px.

## What this draft does not cover

It does not decide Home's content (H1, a separate screen), does not implement the real Guide
grid or Watch panel (those are #1795/#1793's actual components, only approximated here), does not
address the native client's own nav (unrelated product), and does not pick between Alt A and Alt B
— that choice is exactly what goes back to the maintainer along with this draft.

## Evidence

Frozen source reviewed: `f497dbf71c0541cba8ccd453da7b7a24913f20de` (branch head at the time of this
draft). Read: the redesign map's Shell section and Decisions table (X1–X3, H2, H6), the roll-up's
row 1, `TabBar`/`client-navigation` from #1776, `ChannelWatch` from #1793, `GuidePage` from #1795,
`AppShell`/`app-shell.type.ts`, `usePhoneWidth`, and `useViewportInsets`.

A local Playwright check ran against the standalone HTML only (not the app's e2e suite), using the
repo's cached Chromium binary directly since the `chrome` channel isn't installed on this machine:

- Alt A, admin, channel present → bar is Home/Watch/Guide/Requests/More.
- Every bar button measures ≥ 44 px tall (measured 49 px).
- The More sheet lists exactly the admin overflow (Filler/People/Settings/Help); <kbd>Esc</kbd>
  closes it and returns focus to the More button.
- Selecting Guide swaps the content header; tapping a programme shows the docked strip, and its
  bottom edge sits above the bar's top edge (no overlap).
- Hiding the channel (X2) drops Watch from the bar with no reserved gap.
- Alt A under the member role keeps the same four primary tabs plus More.
- Alt B under the member role with no channel fills exactly five tabs with **no** More sheet.
- Tab order from the review controls reaches the bar's Home before its More button, matching
  left-to-right visual order.
- The 320 px frame option actually resizes the frame to 320 px.

This is a prototype layout check, not production integration, device testing, or accessibility
certification. No screenshots or device captures are included.

## Checklist (from the brief)

- [x] `project/prototypes/phone-web-nav.html` — standalone, offline, invented fixtures, no remote
  images, admin/member switcher, every destination selectable, a More/overflow sheet (needed:
  8 admin / 6 member destinations exceed what fits), Watch hidden until a channel exists (X2),
  safe-area inset handling (simulated, documented above), 390 and 320 px frames, keyboard focus
  order, 44 px+ targets.
- [x] `project/evidence/phone-web-nav-2026-10-03.md` — this file, recording each choice's source
  and the two labelled alternatives for the one thing the inputs don't decide.
- [ ] `make verify BASE=origin/main` — run after this draft lands in the PR, see the PR body.
