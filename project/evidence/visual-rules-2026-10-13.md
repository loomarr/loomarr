# Accent, type, width and card-vs-row rules — design review candidate

Review checkpoint for [#1817](https://github.com/loomarr/loomarr/issues/1817) item 9 ("Accent/focus/
selection semantics, typography hierarchy, content widths and card-vs-row rules across approved
surfaces") and parent [#1659](https://github.com/loomarr/loomarr/issues/1659). This is a proposal,
not an approved behaviour change. No production component, token, route or API contract changes.

Open [the rules sheet](../prototypes/visual-rules.html) directly in a browser. It has one section
per rule. Each section shows the built code beside the proposed rule, followed by the call sites the
rule would change. The review bar filters by rule and switches between built only, proposed only
and side by side. It also swaps the two alternatives for open questions 1 and 3, and narrows the
sheet to 390 px. All fixtures are invented, and nothing calls a server.

## What this covers and why

The September review asked for this directly: "Keep the identity; clarify primary action, actual
focus, selection and warning semantics. Prioritise readable titles, reserve small mono text for
secondary information, and choose width and containers by task"
([design-review-2026-09-30.md](design-review-2026-09-30.md), Visual system). The other eight #1817
items were each mocked and built on their own. This item audits what those builds actually render
and proposes one rule set for all of them. Audited, on `main` at `1d8b8b717`:

- Home [#1843](https://github.com/loomarr/loomarr/pull/1843)
- phone bottom bar [#1849](https://github.com/loomarr/loomarr/pull/1849)
- Watch nav entry [#1873](https://github.com/loomarr/loomarr/pull/1873), and the channel header and
  drawer in `channels/$id/route.tsx` and `channel-watch.tsx`
- pairing [#1874](https://github.com/loomarr/loomarr/pull/1874)
- manual channel [#1878](https://github.com/loomarr/loomarr/pull/1878)
- Programming [#1889](https://github.com/loomarr/loomarr/pull/1889)
- Requests [#1904](https://github.com/loomarr/loomarr/pull/1904)
- TV [#1886](https://github.com/loomarr/loomarr/pull/1886)
- the tokens: `web/packages/design-system/src/tokens/tokens.ts` (RN and shared components) and
  `web/packages/tokens/generated/theme.css` (web Tailwind)

## Sourced from code and docs (the audit)

### Amber today

There is one amber, `#FFB020`. It is `brand.signal`, `action.primary` and `action.focus`
(`tokens.ts:44-46,152`). On web it is also `--primary` and `--ring` (`apps/web/src/styles.css`). In
the light theme, primary (`#795000`) and focus (`#8A5700`) are separate stops (`tokens.ts:118-120`).
It also feeds `border.airing` and `stateSurface.airing` (`tokens.ts:26,41`) and the guide's break
and clip fills (`:70-71`). The palette doc gives it "Brand & primary actions, focus ring, active
nav" (`docs/frontend-design.md` §2.1). The native board says "signal for focus and the primary
action only", and TV 5b says "focus is the only amber" (redesign map § Native, § TV Keep).

What the built code actually uses amber for:

| Meaning | Call sites |
| --- | --- |
| Focus | web `--ring` on every primitive; `Action` focused border (`interaction.tsx:214-221`); `FocusSurface` (`primitives.tsx:103-116`); TabBar focus (`tab-bar.tsx:100`); skip link (`app-shell.tsx:140`) |
| Primary action | `Button` default (`button.tsx:54`); `Action tone="primary"` (`interaction.tsx:136`); TV Play/Retry (`watching-surface.tsx:485,495`); bulk Approve (`approval-bulk-group.tsx:88`) |
| Selection / location | desktop nav (`app-shell.tsx:179-180`); NavTabs pill and underline (`nav-tabs.tsx:78,85,109`); Guide span, View and NOW (`guide-page.tsx:465,504,518,525`); Watch drawer button and tuned row (`channel-watch.tsx:458`, `channel-drawer.tsx:167-176`); TV surf card (`surf-channel-card.tsx:19-21,36`); TV filter (`guide-tv.tsx:60-67`); `Action` selected (`interaction.tsx:140-146`); auto-tune current value (`auto-tune-setting.tsx:37-39`); overlay (`overlay.tsx:46-48`) |
| Identity text | channel numbers on Home (`on-now.tsx:60,121`, `highlight-row.tsx:34`, `recently-tuned.tsx:29`, `new-this-week.tsx:69`); the RN role default for `channelNumber`/`cardChannelNumber`/`guideNumber`/`code` (`primitives.tsx:190,231-234`) |
| Link / hover | "Watch live →" (`on-now.tsx:133`); Home footer link (`home-page.tsx:122`); hover borders (`new-this-week.tsx:63`, `channel-icon-field.tsx:229`) |
| Status | Custom badge (`policy-field-source.tsx:21` → `badge.tsx:27`); unsaved-draft banner (`-channel-programming.tsx:224`); now-airing tint (`guide-tv.tsx:300-311`); `ProgressTrack` default fill (`primitives.tsx:483,493`, used by `surf-channel-card.tsx:68-72` and `watching-surface.tsx:408-413`) |
| Warning | refused titles (`approval-queue-item.tsx:170-172`); safety icons (`channel-cycle-preview.tsx:192,230`) |
| Brand moment | colour bars, the tuner lock (`tuner-loader.tsx`), the direct-tune digits (`channel-watch.tsx:393`), the pairing code |

Two inconsistencies stand out. The phone bottom bar already draws "current page" neutral and keeps
amber for focus (`tab-bar.tsx:98-100`), while the desktop rail draws the same state in amber. The
channel header's own "CH 07" is foreground (`route.tsx:259`), while Home's channel numbers are
amber.

**The live violation, #1887** (confirmed). `selected` in `surf-rail-tv.tsx:109` follows channel
focus only (`:118`). The rail actions (`:127-135`) never clear it, so the card keeps its 3 px
`$actionFocus` border while the focused action draws its own. The web drawer has the same pattern:
the tuned row is amber and the focus ring is amber too.

### Type today

Web's Tailwind scale is 11/12/13/14/16/20/24/32 (`theme.css`). The design-system pointer roles
used by shared components on web put body at 16, label at 14/600, metadata at 12/500 mono and
title at 24/650 (`tokens.ts:213-246`). Both scales render on the same Requests page:
`approval-bulk-group.tsx` uses DS roles, and `request-card.tsx` uses Tailwind.

| Role | Sizes in use |
| --- | --- |
| Page title | 24/600 (`page-header.tsx:16`); 32 (`channels/$id/route.tsx:233`); 30, off-scale (`pair-device.tsx:30,102`) |
| Status line | 32 (`home-strip.tsx:32,41`), larger than the 24 px Home title above it |
| Section | 20/600 (`section-header.tsx:14`, `request-lists.tsx:166,174`, `-channel-programming.tsx:113`); 24 (`proposal-review.tsx:163,167`); 20/500 (`manual-channel-builder.tsx:119`) |
| Eyebrow | 11 mono (`channel-breaks.tsx:34`, `channel-upcoming.tsx:24`); 12 mono (`refine-review.tsx:107,235`) |
| Item title | 13/500 (`list-row.tsx:33`); 13/600 (`on-now.tsx:126`); 14/600 (`recently-tuned.tsx:34`, `channel-ideas.tsx:44`); 14/500 (`request-card.tsx:38`) |
| Under 11 px | `live-indicator.tsx:38` (10, on Watch). Outside the approved list: `segment-filmstrip.tsx:126`, `segment-preview.tsx:124,139`, `split-review-editor.tsx:381`, `tasks-page.tsx:262` |
| Arbitrary 13 px | `text-[13px]` 16 times, the same value as `text-sm` |

TV type comes entirely from `typography.tv` roles through `textRole`. It needs no change.

### Widths today

| Route | Width |
| --- | --- |
| Guide | full (`docs/design/web-ui.md`) |
| Home | 1120 (`home-page.tsx:81`) |
| Channel detail, every tab | 896 (`route.tsx:289`) |
| Requests | 768, left-aligned (`request-lists.tsx:120,163`) |
| Settings | 1024 (`settings/index.tsx:98`) |
| Account | 672 (`account.tsx:91`) |
| Pair | 512 (`pair-device.tsx:29,101`) |
| The Guide's create panel | unbounded, with a 28 px gutter (`guide-page.tsx:402`); the contract says 24 |
| TV | surf rail 420 (`surf-rail-tv.tsx:77-90`); programme card 760; overlay 880 |

### Cards vs rows today

On Home, a playing channel is a picture card that links to Watch, and a channel with nothing
playing is a status row (`on-now.tsx:47-137`). On the way, Tonight and Your requests are rows. A
request is a `Card` on `/requests` (`request-card.tsx:31`) but a `ListRow` on Home. Recently tuned
is a card with no picture slot (`recently-tuned.tsx:25`). On TV, every surf channel is a card whose
border shows only when it is selected, and guide channels are rows.

The TV call sites were collected by a child agent (read-only, Sonnet) and spot-checked in this lane
(`surf-channel-card.tsx:15-40`, `interaction.tsx:134-147`, `guide-tv.tsx:298-312`,
`surf-rail-tv.tsx:105-120`).

## Chosen here (the draft's proposal, not sourced)

1. **Accent.** Amber means focus (a ring or border) or the primary action (a solid fill, at most
   one per view). The brand moments listed above keep it too. Selection becomes neutral: elevated
   fill, foreground text and weight, with `aria-current`/`aria-pressed` unchanged. Identity numbers
   are foreground at rest. Links are `tune`, as the palette doc already says, and hover is
   `static-500`. Warnings are `caution`, unsaved and info are `tune`, and elapsed progress is
   foreground. Download progress stays `tune`, as `list-row.tsx:38` already has it.
2. **Type.** Each role gets one size from the existing scale:
   - page title 24/600
   - focused-flow title 32/600 (the top step, replacing the off-scale 30)
   - status line 20/600
   - section 20/600
   - subsection 14/600
   - eyebrow 11 mono
   - item title 14/600 on cards, 14/500 on rows
   - body 14
   - meta 12
   - data 12–13 mono
   - badge 11 mono

   Nothing renders under 11 px, and content stays at 12 px or above (the redesign map's own
   floor). `text-[13px]` becomes `text-sm`, with no visual change.
3. **Widths.** The six values already in use become named tiers: browse (full), overview (1120),
   detail (896, the Watch player included), reading (768), form (672) and focused (512). Every
   tier is centred, with a 24 px gutter and 16 px below `sm`. Settings (1024) is noted and left
   alone.
4. **Card or row.** A card is one picture slot and one destination, in a grid, with no nested
   controls. A decision card is one item with several actions and consequence text. A row is a
   scannable item of one kind with at most one progress readout or one trailing action, inside a
   `ListGroup`. A panel holds a form or a group of rows, and is never a link. The same entity takes
   the same shape at the same density.

## Decisions

On 2026-10-13 the maintainer accepted every recommendation below (recorded on #1909).
- **Accent:** amber is for focus, the primary action, and brand moments only. This covers Q0 (it fixes #1887), Q2, Q4 and Q5.
- **Q1:** the desktop rail and tabs mark the current page in neutral, matching the phone. This overturns the amber pill picked on 2026-08-02.
- **Q3:** now-airing is shown as a neutral tone step, with progress fills in foreground (elapsed) or tune (downloads).
- **Q6 to Q12:** one size per role, the six width tiers, and the card-vs-row criterion, all as proposed.

The build is split into three PRs: design-system and TV; web accent; web type, widths and shapes.

## Open questions (each changes an approved screen; none is decided here)

0. **TV selection vs focus (#1887 and its siblings).** Should a selected item that does not have
   focus fall back to its at-rest look, with amber only on the focused element? That covers
   `surf-channel-card.tsx:19-21,36`, `surf-rail-tv.tsx:109,118`, `interaction.tsx:140-146` (every
   RN `Action`), `auto-tune-setting.tsx:37-39` (#1886's picker), `guide-tv.tsx:60-67` and
   `overlay.tsx:46-48`. The surf rail would also become rows at rest, with the focused one
   expanding into TV 5c's "one focused card". *Recommendation: yes.* It is the approved TV rule
   and #1887's own expected fix.
1. **Where-you-are on desktop.** (A) Make the rail's current item and NavTabs' active pill and
   underline neutral, matching the phone bar. (B) Keep the amber tint as a named exception,
   because the pill was the maintainer's pick on 2026-08-02 (`nav-tabs.tsx:23`). *Recommendation:
   A.* Today the same state is amber on desktop and neutral on a phone, and on desktop it draws two
   ambers whenever keyboard focus is elsewhere. Toggle Q1 in the sheet to compare.
2. **Pressed toggles and the tuned row on web.** Should the Guide's span, View and NOW
   (`guide-page.tsx:465,504,518,525`), the Watch drawer button (`channel-watch.tsx:458`) and the
   drawer's tuned row (`channel-drawer.tsx:167-176`) become neutral-pressed? *Recommendation: yes.*
   This is the web form of question 0.
3. **Now-airing.** The web mock's guide tints airing blocks amber, and the built TV guide does too
   (`border.airing`/`stateSurface.airing`, `guide-tv.tsx:300-311`). TV 5b calls now-airing "a tone
   step" with focus as the only amber. (A) Use a neutral tone step. (B) Keep the amber tint as the
   one status allowed to borrow amber. Either way, the default `ProgressTrack` fill
   (`primitives.tsx:483,493`) must be decided at the same time. *Recommendation: A*, because 5b is
   the more specific approved reference. Toggle Q3 in the sheet to compare.
4. **Channel numbers, "Watch live →", links and hover on Home (#1843).** Should these move off
   amber, and should the RN number roles default to foreground (`primitives.tsx:190,231-234`, a
   shared output that needs its own PR and claim)? *Recommendation: yes.* The channel header
   already draws the number in foreground.
5. **Status and warning amber on Programming (#1889) and Requests (#1904).** The proposal is: a
   neutral Custom badge, a `tune` unsaved-draft banner, and `caution` for "won't be included".
   *Recommendation: yes.* The palette doc already gives caution to warnings and tune to info.
6. **Home's type (#1843).** Should the status line drop from 32 to 20, below the page title, and
   should item titles go up from 13 to 14 (`list-row.tsx:33`, `on-now.tsx:126`,
   `highlight-row.tsx:35`)? *Recommendation: yes.* The design review asked for readable titles.
7. **The channel header's title (Watch header) and the 10 px live indicator.** Should the title go
   from 32 to 24? The page-edge contract already says entity headers "keep the same … title
   scale". Should `live-indicator.tsx:38` go from 10 to 11? *Recommendation: yes to both.*
8. **Focused-flow and creation type (#1874, #1878, #1889).** Should pairing's title (30, off-scale)
   go to 32 and its body from 20 to 16? Should the create panel's h2 go from 24 to 20, the manual
   builder's heading weight from 500 to 600, and the refine eyebrow from 12 to 11?
   *Recommendation: yes*, but keep pairing's larger body if the maintainer wants it to read at arm's
   length.
9. **Which scale wins on web.** Should the design-system pointer roles (body 16, compact 13, label
   14) be re-pointed at the web scale (body 14), so shared components on web match the page around
   them, or should the web move to body 16? *Recommendation: re-point the pointer roles.* That
   changes only RN-on-web text and needs the `tokens` claim. Moving web to 16 reflows every page.
10. **Requests width and the create panel width (#1904, #1878).** Should Requests be centred at
    768, and the Guide's create panel capped at 672 with a 24 px gutter? *Recommendation: yes.* The
    create panel's own failure card is already 672 and centred.
11. **Requests In progress / Done as rows.** Should `RequestCard` become a `ListRow`, matching
    Home's Your requests? The map describes the mock as "rows with two thumbnails". The thumbnails
    wait for artwork data. *Recommendation: yes.*
12. **Recently tuned.** It is a card with no picture slot. Should it stay a single hero card as an
    exception, or gain the artwork fallback? *Recommendation:* add the fallback, so the criterion
    stays one sentence.

## Not covered here

- The light theme. It has no mock, but the rule carries over: primary and focus are already
  separate stops there.
- Settings, Filler, People, Account and Help, which are outside the approved list. Only their
  sub-11 px sizes are noted.
- iPhone and Android phone native screens. They take the design-system roles, so question 9
  covers them.
- Any token value change. Every proposed size, colour and width already exists.

## Evidence

- Code read at `1d8b8b717` (`main`). File:line references are in the sheet and the tables above.
- Docs: `docs/frontend-design.md` §2.1–2.3 and the page-edge contract; `docs/design/web-ui.md`
  (Guide full width); `project/redesign-map-2026-09.md` (§ Native, § TV, Accessibility);
  `project/evidence/design-review-2026-09-30.md`.
- Issues: [#1887](https://github.com/loomarr/loomarr/issues/1887) (the confirmed live violation)
  and [#1817](https://github.com/loomarr/loomarr/issues/1817) (the review method).
- The sheet was rendered with Playwright's bundled headless Chromium at 1440 and 390 px and
  checked by eye. No app backend was needed, because the sheet is static.
