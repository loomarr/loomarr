# Native Requests design review candidate

Review checkpoint for decision [N2](https://github.com/loomarr/loomarr/issues/1659#issuecomment-5862244089)
("Phones can request: native gets Requests") and parent
[#1659](https://github.com/loomarr/loomarr/issues/1659). N2 says this PR needs a native Requests
mock, and none existed — this is that mock, drafted for maintainer approval before the build PR
(map row 17). This is a proposal, not an approved behaviour change. No production component,
route or API contract changes.

Open [the interactive prototype](../prototypes/native-requests.html) directly in a browser. Its
controls switch role (admin/member), show one or both phone sizes, force a long title, move the
Requests tab's position in the bottom bar, and switch Android's primary action between an app-bar
icon and a FAB. Within the prototype, tapping Select/Approve/Deny/bulk-approve drives the real
interaction (checkbox state, a BottomSheet, a simulated partial-failure result) — nothing calls a
server. All fixtures are invented; no real titles, names or hostnames.

## What this had to resolve

The redesign map's Requests section (`## Requests`) is "built" on web, but N2 adds Requests to the
native phone tab bar for the first time, and no native mock exists. The map's critique **Major: two
domains in one queue** (channel proposals and filler clip downloads sit together under one bulk
"Approve") is still open on web too, and decision **R1** says group by kind with a bulk approve
per group — this draft is also the first concrete shape for R1 at phone width, where there is far
less room to separate two lists visually than on web.

## Composition

- **Within-page tabs**: Needs you (n) / In progress / Done (n), matching the web mock and today's
  built web page (`request-lists.tsx`) exactly in copy and counts. No native touch-sized
  "count tabs" primitive exists yet (web's is `NavTabs`, PR sequence item 3) — this prototype
  improvises a 44pt segmented row rather than inventing a new design-system component; the real
  build should decide whether that becomes a shared `ui` primitive or a one-off here.
- **Needs you (admin)**: two independently-selectable groups, **Channel requests** and **Filler
  downloads** ("Requested by Loomarr"), each with its own "Select" toggle and its own bulk-approve
  action. This directly implements R1's "group, bulk approve per group" and resolves the critique's
  mixed-bulk concern: there is deliberately **no** "select all" that spans both groups. Below both,
  **Couldn't be built** lists the viewer's own failed requests with one fix action, sourced from
  `journey.actions`/`failure.recoveryAction` the way `requestFixLabel` already picks it
  (`web/apps/web/src/queue/request-status/request-status.ts`).
- **Needs you (member)**: only **Couldn't be built**, for their own requests — members never see
  approvals (admin-only, §11); reading remains global (§342), matching the web page's
  `NeedsYouList`.
- **Bulk approve → partial failure**: selecting items in one group opens a BottomSheet
  (`web/packages/design-system/src/sheet/sheet.tsx`, #1776) with a count and the one-group scope
  statement, then "Approve N". The result reuses the same sheet to show `ok`/`error` per item —
  the exact shape of `BulkApproveResult` (`approved`, `results: {id, ok, error}[]`) already
  returned by `POST /v1/proposals/approve` (`model-bulkApproveOutputBody.ts`,
  `model-bulkApproveResult.ts`). One item is always shown failing ("Already decided by another
  admin") so the maintainer can see the failure state without hunting for it.
- **Deny**: a single row's Deny opens the same BottomSheet with an optional note field, instead of
  an inline reveal (web's `ApprovalQueueItem` opens an inline reason input beside the row) — phones
  have far less width beside a row's text, and the sheet is the component #1776 built for exactly
  this kind of single, modal decision.
- **In progress / Done**: rows with two offset thumbnail placeholders, a status badge, the relative
  date and a one-line hint, reusing the **exact** badge label/tone pairs
  `request-status.ts` already computes (`Generating` / `Waiting for approval` / `Getting N titles
  (…)` / `Couldn't get N titles` / `Building your channel` / `On your channel` / `Not approved` /
  `Channel removed`, tones `suggest`/`caution`/`onair`/`lock`), not new copy.
- **Empty / loading / error**: per-tab empty copy matches `request-lists.tsx`'s `TabEmpty`
  strings verbatim ("Nothing needs you", "Nothing in progress", "Nothing finished yet", each
  ending in "Request a channel"). Loading is three skeleton rows; error is "Couldn't load your
  requests." + Try again — invented wording (no mobile-specific copy exists yet to source this
  from); flag for a copy pass in the build PR.
- **Long titles**: a toggle swaps every title for an invented 150-character sentence to check
  2-line clamping and that the 44pt action row still reaches full width without the title pushing
  it off-screen.
- **Where Requests sits in the tab bar**: `ClientNavigation`'s current three destinations are
  Watching / Guide / Surf (`web/packages/ui/src/client-navigation/client-navigation.tsx`). Nothing
  in the map or the N2 decision says where a fourth item goes, so the prototype shows **two
  labelled alternatives** via a control: **A — after Guide** (Watching, Guide, Requests, Surf;
  groups "finding/approving something to watch" together) and **B — after Surf, last**
  (minimal-diff append). Neither is picked here.

## Open questions this draft did not settle (show, don't guess)

1. **Tab-bar position for Requests** (A vs B above) — needs a maintainer call; the prototype
   defaults to A but both are live via the control.
2. **Android's primary "request a channel" action**: an app-bar icon button (parity with iOS) vs a
   Material FAB (the platform's own idiom for a primary creation action). Shown as a control;
   not decided.
3. **No `requests` glyph exists** in `web/packages/design-system/src/icons/icons.ts` — today's
   vocabulary is `back, channelDown, channelUp, channels, close, guide, home, info, loading, menu,
   pause, play, previous, search, settings, skipForward, success, time, volume, volumeMuted,
   warning`. The prototype draws a placeholder glyph (▣) for every "Requests" icon. Two
   alternatives for the build PR: (a) add a new icon (e.g. an inbox/tray glyph) to the shared
   vocabulary, or (b) reuse an existing glyph (`info` or `success`) at a different tone. Not
   decided here — a design-system change, not a prototype decision.
4. **Filler-pull bulk approve has no backend endpoint today.** `POST /v1/proposals/approve` returns
   `BulkApproveResult[]` for proposals, but filler pulls only have single
   `POST /v1/filler/pulls/{id}/approve` / `.../dismiss`
   (`web/apps/web/src/suggest/approval-queue/approval-queue.tsx` calls `useApproveFillerPull` and
   `useDismissFillerPull` per row, never in bulk). Two alternatives for the Requests (native) build
   PR: (a) add a `POST /v1/filler/pulls/approve` bulk endpoint mirroring the proposals shape
   (`{ids} → {approved, results}`), the more consistent and recommended shape; or (b) have the
   client loop the existing single-approve call per id and assemble the same `{ok, error}` shape
   itself. The UI in this prototype is identical either way — this is a backend-shape decision,
   not a design one.
5. **No native touch "count tabs" / segmented primitive exists** (noted above) — whether the
   in-page Needs you/In progress/Done row becomes a shared `ui` component or stays local to this
   screen is left to the build PR.
6. **"Request a channel" from this screen opens Guide's composer**, which is out of this mock's
   scope (Requests only). The prototype intercepts the tap and announces the destination rather
   than reproducing the composer.

## Source grounding

- `project/redesign-map-2026-09.md` — `## Requests` (mock, critique, data), Decisions
  **R1** (group filler downloads, bulk approve per group), **N2** (native gets Requests, needs a
  mock), **N5** (controls under the picture is a phone exception — not applicable to this screen,
  which has no player), **N6** (phone Guide keeps the compact grid — not applicable here, carried
  for context only), the `### iPhone` / `### Android phone` sections (5d/5e/5f/5g describe Guide
  and Watching, not Requests — nothing there to copy directly, confirming the screen is genuinely
  new), and the roll-up's row 18 and row 37 (two domains in one queue; numeric dates/disabled-button
  a11y, both already fixed on web per PR #1769 and inherited here by reusing its exact copy and
  tone pairs).
- The merged web Requests screen: `web/apps/web/src/queue/request-lists/request-lists.tsx`,
  `web/apps/web/src/queue/request-status/request-status.ts` (status line/tone source of truth),
  `web/apps/web/src/suggest/approval-queue/approval-queue.tsx` (admin approval queue, bulk
  selection, filler pulls alongside proposals), `web/apps/web/src/components/loomarr/ai/
  approval-queue-item/approval-queue-item.tsx` (single-row approve/deny shape).
- The native tab bar and BottomSheet, #1776: `web/packages/design-system/src/tab-bar/tab-bar.tsx`,
  `web/packages/design-system/src/sheet/sheet.tsx`, `web/packages/ui/src/client-navigation/
  client-navigation.tsx` (today's three destinations, confirming Requests is a genuine fourth).
- The request/approval API: `model-proposalDTO.ts`, `model-proposalJourneyDTO.ts`,
  `model-proposalJourneyDTOMilestone.ts`, `model-proposalJourneyFailureDTOCode.ts`,
  `model-proposalJourneyFailureDTORecoveryAction.ts`, `model-bulkApproveInputBody.ts`,
  `model-bulkApproveOutputBody.ts`, `model-bulkApproveResult.ts`, `model-pullDTO.ts`,
  `model-approveFillerPullInputBody.ts` — read directly from `web/packages/api/generated/model/`
  for exact field names and enum values; every fixture label (`Getting 3 titles (3 downloading)`,
  `Couldn't build`, etc.) is copied from `request-status.ts`'s actual strings, not invented.
  Member-vs-admin actions: approving/denying is admin-only (`ApprovalQueue`'s callers all gate on
  `useAuth().isAdmin`); members only ever read their own `journey` rows and act on `actions`/
  `recoveryAction` for their own failures.
- Visual tokens (`web/packages/design-system/src/tokens/tokens.ts`,
  `web/packages/tokens/generated/theme.css`): exact dark-theme hex values for surface, content,
  border and the badge tone colours (`suggest #D6409F`/`#DC5BAC`, `lock #3DD68C`, `caution
  #F5D90A`, `onair #E5484D`/`#E85A5F`, `tune #4CC9E8`), so the prototype's palette matches the
  design system rather than approximating it.

## Verification

No browser or Playwright install is available on this machine (checked: no `playwright` in
`web/node_modules`, no `chromium`/`google-chrome` binary on `PATH`). The acceptance criteria allow
a local Playwright check of the prototype only, but this machine cannot run one — recorded here
rather than claimed. What was actually done:

- `node --check` against the prototype's extracted `<script>` body: no syntax errors.
- Manual read-through of the rendered HTML/CSS for both frame widths (390×844 iPhone, 412×892
  Android) against the fixture data, confirming every control-driven branch (role, scenario,
  long-title, nav position, Android primary action) produces distinct markup.
- Every interactive action (`approve`, `deny`, `toggleSelect`, `toggle`, `confirmBulk`,
  `confirmDeny`, `fix`, `edit`, `newRequest`, `retry`) is wired to a `data-act` handler and
  announced in the status line below the frames; none performs a network call.

This is not a device test, an accessibility audit or a production integration check — only a
layout/interaction sanity check of the standalone file. The build PR (map row 17) should re-verify
on-device with the real `ui/guide`-adjacent primitives once this draft is approved, and should run
the repository's actual local Playwright prototype check once Playwright is available in this
environment.

Local reproduction: open `project/prototypes/native-requests.html` directly in a browser (no
server needed — there are no external resource loads).
