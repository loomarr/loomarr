# Surface + drift audit — 2026-07-26

Merged in **#86** (squashed as `d38d0dc`). Two audits run as parallel subagents
(`/surface-audit channels`, `/doc-drift "design.md §12"`), verified against the code, with the
outcome recorded here so the remaining work survives the session that found it.

**Why this file exists at all:** the findings are larger than one session, and the repo's own
history says an unrecorded finding is a lost one — five phases shipped without a `PROGRESS.md`
row and a later session spent four investigations rediscovering them.

## What was fixed in the session that found it

| | |
| --- | --- |
| `fillerIngest` SSE frame never fanned out | A **live defect**: "Download clips" pinned at "starting" forever. Gated by `events-provider.test.tsx`. |
| `scope.runtimeMax` unreachable | Control in `ChannelPolicyFields` → Programming → What plays |
| `separation.seriesMinGap` unreachable | Control in Programming → How it's ordered |
| `separation.blockMax` unreachable | Control in Programming → How it's ordered |
| `strategy` unreachable | Control beside Ordering — the value its "inherit channel default" refers to |
| 12 surface-map rows wrong or missing | §12's map now records `ORPHANED` where there is no door |

## Still open — channel capabilities with no door

Ordered by how much each costs. All are PATCHable today and unreachable from any route.

### Tier 2 — needs a real control, not a text box

- **`policy.autoCurate`** (+ `.maxTitles`, `.minScorePct`) — *the sharpest one.* Backend
  complete, validated, live-verified (`internal/schedule/policy.go:71-107`,
  `internal/suggest/autoapprove.go`). **The opt-in IS the object's presence**, so there is no
  toggle to hide — nothing can construct it. Needs a lifecycle block on the channel detail;
  note §12 previously claimed a "Settings → lifecycle" surface that has never existed.
- **`policy.playout.backend`** — per-channel internal/Tunarr switch. §9.1's own copy says
  *"Switch one from its own page"*, so the intended home is documented and unbuilt.
- **`policy.audience.unrated`** — the safety pair to `ceiling`, which is editable.

### Tier 3 — needs design work first

- **`scope.collections`, `scope.series`** — lists of external identifiers. A comma-separated
  text box is a door in name only; these want a search-backed picker, i.e. roughly the lineup
  editor again.
- **`policy.seasonal.*`** (`mode`, `holidays[]`, `offSeason`) — the holiday picker is its own
  problem. `loomarr-curation-research` records TMDB holiday keyword IDs as the intended
  mechanism.
- **`policy.window`** at channel level — only per-rule windows are settable, and only
  implicitly via the marathon preset.
- **`POST …/{id}/programming/preview`** — not a control: rewiring the Programming tab's preview
  to run against unsaved edits, the way the Filler sandbox already does.

### Deliberate, needs no door

- **`POST …/{id}/reconcile`** — §9: every edit auto-reconciles; there is no manual rebuild.
- **`POST /v1/channels`** — the list has one door (describe → approve) and says so. Note the
  consequence: `strategy` is a *required* field of that body, so it is unsettable at creation
  even now that it is editable afterwards.

## Still open — §12 doc drift

11 drifted claims, 6 unbuilt, 5 undocumented surfaces. The ones that mislead a reader most:

- **`:714`** describes a **Settings** tab on the channel detail. The fourth tab is *Danger zone*;
  `SECTION_IDS = info/programming/filler/danger`. Identity lives in the page header.
- **`:711` contradicts `:723`** — the prose says the icon editor is in "Settings → Identity",
  the surface-map row says "Overview → Channel icon". The code matches the row.
- **`:700`** — "orval from **committed** `api/openapi.yaml`". The spec is committed; the orval
  *output* is gitignored (`git ls-files web/packages/api/generated` → 0). The generator's own
  header repeats the claim (`orval.config.ts:5-6`). This is also the worktree gotcha: a fresh
  worktree needs `codegen` or every `@loomarr/api` import fails.
- **`:757`** — the ⌘K palette is described as cmdk/shadcn `Command` over `/v1/search` scopes.
  It is hand-rolled (no listbox roles, no arrow-key nav) and passes **no** scopes.
- **`:752`** — "retry/cancel; members see their **own** submissions". Cancel does not exist in
  the route, and there is no per-member scoping at all: `TitleDTO` carries no requester field,
  so every user sees every tracked title. ⚠ Worth triaging as a privacy question, not just drift.
- **`:750`** — "Zoom controls the window span". After V14a, zoom scales chrome only; span is a
  separate control.
- **`:753`** — edit-via-search pre-approval. `ProposalReview` accepts `onEditItem` and **no
  production caller passes it**, so the button never renders. This is what V25b builds.

**Undocumented surfaces** (code with no §12 coverage): `/account` (password change + session
revoke), secret regeneration, the Tasks job console, the AI model manager, and the entire
seven-tab Settings IA.

## How to pick this up

The audits are re-runnable: `/surface-audit channels`, `/doc-drift "design.md §12"`. Prefer
re-running to trusting this file — it was accurate when written and is a hypothesis now, which
is the same rule `/register-check` applies to the build plan.

Tier 2 is the natural next slice: three controls, one new lifecycle block, and it closes the
finding that most reproduces the pattern the audit exists to catch.
