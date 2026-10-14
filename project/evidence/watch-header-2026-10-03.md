# Watch header & management entry — design review candidate

Review checkpoint for [#1817](https://github.com/loomarr/loomarr/issues/1817) item 3 and parent
[#1659](https://github.com/loomarr/loomarr/issues/1659). This is a proposal, not an approved
behaviour change. No production routes, components or API contracts change.

Open [the interactive prototype](../prototypes/watch-header.html) directly in a browser. Its
controls switch role, the shell's Watch nav-entry state, the two open-question alternatives, air
state, active channel section and a 390px frame. All fixtures are invented; nothing calls a
server; links are inert.

## What this covers and why

[#1817](https://github.com/loomarr/loomarr/issues/1817) item 3 asks for "viewer-first channel
Watch header and management entry," explicitly retaining the approved phone Watching layout
(mock 5e, [#1793](https://github.com/loomarr/loomarr/pull/1793),
[#1804](https://github.com/loomarr/loomarr/pull/1804)) and not restoring the rejected mini-player.
Three things were already decided and built before this draft:

- **W1** ([redesign-map-2026-09.md](../redesign-map-2026-09.md#channel-detail)): keep the tab
  player, borrow the console mock's channels drawer and 0–9 direct tune, drop the encoder
  readout. Built in [#1725](https://github.com/loomarr/loomarr/pull/1725).
- **X3**: drop the sidebar mini-player. No code reintroduces it; this draft doesn't either.
- Channel-detail tabs, their content (Info/Programming/Filler/Danger) and the header's back
  arrow/ident/name/air-line/CH-number layout are already built
  ([#1739](https://github.com/loomarr/loomarr/pull/1739),
  `web/apps/web/src/routes/_authed/channels/$id/route.tsx`).

What is **not** built is the **X2** shell decision this header and tab bar sit inside: Watch is
the shell's nav entry for "the last-tuned channel," hidden until one exists, and channel
management (Info/Programming/Filler/Danger) highlights **Guide** in the rail rather than Watch —
so one channel entity never lights two nav items at once (redesign-map critique roll-up row 5).
`app-shell.tsx`'s nav lists (`ADMIN_NAV`/`MEMBER_NAV`) have no Watch item today, and nothing
computes "last-tuned channel." This draft is the design record for that entry and for how the
existing header/tabs read for a viewer versus an admin, before that shell work is built.

## What the prototype shows

### Shell Watch entry (SOURCE: decisions X2, X3; `app-shell.tsx` nav lists)

- **Hidden** when no channel exists in the household at all (`chstate = "No channel exists yet"`)
  — X2's "hidden until there's a channel." No mini-player is reintroduced in its place (X3);
  the content area explains there is nothing to preview.
- **Visible** once a last-tuned channel exists, pointing at it. No drawer or still player is
  drawn inside the rail item itself — it is a plain nav row, matching every other destination
  (`Home`, `Guide`, …), never a resident video surface.
- The nav list order and the admin/member split come straight from today's `ADMIN_NAV`/
  `MEMBER_NAV` arrays, with `Watch` inserted second, matching the position the
  [#1839 phone-bottom-bar draft](../prototypes/phone-web-nav.html) already modelled for the same
  decision.

### Channel header (SOURCE: `route.tsx`'s built header, redesign-map Channel-detail § Watch tab)

- Back (24px target, fixing roll-up row 36's 16px measurement — already fixed in the merged
  code), the channel's monogram ident (N8: `ChannelIdent`, hue by channel number, initials via
  `monogramOf`), the name, an air line, and `CH nn` at the right.
- Air line reads four ways, matching `headerAirLine`/`airStateOf` in the merged route: **On
  now: … · Nm left** (red dot), **Paused** (grey dot), **On air (catching up)** (grey dot,
  reconciling), **Not on air yet** (grey dot). Toggle "Air state" to compare; it is disabled
  when Watch has no reachable channel (header isn't shown).

### Management entry, without crowding viewers (SOURCE: `route.tsx`'s `SECTIONS` filter, X2)

- The tab bar is the existing `NavTabs` pill style. A **member** always sees exactly **Watch**
  and **Channel info**. An **admin** additionally sees **Programming**, **Filler** and **Danger
  zone** — filtered by role, not hidden behind a disclosure or a second menu. A viewer never sees
  the three admin tabs exist; that is the entire answer to "without crowding viewers," and it is
  already the shipped behaviour, not a new pattern invented for this draft.
- **Nav highlight transfer (X2, the fix for critique row 5):** selecting Watch highlights the
  rail's **Watch** item; selecting any of Info/Programming/Filler/Danger highlights **Guide**
  instead, even though the URL stays under `/channels/$id/…`. Toggle "Section" to see the rail
  highlight move.

### Desktop and phone

- **Desktop:** the full rail + header + tabs + a schematic Watch frame (top bar, control bar,
  drawer/direct-tune noted but not redrawn — those are #1725's approved surface, out of scope
  here) for Watch, and placeholder cards carrying #1739's real section copy for Info/Programming/
  Filler/Danger.
- **Phone (390px):** the rail collapses to the existing 56px icon-only floor (X1 is still
  undecided and out of scope for this draft — [#1839](../prototypes/phone-web-nav.html) owns
  it). The header and tabs stay above the picture; the Watch section shows the approved
  edge-to-edge picture and `WatchingPanel` from #1793/#1804 **unchanged**, labelled as such — this
  draft does not restyle or re-decide that surface.

## Open questions (not decided by any input)

1. **Watch rail label once a channel is tuned.** Alt A keeps a static "Watch" label (matching
   every other nav row, and what the #1839 draft already drew). Alt B shows the live channel
   ("CH 07"), which is more informative but means the rail's own label changes as someone surfs,
   and duplicates information the header already states one click away. Neither X2 nor any
   merged code picks one. *No recommendation is baked into the prototype's default — compare
   both via "Watch rail label."*
2. **What Watch targets before any tune has happened.** X2's text — "Watch = the last-tuned
   channel… hidden until there's a channel" — is ambiguous at the edge where channels exist but
   nobody has tuned one yet (first run, right after creating the first channel). Alt A reads "a
   channel" as "a channel exists" and opens the first one by number (the redesign map's original
   recommendation, before X2's wording superseded it). Alt B reads it strictly — hidden until a
   *last-tuned* channel exists, i.e. until someone tunes once, which could mean a fresh
   install's Watch entry never appears on its own. [#1662](https://github.com/loomarr/loomarr/issues/1662)
   (the last-tuned-channel API) is also still open, so this can't be settled from code either.
   Select "A channel exists, never tuned" to compare both alternatives.

Both questions are candidates for the maintainer to settle before the Shell PR (map PR #4) is
built; this draft does not pick one.

## Not covered here

- The player, channels drawer and 0–9 direct tune themselves (#1725, built and approved).
- The phone Watching picture/panel layout itself (#1793/#1804, built and approved) — shown
  unchanged, not redesigned.
- The X1 bottom-bar question for phone-width shell navigation ([#1839](../prototypes/phone-web-nav.html)
  owns it; this draft keeps today's 56px icon-rail floor at phone width, per X1's "don't build
  until the mock exists").
- Named-viewing counts/identity (H2) — this header shows no viewer-count or presence data at all;
  that is Home's concern, not Watch's.

## Evidence

Frozen source reviewed: `d7e76eb85c0ab123b0a6e47f4232ac92e1aff9bb`. Read: issue #1817, the
redesign map's Channel-detail section and Decisions table (W1, X2, X3, H2), and the merged PRs
#1725, #1739, #1793 and #1804. The channel header, `NavTabs` and `ChannelIdent` component source
were read directly (`web/apps/web/src/routes/_authed/channels/$id/route.tsx`,
`web/apps/web/src/components/ui/nav-tabs/nav-tabs.tsx`, `web/packages/ui/src/channel-ident/channel-ident.tsx`)
to keep the header/tab wording and visual language faithful rather than invented. The shell's nav
lists were read from `web/apps/web/src/components/loomarr/shell/app-shell/app-shell.tsx` to confirm
no Watch entry exists today.

A short local Playwright check drove the standalone HTML headlessly (Chromium, via
`web/node_modules/.pnpm/playwright@1.62.1`): every role/channel-state/section/frame-size
combination rendered with zero console or page errors, and the rail's highlighted item moved
from Watch to Guide when the active section changed from Watch to any management tab. This is a
prototype layout check, not production integration, device or accessibility certification.

Local reproduction: open `project/prototypes/watch-header.html` directly in a browser (file://,
no server needed).
