# Filler — normal/exception hierarchy, library clip geometry, management return paths — design review candidate

Review checkpoint for [#1817](https://github.com/loomarr/loomarr/issues/1817) item 7 ("Filler
normal/exception hierarchy, library clip geometry and management return paths") and parent
[#1659](https://github.com/loomarr/loomarr/issues/1659). This is a proposal, not an approved
behaviour change. No production component, route or API contract changes.

Open [the interactive prototype](../prototypes/filler-hierarchy.html) directly in a browser (no
server, no external URLs). The review bar switches **Variant** (Built today / Draft), **Role**
(admin/member), **State** (normal, attention, failure, empty, unconfigured, loading, error),
**Screen** (Overview, Library, Manage, Incoming, Sources, Tags, Settings, split review, and a
return-path map), **Library view** (grid/list), the draft's **Phone grid** alternative, and
**Frame** (desktop / 390×844 with the phone bottom bar). Buttons, tabs, back links and sheets
navigate inside the prototype and the status line under the frame names the route and any redirect.
Every fixture is invented: clip names, brands, channel names, collection ids and hashes are made
up, and the thumbnails are code-drawn gradients.

**Built today** reproduces main at `1d8b8b717`. **Draft** draws only the deltas, each one in a
pink dashed box tagged "draft". Text in cyan marks behaviour that depends on beta.11 filler scope.

## Scope against what exists today

- The filler pull **producer** (`POST /v1/filler/pulls`, `FillerPullPlanner`) was removed in
  #1846. List/approve/dismiss stay, so any pending "Requested by Loomarr" pull is a legacy row, and
  it lives in Requests (`pull-card.tsx`), not in the Filler area. This draft adds **no** pull
  surface to Filler.
- Coverage-driven acquisition (#749) and general filler certification are beta.11 per
  `project/roadmap.md`. The Overview's coverage cards and the `improve_channel_coverage` next
  action only *report* today. Their one action, "Browse library", is the whole of what exists.
  The prototype flags this in cyan rather than drawing an acquisition flow.
- The brief's "quarantined clips" has **no user-facing concept** in the web app or the API.
  "Quarantine" appears only inside the download/transcode pipeline (`internal/clipfetch/
  clipfetch.go`, `internal/filler/stage_transcode.go`, `acquisitionmanifest.go`) as a private
  staging location. The closest user-visible exceptions are clips **held** for a choice in
  Incoming (`ClipDTO.held`, `FillerWatchOutputBody.held`) and **diagnostics** in Manage → Needs
  attention (`recovery.mode`: automatic_retry / manual_retry / inspection / configuration). The
  draft treats those two as the exception set. See open question 1.

## Sourced from code (Built today)

Page shell and header
- `routes/_authed/filler/{index,library,manage,incoming,taxonomy}.tsx`, `settings/{route,index,$section}.tsx`,
  `sources/{route,index,$sourceId}.tsx`, `splits/$proposalId.tsx`, `advanced.tsx` (redirects to
  `/filler/manage`). Members are redirected by `beforeLoad`: Incoming → Library, Sources →
  Library, Settings → Manage.
- `filler/filler-page/filler-page.tsx`: the `PageHeader` title/description, the watch line
  ("3 of 4 sources on · 1,284 clips · 3 need you · checked 12m ago"), NavTabs Overview / Library
  (count) / Manage, and Manage lit for Sources, Incoming, Tags and Settings (lines 116-135).
  Loading and "No filler folder configured" states (61-93). The catalog-paused card on Library
  (139-160). Page padding `p-6` at every width.
- `components/loomarr/filler/watch-line/watch-line.tsx`: dot is lock green (pulsing) when
  healthy, caution when attention, static-500 when unconfigured, with an sr-only sentence.

Overview
- `filler/filler-overview/filler-overview.tsx`: `readinessAction` copy is reproduced verbatim for
  `review_incoming`, `retry_failed_work` and `add_filler`. The status card uses
  `border-signal/35` + a **signal (amber)** "Working automatically" badge when ready, and
  `border-caution/40` + a caution "Action recommended" badge otherwise (158-183).
  `COVERAGE_BADGE` (130-136) already carries the redesign map's corrected tones (exact → lock,
  widened/audience → caution, bumper_card → onair), so map critique #19 is fixed on main.
- `internal/api/filler.go:49-54`: `filler-readiness` is registered `RoleMember`, and the overview
  renders the action button without a role check.

Library
- `filler/filler-catalog/filler-catalog.tsx`: the filter card (`grid-cols-2` below `sm`, search
  spanning both, Kind, Audience, "Untagged only", Grid/List radiogroup with labels hidden below
  `sm`), the count line + "Select this page", the bulk bar ("Remove from catalog"), the
  composite "Back to top-level catalog" bar, and the pager (515-830). The grid is
  `grid-cols-1 sm:grid-cols-2 lg:grid-cols-3` (789).
- `components/loomarr/filler/pool-health/pool-health.tsx`: Clips / Fits a break / Channels
  covered / Weakest stats, with the "falls back to a looser match" note.
- `components/loomarr/filler/clip-card/clip-card.tsx`: `Card p-3 gap-2.5`, a full-bleed
  `aspect-video` frame with duration, quality and a select checkbox on scrims. The status chip
  is **"Ready" in signal amber** on every playable clip (287-289). Then kind/era/audience/category
  chips and the "aired" line (`playsLine`).
- `components/loomarr/filler/clip-row/clip-row.tsx`: grid `auto 54px 1fr auto` below `md`,
  adding kind/era at `md` and audience/plays at `lg` (`… 88px 60px 84px auto 190px`).

Manage, Incoming, Sources, Tags, Settings, split review
- `filler/filler-manage/filler-manage.tsx`: hub rows Sources / Incoming / Tags / Settings /
  Problems, with StatusDot tones (Problems uses `notice` = brand amber, 246). Recent activity
  badges (`activityPresentation`: admits are `signal` amber, 32-42). The Needs attention
  disclosure opens only from the Problems row or a `#diagnostics` hash (290-293), and recovery
  copy per `recovery.action` (44-77).
- `filler/incoming/incoming.tsx`: summary headline, Getting clips ready / Needs your help (one
  choice at a time, Previous/Next) / Ready groups, and the empty "Nothing on the way" state.
  **No link back to Manage.**
- `filler/sources-panel/sources-panel.tsx` + `components/loomarr/filler/filler-sources/filler-sources.tsx`:
  Storage card, "Where filler comes from", Your files, provider sections with switches, source
  rows with readiness badges, and the right-hand source sheet. **No link back to Manage.**
- `filler/taxonomy-tab/taxonomy-tab.tsx`: "What belongs here" and Classification coverage across
  the five axes (abbreviated in the prototype; the vocabulary editor is not redrawn). **No link
  back to Manage.**
- `filler/filler-settings/filler-settings.tsx`: the index has a **"← Manage" link** (129-135) and
  each section has "← All filler settings" plus a task switcher (88-124). Labels come from
  `filler-settings-section.ts`.
- `filler/split-review-page/split-review-page.tsx`: a sibling route with no Filler header or
  tabs. Back → `/filler/library` always (183). Keep → Library (95). Recheck → Incoming (105).
- Shell: `phone-bottom-bar` (Alt A: Home, Watch, Guide, Requests + More, 49px row) lights **no**
  tab on an overflow page such as Filler (`phone-bottom-bar.tsx:16-20, 179-184`).
- Tokens: `web/packages/tokens/generated/theme.css`. Badge geometry (mono, uppercase, 11px,
  radius 4, 15% tint) is from `components/ui/badge/badge.tsx`.

## Measured geometry (prototype, pinned container)

Measured in `mcr.microsoft.com/playwright:v1.62.1-noble` (the image in
`scripts/run-playwright-container.sh`) by loading the prototype and reading bounding boxes. This
measures the reproduction, not the live app.

| Frame, variant | First clip top | Card size | Columns | Clips fully above the fold |
| --- | --- | --- | --- | --- |
| Desktop, built | 493px | 322×333 | 3 | n/a (frame not height-bounded) |
| Desktop, draft | 493px | 322×307 | 3 | n/a |
| 390×844, built | **740px** | 340×318 | 1 | **0** (fold at 772px: 844 − 49 bar − 20 safe area) |
| 390×844, draft A (2-up compact) | 445px | 172×130 | 2 | 4 |

No horizontal overflow in either frame. The smallest interactive target is the 16–18px select
checkbox (`size-4`), in both Built and Draft. That is below the 24px WCAG 2.2 minimum, a built
issue this draft does not fix (open question 7).

## Chosen here (Draft — not sourced, needs approval)

Normal vs exception hierarchy. The principle: amber is the brand's *action* colour, so a state
where nothing needs doing should not wear it. Green or nothing means normal, caution means
exception, and onair stays reserved for "nothing to play".
1. **Quiet normal.** The ready Overview card drops the amber border and amber "Working
   automatically" badge for a plain card with a lock-green dot. That agrees with the watch line,
   which is already green when healthy, where today the header says green and the card says
   amber. The exception card keeps the built caution treatment and the readiness copy unchanged.
2. **No "Ready" chip on clip cards.** A clip with no status chip is ready. Chips appear only for
   exceptions ("Preparing", "Recording", and a caution "Needs details" on an untagged commercial,
   which today shows only through the "Untagged only" filter).
3. **Routine activity goes neutral.** "Added automatically" / "Added after review" lose the amber
   tint, and only "Needs a decision"-type kinds stay caution.
4. **Problems row tone = caution**, not amber `notice`. It is the same `retry_failed_work`
   condition the Overview already shows as caution.
5. **Members see the state but no button.** Today a member sees "Review clips" / "Open
   diagnostics" / "Open sources" and is silently redirected to Library or Manage. The draft shows
   the text plus "An admin can sort this out from Filler › Manage."

Library geometry
6. **Phone (390): 2-up compact grid (alternative A, recommended).** 172px cards: frame, duration
   overlay, one-line name, exception chip only. Kind/era/audience chips, brand and the aired line
   move to the details sheet. Alternative B (list rows by default on phone) is selectable in the
   review bar.
7. **Phone: fold the stats and filters.** PoolHealth becomes one line ("1,284 clips · 2/3
   channels covered" + Details), and the filter card becomes search + a Filters button + the view
   toggle. Together with 6, the first clip moves from 740px (below the fold) to 445px.
8. **Phone gutter 16px**, matching the approved phone-web-nav page gutter, instead of the built
   `p-6` 24px at every width. Desktop is unchanged.

Return paths
9. **"← Manage" on every Manage child.** Incoming, Sources and Tags reuse the existing
   Settings-index back link (`filler-settings.tsx:129-135`) at the top of the page. The lit Manage
   tab stays.
10. **Overview's "Open diagnostics" deep-links `/filler/manage#diagnostics`**, so the Needs
    attention panel arrives open. The hash handling already exists in `FillerManage`; only the
    link target changes.
11. **Split review's Back returns to where review was opened** (Incoming's "Needs your help"
    card, or a Library card's "Split into clips"), drawn as a "← Incoming" / "← Library" link
    above the title. The mechanism is open (question 4).

Unchanged and deliberately not redrawn: the coverage badge tones (fixed on main), the
clip-details and source sheets, the composite "Back to top-level catalog" bar, the settings
section switcher, the split cut editor, the taxonomy editor, and per-channel filler
(`/channels/$id/filler`, its own #1817 row).

## Open questions for the maintainer

1. **"Quarantined clips"** doesn't exist as a user-visible state (see Scope). Is held-in-Incoming
   plus Needs attention diagnostics the right exception set, or did you mean something new, such
   as a "set aside" bucket for clips Loomarr rejected that an admin could restore? That would need
   backend work (`PipelineOverviewDTO.rejected`/`dismissed` are counts with no list surface) and
   probably belongs with beta.11.
2. **Amber for "normal"**: approve dropping signal amber from the ready card, clip "Ready" chips
   and routine activity badges (choices 1–3)? The alternative is to keep amber and only add the
   green dot. Recommendation: drop it, because otherwise every card in a healthy 1,284-clip
   library carries the brand accent.
3. **Phone library geometry**: A (2-up compact, recommended), B (list rows default), or keep the
   built 1-column full card? A keeps the frame, which is how commercials are recognised. B is
   denser but hides the thumbnail at 54×30.
4. **Split review origin**: should Back return to the opener? Options: (a) a `from=incoming|library`
   search param on `/filler/splits/$id` (recommended, small and URL-visible); (b) `history.back()`
   (breaks on a deep link); (c) keep Library always. `IncomingHelpDTO.actionHref` is server-built,
   so (a) means the server appends `from=incoming`.
5. **"← Manage" link vs the tab alone**: is the lit Manage tab enough of a return for Incoming,
   Sources and Tags, or do you want the explicit link (choice 9, recommended for phone, where the
   tabs scroll away with the page)?
6. **Coverage card → channel filler page** leaves the Filler area with only browser Back to
   return. Should the channel filler page offer "← Filler overview" when entered from here, or is
   it a normal cross-area link?
7. **Select checkbox target (16px)** on cards and rows is below 24px. Fix it in the build PR for
   this item, or file separately?
8. **Member experience of admin-only next actions** (choice 5): text only, or hide the status card
   entirely for members and show coverage alone?

## Verification

- `node --check` on the prototype's extracted script: no syntax errors. `grep -c http` on the
  prototype: 0, so there are no external URLs.
- Loaded in the pinned Playwright container (same image as `scripts/run-playwright-container.sh`,
  run under `flock /tmp/loomarr-heavy.lock`): 7 screens × built/draft × desktop/390 captured with
  **no page or console errors**, and geometry as in the table above. The screenshots were review
  aids only and are not committed.
- Not done: a live check against a lane backend. The Built column is read from code, not
  observed in the running app. The build PR should verify each delta in the real app at desktop
  and 390.
