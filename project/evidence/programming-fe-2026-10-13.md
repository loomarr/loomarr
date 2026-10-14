# Programming defaults, overrides and change list: live evidence

Evidence for [#1889](https://github.com/loomarr/loomarr/pull/1889) (Refs
[#1817](https://github.com/loomarr/loomarr/issues/1817),
[#1877](https://github.com/loomarr/loomarr/issues/1877),
[#1659](https://github.com/loomarr/loomarr/issues/1659)). All data is the demo library's invented
catalogue (`make demo-library` + `make demo-seed`). Nothing ran against a real media server.

## Setup

The household dev backend on :8080 wasn't running, so this used the worktree's isolated backend
(`make dev-be`, Air) and Vite (`make dev-fe`), built at the PR head. Media jobs were paused first.
Every change went through the UI with Playwright's Chromium and a dev-login session cookie. Each
state is shot at 1280px and 390px. The app scrolls an inner container, so each viewport was grown
to the content's height instead of using a full-page capture.

## States

| State | Desktop | 390px |
| --- | --- | --- |
| Fresh channel: 0 of 11 overrides, every field Default, Resets disabled | [desktop](programming-fe-2026-10-13/01-fresh-all-default-desktop.png) | [390](programming-fe-2026-10-13/01-fresh-all-default-390.png) |
| Five overrides set in the UI: ceiling, play order, Era 1985–1995, movie spacing 168h, max from one series 2 | [desktop](programming-fe-2026-10-13/02-overrides-desktop.png) | [390](programming-fe-2026-10-13/02-overrides-390.png) |
| After Reset on movie spacing: badge back to Default, box re-seeded empty | [desktop](programming-fe-2026-10-13/03-after-reset-desktop.png) | [390](programming-fe-2026-10-13/03-after-reset-390.png) |
| "Use separate date windows" from Era: three identical axes, "Use a single era" offered | [desktop](programming-fe-2026-10-13/04-separate-windows-identical-desktop.png) | [390](programming-fe-2026-10-13/04-separate-windows-identical-390.png) |
| "Use a single era": back to Era with the same years | [desktop](programming-fe-2026-10-13/05-back-to-era-desktop.png) | [390](programming-fe-2026-10-13/05-back-to-era-390.png) |
| Movie release diverged to 1985–2005: per-axis mode stays, "Use a single era" withdrawn | [desktop](programming-fe-2026-10-13/06-per-axis-diverged-desktop.png) | [390](programming-fe-2026-10-13/06-per-axis-diverged-390.png) |
| Rules draft (Weekday, in order): draft banner, "65 changes until Wed 9:29 AM" with rows | [desktop](programming-fe-2026-10-13/07-rules-draft-change-list-desktop.png) | [390](programming-fe-2026-10-13/07-rules-draft-change-list-390.png) |
| Auto-curate opted in, Quality bar 70 (override), Title cap Default | [desktop](programming-fe-2026-10-13/08-auto-curate-opted-in-desktop.png) | [390](programming-fe-2026-10-13/08-auto-curate-opted-in-390.png) |
| Rules draft (Weekend) on a Tuesday: "No upcoming slots change" | [desktop](programming-fe-2026-10-13/09-change-list-empty-desktop.png) | [390](programming-fe-2026-10-13/09-change-list-empty-390.png) |

The truncated state ("Showing 100 of N") was not reachable. The 24-hour schedule horizon holds 41 to
65 slots per demo channel, so reaching it would mean changing the household Schedule horizon setting.
The `ChannelProgrammingChanges/Truncated` story baseline covers it.

## Fixed after this pass

- At 390px the Seasonal selects (`w-64`) ran 4px past the card. They are now `w-full max-w-64`.
- At 390px a date range's From and To wrapped apart, so "Remove" read like its own row. Each pair now
  stays together, and only Remove or Add range wraps.
- Every per-axis row was named "From year", "To year" and "Remove". Each name now ends with its axis
  and range, for example "Remove (Movie release, range 1)", in the same visible-text-plus-sr-only
  pattern as Reset.
- The auto-curate thresholds put their badge inline after the label. They now use every other
  field's label row (label left, badge and Reset right) in the mock's two-column grid, without the
  indent that split the badge row at 390px.

## Seen, not changed here

- At 390px the lineup editor's rows (`ChannelLineupEditor`, untouched since #901) stack
  Keep/Less/Never/Surprise over the title, and the Seasons inputs clip.
- Two policy edits within the save round trip (Era From, then To immediately) 409 on the second
  PATCH, and that value is dropped. The channel save path (`route.tsx` `update`) is not part of this
  PR.
