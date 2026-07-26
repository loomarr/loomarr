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

11 drifted claims (**2 resolved, 9 open**), 6 unbuilt, **0 undocumented surfaces** (all 5
closed — see the end of this section). The ones that mislead a reader most:

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
- ~~**`:752`** — "retry/cancel; members see their **own** submissions"…~~ **Resolved 2026-07-26
  (doc-only fix). Not a privacy bug — the audit misread it.** The facts held: cancel is absent
  from the route, and `TitleDTO` carries no requester so every authenticated user sees every
  tracked title. The *conclusion* did not. **§342 makes global read visibility deliberate** —
  "read visibility is global for all authenticated users… members see all channels and titles"
  — so the code was right and the prose contradicted its own §342 one document over.
  ⚠ **Do not "fix" this by adding `requireAdmin` to `GET /v1/titles`.** Authorization here is
  two-state (see the comment at `guide_test.go:351`), so an admin gate would not scope the list
  to the member — it would delete the queue from every non-admin account. Real scoping is a
  schema change (requester column + filtered route), not an auth tweak. `:764` now says so, and
  separates the *backend* cancel at `:253` (a real withdrawal under the direct requester,
  admin-only via `DELETE /v1/titles/{key}`) from the member-facing control that was never built.
- ~~**`:750`** — "Zoom controls the window span"…~~ **Resolved 2026-07-26.** Verified against
  `guide-grid.type.ts:12-16` (zoom scales rail width / row height / type, and "the window always
  fits the viewport") and `guide-page.tsx:157` (a separate `Window span` select — 2H/4H/6H/day —
  which asks the API for a different window). The doc now states both controls and why they are
  separate: the TV-guide convention is that you change how much *detail* a row shows, not how
  much *time* is on screen.
- **`:753`** — edit-via-search pre-approval. `ProposalReview` accepts `onEditItem` and **no
  production caller passes it**, so the button never renders. This is what V25b builds.

~~**Undocumented surfaces** (code with no §12 coverage)…~~ **Resolved 2026-07-26.** All five
now carry §12 rows, each verified against code rather than inferred:

- The **seven-tab Settings IA** — `Connections · AI · Channels & playback · Filler · Tasks ·
  Users & security · Advanced` (`routes/_authed/settings/route.tsx:13-19`). §12 had a one-line
  "Settings/health" bullet naming only provider visibility, `/readyz`, and the checklist; the
  tabs, and everything below, were absent. The row records *where the surfaces are* and defers
  the mechanics (typed registry, `env > db > default`, hot-apply, save bar, secrets lifecycle)
  to `config-design.md`, which wins on its own domain per CLAUDE.md's precedence rule.
- The **Tasks job console** → Settings → Tasks (cron, last/next run, Run-now, cron editor).
- The **AI model manager** (§8.1) → Settings → AI.
- **Secret regeneration** → Settings → **Users & security** (`settings/users.tsx:10` renders
  `SecretsSettings` as the tab footer) — not its own tab, which is why it read as missing.
- **`/account`** — its own top-level row, not a Settings tab. It is the one settings-shaped
  surface a **member** can reach (`account.tsx:17`: "A viewer sees exactly what an admin sees
  here. Nothing on this page is privileged"), so filing it under the admin-only `/settings` IA
  would have misdescribed who can use it.

## How to pick this up

The audits are re-runnable: `/surface-audit channels`, `/doc-drift "design.md §12"`. Prefer
re-running to trusting this file — it was accurate when written and is a hypothesis now, which
is the same rule `/register-check` applies to the build plan.

Tier 2 is the natural next slice: three controls, one new lifecycle block, and it closes the
finding that most reproduces the pattern the audit exists to catch. (It is built — see the
tier-2 PR — but that is a separate change; this file records only what has landed here.)

Also still open: the **§12 doc drift** above — **9 claims**, after `:752` and `:750` were
resolved. The five **undocumented surfaces** (the reverse defect — code with no §12 coverage)
are **all closed**.
