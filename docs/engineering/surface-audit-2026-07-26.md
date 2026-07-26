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

### ~~Tier 2~~ — CLOSED 2026-07-26

All three built, with tests and gallery stories. §12's surface-map rows now name their doors.

- **`policy.autoCurate`** (+ `.maxTitles`, `.minScorePct`) → **Programming → When it changes**,
  below the curation rules: the same "when does this change" question on a slower clock. The
  §12 "Settings → lifecycle" home it used to claim never existed, so the *doc* was corrected
  rather than a fifth tab invented — identity lives in the page header by an explicit earlier
  decision. Two structural notes worth keeping: the checkbox **constructs/deletes the object**
  (the opt-in IS its presence, which is why no generic field editor could reach it), and the
  opt-out is only safe because `MergeFromOperator` is a wholesale replace (`out := incoming`),
  so an absent key genuinely clears rather than reading as "unchanged". Disabled with a stated
  reason on a hand-made channel — §8.2 skips a channel with no `IntentRef`.
- **`policy.playout.backend`** → **Overview → Advanced → Broadcast**, beside the Tunarr link
  (same subject: who streams this channel). "Follow the default" lowers to `""` so §9.1's
  inherit shape — and its "changing the default affects new channels only" promise — survives.
- **`policy.audience.unrated`** → **Programming → What plays**, beside the ceiling its default
  is derived from. "Automatic" names which way it *currently* resolves ("— skipped" under a
  kids ceiling, "— allowed" otherwise), because the bare word is not actionable without
  knowing Go's `resolveUnrated` rule.

⚠ **One defect the unit tests could not see.** The opt-in's hint always read *"Off, new titles
wait for your approval"* — including when the box was ticked. Every assertion passed (checkbox
state and committed payload were both correct); the **story screenshot** is what showed a
control contradicting its own description. Now pinned in both directions. Worth remembering
when adding the Tier-3 doors: a payload test proves a control *saves*, not that it *reads* true.

### Tier 3 — ~~needs design work first~~ (two of four done; **re-verify before trusting this tier**)

⚠ **This tier's cost estimate was wrong, in the direction the file's own rule predicts.** Two of
the four turned out to be *reuse*, not new design — the estimates were written from the audit's
memory of the domain rather than from the code. Re-read `seasonal.go` / `presets_vocab.go`
before believing anything below.

- ~~**`scope.series`**~~ → **done.** Programming → What plays. The premise held (the field is
  `[]provision.Key` — resolved ids, never names — so a text box really would be a door in name
  only), but "roughly the lineup editor again" understated the reuse: the lineup editor's
  `keyOf` derivation and the shared `SearchCommand` *are* the picker. Narrowed to the series
  branch; movies and id-less series are filtered out of results rather than offered and 422'd.
- ~~**`policy.seasonal.*`**~~ → **done.** Programming → When it changes. **The stated blocker
  did not exist.** `builtinCalendar` (`schedule/seasonal.go:31`) is a FIXED list of five
  holidays with their keywords already baked in, and its ids already reach the frontend through
  the rule vocabulary (`presets_vocab.go:59`, lowered via `LowerWhen` → `knownHoliday`). TMDB
  keyword IDs were the research path for *matching titles to a holiday*, not for *choosing
  which holidays a channel observes* — the audit conflated the two. So this was a checkbox list
  over a closed set, with no backend work. Off-season fallback renders only in `exclusive` mode,
  the only mode that reads it (`seasonal.go:154`).
- **`scope.collections`** — still open, and the blocker is **not** the control. `[]string` of
  media-server collection ids; the frontend has no endpoint that lists them, so a picker has
  nothing to pick from. **The endpoint is the prerequisite** — build that first, then this is
  the same shape as the series picker.
- **`policy.window`** at channel level — still open. Only per-rule windows are settable, and
  only implicitly via the marathon preset.
- **`POST …/{id}/programming/preview`** — still open, and not a control: rewiring the
  Programming tab's preview to run against unsaved edits, the way the Filler sandbox already
  does.

**Reusable finding for the next slice:** `SearchCommand` renders a plain `<input aria-label="Search">`
— no `combobox`/listbox roles, no arrow-key nav. That is drift claim `:757` above, and it bit
while writing these tests (a `getByRole("combobox")` query failed against the real markup).
Query it by label until `:757` is fixed; fixing it is a shared-component change that also
touches the ⌘K palette, so it wants its own PR.

### Deliberate, needs no door

- **`POST …/{id}/reconcile`** — §9: every edit auto-reconciles; there is no manual rebuild.
- **`POST /v1/channels`** — the list has one door (describe → approve) and says so. Note the
  consequence: `strategy` is a *required* field of that body, so it is unsettable at creation
  even now that it is editable afterwards.

## Still open — §12 doc drift

11 drifted claims (**4 resolved, 7 open**), 6 unbuilt, **0 undocumented surfaces** (all 5
closed — see the end of this section). The ones that mislead a reader most:

- ~~**`:714`** describes a **Settings** tab on the channel detail…~~ **Resolved 2026-07-26.**
  The bullet now describes the *Danger zone* tab that exists, and carries an explicit ⚠ saying
  there is no Settings tab and where identity and auto-curate actually live. Worth noting the
  cost this one line imposed: it is what let `autoCurate`'s map row claim a home ("Settings →
  lifecycle") for a surface nobody had built, so the reachability question answered *yes*.
- ~~**`:711` contradicts `:723`**…~~ **Resolved 2026-07-26** — `:711` now points at the page
  header, matching both the row and the code.
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

~~Tier 2 is the natural next slice~~ — **done 2026-07-26** (see above). **Tier 3 is what
remains** — but re-read that section before costing it: two of its four items turned out to be
reuse rather than design work, which is the file's own "accurate when written, a hypothesis
now" rule applying to itself.

Also still open: the **§12 doc drift** above — **7 claims**, after `:752`, `:750`, `:714`, and
`:711` were resolved. The five **undocumented surfaces** (the reverse defect — code with no §12
coverage) are **all closed**.
