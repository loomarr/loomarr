# Requests approval-consequence summary: live verification (2026-10-13)

Build of the approved mock [#1876](../prototypes/mock-requests-consequences.html) for
[#1817](https://github.com/loomarr/loomarr/issues/1817). Screenshots are in
[`requests-consequences-2026-10-13/`](requests-consequences-2026-10-13/). All data is the demo
library's invented catalogue. There are no real titles, people or hosts.

The screenshots show the final build. The bulk groups and consequence dialog are
`@loomarr/ui` components (`ApprovalBulkGroup`, `ApprovalConsequenceDialog`) built from
design-system primitives and the shared `ModalOverlay`. The first legacy-stack build was
re-shot after this rebuild (maintainer decision A, #970).

Every shot was re-taken on current `main`, after merging the phone bottom bar (#1849). The 390
shots show the bottom bar, not the old side rail. They also show the retry fix: a failure is
offered Retry only when the server marks it `retryable`.

## Setup

- An isolated lane backend (`make dev-be`) and Vite (`make dev-fe`) from this worktree, plus the
  demo library stand-in (`make demo-library`). The main `:8080` backend was not touched, and the
  DB was not written directly.
- The demo seed was run with the fix from #1901. Without it, `GET /v1/proposal-jobs` returned 500
  ("done job has no Attempt") and the Requests page failed about a second after load (#1534
  class). The first lane DB was left as it was, and a fresh DB file was used instead.
- Media jobs were paused (`filler-pipeline`, `filler-split-sweep`, `filler-fetch`,
  `channel-maintenance`) right after login and again after the seed.
- Requests were created through the real API (`POST /v1/discovery/manual-channel`), with
  lineups picked from library search. Sizes come from the stand-in's `Size`, which is the
  generated file's real byte count, the way Emby reports it.
- The pages were driven with a scratchpad Playwright script against the lane, at 1280×1000 and
  390×1400, plus 390×844 (a real phone height) for the bottom-bar check. The Playwright MCP
  browser has no Chrome on this machine. All lane processes were stopped afterwards by exact PID.

## States

| State | What the summary said (live) | Shots |
|---|---|---|
| All known (3 owned films) | `Size: 100.4 MB` | `confirm-all-known-*` |
| Mixed (1 film, 2 series) | `Size: 33.5 MB for 1 of 3 titles · 2 unknown` | `confirm-mixed-*` |
| All unavailable (2 series) | `Size: estimate unavailable` | `confirm-all-unavailable-*` |
| Selection aggregate (3 requests) | `Size: 133.9 MB for 4 of 8 titles · 4 unknown` | `confirm-selection-aggregate-*` |
| Partial failure | `Approved 1 of 2`. The second request was approved through the API between opening the summary and confirming (a real concurrent-admin race). The wire answered `{"ok":false,"error":"This suggestion has already been approved or dismissed.","status":409}` with no `retryable`. The failed row states that reason with **no Retry**, and there is no "Retry N failed" footer: a retry could only repeat it. The script counted 0 Retry buttons at both widths. | `result-partial-failure-*` |

The race used fresh requests created for this run: "Harbour Lights" and "Lantern Hour" (desktop),
"Tidewater Reels" and "Copper Kettle Hour" (390). There were no console errors and no horizontal
overflow at any width.

**Bottom bar at 390.** The script measured the dialog panel against the phone bottom bar
(`nav[aria-label="Primary"]`). Neither touches the other in any state. At 390×844 the tallest
dialog, three requests (`confirm-selection-aggregate-390x844`), runs from y=69 to y=775, and the
bar starts at y=790. At 390×1400 the result dialog ends at y=849 and the bar starts at y=1346.
The modal's scrim paints over the bar (`elementFromPoint` at the bar's centre is not the bar), so
the bar can't take a press while the dialog is open.

**A retryable failure is not shown live.** A real 5xx per item needs a store or downloader fault,
which a lane can't cause without touching the DB. The `RetryableFailure` and
`FillerPartialFailure` stories cover it, as do the dialog unit tests and the Go handler tests
(proposals 500 → retryable; filler 502 → retryable; 404/409/422/501 → not).

A Tamagui prop-order slip, `borderWidth={0}` after the one side that should keep its rule,
dropped the group's row dividers. The live overview showed it, and it was fixed before the final
shots and baselines. Escape cancelled each
summary and kept the selection.

## Found and fixed during this check

At 390px the race emptied the queue, because those were the last two pending requests. The page
unmounted the approval queue when the Needs-you count reached zero, and the result dialog
vanished unread. The fix keeps the queue mounted while a review is open. The regression test is
`request-lists.test.tsx`, "keep the bulk result on screen after approving the last pending
request". The re-shoot left three requests pending, so the queue under the result dialog is not
empty in these shots. The regression test covers the emptied case.

The previous re-shoot's partial-failure row offered Retry, and "Retry 1 failed", on that
permanent 409. Retrying could never succeed. Both bulk approve endpoints now return each failed
id's `status` and a server-decided `retryable`, and the dialog offers Retry only on retryable
rows (Refs #1876).

## Not verifiable live

- **Filler downloads group.** Nothing creates pending pulls since #1846, so stories and unit tests
  cover it: `FillerDownloads`, `FillerPartialFailure`, "12 clips queued", "Channel: unavailable".
- **"Updates the channel …".** Revising an approved job needs the LLM, which this lane doesn't
  have. Covered by the `UpdatesExistingChannel` story and unit/queue tests.
