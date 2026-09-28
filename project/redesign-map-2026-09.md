# Redesign map (beta.9, #1659)

**For:** the maintainer, and the lanes that build the redesign.
**You'll get:** for each screen of the Claude Design mocks, what it shows, what it replaces, what the
design system and the API already have, a critique with severities, keep/change/drop calls, the
questions only you can answer, and a proposed PR sequence.

Phase 0 of #1659. The maintainer answered the open questions on 2026-09-28. The
[Decisions](#decisions) section records the answers, and they win over any earlier recommendation
here. Delete this file when #1659 closes.

## How this was made

- **Sources** (private, outside the repo, never committed): `Loomarr Web.dc.html` (web app),
  `Home options.dc.html` (three Home directions, admin and member), `Loomarr TV + iOS.dc.html`
  (native frames 5a–5h), `watch-a-console.dc.html` (a watch-page variant), and `composites-data.js`
  (the filler mocks' demo data). The mocks contain real titles, so this map uses genre words,
  and there are no mock screenshots here.
- **Walked in Chromium** with the repo's Playwright: 94 scenarios covering every nav item, tab,
  wizard step and dialog, in both roles (`admin`, `member`), under all four data states the mock
  models (`ready`, `loading`, `empty`, `error`). 1440 px throughout, plus 1024 and 390 px for the
  main screens. The canvas mocks were captured full-page.
- **Measured:** axe-core 4.13 (WCAG 2.0/2.1/2.2 A+AA) on 26 screens. WCAG contrast of every
  rendered text node, composited over its real background (1,534 nodes). Tab order and visible
  focus on 7 screens. Hit-target sizes at 390 px.
- **Diffed** against today's routes (`web/apps/web/src/routes`), packages (`web/packages/*`) and
  `api/openapi.yaml`. The mock's sync note says the web mock is "a faithful baseline of
  `web/apps/web` on main" plus changes, so most screens are **deltas**, not rewrites. The
  deltas below come from reading code and the mock side by side. I did **not** run today's app
  next to the mock, and the native deltas come from the package inventory, not from an
  emulator run.

## Summary

| | |
| --- | --- |
| Screens mapped | 17 web surfaces (39 distinct states), 3 Home directions × 2 roles, 8 native frames, 1 watch variant |
| Mostly built today | Guide, channel detail (5 tabs), Requests, Filler, Settings, People, Account, Help, sign-in, pair, wizard, TV watching/guide/surf, phone shell |
| New or rebuilt | **Home** (replaces the admin Dashboard and becomes the landing page for both roles), the Watch nav entry, the sidebar mini-player, member channel ideas, native favourites/recents, phone watching + detail sheet, the iPad split view |
| API gaps filed | [#1662](https://github.com/loomarr/loomarr/issues/1662) viewing · [#1663](https://github.com/loomarr/loomarr/issues/1663) new this week · [#1664](https://github.com/loomarr/loomarr/issues/1664) tonight highlights · [#1665](https://github.com/loomarr/loomarr/issues/1665) channel ideas · [#1666](https://github.com/loomarr/loomarr/issues/1666) favourites/recents · [#1667](https://github.com/loomarr/loomarr/issues/1667) small gaps |
| Critique | 4 blockers, 27 majors, 9 minors in the [roll-up](#critique-roll-up), plus TV B4's dependency on the #1627 primitives |
| Biggest risks | The web phone layout waits for its mock · Home waits on four API issues (#1662–#1665) · the Guide becomes a migration to `ui/guide` · native Requests has no mock yet |
| Decided | Every open question, 2026-09-28 ([Decisions](#decisions)) |

Severity: **blocker** = can't build this screen until it's decided or fixed. **Major** = would
ship a real usability, accessibility, privacy or correctness defect. **Minor** = polish or
consistency.

---

## Cross-cutting (web)

### Tokens and type

The mock uses today's tokens (`web/packages/tokens`): canvas `#0B0C0E`, raised `#131519`,
elevated `#1B1E24`, hairline `#2A2E37`, text `#E7EAF0` / `#8B93A3` / `#5A6170`, signal `#FFB020`,
on-air `#E5484D` (text `#E85A5F`), lock `#3DD68C`, tune `#4CC9E8`, suggest `#D6409F` (text
`#DC5BAC`), caution `#F5D90A`, Geist and Geist Mono, radii 4/8/12. **No new tokens are needed.**

Measured contrast of the text tokens (WCAG 2.x, AA needs 4.5:1 for body text, 3:1 for large text and
for control boundaries):

| Foreground | on canvas | on raised | on elevated | Verdict |
| --- | --- | --- | --- | --- |
| signal `#FFB020` | 10.70 | 9.99 | 9.13 | passes everywhere, including 11 px |
| muted `#8B93A3` | 6.34 | 5.92 | 5.41 | passes |
| on-air text `#E85A5F` | 5.65 | 5.27 | 4.82 | passes |
| suggest text `#DC5BAC` | 5.72 | 5.34 | 4.88 | passes on solid surfaces; **3.45–3.78 on its own 15 % tint over on-air tint** (measured, 11 px) |
| **tertiary `#5A6170`** | **3.15** | **2.94** | **2.69** | **fails body text** everywhere; fine only for disabled or decorative text |
| **control outline `#61646B`** | 3.30 | 3.08 | **2.82** | **fails 1.4.11** (3:1) on elevated surfaces |
| hairline `#2A2E37` | 1.44 | 1.34 | 1.23 | decorative only: must never be the only boundary of an input |
| `#0B0C0E` on signal (primary button) | 10.70 | | | passes |

Rendered screens: **9 of 1,534 text nodes fail**. They're the tertiary token, suggest-on-tint
badges, and the disabled bulk "Approve" label (3.18:1). Type sizes rendered: 11 px ×182, 12 px
×461, 13 px ×713, 14 px and up ×178. That's 42 % of all text at 11–12 px.

### Shell: sidebar, search, mini-player, account

- **Mock:** a 224 px rail with the wordmark, a Search… ⌘K button, the nav, a mini-player (thumbnail,
  channel number and name, title, progress; it opens Watch), the version (it opens About for
  admins), and the account row with sign-out. Nav: admin gets Home, Watch, Guide, Requests (with a
  badge), Filler, People, Settings, Help. Member gets Home, Watch, Guide, Requests (with a badge),
  Notifications, Help.
- **Today:** `components/loomarr/shell/app-shell` has admin Dashboard, Guide, Requests, Filler,
  People, Settings, Help, and member Guide, Requests, Notifications, Help. The rail **collapses to
  a 56 px icon rail below `md`**. There's no mini-player and no Watch entry.
- **Design system:** everything exists except the mini-player and a "last channel" source.
- **Data:** the badge comes from pending proposals, filler pulls and failed journeys (API has all
  three). Version: `/v1/system/version` (has it). Mini-player: a per-user last-tuned channel
  (**[#1662](https://github.com/loomarr/loomarr/issues/1662)**, or client-local).
- **Interactions:** ⌘K / Ctrl-K toggles the palette and Esc closes it. The mini-player opens
  Watch. The version opens About (admin only).

**Critique**

- **Blocker: there's no phone or narrow layout.** At 390 px the 224 px rail stays and content
  gets about 110 px: headings wrap a word per line, status-strip buttons clip off-screen and
  Watching-now cards collapse to slivers. Wizard step 4 scrolls horizontally. Today's shell already
  collapses to an icon rail. *Recommend:* keep today's collapse as the floor, and get a mock for
  under 768 px before the shell PR (Q-X1).
- **Major: the nav highlight jumps between sections for one channel.** The Watch tab lights
  "Watch", but Info, Programming, Filler and Danger light "Guide", so the same URL entity lives in
  two sections. *Recommend:* Watch is the player route only, and channel management is always under
  Guide (Q-X2).
- **Major: "Watch" has no defined target on a fresh install or for someone who hasn't tuned.** The
  mock hard-wires channel 7. *Recommend:* the last-tuned channel, else the first channel, else hide
  the entry (Q-X2).
- **Major: the mini-player duplicates Watching now and the player.** If it plays video, every
  open tab holds a playout session. Beta.8's on-demand playout spends one GPU stream per channel,
  and a second audio source fights the main player. *Recommend:* **drop**, or make it a still
  "Resume CH 07" link with no media (Q-X3).
- **Minor:** Search… is a button that only shows ⌘K. Show Ctrl-K on non-Apple platforms.

### Command palette (⌘K)

- **Mock:** grouped results for Admin tools (Diagnostics, Restart Loomarr), Channels, In your
  library, Not in your library yet (TMDB), Filler clips and Help. The first row is preselected, and
  there's an empty-query hint and a no-match state.
- **Today:** `shell/search-command` exists.
- **Data:** `/v1/search` (library and TMDB, `inLibrary`), `/v1/channels`, `/v1/filler` (clips),
  `/v1/docs` (help). All present, merged client-side.
- **Critique:**
  - **Minor:** "Restart Loomarr" is two keystrokes from anywhere. It goes through the confirm
    dialog, so keep it, but never put it on the first row.
  - **Major (carried over):** "How prepared playback works" is a withdrawn feature (see
    [Settings](#settings-and-this-server)).

### Restart overlay and confirm

- **Mock:** the confirm dialog explains the drop ("3 channels Loomarr is streaming will cut out
  for a few seconds"). Then a full-screen "Restarting Loomarr" spinner, then "Loomarr is back".
- **Today:** `shell/restart-overlay` and `dashboard/restart-needed-banner` both exist (§9.2).
- **Data:** `/v1/system/restart` exists. The "N channels will cut out" count needs
  `/v1/playout/sessions` (has it).
- **Keep.**

---

## Home

- **Mock (web, `dashboard` screen):** it's the landing page for both roles, and only the variant
  sections differ by role.
  - **Status strip.** Admin gets "All N channels are playing", "N requests need you [Review]" and
    "Restart Loomarr to finish saving N settings [Restart…]". Member gets "Everything's playing"
    and "1 of your requests couldn't be built [Edit and retry]".
  - **Watching now** (3 cards: person, device, title, channel, progress). The member's own card is
    first and outlined in amber.
  - **Channel ideas** (member only): 3 idea cards with reason, pitch, 4 posters, count and
    availability, plus Request / Edit first / Hide (with undo) and "Different ideas". There's also a
    "describe your own" row.
  - **Your requests** (member) or **On the way** (admin: downloads with progress and ETA).
  - **Tonight** (4 highlight rows with a reason).
  - **New this week** (a new-channel card plus 5 posters).
  - Admin gets a single "An idea from your library" card, and a footer pointing to Settings →
    This server for "encoder, storage and service details".
- **States in the mock:** loading ("Checking your channels…", pulsing strip). Empty ("Nothing on
  air yet" + "Add your first channel" / "Request a channel"). Error (red strip "Two connected
  services aren't answering" + Fix).
- **Home options (canvas):** three directions. **1a On-now board**: every channel as a tile with
  who's on it, and tonight and downloads in a side column. **1b Living room**: Watching now,
  Tonight's highlights as a timeline, New this week, New channels, On the way, ideas. **1c
  Console**: a now/next table per channel with viewer initials. Member versions are 2a–2c. The
  web mock is closest to **1b**, but its Tonight is a list, not 1b's timeline.
- **Replaces:** `routes/_authed/dashboard.tsx` (stat cards, `PlayoutPanel`, `ServicesPanel`,
  `ActivityFeed`, `RestartNeededBanner`). **All of that content is absent from the new Home**; the
  mock's script still computes it but never renders it. Members have no Home today (§11 kept the
  Dashboard admin-only because it was machine state).
- **Design system:** has cards, badges, progress, buttons, and the status dot. **Missing:**
  StatusStrip, SectionHeader (title + meta + trailing link), WatchingCard (still, avatar chip,
  progress), PosterRail, HighlightRow, IdeaCard, and a list row with dot, action and progress.
- **Data:**

  | Datum | API | Gap |
  | --- | --- | --- |
  | N channels playing, service failures | `/v1/guide` status, `/v1/system/services` | none |
  | Requests needing you | `/v1/proposals?status=`, `/v1/filler/pulls`, `/v1/proposal-jobs` | none |
  | Restart pending | settings `apply=restart` + `RestartNeededBanner`'s existing source | none |
  | Who is watching, device, progress | `/v1/playout/sessions` has a **count** per channel only | **[#1662](https://github.com/loomarr/loomarr/issues/1662)** |
  | Tonight's highlights and their reasons | `/v1/guide` has season/episode/year | **[#1664](https://github.com/loomarr/loomarr/issues/1664)** (selection, "new on channel") |
  | New this week (titles + which channel), new channel "added Tuesday", "requested by" | `TitleDTO` has no arrival time or channel | **[#1663](https://github.com/loomarr/loomarr/issues/1663)** |
  | On the way: title, channel, progress, ETA | `/v1/titles?state=downloading` + `ChannelDTO.lineup` | "8 of 36 episodes" → **[#1667](https://github.com/loomarr/loomarr/issues/1667)** |
  | Channel ideas (reason, pitch, posters, counts, availability) | only static §13 templates | **[#1665](https://github.com/loomarr/loomarr/issues/1665)** |
  | Hide an idea | `/v1/discovery/feedback` | none |
  | Posters and stills | `/v1/images` (with `placeholder` and `dominantHex`) | none |

- **Interactions:** strip buttons (Review → Requests › Needs you; Restart… → confirm; Edit and
  retry → Requests). Cards open the channel's Watch. Idea Request toggles to "Requested" with
  Undo. Edit first opens Guide › Add a channel prefilled. Hide shows an undo toast. Different
  ideas pages through 3 at a time. Describe your own + Enter/Continue opens the add panel.

**Critique**

- **Blocker: the direction isn't chosen.** The web mock, 1a, 1b and 1c all differ, and the web
  mock is a fourth hybrid (Q-H1).
- **Blocker: household viewing privacy.** Watching now names the person, the device and exactly
  what they're watching, and shows it to every member, including other members' viewing and, if
  kids have accounts, kids'. §342 makes *titles and channels* globally readable, but that's the
  catalogue, not a record of who watched what. **Decided (Q-H2):** admins see named viewing,
  members see counts. Each person still sees their own "continue watching". The server enforces it
  (#1662), not the UI.
- **Major: a viewer's home and an operator dashboard in one page.** Approvals, restart and
  "couldn't be built" sit above what's on. The Requests badge already carries the approval count,
  so the strip repeats it. *Recommend:* one Home for everyone, led by what's on. The admin strip
  shrinks to one line of **counts that link out**, and the restart item appears only when a
  restart-apply key is actually dirty (it's already that precise today) (Q-H1).
- **Major: there's no kids view, and the API has no kids role.** Roles are `admin|member`, and
  the audience ceiling is per **channel** (kids/teen guardrail; adult is the default). A kid
  signed in as a member would see ideas, New this week and Watching now drawn from adult
  channels. *Recommend:* decide whether a restricted viewer is in scope for beta.9. If it is,
  it's a role (API work), not a Home variant (Q-H6).
- **Major: data realism.** Four sections need data the API doesn't have (table above). The mock's
  reasons ("Season 4 premiere", "Back-to-back until 11", "New on <channel>", "<holiday> is five
  weeks away", "your request found 2, this finds 9") must come from typed server reasons that the
  client words, never from strings in the client. Every poster and still is an image slot: the
  mock never shows a **missing artwork** state, and household libraries have gaps. *Recommend:*
  a code-drawn fallback from tokens (channel monogram and dominant hue), following the rejected-AI-art
  rule.
- **Major: where did today's operator panels go?** Playout telemetry (encoder, speed, buffered,
  viewers), services and activity have no screen in the mock. "This server" in the mock has
  Playback *settings*, not live telemetry. *Recommend:* move the playout and services panels to
  **This server → Playback** and **Diagnostics**. Don't drop them silently (Q-H7).
- **Major: the LLM-off path.** Ideas are fine *if* #1665 builds them without the LLM. "Edit first"
  and "describe your own" land on Add a channel, which needs the LLM, and it's off on the
  household install. See [Guide](#guide-channels).
- **Minor: first run is one line.** The empty Home doesn't tell "no channels" apart from "nothing
  connected yet" (media server unset → the dead-air card). *Recommend:* on empty, show which setup
  step is missing and link to it.
- **Minor: scale.** Watching now is fixed at 3 cards and Tonight at 4 rows. With 8 viewers or 60
  channels, the rule for what's shown isn't stated.
- **Minor:** the admin idea card shows one idea without Hide or shuffle, while the member card
  has both.

**Keep:** leading with what's on, the Tonight list, New this week, member ideas with
undo-able hide, the footer link to operator detail. **Change:** the strip to counts only, the
admin/member split to data scope rather than two layouts, and named viewing to opt-in. **Drop:**
the duplicate approvals messaging.

---

## Guide (Channels)

- **Mock:** header "Channels" with the primary **Add a channel** (member: **Request a channel**).
  Toolbar: Today ▾, ‹ NOW ›, span 2h/4h/6h/12h, View. **On now**: 4 cards for admins, 6 for members
  (larger text), each with a still, progress, number, name, title and "Nm left". **Grid**: a 260 px
  channel column (monogram ident, number, name, live/reconciling/off dot, a status chip for
  **Still downloading / Updating / Paused**, and a ⋯ menu). Airing blocks are amber-tinted, pending
  blocks dashed tune, and filler pods show as a strip of clip segments. There's an "Off air" block
  and a now line.
  - **Add a channel** swaps the page into a describe panel ("Describe the channel" → **Suggest
    titles**) with Close.
  - **States:** loading (empty rows), empty ("DEAD AIR / No channels yet" + Add a channel), error.
- **Replaces:** `channels/guide-page/guide-page.tsx`, `components/loomarr/guide/guide-grid`,
  `channels/channel-ident`. It's mostly built. The deltas are the On now cards, the span picker
  and the View menu.
- **Design system:** has the grid, ident and chips. **Missing:** OnNowCard (shared with Home's
  WatchingCard: same card, different overlay).
- **Data:** `/v1/guide` gives airings, `kind=program|filler|pending|flex`, pods with clip entries,
  `status=building|live|empty|drifted|detached|paused` and `pendingCount`. `/v1/channels/now-next`
  covers the rest. **No gaps.** The chip wording maps from the `status` enum (orval), not from
  strings.
- **Interactions:** a row opens the channel. Span changes the time scale. ‹ NOW › pages time.
  ⋯ opens a channel menu (its content isn't shown in the mock).

**Critique**

- **Major: the mock's own bug.** Paused channel 55's current block gets the amber "airing"
  treatment: the block style checks the time, not the channel's air state. Don't copy it. A
  paused channel's blocks are the neutral tone.
- **Major: narrow blocks lose their label.** Under 74 px (a 22-minute episode at 4h) blocks have
  no text, and under 132 px they lose the time. The mock gives them a `title=` tooltip only, which
  keyboard and touch users never see. *Recommend:* every block is focusable with an accessible
  name "Series, episode, 8:00–8:22 PM", and focus shows the detail strip the TV guide already
  has (5b).
- **Major: huge libraries.** The mock has 8 channels and the native mocks say 54. Rows are 56 px,
  and there's no jump-to-channel, search-in-grid or virtualisation. *Recommend:* type-to-jump by
  number or name, and virtualised rows (Q-G1).
- **Major: keyboard model.** A grid with 8×12 focusable blocks and no arrow-key model means about
  100 Tab stops (the Tab walk hit 40 stops before leaving the nav and On now). *Recommend:* a
  roving tabindex with arrow keys inside the grid, the same model as `ui-tv/guide-navigation`.
- **Major: LLM off.** Add a channel → "Suggest titles" and channel Programming → "Refine with AI"
  depend on the LLM, which is off by design on the household install. The mock has no "AI isn't set
  up" state. *Recommend:* the describe panel offers the §13 starter templates and a manual
  lineup when the LLM is unavailable, and says why (Q-G2).
- **Minor:** On now shows 4 for admins and 6 for members, an arbitrary cut. Show every channel on
  air, scrolling horizontally, or tie it to the viewport.
- **Minor:** channel-name buttons measure 150×20 px (the WCAG 2.5.8 minimum is 24 px tall).

**Keep:** the grid, pods as segments, the status chips, the On now row. **Change:** the
blocks' focus and names, and a paused channel's colour. **Add:** jump-to-channel and the LLM-off
state.

---

## Channel detail

One route with tabs (`routes/_authed/channels/$id/{watch,info,programming,filler,danger}.tsx`),
already path-based through `NavTabs`. Members see only Watch and Channel info.

### Watch tab

- **Mock:** the header has back, icon, name, "On now: … · Nm left" and "CH 7". The player
  overlays its controls on the frame (LIVE badge, "CH 7 name", a progress bar with pod marks,
  pause, LIVE, elapsed/duration, "Nm left", channel ▾▴, volume, fullscreen). A note below says
  "The audio language you pick applies to everyone watching this channel", with **Open in
  Jellyfin**. It has loading and error variants, and paused channels show "Paused" in place of On
  now.
- **Variant, `watch-a-console`:** a full-bleed console. It has a "Watch live" gate over a blurred
  title card, a top bar ("CH 03 name · 2 watching", encoder "hevc_nvenc · 1080p · 1.7× rt", Open in
  Jellyfin), a programme strip with pods under the frame, a bottom bar (pause, LIVE, title and time,
  channel stepper, ☰ Channels, audio, CC, fullscreen) and keyboard hints ("0–9 TUNE DIRECT · ⌃⌄
  CHANNEL · SPACE PAUSE · G CHANNELS · M MUTE"). A **Channels drawer** lists "NOW → NEXT 2H",
  Favorites and All channels, with star toggles, a TUNED marker and a progress line.
- **Replaces:** `channels/channel-watch`, and the player in `web/packages/player`.
- **Data:** `/v1/channels/{id}/play-url`, `/tracks`, `/timeline`, `/upcoming`, and now-next:
  present. "N watching" is a count from sessions, which is present. Favourites (drawer) are
  **[#1666](https://github.com/loomarr/loomarr/issues/1666)**. "Open in Jellyfin": the server type
  comes from settings (`library` server kind), never a hard-coded brand.

**Critique**

- **Major: which watch page?** The tab and the console differ a lot (Q-W1). The console shows
  **encoder, resolution and speed to viewers**. That's operator telemetry on a viewer surface, the
  "operator-console channel detail" §12 rejected. *Recommend:* keep the tab player (overlay
  controls, per the 2026-08-07 rule), borrow the console's **channels drawer** and **0–9
  direct tune + keyboard hints**, and drop the encoder readout from viewers.
- **Minor:** the header shows the channel number twice ("CH 7" at right, and in the player badge).
- **Minor:** the back arrow measures 16×16 px (under 24).

### Channel info

- **Mock:** an air state ("On air · Playing now in your TV guide" / "Paused · Off air, but kept"),
  **What's on** (4 upcoming rows, NOW marker), **Between shows** (break summary), "12 of 15 titles
  ready, the rest fill in as titles arrive…", channel icon and watermark, and a collapsible
  diagnostics panel.
- **Data:** `/upcoming`, `ChannelDTO` (lineup states, `breakCount`, policy), `/icon`,
  `/watermark`: present. **No gaps.**
- **Keep.** **Minor:** "12 of 15 titles ready" reads as a problem to members. Show it to admins
  only.

### Programming (admin)

- **Mock:** "1 setting is custom · the rest follow Channel defaults". **Refine with AI** ("Suggest
  changes"). Four collapsible blocks:
  - **What plays**: the lineup with READY / ACQUIRING / WANTED and remove, "+ Add a title", Years
    (CUSTOM), Highest rating allowed (DEFAULT).
  - **How it's ordered**: order, with an explanation.
  - **When it changes**: rules like "Saturdays 8 PM–2 AM · marathon <series>", Add a rule,
    Seasonal, Auto-curate.
  - **Preview schedule**: a time → the title and which rule won.
- **Replaces:** `-channel-programming.tsx`. It's built, and the policy shape matches `ChannelPolicy`
  (`scope`, `ordering`, `rules`, `seasonal`, `autoCurate`, `audience`, `operatorSet`).
- **Data:** `/programming/preview`, `/refine`, `/programming/vocabulary`: present. **No gaps.**
- **Critique:**
  - **Major:** "Refine with AI" has no LLM-off state (as in Guide).
  - **Minor:** "Remove" targets measure 23×19 px.
  - **Minor:** "Suggest changes" is the verb, and fine per the Proposal naming rule, but AI setup's
    "models Loomarr uses for **suggestions**" names the artifact. Say "for proposals" or "to
    build channels".

### Filler (per channel, admin)

- **Mock:** "This channel picks its commercials and clips automatically…". **What this channel can
  play** ("412 clips match · about 26 hours before a repeat"). **Choose what fits** (kind chips,
  filters). **Preview break**.
- **Replaces:** `filler/channel-filler`. **Data:** `/filler/coverage`, `/pods/preview`,
  `/v1/filler/fit`: present. **Keep.**

### Danger zone (admin)

- **Mock:** Pause (off air, kept), **Stop updating** (it keeps playing, with no new titles or
  schedule changes; that's `status=detached`), Delete ("and from Tunarr if it's there").
- **Replaces:** `channels/channel-danger-zone`. **Keep.**

---

## Requests

- **Mock:** tabs **Needs you (n) / In progress (n) / Done (n)**. The Needs you count is suggest-pink
  when above zero.
  - **Needs you** (admin): "Waiting for your approval" with select all and bulk **Approve**. Each
    card has title, requester, summary, title chips, "+N more" and meta ("14 titles · 3 to download
    · up to TV-14"), with Edit / Deny / Approve. Filler clip downloads offered by a source appear in
    the same list, "Requested by Loomarr". Below that, "Couldn't be built": the failure with its
    guidance and **Edit and retry**.
  - **In progress / Done:** rows with two thumbnails, a status badge (Downloading 3 of 11 / Waiting
    for an admin / Finding titles / On its channel / Declined + reason), the date and a hint.
  - The member version is the same, scoped to their own requests.
  - Empty: "nothing yet" + Request a channel.
- **Replaces:** `routes/_authed/requests/_tabs.*`, `queue/request-lists`, `ai/request-card`,
  `suggest/approval-queue`. It's built.
- **Data:** `/v1/proposals` (`createdByName`, acquisitions with `officialRating`, `inLibrary`),
  `/v1/proposal-jobs` (failure `code`, `guidance`, `recoveryAction`), `/v1/filler/pulls`
  (approve/dismiss), `/v1/proposals/approve` (bulk): present. **No gaps.**

**Critique**

- **Major: two domains in one queue.** Channel proposals and filler clip downloads sit together
  under "Waiting for your approval". Bulk-approving a mix is easy to do by accident. *Recommend:*
  group by kind, with a bulk approve per group (Q-R1).
- **Minor:** dates are `9/25/2026`. Use the locale and relative times like the rest of the app.
- **Minor:** the disabled bulk Approve label measures 3.18:1. Fine for disabled, but the button
  must also be `disabled` or `aria-disabled`, not just dimmed.
- **Keep:** the three tabs, failure guidance with a recovery action (it comes straight from
  `ProposalJourneyFailureDTO`), and the member scope.

---

## Filler

- **Mock:** header "Filler" and a watch line ("3 of 4 sources on · 1,284 clips · checked 12m ago").
  Tabs Overview / Library (count) / Manage.
  - **Overview:** a status card ("Working automatically / Filler is working on its own", or "Action
    recommended / A few clips need your help"), then **Channel coverage** cards (duration and clip
    count playable, level badge, "N categories · N brands") and Browse library.
  - **Library:** kind chips and a paged clip list.
  - **Manage:** rows for Sources, Incoming, Tags, Settings, Problems.
  - Unconfigured, loading and error states exist.
- **Replaces:** `filler/filler-page`, `filler/filler-overview`, `filler/watch-pill`, and routes
  `filler/{index,library,manage,incoming,sources,splits,taxonomy,advanced,settings}`. Built. The
  deltas are the coverage cards and Manage as a hub.
- **Data:** `/v1/filler/watch`, `/v1/filler/decisions/overview` (`nextAction`), `/v1/filler`,
  `/v1/channels/{id}/filler/coverage` (`level=exact|widened|audience|bumper_card`). The
  all-channel coverage batch and category/brand counts are
  **[#1667](https://github.com/loomarr/loomarr/issues/1667)**.

**Critique**

- **Major: colour meaning is inverted.** The *worst* coverage level, "Bumpers only"
  (`bumper_card`), gets the green **lock** tone, while "Good match" (`exact`) gets amber
  **signal**. Green reads as "all good". *Recommend:* `exact` → lock, `widened`/`audience` →
  caution, `bumper_card` → on-air, and the label wording from the orval enum.
- **Minor:** coverage for every channel costs one request per channel until #1667.
- **Keep:** "working on its own" as the default message, and Manage as a hub.

---

## Settings and This server

- **Mock (Settings home):** **Find a setting** (everyday words, a key, or an env var), grouped
  destination cards (Set up Loomarr · Access and devices · This server · Troubleshoot · Filler), and
  Advanced settings. **This server** is a second home (Run this server, Troubleshoot). Leaves
  render blocks of fields with provenance (env-pinned fields get a darker, read-only input),
  connection verdicts (OK/Failing + a fix hint) and a sticky "N unsaved changes · Discard ·
  Save" bar. Row leaves (Background tasks, Diagnostics) are status rows with Run now / Open.
  Members get Notifications only.
- **Replaces:** `routes/_authed/settings/*`, `settings/settings-destinations`,
  `settings/settings-page`, `settings/settings-context-nav`, `settings/system/*`. Built.
- **Data:** `/v1/settings` (`group`, `label`, `doc`, `envVar`, `provenance=env|db|default`,
  `apply=live|restart`, `secret`, enum options), `/v1/system/services`, `/v1/jobs` (run and pause),
  `/v1/diagnostics/*`, `/v1/system/{backups,database,version}`, `/v1/notifications/*` (Web Push
  exists, #770): present. **No gaps.**

**Critique**

- **Major: prepared media is back.** "Space for prepared shows" (System → Playback), the
  "Prepare upcoming shows" task, "How prepared playback works" (palette) and the dead
  prepared-media Home panel all describe prepared media, **withdrawn in beta.8** (on-demand
  playout, #1512). *Recommend:* **drop** all four.
- **Minor: the restart banner doesn't contradict hot-apply.** Five keys are `apply=restart` today
  (`filler.dir`, `filler.watch_dir`, `filler.structure_window_authority_path`,
  `filler.structure_window_deployment_path`, `diagnostics.dir`). The banner names the key, which is
  why it exists (§9.2). *Recommend:* **keep** it, admin-only. Separately, ask whether those five
  can become hot-apply, which would retire the banner entirely (Q-S1).
- **Minor:** "Your location" appears as a Set-up card *and* inside the Access and devices leaf.
  Pick one home.
- **Minor (a11y, measured):** axe `label` (critical) on Connections: inputs aren't programmatically
  labelled in the mock. Today's `components/loomarr/settings/setting-field` labels them, so keep
  it.
- **Keep:** Find a setting, provenance display, the unsaved-changes bar, Advanced by key.

---

## People

- **Mock:** "Who can sign in to Loomarr, and what they can do". Invite someone. A table of
  person (initials), role badge, **signs in with** (Loomarr / media server), **last seen** and a ⋯
  menu.
- **Replaces:** `routes/_authed/people.tsx`. Built.
- **Data:** `/v1/users` (`role`, `local`, `disabled`, quotas), `/v1/invitations`: present. **Last
  seen** is **[#1667](https://github.com/loomarr/loomarr/issues/1667)**.
- **Keep.** **Minor:** the ⋯ menu's contents aren't shown (role, disable, quota, reset password
  all exist today, so keep today's).

## Your account

- **Mock:** Profile (display name, username), Password ("Changing it signs you out everywhere
  else"), **Where you're signed in** ("Firefox on macOS · <city> · now · THIS DEVICE",
  "Safari on iPhone", "Living room Shield · paired device"), with Sign out per row.
- **Replaces:** `routes/_authed/account.tsx`.
- **Data:** `/v1/auth/me`, `/v1/auth/password`, `/v1/users/{id}/sessions`
  (`createdAt`, `expiresAt`, `current`), `/v1/auth/devices` (`deviceName`, `lastSeenAt`). The
  browser/OS label and last-active time are **[#1667](https://github.com/loomarr/loomarr/issues/1667)**.
- **Critique:**
  - **Major: the per-session city is geo-IP**, a privacy cost for a household app, and wrong
    behind a VPN or tailnet. *Recommend:* **drop** the city.
  - **Keep** the rest.

## Help

- **Mock:** "Ships with Loomarr. No internet needed." Search, a page list and the article.
- **Replaces:** `help/help-page`. **Data:** `/v1/docs`, `/v1/docs/{slug}`. **No gaps.**
- **Minor:** the copy says "When a check on the **Dashboard**…", but the Dashboard is renamed and
  its checks moved. Fix the doc text, not the UI.

## Sign-in, join, forgot and reset password

- **Mock:** a centred card with the wordmark and "always something on". **Login:** username,
  password, Forgot password?, Sign in, Sign in with single sign-on. Error and "Signing in…" states.
  **Join:** "<name> invited you to Loomarr / You'll join as a member", with name, username and
  password. **Forgot/reset** cards.
- **Replaces:** `routes/{login,join,forgot-password,reset-password}.tsx`, `setup/login-shell`,
  `setup/login-form`. Built.
- **Data:** `/v1/auth/*`, `/v1/invitations/preview|redeem`, `/v1/auth/password-recovery/*`,
  SSO: present. **Keep.**

## Pair a TV

- **Mock:** "Type the code your television is showing", name this device, Pair, and "Signed in as
  <you>. The TV gets member access; you can remove it any time in Settings → Sign-in and devices."
  Plus an error state.
- **Replaces:** `routes/pair.tsx`. **Data:** `/v1/auth/device/*`, `/v1/auth/devices`. **Keep.**
- **Major:** "The TV gets member access" is ambiguous. Does the TV act *as the person who paired
  it*, or as a generic member? That decides whose favourites, recents and (if kids exist) ceiling
  it gets (Q-N3).

## Setup wizard

- **Mock:** a left step rail (✓ done, current, "skipped") and a card per step. **Account** (name,
  username, password). **Playback** (Loomarr plays them, recommended, or Tunarr). **Location**
  (country, local market; skippable). **Connections** (media server address, API key, a live
  check "Connected · <server> <version>" or a refusal hint). **Tunarr library** (skippable; only
  if Tunarr). **People** (import media-server users, "4 of 4 selected"). **First channel**
  ("Describe it" → **Build my channel**).
- **Replaces:** `routes/wizard.tsx`, `setup/wizard-shell`. Built.
- **Data:** `/v1/setup/{state,status,test,bootstrap,tunarr-connect}`, `/v1/users/candidates`,
  `/v1/users/import`, `/v1/locations*`: present.

**Critique**

- **Major: the last step depends on the LLM.** "Describe it → Build my channel" fails on an
  install without the LLM, which is the household default. First run ends in an error.
  *Recommend:* offer the §13 starter templates (they already exist in `packages/core/src/templates`
  and were built for this step) and a manual lineup when there's no LLM (Q-G2).
- **Major: every step is a button that jumps.** A user can go to Connections before creating the
  account. *Recommend:* only done steps and the next step are reachable.
- **Minor:** at 390 px, step 4 scrolls horizontally.
- **Keep:** skip-with-marker, the live connection check with a fix hint.

---

## Native

The shared system header on the native board: "Same tokens as Loomarr Web… signal for focus and
the primary action only… Type follows each density's scale: tv on the 960×540 logical canvas,
touch on iPhone, pointer on web." The native apps are **viewer clients**: Watching, Guide, Surf.
There's no Home, Requests or Settings on native.

Shared code today: `web/packages/ui` (`watching-surface`, `guide`, `guide/guide-tv`, `surf-rail`,
`client-navigation`, `client-shell`, `pairing-shell`, `channel-switch-overlay`, `programme-card`),
`web/packages/ui-tv` (focus registry, guide, surf and watching navigation) and
`web/packages/design-system` (tokens, primitives, motion). The web app already bundles
`react-native-web` and renders `@loomarr/ui` in its stories and platform proof, so "shared with
web" can mean **shared code**, not just a shared look.

### React Native design-system gaps (from #1627 and this map)

| Need in the mocks | RN design system today | Needed |
| --- | --- | --- |
| Channel-switch snow (B4, web `TvStatic`) | `FeTurbulence` renders null on react-native-svg 15.15.5 | a seeded, code-drawn snow primitive (#1627) |
| Tracked labels ("FAVORITES · 6", mono readouts, `TUNING IN_`) | `Text` has **no `letterSpacing`** | a tracking option on `Text` (#1627) |
| Halo text over video (the 5a digit entry, B4 readout) | `Text` takes **one shadow** | a two-layer halo option (#1627) |
| Bars without a label | `SignalLoader` always renders its label | label-less bars (#1627) |
| Detail sheet (iPhone 5d: drag handle, docked primary) | none found in `packages/ui` | a BottomSheet primitive |
| Android bottom bar with a pill indicator (5f) | `client-navigation` has one idiom | a platform variant of ClientNavigation |
| Side rail on iPad (5h) | none | a rail variant of ClientNavigation |
| Filter chips (All/Favorites/Recent) | present, **disabled** | needs data ([#1666](https://github.com/loomarr/loomarr/issues/1666)) |

### TV (Android TV / Shield; the frame is also labelled Apple TV)

- **5a Watching:** the typed digits count down to auto-tune ("21_ … auto-tunes in 1.2 s", OK tune
  now, BACK cancel). The bottom chrome shows number, name, Live, title "episode", S/E · year · time,
  progress and "Up next". The hints are ▲▼ tune, ◀ surf, OK guide.
- **5b Guide:** "6 × 48 rows", filters All · 54 / ★ Favorites · 6 / Recent · 4, "21 of 54", a now
  marker, and now-airing cells shown as a tone step with **focus as the only amber**. A detail
  strip at the bottom (thumbnail, title, time, "starts in", year, rating, description) and OK tune.
- **5c Surf:** the shared left rail with Favorites and Recent groups, one focused card with
  progress, hints and "Loomarr TV 0.2.0 · Server 0.2.1" in the rail footer, and the current channel
  bottom-right.
- **Today:** `web/apps/tv/src/app.tsx` + `ui/watching-surface`, `ui/guide/guide-tv`,
  `ui/surf-rail`, `ui-tv/*`. Built. The deltas are the filters (#1666), the digit-entry countdown
  readout, and B4 (#1627).
- **Data:** `/v1/guide`, now-next, play-url and version are present. Favourites and recents are
  **[#1666](https://github.com/loomarr/loomarr/issues/1666)**.

**Critique (10-foot, D-pad)**

- **Keep: the focus model is right.** Focus is the only amber, now-airing is a tone step, and every
  frame shows its hints. That's the correct 10-foot hierarchy.
- **Major: guide paging isn't specified.** What does ◀ do at the first cell (open Surf, or go back
  in time)? And ▶ past the window's edge? Long cells span the whole window, so ▶ from them has
  nowhere to go. How do you reach the filter row: ▲ from the top row? *Recommend:* spell it out
  in `ui-tv/guide-navigation` before the PR (Q-N4).
- **Major: the timed auto-tune.** 1.2 s after the last digit is short for motor-impaired users, and
  WCAG 2.2.1 asks for an adjustable timer. *Recommend:* keep 1.2 s as the default, make it a
  per-device setting, and keep OK to tune at once.
- **Major: overscan and safe area.** The frames look about 48 logical px (5 %) inset, which is
  Android TV's guideline. **Not measured**: verify on the Shield in the TV PR.
- **Major: sizes can't be judged from a web canvas.** Label and hint sizes must be measured on
  device at 1080p (an Android TV body-text floor of about 24 dp at 960×540 logical). **Not measured
  here.**
- **Blocker (for B4 only):** snow and halo text need the #1627 primitives first.

### iPhone

- **5d Guide:** the launch tab. Large title "Guide", a search icon, filters, a compact grid (8 PM–9 PM
  across the screen), and **tap selects a cell**: a detail sheet with a drag handle holds the one
  primary action, "Watch 21 now".
- **5e Watching:** the picture on top with **controls under the picture, not on it** (Previous,
  Channel −, Pause, Channel +), then "Next …", then Favorites rows with progress. The tab bar has
  Watching · Guide · Surf.
- **Today:** `web/apps/mobile/app/index.tsx` renders `ClientShell` + `PairingShell` from
  `@loomarr/ui`. The shell exists; the phone Watching layout, the sheet and the filters are the
  deltas.

**Critique**

- **Settled (Q-N5): controls under the picture.** The 2026-08-07 rule is that player controls
  overlay the frame. The maintainer made **portrait phone** an intended exception: landscape and
  TV still overlay.
- **Major: guide cells at phone width.** In the mock, titles already truncate to a word or a word
  and an ellipsis, and short programmes will be narrower than 44 pt. **Decided (Q-N6):** keep the
  compact grid, so each cell's hit area must still reach 44 pt, with the detail sheet carrying the
  full title.
- **Minor:** the search icon in 5d has no destination defined (a channel filter? the web palette?).

### Android phone

- **5f Guide / 5g Watching:** "same screens as iPhone; Material bottom bar with pill indicator,
  filter chips, system back". The selected programme docks above the nav bar with Watch.
- **Critique:** **Minor:** the docked strip (5f) and the iPhone's sheet (5d) are two components
  for one job. Fine if deliberate (platform idiom), but it doubles the work.

### iPad

- **5h:** landscape 1180×820. The tabs become a side rail, and Watching (player + controls) sits
  left with the full guide, channel names included, on the right.
- **Decided (Q-N1):** iPad runs the **phone layout scaled**, so 5h's split view isn't built, and
  its portrait gap and controls placement are moot.

### Native: shared with web

| Part | Web today | Native today | Share? |
| --- | --- | --- | --- |
| Guide grid | `components/loomarr/guide/guide-grid` | `ui/guide`, `ui/guide/guide-tv` | Two grids exist. The redesign is the moment to decide whether web adopts `ui/guide` through react-native-web (Q-N7) |
| Now/next card with progress | Home WatchingCard, Guide OnNowCard, mini-player | `ui/programme-card`, surf card | one card model, per-density rendering |
| Channel drawer (watch console) | new | `ui/surf-rail` | the same rail model: Favorites, Recent, All |
| Channel identity | the Guide uses **monogram idents** ("LN") | native uses **numbers only** | decided: the monogram ident everywhere (Q-N8) |
| Filters All/Favorites/Recent | the console's star toggles | `GuideFilter`, disabled | one data source (#1666) |

---

## Missing states (not in any mock)

| State | Where | Needed |
| --- | --- | --- |
| Phone and tablet web layout | everywhere | a bottom bar, once the maintainer mocks it (Q-X1) |
| LLM not configured / unreachable | Add a channel, Refine, the wizard's last step, ideas | a design (Q-G2) |
| Missing artwork | every poster, still and channel icon | a code-drawn token fallback |
| Media server not connected (dead air) | Home, Guide, Watch | the dead-air card exists; Home needs its pointer |
| Huge household (60+ channels, 10+ viewers, 1,000 requests) | Guide, Home, Requests | virtualisation and caps |
| Partial failure per section | Home (one section errors, others fine) | a per-section error, not a page error |
| Offline / server restarting | shell | the restart overlay exists; a general "can't reach Loomarr" state doesn't |
| Kids / restricted viewer | Home, Guide, native | not in beta.9 (Q-H6) |
| TV: server unreachable, pairing expired | TV | `ui/device-disconnect` exists and isn't in the mock. Keep today's |

## Accessibility (measured on the web mock)

| Check | Result | Impact on the build |
| --- | --- | --- |
| axe `nested-interactive` (serious) | **45 nodes on 21 of 26 screens**: clickable cards that contain buttons (Watching now, ideas, guide rows with ⋯, request cards) | cards are one link, with actions outside it or using a stretched-link pattern. Never a button inside a button |
| axe `label` (critical) | Connections inputs | real `<label for>` (today's `setting-field` already does it) |
| axe `color-contrast` / `link-in-text-block` | 5 + 4 nodes (Guide, Requests) | tertiary token and inline links need an underline or 3:1 against body text |
| axe `document-title`, `html-has-lang` | every screen | mock-harness artefacts, not design issues |
| Visible keyboard focus | about 40 % of Tab stops had no outline or shadow: **every text input**, plus the image drop-slots (a mock artefact). Border-colour change on focus wasn't measured | inputs get the token focus ring |
| Tab order | nav first, then content in DOM order; no skip link | add "Skip to content" |
| Targets under 24×24 px at 390 px (WCAG 2.5.8) | 1–10 per screen: back arrow 16×16, viewer chips 31×18, Remove 23×19, channel names 150×20, "Different ideas" 104×17 | 24 px minimum hit area, 44 on touch layouts |
| Targets under 44 px at 390 px | 13–42 per screen | the phone layout mock should size for touch |
| Horizontal scroll at 390 px | wizard step 4 | fixed by the phone layout |
| 11–12 px text | 42 % of rendered text | no WCAG failure, but make **12 px the content floor** and keep 11 px for mono badges |

## Re-creates something already decided

| Mock | Standing decision | Recommendation |
| --- | --- | --- |
| Prepared-media space, the prepare task, the help page, the Home prep panel | prepared media **withdrawn** in beta.8 (#1512) | drop |
| Phone controls under the picture (5e, 5g, 5h) | controls overlay the frame (maintainer, 2026-08-07) | decided: portrait phone is an exception; landscape and TV overlay (Q-N5) |
| Encoder/speed telemetry on the viewer watch console | §12 rejected the operator-console channel detail | drop it from viewers |
| Named household viewing shown to all | #186 (viewer identity) was never built; §342 covers the catalogue, not viewing | decided: admins see names, members see counts (Q-H2) |
| "…models Loomarr uses for **suggestions**" | the artifact is a **Proposal** (CONTEXT.md, V41) | reword |
| Watch nav target hard-wired to one channel | the backend is the source; no hard-coded data | last-tuned channel ([#1662](https://github.com/loomarr/loomarr/issues/1662)) or first channel |
| "Open in Jellyfin" as literal copy | the server kind is a setting (Emby or Jellyfin) | read it from settings |
| Member Home | §11: members don't get the machine-state Dashboard | fine *if* Home carries no machine state for members, which the mock honours |

## Critique roll-up

| # | Sev | Screen | Finding |
| --- | --- | --- | --- |
| 1 | blocker | Shell | no phone or narrow layout; content gets about 110 px at 390 |
| 2 | blocker | Home | direction not chosen (web mock vs 1a/1b/1c) |
| 3 | blocker | Home | named household viewing shown to everyone |
| 4 | blocker | iPhone/iPad | controls under the picture vs the overlay rule |
| 5 | major | Shell | nav highlight moves between Watch and Guide for one channel |
| 6 | major | Shell | Watch has no target on a fresh install |
| 7 | major | Shell | the mini-player duplicates the player and may hold a playout session |
| 8 | major | Home | viewer and operator content mixed; the strip repeats the badge |
| 9 | major | Home | no kids/restricted viewer; the API has no such role |
| 10 | major | Home | four sections need API data (#1662–#1665) and an artwork fallback |
| 11 | major | Home | today's playout/services/activity panels have no new home |
| 12 | major | Guide | the mock styles a paused channel as airing |
| 13 | major | Guide | narrow blocks unlabelled; tooltip-only names |
| 14 | major | Guide | no jump or virtualisation for large channel counts |
| 15 | major | Guide | no arrow-key grid model (about 100 Tab stops) |
| 16 | major | Guide, Programming, Wizard | no LLM-off path, and the LLM is off on the household install |
| 17 | major | Watch | console variant shows operator telemetry to viewers |
| 18 | major | Requests | channel proposals and clip downloads in one bulk queue |
| 19 | major | Filler | worst coverage level shown in green |
| 20 | major | Settings | prepared media back after its withdrawal |
| 21 | major | Account | per-session city (geo-IP) |
| 22 | major | Pair | "member access": whose identity the TV takes |
| 23 | major | Wizard | any step reachable out of order |
| 24 | major | a11y | 45 nested-interactive nodes across 21 screens |
| 25 | major | a11y | inputs lack a visible focus ring |
| 26 | major | Tokens | tertiary text 2.69–3.15:1, control outline 2.82:1 on elevated |
| 27 | major | TV | guide edge and paging behaviour unspecified |
| 28 | major | TV | 1.2 s auto-tune not adjustable |
| 29 | major | TV | overscan and sizes not verified on device |
| 30 | major | Phone | guide cells truncate below 44 pt |
| 31 | major | iPad | portrait missing |
| 32 | minor | Shell | ⌘K shown on every platform |
| 33 | minor | Home | first run doesn't separate "no channels" from "nothing connected" |
| 34 | minor | Home | fixed caps (3 viewers, 4 highlights) with no rule |
| 35 | minor | Guide | On now count differs by role (4 vs 6) |
| 36 | minor | Channel | duplicate channel number; small back and Remove targets |
| 37 | minor | Requests | numeric dates; the disabled bulk button needs real `disabled` |
| 38 | minor | Settings | "Your location" in two places; restart keys could hot-apply |
| 39 | minor | Help | copy still says "Dashboard" |
| 40 | minor | Android/iOS | two different programme-detail components (sheet vs docked strip) |

---

## Decisions

The maintainer answered every open question on 2026-09-28
([#1659 comment](https://github.com/loomarr/loomarr/issues/1659#issuecomment-5862244089)).
Conditions the supervisor added are marked *(supervisor)*. Where a recommendation earlier in this
map differs, **the decision wins**. A reference like "(Q-H2)" in the body points to row H2 below. Roll-up rows 2–4, 7, 9, 11, 18, 22, 27 and 31 are settled here.
Row 1 waits for a mock. The rest are build requirements.

| Q | Decision | What it changes in the build |
| --- | --- | --- |
| X1 | Web gets a **bottom bar** at phone width. The supervisor drafts a brief; the maintainer mocks it | **Don't build the phone layout until the mock exists.** Until then the shell keeps today's 56 px icon rail below `md` |
| X2 | Watch = the **last-tuned channel**. Channel management highlights **Guide**. Watch is **hidden** until there's a channel | Needs a per-user last channel (#1662) |
| X3 | **Drop** the mini-player | none |
| H1 | Build **the web mock's Home** (not 1a/1b/1c) | The strip keeps the mock's sentences |
| H2 | **Admins see named viewing, members see counts** | #1662 enforces it server-side. Members still get their own "continue watching" |
| H3 | Tonight's highlights = **premieres + marathons** | #1664 needs only `season_premiere`, `series_premiere` and `marathon`. The mock's "Movie · year" and "New on <channel>" rows aren't highlights |
| H4 | Channel ideas are **members-only**, as mocked | The admin "An idea from your library" card isn't built (confirmed 2026-09-28) |
| H5 | Ideas **may** use the LLM when it's on. *(Supervisor)* they must work fully without it: library facets and the seasonal calendar first, the LLM only enriches | #1665 |
| H6 | **No** kids/restricted role in beta.9 | Home shows members the whole catalogue, per §342 |
| H7 | **Move** the Dashboard panels to This server → Playback / Diagnostics | `PlayoutPanel`, `ServicesPanel` and `ActivityFeed` keep working, on new routes |
| G1 | Design for **100 channels**, with jump-to-number/name and virtualised rows | Guide PR |
| G2 | With the LLM off: **starter templates plus a manual lineup**, with a one-line reason | Guide, Programming and the wizard's last step |
| W1 | **Tab player**, plus the console's **channel drawer** and **0–9 direct tune**; **no encoder readout** | Channel-detail PR |
| R1 | **Group** filler downloads, with bulk approve per group | Requests PR |
| S1 | Make the **five restart-apply keys hot-apply** and retire the banner. *(Supervisor)* a key that truly can't apply live keeps an **admin-only** restart notice; the PR names which | Its own backend PR (see below) |
| N1 | **Android TV, iPhone, Android phone**, plus **Apple TV**. iPad uses the **phone layout scaled**. *(Supervisor)* Apple TV is **beta.10** (devices): `apps/tv` is Android-TV-only and tvOS needs react-native-tvos | No iPad layout (roll-up 31 drops). The iPad split view (5h) isn't built |
| N2 | **Phones can request: native gets Requests** | New PR. **Needs a native Requests mock** (none exists) |
| N3 | A paired TV **acts as the person who paired it** (favourites, recents, ceiling) | #1666 keys favourites and recents to the pairing user |
| N4 | D-pad at the guide edges **as described in the map** | TV PR |
| N5 | **Portrait phone with controls under the picture is an intended exception** to the overlay rule. Landscape and TV still overlay | Phone Watching. Roll-up 4 is settled |
| N6 | The phone guide keeps the **compact grid** | Phone Guide. Cells still need 44 pt targets |
| N7 | Web's guide **moves to the shared `ui/guide`** via react-native-web | The Guide PR becomes a migration (larger; see below) |
| N8 | Channel identity is the **monogram ident** | Native adopts the ident: `ui/guide`, the surf rail and the watching chrome |

**Required regardless:** fix the measured WCAG failures (tertiary text and outline contrast, the
45 nested-interactive nodes) **as part of the build, not as a follow-up**.

---

## PR sequence

Each screen PR brings demo-library screenshots next to the mock for approval, axe and keyboard
checks, and CI visual baselines. Sizes are rough (S ≤ 300 lines, M ≤ 800, L > 800, excluding
generated files). "Mock?" says whether a new mock has to exist first.

| # | PR | Depends on | Mock? | Size |
| --- | --- | --- | --- | --- |
| 0 | The map and these decisions | none | no | S |
| 1 | **Tokens and a11y floor**: `border-control` to 3:1 on every surface a control sits on (today 2.82:1 on `static-800`), plus a skip link. The other two mock findings aren't defects in today's app. Its inputs already have a `focus-visible` ring, and `static-500` is documented disabled-only and used only decoratively. Screen PRs keep it off information text, and axe on stories enforces that | none | no | S, with a baseline diff |
| 2 | **Restart-apply keys hot-apply** (S1): the five keys apply live, the banner retires, any key that can't stays behind an admin-only notice | none | no | S–M (backend) |
| 3 | **Shared web primitives**: SectionHeader, StatusStrip, list row (dot, progress, action), WatchingCard/OnNowCard (one card), PosterRail, HighlightRow, IdeaCard, count tabs through `NavTabs`, a stretched-link card (fixes nested-interactive), the artwork fallback | 1 | no | M |
| 4 | **Shell**: Dashboard → Home, Watch (last-tuned, hidden until a channel), member Home route, Ctrl-K label, no mini-player | 3, #1662 (last channel) | no | M |
| 5 | **This server**: move the playout, services and activity panels (H7); drop prepared media; dedupe location | 4 | no | M |
| 6 | **Home** (the web mock's), sections behind their data; sections without their API ship hidden, not faked | 3, 4, #1662–#1665 | no | L |
| 7 | **Guide on `ui/guide`** (N7): monogram idents, On now cards, span picker, block focus and names, arrow keys, jump-to-channel, virtualised 100 rows, paused styling, the LLM-off describe panel | 3 | no | L (migration) |
| 8 | **Channel detail**: Watch drawer and 0–9 direct tune, Info, Programming LLM-off, Filler, Danger | 7 | no | M |
| 9 | **Requests (web)**: grouped approvals with per-group bulk approve, locale dates, the disabled bulk button | 3 | no | S–M |
| 10 | **Filler**: coverage cards with corrected tones, Manage hub | 3, #1667 | no | S–M |
| 11 | **People + Account**: last seen, session labels, no city | #1667 | no | S |
| 12 | **Help, sign-in, pair, wizard**: copy fixes, wizard step gating, starter templates on the last step | none | no | S–M |
| 13 | **RN design-system primitives**: Text tracking and halo, snow, label-less bars (#1627), BottomSheet, the Material ClientNavigation variant | none | no | M |
| 14 | **Favourites/recents** across TV, phone and web, keyed to the pairing user (N3) | #1666 | no | M |
| 15 | **TV** deltas: digit-entry readout, guide edges (N4), adjustable auto-tune, monogram idents | 13, 14 | no | M |
| 16 | **Phone** Watching (controls under the picture in portrait, overlay in landscape) and the compact Guide with sheet / docked strip; iPad runs it scaled | 13, 14 | no | M–L |
| 17 | **Native Requests** (N2) | 16, **a native Requests mock** | **yes** | M |
| 18 | **Web phone layout** (X1): bottom bar | **the maintainer's mock** | **yes** | M |
| — | **Apple TV** (N1): react-native-tvos target for `apps/tv` | **beta.10**, not this milestone | n/a | L |

Rough total: 18 PRs in beta.9, around 9–12k changed lines in the clients, plus the six API issues
sized by their own lanes. The critical path is **#1662–#1665 → Home**. PRs 1, 2, 3, 9, 12 and 13
can start now. PRs 17 and 18 wait for their mocks.
