# Requests — approval consequence summary & bulk scope design review candidate

Review checkpoint for [#1817](https://github.com/loomarr/loomarr/issues/1817) and parent
[#1659](https://github.com/loomarr/loomarr/issues/1659). This is a proposal, not an approved
behaviour change. No production component, route or API contract changes.

Open [the interactive prototype](../prototypes/mock-requests-consequences.html) directly in a
browser. Its controls switch role (admin/member), the pending-work scenario (both groups / channel
requests only / filler downloads only / empty), and frame width (desktop/phone). Checking a row and
tapping a group's "Approve N" drives the real interaction (selection state, a consequence-summary
dialog, a simulated partial-failure result with per-item retry) — nothing calls a server. All
fixtures are invented; no real titles, names or hostnames.

## What this had to resolve

The redesign map's `## Requests` section is "built" on web with **no data gaps**, but its own
critique flags **Major: two domains in one queue** — channel proposals and filler clip downloads
sit under one mixed "Waiting for your approval" with one bulk Approve — and Decision **R1** answers
it: group by kind, bulk approve per group. #1817 asks specifically for the piece neither the built
page nor R1 itself spelled out: what approving actually *does*, shown before the admin confirms,
plus an explicit confirm step and a partial-failure result with per-item retry. None of that exists
on the built page today — `ApprovalQueueItem`'s Approve button fires immediately
(`web/apps/web/src/suggest/approval-queue/approval-queue.tsx:247`), and the shared bulk toolbar fires
`useBulkApproveProposals` directly on click, with no intermediate step
(`approval-queue.tsx:183-190`).

## Composition

- **Two independently-selectable groups**, directly implementing R1: **Channel requests** and
  **Filler downloads** ("Requested by Loomarr"), each with its own select-all checkbox and its own
  "Approve N" action. There is deliberately **no** control that spans both groups — the critique's
  mixed-bulk concern is resolved by construction, not by a warning.
- **Consequence summary, shown before confirming** (the new piece #1817 asks for). Tapping a
  group's Approve opens a dialog (web's `Dialog` primitive,
  `web/apps/web/src/components/ui/dialog/dialog.tsx`, not the native mock's `Sheet` — web already
  has its own modal and the native `Sheet` is a different package) listing, per selected item:
  - the title/requester meta line verbatim from the map's own mock copy ("14 titles · 3 to
    download · up to TV-14"),
  - for a channel request: "Becomes a new channel named …" — grounded in `approval-queue.tsx`'s own
    `onSuccess` navigating to `/channels/$id` on approval, i.e. approving a proposal creates a
    channel, not edits an existing one, today;
  - for a filler download: the clip estimate and source count, explicitly labelled an estimate
    (`pull-card.tsx`'s own caption: "An estimate, rendered as one... a number presented as exact
    becomes 'Loomarr said 40 and downloaded 12'"), and a line stating the channel is **not shown**
    for filler because the API has no such field (see Open questions #1).
  - a running total ("N titles will start downloading" / "~N clips will start downloading, where
    still available").
  - **No disk-size or byte estimate anywhere** — flagged explicitly in the dialog rather than
    invented (see Open questions #2). The brief said "if the API exposes it"; it does not.
- **Edited rows are excluded from bulk**, exactly as the built `ApprovalQueueItem`/`ApprovalQueue`
  already do (`approval-queue.tsx:118-127`): no checkbox at all, a visible "Edited" badge, and a
  note to approve that one individually so the edit is kept. This prototype shows it rather than
  re-deciding it.
- **Confirm step**: "Cancel" or "Approve N" in the dialog footer. Cancelling leaves every item
  exactly as selected — nothing was approved.
- **Partial-failure result**, reusing the shape already returned by both bulk endpoints
  (`BulkApproveOutputBody`/`BulkApproveResult` for proposals, `BulkApproveFillerPullOutputBody`/
  `BulkApproveFillerPullResult` for filler, #1863): a per-item ✓/✕ row with the server's own
  `error` string on failure ("Already decided by another admin" / "Pull was dismissed by another
  admin" — both plausible concurrent-admin races, not invented failure modes) and a **Retry**
  button on that one row, plus a "Retry N failed" button that retries only the failed ids. One item
  always fails in the simulation (the last selected, when more than one is selected) so the state is
  reachable without hunting for it, matching the native mock's same choice.
  - **The two result shapes are asymmetric and the dialog shows it rather than hiding it**:
    `BulkApproveResult` carries `enqueued` (titles enqueued) and `channelId`, so a successful
    channel-request row can say "3 titles enqueued · channel created"; `BulkApproveFillerPullResult`
    carries only `{id, ok, error}` — no enqueued/clip count — so a successful filler row can only say
    "Approved" with a note that no per-item count comes back today. This is a real API gap, not a
    copy choice (see Open questions #3).
- **Admin vs member**: only an admin sees the two approval groups at all
  (`request-lists.tsx`'s `NeedsYouList`: `showApprovals = isAdmin && pendingApprovals > 0`, and
  approving is admin-only per §11). A member's Needs-you tab shows only their own "Couldn't be
  built" failures — identical content to what they already get today, not re-decided here. The
  prototype's role switch demonstrates this by removing both groups entirely under "Member",
  leaving only the failure list.
- **Desktop and phone**: one frame, toggled by a width control (same convention as
  `home-review.html` and `phone-web-nav.html` — `#frame.phone` narrows the chrome rather than a
  second markup tree). Below phone width the row's actions wrap under the copy and the dialog
  becomes a near-full-width sheet-style card; the group header's checkbox/label/count/button wrap
  onto two lines rather than being hidden. This screen already exists on both desktop and phone web
  (it's the built Requests page), so this draft reuses that reflow rather than inventing a new one.
- **Out of scope, deliberately**: In progress / Done and single-row Deny are unchanged from the
  built page (map: "No gaps") and are not redrawn here — only the Needs-you approval flow and its
  new consequence/confirm/result steps are in this draft, per the task brief's explicit list.

## Open questions this draft did not settle (show, don't guess)

1. **Filler pulls carry no channel reference in the API.** `PullDTO`
   (`web/packages/api/generated/model/pullDTO.ts`) has no `channelId` or equivalent field, and
   neither does `AcquisitionIntent` or `PullPlanRowDTO`. (`internal/api/fillerpool.go`'s
   `ChannelID` belongs to a different type, `FillerPool`, not a `Pull`.) So "which channel changes"
   cannot be shown for a filler download today — the dialog says so explicitly rather than guessing
   one. Two alternatives for the build PR: (a) ship the summary without a channel line for filler,
   since pulls genuinely aren't scoped to one channel in the data model, or (b) add a channel/
   filler-selection reference to `PullDTO` first, if pulls are in fact meant to be per-channel (cf.
   memory of a "per-channel filler" design — not confirmed against current code in this pass). Not
   decided here.
2. **No disk/byte-size estimate exists anywhere in the proposal or pull APIs** — not on
   `ProposalItem`, `BulkApproveResult`, `PullDTO`, or `PullPlanRowDTO` (checked all four). The task
   brief said to show it "if the API exposes it"; it doesn't, so the dialog states that gap instead
   of inventing a number. Two alternatives for the build PR: (a) ship without a size figure
   (titles/clip counts are already a meaningful consequence signal), or (b) add a size estimate to
   the acquisition pipeline first (a bigger change — current acquisition doesn't know a title's size
   until it downloads). Not decided here.
3. **`BulkApproveFillerPullResult` has no `enqueued`/clip-count field**, unlike
   `BulkApproveResult`'s `enqueued`. A successful filler bulk-approve result can say "approved" but
   not "how many clips". Two alternatives for the build PR: (a) add a count field to
   `BulkApproveFillerPullResult` mirroring `enqueued`, for symmetry with the proposal result, or (b)
   leave it asymmetric, since a filler pull's own `estimateClips` is already an estimate and a
   post-approval count would still only be approximate until clips are fetched. Not decided here —
   a backend-shape decision, not a design one.
4. **The dialog is web's `Dialog` (base-ui), not a `Sheet`.** The approved native mock (#1840) uses
   a `BottomSheet` for the same confirm/result steps because that's native's existing modal
   primitive (#1776); web already has its own `Dialog` (`components/ui/dialog`), used elsewhere for
   confirmations, so this draft reuses that rather than introducing a second modal pattern to web.
   Not flagged as open — this follows directly from what each platform already has — but noted so
   a reviewer doesn't read the platform difference as an oversight.
5. **Retry re-submits through the same bulk endpoint with only the failed ids**, which is how the
   existing single-approve-already-in-a-loop endpoint works today (a retry is just another bulk call
   with a smaller `ids` list) — not a new retry-specific endpoint. This is the simplest reading of
   the existing contract and is not itself a new decision, but the build PR should confirm the
   client does this rather than looping single-approve calls, to keep one bulk round trip per retry.

## Source grounding

- `project/redesign-map-2026-09.md` — `## Requests` (mock, critique, data: "**No gaps**" on bulk
  endpoints), Decision **R1** (group filler downloads, bulk approve per group), the roll-up's
  numbered rows for the "two domains in one queue" critique and the disabled-bulk-button a11y note
  (both already fixed on web per PR #1769/#1867, inherited here by reusing their copy and controls
  rather than redrawing them).
- The merged web Requests screen: `web/apps/web/src/queue/request-lists/request-lists.tsx`
  (`NeedsYouList`'s admin-only `showApprovals` gate), `web/apps/web/src/queue/request-status/
  request-status.ts` (status line/tone source of truth, unused directly here since this draft is
  scoped to Needs-you, not the status lines on In progress/Done), `web/apps/web/src/suggest/
  approval-queue/approval-queue.tsx` (today's single mixed bulk toolbar, the `edited` exclusion rule
  this draft reuses, `useApproveProposal`'s `onSuccess` navigating to the created channel),
  `web/apps/web/src/components/loomarr/ai/approval-queue-item/approval-queue-item.tsx` (today's
  immediate-fire Approve button — the gap this draft closes with a confirm step — and the `refused`
  titles disclosure, out of scope here since it's about audience-ceiling drops, not bulk/consequence),
  `web/apps/web/src/components/loomarr/filler/pull-card/pull-card.tsx` (the `estimateClips`
  "rendered as an estimate" convention this draft copies verbatim for the filler consequence line).
- The approved native Requests mock, #1840 (`project/prototypes/native-requests.html`,
  `project/evidence/native-requests-2026-10-03.md`): the per-group bulk-selection shape (two
  independently selectable groups, no cross-group select-all) and the partial-failure-always-shown
  convention, both carried into this web draft; the platform's own modal primitive substituted
  (`Dialog` here vs `Sheet` there, open question #4).
- The bulk-approve API, #1863: `web/packages/api/generated/model/bulkApproveInputBody.ts`,
  `bulkApproveOutputBody.ts`, `bulkApproveResult.ts` (`enqueued`, `channelId`, `error`),
  `bulkApproveFillerPullInputBody.ts`, `bulkApproveFillerPullOutputBody.ts`,
  `bulkApproveFillerPullResult.ts` (`{id, ok, error}` only — no count field, open question #3);
  `web/packages/api/generated/model/pullDTO.ts`, `acquisitionIntent.ts`, `proposalItem.ts` — read
  directly for field names, confirming no disk-size field exists anywhere (open question #2) and no
  channel reference exists on a pull (open question #1); `api/openapi.yaml` lines 643–700 and
  13972–14030 for the `BulkApproveFillerPull*` route and schema definitions; `git log --oneline -- "*filler*"`
  confirming #1863 (`d7e76eb85`, `feat(api): bulk-approve filler downloads`) actually landed, not just
  proposed.
- Visual tokens (`web/packages/tokens/generated/theme.css`): exact dark-theme hex values for
  surface/border/badge tones (`tune #4CC9E8`, `lock #3DD68C`, `onair #E5484D`/`#E85A5F`, `caution
  #F5D90A`, `suggest #D6409F`/`#DC5BAC`), and `web/apps/web/src/components/ui/nav-tabs/
  nav-tabs.tsx` for the underline-tab variant Requests itself uses (map: "underline... is the v2
  mock's Queue tab bar"), including its exact count-badge contrast fix (attention pink stays through
  selection).

## Verification

No browser or Chromium binary is available on this machine — checked both the local path
(`npx playwright install chrome` target `/opt/google/chrome/chrome`, absent) and the session's
Playwright MCP tool, which returned the same "Chromium distribution 'chrome' is not found" error on
`browser_navigate`. The acceptance criteria allow a local Playwright check of the prototype only;
this machine still cannot run one, recorded here rather than claimed, same gap the native-requests
draft (#1840) hit on this machine. What was actually done:

- `node --check` against the prototype's extracted `<script>` body: no syntax errors.
- Manual read-through of the rendered HTML/CSS for both frame widths (desktop ~1040px, 390×844
  phone) against the fixture data, confirming every control-driven branch (role, scenario, frame
  width) produces distinct markup, and that the member role removes both approval groups while
  keeping the failure list.
- Every interactive action (`toggle`, `select-all`, `open-confirm`, `cancel-dialog`,
  `close-dialog`, `confirm-bulk`, `retry-one`, `retry-failed`, `approve-one`, `deny`/`dismiss`/
  `noop`) is wired to a `data-act` handler and announced in the status line below the frame; none
  performs a network call. Traced by hand: selecting 2 of 3 channel requests → Approve 2 → confirm
  dialog lists exactly those 2 with their meta lines and "Becomes a new channel" lines → Approve 2 →
  result dialog shows 1 ok (enqueued count, channel name) + 1 failing ("Already decided by another
  admin") with its own Retry button → Retry → both show ok. Same trace for filler downloads,
  confirming the result lacks a clip count on success (open question #3) and the dialog's
  "Channel: not shown" line appears for every filler item (open question #1).
- Confirmed the edited row (`cr-3`) never renders a checkbox in either the row or the bulk-selected
  count, and carries the "Excluded from bulk" note, matching `approval-queue.tsx`'s real rule.

This is not a device test, an accessibility audit or a production integration check — only a
layout/interaction sanity check of the standalone file. The build PR should re-verify in the real
app once this draft is approved, and should run the repository's actual local Playwright prototype
check once a Chromium binary is available in this environment (recommend adding that gap to the
`make agent-status`/dev-watch environment notes — this is the second mock in a row to hit it).

Local reproduction: open `project/prototypes/mock-requests-consequences.html` directly in a browser
(no server needed — there are no external resource loads).
