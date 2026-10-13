# Requests approval-consequence summary: live verification (2026-10-13)

Build of the approved mock [#1876](../prototypes/mock-requests-consequences.html) for
[#1817](https://github.com/loomarr/loomarr/issues/1817). Screenshots are in
[`requests-consequences-2026-10-13/`](requests-consequences-2026-10-13/). All data is the demo
library's invented catalogue. There are no real titles, people or hosts.

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
  390×1400. The Playwright MCP browser has no Chrome on this machine. All lane processes were
  stopped afterwards by exact PID.

## States

| State | What the summary said (live) | Shots |
|---|---|---|
| All known (3 owned films) | `Size: 100.4 MB` | `confirm-all-known-*` |
| Mixed (1 film, 2 series) | `Size: 33.5 MB for 1 of 3 titles · 2 unknown` | `confirm-mixed-*` |
| All unavailable (2 series) | `Size: estimate unavailable` | `confirm-all-unavailable-*` |
| Selection aggregate (3 requests) | `Size: 133.9 MB for 4 of 8 titles · 4 unknown` | `confirm-selection-aggregate-*` |
| Partial failure | `Approved 1 of 2`. The second request was approved through the API between opening the summary and confirming (a real concurrent-admin race). The failed row shows the server's "This suggestion has already been approved or dismissed." with its own Retry. | `result-partial-failure-*` |
| Retry | Retry re-sends only the failed id. It fails again, honestly, because that request really is decided. | `result-after-retry-*` |

There were no console errors and no horizontal overflow at either width. Escape cancelled each
summary and kept the selection.

## Found and fixed during this check

At 390px the race emptied the queue, because those were the last two pending requests. The page
unmounted the approval queue when the Needs-you count reached zero, and the result dialog
vanished unread. The fix keeps the queue mounted while a review is open. The regression test is
`request-lists.test.tsx`, "keep the bulk result on screen after approving the last pending
request". The 390 result shot above was taken after the fix, with "Queue's clear" visible under
the dialog.

## Not verifiable live

- **Filler downloads group.** Nothing creates pending pulls since #1846, so stories and unit tests
  cover it: `FillerDownloads`, `FillerPartialFailure`, "12 clips queued", "Channel: unavailable".
- **"Updates the channel …".** Revising an approved job needs the LLM, which this lane doesn't
  have. Covered by the `UpdatesExistingChannel` story and unit/queue tests.
