# Filler — normal/exception hierarchy, phone library, return paths — build evidence

Build of [#1817](https://github.com/loomarr/loomarr/issues/1817) item 7 (parent
[#1659](https://github.com/loomarr/loomarr/issues/1659)). It follows the Draft in
`project/prototypes/filler-hierarchy.html` and the maintainer decisions recorded on #1908
(`project/evidence/filler-hierarchy-2026-10-13.md`, "Decisions").

## What was built, per decision

| Decision | Built | Where |
| --- | --- | --- |
| 1. Exception set = held in Incoming + Problems/diagnostics | Only a next action makes the Overview loud; Problems keeps its diagnostics panel | `filler-overview.tsx`, `filler-manage.tsx` |
| 2. Normal is quiet, no READY chip | Ready card: plain card + lock-green dot, no badge. Exceptions: one caution card (`border-caution/40 bg-caution/5`). Clip cards and the compact card name only Preparing / Recording | `filler-overview.tsx`, `clip-card.tsx` |
| 3. Phone library = A | Below `md` (`usePhoneWidth`): 2-up compact grid; PoolHealth folds to "N clips · x/y channels covered" + Details; toolbar folds to search + Filters + icon view toggle; 16px page gutter (header and body) | `filler-catalog.tsx`, `pool-health.tsx`, `filler-page.tsx` |
| 4. Split-review Back via `from=` | `validateSplitReviewSearch` keeps only `incoming`/`library`; anything else falls back to Library. The crumb above the title and Back both use it. Incoming's server-built link is `/filler/splits/{id}?from=incoming`; Library's split navigation sends `from=library` | `split-review-page.tsx`, `routes/_authed/filler/splits/$proposalId.tsx`, `internal/api/fillerincoming.go`, `filler-catalog.tsx` |
| 5. "← Manage" on Incoming, Sources, Tags | The Settings-index markup, rendered once by the Filler shell for those three tabs | `filler-page.tsx` |
| 6. Return from the coverage card's channel page | Coverage card links `?from=filler`; the channel filler route validates it and shows "← Filler overview" | `filler-overview.tsx`, `routes/_authed/channels/$id/filler.tsx`, `channel-filler.tsx` |
| 7. 24px select target | `SelectCheckbox`: the 16px checkbox inside a 24px label, shared by card and row | `clip-card.tsx`, `clip-row.tsx` |
| 8. Members: text, no admin buttons | "An admin can sort this out from Filler › Manage." and no action button | `filler-overview.tsx` |
| Overview "Open diagnostics" | `retry_failed_work` and `retry_acquisition` link `/filler/manage#diagnostics`; Manage opens the panel and scrolls to it once the admin session resolves | `filler-overview.tsx`, `filler-manage.tsx` |
| Draft 3/4: activity + Problems tone | Routine admits neutral (like routine skips); Problems row `warn` (caution), not amber `notice` | `filler-manage.tsx` |

Not built: the Draft's caution "Needs details" chip on an untagged commercial. `ClipDTO` has no flag
for it; the rule lives only in store SQL (`internal/store/clips.go`, `UntaggedOnly`). Building it
honestly needs a server-computed field (an `openapi-client` change), so it is left as a follow-up
for the coordinator to schedule.

## Live check (lane backend, demo data)

Isolated lane backend `make dev-be` (:18793, own SQLite) + `make dev-fe` (:15793) from this
worktree, `make demo-library` + `make demo-seed` (stand-in Emby, invented titles). Filler jobs and
channel-maintenance paused via `/v1/jobs/{name}/pause` before and after the seed. Driven with the
repo's `@playwright/test` Chromium as admin (dev login) and as a created member, at 1280×800 and
390×844. Live state: `nextAction=enable_fetch`, 3 catalog clips, 12–15 held, 6 channels on
`audience`, no diagnostics.

| Check | Desktop | 390 |
| --- | --- | --- |
| Overview, admin: caution card + "Review automation", no member line | ✓ | ✓ |
| Overview, member: member line, no button | ✓ | ✓ |
| Coverage card href | `/channels/{id}/filler?from=filler` | same |
| Channel filler: "← Filler overview" → `/filler` | ✓ | ✓ |
| Library columns | 3 | 2 |
| First clip card top (with frames) | ~497px | ~437px (fold 772px) |
| READY chips in the grid | 0 | 0 |
| Select target (admin) | 24×24 | 24×24 |
| Member checkboxes | 0 | 0 |
| `/filler/manage#diagnostics`: panel `aria-expanded` | true | true |
| Incoming / Sources / Tags "← Manage" → `/filler/manage` | ✓ ✓ ✓ | ✓ ✓ ✓ |
| Horizontal overflow, console/page errors | none | none |

The diagnostics panel is open and in view on arrival (top at 657/800 and 636/844). With no
diagnostics on this seed the page is too short to scroll it to the top; the scroller sits at its
maximum.

Not observable on this seed, so covered by unit tests and stories instead: the quiet ready card,
the deep-link button itself (needs `retry_failed_work`), routine-admit activity tone (no activity
rows), and split-review Back (the demo has no compilations, and making one means running split
detection).

Screenshots: `filler-hierarchy-build-2026-10-13/{admin,member}-{desktop,phone}-*.png`.

## Gates run locally

- Vitest: every `src/filler` and `components/loomarr/filler` suite (40 files, 430+ tests),
  including new tests for: the deep link (both diagnostics actions), the member line with no
  button, the coverage `from=filler` href and the channel page's return link, `from=` validation
  and Back to Incoming / Library / unknown→Library, the phone grid breakpoint (2-up compact vs
  3-up full) and the folded filters, the PoolHealth fold, no READY chip, the compact card, and the
  24px target toggling the checkbox.
- `go test -run Incoming ./internal/api/`.
- `tsc --noEmit`, biome, `pnpm legacy:check` (see the raises below).
- Visual, in the pinned container (`mcr.microsoft.com/playwright:v1.62.1-noble`, under
  `flock /tmp/loomarr-heavy.lock`), grep-scoped to `Filler/` plus the three filler specs. Updated
  only the 13 stories that changed (26 shots), added 8 new stories (16 shots), inspected each, then
  ran plain: 228 gallery tests (screenshot + axe) and 19 spec tests passed.

## Legacy-usage ledger raises (fixes to existing components)

| File | Raise | Reason |
| --- | --- | --- |
| `clip-card.tsx` | tailwind 115→124 | compact variant (decision 3) + 24px select target (decision 7) |
| `pool-health.tsx` | tailwind 21→33, legacyUi 1→2 | folded phone line + Details toggle (decision 3) |
| `channel-filler.tsx` | tailwind 110→118 | "← Filler overview" return link (decision 6) |
| `filler-catalog.tsx` | tailwind 150→159 | folded phone toolbar + 2-up grid (decision 3) |
| `filler-overview.tsx` | tailwind 58→72 | quiet normal card + loud exception card + member line (decisions 2, 8) |
| `filler-page.tsx` | tailwind 42→56 | "← Manage" link (decision 5) + 16px phone gutter (decision 3) |
| `split-review-page.tsx` | tailwind 52→60 | origin crumb (decision 4) |

No new file carries legacy usage; `clip-row.tsx` fell.
