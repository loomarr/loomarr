# PROGRESS.md — Loomarr build tracker

One row per phase (design doc §21). A phase is **done** only when its gate (a set of
tests) is green and the evidence — commit SHA + the exact test command that proves it —
is recorded here. See `CLAUDE.md` for the prime directives; one phase per session/PR.

**V52 — the image service: IN PROGRESS on `v52-image-service`, phases 0–4 of 8 done.**
Gate for what has landed: `make check` **exit 0, zero failures** (0 lint, `-race`) +
`make config-docs` + `make openapi` regenerated with no drift + `make test-pg` green with the
**14** `Images` conformance subtests **verified to actually execute under Postgres** +
`make fe` (**15** `Image` unit tests, verified by NAME under `--reporter=verbose`, not by exit
code) + `make fe-visual` (**778** passed, 14 new `UI/Image` baselines, against a
**freshly rebuilt** `storybook-static`).

⚠ **"Exit 0" and "the assertions ran" are two different claims, and this branch has now been
bitten by both halves.** The pipe version is already recorded below (`make … | tail` reports the
pipe's status over a red suite). The second: `go test -run TestPostgresConformance ./internal/store/`
prints **`ok … [no tests to run]` and exits 0** without `-tags=integration`, because the file is
not compiled at all — so the filter matches nothing and the run proves nothing. The only sufficient
evidence is seeing the subtest NAMES in `-v` output. `make test-pg` passes the tag; a hand-rolled
`go test` does not.

Commits, rebased onto main `d7868a9`: `7f80749` (§22 doc), `fd18f29` (codec), `511cb97`
(service + store + migration), `1049395` (routes), `872872b` (renumber to 00045), `c68ec2d`
(the 10.4× test-harness fix), `c9a05d7` (phase 3a — wiring + `IMAGES_*` settings),
`05e1aee` (gofmt fix, see below), `cc754e6` (phase 3b — the four jobs).

⚠ **Phase 3a's recorded gate evidence was WRONG and this is the correction.** `c9a05d7` says
`make check` exit 0; it did not. Adding `Images ImageService` to `api.Server` landed inside an
aligned run of struct fields, so gofmt wanted to re-align its five neighbours and the tree failed
at the FIRST step of `make check` — `fmt`, before vet, lint or a single test. CI on PR #199 would
have been red the whole time. Fixed in `05e1aee`.

The mechanism is worth keeping because it is not a typo class: **gofmt aligns a whole run of
adjacent fields, so inserting one line re-writes lines you did not touch** — a hand-added field in
an aligned block is unformatted by default rather than by accident. And the reason it was reported
green is the same masking this file already warns about one paragraph up.

Loomarr shows images from four sources and handled each differently — icons as **database
blobs**, clip stills and hover loops on disk under `FILLER_DIR`, TMDB posters **hot-linked from
the operator's browser**. `internal/images` (§22) is one pipeline: sha256 content addressing,
disk storage sharded 2/2 under `images.dir`, AVIF+WebP+JPEG, ThumbHash placeholders, immutable
cache headers. **Nothing is wired yet** — phases 2–8 add the routes, jobs, FE primitive, and
migrate the four existing paths onto it.

⚠ **Three measurements contradicted the research the plan was built on, and the doc was corrected
rather than left to be discovered.** (1) `-tags nodynamic` costs **5.3×**, not ~3× — 12.5ms →
66.9ms per 500px WebP encode, and the fast path is fast *because* this box has a system libwebp
the library `dlopen`s, which is exactly the non-reproducibility the tag prevents. (2) **AVIF is
NOT an order of magnitude past WebP**: with `libaom -still-picture -cpu-used 6` it is **86ms vs
67ms**, about 1.3×. The scary 300–1200ms figures come from running a *video* encoder at video
defaults — asked for ONE frame, SVT-AV1 allocated **2.34 GB**, spawned **82 threads**, and
produced a file **78% larger** than libaom. The AVIF-is-a-job decision survives on a reason
measurement supports — **concurrency, not latency**: each encode forks a multithreaded process,
so a cold grid of 50 posters forks 50 at once. (3) A benchmark caught the plan's own rule being
broken in its own implementation (`Resize` per rung re-walks the halving chain: 231ms → 100ms).

⚠ **Two pre-existing defects found by adding one test group, both worth knowing:**

1. **`make test-pg | tail` reports exit 0 while `make` exited 1.** The pipe masks the status; the
   suite was RED and looked green. Capture the exit code directly, always.
2. **The Postgres conformance `TRUNCATE` list was a hand-written literal** covering ~8 of 20
   tables, under a comment asking the next person to keep it in step. Rows leaked between
   sub-tests, so an assertion over a GLOBAL query passed on SQLite (fresh file per sub-test) and
   failed only on Postgres. ⚠ **Completing the list is WRONG** — proven by two Filler tests going
   red: `filler_sources` and the taxonomy carry **migration-seeded** rows, and nothing in the
   literal distinguished "omitted by accident" from "omitted on purpose". Replaced with
   `DROP SCHEMA` + `CREATE SCHEMA` so `Open` re-runs every migration: seeded rows return by
   construction and new tables are covered the day they exist. Integration suite 9s → 21s.

**Phase 2 is also done** (`1049395`): three Huma operations — `rawOp` byte serve, typed record,
multipart upload — registered in both register lists, spec regenerated, `openapi-verify` green.

**Open as PR #199**, branch `v52-image-service`, worktree `../loomarr-v52-images`. Not merged:
CI had not reported at hand-off, and merging past an unreported check is not a thing this project
does. Re-check with `gh pr checks 199`; the local gate was green on the same tree.
⚠ A fresh worktree needs `npx pnpm@11.13.1 install --frozen-lockfile && npx pnpm@11.13.1 codegen`
before any FE work — `packages/api/generated/` is gitignored, so a skipped codegen typechecks red
*after* a successful install.

✅ **BLOCKER 1 — the migration collision, resolved.** This branch defined `00044_images.sql`;
**V51b merged `00044_filler_clip_pipeline.sql` to main** while it was open. Rebased onto main
(`d7868a9`) and renumbered both dialects to **`00045_images.sql`**; the postgres file's
"mirror of the sqlite 00044" cross-reference moved with it.

⚠ Keep the reasoning, because the next branch that sits open across a merge hits it again.
This was worse than a rename: goose records applied migrations **by version parsed from the
filename prefix**, so a database that already ran one `00044` **silently skips** the other — no
error, no failure, until something queries a table that was never created. Forward-only (§16)
decides which one moves: the **unmerged** migration renumbers, never the merged one. Check
`goose_db_version` on a dev DB for a stale `44`; if it is there, rebuild rather than hand-edit.
This is the concrete form of the conflict CLAUDE.md's worktree table warns about, and 00043
carries a comment about the same trap. The renumber was cheap only because
`internal/store/embed.go` globs `migrations/*/*.sql` — there is no hand-maintained list to
drift.

✅ **BLOCKER 2 — the CI timeout, resolved, and the diagnosis in this entry was WRONG.** It read
`panic: test timed out after 10m0s` → `FAIL internal/api 600.060s` as "the suite is big; raise
`-timeout` or split the package". Both readings were wrong, and measuring before changing anything
is what showed it: **462 top-level tests, 258.8s of a 259.7s package, slowest single test 5.3s,
median ~0.6s.** There is no slow test and nothing to split out.

Benchmarking the shared harness under `-race` found it — `store.Open` on a fresh file **503ms**
(45 goose migrations), `api.Router` **17ms**, and 462 tests, so **~232s of the 259s was the same
45 migrations re-run once per test.** Fixed by migrating ONCE per package into a template SQLite
file and copying it per test (**26ms**): measured **259.7s → 25.0s**, a **10.4×** drop, `make
check` exit 0 with zero failures (`c68ec2d`).

⚠ **Not a weakened gate** (prime directive 2). Every test still runs against a real, fully-migrated
database built by the real `store.Open`, on its own private copy — nothing shared, no assertion
changed. `autoMigrate` stays true on the per-test open, which is what makes both failure modes
safe: an empty or truncated copy is a version-0 database goose migrates the old way (slow, still
correct), and a corrupt one fails `store.Open` and calls `t.Fatal`. There is no path where a test
runs against a schema it did not ask for.

⚠ **The reason a split would have "worked" is worth keeping**: Go's `-timeout` is per test binary,
so N packages each get their own 10 minutes. The panic goes away while all 232s of wasted work
stays, and every test added later still pays 503ms. It buys headroom, not speed — and this branch
tipped the package over precisely by adding `00045`, one more migration on the 503ms path.

⚠ **`newServer` covered only 15 of the 462 tests** — converting it alone measured 247s, i.e. almost
nothing. The other 45 `store.Open` call sites are file-local helpers serving ~10 tests each, and
all of them moved to the shared `openTestStore`. **This is a per-package fix and 56 test files
across the repo open a store the same way**; `internal/store` (33s), `internal/integration` (25s)
and `internal/recurate` (15s) are the next candidates if the Go job ever needs more.

**Phase 3a done** (`c9a05d7`): the service is WIRED and proven end-to-end.

⚠ **`Server.images` was nil, so all three phase-2 routes 404'd in a running instance** — the
"a built component nobody imported" pattern this file already records against V1, V17a and V23.
No unit test can catch it, because the defect IS the absence of a caller, so the guard drives the
real composition root: `TestImageRoutesAreWired` goes through `BuildHandler`, uploads a real PNG,
reads the record and fetches a rendition over HTTP. **Sabotage-verified** — returning nil from
`imageService` makes it fail with the 501 its message names.

The adapter (`internal/app/imageadapter.go`) is the whole cost of "no domain package imports
internal/store": a type-for-type translation across a typing boundary. ⚠ Boxing goes through
`imageService()` because a nil `*images.Service` assigned straight into an interface field is a
**non-nil interface holding a nil pointer**, so `if s.images == nil` would silently stop working —
app.go already carries that warning for `*tmdb.Client`.

Settings: the seven `images.*` keys from §15 under a new `GroupImages`, plus the four job schedule
keys (an undeclared `ScheduleKey` **panics the settings service at startup**, so they land before
the jobs do). `Groups()` is consumed only by the docs generator, so a group with no Settings page
adds a docs section and nothing else — the keys are reachable via Settings → All until a later
phase gives them a form.

⚠ **Two defects in phase-1 code, both fixed in 3a:**

1. **`images.formats` was a DEAD KNOB.** `Config.Formats` existed, `New` defaulted it, and nothing
   read it — an operator dropping `avif` to save CPU or `jpeg` to save storage would have changed
   nothing while `docs/configuration.md` promised both. `Produces` is now its single reader, with a
   test that names the setting, because a setting with no reader cannot be caught by a test that
   does not name it.
2. **`Config` promised hot-apply and could not deliver it.** Its doc comment claimed the values
   were "resolved live from settings by the caller", but they were plain fields captured at
   construction. `MaxUploadBytes`/`PublicBaseURL`/`Formats` are funcs now; `Dir` stays a value
   because the blob store is built from it and re-pointing it at runtime would orphan every file
   already written. **The shape now encodes which knobs hot-apply.** `server.public_url` is the one
   that matters — Tunarr fetches stored icon URLs machine-to-machine, so setting it in the wizard
   must not need a restart.

**Phase 3b done** (`cc754e6`): the four jobs — `images-fetch`, `images-avif`, `images-rehydrate`,
`images-gc` — registered through `registerImageJobs` and pinned by `TestJobSet`, which went red the
moment they appeared and is the only test that can see a job constructed and never registered.

⚠ **The re-key is the part to understand before touching the fetcher.** `Adopt` keys a
not-yet-fetched row on `sha256("url:"+srcURL)` — a namespaced placeholder, because the content hash
cannot be known before the bytes arrive. When the bytes land, identity MUST become the real content
hash or `Cache-Control: immutable` stops being true: a URL would keep its name while its content
changed, which is the one thing content addressing exists to prevent. So `fetchOne` writes a new
row, moves the refs, and deletes the placeholder — **in that order**, because `image_refs` cascades
on delete and the obvious ordering drops the association silently. Sabotage-verified: swapping them
loses the ref and the test says so.

The same machinery covers a case that is not a placeholder at all — a TTL refresh or rehydrate of
artwork upstream has REPLACED. Hashes differ, the row re-keys, every derivative URL changes. That
is the honest outcome; the bytes really are different.

⚠ **The store needed FOUR new methods, not the two this file predicted.** The two known ones were
the GC's (`TotalImageDerivativeBytes`, `ListColdestDerivatives`). The two that were missed:
`RepointImageRefs` for the re-key above, and `ListImagesByOrigin` because rehydrate's work list —
"every remote row, whatever its fetch state" — is expressible by neither `ListImagesAwaitingFetch`
(scoped to the never-fetched sentinel) nor `ListImagesExpiredBefore` (scoped to a cutoff). A file
can go missing under a row in either state. The prediction "the other three jobs need no store
change" was an estimate, not a gate.

⚠ `RepointImageRefs` is **insert-then-delete, never `UPDATE image_refs SET image_hash`**. Two
distinct source URLs can hold identical bytes, so both placeholders re-key onto ONE content hash and
the second update violates the primary key. `DO NOTHING` makes the collision the no-op it should be.

**The TTL decision (maintainer's call): purge-then-requeue.** The GC deletes the original and every
derivative, clears `origin_fetched_at`, and `images-fetch` re-downloads within the minute. Rejected:
re-fetch in place and delete only on failure, which reads as strictly nicer and puts the compliance
question **inside an error branch** — TMDB unreachable for a day would silently keep serving expired
bytes, and the ceiling would be enforced by nothing. A ceiling that holds only while the network is
up is not a ceiling. Recorded doc-first in §22.

⚠ **The GC collects orphans BEFORE it expires, and a test found the interaction.** Both sweeps can
select the same row (past its TTL *and* unreferenced). Expiring first purges the bytes and queues a
fresh download moments before the orphan sweep deletes it — and if that delete fails, a download
instruction for an image no surface will ever show is what survives.

⚠ **Eviction is image-level LRU**, as decided: order by `images.last_used_at`, drop the coldest
derivatives, stop once under `images.cache_budget_mb`. Per-derivative LRU stays rejected —
`image_derivatives` has no `last_used_at` and adding one would make every image request a row WRITE
(a 50-poster grid = 50 writes per page load, on SQLite with `SetMaxOpenConns(1)`). The conformance
test pins the ordering by making `created_at` and `last_used_at` **disagree**: the coldest image's
derivative is the NEWEST file, so a `created_at` ordering returns exactly the wrong answer and still
looks plausible. Sabotage-verified.

**SSRF** (`images-fetch` is the second job to reach the internet unattended, after `filler-fetch`,
and the first driven by a URL in a row rather than a source an operator added): https only, host
allowlist, redirects capped at 3 and refused into private/loopback/link-local ranges, body capped on
the READ. ⚠ **The private-range rule deliberately does NOT apply to the first hop.** A self-hosted
media server is normally ON a private address — that is what self-hosted means — so a blanket ban
would refuse exactly the artwork the install owns. What makes hop one safe is that its host came
from the install's own configuration; hop two is the one an allowlisted host controls.

⚠ The allowlist is **derived** (`tmdb.org` + the `library.url` host), not a settings key. An
allowlist is only a control if something other than the attacker decides what is on it, and a knob
whose only correct values are these two is a third place to get it wrong. ⚠ Matching is exact-or-
dot-anchored-suffix, never a bare `HasSuffix` — sabotage-verified that the bare version accepts
`eviltmdb.org` against an allowlist containing `tmdb.org`.

⚠ **`Derivative.Path` was documented as "relative to the images dir" and never was.** Nothing read
the field before, so the lie was free; the GC hands it straight to a remove, where believing the
comment would have deleted nothing, reported success, and left the budget permanently over.

**The known Dockerfile gap is CLOSED** — `/data/images` is pre-created alongside `/data/filler`.
Phase 3b is what made it bite rather than merely untidy: `images-fetch` runs every minute, so on a
zero-env first run the first thing to touch that path is a background job failing into a root-owned
volume once a minute forever, with nothing connecting it to a directory nobody created. ⚠ This edits
`Dockerfile`, so the CI **Image job runs** (~30 min, both platforms under QEMU) — expected, not a
misfire.

**Phase 4 — the FE `<Image>` primitive (2026-08-09).** One Layer-1 component, seven Storybook
stories, 15 unit tests, 14 visual baselines, both hand-maintained barrels (`ui/index.ts` and
`packages/api/src/index.ts`'s `imagesApi`), and the `thumbhash` §14 row. §22's *Frontend contract*
was already written at phase 0 and the implementation matches it as specified — explicit
`width`/`height`, a `priority` mode flipping loading/fetchPriority/decoding **together**, a built-in
error fallback, and explicit `sizes` (never `sizes="auto"`; Safari supports it in no version).

⚠ **The failure flag was a bare `useState(false)`, and that is a bug a full green suite could not
see.** React reconciles by POSITION, so a grid that paginates, filters or sorts hands a *different*
`image` to the same instance — and the flag survived the swap, rendering a perfectly good image as a
colour block permanently. It would not have looked broken either: the block reads the NEW image's
`dominantHex`, so it reads as a deliberate empty state. Every existing test rendered one image and
never swapped it, which is exactly why nothing caught it; the probe that found it was a `rerender`.

Fixed by giving the failure an identity — `const failed = failedHash === image.hash` — which needs
no effect and no reset, because the comparison goes false in the same render the hash changes.
⚠ **Sticky per image, deliberately:** a `logo` is an operator-pasted arbitrary URL (§22), so a
failure is usually permanent and retrying known-bad bytes on every re-render is the worse default.

⚠ **Both halves are sabotage-verified, and the second one mattered.** The recovery test was proven
red before the fix. The stickiness test passed on the FIRST try, which this file already treats as
suspect — so the plausible-wrong implementation (reset-on-hash-change via a during-render
`setState`) was written and run: it passes recovery and **fails** stickiness. The two tests together
pin the semantics rather than merely the bug.

⚠ **A comment that contradicted its own code, corrected rather than left.** The placeholder memo
was documented as keyed on the hash while the dep array read `image.placeholder`. The code was
right and the comment was wrong, and the reason is specific to this service: phase 3b's fetch
**re-keys** a row from `url:`-hash to content-hash and back-fills `placeholder`, so a hash-keyed
memo is the classic stale-closure shape — it would hold the previous row's blur.

⚠ **Nothing in the app renders `<Image>` yet, so there is no browser verification to have.** The
Storybook gallery is the only real surface until phase 5 wires the first consumer; this is stated
rather than glossed, because "green tests" and "seen working" are different claims and only the
first one is available here.

⚠ **Merged `origin/main` in (V51c + V53a/b/d/e landed while this branch was open), and the merge
found a drift this file has a standing warning about.** `make check`, `openapi-verify` and
`retired-verify` stayed green; `make fe` went **red on two `barrel.test.ts` cases**. The cause is
the interesting part: when phase 4 was written there was **one** hand-maintained API barrel, and
V53d/e added **two more** (`src/zod/index.ts`, `src/msw/index.ts`) plus the guard test that catches
an unexported tag. So `images` was correctly exported from the barrel that existed and missing from
two that did not. **A hand-maintained list can drift because the list MULTIPLIED, not only because
someone forgot an entry** — and no amount of care on the branch could have anticipated it. The
guard main added is what turned a silent gap into a red gate; a fourth barrel would behave the same.

⚠ **`rebase` was the wrong tool and `merge` was the right one, for a reason worth reusing.** All 16
commits touch `PROGRESS.md`, so the rebase demanded the same resolution up to 16 times; one merge
resolved it once. That is only safe because this repo **squash-merges** — the branch's internal
shape is discarded at merge, so linear history buys nothing here. ⚠ `pnpm-lock.yaml` auto-merged
**textually**, which is precisely how a corrupt lockfile enters a tree; `pnpm install
--frozen-lockfile` is the cheap proof that the merged lockfile and merged `package.json` agree, and
`pnpm codegen` had to re-run because main changed `orval.config.ts` (it now emits `zod` and `msw`).

**Next up: 5–7** migrating channel icons, clip artwork and TMDB onto the service; **8** retirements +
`scripts/check-retired.sh` + the `docs/help/` sweep. ⚠ Phases 5–7 each regenerate the orval client,
so per CLAUDE.md's worktree rule they are **not** parallelisable. ⚠ A fresh worktree needs
`npx pnpm@11.13.1 install --frozen-lockfile && npx pnpm@11.13.1 codegen` first —
`packages/api/generated/` is gitignored, so a skipped codegen typechecks red *after* a successful
install.

⚠ **One known gap remains**: the WebP `-tags nodynamic` gap, unchanged and still open.

**V53d — one mock layer instead of 31, and the first migrated file found two defects in the stub
it replaced (2026-08-09, branch `feat/msw-fixtures`).** Gate: `make fe` (**1222** app + **19** api +
51 core + 5 tokens, biome clean on 922 files) + `make retired-verify` (25). Zero Go files touched.

31 test files each hand-rolled a local `stubFetch`, so **31 places independently encoded what the
wire looks like** — the frontend doing exactly what the Go side bans (*"Phases do not invent private
mocks; extend the testkit"*). This is that shared layer: `msw` + orval-generated handlers behind
`@loomarr/api/msw`, with `src/test/msw/server.ts` owning the lifecycle.

⚠ **V53c was PLANNED AND DISSOLVED, on evidence.** It was to be runtime response validation through
the mutator. Orval cannot do it here: `runtimeValidation` does not exist in 7.21.0 (zero occurrences
in any `@orval/*` package), and in 8.24.0 the **only** `.parse()` injection is the Angular
`.pipe(map(...))` path — the fetch client's mutator branch builds its request function and returns
before reaching it. Loomarr's transport IS a custom mutator (CSRF, cookie auth, RFC 7807), and orval
PR #3226 — *"pass zod schema to custom fetch response implementation"* — is still open. **But the
value survived the design dying:** what it was actually for was catching FIXTURE drift, not backend
drift (`openapi-verify` already covers that), and a test knows which endpoint it stubs, so
`validated(schema, fixture)` needs no URL→schema map and no transport change.

**What is generated is the WIRING, not the data.** The URL, method and status come from the spec, so
a renamed route is fixed by a regenerate where a hand-written path silently stops matching and its
test keeps passing against nothing. ⚠ **The generated DATA is never trusted:** optional fields emit
as `arrayElement([value, undefined])`, so presence varies per CALL and nothing is seeded — flaky
rather than merely arbitrary. `useExamples` stays unset (it reads singular `example`; Huma emits 3.1
plural `examples:`).

⚠ **`onUnhandledRequest: "error"` is NOT used, because it does not fail a test.** MSW's docs define
it as *"print an error and halt request execution"*, and the maintainer confirms in mswjs/msw#946
that the interceptor handles the exception as the native class would, so *"from MSW's perspective no
exception has happened"* and the runner never sees it. The server records unhandled requests and
throws in `afterEach` instead. **Verified by direct probe** — a fetch to an unrouted path fails the
test by name — after a first sabotage attempt passed and looked like a broken guard. It was not: the
sabotage was invalid, because the component never made the request I removed the handler for.

⚠ **THE FIRST MIGRATED FILE FOUND TWO REAL DEFECTS IN THE STUB IT REPLACED, and this is the argument
for the sweep.** Neither was findable by reading:

- The old stub answered every non-PATCH call with `{ entries: [] }`, which read as *"the modal loads
  the settings list on mount."* It does not. **A catch-all stub structurally cannot distinguish
  "handled" from "never asked for"** — MSW can, because an unmatched request is now an error.
- It returned `{ results: {} }` where the wire says `results: SettingResult[]`. **The test asserted
  against a shape the API never produces**, and passed for as long as it existed, because a
  hand-rolled stub is untyped by construction. The generated handler is typed, so it is now a
  compile error.

Also: the old stub matched on `init?.method === "PATCH"` alone, so it would have accepted a PATCH to
ANY url — including one the component should never call.

**Migration is incremental by construction.** MSW installs globally with NO handlers, and an
unmigrated test's `vi.stubGlobal("fetch", …)` replaces global fetch outright and never reaches the
interceptor — verified: all 158 files still pass with the server installed. The two mechanisms
coexist until the last stub is gone.

⚠ **`retired-verify` caught this entry's own prose**: the §14 row and `server.ts` both cite
`/v1/suggestions` → `/v1/proposals` as the historical rename example, which is a banned identifier.
Both carry the explicit `retired-ok` opt-out rather than a reworded dodge — the guard is supposed to
fire on that string, and a mention that is deliberate should say so.

**V53e — the migration, in batches (IN PROGRESS, 2026-08-09).** Batch 1 (`#206`): `use-auth`,
`users-step`, `first-channel-step`, `sources-tab`. Batch 2 (`feat/msw-batch-2`): `wizard-ai-block`,
`channel-row-menu`, `use-channel-refine`, plus a shared `channel()` fixture. **8 of 31 migrated; 23
remain** — counted (`grep -rl 'const stubFetch|vi.stubGlobal("fetch"'` vs `@/test/msw/server`), not
tallied, because a running count in a commit message is exactly the sort of number that drifts.

⚠ **Nine defects in eight files — the yield is not tapering, and every one is the same root cause
wearing a different face: a hand-rolled stub is UNTYPED and UNBOUND.**

- **Required fields no stub ever supplied.** `MeBody` requires `local`; `SystemLLMStatus` requires
  `local` AND `reachable`; `ChannelDTO` requires **eleven** fields where tests invented
  `{ id, name, status }`. A component reading `pendingCount` off one of those would see `undefined`
  where the server always sends a number.
- **A response shape the API never produces.** `{ results: {} }` where the wire says
  `results: SettingResult[]` — the test asserted against a fiction and passed for as long as it
  existed.
- **Catch-all branches answering requests that are never made** (three of them), and one hiding a
  request that IS made: `WizardAiBlock` calls `/v1/system/llm/discover`, which `json({})` answered
  silently, so that path ran against an empty object.
- **Assertions matching a url SUBSTRING the test wrote itself** — `calls.find(c => c.method ===
  "PATCH")` would match a PATCH to any endpoint at all. Reaching a route-bound resolver is the
  stronger claim.

⚠ **Error cases stay hand-written, and it is the SPEC's shape, not convenience:** this API declares
errors with OpenAPI `default:` (RFC 7807) on 132 of 134 operations — **zero** explicit 4xx/5xx codes
— so orval has no status to generate an error handler from. Verified safe against a rename: with the
path deliberately broken, the component's real request goes unhandled and the guard fails the test
BY NAME. The failure mode is "no handler" (loud), never "wrong data" (silent).

⚠ **`make fe` does NOT run `fe-visual`**, so a green local gate never covers the Playwright job.
Batch 1's first CI run went red on `mcr.microsoft.com` TLS handshake timeout — a Docker image pull,
`Error 125`, no browser ever started. **The tell was shard asymmetry**: 1/2 passed in 8m49s on the
identical commit while 2/2 died in 57s, and a real snapshot diff cannot be shard-asymmetric on the
same code. Re-run, not a code change.

**Next up: V53f+** — the remaining 23 files in batches of 5–8, then add `vi.stubGlobal("fetch"` to
`scripts/check-retired.sh` in the FINAL batch. ⚠ Not before: the guard would fail on every file
still waiting. `use-channel-refine` is deliberately deferred — deferred promises plus method-only
dispatch (its own comment says it avoids "pinning to exact URL strings", which is the weakness being
removed), so it needs a careful pass rather than a batch slot.

**Also still open, and not superseded by the V53 arc: V50d** (house-style conformance across
`components/ui`), carrying `connection-block`'s collapsed-body focus gap — its Test action and Fix
link remain keyboard-reachable while the section is closed, and it has no story so axe never sees
it. Recorded here because V50c's own forward pointer was removed with it: two live "Next up" lines
is the pattern this file warns about, but a gap with no pointer at all is worse.

**V53b — arrays are not nullable; `null` stops being a second empty list (2026-08-09, branch
`feat/non-nullable-arrays`).** Gate: `make check` (0 lint, `-race`) + `make openapi-verify` +
`make retired-verify` (25) + `make fe` (**1222** app + 18 api + 51 core + 5 tokens, biome clean on
919 files).

Every list field in this API was typed `T[] | null` — **109 nullable type-unions against 4 plain
arrays** — so every client handled two representations of "nothing", forever. The generated zod
carried `.nullish()` on every array and the FE coalesced `?? []` at each use.

The cause was `huma.DefaultArrayNullable`, which defaults to true *correctly*: a Go nil slice really
does marshal to `null`. It is now false, set in `humaConfig` — the single constructor behind both
the served API and the spec export, so runtime and document cannot disagree.

⚠ **The flag alone would have made the spec LIE**, and Huma says so in its own doc comment: *"any
`nil` slice will still encode as `null` in JSON."* Flipping it changes what the document CLAIMS
without changing what the wire SENDS. It ships with a guard or not at all.

**The guard.** `TestResponses_ContainNoJSONNull` drives every parameterless GET — **derived from the
exported spec**, not a hand-kept roster, because a new list endpoint nobody added to a list is
exactly the one that would regress — against an **EMPTY STORE**, and fails on any JSON null in a
success body. ⚠ The empty store is the point, not an accident: nil slices are what a repository
returns when it finds no rows, so a fresh database is precisely the state that produces them, and a
seeded fixture would hide the entire class by never taking the empty branch. Since the spec now
declares nothing nullable, *"no null anywhere in the body"* is the exact invariant.

**It found one leak in 46 paths, and it was the one that matters most.** `/v1/setup/status`
returned `checks: null`: `runConnectionChecks` used `var checks []SetupCheck` and deliberately
contributes no check for unwired services — so an **unconfigured install, the wizard's entire reason
to exist**, was the case that produced it. One leak in 46 is also the shape of the change: 51
`make([]T, 0)` guards already existed across the handlers, so this finishes an inconsistency rather
than inventing a convention.

**Frontend fallout, all benign and all worth naming:**

- The generated zod lost `.nullish()` **entirely** (count: 0) — the ambiguity is gone from the
  schemas, not merely handled at each call site.
- ⚠ `VocabularyWhen`/`VocabularyWhat`/`VocabularyHow` **stopped being generated, and nothing was
  renamed.** Orval only emitted those aliases because `X[] | null` needed a name; a non-nullable
  array inlines to `WhenVocab[]`, so they had no reason to exist. First read was "orval renamed
  them" — worth checking rather than assuming, because the fix differs.
- `presets.ts` carried a comment stating the served arrays *"are nullable on the wire"*. True when
  written, and about to become a lie. The coalescing it justified **stays**, for a different and
  still-real reason: the vocabulary is `undefined` until its query resolves. **Null is gone;
  unloaded is not.**
- Three fixtures passed `models: null`; the component already does `hp.models ?? []` and branches on
  `length === 0`, so `[]` is the identical branch rather than a changed one — checked before
  swapping, since a fixture edit that silently moves a branch is how a test stops testing.

⚠ **This also unblocks orval's MOCK generator, which V53a recorded as rejected** — it degraded
`type: ["array","null"]` to `arrayElement([[], null])` without descending into `items`. Re-measured
on the new spec: **137 never-populated list mocks → 0**, and **247** populated `Array.from(...)`
generators. That is a consequence, not the justification: `null` vs `[]` was an API defect on its
own terms, and if codegen had been the only argument the right answer would have been to leave it
alone.


**V53a — the form schemas stop mirroring the wire and start deriving from it (2026-08-09, branch
`feat/zod-from-spec`).** Gate: `make fe` (**1222** app + **18** api + 51 core + 5 tokens, biome clean
on 919 files) + `make retired-verify` (25 identifiers). Zero Go files touched.

`packages/core`'s three zod schemas mirrored wire field names **by hand**, and that shipped a bug:
`intentSchema` said `maxAcquire` where the wire says `maxAcquisitions` and `runtimeTarget` where it
says `runtimeTargetMin`. Both parsed, both serialized into JSON the server ignored, and a user's
acquisition cap and runtime target silently vanished. A contract test was written afterwards to
catch it — and covered **one of the three**. `bootstrapSchema` and `loginSchema` were unguarded
(checked: neither had drifted, yet).

Orval now emits zod schemas from the same spec (`@loomarr/api/zod`), and each form schema is
`.pick()`ed off its wire schema, then `.extend()`ed with the rules. **A lookalike name is now a
compile error at the schema definition**, for all three and every future one.

⚠ **The error message is cryptic and worth recognising**: picking a key that is not on the wire
fails as `Type 'true' is not assignable to type 'never'`. It means exactly one thing — *that field
does not exist on the wire*. Verified by sabotage: restoring `maxAcquire`/`runtimeTarget` fails
`tsc` at both lines.

⚠ **Generation carries NAMES and TYPES, not RULES — and the split is deliberate, not a shortcut.**
This spec declares 5 `minimum`, 3 `maximum` and 7 `minLength` across ~9k lines, `maxAcquisitions`
has no bounds at all, and OpenAPI has nowhere to put a user-facing message. So the trims, the 0–200
cap, the 8-character password floor and every message stay hand-authored in `.extend()` — and
`confirm` (form-only, never sent) is added there too. That is why the pattern is pick-then-extend
rather than a straight derive: **the wire decides which names are real; the form is free to add its
own on top.**

⚠ **The contract test was kept, not deleted, and the reason is a real distinction.** `.pick()`
guarantees the NAMES exist on the wire; `intent-contract.test.ts`'s assignability check guarantees
the parsed OUTPUT still satisfies `Intent` after `.extend()` rewrites the value types. Proven
complementary rather than assumed: changing `maxAcquisitions` to `z.string()` in the extend produced
**0 errors** in core and **failed** the assignability check. Two claims, two guards.

**The zod barrel is hand-written and therefore guarded** (`barrel.test.ts`), same as the endpoint
barrel — a generated module nobody can import is indistinguishable from one that was never built.
⚠ It checks the ZOD output directory, not the endpoint one: `events` is SSE and has no schemas to
generate, so comparing against endpoints would fail forever on a tag that is correct to be absent.
Sabotage-verified (dropping `users` reports it by name). It is FLAT where the endpoint barrel is
namespaced — namespacing exists there only because orval repeats helper enums per tag file, and zod
names are operation-scoped and collision-free.

⚠ **Only the zod half of orval's generation was adopted; the mock generator was measured and
rejected for its DATA.** It targets OpenAPI 3.0 idioms and this spec is 3.1: **109 nullable
type-unions vs 4 plain arrays, 0 `nullable: true`**. Meeting `type: ["array","null"]` it emits
`arrayElement([[], null])` without descending into `items` — **137 list fields that are never
populated** — and `useExamples` reads singular `.example` where Huma emits plural `examples:`, so
**0 of 53** example tags are used, across **1304 unseeded faker calls**. Not configurable away. The
zod generator handled the same 3.1 spec correctly, which is what isolates the gap to the mock half.


**V51c — sources roll up by provider, with no column, no table and no migration (2026-08-09).**
Gate: `make check` (0 lint, `-race`) + `make test-pg` (both dialects) + `make openapi-verify` +
`make retired-verify` + `make config-docs`.

Three archive.org collections sat as three sibling rows with no indication they are one service.
The Sources tab now shows one **Archive.org** row and one **YouTube** row, each twirling down to
the targets beneath it — and the whole thing is **derived from `kind` at read time**.

⚠ **The argument for deriving it is a correctness argument, not a shortcut: the grouping being
asked for is already a column.** Every `archive` row belongs under Archive.org and there is no
representable case where it belongs elsewhere, so a stored `parent_id` would be a second encoding
of a fact `kind` already carries — and second encodings make illegal states representable
(`kind='archive', parent_id='provider:youtube'`). Three concrete costs it would have added, each
measured against code that exists: a **duplicate** blank-URI YouTube row beside migration 00034's
seeded one (both invisible to `idx_filler_sources_uri`, whose `WHERE uri <> ''` excludes them —
"one source appears twice", which 00023 and 00029 both exist to prevent); a **four-state** inherit
problem for the nil/0/N fetch overrides whose own ⚠ says callers "must not re-derive this
three-state logic separately"; and a 409 on any pending pull, because `filler_pulls.plan_json`
stores `SourceID` strings looked up at approve time.

The escape hatch is recorded in §10 so it is not re-litigated: a provider that ever gains state of
its own gets a `filler_providers` table keyed on the existing `kind` vocabulary — **not**
`parent_id`, because that state is per-provider, not per-node.

⚠ **The phase found a guard that had been protecting nothing for two phases.**
`TestSetFillerSourceEnabled_RefusesRowsWithNothingToStop` asserted a 409 for `PATCH
/v1/filler/sources/remote`. V37 retired that container, so from that point no read model could
produce the id and no client could send it — the test stayed green by asserting a refusal for a
row that did not exist, and the handler kept a `case "remote"` that read as protection. The DELETE
handler had already removed its twin for exactly this reason, in a comment saying so; the PATCH
side was simply missed. The guard is now a **prefix test over the derived provider ids**, which
the read model emits on every request, so it covers a case an operator can actually reach.

**Three things deliberately do not inherit**, each an opinionated call: `enabled` (no group switch
— cascade-on-write destroys each child's own choice, which the store forbids in as many words, and
a computed `effective = parent && child` fails in the direction of *fetching from a provider the
operator switched off*); fetch overrides (leaf only); and `lastFetchedAt`, which becomes a
read-only `MAX` over children computed in the API so no column can disagree.

⚠ **Both new behaviours were sabotage-verified**: splitting the pre-order into "all groups, then
all children" turns the ordering test red, and disabling the prefix guard turns the 409 test red
(404, not 409 — the id resolves to nothing). A first-try green on a new constraint is suspect.

**Honest gap, exposed rather than created:** `sync.go` writes `Source = "filler-dir"` for every
clip the folder scan finds and nothing records which SOURCE a downloaded clip came from, so
`bySource["archive"]` is 0 on essentially every install. A group reports the **sum of its
children's counts** — honest arithmetic over whatever they claim, never an invented number.
Per-source attribution is an intake change, filed separately.

**V51b — ingest becomes one watchable pipeline; seven sweeps become two jobs (2026-08-08).**
Gate: `make check` (0 lint, `-race`) + `make test-pg` (both dialects) + `make openapi-verify` +
`make retired-verify` (**25 identifiers**, up from 9) + `make config-docs`.

Filler had grown **one cron sweep per capability** — sync, fetch, language, split, transcribe,
vision, reindex — each scanning the whole catalog for its own kind of work, on its own schedule,
knowing nothing about what the others had done to a clip. The operator-visible consequence was
that a download of forty commercials said *"waiting to be checked"* for up to an hour while three
jobs worked on it at :15, :30 and :50. **The system was working and looked broken.**

Eight rungs now run in order per clip — `probe → transcode → split → language → transcribe → tag →
vision → score` — behind one `filler-pipeline` job every two minutes. Each rung answers two
questions separately: *does this apply to this clip, in this install?* (no exec, re-evaluated
every pass, so flipping `filler.vision.enabled` on picks up clips that already went past it) and
*do the work*. State lives in `filler_clip_pipeline`, a **sibling of `clips`** — the cache has
been DROP-TABLE-recreated twice, and these rows record that ~341s of Whisper and a paid vision
call have already been spent.

⚠ **Three findings, all live on `main` before this phase, none of which any test could have
caught.**

**1. `filler.autofile.normalize_loudness` has been inert since V42 shipped.** `NormalizeInPlace`
had **no production caller at all** — only tests. The settings comment asserted the opposite,
citing §15's own rule that a setting nothing reads does not exist: *"this one lands with its
consumer (`filler.NormalizeInPlace`, called from the auto-file step)"*. A comment claiming a
consumer exists is not a consumer existing, and nothing failed when it stopped being true. The
transcode rung applies the loudness filter in the pass that is already re-encoding, which finally
wires the toggle.

**2. The score rung would have silently disabled half of `Score`.** `Score(modelConfidence)` lets
the model LOWER the grounded ceiling and never raise it — so *"unsure about a clip whose tags all
verify"* still reaches a human. That self-report exists only inside the tag rung; recomputing from
the persisted row with 0 would score the clip a full 100 and auto-file it. `ScoreClip` passes the
row's confidence as the model layer, so lowering survives and raising stays impossible.

**3. A composite reaching the score rung would have destroyed the reel its segments come from.**
Composite detection moved to `probe` (so a 16-minute recording stops being airable when it is
MEASURED, not when someone finally splits it — §10 V45's bug). But a compilation has no coherent
grounding, so `filler.reject.unidentified` — **ON by default** — would tombstone it. The skip is
**one rule in `advance`**, not six `Applies` checks: six copies is six chances for a new rung to
forget it, and forgetting is silent.

**What was retired, and the one reversal.** Four jobs and their schedule keys, `filler.split.every`
(splitting is a rung every long recording reaches, so "how often do we go looking" stopped being a
question with an answer), and `NormalizeInPlace`. ⚠ **`filler.autosplit.enabled` flips to ON**,
reversing a default whose ⚠ argued for OFF because a mis-cut clip plays half an advert and the
source is consumed either way. That risk is unchanged; the evidence moved — the gate's measured
failure mode is refusing GOOD reels, and off-by-default meant every compilation waited for a click
the design says should be unnecessary.

⚠ **Both drift guards fired during the work, which is the best evidence they work.**
`TestEveryPublishedEventIsInTheEventTypeMap` reads `internal/app` as SOURCE for
`Type: "…", Payload: api.X{` — hoisting the new frame into a local variable hid it, and the guard
reported `filler_clip` as declared-but-never-published. `retired-verify` then caught eleven stale
references to the retired identifiers, including two settings comments that were now simply false.

**Tests: the four retired jobs' suites were PORTED, not deleted** — `stage_language_test.go`,
`stage_transcribe_test.go`, `stage_vision_test.go`. Three cases went with the sweeps rather than
being rewritten (`BoundsOnePass` × 3): the bound is now the runner's budget, and asserting it
against a stage would be testing something that file no longer owns. The grounding tests — the
ones that matter — survive unchanged, because `groundVisionTags` did not change.

**V51a — the filler pre-flight: four dead code paths, one blind fixture class (2026-08-08, `7119d92`).**
Gate: `make check` (0 lint, `-race`) + `make test-pg` (both dialects) + `make openapi-verify` +
`make retired-verify` (9 identifiers) + `make fe` (**1196 tests**, up from 1190).

⚠ **This is the V41 lesson recurring, in the one place V41 did not look.** V41's entry below says
the identity/location fixture blind spot "was written into a comment and the fixture was never
fixed" — it fixed `clipAt`, `sampleClip`, `untaggedClip` and `tagMemStore`, and left the SPLIT
fixtures alone. All four defects here lived behind exactly those.

**1. Split confirm had never worked since V38c.** The persisted proposal stored `clip.Path` in a
field named `clipPath`; `Confirm` fed it to `GetClip`, which is `WHERE hash = ?`. Every confirm
returned *"compilation … no longer in the catalog"* for a clip that was in the catalog — an
operator could detect, open a 41-segment reel, edit it, and never commit. Reproduced first against
a real store, then fixed: the proposal carries `clipHash`, the file location is derived from the
row exactly as `Propose` already derives it.

**2. Every segment upserted with an empty hash.** `UpsertClip` is `ON CONFLICT(hash)`, so segment
N overwrote segment N−1 and a whole reel became **one** row. Cuts are now hashed the moment they
are written and filed at their own shard path, which retires `uniqueClipPath`/`sanitizeClipName`
(content addressing leaves no name to sanitise and no collision to break). **Sabotage-verified**:
removing the hash assignment collapses the test to one row.

**3. `dedup`'s self-exclusion never fired.** Parameter named `clipPath`, tested against `c.Path`,
called with the hash — so every segment was compared against the compilation it was cut from and
came back flagged a duplicate. Noise in the review, and enough for `AutoConfirmable` to reject a
sound reel.

**4. `clips.confidence` had no writer, in any catalog, ever.** `Score` computed the
grounding-capped number and `Tagger.Run` used it for the filing decision and dropped it. Auto-file
worked; the number an operator judges it by was permanently 0, so the Incoming meter never
rendered. `SetClipConfidence` is the writer now. ⚠ The store's conformance case passed the whole
time because it seeded the value through `UpsertClip`'s INSERT — **a column can round-trip
perfectly and still have no producer**.

**Plus a live FE defect found on the way.** `useLoomarrEventListener` never re-dispatched
`onPlayout` or `onDatabase`, so `settings/system/database.tsx`'s migration progress was dead. The
drift guard was green because it regexed BOTH handler objects in one pass and asserted their
**union** — a guard that ORs its two sources cannot see drift between them. It now slices and
asserts each independently, and throws rather than degrading if its anchors move.

**The durable half.** Both split fixtures are hash-keyed with identity and location deliberately
unequal; `seedCompilation` returns the hash so no test can re-derive it; the fake `Cut` writes
span-derived bytes (identical bytes would make segments legitimately collapse, masking the bug);
and `Confirm` is exercised end to end against a real store for the first time.

⚠ **The register is still behind.** V42, V44, V45a and V46–V49 shipped without rows; this entry
sits above V41 because that is the last recorded phase, not because nothing happened between.
Back-filling them is scheduled with the V51 doc pass — see the plan.

**V50c — CollapsibleSection onto Base UI, and a collapsed section stops being focusable
(2026-08-08, branch `feat/base-ui-v50c`).** Gate: `make fe` (**1222** app + 17 api + 51 core + 5
tokens, biome clean on 918 files) + `make fe-visual` (**764 passed, 0 failed, 0 flaky, 0 axe**, no
baseline changed) + `make check` + `retired-verify` (14 identifiers).

⚠ **Scope is ONE component, deliberately.** V50c was sketched as "disclosure/form primitives" and
the form half is dropped: `checkbox` and `switch` are native `<input>`s carrying explicit decisions
AGAINST a primitive, and both record that the reasoning **predates V50a and survives the vendor
change**. Porting them would re-implement semantics the platform already gets right. `search-command`'s
hand-written `role="combobox"` was audited in the same pass and honours its contract in full. **The
sketch was written before anyone read the files; the files had already decided.**

**What the port buys.** The old version's header a11y was already correct (a real
`<button aria-expanded aria-controls>`), so the win is `hiddenUntilFound`: the browser's find-in-page
reaches text inside a CLOSED section and opens it, where `overflow:hidden` left it findable by
nothing.

⚠ **And it removes a defect two tests were resting on.** The old `.reveal` closed with
`grid-template-rows: 0fr` + `overflow:hidden` — zero height but **NOT `display:none`** — so collapsed
controls stayed in the accessibility tree and stayed **focusable**. A keyboard user could Tab into a
section they could not see. Two `channel-filler` tests reached a control inside a closed body with
`findByRole` and passed; they open the section first now, as a user must. **The bug surfaced as a
test that could only pass while it existed.** ⚠ Its failure mode is deliberately unhelpful and worth
recognising: `asyncUtilTimeout` and `testTimeout` are both 5000ms, so findBy's own "Unable to find
role" never surfaces — the test times out first and reports only `Test timed out in 5000ms`.

⚠ **`hiddenUntilFound` is load-bearing, not a bonus.** Sabotaging it to prove the new test could fail
showed both tests failing with *"Unable to find an element with the text"* — Base UI's Panel
**unmounts its children when closed** by default, where the hand-rolled version always kept the body
mounted and merely clipped it. A port that swapped the elements and adjusted assertions to match
would have shipped a silent change to a **mounting** contract, breaking anything holding form state,
scroll position or a ref in a collapsed section. Nothing in the gate reads a mounting contract.

⚠ **The motion stayed hand-rolled, verified rather than assumed.** Base UI measures the panel
(`scrollHeight` → `--collapsible-panel-height`) so an author can transition `height`; the `.reveal`
grid-rows 0fr→1fr trick is height-agnostic, so nothing is measured and a body that changes size
mid-open cannot desync from a stale measurement. The primitive owns state and semantics; the
stylesheet still owns motion. Reduced-motion needed no work — it is a global `*` rule inside a media
query, independent of implementation.

⚠ **The CSS rule is two selectors on purpose.** `.reveal` has a second consumer: `connection-block`
borrows its mechanics and passes `data-open={open}`, a React boolean, while Base UI emits `data-open`
VALUELESS when open. Loosening the rule to `.reveal[data-open]` would match the **string `"false"`**
that React renders for `data-open={false}` — React stringifies booleans for `data-*` rather than
omitting them — and pin connection-block permanently open. **Caught by nothing:** jsdom does not
apply the stylesheet, and connection-block has no story for the visual suite to snapshot.

**Doc-first (§14):** no dependency conversation needed, because the row already anticipated this
("the primitives the app still hand-rolls … stop needing a §14 conversation each"). ⚠ But its list
was stale in **two** places — `menu` left it in V50b without the doc catching up, and `collapsible`
leaves it here. Both removed, with `combobox` recorded as deliberately kept rather than merely
omitted.

⚠ **KNOWN GAP, not fixed here.** `connection-block` still closes with the bare `.reveal`, so its
collapsed body — a Test action and a Fix link — **remains focusable and announced**. Same defect,
left alone because scope is one component; it has no story, so axe never sees it. This is the
strongest candidate for the next slice.


**V50b — the hand-rolled overlays fold onto the primitives (2026-08-08, branch
`feat/base-ui-v50b`).** Gate: `make fe` (**1221** app + 17 api + 51 core + 5 tokens, biome clean on
911 files + tsc + SPA build + storybook build) + `make fe-visual` (**764 passed, 0 failed, 0 flaky,
0 axe**, and **no baseline changed** — V50b touches no story or spec) + `make check` +
`retired-verify` (14 identifiers).

Five components, and **two of them were asserting accessibility they did not implement**. That is
the through-line: a hand-rolled overlay can spell `role="dialog" aria-modal="true"` into the DOM
without owning a single behaviour the role promises, and nothing in this repo's gate reads a role
and checks whether it is TRUE. Not types, not lint, and — the surprising one — not axe, which
validates that attributes are well-formed, not that they are honest.

- **`channel-row-menu` → Menu.** Deletes the app's only `createPortal`, a `getBoundingClientRect`
  layout effect, three hardcoded pixel constants, both-axis viewport clamping, flip-up-on-overflow,
  `invisible`-until-measured, a full-bleed `<button>` backdrop, capture-phase scroll/resize
  listeners, and hand-written menu roles. ⚠ Two were latent BUGS, not verbosity: `PANEL_MAX_H = 210`
  went stale the moment the menu's content changed (nothing enforced it, and the armed-confirm state
  is the tallest), and **dismiss-on-scroll was a workaround for an anchor the portal could not
  track** — the popup follows its trigger now instead of closing. Gains arrow-key nav, typeahead, a
  focus trap, focus restore and Escape, none of which it had.
- **`command-palette` → Dialog.** It claimed `aria-modal="true"` on a `fixed inset-0` div with no
  portal, no focus trap, no focus restore, no scroll lock and no inert background: a screen-reader
  user was told the page behind was inert while Tab walked straight into it, and closing dropped
  focus at the top of the document. ⚠ Escape moved WITH it — `useCommandShortcut` bound Escape at
  the window only because the palette had no dismiss of its own; keeping both would be two closers
  racing.
- **`restart-overlay` — ⚠ DELIBERATELY NOT AlertDialog, against the plan.** It made the same false
  claim, and the planned fix was to make the claim true. Reading the spec says otherwise:
  `alertdialog` is defined for interrupting with a REQUIRED RESPONSE and needs a focusable element,
  and this overlay has no interactive content in any of its three states. Worse, its "came back"
  state is deliberately `pointer-events-none` so a lingering confirmation cannot swallow the
  operator's next click — which a modal forbids. The false attributes are dropped instead: `status`
  (polite) normally, `alert` (assertive) on failure. **A real modal would have satisfied the audit
  and broken the interaction.**
- **`timeline-scrubber` → Tooltip positioner with a virtual anchor.** Deletes `CARD_WIDTH = 256`,
  which duplicated the `w-64` class and would decouple the moment either changed; `shift()` now
  keeps the card on screen against the VIEWPORT, measured rather than assumed, and the portal
  removes the `overflow:hidden` clipping risk.
- **`search-command` — ⚠ RE-EXAMINED AND DELIBERATELY KEPT**, with the reasoning written into the
  file so it is not re-litigated from scratch. The standing objection ("cmdk would need a §14
  change") dissolved when Base UI landed, but Autocomplete still does not fit on SHAPE, not effort:
  it is Portal→Positioner→Popup with no inline mode, and this is an always-visible panel embedded in
  six layouts that each position it themselves. The v2 mock decides it — the ⌘K block draws a modal
  overlay with the list in a centred card, which is what the Dialog port produces; a floating
  combobox would be moving AWAY from the mock.

⚠ **A hooks-order violation shipped in the first cut of the scrubber and is worth recording**,
because the test that should have caught it existed and could not. The anchor's `useMemo` sat BELOW
the empty-airings `return null`, so an empty render runs one fewer hook than a populated one and
React — which matches hooks by CALL ORDER — throws on a channel whose guide data empties out. The
existing "renders nothing when there are no airings" test mounts once with `[]`, so it never sees a
change in hook count and passes either way. **A test can cover a state and still not cover the
TRANSITION into it.** The added regression test rerenders one instance both ways; confirmed red
against the pre-fix component with the other three tests passing beside it. `trackW` left hover
state in the same pass — the clamp was its only reader, and a width still riding in state would read
as though the card bounds itself to the strip, which is exactly what stopped being true.

⚠ **The scrubber's visible behaviour change is NOT covered by the visual suite, and the reason
generalises**: near the track's ends the hover card now stops at the viewport edge rather than the
strip edge. No baseline moved — because the card only exists while hovering and the gallery never
hovers. Those stories cover the STRIP, not the card, and they would not have moved if the change
had broken it either.

**Test harness:** `asyncUtilTimeout` raised to 5s. Turning `getBy` into `findBy` across the
migration took the suite from 2 intermittent failures to 6 — every one passing in isolation, all CPU
contention across parallel specs rather than anything in the code. ⚠ A longer wait cannot make a
broken assertion pass; it only stops a red build that means nothing. Net effect: the two
PRE-EXISTING baseline flakes (help, filler) are green too.

**V50a — Radix → Base UI, the vendor consolidation (2026-08-08, branch `feat/base-ui-v50a`).**
Gate: `make fe` (**1219** app + 17 api + 51 core + 5 tokens, biome + tsc + SPA build + storybook
build) + `make fe-visual` (**760 passed, 0 failed, 0 flaky, 0 axe** on a CLEAN verify run, after a
reviewed 2-baseline update) + `make e2e` (7) + `make check` + `retired-verify` (14 identifiers).

Six `@radix-ui/*` packages → one `@base-ui/react`; **823 lines of transitive lockfile closure
deleted**. Doc-first per directive 1: §14's stack row keeps every primitive's ORIGINAL rationale
(Select-over-native, tooltip-over-`title=`, slider-owns-the-WAI-ARIA-contract, menu-is-not-a-select)
because only the vendor moved.

⚠ **Two user-visible regressions that a compile-clean port would have shipped.** Neither is caught
by types, lint, or axe:

1. **Every Select trigger would have shown its RAW VALUE.** Base UI resolves `<SelectValue>` to the
   item's label only when Root is given an `items` map; all 23 selects here pass options as inline
   `<SelectItem>` JSX. Triggers would read "240" where the list says "4 hours". Hand-writing
   `items={[…]}` per site would duplicate every label expression (several computed) — the
   two-lists-that-must-agree pattern this repo has been bitten by — so the wrapper DERIVES them
   from the items. Found only because the assertion was written before the port was believed.
2. **Tooltips lost their accessible description.** Base UI's tooltip is visual-only BY DESIGN — no
   `role="tooltip"`, no `aria-describedby`. Harmless for 36 of 37 consumers, whose trigger
   `aria-label` restates the content. Not for `FieldHelp`, which renders each setting's `doc`
   prose: a screen-reader user got "About Ordering, button" and nothing else.

⚠ **The durable half is the COMMENTS.** Three explained why something was the way it was, each was
true under Radix, and each would have silently become a lie: `field-help` claimed the SR user
"hears the same guidance"; `volume-control` put `aria-label` on the Thumb "because Radix puts
role=slider there" (still the right placement — Base UI nests an `<input type="range">` — but a
different reason); `setup.ts` credited five jsdom shims to Radix. Removing the shims one at a time
proved the pointer-capture trio was **dead weight** and that `scrollIntoView`, the one that matters,
was never Radix's need — it is our own `search-command`, and removing it fails 31 tests.

Other findings: Base UI mounts portalled popups **asynchronously**, so six tests needed `findBy`
over `getBy` (waiting, not weakening); `closeOnClick` defaults false where Radix closed on select
(preserved for the track picker); Menu has no standalone Label, so the heading now NAMES its group;
`asChild` renamed to `render` at all 11 sites, with both `@radix-ui` and `asChild` added to
`check-retired.sh` so neither vocabulary creeps back. **The single visual delta in 760 snapshots**
was the volume thumb at value 0 — Radix clamped it inside the track at the extremes, Base UI centres
it on the value; 54px, reviewed, baseline updated. Debt register 43 → 40 (tooltip, select, dialog).

**V41 — the audit pass: three live defects, six cleanups (2026-08-05, `aba3b22`, PRs #169–#175).**
Gate, per PR: `make check` (0 lint, `-race`) + `test-pg` (both dialects) + `openapi-verify` +
`retired-verify`; the web PRs additionally `make fe` (**1116 tests**, up from 1108) + `fe-visual`
(**716 passed, 0 axe**, on CLEAN verify runs, never `--update-snapshots`) + `make e2e` (7).

Started as "what needs refactoring?" and found working code was the smaller half of the answer.

⚠ **THE AI TAGGER HAD NEVER TAGGED A CLIP, and play counters had never moved** (#169). V38c split
clip identity (`Hash`) from location (`Path`); three callers kept passing the path into hash-keyed
store methods. `UpdateClipTags` returns `ErrNotFound` and the tagger treats that as **fatal**, so
every run aborted on its first taggable clip. Playout's `RecordClipPlay` swallowed the same miss as
telemetry. `PATCH /v1/filler/{id}` 404'd from the UI. `PodEntry` had no `Hash` field at all — which
is *why* playout had only a path to pass.

⚠ **The reason it survived two releases is the durable half.** `clipAt`/`sampleClip` (store) and
`untaggedClip` (filler) all set hash and path to the SAME STRING, so a hash-keyed call and a
path-keyed one were indistinguishable to every test. **This is the identical blind spot that already
shipped the `DeleteClipsNotIn` catastrophe** — the lesson was written into a comment and the fixture
was never fixed. All three now derive one field from the other and are never equal; `tagMemStore`
mirrors the real store's split (hash for tags, path for held) instead of accepting either. Making
them honest surfaced **four tagger tests passing for the wrong reason**, including one whose comment
documented the bug as the design, and a V28 play-count test still pinning the pre-V38c contract.

Also #169: the **visual gate was asserting an error page** — all four `ChecklistItem` stories × both
viewports were ONE byte-identical 1160-byte PNG of the words "Not Found", because `/wizard` was
missing from `RouterHarness`'s `NAV_PATHS`. The error page satisfies every wait the spec does and is
perfectly stable, so it passed forever. `RouterHarness` now **throws** on an unregistered path.
And five endpoints loaded the whole catalog to produce an integer (`CountClips`,
`CountClipsBySource`, `AutoFiledOnly`; `Pool` went from one full-table read *per channel* to one).

**The cleanups.** `recurate` was a **second lineup writer** — it trimmed `ch.Lineup` and persisted
the channel moments before the binder's additive union ran over the same field, the two ordered
against each other by a code comment (#171). Retirements now ride the proposal (`Proposal.Retired`)
and the binder applies them through the one `schedule.ApplyLineup` primitive; `UpsertChannel` is
**gone from `CuratorStore`**, so the two-writer arrangement is unexpressible rather than merely
discouraged. Incoming and Sources own their own queries (#170, #172). `/v1/suggestions` →
**`/v1/proposals`** (#172), the glossary's own rule finally followed — with three deliberate
survivors recorded in `CONTEXT.md` (the persisted job kind `"suggest"`, the SSE phase frame, the
`FeatureSuggestions` capability: all the verb, never the artifact). Dead `schedule/backfill.go`
deleted, two indexed lookups replacing full-table scans (migration `00037`), and `conformance.go`
split 2,650 lines → an 89-line runner + five domain files, **verified by diffing the subtest names
the suite actually runs: 39 before, 40 after, zero dropped** (#173).

⚠ **Every fix was sabotage-verified** — reintroduce the bug, confirm the tests go red. That is what
caught the near-miss: the first fixture fix left the suite GREEN, which proved the tests had never
exercised the distinction at all. It also stopped a wrong deletion (the shell's `confirmEra` looks
redundant but serves the Catalog's tag-cycling) and it is how the ffmpeg helper's **total absence of
coverage** was found (#174): the two language backends carried byte-identical copies of one ffmpeg
invocation, and changing `-ar 16000` to `-ar 8000` failed **nothing**.

**CI now installs ffmpeg** (#175), measured rather than assumed — a probe reported `ffmpeg: MISSING`
with three tests reporting SKIP while the job stayed green. Cached debs: ~38s → **~14s** on a hit.
⚠ Cache apt's **delta** (87 debs / 61MB), never the `--recurse` closure (298 packages / **244MB** —
it includes base packages the runner already has, and would cost more of the 10GB budget than it
saves). `--no-download` makes a broken cache FAIL rather than silently re-fetch.

**Deliberately NOT done, and each argued rather than skipped:** `app.go`'s 1,187 lines and
`api.Server`'s 44 fields (§14.1 examined and kept both; `architecture_test.go` encodes it), the 75
in-body `requireAdmin` calls (defence-in-depth on an authorization boundary — the wrong direction to
tidy), and extracting the Catalog tab (its query feeds the shell's own badge and it is what renders
when no other tab does, so extraction buys no runtime saving and costs a ~10-prop interface).

**V40 — language detection, two backends behind one seam (2026-08-05, `b7ef76a`, PR #167).** The
detection §10 designed and the V40 quality-gate entry below records as *unbuilt*. A scheduled job
over a ~10s span (never an inline scan pass — `whisper-cli` is ~3s natively but **~341s under
QEMU**), with two interchangeable backends: local `whisper-cli`, or an audio-capable hosted model
through the §8.1 provider. ⚠ The silence guard is the load-bearing part: a 978s recorded ad break
whose first 10s measure −70 LUFS was answered `ar` and **tombstoned**. Asked what language silence
is in, a model does not decline — it guesses, and re-asked it answered `en`. Two fixes: `LanguageSpan`
samples long recordings from the MIDDLE (where we look), and `spanIsSilent` refuses to ask below the
floor (holds wherever we land). Parser pinned to REAL whisper output, not remembered field names.

**V40 — the quality gate: reject the broken, normalise the quiet (2026-08-03, PR #166).** Gate:
`make check` (0 lint, `-race`) + the settings suite green **with ffmpeg hidden from PATH** + the
filler/playout/api suites. Automatic by maintainer decision — no badges, no review step, no
operator choice.

Two rejects at the scan boundary, so nothing downstream has to know: **shorter than 10s**
(`filler.min_duration`) and **no audio stream**. The floor exists because `DurationMs > 0` was the
only guard, and a **2.9KB / 33-millisecond truncated download passed it** — then sat `filed` and
airable in the dev catalog. Loudness is normalised **at playout** to −23 LUFS
(`filler.target_lufs`), verified end-to-end on real clips: −26.8 → **−23.4**, and −32.6 → **−23.1**
(a 9.5 dB correction landing within 0.1 dB of target).

⚠ **Normalisation never rewrites the file.** The clip folder holds files a person put there;
in-place is destructive and unrepeatable — the original is unrecoverable, and a re-scan cannot tell
it already happened, so a second pass would normalise an already-normalised file. ⚠ **Filler
only**: `Airing.Source` is the discriminator (set for a clip under `FILLER_DIR`, empty for a
library title), because normalising a feature film to advert loudness flattens its dynamic range.
⚠ **Single-pass**, despite ffmpeg preferring two — two-pass reads the whole file before emitting a
frame, which is fatal for a live stream.

⚠ **It also fixed a CI failure that had been red since V38c.**
`TestFeatures_UnsetToolPathsFallBackToPathLookup` faked `execProbe` but left the real
`exec.LookPath` running, so it asserted the **host's** PATH: green on any machine with ffmpeg
installed, red on every runner without it. Reproduced locally by hiding ffmpeg from PATH.

⚠ **`Probed.HasAudio` was the wrong polarity, and the lesson generalises.** `false` meaning
"reject" retroactively changed every `Probed{...}` literal written before the field existed — nine
test doubles that were correct when written began rejecting every clip, and the suite panicked on
an empty catalog. Renamed to `Silent` so the zero value is permissive. **A gate whose zero value
denies is a gate that breaks its own callers.**

**Deliberately not done: brightness.** Measured YAVG 64–127 against a mid-grey of 128, and the dim
end is what an eighties VHS transfer looks like — auto-brightening invents a picture the source
never had, and unlike loudness it is not recoverable at playout. **Language detection** is designed
in §10 but unbuilt: `whisper-cli` is vendored, but ~3s natively and **~341s under QEMU**, so it must
be a job over a ~10s span rather than an inline scan pass.

**V39 — hover previews and a clip player (2026-08-03, `6fd0147`, PR #164).** Animated WebP previews
generated **in the same ffmpeg pass as the still** (a `split` filter, one decode, ~0.47s/clip), a
`VideoPlayer` core primitive that knows nothing about clips, and a Radix Slider scrubber (§14)
replacing a hand-rolled `role="slider"`. ⚠ It surfaced that **`/v1/filler/media` had 404'd for every
clip since V38c** — the handler used `GetClip` (`WHERE hash = ?`) while its URL carries the sharded
*path*, and nothing noticed because no client called it until the player shipped.

**V38c — one pipeline in, content-addressed clips, path-based navigation (2026-08-02, `f058075`,
PR #162).** Gate: `make check` (0 lint, `-race`) + `test-pg` (both dialects) + `openapi-verify` +
`make fe` (biome + typecheck + **1076 tests**) + `fe-visual` (**706 passed, 0 axe**, on a CLEAN
verify run, not `--update-snapshots`) + `make e2e` (7). ⚠ ~~**Still open: V38c.8–9**~~ **UPDATED
2026-08-03 — V38c.8 shipped (`837d38b`, the seeded default sources) and loudness normalisation
shipped in V40.** Genuinely still open: **V38c.9** (the Add-a-source polish against the mock), plus
V38b's remaining carried list — the first-run starter pull, reel name/thumbnail, the Tune panel and
filmstrip. Amended rather than deleted for the reason the superseded "Next up" at the bottom of this
file records: a list of open items that outlives the work it names still reads as current.

Every source now takes **one route** into the catalog — arrive in the watch folder, hash, move
into the clip folder as `<hash>.<ext>`, sidecar, catalogue. Identity is a **sparse content hash**
(head 64KB + tail 64KB + size), the third identity change and the one that finally keys on
something Loomarr owns. Many watched folders made the path unusable: two folders each holding
`ads/coke.mp4` produced one identity and silently overwrote each other.

**Library sources are SCANNED again** (maintainer, reverses V35). §10 and §9.1 amended doc-first,
carrying the guardrails the reversal must not break — a library is one source among several and
never the catalog's only route, identity stays the hash rather than a media-server item id, clips
are copied into the clip folder rather than played in place, and a filler library is never offered
to the suggester as programme material.

⚠ **THREE BUGS THAT EVERY GREEN GATE MISSED, all found by running the real binary.** This is the
phase's most transferable lesson:

1. **`DeleteClipsNotIn` deleted the entire catalog on every sync.** It matched `WHERE path NOT IN
   (…)` while the sync passes HASHES; a hash never equals a path, so every clip counted as "not in
   the keep set". The route reported `1 added, 1 pruned` forever and held nothing — filler
   silently never worked. **The conformance suite passed throughout because its fixtures set
   `path` and `hash` to the same string.**
2. **A file dropped straight into `FILLER_DIR` was catalogued and then pruned in the same pass** —
   the documented drop-folder, and what every pre-V38c install does. `ClipPath`'s new allow-list
   accepts only hash paths, so a human-readable name was not a valid id. Every intake test missed
   it because they all dropped fixtures into the WATCH folder.
3. **`Sync` still keyed on `rc.Path`** after identity moved to `rc.ID`. Nothing caught it because
   `DirSource` fills both AND the in-memory double keyed on the path too — a double that models
   identity differently from the real thing tests the double.

⚠ **An unlayered CSS rule had disabled every accent border in the app.** `styles.css` set
`* { border-color: … }` outside any layer; Tailwind v4 sorts by LAYER before specificity, so it
beat `.border-signal` and every sibling across **26 files**. The active tab's amber underline
rendered grey. No gate could see it — the class was in the JSX, in the compiled CSS and on the
element, simply always losing — and **the visual baselines had been captured while it was broken,
so they encoded the grey as correct**. Found by diffing `getComputedStyle()` on the running app
against the same element in the rendered mock; ~330 regenerated baselines are that correction.

**Navigation is path-based app-wide** (maintainer): `/filler/sources`, `/queue/approval`,
`/channels/$id/<section>` join `/settings/<page>`; old `?tab=`/`?section=` links redirect. ⚠ Only
the TAB became a path segment — Filler's `q`/`kind`/`audience`/`view` stay in the query, because
they narrow the clip grid and nothing on the other tabs is a clip. `NavTabs` replaced `CountTabs`
and `ChannelNav`; tabs are real `<Link>`s with `aria-current`, **not** `role="tab"`, because these
navigate and the tab role would promise arrow-key behaviour navigation does not have.

**`fe-visual` earned its place twice.** It caught two real a11y bugs in components written this
phase — an invalid `aria-controls` at CRITICAL impact (copied from `CountTabs`, where tabs
genuinely controlled panels) and a 4.11:1 count pill (`signal` on a `signal` tint composites
amber-on-amber). ⚠ It also surfaced that the **e2e wizard baseline was long stale**, showing a
six-step flow with *Webhooks* and *Library* — both retired phases ago. The border diff only made a
dead snapshot fail loudly.

⚠ **Two deliberate departures from the mock**, recorded in the code so the next diff does not
"correct" them back: the switched-off source row keeps its text readable (the mock's
`opacity:.45` measures **2.95:1** against a required 4.5:1) and its `off` stat is not dimmed to
`#5A6170` (**3.14:1**). The mock is authoritative for look; an axe failure is a hard gate.

**A process note worth keeping:** `docker run -e CI` passes NOTHING when `CI` is unset on the
host, so `playwright.config.ts`'s `workers: cpus().length` never applies locally and the visual
suite runs at half the cores. Unfixed here deliberately — a one-line Makefile change that deserves
its own commit and a single timed run.

**V38b — clips arrive from SOURCES, not from a URL box (2026-08-02).** Registered sources are now
polled on a schedule and new clips download unattended, bounded by four configurable limits. The
paste-a-URL panel is **deleted**. ⚠ **Still open: the first-run starter pull, loudness
normalisation, reel name/thumbnail, the Tune panel and filmstrip.**

**This phase started from a user question, not a plan.** The Incoming tab said *"Everything
downloaded so far has been filed"* directly above a panel saying *"Downloading isn't available in
this install"* — two claims that cannot both be true. Pulling that thread found a real defect
underneath.

⚠ **ONE `ingest` flag gated TWO downloaders with different needs.** archive.org is fetched over
plain HTTP and needs only ffmpeg; yt-dlp is for YouTube. Requiring both meant a box with ffmpeg
and no yt-dlp reported "downloading unavailable" **while being perfectly able to fetch** — and the
starter pull is an archive.org collection, so first-run acquisition was blocked by a binary it
never invokes. Same shape as V37's `Fetchable()`: an invariant true when written ("every ingest
needs yt-dlp") that quietly stopped holding when a second downloader landed beside it.

⚠ **The test defended the bug.** `TestFeatures_IngestNeedsBothToolsPresent` asserted *both*
directions. One is a real dependency and is documented — yt-dlp shells out to ffmpeg to merge
streams. The mirror case had **no comment and no argument**: it was asserted by symmetry. Now
split, and sabotage-verified — re-coupling them fails naming the defect.

**Also found while checking:** §15 has always said the tool paths "default to the vendored
binaries"; the registry defaults were `""` and only the Docker image set them, so **every source
build had ingest off even with the tools installed**. Unset now falls back to a PATH lookup, which
makes the documented behaviour true.

**Auto-fetch, and what bounds it (maintainer, 2026-08-02).** §15's *"there is no unattended
crawler"* is **superseded**: a registered, enabled source is polled and new items download without
anyone asking. ⚠ **The superseded rule's concern was legitimate, so it survives as LIMITS rather
than as a prohibition** — `filler.fetch.every` (6h, `0` = off), `max_per_run` (10),
`max_catalog_clips` (2000), `max_disk_gb` (20). Every one fails toward doing less.

**Three properties, each sabotage-verified:** only **enabled** sources are polled (the Sources
switch already promises it); everything fetched arrives **held**, so the unattended step is
*acquisition* and never *admission*; and a limit that stops a pass is **reported**, because a
crawler that quietly does nothing is indistinguishable from one that is broken.

⚠ **Dedupe has no cursor, deliberately.** archive.org discovery is a search, not a feed, so "new"
can only mean "not already here" — the catalog is the high-water mark. It compares **basenames
without extension**, because a downloaded clip lands as `<id>.mp4` and comparing raw strings
against bare archive ids would match nothing and re-download everything on every pass. And the
catalog read must include **held** clips or the job re-downloads its own review queue forever.

**The URL box is retired**, not moved. Clips arrive because you added a source — registration,
per-row search, an approved pull, or auto-fetch — and a box that made you find the URL yourself
was the odd one out. Its two end-to-end tests were **deleted rather than relocated**: the
behaviour is gone, and a test kept alive against a deleted surface passes by rendering something
nobody can reach. `check-retired.sh` now guards the name.

⚠ **A bug I introduced and the maintainer's question surfaced.** Asked whether the Sources panel
was being redesigned too (it was not — V37 did that), checking found the fetcher polled sources
and **never stamped them**: `MarkFillerSourceFetched` shipped in V33, is in the store interface,
and had **no production caller**. Every row would have read *"never fetched"* forever while
auto-fetch downloaded from it every six hours. Now stamped — ⚠ only after a successful queue AND
only when something was actually queued, because "last fetched" must mean "last brought something
in" or a source polled fruitlessly for a week reads as freshly productive. Sabotage-verified.

**Two guards that fired usefully:** `Resolve` **panics** on an undeclared `ScheduleKey`, so
registering a job without its settings row takes the app down at boot — fail-fast, caught by
`make check`. And `TestJobSet` pins the whole job roster, so **the first job that reaches out to
the internet unattended could not be added quietly**; its row is now the record that §15's old
claim no longer holds.

Gate: `make check` GREEN (`-race`) · `make fe` GREEN (1050 tests) · **`fe-visual` 690 passed, 4
modified baselines** (the corrected empty state), verified without `--update-snapshots` ·
`make e2e` 7/7 · `retired-verify` clean (6 identifiers) · `config-docs` regenerated.

**V38 — the confidence spine (2026-08-02).** Clips gain a **lifecycle**: an ingested clip is
**held** — recorded but not in the playable catalog, not matched into a pod, not counted as
coverage — until it is filed, by a human or by clearing a confidence threshold. Incoming reads
held clips, files them (singly or **all-as-suggested**), and shows **what was filed without
asking** with a one-click undo. ⚠ **Still open: loudness normalisation, reel name/thumbnail, the
Tune panel and filmstrip.**

**Maintainer calls:** grounding-gated score with the model refining **within the cap**;
auto-filing **ON at 85**; and — after I found the shipped pipeline had no filing step at all —
**build the real hold-then-file pipeline** rather than repurposing a heuristic.

⚠ **I mis-scoped this twice, and both corrections matter more than the code.** First I planned to
gate "filing" without checking one existed: the scan catalogued a clip and the tagger tagged it
**in place**, so nothing was ever held. Then I warned that auto-filing ON meant clips reaching
channels unreviewed — **backwards**: before V38 an ingested clip was catalogued and playable
*immediately*, no score, no gate. Auto-filing at 85 is the **first** thing ever to stand between a
download and a channel. §10 records both.

**Six safety properties, each sabotage-verified:**

1. ⚠ **An ungrounded era cannot be auto-filed at ANY settable threshold.** Grounding facts set a
   **ceiling**; the model may only lower it. Verified twice — as arithmetic, and end-to-end
   through the filing decision, because **a correct cap that nothing consults protects nothing**.
2. ⚠ **Held clips never reach a pod** — excluded at the one `ListClips` chokepoint, the shape
   V35's tombstone established, because pod assembly reads it with a zero filter.
3. ⚠ **A re-scan cannot file a held clip** — omitted from `UpsertClip`'s `DO UPDATE` *and*
   preserved in the sync merge. One scan pass would otherwise empty the queue into live channels.
4. ⚠ **The policy fails CLOSED** — `boolv`, not `boolOn`. They differ only when settings cannot
   answer, and there `boolOn` would publish unreviewed clips exactly when the install is degraded.
5. ⚠ **Hand-filing CLEARS `auto_filed`** — a human looked, and leaving the marker set makes a
   reviewed clip indistinguishable from an unreviewed one.
6. ⚠ **Confirm-as-suggested carries all three tags** — `UpdateClipTags` writes era, audience and
   category unconditionally, so an era-only write blanks the other two. Second call site to hit
   this; the first is commented in `filler-page.tsx`, which is now a strong argument that the
   store method's all-columns behaviour is the real smell.

**The lifecycle fork needed a maintainer decision.** Ingest downloads into the *same folder the
scan watches*, so at catalogue time a fetched clip and a hand-copied one are both just files. The
`.info.json` sidecar is the signal: sidecar ⇒ Loomarr fetched it ⇒ hold; none ⇒ a person put it
there ⇒ file on sight. Holding a hand-copied clip is the ceremony §7 warns teaches people to click
through gates.

**Five defects the tooling caught — three of them mine:**

- ⚠ **A test that passed under sabotage.** `TestSync_ReScanDoesNotReHoldAFiledClip` went green with
  the preserve replaced by a recompute: `serverFieldsUnchanged` skips an unchanged clip *before any
  write*, so the second sync never exercised the merge. It now forces a write. **The protection was
  real but came from the idempotent skip, not the line the test claimed to cover** — false
  assurance that would have shipped.
- ⚠ **A comment that disagreed with its own code** — I wrote that `confidence` was omitted from
  `DO UPDATE` while the SQL still updated it. Only executing it found that.
- ⚠ **`boolToInt` into a Postgres BOOLEAN — the third dialect trap this session.** `ai_tagged` uses
  that helper safely because its column is INTEGER in *both*. **The split is per COLUMN.**
- ⚠ **`filler.autofile.normalize_loudness` shipped declared-but-unconsumed** and I caught it in the
  final gate. `make config-docs` published a key nothing reads — the exact defect that got these
  keys removed in V35's review, **re-committed inside the phase that documents the lesson**.
  Removed; it lands with its ffmpeg pass.
- ⚠ **Every filler duration in Incoming rendered "0m"** — `formatDuration` is minute-granular
  (programme runtimes); clips are 5–60 seconds. `formatClipDuration` already existed and is what
  the card and row use. No test asserted on it, and "0m" is a plausible-looking string; only the
  baseline image showed it.

⚠ **And one thing I got wrong twice about the confidence bar**: I read three scores as rendering
identically and "fixed" it twice before measuring the pixels — 17px, 30px, 38px for 40/72/92. It
was correct all along; a 40×1px bar simply reads as a band. What the measurement *did* find is
that a native `<meter>` cannot be themed (it drew the browser's green, not `lock`), which is why
it is a `role="meter"` div. **Reading an image is evidence; guessing from one is not.**

Also: `ListUntaggedCommercials` needs `IncludeHeld` — held clips are precisely the ones needing
tags, and without it the tagger skips every fresh download and the threshold appears inert. And
the reachability stub had **no `/v1/filler/incoming` branch**, so the tab tested the catch-all —
the second time that stub has answered the wrong shape (V35b found the first).

Gate: `make check` GREEN (`-race`) · `test-pg` GREEN (00030/00031 both dialects) · `make fe` GREEN
(**1053** tests) · **`fe-visual` 690 passed, 6 new baselines**, verified without
`--update-snapshots` · `make e2e` 7/7 · `config-docs` regenerated · `openapi-verify` clean once
committed.

**V37 — sources as one flat list, and YouTube becomes real (2026-08-02).** Every source is one
row in one list: the drop-folder, the media-server library, each archive collection, each YouTube
playlist. The `remote` CONTAINER row is retired, `POST /v1/filler/sources` takes a **kind**, and a
registered source carries its own switch, fetch time and Remove.

⚠ **This SUPERSEDES a rule that was load-bearing, so the doc records what survives it.** §10 said
the folder is derived-from-config and remotes are rows, *"and that asymmetry is deliberate and is
why one source never appears twice."* Flattening does not dissolve that concern — it moves it, so
§10's new *"Sources are one flat list"* names the two properties the flat model must carry itself:
**"not configured" stays expressible** (the config kinds are rows even when unset — §10's own
answer to *"why is my catalog empty?"*, which a table of things-that-exist cannot give), and **no
source appears twice** (those kinds are singletons). Doc-first, before any code.

**Five findings, three of them defects the tooling caught:**

1. ⚠ **A pull would have enqueued a fetch of an empty URL.** Both pull paths filtered on
   `Enabled` alone, which was correct while the table held *only* fetchable remotes — my seeded
   singletons are enabled, so a plan gained a folder row whose `uri` is `""`, and approval handed
   `Ingest` a blank string. **The invariant "every row here is fetchable" was implicit and became
   false the moment the table flattened.** Now explicit as `FillerSource.Fetchable()`, used by
   propose *and* the approve-time re-check so the two cannot disagree.
2. ⚠ **`enabled` is BOOLEAN on Postgres and INTEGER on SQLite** — a literal `1` copied across is
   a hard 42804 at migrate time. Invisible reading the two files side by side; `test-pg` caught
   it. The good failure mode: it cannot reach an install half-applied.
3. ⚠ **The singleton guard lives in the DATABASE, not in Go.** A partial unique index on
   `kind IN ('folder','library')`. Sabotage-verified — dropping `UNIQUE` fails the conformance
   suite with *"a SECOND folder row was accepted"*. A Go-only guard is one the next caller
   forgets, and the failure mode is two drop-folder rows with different switches: exactly the
   precedence question the superseded model refused to have.
4. ⚠ **Registration is deliberately STRICTER than ingest.** `clipfetch.KindForURL` defaults an
   unknown host to YouTube because yt-dlp handles the widest set of sites — right for an admin
   pasting one URL and watching it work, wrong for a row that persists and fetches unattended.
   The host check is **exact**, not `Contains`: sabotage proved a substring check registers both
   `youtube.com.evil.test` and `notyoutube.com`.
5. ⚠ **The on/off switch had never been in a visual baseline.** No story passed
   `onToggleEnabled`, so the tab's most consequential control — whose copy is a behaviour claim —
   was unrendered in every snapshot. Pre-existing, not a V37 regression; closed by the new
   `WithRegisteredSources` story, and found by *looking at the picture*.

Also: the row id is namespaced (`archive:…` / `youtube:…`) because one table now holds two
vocabularies and a bare identifier lets one kind silently UPSERT over another's; `searchable` is
**server-sent** rather than re-derived client-side (V35b hard-coded `kind === "remote"` and
predicted this); and the page's Add-a-source form closes a route that had been **API-only for two
phases** — `POST /v1/filler/sources` shipped in V35 with nothing calling it.

**Deferred, deliberately: community packs.** There is no pack index — no URL, no manifest, no
fetcher — so a `PACKS` row would be the "control that dims and changes nothing" §10 forbids two
paragraphs above. **Licence stays off the wire** (maintainer, 2026-08-02): §6.3 measured ~92% of
archive items declaring none, and a badge absent nine rows in ten teaches an operator to read
absence as "fine" rather than "unknown".

Gate: `make check` GREEN (`-race`) · `test-pg` GREEN (00029 conformant on **both** dialects) ·
`make fe` GREEN (1049 tests) · **`fe-visual` 686 passed, 2 new baselines and 10 modified**,
verified by re-running **without** `--update-snapshots` · `make e2e` 7/7 · `config-docs` no drift ·
`openapi-verify` clean once committed (spec verified stable across regenerations).

⚠ **`retired-verify` fails on an UNTRACKED local `docker/.env`** holding a generated
`WEBHOOK_SECRET` from an old dev run. Not V37 and not something CI sees — the check greps the
working tree, not the index. Verified clean with that file moved aside (5 identifiers checked).

**V35b — the honest frontend delta (2026-08-02).** The part of the redesigned Filler page that
needed no backend: the catalog's **list ⇄ grid** toggle (`?view=list`, new `ClipRow`), the card's
**thumbnail overlays** (duration / quality / select move onto the frame), **Select all**, and the
per-source search **re-scoped into the archive row** behind a *Search it* toggle — finding clips is
something you do TO a source, so it no longer sits in a detached card.

**Four things the tooling caught that no assertion would have:**

1. ⚠ **`/v1/filler/sources` was never stubbed in the reachability suite** — it fell through to the
   clips payload, so the Sources tab rendered **zero source rows** in every run, and the old
   *"Find clips"* assertion passed anyway because that heading sat OUTSIDE the list. Moving the
   search INTO the row is what made the test depend on the shape. **A stub answering the wrong
   shape is indistinguishable from a working page until something depends on it.**
2. ⚠ **The list row crushed the clip name.** A grid item's default `min-width:auto` refuses to
   shrink below its content, so a long name pushed the tag columns out instead of ellipsising —
   the one column that must never be squeezed. Every test passed; **only reading the baseline
   image showed it.** Fixed with `min-w-0` and pinned by a `LongName` story.
3. ⚠ **`aria-label` on a bare `<span>` is ignored.** The quality overlay carried one to stop
   "1080P" being announced as letter-spaced shouting; ARIA only permits naming on an element with
   a role, and the chip-row version got its role from `Badge`. Biome caught it — a real a11y bug,
   not a lint nit. `role="img"` now carries the name.
4. ⚠ **A `widthFrame(360)` story does NOT test a mobile layout.** Tailwind's `md:`/`lg:` resolve
   against the VIEWPORT, so the decorator narrows the container and changes which columns render
   not at all — the story drew a squashed desktop row that *looked* like proof. Deleted (with its
   two orphaned baselines, which `--update-snapshots` writes but never prunes); the visual suite's
   mobile viewport is what actually exercises the collapse.

⚠ **The mock's `searchable: id === 'archive'` could not be followed literally** — archive.org has
no peer row today; it lives nested under `remote`. The expander is a **render prop keyed by row**,
so V37's flattening changes which row matches and not this shape.

**Not adopted from the mock, deliberately:** list rows carry **no per-row actions**. List view is
for scanning and bulk-selecting; acting on one clip is what the card is for, and rebuilding it
badly in 46px of column would be worse than the split the mock draws.

Gate: `make fe` GREEN (**1047** tests, up from 1039) · `fe-visual` **684 passed, 14 new baselines
and 2 modified** (both the intended ClipCard overlay change), verified by re-running **without**
`--update-snapshots` so the axe half ran · `make check` GREEN.

**The maintainer export landed, and it split the Filler track (2026-08-02).**
`design/loomarr-prototype-desktop-v2.dc.html` is now the **632,110-byte** export — the one the
V35 row below says was needed. It carries the redesigned Filler screen *and* the ~270 KB of JS
every previous fetch truncated. **Next up: V35b, then V37 and V38** (plan §6.5).

⚠ **Reading the markup alone had understated one screen.** The confidence meter looked like
layout; the JS shows `conf >= autoConf` is the **routing decision** for every incoming segment —
what gets filed silently vs. surfaced to a human. So the Incoming redesign is **not frontend
work**: the score is a backend capability the UI only reports. It became its own phase (V38)
rather than a step inside a page phase.

⚠ **Two verification lessons, both about the truncated fetch rather than the design.** (1)
`support.js` was recorded as byte-identical in three places; it was **not** (64,222 → 69,150
bytes, 155 lines). The check was real but ran against the capped read — **a hash taken from a
truncated fetch describes the cap, not the file.** (2) The "do not invent `poolStats`/`asks`/
`reels`/`services`" instruction was correct while the JS was unreachable and is now **retired**,
because those values are readable at source. Both corrected in `design/README.md`,
`design/FILLER-DELTA-2026-08-01.md` and plan §6.5.

⚠ **1.7 was recorded as NOT DONE and that was wrong** — see the corrected row below. The work sat
uncommitted in the tree, so the record and the code disagreed in the direction that costs most:
the plan would have re-specified a built feature.

**Three ratified decisions (maintainer, 2026-08-02)** now in plan §6.5: **follow the mock's
design**; **YouTube + community packs become real registrable sources** (V37 — a schema and
contract change, not a restyle); and **the tagger records a confidence score** (V38 — the
dependency the whole Incoming redesign rests on).

**V35 — the Filler page as redesigned (2026-08-01).**
Branch `feat/filler-clip-card-detail`, 14 commits. The design project's prototype was re-fetched
and the Filler screen had been **rewritten** — 4 tabs → 3, Discover deleted into Sources, a new
Incoming tab, and filler acquisition behind an **approval gate**. Plan §6.5 specs it; the
structure is in `design/FILLER-DELTA-2026-08-01.md`.

⚠ **It invalidated an audit written the same day** (the "Filler UI is behind its own backend"
block below): **five items retired, three reshaped, two survive**. That block is now annotated
rather than deleted, because the lesson is the durable part — an audit is a claim about a moving
reference, and this one had a shelf life of one day.

~~⚠ **`design/loomarr-prototype-desktop-v2.dc.html` is STALE for this screen and could not be
updated.**~~ ✅ **RESOLVED 2026-08-02 — the export landed** (see the top entry). The cap that
caused it was real and is worth keeping: `DesignSync.get_file` stops at 262,144 bytes, the file is
~310 KB of markup before its JS, and a markup-only splice renders nothing. ⚠ The last sentence of
this block — `support.js` "byte-identical, so no runtime change accompanies it" — was **wrong**,
because it was verified against the truncated fetch.

**Shipped (backend + contract):**

| | |
| --- | --- |
| **1.1 Pool health** | `GET /v1/filler/pool`. ⚠ Per-channel numbers **ARE** `Coverage`, called once per live channel — no aggregate ladder. Sabotage: giving it its own opinion fails on all 3 channels |
| **1.2 Clip media** | `GET /v1/filler/media/{path...}`. Two gates the mock cannot show: a **catalog** check (the drop-folder is not a public share) and an **extension allowlist** (serving `mime.TypeByExtension` from an operator-writable folder is stored XSS from our own origin) |
| **1.3 Source switch** | Migration `00026` + `filler.source.folder.enabled`. Gate lives in the **Syncer**, not its three callers; `ErrSourceDisabled` → a 409 naming the switch |
| **1.5 Pull proposals** | Migration `00027`. **The approval gate for filler acquisition** — §10 stated "the machine proposes, a human commits" with nothing to hang it on; a pull is that object |
| **Step 2** | `make openapi` + orval regen; the typecheck caught every hand-built `FillerSourceDTO` fixture, which is the contract-1:1 principle paying out |

⚠ **Three findings that changed the design rather than the code.** (1) §15 already specified
`FILLER_SOURCE_LIBRARY_ENABLED`; implementing it found **there is no scan to gate** — §10 took the
media server out of the filler path — so the key was removed doc-first rather than shipped as a
control that dims a row and changes nothing. (2) The first draft of `00026` put `enabled` in the
upsert's `DO UPDATE` list: a Go bool zero-values to **false**, so any existing caller re-registering
a source would have silently switched it **off**. (3) `resolved.boolv` answers false when settings
cannot answer, which would have stopped the catalog scan on a degraded boot — `boolOn` fails **open**
for keys whose declared default is true.

**Every safety property is sabotage-verified**, not assumed: proposing that downloads, a
double-approve, an aggregate with its own ladder, a gate placed below the scan, an upsert that
flips the switch, and an allowlist replaced by a mime lookup each fail a named test.

**Also shipped (frontend + the two remaining backend items):**

| | |
| --- | --- |
| **1.4 Incoming** | `GET /v1/filler/incoming` + the tab. ⚠ **No confidence score** — the mock draws a bar per row; the tagger records neither a score nor a rationale, so each row carries the REASON it is waiting, derived from real state. `filler.autofile.*` is the knob that would need one and is not built |
| **1.6 Bulk ops** | The blocked decision, answered: **a tombstone**. Migration `00028`. Not a row delete (`clips` is a synced cache, so the next scan puts it back) and not a file delete (nothing here deletes an operator's media). The UI says "Remove from catalog" and the files stay |
| **Step 3** | Three tabs under the pool strip; Discover **retired**; bulk selection; the `FILLER PULL` card on the Queue; source switches |

⚠ **Discover is gone as a tab**, and its query state was deleted rather than left wired to
nothing. The ingest panel moved to Incoming, the tab about how clips *arrive*. An old
`?tab=discover` bookmark falls back to Catalog.

⚠ **The reachability suite earned its keep again**: retiring a tab broke four assertions pointing
at `?tab=discover` — exactly the "a tab nobody can navigate to passes as reachable" failure it
exists for. It now also asserts the health strip is present, since a strip has no nav entry.

**Three defects the tooling caught, each worth the note:**

1. `pluralize()` already emits the count, so the first draft printed numbers twice ("of 90 90
   commercials") — and I repeated it in a second spot before noticing.
2. `onConfirmEra` sent a bare `{era}`. The comment **directly above the mutation** warns that
   `UpdateClipTags` writes all three tag columns unconditionally, so that wipes audience and
   category.
3. Two comboboxes named "Audience" on one page (the filter bar already owned the name) — a real
   ambiguity for anyone on a keyboard, surfaced as "found multiple elements". They are "Set
   era/audience/category" now, which also separates the two jobs.

Plus an a11y finding: `role="switch"` **replaces** the native checkbox semantics, so `checked`
alone leaves the control announcing no state; `aria-checked` is explicit.

**NOT DONE, and deliberately so:**

- ~~**1.7 per-channel include-set override + `fitNote`**~~ **DONE — this row was stale, not
  accurate (corrected 2026-08-02).** `components/loomarr/filler/channel-override-picker/`
  implements exactly the mock's shape: one checkbox per channel, a `fitNote` per row, and **Back
  to automatic** as the route to the third state. It is reachable through the rewritten
  `pin-clip-dialog` (no longer pin-only — it can exclude and un-pin), which `filler-page` renders,
  and `reachability.test.tsx` asserts the entry point. ⚠ It sat **uncommitted** while the record
  said NOT DONE, which is the direction of drift that costs most: the next phase would have
  re-specified a feature that was already built. Commit `8af15e4` carries the backend half
  (`GET /v1/filler/fit`, `ChannelFitDTO` with a reason **code** the frontend words).
- ~~**The card's cycle → "Edit tags" swap.**~~ **DECIDED: not doing it** (maintainer,
  2026-08-01). Click-to-cycle stays. It is faster for the common case (one wrong tag on one
  clip), and the select editor's real advantage — several fields across several clips — is what
  V35's **bulk bar** now covers from a selection, which a per-card editor never could. Recorded
  in `design/FILLER-DELTA-2026-08-01.md` so nobody "fixes" the code back to the drawing. ⚠ The
  general rule: **a mock is authoritative for what a screen SHOWS, not for deleting a working
  interaction it happens not to draw.**
- **Sources: per-source inline search and "Add a source" UI.** The routes exist
  (`POST` / `DELETE /v1/filler/sources`, `GET /v1/filler/discover`); nothing on the tab calls
  them, so all three are **API-only** for now. ⚠ `DiscoverPanel` was **deleted** rather than
  left orphaned — the redesigned per-source search is a different shape (a result row carries
  `date · duration · quality` and a *Queue download*), so it was not reusable.
- **The pull composer is a stub, and says so.** One plan row per enabled source, a constant
  `why`, `estimateClips: 0`, and the operator's `note` is **stored but never reaches `Ingest`**.
  A real forecast needs an upstream search per row at propose time. The gate around it is
  complete; what it proposes is not yet clever.
- **`filler.starter_collection` is declared and read by nothing** — the declared-but-unconsumed
  defect V12's row says blocked that phase. It predates V35 (`03116f0`) but is now §10's
  business, since §10 says it seeds a pull.
- **Live verification** on the maintainer's stack.

### Review findings, fixed (2026-08-01)

`/code-review` over the branch, two axes in parallel. **Every safety property verified good** —
no second ladder, a pending pull writes only, the tombstone holds end to end, no test neutered.
Six real defects, all fixed here:

1. ⚠ **A dead control, found INDEPENDENTLY BY BOTH AXES** — the empty catalog's "Find clips"
   navigated to `tab: "discover"`, a tab this phase retired. `validateSearch` drops the unknown
   value, so the button landed back on the empty catalog it was offered from. **Deleting a
   destination is not done until every route TO it is gone, and a nav target is not
   type-checked.** It is now *Propose a pull*, calling the same mutation the strip does.
2. **§15 declared `FILLER_AUTOFILE_MIN_CONFIDENCE` / `_DROP_BELOW`; neither is in the registry** —
   against §15's own rule that a setting not in the registry does not exist, and against §10 four
   paragraphs earlier saying autofile is not built. Removed from the doc.
3. **§10 over-claimed the source switch** ("not scanned, not searched, and not downloaded from").
   Only the scan and the pull path are gated, and the other two routes are *not source-scoped* —
   keyword discovery searches the Archive globally, and ingest takes a URL the operator typed.
   Both are the ratified single-item-direct path. The claim is narrowed rather than the routes
   gated.
4. **Migration `00026`'s comment named `filler.source.library.enabled`**, a key dropped later in
   the same phase once implementing it found no scan to gate — a permanent contradiction in a
   forward-only file. Corrected in both dialects (comment only; no schema change).
5. **`DiscoverPanel` was orphaned** — barrel-exported, storied, baselined, rendered nowhere: the
   exact "built and imported by nothing" state the reachability suite exists to catch. Deleted
   with its 10 baselines.
6. **Three stories hand-rolled DTOs**, against `frontend-design.md` §5.1b. Moved to
   `@loomarr/fixtures`; the migration was content-preserving, which the visual suite proved by
   moving **zero** of their baselines.

Plus a defensive nil-store guard in `fillermedia.go`. ⚠ Not a live panic — `liveConfig` is nil
exactly when the store is, so the handler already 404s — but that coupling was true by
construction and stated nowhere, and every sibling guards the store directly.

⚠ **One reviewer finding was MISATTRIBUTED and is recorded as such**: the "scope creep" of the
starter pack and the `filler.dir` default flip both come from `03116f0`, which predates V35. The
Spec axis reviewed `main...HEAD`, which includes six earlier commits on this branch.

Gate at `8b87703`: `make check` GREEN (`-race`) · `test-pg` GREEN (`00026`–`00028` conformant on
both backends) · `make fe` GREEN (1018 tests) · **`fe-visual` 658 passed, 26 NEW baselines and
zero modified**, verified by re-running *without* `--update-snapshots` · `make e2e` 7/7 ·
`openapi-verify` / `config-docs` / `retired-verify` no drift.

⚠ **A pre-existing flake, seen once:** `TestGuide_GapsArePreservedNotFiltered` failed with a 1ms
timeline hole (`block 1 starts at …240 but 0 stopped at …239`) and passed on five consecutive
re-runs. Unrelated to this phase — no guide code changed — but recorded so the next person to see
it knows it is not new.

**V12 — System → Backup + About (2026-07-29).** The two sub-tabs V9 deliberately left out,
plus the scheduled backup job behind them. The phase could not start until a doc conflict was
resolved: the gate said *"retention honored"* while `design.md:1397` deferred scheduled
backups to §20 and recorded `backup.schedule`/`backup.retain` as declared-but-unconsumed —
which they had been since V4, read by nothing. Shipping the page over dead settings would have
made its own footer ("nightly at 03:30 · keeps 7") a false claim, so the keys are consumed and
§16/§18.1/§7/§20 were amended first. **Three things that are safety properties, not features:**
retention filters on the writer's own filename pattern (`backup.dir` may hold the operator's
files — without the filter the prune takes their photos *and* the live database); prune runs
*after* a successful write (pruning first leaves fewer backups than it started with when the
snapshot fails); and the download's client-supplied filename is validated before it reaches the
filesystem (without it, `../loomarr.db` serves the live database). Each verified by sabotage,
the traversal one against the real service rather than a fake. Full row below.

**V31/V32 — the Dashboard's last two panels (2026-07-29).** Services (what is broken) and
Recent activity (what happened). Two maintainer questions shaped both. First: where do feed
rows come from? Answer — each domain **transition**, not the event bus, which `bus.go` itself
documents as lossy ("a dropped event is a latency bug"); a feed built there loses rows exactly
when the install is busiest. Second: why does Services **poll** rather than use SSE? Answer —
a probe result is not an event anyone observes (the server only learns it by making six
outbound calls, **730ms measured**), so pushing it would mean probing forever on an idle box.
But asking flipped the *other* panel: the feed IS event-shaped, so it now takes an `activity`
frame and polls not at all. ⚠ A wrapped "14m ago" made rows uneven and **every test stayed
green** — caught only by reading the baseline image. Full row below.

**V13 — restart in place, and Windows decided the mechanism (2026-07-29).** Loomarr restarts
by rebuilding itself in the same process: `for { app := Build(); app.Run(); app.Shutdown() }`.
Same PID, no supervisor, **identical on Windows** — which is the whole reason it is not
`syscall.Exec`. That was the settled choice until the maintainer asked how Windows would work:
`exec_windows.go` returns `EWINDOWS`, so it **compiles cleanly and fails only at runtime**, and
the button would have shipped broken. Jellyfin takes the same in-process approach for the same
reason. ⚠ The live test then caught a package-level `startedAt` **I had added in V12** that
survived the rebuild — About would have claimed days of uptime on an instance restarted seconds
ago, silently. Three maintainer corrections each *removed* something: Reload (settings already
hot-apply on save), the five-row consequence list (one line now), and full interactivity during
a restart (the app dims and blocks input app-wide). Full row below.

✅ **CI is running again** (2026-07-29). The GitHub Actions billing block recorded against
Phases 4–5 and several v2 rows below is **resolved** — runs on `main` are green, so a PR goes
through real CI rather than the merge-on-green-local exception those older rows describe.
Treat every "billing-blocked" note below as history, not as the current state.

**Three follow-ups the same day, two of them maintainer corrections to my judgment.** (1) I
argued the backup job should stay *unregistered* on Postgres since three other surfaces
explain `pg_dump`; the maintainer overruled it, correctly — **an omitted row is also a
claim**, indistinguishable on the Tasks page from a job that runs fine and has never failed.
`DisabledReason` now states it, enforced at four points in the scheduler rather than by UI
convention. (2) I shipped About *without* the mock's Go-runtime/uptime/schema rows because
the endpoint could not fill them; the maintainer asked for the fields instead, so
`/v1/system/version` now carries them — ⚠ `startedAt` as an **instant**, never a
pre-computed uptime. (3) Chasing "why don't I see this at `:5173`" found the backend was an
**orphaned `go run` binary serving pre-change code for hours**, and that `.air.toml` had been
committed since July with Air never installed *and* pointed at a database that does not
exist. All three rows below.

**V14 remainder + the Guide's 17× latency fix (2026-07-26).** Branch
`feat/guide-fold-and-dev-login`, 11 commits. Two threads: finish the IA fold V14 left open,
then make the resulting surface fast enough to use.

**The fold (V14's remaining gate, C8 answered).** `/channels` and `/suggest` are DELETED;
`/guide` is the channels surface, headed "Channels", owning origination via `✦ Add a channel`
in its header. Admin nav 9 → 7, matching the v2 mock's `navDefs` exactly; member nav 4 → 3
(`Request a channel` was a nav entry only while `/suggest` was its own page — a second link to
`/guide` would be a duplicate React key, not an IA choice). Members get the header affordance,
labelled for them, since it is now the only origination door in the app.

This had been blocked for several phases on one stated reason, recorded in four places: "the
grid has no origination affordance yet". **The mock always had one** — its Guide screen is
headed "Channels" and carries the button. Nothing had ported it; the blocker was a reading
error, not a missing decision. §12 amended doc-first (the ⚠ note said "this line comes out with
it"). **C8** is now recorded as API-ONLY BY DECISION rather than left orphaned. Also fixed in
passing: `router.history.push("/channels")` after login — a raw string nothing type-checked,
which would have 404'd every sign-in post-fold.

**The Guide rebuilt against the RENDERED mock.** The first pass was built by reading
`.dc.html` source and inferring, which produced ten defects the maintainer found by looking at
it. Now the mock renders locally (`python -m http.server` over `design/`) so comparison is
repeatable. Two were real bugs, not styling: `border-l-signal` never rendered (Tailwind emits
`border-<color>` and `border-l-<color>` in a build-time-fixed order, so the all-sides utility
won regardless of `cn()` ordering — every accent was #2A2E37, code reading correctly and
rendering wrong), and clicking a block did nothing (a `<button>` with no handler). Plus: the
now-line was 1px amber running through amber blocks (now 2px `onair` red + time chip), the row
menu opened clipped and offered no Edit, the header had no toolbar, and a horizontal scrollbar
sat under a grid that had room (`min-w` was a hardcoded 880px, so zooming out could never make
it fit).

**Perf: 1910ms → 103ms click-to-rows, measured in the browser.** Five layers, each found by
measurement after the previous hypothesis was disproved:

| | click→rows | what it was |
| --- | --- | --- |
| start | 1910ms | |
| `5fcd031` | 610ms | N+1: availability re-resolved per layout pass (72 library calls/request) |
| `97523e9` | 421ms | channels resolved sequentially |
| `43fd680` | 218ms | `pickOrder` fully sorted n candidates at each of n positions to use one |
| `b939576` | **103ms** | the relaxation ladder re-ran the whole placement per step |

**Hypotheses disproved by measurement, recorded so nobody re-tries them:** Emby is slow (one
`/Items` call is 1.5–10ms; an 803-episode enumeration is 40ms); the connection pool is starved
(`MaxIdleConnsPerHost` 4 vs 64 — no difference, 29 concurrent calls in 4–7ms either way); JSON
decode of large payloads (629KB in 5ms); and `ComputeDesiredAt` is inherently expensive (45ms
for **200** channels). The warm split is now 4ms client + 87ms API + 12ms render — the client
is not the cost.

**A 17× REGRESSION, reverted (`eb55c7e`).** The "obvious" `pickOrder` fix — stop building the
tier-2 candidates the caller discards — took the guide from 250ms to 4.3s with every test
green. Those candidates are load-bearing: the caller skips them but only after `budget--`, so
they are what makes a hard pool exhaust `backtrackBudget` and fall back to greedy. The
wasteful-looking code was the circuit-breaker. A ⚠ comment records it with the measurement.

**New in §5/§7/§14/§15/§18.1, all doc-first:** `series_episodes` (migration `00018`, both
dialects) with the `series-episode-refresh` job — deliberately NOT a `library-scan` hook, which
only correlates in-flight acquisitions and would never revisit an already-`available` show;
`@tanstack/react-virtual` for row windowing (200 channels → 19 rows / 793 nodes, verified);
`/debug/pprof/*` behind `LOOMARR_PPROF` (default off, not-registered-is-the-gate, boot WARN);
and `POST /v1/auth/dev-login` behind `LOOMARR_DEV_LOGIN` (same posture; excluded from
`api/openapi.yaml` on purpose — a dev bypass is not part of the product contract).

**Three defects that only sabotage-testing caught**, each a test that looked right and guarded
nothing: an expiry test written as `memoTTL + 1s` (so raising the TTL to 100h still passed); a
§19 member negative using `getByRole("tab", {name})` where CountTabs puts the count inside the
button, so the matcher matched nothing and passed whether or not the tab rendered; and a data
race in `fakeXMLTVGuide` that `-race` missed — my own concurrency change turned a sequential
interface concurrent, and every implementation including test doubles inherited a
thread-safety obligation. Also: `/debug/` was missing from `apiPrefixes`, so with pprof OFF the
profiler endpoint returned the SPA's index.html with a 200 (`go tool pprof` reports that as
"unrecognized profile format", which reads as a broken profiler rather than a disabled one).

Gate: `make check` GREEN (-race); `make test-pg` GREEN (migration `00018` conformant on both
backends); `make fe` GREEN (biome 663 files, typecheck, 766 tests); **fe-visual 504 passed, 0
flaky** (15 baselines regenerated — exactly GuideGrid + AppShell, verified by reading the diffs
and re-running WITHOUT `--update-snapshots`); **e2e 7/7**; `openapi-verify` + `config-docs` no
drift. Guide verified live in the browser throughout, per the maintainer's standing rule that a
curl timing is not what a user experiences.

**Still open:** click-to-rows is 103ms against a ≤50ms target — the remaining cost is the first
placement (~6ms/channel, genuine work) plus store reads and JSON, so further gains need a
different approach rather than more of this one. The V18 gate text says 375px while the
harness renders 390px, and V22's gate cites `design.md:940`, which has shifted; both need a
doc-first correction (found by `/register-check`, 2026-07-26).

**Curation-rule-engine arc — self-updating channels / re-curation (Phase B, 2026-07-24).** The
second half of the rotation/re-curation plan (`.claude/plans/curation-rotation-and-recuration.md`):
a channel built from an intent no longer freezes — an **opted-in** channel periodically
re-evaluates its intent against the current library and evolves its lineup, **preferring
in-library matches, weighting net-new acquisitions by quality + intent, and never bypassing the
approval gate** (programming-design §8.2). Composed from existing parts: **B0** extracted a
`ChannelBinder` (`internal/binder`) — the "materialize approved proposal → channel" logic — out of
the API behind an interface the composition root wires ONCE, so manual-approve AND every
auto-approve path bind identically (also **closed a latent gap**: a per-user auto-approved refine
enqueued acquisitions but never rebound its channel). **Per-channel opt-in** = `policy.autoCurate`
(rides `policy_json`, NO migration — like rules/filler/window) with optional MinScorePct/MaxTitles
overrides; **global knobs** `job.recurate.schedule` (weekly), `recurate.min_score_pct` (60),
`recurate.max_titles` (40). **`recurate.Curator`** = the channel-scoped auto-curate grant: filters a
re-curation proposal's net-new acquisitions to those clearing the quality bar within the cap
(in-library adds free), then approves through the ONE `suggest.Approve` gate (audit "auto-curate")
— **never a raw `wanted` write**, fails closed. **`recurate.Runner`** + the `channel-recurate` job
iterate eligible channels (live+intent-backed+auto-curate; skips paused/detached/hand-made/
not-opted-in) → trigger a refresh refine → the worker's new `ChannelAutoCurator` considers it.
Tests: quality bar, in-library-no-acquisition, **approval-gate negative** (not-opted-in never
requests), title cap, per-channel override, runner eligibility, + the B0 fix. **Live-verified** on
the homelab: opted the 1980s Action Heroes channel in, ran the job → refine → Curator approved
(audit "auto-curate") → bound; **enqueued=0** (library already complete — the correct idempotent,
conservative-spend outcome), **zero stray wanted titles** (gate honored). Doc-first §8.2 +
config-design + design §8. Gate: `make check` (`-race`) + `test-pg` + `openapi-verify` +
`config-docs` + FE `tsc` all GREEN. **The rotation/re-curation plan is now COMPLETE** (Phase A
rotation + Phase B re-curation both shipped). Merged on green-local.

**Curation-rule-engine arc — audience ceiling is a kids/teen guardrail (2026-07-24).** The
"1980s Action Heroes" channel came out capped at **TV-14**, silently dropping its 9 R-rated
films (Die Hard, Predator, Terminator…) — a small model reflexively caps genre channels, and
`groundPolicy` kept any proposed ceiling unconditionally. **Rule (maintainer):** the ceiling
exists ONLY so a channel a user asked to be *for kids/teens* can never show adult content; an
unqualified channel is **adult-default**. **Fix:** `groundPolicy(…, intent)` now keeps a
proposed ceiling ONLY when `intentSignalsKids` matches the intent text (kids/family/cartoons/
Bluey/Saturday-morning/teen/… across description/tone/era/refine/must-include); **no signal →
the ceiling is dropped** (everything admitted). Safety asymmetry absolute: dropping an
unjustified ceiling only *loosens*; when a kids signal IS present the ceiling stays + is
enforced fail-closed, and the raise-to-admit-picks (generalized from TV-G→TV-PG to the whole
kids band) **never crosses the kids→adult line** (`KidsCeilingRank`) — a stray R pick on a kids
channel is dropped by §4, never admitted. Prompt reinforced to omit the ceiling for non-kids
channels. Tests: no-kids→drop, 6 kids phrasings→keep, raise-never-crosses-kids-line, existing
raise/never-lower + grounding fuzz still green. Doc-first §4/§8. **Live-verified:** "90s action
heroes movies" → ceiling None; "cartoons for little kids" → TV-Y7. Gate: `make check` GREEN
(`-race`); `openapi-verify` + `config-docs` no drift. Merged on green-local.

**Curation-rule-engine arc — window rotation fix (2026-07-24).** A movie channel whose
library exceeds its window (the maintainer's "1980s Action Heroes", 15 films ≈30h, 24h window)
**repeated the same subset daily and never aired the tail**. Root cause: `truncateToWindow` kept
the deck HEAD (`slots[:kept]`); the window index advanced the shuffle SEED (order) but not the
slice OFFSET, so `sequential`/`syndication` channels (stable order) looped the same prefix.
**Fix:** `windowSlice` — a ROTATING ~window-of-runtime slice whose start advances with the window
index and WRAPS the catalog (window 1 continues where window 0 left off — TILES, not slides), so
over a full cycle every program airs (coverage invariant). Deck order is now seed-stable per
channel; rotation lives in the offset. Runs on the COLLAPSED deck (franchise/two-parter never
split by the seam); idempotent within a window (no re-push). New tests: rotation coverage (the
exact defect reproduction — 15/15 films air over a cycle), tiling-vs-sliding, idempotency,
franchise-never-split-by-seam across all offsets. **Live-verified** on `ch_2986070a483d5cb0`:
with an eligible pool > window, the aired set rotates and all 15 films air. **Also surfaced (not
a bug):** the channel's **TV-14 ceiling excluded its 9 R-rated films** (Die Hard, Predator,
Terminator, …) — the §4 audience filter working as designed — which is why it looked worse (only
6 PG/PG-13 films, < one window). Maintainer chose to raise this channel to R. Gate: `make check`
GREEN (`-race`); `openapi-verify` + `config-docs` no drift. This is the **catalog-rotation half of
the rotation/re-curation plan** (`.claude/plans/curation-rotation-and-recuration.md`); **next: the
scheduled re-curation job (Phase B, per-channel auto-curate opt-in decided)**. Merged on
green-local (CI still billing-blocked).

**Curation-rule-engine arc — Phase 4: "Programming rules" editor + time-travel preview (2026-07-24).**
The authoring + legibility layer for the wall-clock curation engine (Phases 1–3 already
shipped the deterministic core, seasonal-as-a-rule, and LLM preset authoring — commits
`2f39787`/`f0d01a9`/`22ec659`). Two halves, doc-first (`docs/programming-design.md` §8.1 written
before code): **(BE)** a read-only **`GET /v1/channels/{id}/cycle?at=<rfc3339>`** cycle preview
that runs the SAME pure `ComputeDesiredAt` reconcile runs (one code path — preview can't drift)
at a chosen wall-clock, returning the resolved slots + **which curation rule is active then** +
the resolved rolling-window horizon; makes first-match-by-priority legible ("at Saturday 9am the
Weekend marathon rule wins"). New `schedule.ActiveRuleAt`/`SchedulingRule.Label`/`Describe`
(attribution derived from the SAME `pickRule` the engine uses; suggester now names LLM-authored
rules), `schedule.ResolveWindow`, `channels.Engine.CyclePreview` (read-only — heals/pushes
nothing), and the `ChannelService.CyclePreview` interface method (returns `schedule` primitives, so
`api` stays decoupled from `channels`). **(FE)** a `ChannelRulesEditor` (token-based WHEN/WHAT/HOW
picker mirroring `internal/schedule/presets.go`, `@dnd-kit` drag-to-priority where **list order IS
priority**, computed labels) + a `ChannelCyclePreview` (datetime-local + quick presets → the
attribution callout + program/pending/break slot list), both wired into the channel "rules" tab.
**Bug caught in FE review + fixed:** a hand-authored **marathon** wrote `window:"0s"` (Duration 0 =
"inherit the window" ⇒ WOULD truncate the binge) instead of `"-1ns"` (the `schedule.WindowFull`
sentinel = "whole run") — the opposite of intent; fixed + regression-guarded so a hand-authored
marathon is byte-identical to an LLM-authored one (§8.1). Gate: `make check` GREEN (`-race`);
`make openapi` regenerated + committed (`openapi-verify` clean post-commit); `config-docs` no drift;
FE `biome + typecheck + 402 vitest + web build + storybook build` GREEN (new component stories +
tests, `story-coverage` guard). **Next (Phase 5):** custom holiday calendars (the last §9 logged
item). **CI note:** GitHub Actions still billing-blocked — validated locally + merged on green-local
per the maintainer's standing call.

**Scheduler arc — Phase 5: retire the inbound webhook subsystem (2026-07-23).** The final
phase of the poll-availability arc. Availability + download progress now come entirely from
polling (library scan §4, arr queue poll §18.1), so the inbound `POST /hooks/arr` webhook has
no remaining job — deleted. Removed: `internal/ingest` (the whole package), the `/hooks/arr`
mount + `api.Options.Ingest`, the app wiring, `SecretWebhook`/`WEBHOOK_SECRET` + its reveal/
regenerate enums, the `webhook` setup check + `SetupCheck.LastReceived` + `webhook_last_received:`
settings, the `loomarr_webhook_events_total` metric + `WebhookEvent()`, the four `{sonarr,radarr}/
*_webhook.json` fixtures + `FINDINGS-arr-webhooks.md`, the FE wizard **Webhooks** step (+ its
steps/routes/shell registrations + e2e handshake block), and the secrets-panel `webhook_secret`
row. **Deliberately kept** (look-alikes, verified before deleting): filler-clip ingest
(`clipfetch`/`filler.Ingest`/`FeatureIngest`/`INGEST_*`), the outbound `event.webhook_url`
notifier, and **`provision.KeyFromWebhook` + `radarr/import_webhook.json`** (still used by the
channel-lineup key-parity path). The retirement was authorized by the Phase-2 safety proof
`TestScanAvailability_NoWebhook` (green — a `requested` title reaches `available` with no
webhook), never by weakening a gate. Gate: `make check` GREEN (`-race`); `openapi-verify` +
`config-docs` no drift; FE biome + typecheck + **382 vitest** GREEN; visual + e2e baselines
regenerated in the Playwright Docker image (WizardShell + wizard-flow snapshots; webhook-step
baselines removed). **NOTE (CI):** GitHub Actions is billing-blocked on the account (jobs fail
in ~1s with a payments notice) — Phases 4–5 validated locally and merged on green-local per the
maintainer's call; restore billing to re-enable CI.

**Scheduler arc — Phase 1: cron scheduler + 4-loop migration (2026-07-23).** First phase of
the direct-Sonarr/Radarr + poll-availability plan (retiring inbound webhooks, since verified
research shows Overseerr/Seerr is entirely poll-driven). A real job scheduler like Sonarr's
System → Tasks: `internal/scheduler` (code-defined registry + `scheduled_jobs` state table +
a leased due-claim reusing the `ClaimDueTitles` idiom — SQLite guarded UPDATE / Postgres
`FOR UPDATE SKIP LOCKED`). Schedules are **full cron** (6-field, seconds-leading, matching
Overseerr) via a new `KindCron` setting validated by `github.com/adhocore/gronx` (pure-Go,
zero transitive deps — added to design §14 + a new §18.1, doc-first). **All 4 existing loops
(reconcile, channel-sweep, filler-sync, session-sweep) migrated to scheduler jobs** — their
standalone `Run`/`WithInterval`/ticker plumbing deleted (`reconcile/runner.go`, `janitor.go`,
the `channels.Runner` loop, `go runFillerSync`); each is now Run-now-triggerable with an
editable cron from day one. `GET /v1/jobs` + `POST /v1/jobs/{name}/run` (admin-only); timing
is BE-authored and pushed over a new `job` SSE frame (the FE never computes countdowns).
FE: Settings → **Tasks** page (Sonarr-style table + "Run now") and a reusable **Modify Job**
modal — human-readable cron presets by default, an **Advanced** toggle revealing the raw
cron field. Also removed the redundant Live TV wizard step + `POST /v1/setup/livetv-connect`
(auto-wires on Connections save; config-design §6). Gate: `make check` GREEN (`-race`); FE
`biome + typecheck + vitest` GREEN (381 tests, 11 new). **Next (Phase 2):** poll-based
availability — `RecentlyAdded`/`AllItems` library scan jobs driving `LibraryConfirmed`.

**Phase 14 — domain metrics, tranche 3: §17 closed (2026-07-20).** The last non-latency,
non-state counters, each hooked at its subsystem's natural point:
`loomarr_llm_tokens_total{kind}` (prompt/completion, parsed from the provider usage block —
Ollama `prompt_eval_count`/`eval_count`, OpenAI `usage.*`; zero → no-op so a provider that
omits usage adds no phantom sample); `loomarr_filler_pods_total{match_level}` (fallback-ladder
rung, recorded in `BuildFillerList` — the attach path, NOT `Preview`, so UI previews don't
inflate it); `loomarr_channel_slot_substitutions_total` (`programWentStale` → `staleProgramCount`,
now returns the count, recorded in reconcile). **Cost is deliberately NOT a metric** — it's
tokens × a per-model posted rate that drifts and is hosted-specific, so it belongs in a
dashboard recording rule over the token series, not baked into the request path. That closes
the §17 metric list end to end (RED + runtime + state gauges + event counters + latency +
these). Gate: `make check` GREEN.

**Phase 14 — domain metrics, tranche 2: latency (2026-07-20).** Client-side RED for every
outbound dependency via ONE instrumented transport in `httpx.NewNamed` — the six RPC
adapters (library/tunarr/seerr/tmdb/ollama+openai) now build named clients, so
`loomarr_outbound_request_duration_seconds{target}` + `loomarr_outbound_requests_total{target,code}`
cover the §17 library-lookup / Tunarr-API-latency-and-errors / LLM-latency in one series
filtered by target (a transport failure → `code="error"`). Health probes stay on plain
`New` (unlabelled) to keep the metric to the operational RPC path. Plus reconcile timing:
`Engine.Reconcile` (named-return + defer) emits `loomarr_channel_reconcile_duration_seconds`
and `loomarr_channel_reconciles_total{result}`; the injected clock keeps it deterministic (0)
under fixed-time tests. `httpx → metrics` is a new edge but acyclic (metrics imports only
prometheus + provision). **Still deferred** (§17): LLM token/cost (needs provider usage),
filler pod-ladder depth, slot-drift — domain counters, not latencies. Gate: `make check` GREEN.

**Phase 14 — domain metrics, tranche 1 (2026-07-20).** The §17 *domain* series, first
tranche: the state gauges via a pull-based collector (`loomarr_titles{state}`,
`loomarr_jobs{status}`, `loomarr_active_sessions`) + the two cleanest event counters
(`loomarr_auth_logins_total{result}`, `loomarr_webhook_events_total{type}`). The collector
reads three new store count-by-state methods on *scrape* (not on the write path), so no
mutation path is instrumented; `RegisterStoreCollector` wires it once at boot from
`BuildHandler`. The store methods are dialect-neutral (GROUP BY / COUNT) — one impl on
`sqlStore`, covered by a new `ObservabilityCounts` case in the one-suite-two-backends
conformance suite. Webhook + login labels are bounded (unknown eventType → "other"; login
result ∈ success/failure, rate-limits excluded — they're the 429 signal). Scrape-time store
errors degrade to `loomarr_metrics_scrape_errors_total`, never a panic or a stale zero.
**Still deferred** (§17, honest): the latency histograms (reconcile / Tunarr-API / LLM /
library-lookup), LLM token/cost, filler pod-ladder depth, slot-drift — each needs its own
timing wrapper around an external call, a different pattern; a later tranche. Gate: `make
check` GREEN; conformance passes on sqlite (`make test-pg` for Postgres on CI/Docker).

**Phase 14 — docs set, compose audit, metrics foundation (2026-07-20).** The user-facing
help set (Quickstart, Integrations, Concepts, Member, Filler + Troubleshooting keyed to
checklist items), rewritten lean on maintainer feedback, embedded and served at `/v1/docs`
(`a50c57b`, deep-link routing fixed `eb09813`). Seed docs folded into `docs/` (`62d9369`);
README got Documentation + Operations sections (`f46ddf9`).

*Compose-profile audit* (`61e1f9c`): topology matched §16; three satellite docs had drifted
— README + compose header never showed `--profile ai` (yet the default LLM points at the
ollama service it gates), and `.env.example` called filler "a profile" when §16 is explicit
it's the `loomarr:filler` image tag. Fixed the docs; design.md was already correct.

*Metrics* — `internal/metrics` + `GET /metrics` (unauthenticated, §7), `prometheus/client_golang`
(already sanctioned in §14 line 633, so no new-dep conversation). **Scope, honestly (no silent
caps):** wired the RED basics — `loomarr_http_requests_total` / `_request_duration_seconds` /
`_requests_in_flight`, labelled by method + *matched route pattern* (bounded cardinality, not
raw path) — plus the free Go/process runtime collectors. **Deferred:** the §18 *domain* series
(records-by-state, reconcile + Tunarr-API latency, LLM tokens, filler pod-ladder depth, logins,
active sessions, job-queue depth, janitor purges). The pull-based gauges among them need store
count-by-state methods, which touch the one-suite-two-backends conformance gate — a separate
follow-up, not smuggled in here.

**Maintainer smoke — §21's DoD closed end to end (2026-07-20, cont.).** The walkthrough
now runs intent → grounded proposal (real Ollama) → approve → a channel PLAYING in Tunarr,
proven live: an 80-program kids channel from "90s Saturday morning cartoons", built by
Loomarr's own approve→reconcile against the real Emby. Eight steps; `make smoke-livetv`
wires+destroys a disposable Jellyfin for the one media-server-writing action. **Bugs the
smoke found, every one CI-green beforehand — the seams BETWEEN gate-green subsystems:**

1. `GET /v1/users` panic — int setting read through a string-only seam (`0dc957e`).
2. Empty env var destroyed the operator's saved value — `.env.example` ships `LLM_MODEL=`;
   the resolver read present-but-empty as a pin (`be860bc`).
3. FINDING 1 — a fresh install had no way in (`/`→`/login`, no account); added the
   unauthenticated `GET /v1/setup/state` the router guards branch on (`38f8215`).
4. Model selection didn't hot-apply — `persist` bypassed the settings service, so the
   choice vanished on restart (`2128db5`).
5. `make smoke` could never exit — `go run` supervises rather than execs (`e8a956b`).
6. Live TV wiring broken on ALL of Jellyfin — the enumerate GETs are write-only there
   (405); moved to `GET /System/Configuration/livetv`, works on both flavors (`57209f8`).
7. Wizard stranded operators behind un-skippable wiring steps; both wiring actions also
   surfaced in Settings (`3f3082e`).
8. FINDING 4 — approving never created the channel (§7 said it should); `channelForIntent`
   (`be9cb35`).
9. FINDING 6 — a kids channel went live with ZERO programs: discovery backfill set
   InLibrary but dropped the rating, so an audience ceiling excluded every entry (dead
   air, §9). Backfill now carries the rating (`bad5814`).

**Open (surfaced, not taken):** FINDING 5 — the `tunarr` setup check only probes
reachability, so an unset/foreign `transcode_config_id` reads green while every channel
create 400s; recommend auto-selecting Tunarr's Default + validating it. Part 2 — the
acquisition-rating gap (a not-yet-owned title has no library rating) is entangled with
§389's stamp-once-at-create-time invariant; the clean fix is one design question: should
an entry's rating refresh at reconcile when library presence resolves? (Also fixes the
upgrade case where a pre-fix cached proposal, §8 24h TTL, carries empty ratings forward.)

**Maintainer smoke — the §21 second half, mechanised (2026-07-20).** Branch
`fix/quota-panic-live-smoke`. `make smoke` stands up a THROWAWAY install (own database,
own Tunarr container) and drives it with Playwright against the real media server, TMDB
and Ollama. Not in CI and never in `make check` — those mock every external service by
rule (§19), and this exists because the seams *between* gate-green subsystems are where
the bugs have been. Seerr is deliberately omitted so no approval can start a real
download. The stack stays up between runs, so the suite is stateful by design: every step
asserts something real in both a fresh and a re-run state, because a suite that is
normally red teaches you to ignore it.

**Three real bugs in its first four steps, none of which any gate could have caught:**

1. **`GET /v1/users` panicked on every load** (shipped in #36) — an int setting read
   through a string-only seam. Unit tests all leave the config seam nil, so the typed
   accessor was never reached. Fixed with a `LiveConfigInt` seam + a regression test that
   wires it (`0dc957e`).
2. **An empty env var silently destroyed the operator's saved value** (`be860bc`).
   `.env.example` ships `LLM_MODEL=`, and the resolver read a present-but-empty var as a
   pin. The §8.1 picker persisted `llm.model` and hot-swapped the live suggester — UI said
   "In use", suggestions genuinely worked — while every *read* still resolved to the empty
   pin, so the checklist said "no model selected" right after one was, and the choice
   vanished on the next restart (verified live: `"model":""` → `"model":"qwen3.5:9b"`).
   config-design §3 now states empty-is-unset and boot WARNs each pin it ignores. Every
   settings unit test missed it because none had ever set a key to `""`.
3. **FINDING 1 — a fresh install had no way in.** `/` redirected to `/login`, which no
   credential could pass because no account existed, and nothing on the page said the
   install was unclaimed; only an operator who guessed `/wizard` escaped. §16 has always
   told operators to "open the UI at `/` and create the owning admin", so the code
   contradicted the doc. Fixed with an unauthenticated `GET /v1/setup/state` (§7, doc-first)
   that the `_authed` and `/login` guards branch on; `needsBootstrap` **fails closed**, so a
   blip on that probe cannot drop a healthy install's users into a first-run wizard.
   Covered by a Go handler test (unauthenticated + flips on bootstrap), FE unit tests for
   the failure directions, and two e2e cases (unclaimed → wizard from both entry points;
   claimed + signed out → login).

Gate evidence: `make check`, `make e2e` (7/7), `make fe` unit (223/223), and `make smoke`
against the live stack.

**Phase 13.4e — the 13.4 gate (2026-07-19).** Branch `feat/fe-gate-13.4e`. Phase 13.4 is
complete: Channels, Board, Suggest, Settings, Users, Filler, Help, and the ⌘K palette.

**The reachability guard is the gate this phase earned.** SEVEN times in 13.4 something
was built, unit-tested, and unreachable — two settings panels never mounted; a formatter
never called (so "·til 8:00 PM" was dead UI on every channel card); a 323-line settings
form rendered by nothing; a clip's tag action gated so the one clip needing correction
couldn't be; a search scope that always returned empty; a Search button wired to a
discarded setState. Component tests passed in every case, because a component test cannot
see whether anything mounts it.

`reachability.test.tsx` asserts every route in the generated tree renders real content and
every feature-gated panel appears when its flag is on. The route list is DERIVED from
`router.routesById` — a hand-maintained list is the same mistake one level up, which
`structure.test.ts` already learned. **Verified against the real regressions:** removing
the secrets panel's mount and removing the palette's mount each fail it with a precise
message.

**The approve-flow e2e is §7's gate under test.** An admin approving enqueues the
acquisition; a member is not offered approval and nothing is enqueued (§19's negative).
Assertions land on the mock's recorded state, not the screen — "the button looked like it
worked" is exactly the failure a gate exists to catch. Verified by deleting the `isAdmin`
condition on the approval queue: the member test fails.

**Contract discipline held.** The mock backend's proposal shape was wrong twice (guessed
field names), and both times the fix was to read the generated DTO rather than the
remembered shape — the same rule the fixtures carry.

**Gate:** `make check` GREEN; `make fe` GREEN (217 app / 24 core / 9 api); `make fe-visual`
GREEN (188, zero flaky); `make e2e` GREEN (5); `make openapi-verify` GREEN.
**Next:** Phase 14 — the user-facing docs set, runbook, README, and folding in the seed
docs (checking each against the design doc first: §2 and §7.2 were both stale this phase).


**Phase 13.4d — Users and Filler shipped; Help + ⌘K remain (2026-07-19).** PRs #26–#33,
all merged. The session ran backend-prep-then-screens, and the prep was worth it: the
gap sweep before writing any UI found surfaces §12/§13 assume but nobody had built.

**Backend prep (#26, #27, #28, #29, #30).** Sessions list/revoke; the `user_sync` gate
that fixed a live `useSyncUsers` 404; the credential-path flag; clip search moved off the
federated scope; the sidecar deleted and ingest folded into the core; pod preview; the
Help docs embed, version endpoint, and a storeless-boot panic fix.

**Screens (#32, #33).** Users (allowlist, sessions, explicit import) and Filler (catalog,
tagging, ingest, pod preview). Both admin-gated in the UI as a courtesy; the server
enforces (§11, §19).

**The recurring failure of this phase, worth carrying into 13.4e.** SIX things were built,
unit-tested, and unreachable — `AiModelSettings` and `SecretsSettings` never mounted;
`formatEpgTime` never called, so "·til 8:00 PM" was dead UI on every channel card;
`SettingsGroupForm` (323 lines + tests + 12 visual baselines) rendered by nothing;
`ClipCard`'s tag action gated so the one clip that needs correcting couldn't be;
`scope=clips` advertising a corpus that always returned empty. Every component test
passed in every case. Page-level tests were added for Users and Filler specifically to
assert WIRING rather than behavior — **13.4e should make reachability part of the gate**:
every route renders, every feature-gated panel appears when its flag is on.

**Gates got two real repairs.** The a11y check rejected a disabled row whose `opacity-60`
fell under the WCAG AA contrast floor. And the visual suite's "1 flaky" — present on every
run, a different story each time — was axe-core's module-global running flag re-entering
through `runPartialRecursive`; Playwright's retry kept it green so it never blocked, but a
gate that cries wolf stops being read. Retried only on that exact message; zero flaky on
every run since.

**Shared-code cleanup (#31).** Four core formatters had NO call sites (the inverse of the
duplication we went looking for), one live grammar bug at n=1, and copy-to-clipboard
hand-rolled twice with the same two defects in both copies.

**Gate:** `make check`, `make fe` (191 app tests), `make fe-visual` (188, zero flaky),
`make e2e`, `make openapi-verify` — all GREEN on main.
**Next:** 13.4d-3 (Help page + ⌘K palette — transport and `troubleshooting.md` already
exist from #30, so it is mostly rendering + client-side search), then 13.4e.


**Phase 13.4d-0c — Help transport, version, and a startup panic (2026-07-19).** Branch
`feat/be-help-13.4d0c`. The last backend slice before the 13.4d screens.

**The docHref anchors had nothing to land on.** The API has emitted deep-links like
`troubleshooting#tunarr` since phase 8, and §13 promises "every red check deep-links to
its section" — but nothing embedded `docs/`, and the troubleshooting page did not exist.
Every red check pointed at a blank page, at exactly the moment an operator is stuck.
Now: `docs/help/` embedded via `docs/embed.go`, `GET /v1/docs` + `GET /v1/docs/{slug}`,
and a **test that every anchor the API emits resolves to a real heading** — renaming a
heading fails the build instead of silently breaking a link. Only `help/` ships; the
design docs beside it are internal and deliberately excluded.

Wrote `troubleshooting.md` with all 8 sections. This is not phase 14's docs set (that
still owns Quickstart/Concepts/Member guide/etc.) — it is the one page the *existing*
backend contract already requires. Also gave `tunarr_library` its own anchor rather than
sharing `#tunarr`: it fails for a specific silent reason (unscanned media source ⇒ dead
air while everything else reads healthy) and deserves that explanation, not generic
connectivity advice.

**Version.** Nothing exposed one — §16 discusses upgrades and the runbook assumes the
operator knows what they run. `internal/buildinfo` stamps via ldflags, falls back to Go's
embedded VCS stamps, then to "dev". Surfaced on `GET /v1/system/version` together with
readiness.

**Deliberately did NOT move /healthz + /readyz into huma.** The sweep suggested it so
orval could type them, but their consumers are Docker HEALTHCHECK and orchestrators,
which hold no session — registering them in the authenticated /v1 API would put auth in
front of a container health probe. The UI gets the typed twin instead. Pinned by a test,
because "type the health endpoints" is a tempting future change.

> ⚠ **REVERSED LATER, and the pin did its job.** The probes are Huma operations under
> `/v1/healthz` and `/v1/readyz` now. What changed is not the concern above but its
> premise: "registering them in huma" no longer implies "in the authenticated API",
> because `RolePublic` makes non-authentication an explicit, greppable property of an
> operation. The two things this entry actually cared about are both preserved and still
> tested — the probes answer with **no credential**, and a **storeless boot** still gets
> 200 from healthz and 503-with-a-reason from readyz (the 503 keeps carrying `ready` and
> `detail` rather than becoming an RFC 7807 problem; huma additionally emits its standard
> `$schema` link, as on every other typed response here). Their bare paths stay as permanent
> aliases, because a healthcheck lives in someone's compose file and cannot be migrated
> by editing this repo. `TestOpsProbesStayUnauthenticated` survives, now covering both
> paths, plus `TestOpsProbeAliasesAgreeWithTheCanonicalPaths`.

**Found and fixed a startup panic (pre-existing, from the 2026-07-18 guide-reader work).**
Starting with no DATABASE_URL is a SUPPORTED mode — main logs "running without a store
(not ready)" and expects /readyz to explain itself. Instead the process panicked in
BuildHandler: no store ⇒ no settings service, and `resolved.str` dereferenced nil. A
container missing DATABASE_URL crash-looped instead of answering the probe that would
have told the operator why. Guarded every `resolved` getter; regression test verified to
reproduce the exact panic without the fix.

**Gate:** `make check` GREEN; `make fe` GREEN (169 app tests); `make openapi-verify` GREEN;
manually verified a storeless boot serves /healthz 200 and /readyz 503 "no store configured".
**Next:** 13.4d proper — the Filler, Users, Help, and ⌘K screens.


**Phase 13.4d-0b pt2 — the sidecar is actually gone (2026-07-19).** Branch
`feat/ingest-in-core-13.4d0b2`. PR #25 changed the DESIGN; this closes the code. Between
the two, the docs said the sidecar was removed while the repo still shipped it — real
drift, caught by the maintainer noticing `Dockerfile.ingest` in the tree.

**The move was smaller than feared.** The download logic was already `internal/ingestkit`
(a core package, fully tested with fake downloaders); only a 71-line driver in
`cmd/loomarr-ingest/` was sidecar-specific. Renamed to **`internal/clipfetch`** because
`internal/ingest` is the Sonarr/Radarr WEBHOOK handler (§6's Ingest port) — two unrelated
concepts one autocomplete apart, now that both live in the core.

**Two real bugs found by actually building and running the image:**
1. **The tooling was x86_64-only.** Inherited from `Dockerfile.ingest`, which hardcoded
   amd64 URLs for yt-dlp/deno/ffmpeg. On arm64 the image BUILT FINE and then died at run
   time with `rosetta error: failed to open elf at /lib64/ld-linux-x86-64.so.2`. Now
   fetched per `TARGETARCH`, with a build-time `--version` check on all three so a broken
   image can never ship looking healthy.
2. **Appending the stage made the fat image the DEFAULT.** Docker builds the last stage,
   so `docker build .` produced 704MB instead of the distroless core. Reordered: filler
   before core, core last. Measured after: **loomarr:latest 31.1MB, loomarr:filler 549MB.**

**The documented size was wrong.** §10/§14/§16 claimed "~170MB" (inherited from the
sidecar's own comment). Measured reality is ~518MB more. Corrected everywhere rather than
left as a comfortable number. Also dropped **ffprobe** (~99MB): Loomarr never probes media
— Tunarr assigns duration during its `local`-source scan — so bundling it was pure weight.

**`ingest` is the first environment-derived feature gate.** It probes for runnable
yt-dlp + ffmpeg rather than reading settings completeness, exactly as config-design §7's
new exception describes. Both tools are required (yt-dlp cannot merge streams without
ffmpeg, so a half-present image would accept the job and fail mid-download on the
high-res sources most worth fetching). The 409 names the remedy — "run the loomarr:filler
image" — and deliberately is NOT `feature_not_configured`, because no setting can open it.

**Ingest returns a job id, not a result.** Downloads run minutes to hours; progress rides
the SSE bus as `filler_ingest` frames, the same shape §8.1's model pull uses. The
background context is deliberately NOT the request context, or every ingest would die the
moment a browser tab closed.

**Gate:** `make check` GREEN; `make fe` GREEN (169 app tests); `make openapi-verify` GREEN;
`docker build` of BOTH targets verified, and yt-dlp/deno/ffmpeg each executed natively on
arm64 (not inferred from a successful build).
**Next:** 13.4d-0b pt3 (pod preview — §12's explicit Filler requirement), then 13.4d-0c.


**Design change — filler ingest moves into the core; the `loomarr-ingest` sidecar is removed (2026-07-19).**
Branch `design/ingest-in-core`. Doc-only; no code, no spec drift. Maintainer call, taken before building
13.4d's Filler page so the page isn't designed against a seam we were about to delete.

**What changed and why.** The sidecar's *only* stated justification was keeping ~170MB of yt-dlp+ffmpeg out of
the core image (§14). It was already Go, so §14's language policy never required it. What that slimness cost:
a second image to build/version/publish, a `filler` compose profile, a proxy endpoint in the core just to reach
the sidecar, ingest progress that could not ride the existing SSE job bus, and a Filler page whose primary
action was a hop to an optional service that might not be running. Ingest is now a normal in-core job, and the
tooling ships in an opt-in image *tag* (`loomarr:filler`) rather than an opt-in *service* — same binary, same
config, same endpoints, so operators move between tags with a restart, not a topology change.

**The one genuinely new mechanism:** `ingest` is the first feature gate that is NOT derived from settings
completeness — no amount of configuring opens it on `loomarr:latest`, because it depends on which image is
running. config-design §7's heading previously claimed `RequiredFor` computed availability and "nothing else
does"; that is now stated as an explicit, reasoned exception rather than left as a quiet contradiction. The
invariant that survives: one function computes the set, every consumer reads it — only the inputs differ.
ffmpeg is bundled (not skipped) so yt-dlp can merge separate video/audio streams; without it, high-res sources
fail or silently downgrade.

**Sections touched:** §2 (filler flow + FillerSource port row), §7 (`/v1/filler/ingest`), §10 (ingestion path,
config, probing), §13 (Filler guide), §14 (Sidecar & CI → Ingest tooling & CI), §15 (four `INGEST_*` keys),
§16 (two tags one binary; compose profiles now sqlite/postgres/ai), §21 (repo layout drops
`cmd/loomarr-ingest/`; phase-12 text); config-design §5 (Filler settings row), §7 (the exception), §6 (wizard
step). The §15 additions are the human mirror only — the Go registry entries land with the implementation, so
`make config-docs` stays drift-free until then.

**Stale docs fixed in passing (a real contradiction, not cosmetics):** §2 lines 85/98 still described filler as
"the media server scans its dedicated filler library" and "media-server filler-library sync" — but §10 was
revised long ago to Tunarr-`local` with the media server *out* of the filler path, and §2 was never updated.
Same for `/v1/filler/sync`'s table description and config-design's Filler settings row (which also referenced
`/v1/filler/sources`, an endpoint that does not exist).

**Gate:** `make check` GREEN; `make openapi-verify` GREEN (no spec drift — doc-only).
**Next:** 13.4d-0a/0b/0c (the backend surfaces 13.4d needs), then 13.4d itself.


**Phase 13.4c-2 — the model picker and the secrets panel (2026-07-19).** Branch `feat/fe-settings-13.4c2`. The two
sub-features deliberately deferred out of 13.4c, each substantial enough to deserve the room.

**The §8.1 model picker renders a judgement it does not make.** Picking a local model is a real onboarding hurdle —
a household admin should not have to know which Ollama tag fits their GPU or supports tool-calling — so the BE
probes the machine and returns a catalog already annotated with `fit` (fits/tight/wont_fit), `approxVramGiB`,
`runtimeOk`, `pulled`, `recommended`, and a `why`. The UI's whole job is to show that honestly. Two calls worth
recording: an unrunnable model is shown **disabled with its reason** rather than hidden, because "why isn't X
listed?" is a worse question than seeing X greyed out; and an unpulled tag offers **only Download**, never "use
this", because `select` 409s on a model that is not local (§8.1). Pull progress exists only on the `llm_pull` SSE
frames, so it arrives through the shared event fan-out built in 13.4a, and a terminal frame refreshes the catalog
so the row flips from Download to In-use without a reload.

**The secrets panel states consequences before the click, not after.** §4's display policy is differentiated by
PURPOSE, so the panel is a closed set of three rather than a generic list: `API_TOKEN` and `WEBHOOK_SECRET` are
values you must paste elsewhere (viewable on demand, eye toggle + copy), while `SESSION_SECRET` has nothing to
paste anywhere and is **never displayed** — Regenerate is its only affordance. Regeneration requires a second
confirm carrying the specific consequence, because they differ sharply: rotating the webhook secret silently
breaks every Sonarr/Radarr hook already configured, and rotating the session secret signs everyone out *including
the person clicking*. An operator should learn that from the button, not the aftermath. Revealing is fetched
**imperatively on click** rather than mounted as a live query — a secret should cross the wire when asked for, not
sit in the cache of every page that renders the panel.

Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN (**189 tests**); Docker visual GREEN
(172, **14 new baselines**, a11y clean); `make e2e` GREEN. **Next: 13.4d** (Filler · Users · Help · ⌘K) then
**13.4e** (the 13.4 gate).

**Phase 13.4c — Settings (2026-07-19).** Branch `feat/fe-settings-13.4c`. The troubleshooting console (§13) and
the first real test of whether `SettingsGroupForm` held up outside the wizard. **It did not**, and that was the
useful finding.

**The save UNIT differs by context, so form ownership had to move out.** config-design §5 specifies ONE sticky save
bar per page — Sonarr's model, chosen because connection settings change *together* (a URL and its token) and a
half-saved pair caught mid-test looks like a broken integration. But Connections alone has four blocks (media
server, requester, Tunarr, TMDB), and `SettingsGroupForm` owned both its state and its own Save button: four
blocks would have meant four save buttons and no aggregate dirty state. Extracted **`SettingsFields`** — a
*controlled* group of fields, no `<form>`, no save — which the wizard (one group per step, own save) and a
Settings page (many blocks, one bar) both compose. `SettingsGroupForm` now wraps it; **all 29 wizard tests passed
unchanged**, which is what made the refactor safe to do.

**Also corrected:** the build plan says 5 Settings pages; config-design §5 lists **6** — it omits **Filler**. §5 is
authoritative on Settings IA (a companion wins on its own domain), so six were built. And `updatedBy`/`updatedAt`
existed on every entry but were never rendered; §5's field anatomy asks for "changed by … · when", now shown —
only where a *person* set the value, since an env pin or built-in default has no author.

**Built:** the six pages behind a nav, each composing blocks with one save bar that appears only when dirty and
sends **only changed keys** (an untouched secret reads back empty (§4) and an empty-string PATCH would clear it
(§9) — the hazard fixed in #14, now guarded at the page level too). Per-block **Test** runs the same named check
the wizard uses. The re-runnable **connection checklist** sits at the top of Connections, making Settings the
troubleshooting console for the life of the install rather than a first-run-only screen.

**Both conventions guards fired on my own work** — `structure.test.ts` caught a hook filed inside another module's
folder, and `story-coverage.test.ts` caught two new Layer-2 components shipped without stories. Biome's a11y rule
caught `role="region"` where a `<section>` belongs. Three convention failures I did not have to notice myself.

**A silent no-op in my own tooling, worth recording:** this entry was missing from the 13.4c PR entirely. The
insert anchored on a marker that did not exist on that branch (13.4b was still unmerged), and `str.replace`
returns the string unchanged rather than failing — the same class of quiet failure as an ignored `maxAcquire` or a
guessed guide field. Entries now anchor on the file's stable preamble and the result is verified, not assumed.

**Deferred, flagged:** the §8.1 **AI model picker** (probe, fit-ranked catalog, hot-swap, `llm_pull` progress over
SSE) and the **generated-secrets panel** (view/copy/regenerate, §4).

Gates: `make check` (30 packages) GREEN; `make fe` GREEN (**167 tests**); Docker visual GREEN (157, **14 new
baselines**, a11y clean); `make e2e` GREEN. **Next: 13.4c-2** (model picker + secrets panel) then **13.4d**
(Filler · Users · Help · ⌘K).

**Phase 13.4b — Channels, Channel detail, Board (2026-07-19).** Branch `feat/fe-channels-13.4b`. The
"where is my stuff" surfaces, on top of the guide read landed as this slice's prerequisite (#19).

**Two derivations that belong in one place, not inline in a page.** `channelHealth` maps the API's lifecycle
(`building/live/drifted/detached`) plus slot fill onto the card's presentational health — deliberately different
vocabularies (channel-card.type.ts says a page must derive this). The interesting case is **live with unfilled
slots**: the channel IS airing (Tunarr plays flex, never dead air, §9) but is not yet what was asked for, so it
reads `pending-slots` rather than `healthy`, which would hide the backfill the operator is waiting on.
`channelOnAir` answers a *different* question — a **drifted** channel still broadcasts, it just no longer matches
intent, so it is `live` there and `drift` in health. Both are pure and tested.

**The Board leads with the journey, not the states.** §13 asks for member framing — "1 of 3 titles have landed" —
so the five provisioning states (§4) collapse into waiting · acquiring · ready. `unavailable` deliberately does NOT
become a fourth stage: a title that gave up after its TTL belongs to the "waiting" conversation (something to
retry), not a verdict on the channel — and it stays in the progress **denominator**, since dropping it would
silently shrink what was requested and read as better progress than reality.

**Built:** Channels (cards with now/next from the one-call guide endpoint, health rollup, reconcile-now, and the
§6 empty state whose single next action is "suggest a channel" — the only way to make one). Channel detail moved
the route into a directory (`channels/index.tsx` + `channels/$id.tsx`) and shows slot fill, the Tunarr link, and
the **relaxation ladder**: each applied relaxation is structured (`kind/from/to`), so a chip reads
"audience: TV-Y → TV-Y7" instead of an opaque label — the honest account of what Loomarr loosened to fill the
channel (programming-design §9). Board groups titles by stage with per-title `StateBadge` for the operator who
wants detail, and offers retry only where a human can act.

**Caught by the typechecker, not by me:** the retry initially posted `{key}`, but the enqueue contract takes the
title's real identity (`mediaType/tmdbId/tvdbId/name/year`) — the 1:1 generated client refusing a hand-guessed
body, which is exactly what it is for.

Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN (**172 tests**); Docker visual GREEN
(144, 0 diffs); `make e2e` GREEN. **Next: 13.4c** — Settings (5 groups, secrets lifecycle, the §8.1 model picker,
and the re-runnable checklist as the troubleshooting console), which reuses `SettingsGroupForm` from 13.3b.

**Phase 13.4b (prerequisite) — reading Tunarr's guide for now/next (2026-07-19).** Branch
`feat/fe-channels-13.4b`. The BE half of Channels: `NowNextStrip` was built in 13.2 and listed under Channels in
the build plan, but had **no data source**. `LineupEntry` carries a duration and no start time — Loomarr owns the
lineup (what should play), Tunarr owns playout (when it actually plays, §6) — so airtimes are Tunarr's truth and
cannot be recomputed here without duplicating its scheduling math.

**How the contract was established, after two wrong turns worth recording.** First I claimed this needed
maintainer-supervised live contact and handed over curl commands. That was invented: the repo vendors Tunarr's
OpenAPI (`api/vendor/tunarr-openapi.json`), a version-pinned Tunarr runs in the dev stack, `TUNARR_URL` is in
`.env`, and Tunarr is open source. The rule I cited forbids the *test suite* touching the network, not
investigating. Second — prompted by "the code is open source, why not look at it?" — reading Tunarr's own zod
schemas at tag `v1.3.8` **invalidated the capture I had just taken**: I had sampled the SINGULAR
`/api/guide/channels/{id}`, whose shape (`{index, lineupItem, startTimeMs}`) carries **no title and no stop time**.
Had that been pinned, Channels would have shipped a title-less strip plus a second lookup just to name a program.
The **plural** `/api/guide/channels` carries `start`/`stop`, a real `title`, and tmdb `identifiers[]` — and is keyed
by channel id, so a Channels LIST costs **one** upstream call, not one per card. That also dissolved the N+1
objection I had raised against doing this at all. Capture + both traps recorded in
`fixtures/tunarr/GUIDE-FINDINGS.md`; the vendored spec does not type the guide response, so the source and the
capture are the only authorities.

**Built:** `programmer.Guide` (parses the guide; tmdb id lifted from `identifiers[]` so an entry joins to a
provisioning key with no second lookup; an ungenerated guide is empty, not an error — the tolerance `GetLineup`
already gives an unprogrammed channel). `guideAdapter` reduces a timeline to the pair a card shows, using a
**containment** test rather than "the first entry", so an out-of-order or already-finished head cannot mislabel
what is on. `GET /v1/channels/now-next` serves every channel from that one call, keyed by the **Loomarr** id (the
FE never sees Tunarr ids), and a Tunarr hiccup returns empty rather than blanking the page.

**Self-review caught two more:** the new route shares a prefix with `/v1/channels/{id}` and could plausibly have
resolved to `get-channel` with `id="now-next"` — Go 1.22 prefers the literal segment, but that was assumed rather
than proven, and is now pinned; and a test carried a hand-rolled `itoa` reinventing `strconv`. Parsing is tested
against the pinned capture and **proven to fail** (read the title-less singular shape → red).

Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN; `make e2e` GREEN; orval generated
`useChannelsNowNext`. **Next: 13.4b proper** — the Channels page (cards, health rollup, now/next, reconcile-now),
Channel detail, and the Board.

**Phase 13.4a — the Suggest workspace + approval queue (2026-07-18).** Branch `feat/fe-suggest-13.4a`. The
product's core loop: a sentence becomes a grounded lineup. 13.4 is 8 surfaces, so it is sliced — **a** Suggest +
approval, **b** Channels + Board, **c** Settings, **d** Filler/Users/Help/⌘K, **e** the gate. Suggest went first
because the wizard already hands off to `/suggest` and it was landing on a placeholder — a broken promise in
shipped code.

**A three-way contract mismatch, fixed at the root (first commit).** `submitInput.Body` was a hand-mirrored copy
of `suggest.Intent` — the exact pattern PR #9 removed from ProposalDTO, whose "zero hand-written mirror on either
side" comment sits *twelve lines below* the mirror that had drifted. It omitted `RuntimeTgt`, so `runtimeTargetMin`
was **unreachable by any client** even though the suggester feeds it to the LLM prompt and the scorer, and §13
lists a runtime target among the constraints a user may set. Meanwhile the FE's `intentSchema` said `maxAcquire`
where the wire says `maxAcquisitions` — parses fine, serializes fine, server ignores it: **a user's acquisition cap
silently vanished**. Fixed by typing the body from the domain rather than patching one field, so `SubmitInputBody`
disappears from the spec entirely and the request body now refs the SAME `Intent` schema `Proposal.intent` already
used (spec net −29 lines). Two now-redundant `submitAdapter` copies deleted — the composition root's and a second
one duplicated inside `internal/integration`. **Both halves are guarded, both proven to fail:**
`TestSubmit_CarriesTheWholeIntent` (drop RuntimeTgt's json tag → red) and a **compile-time** `intent-contract.test.ts`
asserting the form's output *is* a valid API `Intent` (rename a field → `tsc` breaks).

**One SSE stream, many listeners.** The live phases (`searching · reasoning · scoring`) exist ONLY on the wire — no
GET returns them — but the layout already opens an EventSource for cache invalidation, so a screen calling
`useLoomarrEvents` again would open a second. `LoomarrEventsProvider` owns the single subscription and fans frames
out; `useLoomarrEventListener` is a no-op outside it, so a component still renders in a story or unit test, just
without live frames.

**Built:** `IntentForm` (the hero + templates from `packages/core`, plus §13's "intent-writing hints" — era, tone,
runtime target, acquisition cap — behind a disclosure, because the blank page is solved by one good sentence, not a
form) → `useSuggestionRun` (submit → job → live phase → the proposal, matched on `jobId` client-side since
`GET /v1/proposals` filters by status but not job, and the DTO already carries it) → `GenerationProgress` →
`ProposalReview`, with approve/deny **admin-only** (§7/§11 — a member reviews without the controls). The
`ApprovalQueue` lists everything still `submitted`: approving is the only path from a proposal to `/v1/titles`, so
that list is the audit surface for what is about to spend real resources. Per §8 the stream stays a latency
optimisation — a dropped `done` frame costs a spinner, not a proposal, because the list refetches on the same event.

Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN (**170 tests**); Docker visual GREEN
(144, 0 diffs); `make e2e` GREEN. **Next: 13.4b** — Channels + Channel detail + Board.

**Phase 13.3d — the phase gate: wizard flow suite + page snapshots (2026-07-18).** Branch `feat/fe-wizard-13.3d`.
Closes Phase 13.3 (frontend-build-plan §5 gate: "wizard e2e smoke vs mocked BE green; page-level snapshot per
wizard step"). `make e2e` was a stub that exited 1; it is now real.

**What it drives.** The REAL embedded SPA build (`internal/web/dist`, served with history fallback so `/wizard`
resolves exactly as it does from the Go binary's embed), against a **mocked `/v1` via Playwright route
interception** — no MSW dependency and no stub server, so there is no second API implementation to drift from
`openapi.yaml`. The mock is deliberately **stateful**: signing in flips `me`, each one-click wiring turns its own
check green, importing marks the candidate imported. A first-run flow that can't progress proves nothing.

**The smoke walks a fresh install end to end** — bootstrap → auto-login → checklist → Live TV → webhook handshake
(asserting the URL is built from the *revealed* secret and that both apps read as listening, neither as failed) →
skip → Tunarr library → import users → guided first channel → lands on `/suggest?intent=…` with `setup.completed`
flipped. Plus both halves of first-run routing: a completed setup opens Channels, an unfinished one is sent back.
**Seven page-level snapshots**, one per step, with a `mask()` for the relative-timestamp region (real behaviour we
want rendering, just not diffing).

**The gate immediately earned its keep — a bug no unit test had caught.** On the optional **users** step, Continue
was permanently disabled: `isStepDone` returns false for any step without a server check, so an operator who
*did* import users was stranded behind a button that could never enable — only Skip worked. Optional steps must
never block; they now gate on skippability rather than completion. Only walking the whole flow as a user surfaces
that. A page snapshot also caught duplicated copy (the shell's step description and the step's own paragraph said
the same sentence), now deduped to the §11 point that actually surprises people.

**Two Playwright configs, one determinism kit.** A maintainer question ("why two configs?") exposed real
duplication: the diff ratio, launch flags, reduced-motion and retries were copy-pasted, so tuning one gate would
silently diverge from the other. They now share `playwright.shared.ts`. The configs stay separate for a concrete
reason recorded there: Playwright boots **every** configured `webServer` regardless of project filter, and the
suites have different build prerequisites (`storybook-static` vs `internal/web/dist`) — merging would force both
builds to run either gate. `make e2e` mounts the repo ROOT (not `web/`) because the SPA build lands outside `web/`,
and serves via pure-JS `http-server` for the same reason the visual suite does: the bind-mounted `node_modules`
holds the HOST's native binaries, so vite/rollup cannot run inside the Linux image.

**Proven to fail:** renaming the Live TV CTA breaks the flow and the suite goes red on all three attempts, then
green again on revert. Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN (165 tests);
Docker visual GREEN (144, 0 baseline diffs); **`make e2e` GREEN (3 specs, 7 committed page baselines)**. CLAUDE.md's
command contract updated (`make e2e` / `make e2e-update`). **Phase 13.3 COMPLETE — next: 13.4, the core product
surfaces**, which inherits the Suggest workspace the wizard now hands off to.

**Phase 13.3c — wizard steps 3–7 + the conventions guard (2026-07-18).** Branch `feat/fe-wizard-13.3c`.
Completes the operator first-run flow, and makes the FE conventions self-enforcing after they were broken twice.

**Two more doc↔code gaps closed (BE prerequisite, doc-first).** Scoping the steps surfaced two read surfaces the
docs promise but nothing implemented — the same class as `setup.completed` in 13.3b. (1) §4 says `API_TOKEN` and
`WEBHOOK_SECRET` are "viewable on demand by admins (eye toggle + copy button)", and `secrets.go` even comments that
webhook_secret is "viewable (as URL)" — but the only route returning a generated secret's value was
`POST …/regenerate`, so an operator could see the webhook secret **only by rotating it**, breaking every webhook
already configured. Added **`GET /v1/settings/secrets/{name}`** (`{value, displayable}`; SESSION_SECRET withheld),
with a test asserting **reading never rotates**. (2) §11/§13 say the admin "picks which Emby/Jellyfin accounts get
in", but `POST /v1/users/import` takes raw media-server ids and nothing listed candidates — an admin would have
needed GUIDs. Added **`GET /v1/users/candidates`** (accounts + `imported` flag), tested through the REAL provisioner
against the testkit media server, asserting the flag flips after an import.

**Steps built.** 3 **Live TV** + 5 **Tunarr library** share a `ConnectStep`: both are idempotent one-click wirings
where **the BE check, not the click, reports success** (§6 "never silent"), so the button stays available once green.
4 **Webhook handshake** shows the paste-able `/hooks/arr?token=…` URL built from the revealed secret and **polls
`setup/status` per app**, so Sonarr and Radarr flip independently while the operator is in the other tab. 6 **Import
users** renders the candidates picker — already-imported accounts stay **visible but locked** ("where did they go?"
is worse than a disabled row), and a missing media server reads as a reason to skip, not a wall. 7 **First channel**
**hands off** rather than duplicating 13.4: it flips `setup.completed` through the ordinary PATCH and drops the
operator into Suggest with the intent prefilled. Channel templates moved from `packages/fixtures` (story data) to
**`packages/core` as product data** — §13 says they ship in the bundle — with a test asserting every template is a
valid `intentSchema` intent.

**A maintainer catch worth more than the feature work.** `src/wizard/` (15 loose files) and `src/auth/` (4) both
violated the standing folder-per-module rule — which was *already written down in memory* and broken anyway, twice.
Both are now folder-per-module with co-located `.type.ts` + `index.ts`, and the shared `wiring-step.type.ts` was
dissolved so each step owns its props. The durable fix isn't better recall, it's **`src/test/structure.test.ts`**:
a conformance test that fails on a loose file, a missing barrel, or a misnamed implementation file — the same move
`story-coverage.test.ts` makes for stories and the GritQL plugins make for arrow-functions/exports. **Proven to
fail** (drop a stray `.ts` in `src/wizard/` → red). The conventions memory now records which check enforces which
rule, so a future rule change ships with its check.

Also fixed en route: `ChecklistItem` renders `hint` only on `fail`, so the webhook step's "Listening — press Test"
silently vanished; the per-app status line now lives in the step rather than bending a shared component (and its
baselines) for one caller. Gates: `make check` (30 packages) + `openapi-verify` GREEN; `make fe` GREEN (**165
tests**, +31); Docker visual GREEN (144 passing, **0 baseline diffs**). **Next: 13.3d** — the phase gate (wizard e2e
smoke vs a mocked BE + a page-level snapshot per step).

**Fix: an empty-string PATCH could silently wipe every stored secret (2026-07-18).** Branch
`fix/settings-secret-clear` (BE-only; independent of the 13.3b/form PRs). Found while hardening the FE settings form:
the FE guard I'd added only protected *our* client, so the hole was still open in the API itself.

**The hazard.** `GET /v1/settings` deliberately returns **no value** for a secret (`internal/settings/list.go` — only
`set`/`preview`, per §4 "never echoed"), while `PATCH /v1/settings` treated `""` as **delete** for *any* key
(`write.go`). So **a GET → PATCH round-trip destroyed every stored secret** — Emby token, Seerr/TMDB/LLM keys. Any
script, the break-glass `API_TOKEN` path, or a future client (mobile, a second FE) would hit it; nothing in the API
prevented it. Non-secrets were never at risk (GET returns their real values, so their round-trip is idempotent).

**The fix (doc-first, maintainer-chosen "reject + explicit DELETE").** An empty-string PATCH on a `secret` key is now
**`invalid`** ("replace-only; send a new value, or DELETE the key to clear it") — making the round-trip *loud and
harmless* rather than quietly destructive, and matching §4's own replace-only language. Clearing keeps working through
a new explicit verb: **`DELETE /v1/settings/{key}`** (admin) drops the stored override so the key reverts to
env/default — `204` cleared · `404` unknown · `409` env-pinned (the environment wins; unset the variable to manage it
in-app). Hot-applies like any write. Docs first: config-design §9 carve-out + §8 route, design §7 endpoint table.
Nothing reachable was lost — the FE already couldn't clear a secret (a secret's baseline is `""`, so the changed-only
PATCH never included it).

**Tests are the point here:** a direct regression test reproduces the naive round-trip (`library.token: ""` alongside a
real edit) and asserts the stored secret **survives**, plus the rejection carries actionable text; `Clear` is covered
for success/unknown/env-pinned, and the route test asserts the three HTTP mappings and that DELETE is admin-only (§19).
`make check` + `make openapi-verify` GREEN; `make fe` GREEN (orval picked up the route — `useSettingsClear` is ready
for 13.4 Settings' "remove this integration").

**Forms: react-hook-form → TanStack Form (2026-07-18).** Branch `refactor/fe-tanstack-form` (stacked on 13.3b).
Maintainer call, done doc-first — the third and final leg of the deliberate TanStack consolidation (Query 13.1,
Router 13.3a, Form now).

**Doc-first (§14 Forms row + frontend-design §4.3/§6 + frontend-build-plan):** the old row justified RHF as *"the
shadcn form convention"* — a justification that was already moot, since Loomarr never adopted shadcn's RHF-bound
`<Form>` wrapper (every form hand-composes `Label`+`Input`). New rationale: `zod@^3.24` implements **Standard Schema**,
which TanStack Form consumes natively, so the `packages/core` schemas pass straight in and **`@hookform/resolvers`
disappears entirely** — two deps collapse to one; field types infer from `defaultValues` (the same end-to-end typing
as orval DTOs + typed router links); `@tanstack/form-core` is framework-agnostic so mobile shares form *logic*, not
just the schemas.

**Behavior-neutral by construction:** validators run `onSubmit` (as RHF did), not `onChange` — errors stay out of the
operator's way while typing. **`LoginForm` and the bootstrap-step tests passed completely unchanged**, which was the
intended signal that neither DOM nor UX drifted; the Docker visual suite agrees (**143 passing, 0 baseline diffs**).
The open question from the plan resolved cleanly: `bootstrapSchema`'s cross-field `.refine(…, {path:["confirm"]})`
does surface on the `confirm` field through Standard Schema.

**`SettingsGroupForm` converted from controlled-props to owning its state** (maintainer chose the wider scope), which
surfaced **two real findings**. (1) **TanStack Form reads a dot in a field name as a nested path** — and every registry
key is dotted, so `name="library.url"` wrote `{library:{url}}` instead of the flat key the API expects. Values are now
carried **positionally** (aligned with `entries`), keeping the form agnostic to key shape; the mapping back to dotted
keys lives in one tested helper. (2) Because the form now produces the PATCH body, it must submit **only changed**
fields: a stored secret always reads back as `""` (§4 never echoes it) and an empty-string PATCH **clears** an optional
key (§9) — so submitting every field would have silently wiped every stored secret on save. `changedEntries` enforces
that, with a dedicated test asserting an untouched secret is absent from the body. The form deliberately does **not**
re-seed on `entries` change (a background refetch must never overwrite typing); a caller wanting a fresh baseline
remounts with a `key`.

Gates: `make fe` GREEN (**134 tests**), Docker visual GREEN (143, **0 diffs**), `make check` GREEN (no Go changes).

**Phase 13.3b — wizard foundation + steps 1–2 (2026-07-18).** Branch `feat/fe-wizard-13.3b`. The settings-driven
wizard machinery, built on config-design §6's rule that **the wizard IS the settings system** — no parallel form
system; each step renders a registry group's form through the same `PATCH /v1/settings` path Settings (13.4) will use.

**Doc↔code gap closed (doc-first):** config-design §6 specified *"a `setup_completed` flag in the registry"* but no
such key existed (checked all 47). Added **`setup.completed`** (bool, `SETUP_COMPLETED`, Advanced, GroupAdvanced) to
`internal/settings/declared.go`, and amended §6 to name the key exactly + note it's written through the ordinary PATCH.
`api/openapi.yaml` unchanged (registry keys are data, not schema — `openapi-verify` clean); `docs/configuration.md`
regenerated (+1 row).

**Built — the reusable half (all Layer-2, story + test each):** `SettingField` — ONE `SettingEntry` as a control,
everything from contract data: `kind` picks the widget (bool→Checkbox, enum→Select, int/url/string→Input,
secret→password), `enum` fills options, `doc` is help text, **`provenance:"env"` locks the field** with a "set via
environment" chip (§3 visible provenance), a stored secret shows its masked `preview` tail with **replace-only**
editing (§4 — never echoed), `caution` explains a self-healed value, and a `SettingResult` renders `invalid(problem)`
inline / `pinned` as a badge. `SettingsGroupForm` — a group's fields + the per-group **Show advanced (n)** toggle (§5)
+ inline live-test result + Save. Two new **dependency-free ui primitives** (`Checkbox`, `Select`) as native elements —
deliberately not Radix, so no §14 dep conversation for a checkbox and a 2-option list. `humanizeSettingKey` added to
`packages/core` (`library.url` → "Library URL") because the settings API ships `doc` but **no display label**; derived
once so wizard + Settings can't drift, and it doubles as the checklist's check-name humanizer.

**Built — the wizard:** `WizardShell` (rail + card + nav; step states `done|current|pending|skipped` — a **skipped
optional step renders neutral, never red**, §6). `src/wizard/steps.ts` holds the **resume-safe derivation as pure,
tested functions**: completion comes from server truth (`GET /v1/setup/status` + whether a session exists), never from
client progress, so a refresh — or finishing from another browser — lands correctly. Required = **media_server +
tunarr** only (§6 "shortest honest path"); Seerr/AI/TMDB/filler report but never block. The three wiring checks
(`livetv`/`webhook`/`tunarr_library`) each belong to their own later step, so Connections doesn't double-count them.
**Step 1 Bootstrap** — `POST /v1/setup/bootstrap` with `bootstrapSchema` (already in core), then **auto-signs in with
the same credentials** (bootstrap issues no session) so the operator types the password once; a 409 isn't an error to
explain away, it's "this instance is past bootstrap → sign in". **Step 2 Checklist** — reuses the 13.2 `ChecklistItem`,
driven by `setup/status`, failures shown as the BE's plain-language hint + doc deep-link (no stack traces, ever).
**First-run routing:** `/` reads `setup.completed` and routes to `/wizard` until set — and **fails open** (a member's
403, a 500, a missing key ⇒ "completed"), so a non-admin is never trapped in operator-only setup.

**A real bug the tests caught:** the wizard computed its resume step before `me`/`setup-status` settled, so it painted
the wrong step and then yanked the operator forward (checklist → TV guide). Now it holds the paint until both settle
and lands right the first time. Gates: `make check` + `openapi-verify` GREEN; `make fe` GREEN (**129 tests**, +27 this
phase); Docker visual GREEN — **143 passing, 32 new baselines** (16 stories × 2 viewports), a11y clean on every new
component. **Next: 13.3c** (steps 3–7: livetv-connect, webhook handshake, tunarr-connect, user import, first channel)
then **13.3d** (gate: wizard e2e smoke vs mocked BE + a page snapshot per step).

**Phase 13.3a — auth foundation on TanStack Router (2026-07-18).** Branch `feat/fe-auth-13.3a`. The FE identity
layer the wizard + product surfaces sit behind (§11, §12) — built, then **the router was swapped react-router →
TanStack Router** mid-branch (maintainer call, doc-first) before more surfaces land, so 13.3a arrives already on
the typed router.

**Router swap (doc-first §14/§12, frontend-design, frontend-build-plan):** `react-router` v6 → **`@tanstack/react-router`
(file-based)**. Rationale in §14: end-to-end type-safe routing (typed params/search/links) matching the orval-contract
ethos; shares the TanStack Query client via router `context`; loader-based auth guards (`beforeLoad` → `redirect`, no
guard-flash). Web-only — routing was always the per-platform seam (mobile keeps Expo Router). `@tanstack/router-plugin`
(Vite) + `-cli` (`tsr generate`) generate `src/routeTree.gen.ts` — **gitignored + Biome-ignored + regenerated by
`pnpm codegen`**, exactly like orval output (never hand-edited). Route tree is `src/routes/`: `__root` (carries the
queryClient context), public `login` + `wizard`, and a pathless **`_authed`** guard layout whose `beforeLoad`
`ensureQueryData(meQueryOptions)` throws `redirect({to:"/login", search:{redirect: location.href}})` on 401 — the app
shell + all stub screens hang off it. `main.tsx` builds `createRouter({routeTree, context:{queryClient}})` + the
`Register` module augmentation (global type-safe Links).

**Auth pieces:** `useAuth` — the one interpreter of `GET /v1/auth/me` via shared `meQueryOptions` (`retry:false`; a 401
is a definite answer), narrowing the success|error union by status → `{user, isAuthenticated, isAdmin, isLoading, error}`
(feeds AppShell name/role). `RequireAuth` **deleted** — the guard is now `_authed`'s `beforeLoad`. `LoginForm` —
presentational (RHF + `zodResolver(loginSchema)` from `packages/core`, shared verbatim with mobile), block-level failure
via `ErrorState` (RFC 7807 → words), field errors inline, no user enumeration. `login` route wires `authApi.useLogin`
(invalidate me + `history.push` to the typed `redirect` search param on success; `beforeLoad` bounces an already-authed
visitor). AppShell `NavLink` → TanStack **`Link`** (active state via `data-status="active"`, so it stays a pure-className
component) + a footer sign-out; `_authed` layout feeds the real user + logout (`authApi.useLogout` → `queryClient.clear()`
→ `/login`). **Kept 1:1:** `LoginForm.onSubmit` takes generated `LoginInputBody`, identity reads `MeBody`. `Placeholder`
moved out of `src/routes/` (file-based = every file is a route) → `components/loomarr` (+ story; its "Dead air" label was
`text-static-500` at 2.94:1 — bumped to `static-400` so the newly-covered a11y gate passes, per the 13.2 precedent).

**Test/story router harness** (the swap's one real cost): TanStack `Link`/route hooks need a RouterProvider even in
isolation — added `RouterHarness`/`withRouter` in `test/story-utils` (a minimal in-memory router over the nav paths).
Coverage: a **router-level `app-router.test`** drives the REAL generated tree — signed-out → login form, authed → screen,
sign-in → Channels (replaces the old login + require-auth component tests, preserves §19 negative-auth). `make fe` GREEN
(codegen now generates the route tree before typecheck; 77 units); Docker visual GREEN — 6 `loginform` + 2 `placeholder`
baselines added, 4 `appshell` regenerated (Link renders pixel-equivalent; sub-pixel delta only). **Next: 13.3b/c** (the
7-step wizard) then **13.3d** (wizard e2e smoke vs mocked BE + a page snapshot per step).

**Contract 1:1 hardening — BE proposal typing + FE de-duplication (2026-07-18).** Branch
`feat/fe-contract-1to1`. A maintainer catch ("is anything hand-written that shouldn't be?") — audited the
FE against the generated client and found several hand-mirrored types (design §12 = no hand-written glue).
**BE:** `ProposalDTO.proposal` was `json.RawMessage` (→ OpenAPI `unknown`); now typed as `suggest.Proposal`
directly (imported the domain struct, dropping the old local `proposalBody`/`lineupItem`/`acqItem` mirrors),
so orval generates the full `Proposal`/`ProposalItem`/`Scores`/`ChannelPolicy` schema — true 1:1. **FE:**
deleted the mirrors — `Clip`/`ClipKind`/`ClipAudience` → `ClipDTO*`, `ProposalView`/`ProposalItemView` →
generated `Proposal`/`ProposalItem`, `ProblemDetail` → `ErrorModel` (deleted `mutator.type.ts`),
`ProvisioningState` → `TitleDTOState | "drift"` (5 states from the generated union + the one FE-only state).
Kept the genuine FE view models — the ⌘K `PaletteScope` (renamed from the colliding `SearchScope`) and the
derived `ChannelHealth` rollup — now documented as such. Forced a real honesty fix: generated
`ProposalItem[]` is `| null` (nil Go slice), so ProposalReview now null-guards. `make check` (BE incl. the
policy round-trip integration test) + `make fe` + the Docker visual suite (104/104, renders unchanged) all
GREEN; `api/openapi.yaml` regenerated + committed. Docs: frontend-build-plan §3 corrected.

**Phase 13.2b/c/d — remaining components + Storybook adoption (2026-07-17).** Branch
`feat/fe-gallery-visual-13.2`. **13.2b DONE:** the remaining Layer-2 components (IntentInput,
GenerationProgress, ProposalReview, PodTimeline, ClipCard, ApprovalQueueItem, SearchCommand) + a shared
`Badge` primitive + `formatClipDuration` (sub-minute-aware) — all with CVA/tokens, states, a11y, tests;
`make fe` + `make check` were GREEN before the Storybook pivot.
**DECISION (maintainer, doc-first): adopt Storybook 10 for the component gallery/workshop AND
visual/a11y testing, replacing the hand-rolled `/__gallery` registry.** This reverses frontend-design §5's
original mechanism, so the docs were updated FIRST (per prime directive #1): frontend-design §3/§4.1/§4.2/
§5/§7, design §14 (new dep row + rationale; **Chromatic rejected** — hosted SaaS breaks the offline rule),
frontend-build-plan §4/§9/§10, CLAUDE.md command contract. **Rationale:** CSF is the industry-standard
component contract + a real dev workshop (controls/autodocs/a11y panel), and it carries to the future
mobile app via `@storybook/react-native` (Expo, on-device). **Every §5 guarantee preserved:** offline
(`storybook-static`), deterministic (Playwright Docker, `document.fonts.ready`, `prefers-reduced-motion`
+ `animations:'disabled'`, frozen clock), committed baselines (`toHaveScreenshot` `maxDiffPixelRatio 0.001`),
and 100%-coverage-enforced (a test maps the component barrel → stories). a11y: `@storybook/addon-a11y` in
the workshop + `@axe-core/playwright` in the *same* Playwright pass over `storybook-static` (one browser
layer for pixels + axe; `addon-vitest` deferred as an optional future for play/interaction tests).
**Mobile-ready:** deterministic fixtures move to
a shared `packages/fixtures`, data contracts to `packages/core`, so web + future RN stories share args.
**Built:** Storybook 10 + addon-a11y; the hand-rolled registry replaced by 52 co-located CSF stories across
15 components (exports-at-end — the maintainer rejected exempting stories from the `no-inline-export` plugin,
and Storybook 10 indexes `export { … }` + `export default meta` fine); `packages/fixtures` + core contracts;
the story-coverage test. **The a11y gate immediately caught + fixed three real WCAG bugs** the earlier
components shipped: informational `text-static-500` (2.94:1, §2.1 says decorative-only) → `static-400`;
`opacity-70` on a denied approval card compositing all its text below AA → removed; a `<li role="alert">`
breaking the `<ol>` list structure → moved to a sibling `<p role="alert">`. **Visual determinism (the hard
part):** `make fe-visual`/`-update` now run Playwright **inside the pinned `mcr.microsoft.com/playwright`
image** (the reference rasterizer, §5.2) — Chromatic still rejected — reusing the host's JS-only node_modules
+ the image's browsers (no in-container install, host binaries untouched). Getting a stable green took three
fixes beyond the doc's kit: `animation: none` (reduced-motion only fast-forwards, leaving infinite spinners
on a random frame), **element-scoped** snapshots (`#storybook-root`, not the centered page whose fractional
margins shift text AA), and **retries** for residual sub-pixel jitter (a real diff still fails every attempt).
**104 `*-linux.png` baselines committed** (non-Linux suffixes gitignored). `make fe` + `make check` GREEN;
the Docker visual suite passes (flaky-on-retry ≤2/104). frontend-design §5.1/§5.2 updated to match.

**Phase 13.2a — design-system foundation: components, conventions, Biome (2026-07-17).** Branch
`feat/fe-design-system-13.2`. The Layer-2 component vocabulary + the codebase conventions + linting that
everything downstream (wizard, surfaces) builds on. **Self-hosted Geist** (@fontsource-variable, bundled
by Vite — visual-test determinism, §2.2). **shadcn primitives** (Button/Card/Input/Label, restyled via
tokens only). **Layer-2 components** (frontend-design §3): StateBadge, OnAirIndicator, NowNextStrip,
ChannelCard, EmptyState, ErrorState (RFC7807 renderer), ChecklistItem, AppShell — each with CVA/tokens,
all states, a11y (sr-only status, sentence-case DOM text so SR reads words not letter-spaced shouting).
**Maintainer conventions established mid-build (saved to auto-memory [[fe-code-conventions]])** and applied
across the WHOLE tree (app + packages, orval-generated excepted): (1) arrow-function expressions, (2)
exports in a single block at end of file, (3) folder-per-component/module with `name.tsx` + `name.type.ts`
+ `name.test.tsx` + `index.ts` barrel, (4) types isolated in `*.type.ts`, (5) barrel imports, (6) a
Vitest+Testing-Library test per module. **Biome** (maintainer's call over ESLint) for lint+format with
sensible settings, wired into `make fe` + new `fe-lint`/`fe-lint-fix` targets; Tailwind-v4 CSS excluded
(Biome's CSS parser can't read @theme/@custom-variant), `noLabelWithoutControl` off (false-positive on the
Label primitive), `useSortedClasses` on (Tailwind class sort in cn/cva/clsx — kills visual-diff churn).
**Two custom Biome GritQL plugins** (maintainer chose the Biome-native path over ESLint-hybrid/ts-morph;
current API verified via web docs since training predates the stable release) enforce the rules Biome
lacks built-ins for: `no-function-declaration.grit` (arrow expressions only) and `no-inline-export.grit`
(declare-then-export-block — catches `export const|let|var` + `export type X =`; `export function` falls
to the first plugin). Known GritQL gap (documented in-plugin): Biome 2.5 doesn't match TS
`export interface|class|enum` nodes, but the `*.type.ts` convention already routes types into export
blocks. A `no-raw-hex-color` plugin (enforce the §2 token layer) was prototyped but dropped — GritQL
regex doesn't reliably scan string-literal contents; noted as a follow-up. **Toolchain fix:**
`vitest@2→3` to dedupe `vite` to v6 (the v5/v6 skew broke the config types). Tests: **28 app + package
tests, all green** (StateBadge/OnAir/ChannelCard/EmptyState/ErrorState/ChecklistItem/AppShell + button/
card/input/label + Layout; core events/format/schemas; api mutator; tokens palette/contrast). `make fe`
GREEN (biome + codegen + typecheck 4 pkgs + tests + embedded build), `make check` GREEN. **Deferred to the
next PR (13.2b–d):** the remaining Layer-2 (IntentInput, GenerationProgress, ProposalReview, PodTimeline,
ClipCard, ApprovalQueueItem, SearchCommand), the `/__gallery` registry, and the Playwright visual + axe
harness.

**Phase 13.1 — FE workspace skeleton + token pipeline (2026-07-17).** Branch `feat/fe-scaffold-13.1`.
The greenfield frontend foundation (`frontend-design.md` §2.5/§4, §7 "Phase 1"): a pnpm monorepo under
`web/` with the shared packages the future Expo app bolts onto. **packages/tokens** — the Test Card
palette as the TS source of truth + a deterministic generator emitting `theme.css` (Tailwind v4 @theme),
a NativeWind-ready `tailwind-preset.cjs`, and `tokens.json`; its **contrast gate reproduces the design's
published WCAG ratios to the decimal** (onair base 4.01→-300 4.53, suggest 3.86→4.65) and fails the build
on any on-tint regression (§2.1). **packages/api** — orval generates typed TanStack Query hooks from
`api/openapi.yaml` (incl. the 6 routes 13.0 rescued — `useLogin`/`useMe`/`useBootstrap` now exist),
namespaced per tag, over a shared fetch mutator (same-origin, cookie creds, `X-Loomarr-Csrf`, RFC7807 →
`ApiError`). **packages/core** — the SSE invalidation bus (maps the BE's `title`/`channel`/`suggestion`/
`llm_pull` frames → coarse query invalidation, the §8 "GET is truth on reconnect" contract), zod schemas
(intent/bootstrap/login — reused by RN later), formatters. **apps/web** — Vite + React 18 + Tailwind v4 +
shadcn (new-york) with the AppShell nav rail, react-router skeleton, QueryClient + SSE providers, sonner.
The build embeds into the Go binary: **internal/web/embed.go** (`//go:embed all:dist`) serves the SPA at
`/` (SPA fallback for client routes; API prefixes guarded to 404, never HTML) with a committed
`.gitkeep` so `go build` works Go-only (serves a "run make fe" notice). Makefile gained
`fe`/`fe-tokens`/`fe-tokens-verify`/`fe-codegen`/`fe-install`. **Verified:** `make check` GREEN (added
SPA-served + guard assertions to `TestWiring_FreshInstall`; the absent-route contract shifted 405→**404**
uniformly via the SPA guard — invariant "absent ≠ 501" preserved), `make fe` GREEN (codegen + typecheck
4 pkgs + tokens/core unit tests + build), `fe-tokens-verify` GREEN. Toolchain decision recorded: **Node
20+/pnpm/Vite** (not bun/deno) — §14-decided, keeps the Expo bridge + CI determinism. Self-hosted Geist
deferred to 13.2 (visual-determinism concern; token font-stack falls back meanwhile). orval output is
gitignored (regenerated from the spec by `make fe`); token artifacts are committed (drift-checked).

**Phase 13.0 — BE contract closed for the FE (2026-07-17).** Branch `feat/be-contract-13.0`. The
prerequisite before any FE code (see `docs/frontend-build-plan.md`): make every route the FE calls
present + typed and every wizard/workspace surface fully BE-backed. Doc-first §7/§8/§13 updated, then:
**(1) OpenAPI coverage** — the 6 routes orval couldn't type (`auth/login|logout|me`, `setup/bootstrap`,
`users/import`, `users/sync`) were absent from the exported spec because `ExportOpenAPI` builds a bare
`Server{}` and the `register*` funcs nil-guard; added a `schemaOnly` flag (set only by export) so their
SCHEMAS emit into `api/openapi.yaml` while runtime nil-guarding is untouched (+296 lines, all additive).
**(2) setup/status completed** — was only `livetv` + `tunarr_library` (its own handler admitted "added by
their phases"); now aggregates the connection probes (`media_server`, `requester`, `tunarr`, `llm` incl.
local reachable+pulled / hosted key-present, `tmdb` via a cheap known-id lookup, `filler` when
configured) reusing the §8 `setup/test` registry, plus the `webhook` handshake check carrying per-app
`sonarr`/`radarr` `lastReceived` (read from the KV the ingest handler writes), each with a `docHref`
Troubleshooting deep-link. Added `llm`+`tmdb` probes to `connectionTests`. **(3) Suggester progress
events** (maintainer chose real per-step over indeterminate) — the worker published nothing intermediate;
now `Suggest` reports `searching`→`reasoning`→`scoring` via a **context-threaded** `ProgressFunc` (zero
signature churn across 13 call sites; a bare ctx = no-op), and the worker emits `done`/`failed` around it
through a narrow `ProgressEmitter` wired in the composition root to publish SSE `type=suggestion` frames
`{jobId, phase}` (parallel to `llm_pull`; the `eventEmitter` gained `SuggestionPhase`). Tests:
`TestProgress_ReportsOrderedPhases` (unit — clean run emits exactly searching→reasoning→scoring),
`TestSetupStatus_FullChecklist` (integration — connection checks carry docHref, webhook check waits with
empty lastReceived). `make check` GREEN; `make openapi` regenerated (commit makes `openapi-verify` green).
Closes findings 1–4 of the FE↔BE audit; finding 5 (coarse drop-tolerant SSE) is a documented property.

**Phase-13 FE plan reviewed vs the live BE — 5 seams found, plan written (2026-07-17).** Before writing any
FE, audited `frontend-design.md` + `design/` prototypes against the real BE contract (32 typed ops + 8
raw routes + the 3-type `title`/`channel`/`job` SSE bus). Found **5 FE↔BE seams**: (1) 6 core routes —
`auth/login|logout|me`, `setup/bootstrap`, `users/import`, `users/sync` — are **not in the exported
`openapi.yaml`** so orval can't type them; (2) `GET /v1/setup/status` returns only `livetv` +
`tunarr_library`, **not the full §13 checklist** (its own handler comment admits "added by their
phases" — they weren't); (3) **no read surface for webhook handshake timestamps** though the store
tracks them (`store.go:102,112`); (4) the **suggester emits no per-step progress** (worker publishes
nothing intermediate) so the mock's `GenerationProgress` steps are unbacked; (5) SSE is coarse +
drop-tolerant (a design property, not a bug — FE must invalidate-and-refetch). Also confirmed the design
is **already mobile/Expo-aware** (`frontend-design.md` §2.5/§4.2: shared `packages/{tokens,api,core}` +
token→NativeWind preset bridge; Expo+NativeWind+RN-Reusables pre-decided). **Maintainer decisions:**
(a) close the contract **first** as a **13.0 PR**; (b) mobile **ready now, build web only** (Expo app is
a future phase + a §14 update); (c) **add real per-step suggester progress events**
(`searching/reasoning/scoring/done/failed`). Full sequenced plan (13.0 close-contract → 13.1 monorepo +
tokens → 13.2 gallery + visual harness → 13.3 wizard → 13.4 surfaces → 13.5 gate), the page→endpoint→SSE
coverage map, and the mobile-auth flag (**native app needs CORS + per-user bearer tokens — a §11/§14
conversation, out of scope for 13**) live in **`docs/frontend-build-plan.md`**. No code yet; next action
is the 13.0 BE PR (doc-first §13/§8).

**Tunarr media-source auto-wiring (tunarr-connect) — onboarding gap closed (2026-07-16).** Branch
`feat/tunarr-autoconnect`. A live-smoke question ("is the Tunarr↔Emby wiring in onboarding?") found
a real gap: the design *required* Tunarr to have the media server as *its* source, enabled+scanned
(§6, Phase-0 #12), but nothing in the wizard did it — an operator could finish all-green yet get
**dead-air channels** (empty Tunarr program table). Maintainer chose **full automation**. **Design
win proven live:** Tunarr accepts Loomarr's **admin API key** as the source access token (verified
vs Tunarr 1.3.8 + Emby 4.10) — so **no Emby user login, no new credential, no §15 expansion**.
Delivered (doc-first §6/§7/§13): `programmer` gains `EnsureEmbySource`/`ConnectLibraries`/
`MediaLibrariesReady` (media-source CRUD + enable/scan of the movie+show libraries, idempotent);
`setup.MediaSourceConnector` orchestrates (resolves the admin userId via `library.ListUsers`, live
flavor/url/token); `POST /v1/setup/tunarr-connect` (admin, idempotent) + a `tunarr_library` check in
`/v1/setup/status`; wired in `internal/app` (the connector uses the same programmer the tests inject,
via a media-source interface type-assert); testkit Tunarr double implements the interface. Tests:
programmer unit (create-then-reuse idempotent, movies+shows-only enable, ready-gate), integration
`TestJourney_TunarrConnect` (real connector → double: connect → 2 libs → `tunarr_library` flips →
idempotent re-run), member-403 matrix + fresh-install 501 extended. **Dogfooded LIVE:** POST
tunarr-connect against real Tunarr returned the existing source **idempotently** (librariesEnabled=2)
and `/v1/setup/status` flipped `tunarr_library: ok=true`. `make check` green.

**Live BE smoke on the Mac — 2 findings (2026-07-16).** Branch `fix/picker-prober-live`. First
real drive of THIS session's changes against the homelab: native Ollama (qwen3.5:9b) + Emby/Seerr
over Tailscale, a FRESH app boot configured through the settings API (the wizard path, not env
pins). **Proven live:** the live-enable fix (fresh install `POST /v1/suggestions` → 501; PATCH
connections → **200 with no restart**); the real connection probes (`media_server` ListUsers +
the NEW `requester` `Seerr.Reachable`, both ok over Tailscale); features flip live. **Finding 1
(FIXED):** the model-picker's Prober base URL was **frozen at boot** (`llm.NewProber(set.str("llm.url"))`),
so configuring `llm.url` via the wizard left the picker reporting `reachable:false` / `pulled:false`
until a restart — the same class as the live-enable gap, and the integration harness missed it
because it seeds `llm.url` BEFORE build. Fix: `systemLLMService` builds the Prober per-call from a
live `ollamaBase()` resolver (like the suggester's Swappable hot-swap); regression added to
`TestWiring_ConfigEnablesLive` (picker reachable after PATCH); **verified live** (reachable=true,
qwen3.5:9b pulled=true after the PATCH). **Finding 2 (model-quality, documented — NOT a code/seam
bug):** qwen3.5:9b (the catalog's top-recommended local model) emits conversational prose instead
of the final JSON proposal (`invalid character 'C'`), so a real grounded job fails the JSON-repair
loop. `Think:false` is already applied on tool turns and the job failed **cleanly** (the graceful
"no valid proposal" path, which is unit-tested) — the SYSTEM operated as designed; the MODEL is the
weak link. Actionable: prefer a known-good local model (qwen3:8b / qwen3:14b Q6_K / llama3.1:8b) or
a hosted model; the catalog's qwen3.5:9b recommendation warrants review; a possible follow-up is a
final-turn `format:json` (tools dropped) to coerce weak models (doc-first, §8 grounding — separate).
**Phase 2 (a real Tunarr channel) not yet run** — needs the dev Tunarr wired to Emby (maintainer creds).

**E2E integration seams + composition-root testability + live-enable fix (2026-07-16).**
On branch `feat/e2e-integration-seams`. Pre-FE hardening: drive the WHOLE app (real composition,
not a hand-wired subset) through every FE-facing flow so the frontend meets a seam-free backend.
**Composition seam:** extracted `run()`'s 260-line wiring body into an importable `internal/app`
package — `app.BuildHandler(ctx, st, log, Overrides) (http.Handler, error)` — that both `run()`
(production) and the tests call, so tests exercise the REAL `api.Options` wiring. `cmd/loomarr/main.go`
shrank 710→133 lines (thin entrypoint); the package is split by concern (`app`/`systemllm`/
`settingsadapter`/`settingsboot`/`filler`/`adapters`/`emitter`/`ids`). `Overrides` injects the two
in-process boundaries (Tunarr `programmer.Programmer`, scripted `llm.Provider`) + a TMDB base override;
library/seerr are real adapters over testkit HTTP doubles via seeded settings. **New testkit
`Ollama`** HTTP double (`/api/version`,`/api/tags`,`/api/pull` stream) so the §8.1 picker
(probe→select→pull + SSE) runs through the real `systemLLMService`+`Prober`. **New E2E suite**
(`internal/integration`, real `app.BuildHandler`, testkit-only, in `make check`): a **new-admin
journey** (bootstrap→409-on-2nd→**local bcrypt login**→settings/feature-gates→`/setup/test` real
probe→picker probe/select(409-unpulled)/pull→intent→approve→channel-with-policy-enforcement→reconcile-
idempotent), a **member journey** (real import→media-server login→allowed set→**403 across the FULL
admin matrix** incl. settings/system-llm/setup/filler/backup that the old §19 test omitted→disable-
kills-session), and a **wiring** file (fresh-install 501/405 nil-dep matrix + the hot-apply proof), plus an **SSE
E2E** test (authenticated subscribe → pull → assert an `llm_pull` frame arrives — the FE's
live-update channel, previously only 401-tested). **Two pre-FE gaps a self-audit found, closed:**
(a) the wizard's "Test Seerr" button had NO backend — added `Seerr.Reachable` (validates URL+key,
no side effects) + the `requester` check in `connectionTests` + a testkit `/settings/main`
endpoint; the admin journey now drives all three probes (media_server/tunarr/requester); (b) the
SSE delivery test above.
**Live-enable fix (honors config-design §3 / §8.1 "no restart"):** the audit-flagged gap — a saved
connection flipped the `features` map but its route stayed **501 until restart** (services were
nil-wired at boot). Fixed by **always-constructing** the feature services (reconciler/channels/
suggester/filler, given a store) with the existing dynamic per-call providers, and moving each
handler gate from `s.X == nil` to a live check — `featureOff(ctx, feature)` (Features() snapshot) for
suggestions/filler, `unconfigured(key)` (live `set.str`) for search/channels/livetv, picker always-on.
The gate is **additive** (`nil OR live-off`), so the api-package unit tests (which wire deps directly,
no live source) are untouched. `TestWiring_ConfigEnablesLive` PROVES a PATCH to `/v1/settings` enables
`/v1/suggestions`+`/v1/search`+reconcile **with no restart**. **Known caveat:** the library *flavor*
is fixed at construction (defaults to Emby), so switching to Jellyfin still needs a restart — url/token
hot-apply; follow-up is a live flavor closure (~15 auth call sites). Gates: `make check` (`-race`, lint
0, config-docs) + `make test-pg` + boot smoke (fresh-install bootstrap 200, `/readyz` ready, clean
shutdown) all green. NOT a phase — pre-Phase-13 hardening; unblocks the FE build on a proven backend.

**LLM provider surface + pull-path fixes + Mac/Linux dev portability (2026-07-16).** Live dev
bring-up on an Apple-Silicon Mac surfaced two §8.1 pull bugs and drove a provider-surface decision
(all `make check` green). **Fixed:** (1) a model **pull aborted at 120s** — `Prober.Pull` used a
whole-request `http.Client.Timeout` (`TimeoutLLM`) that kills a multi-GB stream mid-body; added
`httpx.NewStreaming()` (no whole-request budget, ctx-governed; connect/TLS/header stages still
bounded) + regression test. (2) **pull progress now surfaces raw bytes** — exported
`llm.PullProgress{Status,Completed,Total}`; the `/v1/events` `llm_pull` SSE frame carries
`completed`/`total` so the FE renders "X of Y GB" + derives ETA (was percent-only). **Design
decision (doc-first, §8/§8.1):** the hosted LLM surface narrows to **OpenRouter** (the blessed
aggregator — one key → every frontier family) + **Custom** (a user-supplied OpenAI-compatible base
URL, gated by live validation, not an allowlist); the curated openai/anthropic/groq/gemini entries
are dropped (reachable via OpenRouter or Custom). Family-tier ranking unchanged. **Dev:**
`compose.dev.yaml` is host-agnostic now (`platform: linux/amd64`, `MEDIA_SERVER_IP` override); NVIDIA
transcode is an opt-in `compose.dev.gpu.yaml` overlay (`make dev-gpu`). Verified live: app native vs
Emby+Seerr+TMDB (Matrix grounding), Ollama on Metal, the §8.1 picker (probe→pull→select). A
cross-cutting fix/refinement, **not a phase** — Phase 13 (Web UI) is still next.

**Auth/identity rework (§11) — COMPLETE (2026-07-15).** On branch `feat/auth-rework` (commits
`4879470`..`4af00e2`), NOT yet merged to `main`. Replaced the claim-on-login / lazy-self-provision
model with **Loomarr-owned identity**: the `users` table is the source of truth + allowlist. Gate:
`make check` (`-race`, lint 0) + `make test-pg` (migration `00009` on both dialects) + `openapi-verify`
+ `config-docs-verify`, **plus a live boot smoke with ZERO media-server config** — `POST /v1/setup/bootstrap`
created the owning admin, a 2nd call 409'd, local admin login returned an HttpOnly/SameSite=Strict session
cookie, wrong password 401'd, and the users table held exactly one row (admin, bcrypt hash set). Delivered:
migration `00009` (nullable `users.password_hash` — set ⇒ local/bcrypt user, null ⇒ imported media-server
user, the credential-path discriminator); `login.go` enforces the allowlist (a name+hash verifies in-app,
else verify vs the media server AND confirm the id is imported — an un-imported user is **rejected even with
valid Emby creds**, no lazy provision; all failures return one `ErrInvalidCredentials`, no user enumeration;
works with a nil media server = local-only); `Provisioner.Bootstrap` (first local admin, once via
`CountAdmins()==0`) + `Provisioner.Import` (explicit media-server ids, admin-only, the ONLY add path);
`POST /v1/setup/bootstrap` (unauthenticated, self-gated) + `POST /v1/users/import` (admin-only);
`store.GetUserByName`. **Closed BOTH lazy-provision hatches:** login (`syncUser` add-branch removed) AND
periodic sync (`UserSync.Sync` now skips un-imported users — it refreshes, never adds, else a sync would
silently re-import everyone). `bcrypt` promoted to a direct dep (§14 updated). Existing auth/flow tests
updated to seed the allowlist first (a stricter contract, not weakened). Reworked doc §11 + reconciled
§13 wizard (Claim→Bootstrap + Import steps), §16, §19 test spec, §21 phase-9/13 gate text. Supersedes the
deferred `loomarr-auth-rework` memory item. Unblocks Phase 13's wizard "Bootstrap" + "Import users" steps.

**Settings subsystem — cross-phase config retrofit — COMPLETE (2026-07-15).** Built `config-design.md`
for real (the deferred Phase-1/8/9 config work) on branch `feat/settings-subsystem` (commits
`7aa3fcc`..`17fe3cb`). Gate: `make check` (`-race`, lint 0) + `make test-pg` (settings audit columns on
both dialects) + `make openapi-verify` + `make config-docs-verify` all green, **plus a live boot smoke**
(temp SQLite): `/healthz` 200, `/readyz` ready, three generated secrets minted + persisted with audit
stamp, `GET /v1/settings` 403 unauth / 47 settings with the API_TOKEN break-glass (secrets **masked**,
value withheld), env-pin reported + **refused** on PATCH ("set via environment"), `job.workers` hot-applied
to db, and the feature gate flipped `acquisition` true the instant `seerr.url` was saved — all with **no
restart**. Delivered: a typed **registry** (single source of truth, ~45 keys transcribed from §15),
`env > database > default` resolution with **asymmetric errors** (bad env → boot fail; bad db → self-heal +
caution), `_FILE` secret loading + `<VAR>`+`_FILE` ambiguity boot-error, in-memory snapshot + `Watch`
**hot-apply**, the secrets lifecycle (idempotent gen, `Redactor` into slog — the **log-grep gate** proves
no secret is ever logged, masked reads, regen side-effects), feature gating from `RequiredFor` (the
requester OR-gate is the one explicit case), the `/v1/settings` + `/v1/setup/test` + secrets-regenerate
API, and `make config-docs` (→ `docs/configuration.md`, drift-gated in `make check` too). New
`internal/settings` package; `config.Config` shrunk to the env-only bootstrap set (§1: `DATABASE_URL`/
`AUTO_MIGRATE`/`LISTEN_ADDR`/`LOG_LEVEL`/`TZ`). **Full read-through rewire** — every consumer resolves via
the snapshot (library/requester/Tunarr connection providers read PER CALL; `reconcile`/`channels` runners
gained `WithInterval` re-tune; the LLM `Watch(llm.*)`-rebuilds). Migration `00008` adds settings
`updated_at`/`updated_by` (2nd real ALTER after `00007`). Closes the ChannelPolicy registry-default
deferral: the `SCHED_*`/`SEASONAL_MODE` policy defaults (§15) now resolve through the registry, not Go
constants. Like ChannelPolicy, a **cross-phase retrofit** (deepens Phase 1/8/9), not a new phase-table row.
**Unblocks Phase 13's wizard-as-settings** (`config-design.md` §5–§7). NOT yet merged to `main` (branch
awaits review). Known small follow-up: `Router`/`ExportOpenAPI` still duplicate the route-registration
list (a shared `registerAll` is the real fix); the media-server/tunarr connection Test probes are shallow
reachability checks.

**Phase 12.5 — End-to-end integration (the seams) — COMPLETE (2026-07-14).** All live-smoke seams
closed: #6/#7/#8/#12/#13 (earlier), then #9 (acquisitions→`ch.Lineup` pending), #10 (provisioner→
scheduler `eventEmitter`), #11 (`/v1/events` SSE), and the §10 filler redesign (Loomarr-owned
commercials via a Tunarr `local` source + per-channel filler-lists). The Emby ~4s Live-TV playback
stop was a **Firefox** client quirk (no code change; troubleshooting note added). Phases 0–12.5 done;
**next: Phase 13 (Web UI + onboarding — recreate the imported `design/` prototypes pixel-perfect in
Vite+React+Tailwind+shadcn; gallery + fe-visual + axe gates).** Real captures earlier: Ollama tool-use
+ Emby SearchTerm shape.
Remaining follow-ups (non-blocking): (a) ~~live TMDB capture~~ **DONE** 2026-07-13 (key supplied →
`fixtures/tmdb/*`; adapter confirmed correct; live grounding smoke passed); (b) Anthropic LLM
provider (opt-in); (c) Archive.org downloader live HTTP walk (sidecar manual-smoke, stubbed);
(d) carried from Phase 6 — Sonarr `import_webhook.json` fixture (28GB re-download; Sonarr webhook
conn id 3 left up to catch it — remove after). Phase-0 findings:
[`docs/engineering/phase-0-findings.md`](docs/engineering/phase-0-findings.md). Deferred captures:
Sonarr Grab/Download → Phase 6; Emby login success body → Phase 9.

## Live manual-smoke findings — 2026-07-13/14 (maintainer's real stack)

First end-to-end run against the live homelab (Emby 4.10 + Sonarr/Radarr/Seerr over Tailscale
`100.75.125.45`, local Tunarr 1.3.8 with **RTX 3080 Ti `cuda`/NVENC transcode wired + verified**,
Ollama `llama3.1:8b` on GPU). The run drove intent → grounded suggester → approval gate → channel.
It surfaced a **chain of unwired seams** (two independently-correct subsystems, no wire between
them; unit tests pass because each side is tested in isolation). **Composition-root lesson:**
most of these live in `cmd/loomarr/main.go`, which builds the domain objects but never constructs
the adapters that connect them.

**FIXED this session (each with a regression test proven to fail against the old code; `make check` green):**

- **#6** `createChannel` ignored `intentRef` → channels built with an EMPTY lineup. Fixed: `internal/api/channel_lineup.go` (`lineupFromIntent` + approval-gated resolver) + `channels.go`. Tests in `channel_lineup_test.go`.
- **#7** program slots had `DurationMs: 0` → Tunarr rejects (`duration > 0`). Fixed: `schedule.Availability.Resolve` now returns `(itemID, durationMs, available)`; engine adapter fills it from `library.Client.ItemDurationMs` (RunTimeTicks); doc §9 updated. Tests in `schedule/lineup_test.go`.
- **#8** `approveProposal` only enqueued acquisitions → in-library picks never became `available` Records → unschedulable. Fixed: `internal/api/suggestions.go` now creates an `available` Record (with LibraryID) per in-library lineup pick. Test in `suggestions_test.go`.
- **infra #1** nonroot image + root-owned `/data` volume → SQLite `CANTOPEN`. Fixed: `loomarr-init` chown sidecar (sqlite profile) in `docker/compose.yaml` + doc §16.
- **infra #5** Tunarr 1.3.8 requires `channel.transcodeConfigId` = valid UUID (empty → 400). Fixed: `TUNARR_TRANSCODE_CONFIG_ID` now passed through `docker/compose.yaml`.

**OPEN — tracked follow-ups (own doc-first PRs; NOT started). Rooted in `cmd/loomarr/main.go` unless noted:**

- **#9 (SEVERE) — FIXED this session.** *acquisitions never entered a channel's `Lineup`.* `lineupEntries` dropped every non-in-library item, so an acquired title, once it landed `available`, was **never placed — not by event, not by the sweep** (the sweep re-derives desired from `ch.Lineup`, which permanently lacked the acquisition key). FIX (`internal/api/channel_lineup.go`): `lineupEntries(p suggest.Proposal)` now builds an entry for **both** `p.Lineup` **and** `p.Acquisitions` (in-library first, then acquisitions; de-duped by `provision.Key`), dropping the `InLibrary` gate entirely — availability is decided at reconcile time by `resolveEntry` against the live library (§9), not by the proposal's possibly-stale flag. A stale `InLibrary:true` whose media is gone resolves to a pending slot (maintainer decision, matches §9 "revalidate at reconcile time"). So an acquisition enters `ch.Lineup` as a pending slot on create and swaps to a program **in place via the 10-min sweep alone** when it lands (#10's event path is now pure latency, not correctness). Regression test `TestCreateChannelBindsAcquisitionsAsPendingEntries` (`channel_lineup_test.go`) — **proven to fail against the old drop logic** (1 entry, not 2). `make check` green. Complements the #8 approve path (`suggestions.go`): approve enqueues the acquisition as `wanted` AND the channel now holds the pending entry; they rendezvous on the `Key` when the webhook flips it `available`.
- **#10 — FIXED this session.** *provisioner availability events → scheduler feed was `nil`.* Both `reconcile.New(…, nil, …)` and the ingest handler only logged terminal events; `Engine.OnAvailability` had zero non-test callers. FIX: one composition-root `eventEmitter` (`cmd/loomarr/main.go`) implements the emitter port for **both** the reconciler (existing `reconcile.Emitter`) and the ingest handler (new local `ingest.Emitter` — accept-interfaces idiom, structural typing, no cross-pkg import; maintainer decision). It fans each `DomainEvent` to `engine.OnAvailability` (backfill the referencing channels) AND `eventBus.Publish`. The engine is wired via `setEngine` after construction; the field is an `atomic.Pointer[channels.Engine]` since the reconciler goroutine (started earlier) reads it on the hot `Emit` path while setup writes it once — `-race` clean. A nil engine (pre-wire / scheduler unconfigured) still reaches the bus; never load-bearing (sweep is the backstop). Regression tests: `ingest.TestImportEmitsAvailabilityEvent` + `reconcile.TestReconcilerEmitsTerminalEvents` (both emit sources: webhook confirm→available, missed-webhook→available, give-up→unavailable; non-terminal ticks emit nothing). Now that #9 carries acquisition keys into `ch.Lineup`, this makes backfill event-driven (sub-sweep latency) rather than sweep-only.
- **#11 — FIXED this session.** *`GET /v1/events` SSE never emitted.* Zero `.Publish(` calls existed; subscribers waited forever. FIX: the same `eventEmitter` publishes a `title` frame (`{key,state,name}`) to the bus on every terminal transition, so `/v1/events` delivers state changes. Regression test `cmd/loomarr.TestEventEmitterPublishesToBus` (domain event → `title` frame reaches a subscriber; also asserts the nil-engine path is safe). Latency-only (`GET /v1/suggestions/{id}` etc. stay source of truth). App boot-smoked: `/healthz` 200, `/readyz` 503 (no store), clean shutdown with the new wiring.
- **#12 (was the smoke blocker — now DIAGNOSED + a channel proven to play)** — *Tunarr manual-programming content-id contract.* Two parts:
  1. **Setup (was the actual blocker):** Tunarr's Emby libraries were **not enabled/scanned** — so Tunarr's program table was empty and *any* content add (UI or API) FK-failed. Fix is operator setup: `PUT /api/media-sources/{id}/libraries/{libId} {enabled:true}` then `POST …/scan`. Enabling Movies+TV indexed **2205 movies**. Belongs in the Phase-14 ops runbook + `docker/tunarr-dev-setup.md`.
  2. **Adapter fix (real, still OPEN):** `internal/programmer/lineup.go:91` sends `{type:"content", id: LibraryItemID}` with the raw **Emby** id. Tunarr's programming `id` must be **Tunarr's own program `uuid`**, obtained by matching our pick against Tunarr's indexed catalog **by TMDB id** (`/api/media-libraries/{lib}/programs` → each program carries `identifiers[{type:tmdb},{type:emby}]` + its `uuid`; or `POST /api/programming/batch/lookup {externalIds:["emby|<id>"]}` once indexed). FIX: resolve slot → Tunarr uuid before the programming push (a Programmer-side lookup keyed on tmdb).
  **FIXED IN CODE (not around it):** the programmer adapter now resolves media-server item id → Tunarr program uuid via a cached index of Tunarr's persisted `/programs` (doc §6; `internal/programmer/resolve.go`; unindexed item → flex, never dead air). **PROVEN through Loomarr end-to-end:** a movie channel and a **series** channel both built by Loomarr's own reconcile (no hand-rolled scripts) and streamed 1920×1080 H.264/AAC with the RTX 3080 Ti transcoding (NVENC 74% / NVDEC 92%). The manual-smoke half of the DoD is met.
- **#13 (NEW, FIXED) — series support in the scheduler.** A series lineup pick is one show id with no runtime and no single Tunarr program; the scheduler now **expands a series entry into one program slot per episode** (`internal/library/episodes.go` `ListEpisodes`; `schedule.Availability.ResolveEpisodes`; `ComputeDesired` expands then orders by strategy — sequential = episode order, shuffle = shuffled). Doc §9 updated. **PROVEN:** the "90s Sitcoms" channel expanded 5 approved series → **720 episode programs (297h)**, shuffled, resolved, pushed, streaming — all through Loomarr's reconcile.
- **METHOD NOTE (maintainer feedback):** earlier in the session I sidestepped #12/#13 with Python scripts that called Tunarr directly and called the channel "working" — that tests Tunarr, not Loomarr, and hides the bug. Corrected: fix the app, drive the app. Both fixes above are proven through Loomarr's own endpoints. (Memory: `loomarr-test-the-app-not-around-it`.)
- **lesser** — #3 split-host: no *advertised* Tunarr URL distinct from `TUNARR_URL` (Emby must fetch the m3u/xmltv from a media-server-routable host; §15 gap). #4: `llama3.1:8b` curates weakly (themeFit 0 on a real intent) — bigger local model or the deferred Anthropic provider. `deleteChannel ?purge=true` accepted but unimplemented (only detaches; `internal/api/channels.go`). Reconcile create-conflict on channel *number* isn't adopted (re-create attempt collides) — hardening.

An audit agent traced both ends of every design-doc "X feeds/drives/on-event Y" claim and **confirmed correctly-wired**: filler catalog-sync → pod assembly; webhook → confirm-via-library → `available`; guide-poke after channel-affecting reconcile; slot drift-revalidation on the sweep; the three fixes above.

## Phase table

| Phase | Status | Gate evidence (commit SHA + proving command) | Notes / deviations |
| --- | --- | --- | --- |
| 0 · Contract spikes | evidence pinned | Fixtures in `internal/testkit/fixtures/*` + `api/vendor/tunarr-openapi.json`; index: `docs/engineering/phase-0-findings.md` | Tunarr 1.3.8 (CRUD ✓, key optional), Radarr 6.2.1 full lifecycle ✓, Sonarr 4.0.19 Test ✓, Emby 4.10 authed ✓, Seerr 3.2.0 requester ✓. **No §6/§9 deviations.** Deferred: Sonarr Grab/Download (P6), Emby login-success body (P9). |
| 1 · Scaffold + harness | done | `make check` green (vet + golangci-lint v2.12.2 0 issues + `go test -race`); `docker build` → 8.31MB distroless image serving `/healthz` 200 as nonroot | Module `github.com/mantonx/loomarr` (go 1.26), MIT. `config`+`httpx`+`api`+`testkit`+`cmd/loomarr`; `/healthz`+`/readyz`+graceful shutdown. Makefile contract (unimpl targets fail loudly). Dockerfile (distroless static, cgo-free), `docker/compose.yaml` (sqlite/postgres/ai/filler). `.env.example` covers all §15. golangci v2 config mirrors nexus-open. |
| 2 · Provisioner domain + state machine | done | `make check` green; `go test ./internal/provision/` — Key derivation, **webhook-key parity vs real Radarr fixture**, happy path, + all 5 §4 invariants (terminal monotonicity, emit-only-terminal, idempotent no-op, library-is-truth, deadline discipline) | Pure domain, no I/O. `Title`/`Key`/`State`/`Record` (§3); `Apply(rec, ev, now) → (Record, []DomainEvent)` (§4) — clock passed in for determinism. Illegal transitions are no-ops, not errors. |
| 3 · Store + SQLite | done | `make check` green (`-race`); `go test ./internal/store/` — conformance suite (round-trip, upsert-idempotent, not-found, list, **ClaimDue + concurrent claim**, settings) + downgrade guard + unknown-scheme; app boots on real SQLite, migrates, `/readyz` ready | `Store` iface (§5); shared `database/sql` impl (one path, `?`↔`$N` rebinding); `modernc.org/sqlite` WAL+busy_timeout, single-conn; goose embedded per-dialect migrations + **startup downgrade guard**; **`ClaimDueTitles` leases rows (deadline→now+lease)** so concurrent callers/replicas never double-claim — SQLite guarded UPDATE, PG `FOR UPDATE SKIP LOCKED`. Conformance is one suite, backend-agnostic (Phase 4 reuses it). |
| 4 · Postgres backend | done | `make test-pg` green (testcontainers `postgres:16-alpine`, 3.3s) — **same conformance suite**, incl. `ClaimDueConcurrent` under real `FOR UPDATE SKIP LOCKED` (passed 5× under `-race`); app boots + migrates on dev-compose Postgres, `/readyz` ready | `pgx` stdlib shim, `$N` placeholder rebinding, PG per-dialect migrations. Postgres test behind `//go:build integration` so default `make check` needs no Docker. Concurrent claim is the meaningful case here (SQLite serializes; PG genuinely races). |
| 5 · Library adapter | done | `make check` green; `go test ./internal/library/` — Lookup present/absent (pinned fixtures), **both-flavor token headers** (Emby `X-Emby-Token` vs Jellyfin `MediaBrowser`), **both-flavor login headers**, auth success + **§11 bad-pw 401 negative path**, ListUsers, SeasonPrecision | Shared Emby/Jellyfin `Client` (one impl, flavor differs only in auth headers — auth.go); `Lookup`/`AuthenticateByName`/`ListUsers` (§6, §11) via `httpx` 10s timeout; `SEASON_PRECISION` series(default)/seasons policy. Testkit `MediaServer` mock serves pinned fixtures + captures headers (both flavors, CLAUDE.md). Login-success body synthesized (real capture deferred to P9). |
| 6 · Requester + ingest | done | `make check` green; `go test ./internal/{ingest,requester}/` — Seerr 201/OK/409-success + 500-fails + no-TMDB; `/hooks/arr` bad-secret 401, Test→200+timestamp, Grab→downloading(+deadline reset), **Import→available ONLY after library confirms (inv. 4)**, untracked-ignored, malformed 400; app wires `/hooks/arr` end-to-end | Seerr requester (`X-Api-Key`, 201-with-existing-media path per P0); `/hooks/arr` handler maps Grab/`Download`(quirk)/Test via Phase-2 keys + Phase-5 Lookup; constant-time secret. **Sonarr Grab+Test captured live** (`sonarr/{grab,test}_webhook.json`); import fixture pending a 28GB re-download (webhook conn left up to catch it) — import *logic* already tested via Radarr's real import fixture. |
| 7 · Reconciler + janitor | done | `make check` green (`-race`) + `make test-pg` re-green (claim SQL changed); `go test ./internal/reconcile/` — retry-wanted (success/fail), **missed-webhook re-check→available**, **deadline give-up→unavailable+Cancel**, not-due/terminal untouched, janitor runs-all/failure-nonfatal; app starts+ticks+**clean shutdown** verified | Ticker `Runner` → `Tick` claims due batch → per-title retry(wanted)/give-up(in-flight w/ library re-check). Janitor scaffold + `Sweeper` iface (targets registered by P9/P11). Requester gains `Cancel`. **2 bugs caught+fixed:** ClaimDue excluded `wanted` (fixed both dialects); lease/deadline interplay blocked give-up. Also fixed shutdown ordering (cancel reconciler before drain). |
| 8 · Self-documenting API | done | `make check` green (`-race`) + `make openapi-verify` green (drift guard); `go test ./internal/api/` — OpenAPI 3.1 + State-enum=code, enqueue(admin)/idempotent, mutation-requires-admin 403, list-needs-state 400, get 404, delete→unavailable, backup 200 SQLite-magic + admin-only, docs-offline; app serves all routes end-to-end | Huma v2 on `humago` (§7.1); `/v1/titles*` (idempotent enqueue, admin-gated POST/DELETE), `/openapi.{json,yaml}`, offline `/docs` (no CDN), `GET /v1/backup` (SQLite `VACUUM INTO` stream; PG→501). Auth seam: `API_TOKEN` Bearer authorizer (Phase 9 adds sessions). `make openapi`→committed `api/openapi.yaml` via `cmd/openapi`. Streaming backup is a plain mux handler. `/v1/events` SSE deferred to Phase 11 (event bus). |
| 9 · Users & auth | done | `make check` green (`-race`) + `make test-pg` (users/sessions schema; SQLite-INTEGER↔PG-BOOLEAN) + `openapi-verify` green. **Gate tests:** `go test ./internal/{auth,api}/` — token-hashed-at-rest, resolve, **disabled-user-session-dies**, **disable-revokes-sessions**, bootstrap roles, bad-pw, rate-limit; HTTP: **member 403 on admin routes (§19)**, admin allowed, **session-dies-on-disable end-to-end (§19)**, CSRF-required, HttpOnly+SameSite=Strict cookie, logout, break-glass token, user-sync admin/member-403 | Sessions (256-bit token, SHA-256 at rest, revocable rows); `/v1/auth/{login,logout,me}` + `/v1/users{,/{id},/sync}`; first-admin bootstrap; login rate-limit (x/time/rate); session janitor sweeper. Fills the Phase-8 `Authorizer` seam (session cookie + API_TOKEN Bearer break-glass). **Real Emby login-success body captured** (`emby/auth_success_response.json`, scrubbed) — validated the mock shape. |
| 10 · Scheduler + Tunarr | done | `make check` green (`-race`, lint 0) + `make test-pg` (channel round-trip/list/delete + **ClaimDueChannels concurrent** under real `FOR UPDATE SKIP LOCKED`) + `openapi-verify` deterministic; app boots with the channel scheduler + sweep, `/readyz` ok, clean shutdown. **Gate tests:** `go test ./internal/{schedule,programmer,channels,setup,api}/` — pure desired-lineup (3 strategies, deterministic shuffle, pending policy); Tunarr adapter server-assigns-id + slot↔lineup + 400-on-empty vs pinned fixtures; **reconcile create→idempotent-no-op, backfill-on-event, event-loss-recovery-via-sweep, drift-substitution, guide-poke-only-when-affecting**; **idempotent livetv-connect second-call-no-op** (unit + over-the-wire); §19 auth negatives (member 403 on create/reconcile/connect/status) | `internal/{schedule,programmer,channels,setup}` + `store` channels (migration `00003`, `ClaimDueChannels` per-channel lease §18) + `library` Live TV wiring + `/v1/channels*` + `/v1/setup/{status,livetv-connect}`. Server-assigns-channel-id honored. **Fixed a pre-existing exporter spec-drift gap** (auth/users routes were never in the committed spec). Added `TUNARR_TRANSCODE_CONFIG_ID` to §15 doc-first. **Live capture DONE** (maintainer-supervised, reversible register→capture→delete vs real Emby 4.10.0.17; Emby verified reverted): `internal/testkit/fixtures/livetv/*` + `FINDINGS.md`. Pinned truth: m3u tuner `{Type,Url}`✓, xmltv provider uses **`Path`**✓, delete via `?Id=`, and **guide-refresh resolves the per-install task Id by the stable Key "RefreshGuide"** (`/ScheduledTasks/Running/<id>`; Key form 404s) — adapter corrected + tested (`library/livetv_test.go`). M3U registration is fetch-validating (unreachable URL → 500), so the real connect is manual-smoke (§21). Fixtures scrubbed of IPs/zip/device-id. |
| 11 · Suggester | done | `make check` green (`-race`, lint 0) + `make test-pg` (jobs/proposals schema; **ClaimDueJobs concurrent** under real `FOR UPDATE SKIP LOCKED` + intent-hash cache) + `openapi-verify` deterministic; app boots with the suggester + worker pool (Ollama provider), `/readyz` ok, clean shutdown. **Gate tests:** `go test ./internal/{catalog,tmdb,llm,suggest,api}/` — **GROUNDING: fabricated-title-never-reaches-proposal**, in-library→lineup vs acquisition classification, acquisition re-validated vs TMDB (404 drops), cap→alternates, deterministic scoring; worker: cache-dedup, job-runs+proposal-persists, **hung-LLM hits JOB_TIMEOUT while pool keeps draining**; **APPROVAL GATE: member approve→403 with ZERO titles enqueued, admin approve→acquisitions become wanted titles (the only proposal→/v1/titles path)**; search any-auth + q-required; events auth-required | `internal/{catalog,tmdb,llm,suggest,events}` + `store` jobs/proposals (migration `00004`, `ClaimDueJobs` lease §18, intent-hash cache) + `library.Search` + `/v1/suggestions*` + `/v1/search` + `/v1/events` SSE. **Real captures:** Ollama tool-use round-trip (`fixtures/llm/*`, arguments-as-object/format:json pinned) + Emby `SearchTerm` shape (`emby/search_matrix.json`). Grounding chokepoint: LLM proposes ONLY via catalog_search; a pick survives only if the tool surfaced its id. Scoring theme-first (0.5/0.35/0.15, maintainer). **TMDB captured + live-verified 2026-07-13** (`fixtures/tmdb/*` + FINDINGS; adapter was already correct — /search/multi + exists 200/404 confirmed; `tmdb/fixture_test` parses the real shape). **Live grounding smoke passed:** federated `/v1/search?q=matrix` against real Emby + real TMDB + Ollama — The Matrix trilogy `in_library=true` (from Emby), TMDB-only titles `in_library=false`, deduped-by-id once. **Deferred (non-blocking):** Anthropic provider (opt-in). |
| 12 · Commercials & filler | done | `make check` green (`-race`, lint 0) + `make test-pg` (clips schema + filter/tags/prune conformance) + `openapi-verify` deterministic; app boots with the pod assembler wired into the scheduler + periodic filler sync (sync 401 non-fatal → degrades to flex). **Gate tests:** `go test ./internal/{filler,channels,ingestkit,library,store,api}/` — **POD-MATCHING: seeded-deterministic, era/audience match, category-variety (no back-to-back), no-repeat-in-window, PodMax density, fallback ladder (exact→widen→audience→embedded bumper card, never dead air)**; **FILLER-NEVER-A-PROGRAM** (structural + explicit test); catalog sync **tag-preservation-on-resync** + idempotent + prune; **AI tagging grounding** (hallucinated enum/year dropped, never persisted); scheduler pod-fill (gaps→matched pods, programs untouched, deterministic seed, no-pods=flex); filler API admin negatives (member 403 on patch/sync/tag); sidecar dispatch/resilience | `internal/filler` (Clip + pure Assemble + sync + Classify/Tagger + PodAdapter) + `store` clips (migration `00005`) + `library.ListFillerClips` (duration from server RunTimeTicks; §10 core-never-probes) + `/v1/filler*` + `cmd/loomarr-ingest` sidecar (Go, `Dockerfile.ingest` bundling yt-dlp+ffmpeg, `filler` compose profile — core has no download config). Filler is a parallel universe to provisioning (clip identity = media-server item id). |
| 12.5 · End-to-end integration (the seams) | **done** — `go test ./internal/integration/` (`TestPipeline_ApproveToChannelWithProgramsAndPodBreaks`) + live manual smoke | Gate: an integration test (intent→suggest→approve→create→reconcile→ pushed Tunarr lineup has real programs **with pod breaks**) + the live manual smoke. **Both met:** the integration test wires the REAL domain objects (store, channel engine, real store-availability, real filler pod assembler, real approval path) and drives approve→create(intentRef)→reconcile through the real HTTP API (only Tunarr faked via the testkit double) — asserting the pushed lineup has ≥3 real program slots, flex break gaps, AND a grounded filler-list attached (commercials), plus second-reconcile idempotency (0 pushes, 0 filler writes). **Proven to FAIL when a seam regresses** (disabling the filler-list attach → "no filler-list attached"; disabling the lineup push → "no lineup pushed"). Runs under `make check` (testkit only, no network, §19). | **Why this phase exists:** the 2026-07-13/14 live smoke proved phases 0–12 were gate-green in isolation but had unwired seams (per-phase unit gates never test the composition). **DONE en route (live-smoke commits `dc14f40`, `ac79a80`):** #6 create-binds-lineup, #7 duration, #8 in-library→available, #12 Tunarr content-id resolution, #13 series→episode expansion — a movie channel AND a 297h series channel built by Loomarr's own reconcile, streaming GPU-transcoded 1080p. **#9/#10/#11 + commercials CLOSED this session** (#9 acquisitions now enter `ch.Lineup` as pending entries + sweep places them; #10 one composition-root `eventEmitter` fans terminal events to `engine.OnAvailability` from both the reconciler and the webhook ingest handler — backfill now event-driven; #11 same emitter publishes `title` frames so `/v1/events` SSE delivers. **Commercials — §10 filler redesign implemented:** filler is now Loomarr-owned via a Tunarr `local` media source + per-channel filler-lists (media server out of the filler path). Clip identity media-server-item-id → **Tunarr program uuid** (migration `00006`, forward-only drop+recreate empty); sync reads Tunarr's local-source `/programs` (was Emby); `PodFiller.FillGap`→`BuildFillerList` (per-channel pool, not per-gap); `reconcile.fillPods`→`attachFillerList` (break gaps stay **flex**, Tunarr fills from the list); `Programmer.EnsureFillerList` builds+attaches the list, **content-based idempotent** (compares the actual program set, not count — a review caught a count-only bug that would freeze commercials on a re-tag). `FILLER_LIBRARY`→`FILLER_DIR` (§15 doc-first). New `programmer/filler.go` + tests; `make check`/`make test-pg`/`openapi-verify` green.) **Emby ~4s Live-TV playback stop RESOLVED 2026-07-14:** root cause was a **Firefox** client-side playback quirk (NOT Loomarr/Tunarr/Emby backend, NOT the earlier-suspected Simkl plugin) — the backend stream is healthy; it plays fine in another client. No code change; captured as a troubleshooting entry (`docs-livetv-integration.md` → folds into the Troubleshooting page in Phase 14). **Phase 12.5 COMPLETE — all seams closed; Phase 13 (UI) unblocked.** See design §21 phase 12.5 + memory `loomarr-filler-redesign`/`loomarr-wiring-backlog`. |
| 13 · Web UI + onboarding | done | 13.4e gate `887f833` (#35). `make check` + `openapi-verify` GREEN; `make fe` GREEN (217 app / 24 core / 9 api); `make fe-visual` GREEN (188, zero flaky); `make e2e` GREEN (5). **Gate tests:** `reachability.test.tsx` (every generated route renders real content, every feature-gated panel appears when its flag is on — DERIVED from `router.routesById`, verified to fail when a panel's mount is removed); the **approve-flow e2e** as §7 under test (admin approve enqueues; member is offered no approval and nothing is enqueued — §19 negative, asserted on the mock's recorded state, verified to fail when the `isAdmin` gate is deleted). | Vite React+TS + orval hooks + TanStack Router/Form, embedded SPA (`internal/web`). Built 13.1→13.4e: design-system + token pipeline (#5, #7) → Storybook gallery + visual/a11y harness (#8) → auth foundation + wizard (bootstrap, checklist, steps 3–7) (#11, #12, #16, #17) → the 8 surfaces: **Suggest** + approval queue (#18), **Channels**/detail/**Board** (#19, #20), **Settings** (6 pages, one save bar each, §8.1 model picker, secrets panel) (#21, #23), **Users** (#32), **Filler** (#33), **Help** + ⌘K palette (#34), per-user quotas (#36). **Recurring lesson (13.4d/e):** seven things were built, unit-tested, and mounted by *nothing* — the reachability gate is what this phase earned. **Deviations, doc-first:** ProposalDTO + submit body typed from the domain (orval 1:1, no hand-mirror — #9, #18); react-hook-form → TanStack Form (§14, #15); Settings IA = 6 pages per config-design §5 (build plan said 5). Visual seed (`design/*.dc.html` + `loomarr-frontend-design.md`) recreated with the two §7 deltas (badge `-300` stops; `static-500` disabled-only). |
| 14 · Docs, harden & ship | done | `make check` + `openapi-verify` GREEN at `0a4a7f2`. **§21 DoD, both halves met:** automated (`make check`/`make e2e`/`make smoke` — compose sqlite+postgres start clean, member→template→grounded proposal→admin-approve→reconcile creates the channel with pods+filler, Import backfills, Help/`/docs`/OpenAPI 3.1 all resolve) + **manual smoke on the maintainer's real stack** (`5cdf6a9`, `60746c1`: wizard all-green → intent → approve → an 80-program kids channel PLAYING in Tunarr with ad pods, real Emby+TMDB+Ollama; `make smoke-livetv` wires+destroys a disposable Jellyfin). | `docs/help/` set (Quickstart, Integrations, Concepts, Member, Filler, Troubleshooting) embedded + served at `/v1/docs` (`a50c57b`, deep-links `eb09813`); seed docs folded into `docs/` (`62d9369`); README Documentation + Operations sections (`f46ddf9`). `internal/metrics` + `/metrics` (unauth, §7): **§17 closed end to end** — RED (`040a127`) → state gauges + login/webhook counters (`5fb073e`) → outbound-client latency + reconcile timing (`07d2359`) → LLM tokens + filler-ladder depth + slot drift (`299b0aa`). CI + release pipeline + community health (`beaa741`); LICENSE + third-party notices + UI-less image fix (`1a10cac`). **9 bugs the maintainer smoke found** (seams between gate-green subsystems) fixed en route (`8a732b5`). **Accepted deltas vs §21 checklist:** runbook lives as the README **Operations** section (no dedicated file); **no shipped dashboard JSON** — cost is deliberately a dashboard recording-rule over the token series, not a baked-in metric (per the metrics entry above). |

## v2 program (post-14) — the mock delta

Phases 0–14 shipped the app. The **v2 program** reshapes it against the v2 mocks and one large
architectural change. Plan: **`docs/engineering/v2-build-plan.md`** (39 phases, one PR each, gates
per phase). Evidence: `docs/engineering/v2-mock-delta-2026-07-24.md`.

**The headline decision (D-I):** Loomarr stops being *"not a transcoder/streamer"* (`design.md:39`)
and plays out its own channels. That reframes Track T from a parallel track into the spine —
**V2b alone blocks 28 of the 39 phases**.

| Phase | Status | Gate evidence (commit + proving command) | Notes |
| --- | --- | --- | --- |
| — · Delta research + build plan | done | `a4f183d` (#68) — 4 CI jobs green | Both v2 mocks read in full (markup **and** the 190KB state block; an MCP fetch had silently truncated 48% of the desktop file). Mocks were generated **from this repo**, so much apparent delta is the mock reflecting shipped code. Register re-verified at HEAD: **5 entries already fixed**, C3 narrowed, **4 new defects (A1–A4)**. Phase graph verified mechanically: acyclic, no dangling refs, all 27 defects mapped. |
| V2b · D-I design amendment | done | `3b0bca2` (#69) — `make check` green; docs only | §1 playout moved out of Explicit non-goals; **new §9.1** (playout backends, per-channel `playout.backend`); §10 mid-roll **in scope** for internal playout (opt-in detection per channel); §11 **device auth for segments** (a TV holds no session cookie — the one path that isn't a person); §14/§16 ffmpeg core + ffprobe back + **one 549MB image** (18× the old default). §20: two dead bullets struck — **closes S12**. Swept the *consumers* too (10 stale `loomarr:filler` refs in endpoint tables, feature gates, env docs, compose) — amending only the defining section is how §20's dead bullets survived. |
| V0 · `DATABASE_URL` envDefault | done | `60cc6a6` (#70) — `go test ./internal/config/`, verified failing without the fix | `design.md:644` promised the default; the struct tag never carried it, so a bare `docker run` came up **store-less and never-ready**. Survived because compose sets the same value. **Closes S1.** |
| V1 · Mount `ChannelIconField` | done | `5fefd0c` (#71) — reachability assertion, verified failing when unmounted | The component shipped complete (stories, 5 visual baselines, admin gate) and was **imported by nothing** — the eighth instance of the bug `reachability.test.tsx` exists for. Mounted on Overview (the icon is what the family sees in the guide). Also corrected the §12 surface map, which claimed a home (`Settings → Identity`) with **no implementation behind it** — a row asserting a UI that doesn't exist makes the audit look clean. **Closes D-H.** |
| V2 · `C3′` + `C10` + `A4` | done | `ae93332` (#72) — separation-delta test verified failing without the fix | **C3′:** `MergeFromProposal` refreshes `separation` like the other four policy fields, but `policyDeltas` didn't diff it — a refine could widen or drop a no-repeat window with nothing shown before Approve. Duration comparison tidies zero-unit padding (`168h` vs `168h0m0s`) or an unchanged window reads as a diff. **C10:** deleted the dead `channel-pods` component — writing the story the coverage gate asked for would have gone green *and kept the dead code*. **A4:** two `TODO(learning)` docblocks describing unimplemented functions sat above complete implementations (`fillCommercials`, `composite`); none remain in the tree. |
| V17a · Read the clip sidecars (`F1`) | done | `2eaaca4` (#73) — 4 tests, all verified failing with the old argument | Every AI-tagged clip's prompt read **`Source description: tunarr-local`** — `sync.go:131` sets a PROVENANCE enum and `tagjob.go` passed it as the *description*, so the classifier got a misleading constant while the real title/description sat unread. Both ingest paths were already writing info-JSON sidecars with a comment saying *"AI tagging reads, §10"*; nothing parsed them. New `internal/filler/sidecar.go`. **`upload_date` deliberately NOT used for Era** — it's when a clip was uploaded, not when it aired. Testkit gained `LLM.LastMessages`/`Prompt()`: the defect lived in the PROMPT, which no output assertion could catch. |
| V23 · Deny-reason UI (`A1`) | **done** | `#74` **merged** — 4 tests; visual verified against a REBUILT storybook-static (status corrected 2026-07-26; the row still read "in review" long after the merge) | `denyInput.Body.Reason` has always existed, persisted, and been **rendered** by `ApprovalQueueItem` with a story and a test — but all three call sites sent `data: {}`, so it was always empty and a denied member learned nothing. Deny is now two-step (arm → reason → send); **Approve stays one click** — approving needs no explanation, declining is where someone is left guessing. Reason is optional; empty sends `undefined`, not `""`. |

| V3 · One 549MB image (`D-E2′`) | **done** | `bf7164a` (#82) — image builds; `USER nonroot:nonroot` at `Dockerfile:205`; `ffmpeg`/`ffprobe` both present | ⚠ **Row added retroactively 2026-07-26** — shipped without one. The filler variant became the runtime: one image instead of a slim core plus a filler build, ~18× the old default. §16 amended in-PR per the phase gate. Verified at HEAD rather than trusted: the Dockerfile carries both binaries and the nonroot user the gate names. |
| V4 · `playout.*` + `backup.*` registry groups (`D-G`) | **done** | `4662f4d` (#83) — `make config-docs` diff **empty** (re-verified at HEAD 2026-07-26) | ⚠ **Row added retroactively 2026-07-26.** §15 amended before `declared.go`, per the doc-first rule. Includes the **per-channel `playout.backend` override** the gate calls for — which is what made the audit's tier-2 door (merged today, #88) a pure frontend change with no spec edit. `playout_token` redaction lives in `api/playout.go`. |
| V5 · Bootstrap-file config tier (`#6`/`D-B`) | **done** | `ffe1b7c` (#84) — precedence tests across all three tiers (`internal/config/bootstrapfile.go`) | ⚠ **Row added retroactively 2026-07-26.** `env > file > default`. `config-design.md:72` amended in-PR. |
|  |  |  | **Why three rows were missing:** V3/V4/V5 are spine dependencies of V6, so V6's green row implied them — but implication is not evidence, and this file's own header says an unrecorded finding is a lost one. Found by cross-checking all 39 planned phases against `PROGRESS.md` and then against the code; each of the three was verified in the tree (Dockerfile, `declared.go`, `bootstrapfile.go`) before its row was written. |
| V6 · Internal playout to first frame | **done** | `2b0b9a4` … `c6aa0e7` — `make check` + `make test-ffmpeg` green; **verified on the maintainer's own Emby** | Loomarr serves its own channels. Emby pulls `/playout/tuner.m3u`, real library films play at 1080p, breaks cut to real commercials, and the full transcode runs on the GPU. Mechanism is Tunarr's HTTP ffconcat loop (prior-art §1): a `-c copy` parent over a 2-line playlist whose entries both resolve to "what's on now" — the demuxer's EOF-and-advance IS the program boundary, so there is no splicing code. Mid-program tune-in verified live (joined 39 min into a film). |
| V6b · XMLTV listings | **done** | `ac17450`, `c8ed906`, `5382fda`, `7fccc63` — `make check` green; live guide 253 programmes / 541ms | `/playout/guide.xml`. Same `CyclePreview` the encoder reads, so listings cannot drift from playout (a test samples 200 instants and asserts they agree). Breaks are **not** advertised (#12). Full metadata: `<desc>` 253/253, `<date>` 253/253, `<category>` 249/253, `<rating>` 247/253 — one bulk `/Items?Ids=` call, 120 items in 24ms, so no cache was needed. |

| V13b · `GET /v1/guide` | **done** | `eeb6338`, `ec8436c` — `make check` + `openapi-verify` green; live 38 blocks with provenance + runtime on all | The JSON time-grid backend. `kind` replaces `gap bool` (a boolean could not tell a commercial pod from a pending acquisition); gaps preserved, unlike `Upcoming`. All 8 gaps in the mock-delta §2f closed — incl. per-airing pod composition, episode runtime, server-assembled provenance, `guide.timezone` + `guide.retention_hours` (§15). |
| V14a · Guide time-grid UI | **done** | `431fd32`, `038b17f` — `make fe` green (460 tests); rail/chips verified by screenshot | The grid itself, built to `-v2.dc.html`. Flex rail + percentage blocks, so ruler/block misalignment is structurally impossible. GuideDetailCard, per-clip pod rendering, airing highlight, channel marks, health chips, row menu, day/window controls. **The IA rename (`/channels`→`/guide`, `/users`→`/people`, two navs) is deliberately NOT here** — see below. |

| V14b · IA rename + two navs | **done** | `a6ac496` — CI green; 461 unit + 342 visual + 7 e2e | `/board`→`/queue`, `/users`→`/people` (git mv). **Two AUTHORED navs** replace `NAV.filter(i => !i.admin \|\| isAdmin)`: a member gets `Guide · Request a channel · My requests · Help` — the same `/suggest` and `/queue` routes under member-facing names, which a filter structurally cannot do (it hides, never renames). `NavItem.admin` deleted. `/v1/users/*` unchanged — this is a frontend rename, not an API one. |

| V7 · local accounts (backend) | **done** | `POST /v1/auth/password`; 6 password tests in `auth_flow_test.go` | Closes **S3**. Changing a password revokes **every** session including the caller's — keeping one means trusting a credential that may be the compromised one. |
| V7b · Account screen | **done** | `routes/_authed/account.tsx`; 3 reachability assertions | Closes **S2 for real**. The screen V7 shipped without: change password, session list, revoke. The copy says all sessions end, matching what the code does. |
| V7c · People: create local + reset | **done** | `people/create-local-panel/` | The admin half of V7's surface. |
| V19 · per-title refine rationale | **done** | `refine-review.tsx` — `rationale` on `DiffRow` | No backend work needed, as the plan predicted: the LLM already populated `ProposalItem.Rationale` and `diffLineup` was dropping it one function before render. Shown on ADDED rows only. |
| V24 · `A3` surface hidden proposal data | **done** | `proposal-review.tsx`, `intent-form.tsx` | `eraBalance`/`overall` render; `mustInclude`/`mustExclude` are wired into the intent form. |

⚠ **Those five were finished but never recorded here**, which sent a later session hunting through
four completed phases before noticing. A phase that ships without a PROGRESS row is invisible to the
next person — including to a future me reading "next up" as authoritative.

| V25 · edit-before-approve (backend) | **done** | `44f02c7` — CI green; 13 approve tests (6 new) + `TestOnlyApproveCreatesWantedTitles` | The edit is a PARAMETER to `suggest.Approve`, not applied by the handler first — otherwise "what gets acquired" is decided outside the gate and auto-approve runs different logic. `mod_summary`/`note` are real columns (migration 00015); the summary is generated server-side, because one the approver types is a claim and one the code writes is a record. **Two backward-compat breaks caught by gates:** a value Body made huma require one (400s on empty), and the pointer fix did not cover orval, which still emits `data` as required (4 FE call sites). Runtime and client compatibility are different things. |

| Surface + drift audit | **findings recorded** | `docs/engineering/surface-audit-2026-07-26.md` | Two audits run as parallel subagents. Fixed in-session: a **live defect** (the `fillerIngest` SSE frame was never fanned out, so "Download clips" hung forever) plus 4 orphaned channel controls and 12 surface-map rows. **Still open:** the tier-3 orphans and 8 §12 drifted claims — all itemised in that file. (Tier 2 and the `:714`/`:711`/`:752` drift are closed; see the row below.) |

| Audit tier-2 doors | **done** | `make check` + biome/codegen/typecheck/tests (569) + SPA + storybook green; 424 visual specs, 14 baselines updated | The three orphaned channel capabilities, all reachable now. **Frontend-only** — all three were already on the wire and in the generated client, so no `openapi.yaml` change and no orval conflict. `autoCurate` → Programming → When it changes; `audience.unrated` → beside the ceiling; `playout.backend` → Overview → Advanced → Broadcast. §12 corrected doc-first: the **Settings tab it prescribed as auto-curate's home never existed** (`SECTION_IDS = info/programming/filler/danger`), so the doc was fixed rather than a fifth tab invented. Three structural notes in the audit file worth re-reading before tier 3: the opt-in **is** the object's presence (nothing generic can construct it); the opt-out only clears because `MergeFromOperator` is `out := incoming`; and `0` is the *inherit* sentinel on both overrides, so blank must send 0, never `undefined`. **A defect the unit tests could not catch:** the opt-in's hint read "Off, new titles wait for your approval" while the box was ticked — state and payload assertions all passed, and the **story screenshot** is what caught it. |

| Audit tier-3 (2 of 4) | **done** | `make check` green (37 pkgs) + biome/codegen/typecheck/**569 tests**/SPA/storybook + **428 visual specs**, 12 new baselines | `scope.series` → Programming → What plays; `policy.seasonal.*` → Programming → When it changes. Frontend-only. **The tier's cost estimate was wrong in the direction the audit file's own rule predicts** — both were *reuse*, not design: the series picker is the lineup editor's `keyOf` + shared `SearchCommand`, and the holiday "blocker" did not exist (`builtinCalendar` is a FIXED five-entry list whose ids already reach the FE through the rule vocabulary; TMDB keyword IDs were for *matching titles to a holiday*, not *choosing which holidays a channel observes* — the audit conflated the two). Left open: `scope.collections`, whose blocker is a **missing endpoint** (no way to list media-server collections), not the control; plus `policy.window` and the draft preview. Also surfaced drift claim `:757` in practice — `SearchCommand` renders a plain `<input aria-label="Search">`, no combobox/listbox roles, so a `getByRole("combobox")` test query fails against real markup. |

| V25b · Edit-before-approve **UI** | **done** | `make check` green (37 pkgs) + biome/codegen/typecheck + **621 app tests** (15 new) + **39 core** + **440 visual specs**, 4 new baselines | The queue's "Show picks" disclosure becomes **"Review & edit picks"** when an edit handler is supplied — drop per title, add via search, note to the requester — and the delta rides the **same approve call**, never a separate save (the edit is a *parameter* to `suggest.Approve`, so "what gets acquired" stays inside the one gate). ⚠ **`undefined` vs `{}` is load-bearing:** the handler maps a body with no drops/adds/note to a **nil** edit so an untouched approval stays byte-identical to pre-V25 behaviour; emitting an empty object would record *"approved with modifications: none"* — a different and false claim. Pinned by a queue test asserting the unmodified body is `{}`. Drops carry the **provisioning key**, never an index or name — a key that disagrees with Go's by one character does not error, it matches nothing and the title the admin removed is acquired anyway — so `provisionKey` was extracted into `@loomarr/core` with a Go-parity test rather than written a **fifth** time. ⚠ **The plan's premise did not hold:** it said `ProposalReview.onEditItem` "already exists with no production caller, which is most of the surface". That is a per-row *pencil/swap* affordance, not V25b's drop/add/note delta — near-zero reuse. Three near-copies of the key derivation remain, each differing deliberately; documented in `provision.ts`. |

| V26 · "My requests" (`A2`) | **done** | `make check` (37 pkgs) + `make test-pg` + `openapi-verify` + biome/codegen/typecheck + **635 app / 39 core** tests (17 new) + **448 visual specs**, 8 new baselines, 0 modified | Closes the defect where **a member could submit a request and then see nothing**: `queue.tsx` fanned `GET /v1/titles` across states and never queried `/v1/suggestions` at all. Adds the missing first tier above the existing title table. **Backend was half-built:** `store.ListProposalsByCreator` already existed *and was covered by the conformance suite* with no API caller — the "built, tested, wired to nothing" pattern one layer below the UI. Two API changes: `?mine=true` and the approval provenance. ⚠ **`mine` is a session-resolved BOOL, not the `ListProposals(status[, user])` the §7 sketch implied** — a client-supplied id is a client-supplied identity, so any member could read another's requests by editing a URL. Doc-first amended; the property tested is that scoping can only ever *narrow*. A break-glass token (no user record, id `""`) gets its own submissions, never everyone's — pinned by a test that fails with `[p-kid p-boss]` when the scoping is removed. ⚠ **`modSummary`/`note` were persisted by V25 and never left the server** — the note V25b captured yesterday reached the database with nothing able to display it, the same shape `denyReason` was in before V23. Now on `ProposalDTO`. The card distinguishes **approved-with-changes** from plain approved (otherwise a lineup silently differs from what was asked for), renders *"CHANGED BY …"* + the server-generated summary, and shows the denial line — or says "No reason was given." rather than an empty line that reads like a bug. **Scoping asymmetry preserved (§12):** proposals carry `created_by` so this tier is genuinely per-member; titles do not, so the table below stays global exactly as #87 documented. |

| V27 · Approvals as Queue's own surface | **done** | `make check` (37 pkgs) + `make test-pg` + `openapi-verify` + biome/codegen/typecheck + **652 app / 39 core** (20 new) + **448 visual**, 12 new baselines, 0 modified | ⚠ **The surface is QUEUE, not a new route — the maintainer caught me reading §12 instead of the mock.** `design/loomarr-prototype-desktop-v2.dc.html` (`navDefs`) shows the v2 admin nav as `Dashboard · Guide · Queue · Filler · People · Settings · Help` — **Suggest and Channels are both gone** — and hangs a `pendingCount` badge off **Queue**, whose `queueTabs` are exactly `Needs approval · In flight · History`. §12's nav table is the *shipped* nav and predates that; recorded as drift with the three consequences (Dashboard = V16, Channels→Guide deferred, and **Suggest's removal needs a home for admin origination first** — undecided, so the entry stays). **Migration `00016` (`approved_at`)** — the gate's third clause needed a schema change: `updated_at` looks equivalent and is not, because THREE callers write it (`suggest.Approve`, deny, **and `recurate`**), so a re-curation would silently rewrite an audit row's approval time. 0 = never approved, and pre-existing rows read as "no recorded time" rather than back-filling a record that was never taken. Stamped at the one chokepoint, so every approve path (human, per-user grant, auto-curate, bulk) records a time. **Bulk approve calls `s.approveProposal` per id** — the same handler, not a copy — so the phase gate's "same chokepoint" is structural rather than asserted; per-id results mean one already-handled id can't hide the rest. ⚠ **A row with a pending EDIT is excluded from bulk** (no checkbox at all, not a disabled one): the bulk endpoint takes no edit field by design, so bulk-approving an edited row would discard the admin's edit and acquire the titles they just removed. **A test-design flaw worth remembering:** the first "bulk went through the gate" assertion counted wanted titles, but `seedProposal` gives every proposal the SAME acquisition and enqueue is idempotent by key — so 2 approvals produced 1 title and the count couldn't distinguish "both approved" from "one did". Fixed with distinct titles per proposal. |

| V16 · Dashboard + transcode telemetry (`D-A`) | **done** | `make check` (37 pkgs) + `make test-pg` + `openapi-verify` + biome/codegen/typecheck + **666 app / 39 core** (15 new) + **474 visual**, 14 new baselines + 2 AppShell re-shot for the nav entry; two clean PLAIN verification runs | ⚠ **The telemetry was already being produced and discarded.** `playout.Start` has parsed ffmpeg's structured progress on a dedicated fd since the supervisor was written — speed, frame, `out_time_ms` (microseconds despite the name, as its own comment records) — and **every caller passed `nil`** for the callback. So the mock's "12× rt / encoded 96s ahead" needed no new subsystem, just wiring. ⚠ **Telemetry reports from the per-program CHILD, not the session's own ffmpeg**, which is the `-c copy` parent and never encodes: its speed would measure remuxing and its encoder would be copy. `Resolve` is load-aware, so the answer can legitimately differ between programs on one channel. A first cut got this wrong and was corrected mid-build. ⚠ **`Stats` filters CLOSED sessions**: teardown is lazy (`close()` marks, the next `Attach` deletes), so an unfiltered snapshot would report a dead encoder as live *indefinitely* on a channel nobody tunes again. **SSE per your instruction**: the `playout` frame fires on channel start/stop only — not per progress sample (~1/s per stream would push several frames/second at every open browser for numbers moving by fractions) — and `GET /v1/playout/sessions` is truth on reconnect (§8). **`running:false` ≠ "nothing playing"**: on a Tunarr-backed install the list is legitimately empty, so the flag is explicit or the panel reads as every channel having died. Member sees a lockout, never a 403 wall. **New guard:** `export_parity_test.go` — `api.go` and `export.go` keep parallel register lists, and `GET /v1/playout/sessions` served fine while being absent from the spec (`openapi-verify` stayed green because the exporter never saw it). Verified failing when the route is removed from either list. |

| V28 · Filler sources read-model + clip metadata | **done** | `make check` (0 lint) + `make test-pg` + `openapi-verify` (2 new routes, register-parity green) + `retired-verify` + **713 app tests** (16 new) + **504 visual** (8 new baselines) + `make e2e` (7); two clean PLAIN verification runs, zero `Error: axe` | ⚠ **The phase row had gone stale in three ways and was corrected doc-first before any code** (plan §6.1). It called for migration `00013` — taken by `clips_path_identity`, so this is `00017`. `quality` was ALREADY shipped by `00014`, whose comment forbids it influencing selection — which contradicts V17c's gate, resolved by the maintainer as an opt-in minimum-quality floor (default off). And `sources` became a **read-model, not a table**: discovery is driven by `filler.dir` plus the library scan, so a table would be a second source of truth needing a precedence rule ("the row says /data/filler, the setting says /srv/clips — which wins?"). V33 owns the persisted registry, when a remote source has state a setting cannot hold. ⚠ **"usage populated" had no honest write point.** `filler.Assemble` takes a `used` map but `adapter.go` passes a fresh empty one per call — per-pod de-duplication with no memory — and pods re-assemble every 10m, so counting there inflates without bound and counts SCHEDULED not AIRED. The `/playout/program` HANDLER was the second wrong answer: it would count per tune-in, so three viewers on one break = three plays. The play is recorded in the RESOLVER, guarded by `into == 0` so a mid-clip re-resolve is not a second airing (verified failing without it). **Only internal playout can see this**, so Tunarr-backed installs read zero — `ClipDTO.playsCounted` exists precisely so the UI renders "not counted" instead of "0 plays". **Two bugs I introduced and caught:** (1) the new hooks went BELOW the `fillerConfigured` early return, which skipped them on one path — "rendered more hooks than during the previous render", the whole catalog replaced by an error boundary; (2) `opacity-60` on unconfigured source rows composited `text-muted-foreground` to **2.95:1 against a required 4.5:1** — an axe color-contrast failure on the one row whose job is to explain why the catalog is empty. The badge already says "not configured", legibly. **Also surfaced `quality`**, which had sat in the store for two phases with no way to see it, and corrected a Sync tooltip still claiming the scan goes "through Tunarr" (§9.1 moved it). Thumbnails are files on disk with the path in the row — thousands of regenerable images in a table would bloat every §16 backup and every V11 migration — extracted at 3s (frame 0 is usually a black fade-in), adopted rather than re-extracted, but a zero-byte partial is retried. |
| V11 · System → Database migration stepper | **done** | `make check` (37 pkgs, 0 lint) + `make test-pg` + `openapi-verify` (5 new routes, register-parity green) + **706 app tests** (14 new) + **496 visual** (16 new baselines, zero churn elsewhere) + **8 store integration tests** vs real Postgres; two clean PLAIN verification runs, zero `Error: axe` | ⚠ **Two bugs found by trying to BREAK the tests, not by running them.** (1) The copy order violated the schema's one foreign key: `sessions.user_id REFERENCES users(id)` is NOT DEFERRABLE and alphabetical order puts `sessions` first — but the seed created no sessions, so the constraint was never exercised and the suite passed green. Fixed with a topological sort over the destination catalog's FK graph; sort and order-assertion both verified failing without it. (2) `bootstrap.json` was stranded by the very migration it records: `DataDirFor` is scheme-dependent, so switching backends moved where the next boot LOOKS for the file — with the database anywhere but `/data` the switch silently undid itself and the app booted back onto SQLite having apparently succeeded. Reads now search both directories. ⚠ **The backup gate is enforced in the SERVICE, not the handler or the UI.** The mock disables the Migrate button until a backup exists; that is a hint, and anything a client can satisfy a client can skip. The state the gate reads lives on the server and is set only by the server's own Backup/Preflight calls. This is why `WriteBackup` had to write a real file — the existing streaming download keeps nothing, so the only evidence would have been the client's word for it. It also makes `backup.dir` the first consumed key of three that had been declared with zero consumers. **Corrected an overstated comment of my own:** I claimed the BOOLEAN coercion prevents a lockout bug. It does not — everything is scanned as NullString and both drivers parse "0"/"1" into a BOOLEAN themselves; disabling that branch changes nothing (verified by sabotage). The BYTEA branch IS load-bearing (channel icons corrupt without it) and now has its own test. **Derived, never hardcoded:** the table list AND the copy order come from the destination catalog, which goose just built from the same embedded migrations — this repo's own `TRUNCATE` list had already drifted to 8 of 10 tables. **Preflight** is 5 checks and refuses a populated target, which is what makes "wipe it and retry" safe advice. **Parity re-counts both sides independently** rather than trusting the copy's own tallies, which would be self-confirming. A copy failure is **409 with the reason**, not 500: the instance is fine and still on SQLite. **An env-pinned DATABASE_URL makes this copy-only** — the server refuses the switchover and the UI says so up front rather than after a full copy. Doc-first: §5 gained the migration contract, §16 the server-written backup, config-design the search-path warning. |
| V10 · All-settings search surface | **done** | `make check` (37 pkgs) + `make retired-verify` + biome/typecheck + **692 app tests** (14 new) + **480 visual** (6 new baselines, zero churn elsewhere) + `make e2e` (7) — two clean PLAIN verification runs, zero `Error: axe` | ⚠ **The mock draws this table READ-ONLY, and shipping it that way would have orphaned 19 settings.** V9 folded the old "Advanced" page into "All settings" (a restructure, not a rename), and `GroupAdvanced` holds 19 keys — job schedules, TTLs, the reconcile interval — whose *only* editor that page was. A lookup-only table here leaves them uneditable with nothing on screen saying so, and the loss falls in the seam between two phases that each look complete alone. Rows edit through the same `SettingField` (new `compact` mode) and stage into V9's cross-tab buffer, so env-pinning and the secret replace-flow behave identically. ⚠ **The save bar had to be hoisted again.** V9 lifted the edit buffer to the layout but left the *bar* in `SettingsPage`; this table is not a `SettingsPage`, so edits staged with no way to commit them. `SettingsSaveBarHost` now renders at layout level. ⚠ **A `serious` a11y violation of my own making**, caught only because an update run is not verification: `compact` drops the visible `<label>`, leaving inputs named by `title` alone (axe `label-title-only` — a mouse user sees a tooltip, a screen-reader user hears an unnamed text box). Fixed with `aria-labelledby` pointing at the visible **Key cell**, so no duplicate text is added; `aria-describedby` also now omitted in compact mode rather than dangling at a doc element that branch never renders. Pinned by a regression test. **Three provenance chips, not the mock's four:** the mock adds `generated`, but generated secrets (`playout_token`, the API token) live in a separate registry (`internal/settings/secrets.go`) and are not settings entries — the API's provenance enum is exactly env/db/default, so a fourth chip would render a value the backend never sends. Search matches key **and** group **and** value (the gate); the `Filtered` baseline proves the group clause — query `jobs` matches two rows whose keys contain no "jobs". |
| V9 · Settings IA + cross-tab save bar | **done** | `make check` (37 pkgs) + biome/typecheck + **678 app tests** (9 new) + **474 visual**, ZERO baseline churn; cross-tab tests verified failing with a per-page buffer | ⚠ **The plan's gate and the authoritative doc disagreed, and the phase could not start until that was decided.** V9's gate ("6 tabs + System sub-tabs") describes the v2 mock; `config-design.md` §5 — which CLAUDE.md makes authoritative for config mechanics — described the six pages the app *shipped*. Different sixes. **Mock won**, amended doc-first: `design/README.md` normally makes prototypes non-authoritative for IA, but that rule exists to stop a prototype overriding a considered decision, and here the v2 program's own phases are written against the mock's shape (V12 is literally "System → Backup UI"; V13's probes are System-shaped) — following the older table would leave four phases describing pages that do not exist. **A restructure, not a rename:** Channels & playback + Filler → **Defaults** (both answer "what does a new channel inherit?"), Tasks → **System → Tasks**, Users & security → **Security**, Advanced → **All settings**. ⚠ **The cross-tab save bar was a real bug, silently.** `SettingsPage` held `edits` in its own `useState`, so switching tabs unmounted the page and discarded them with no warning — edit a connection, check a default, come back, work gone. The buffer now lives in the layout above the `<Outlet />`. Pinned by two tests **verified failing** when the buffer is put back per-page. ⚠ **Defined the gate's "four inline-commit exceptions", which nothing documented** — found by reading which surfaces mutate directly. They are all **verbs**: select a model (hot-swaps the live suggester), pull a model (a streaming download), regenerate a secret (destructive and immediate), run a job now. Lifting the buffer made it newly easy to sweep them in "for consistency", so `settings-inline-commit.test.ts` asserts they stay out — with the save-bar host as the control case, so the assertion tests a real distinction. **System sub-tabs ship with Tasks only:** Playout/Database/Backup/About are V11–V13, and a tab leading to an empty page advertises a surface that does not exist — the same reasoning §12 used to defer the Dashboard nav entry until V16 built it. |
| — · Linux/macOS dev-env parity | **done** | `7807867` (branch `fix/dev-env-linux-mac-parity`) — `make check` + `make test-ffmpeg` + `make fe` + `openapi-verify` + `config-docs` + `retired-verify` all exit 0; both new tests verified by sabotage | Not a phase — fallout from bringing a **macOS** dev env onto main after 220 commits, which is the first time the post-§9.1 tree ran outside Linux. ⚠ **The offline card assumed `drawtext` and would have killed the channel.** `font.go` guarded on *"is there a font file?"*, but drawtext is a **compile-time** option (libfreetype, + libharfbuzz on ffmpeg 8). Homebrew's bottle carries neither **while macOS ships Arial** — so the guard passed, ffmpeg rejected the graph with *"Filter not found"*, the encode exited 8, and the channel was dead: exactly the outcome font.go's own comment promises never to cause. Fixed by asking the BUILD (`playout.CardFontFor`), mirroring what `listEncoders` already does for encoders — an unprobeable ffmpeg resolves to *unlabelled*, never an assumed yes. Injected into the API like `PlayoutSecret` (a property of the host, not of a request) and memoised, since the offline card is re-served on a loop. **This is parity, not a macOS workaround — a minimal Linux ffmpeg fails identically.** Doc-first in §9.1. ⚠ **`make test-ffmpeg` did not COMPILE, on any platform.** `chainResolver` had drifted from `PlayoutResolver` (missing `AudioTrackFor`) and from the `PlayoutEncoder` signature (`onProgress`, added in V16) — invisible because the target is not in `make check`, so a build-tagged suite rots silently. **A test of mine that proved nothing:** the first memo assertion was `f() != f()`, which staticcheck caught (SA4000) — replaced with a stub binary that counts execs, then verified failing without `sync.Once`. Also corrected, all parity-affecting: `.env.example` gained the entire missing **Playout** section (`SERVER_PUBLIC_URL` is a hard **tune-time** failure with **no boot warning**) plus Guide/Backup; the Node floor was `>=20` in `web/package.json` against a `node:sqlite` dependency needing **22.5+**, so Node 20 installs clean and then fails (README/CONTRIBUTING disagreed at 20 and 22 — all three aligned); and CONTRIBUTING listed no ffmpeg at all despite internal playout being the **default** backend. |
| — · Take a setting back from the environment (§3.1) | **done** | `957eb39` — `make check` + `make fe` + `make test-pg` + `openapi-verify` + `config-docs` + `retired-verify` all exit 0; 8 new service tests + store conformance (both backends) + 5 new field tests; precedence gate and clobber guard both verified by sabotage; **live round trip on the dev box** (unlock → edit → restart → held) | Not a phase — a maintainer request from hitting the wall in the wizard. ⚠ **This CHANGES A LOAD-BEARING RULE and was a doc-first design conversation before any code.** `env > database > default` with "env wins and locks the field" (config-design §2) is the GitOps contract; the motivating case is the operator who puts a value in `.env` to boot the app, reaches the wizard, and finds the field they must correct is read-only — with the only documented way out being *edit a file on the host and restart*. Answered as an explicit, per-key, admin-only, audited claim rather than by weakening precedence generally. ⚠ **The claim MUST be durable, and that decision is the whole feature.** Env is re-read every boot, so an in-memory unlock resolves back to env on restart and silently discards the operator's value — **precisely the `LLM_MODEL` failure §3 already records** (write succeeds, every read still returns env, value vanishes on restart). A button reproducing that is *worse than the locked field*, which at least tells the truth. Hence migration `00020` (`env_override`, both dialects), in §16 backups. **Provenance stays the three-value enum** — an unlocked key resolves honestly as `db`, with lock state on a separate `envOverride` flag, because "the operator overrode `BACKUP_DIR`" and "the environment never mentioned `BACKUP_DIR`" are different facts and a chip that conflates them cannot explain itself. **Unlock SEEDS from the env value** so the act transfers authority without changing behaviour — otherwise unlocking a URL to fix one character blanks it and drops the service on the click. ⚠ **A SECRET NEVER SEEDS:** copying a credential out of env into the database puts it in every backup — a security change wearing a convenience's clothes. ⚠ **`env_override` is deliberately absent from `UpsertSetting`'s `DO UPDATE` list**, or an ordinary save would re-lock the key at the one moment the operator is certainly editing it (pinned by the conformance suite, verified failing). **Bootstrap keys are not unlockable** — read before the database opens, so a flag stored in it could not affect them; they 404 rather than accepting a write that does nothing. FE: **the lock IS the control** (maintainer's correction — a separate unlock button puts the explanation and the way out in two places), with the action in the accessible name since the visible chip text is the state. **The honest cost, recorded not glossed:** a redeployed `.env` no longer fully describes a running instance; an operator who never unlocks keeps the old contract exactly. |
| — · Guide outage: a genre is not a holiday signal | **done** | `38beb87` (#113) — `make check` + `make test-pg` + `make fe` + `make e2e` + `openapi-verify` + `config-docs` + `retired-verify`; 3 tests, each sabotage-verified. **Verified live:** reconciled the maintainer's channel, `airings: []` → 3 programmes. | `horror` is in the Halloween keyword set and seasonal detection matched an entry's **genres** as well as its title, so on a year-round horror channel EVERY entry detected as seasonal and `auto` mode benched all of them for the 11 months outside October. The channel was legal to configure and dark by construction. Detection now matches **title only** (doc-first, programming-design §6): a genre is what a channel IS, a holiday keyword is what a title is ABOUT. Deleting `horror` would have fixed one channel and left the mechanism to re-fire (Romance near Valentine's, Family near Christmas). Second half: `statusFor` returned `live` without ever looking at the deck, which is how a total outage read as healthy — new `StatusEmpty`. ⚠ `len(d.Slots)==0` was the obvious empty-deck predicate and the WRONG one (filler slots make a dead deck non-empty); my own test caught it. ⚠ `openapi-verify` passed while the DTO enum still lacked `empty` — green only because the spec matched the wrong code. Genre matching had **no test at all**, which is how a total outage cleared every gate. |
| — · Suggester latency: keep the local model warm | **done** | `e19dbb7` (#114) — all 6 CI jobs green (`check`, `test-pg`, `fe`, Playwright visual+e2e, `openapi-verify` no drift, `config-docs`); 12 assertions, each sabotage-verified. **Measured live:** boot logged `llm model warmed took=8.449s`, Ollama reported `expires_at` exactly 30m out (not its 5m default), next call **0.56s**. | Two problems. (1) **Residency** (§8.2, new): a cold 8B model costs **~9.1s** vs **~0.5s** warm, and Ollama unloads after 5m idle, so a describe→read→refine cycle re-paid the load every time. `keep_alive` now rides every call (`llm.keep_alive`, §15, default 30m, 0 disables) and holds between the ROUNDS of one tool loop; a best-effort background warm-up fires on boot and every §8.1 model pick, hung off `Swappable.Set` since both paths already take it. (2) **The label lied**: `searching` was emitted ONCE before the tool loop and `reasoning` only after it exited, so the UI read "Searching the library" for the entire run including every model turn — it named the fastest step as the explanation for the slowest. Phases now report from INSIDE the loop and may repeat, each carrying its round; a slow run shows `pass 3 · 24s`. ⚠ `llm.keep_alive` is `KindDuration`, so `set.str()` **panicked at boot** — the integration journey caught it, no unit test would have. ⚠ My first "don't tick a step mid-loop" test asserted a LABEL (which renders in both states) and **passed against the sabotage**; it now asserts the tick mark. |
| — · An era must not exclude a channel's own picks | **done** | `d1e285d` (#115) — `make check` + `make test-pg` + `openapi-verify` (no drift) + `config-docs`; 5 tests, each sabotage-verified (one reproduces the live bug exactly). **Verified live:** PATCHed the affected channel `1982 → 1979`, `programCount` **4 → 5**, Alien now airing. | A "Midnight Sci-Fi Horror" proposal carried `era.from: 1982` AND **Alien (1979)** on its approved lineup, so the §4 enforcer filtered out a title the operator had explicitly approved: six on the lineup, four in the guide, nothing naming the missing two. Extraction and enforcement disagreeing about one proposal is a **self-contradiction**, which programming-design §4 already resolves toward the content for the audience ceiling — that bullet stated the principle generally and implemented it for exactly one field. `eraAdmittingPicks` adds the era twin (doc-first). Acquisitions count too (one becomes a real airing the moment it lands, so exclusion would drop it AFTER the download). ⚠ A `0` bound is UNBOUNDED and must never be widened — stretching it would close an open end and NARROW the era; caught by sabotage, not reasoning. No kids-line bound needed: a year is a curation choice, never a safety property. The era still constrains everything NOT picked (backfill, re-curation, filler); only approved titles are grandfathered. |

| V12 · System → Backup + About (`S6`, `S7`) | **done** | `63a3a25` + `604df62` — `make check` (37 pkgs) + `make test-pg` + `openapi-verify` + `config-docs` + `retired-verify` no drift + biome/codegen/typecheck + **778 app tests** (14 new) + **534 visual** (20 new baselines, **0 modified**) + `make e2e` (7); two clean PLAIN visual runs, zero `Error: axe`; the load-bearing claims each verified failing by sabotage | ⚠ **The gate said "retention honored" and `design.md:1397` said the opposite** — that scheduled backups with rotation were §20 future work and `backup.schedule`/`backup.retain` "remain declared-but-unconsumed". Both keys had been declared by V4 and read by **nothing**, so the page's own footer would have claimed "nightly at 03:30 · keeps 7" against two settings that did neither. Resolved doc-first toward consuming them (maintainer's call): §16 amended, the §20 bullet struck, §18.1 gained the job, §7 the endpoints. ⚠ **Retention filters on the `loomarr-<ts>.db` filename pattern the writer produces, never "the oldest files in the directory".** `backup.dir` is operator-set and may hold their own files; sabotage-verified — without the filter the prune deletes the operator's photos **and the live database**. ⚠ **Prune runs AFTER a successful write, never before.** Pruning first satisfies retention by destroying the oldest backup and then, if the snapshot fails, leaves *fewer* backups than it started with — worst behaviour at exactly the moment the database is unhealthy. Asserted as an OUTCOME (a failing writer must delete nothing), not by spying on call order. ⚠ **The job is not registered on Postgres**: `WriteBackup` is SQLite-only, so registering it there would put a permanently red Tasks row against a `pg_dump` strategy the operator is correctly running. The job calls the **same service** the "Back up now" button does — two implementations of a retention policy is how the scheduled and manual paths come to disagree about which files are safe to delete. ⚠ **`GET /v1/system/backups/{name}` takes a client-supplied path segment**, validated against the writer's own pattern before it reaches the filesystem; sabotage-verified — without it `../loomarr.db` serves the live database and `secrets.env` is readable. The traversal test drives the **real service**, not a fake, because a fake would only prove the fake validates. ⚠ **About shipped omitting rows the server could not fill** — the mock draws Go runtime, uptime and schema version and `/v1/system/version` returned none of them; "Uptime —" is a promise the page cannot keep. *(Closed the same day by the row two below, which added the fields rather than leaving the gap.)* Also promoted `formatBytes` into `@loomarr/core` (it lived only inside database-migration, and a second copy is how four core formatters previously acquired a live grammar bug). |

| — · A job that cannot run here says so | **done** | `a457940` — `make check` GREEN **in both build configurations** + `make test-pg` + new Postgres integration tests through the REAL composition root + biome/typecheck + **782 app / 39 core** (4 new) + **534 visual** (zero baseline churn) + `make e2e` (7); every enforcement point verified failing by sabotage | **Maintainer overruled my recommendation, and was right.** V12 omitted the backup job on Postgres; I argued the Backup page + help doc + `/v1/backup` 501 already said so. But an omitted row is *also a claim*: from the Tasks page alone it is indistinguishable from a job that runs fine and has never failed — and for backup that ambiguity means believing you are covered when you are not. The un-registration sabotage prints it plainly: nine healthy-looking jobs and no backup row anywhere. `Job.DisabledReason` (§18.1, doc-first) is enforced at **four** points: never SEEDED a state row; never CLAIMED even when one exists — ⚠ **load-bearing, not belt-and-braces**, because a SQLite → Postgres migration (V11) leaves the same install holding a due `backup` row against a backend that can no longer run it, and the sabotage run *executed* it; `Trigger` → 409 (⚠ **409, not 404** — the job EXISTS and is listed, so a 404 would send an admin hunting for a name on their screen); and `NextRun` zeroed, since a leftover row would promise a run that never comes. ⚠ **Not an operator "off" switch** and no enable control — it states a fact about the environment. The queue pollers stay conditionally registered *deliberately*: exactly one is correct at a time and the other is **irrelevant, not unavailable**. FE: reason under the title (where `lastError` sits), "Not scheduled" rather than a cron that will never fire, and Run-now/Modify **absent, not greyed** (a disabled button invites a hunt for a tooltip when the reason is already on the row). ⚠ The status dot checks disabled FIRST — falling through to "Not run yet" reproduces the exact ambiguity one cell over. ⚠ `make check` caught a helper only the integration build calls (`unused`): a build-tagged file rots silently, the same class as the `make test-ffmpeg` drift. |

| — · About: the rows an operator quotes in a bug report | **done** | `35d7e23` — `make check` + `make test-pg` + biome/typecheck + **786 app / 43 core** (8 new) + **536 visual** (2 new + 6 updated baselines, all four AboutPanel stories, zero unrelated churn) + `make e2e` (7); two clean PLAIN visual runs, zero `Error: axe`. **Verified live** against the maintainer's own install: `go1.26.4`, `darwin/arm64`, `sqlite`, `schema 20`. | Closes the gap V12's row recorded rather than leaving it as known drift (maintainer's call). §7 amended doc-first. ⚠ **`startedAt` is an INSTANT, never a pre-computed uptime** — a duration is stale the moment it is serialized, so the server sends the instant and the client derives elapsed time it can keep current; sabotage-verified (measuring from `builtAt` instead of process start fails two tests). ⚠ **`schemaVersion` is read at CALL time, not cached at boot** — the V11 stepper can move an install onto a different database within one process lifetime, and a boot-time value would then describe a database the app no longer talks to. Verified to return the real **20**, matching `00020_settings_env_override.sql`, not a placeholder. ⚠ **Rows the server cannot fill stay ABSENT** (a source build has no commit; a store-less boot no schema) — and the store-less path still serves the endpoint, since it is what an operator reads to find out *why* an install is unhealthy. **ONE query on the page:** `backend` used to come from the database-status endpoint while everything else came from the version endpoint, so the page could render two half-answers from two requests that resolved differently; it now rides `/v1/system/version`, paired with the schema version in one Database row. New `formatUptime` in core — distinct from `formatDuration` (caps at hours: a week reads "168h 0m") and `formatRelative` (too coarse); "just started" rather than "0m", which reads like a broken value. |

| — · Dev backend live-reload was committed but unrunnable | **done** | `09be9b9` — `make check` GREEN + `make ci-lint` clean. **Verified by running it:** `make dev-be` boots against the REAL store (the maintainer's "Midnight Sci-Fi Horror" channel is present) and a touch to `internal/api/help.go` restarted the process in ~26s, confirmed by `startedAt` advancing across the change *and* its revert. | Not a phase — found by asking why new About fields "weren't showing up" at `:5173`. They were: the backend on `:8080` was an **orphaned `go run` binary from 09:48** (parent `launchd`, supervisor gone) serving pre-change code for hours. ⚠ **`.air.toml` had been committed since July with Air never installed**, so nothing ran it — and worse, its `full_bin` pinned `DATABASE_URL=sqlite://./loomarr-dev.db` (**which does not exist**; the real store is `./data/loomarr.db`) and set **no other env**, while `LIBRARY_URL`/`TUNARR_URL`/`SEERR_*`/`TMDB_API_KEY`/`LLM_*` all come from `.env` and the binary does not read `.env` itself. Its own comment claimed this was "the same as the manual launch"; running `air` would have booted an **empty, disconnected instance on the same port** — which reads as data loss until you work out it is another database. Now sources `.env` (one source of truth) and `make dev-be` runs Air via `go run air@v1.67.3` (a dev tool, so it stays out of `go.mod` per §14). This is the **same `go run` supervises-rather-than-execs** behaviour recorded as smoke-bug #5. `GET /v1/system/version` now reports the built-from commit + dirty flag, so "is this process stale?" is one curl. Documented in README + CONTRIBUTING, since undiscoverability is what caused it. |

| V13 · Restart/reload control + service probes (`S5`) | **done** | `fba6625` (doc) + `6946759` (backend) + `ea73386` (UI) — `make check` GREEN + `make test-pg` + `make fe` (**806 app / 43 core**) + **558 visual** over TWO clean PLAIN runs (30 new baselines, **0 modified**, zero `Error: axe`) + `make e2e` (7) + openapi/config-docs/retired no drift. **Verified live on the real stack**, not just in tests. | ⚠ **The mechanism was a real decision and the maintainer's Windows question changed it.** I had settled on `syscall.Exec` (same PID, no supervisor, bounded failure) until asked how Windows would work: `syscall/exec_windows.go` returns `EWINDOWS`, so it **compiles cleanly and fails only at runtime** — the button would ship broken rather than refuse to build. Chose the **in-process rebuild loop** instead (`for { app := Build(); app.Run(); app.Shutdown() }`), which is what **Jellyfin** does and the reason Windows needs no special case there. Rejected exit-and-be-restarted: it assumes a supervisor `make dev-be` and any bare binary lack, and the exit-code contract is supervisor-specific (Docker `unless-stopped` restarts on 0; systemd `on-failure` does not). Audited this repo before committing to it — `http.DefaultServeMux`/`expvar`/`sql.Register` are **entirely absent**, the one `prometheus.Register` already tolerates `AlreadyRegisteredError` ("a second boot in one test process" — exactly a restart), and every `sync.Once` is closure-local. ⚠ **The live test found a bug no unit test would have:** `startedAt` was a package-level var **I added in V12** — it SURVIVED the rebuild, so About kept reporting the original boot and would have claimed days of uptime on an instance restarted seconds ago. Silent: no panic, no log line, just a wrong number in the one place an operator looks. Exactly the hazard §9.2 warns about, in my own code. ⚠ **`RestartRequired` is DERIVED** from running-vs-resolved bootstrap values, so reverting an edit stops the nagging — a flag written at save time never would. ⚠ Admin is checked **before** the loop is asked to rebuild (a 403 that still restarted would be a DoS with an error message), and reload must not restart (it is the no-downtime option). **Three maintainer corrections, each REMOVING something:** Reload deleted from the UI (settings already hot-apply on save, so the button implied saving was not enough); the five-row "here's exactly what happens" fact list replaced by the one consequence that varies; and the app now **dims and blocks input** during a restart — app-wide in the shell, because an operator can navigate away the instant they click. ⚠ **A visual-harness bug found by READING the baseline rather than trusting a green run:** the overlay is `fixed inset-0`, so it renders OUTSIDE `#storybook-root` and the first baseline captured an empty sliver. Also deleted two "renders nothing" stories — the harness waits for an element, so they timed out at 30s each. **The §9.2 gate is a test, not a rule:** an N-generation Build/Run/Shutdown asserting stable goroutine count, verified to catch a simulated leak (5→8→11→14→17); adds `go.uber.org/goleak` per §14. |

| V31/V32 · Dashboard Services panel + Recent activity | **done** | `1cc8390` (doc) + `83ece67` (backend) + `ec53ee0` (SSE frame) + `6e1cd08` (UI) — `make check` GREEN + `make test-pg` (migration `00021` conformant on both) + `make fe` (**818 app / 43 core**) + **570 visual** over TWO clean PLAIN runs (12 new baselines, **0 modified**, zero `Error: axe`) + `make e2e` (7) + openapi/config-docs/retired no drift. | ⚠ **The feed is written at each domain TRANSITION, not by tapping the event bus** (maintainer's call). Persisting at the bus is one line and looks free, but `events/bus.go` says outright it is in-memory and DROPS events for a slow subscriber — "a dropped event is a latency bug, not a correctness bug" — so a feed built on it would lose rows exactly when the install is busiest, and V32's gate is "persisted; survives restart". It is also domain-neutral: it carries `{type:"title"}` where the feed needs "Darkwing Duck landed". ⚠ **Then the maintainer asked why Services polls instead of using SSE, and the answer flipped the OTHER panel.** A probe result is not a state change anyone observes — the server only learns it by making six outbound calls (**measured at 730ms** against the real stack), so "pushing" it would need a server-side timer probing continuously whether or not a browser is open, and it would invert §8's rule (a frame is droppable precisely because a GET can re-derive the truth; here the probe IS the truth). The FEED is the opposite shape, so it now takes an `activity` frame and polls not at all. The asymmetry is recorded in §12 so it is not later "fixed" into consistency. ⚠ **The frame carries NO payload and fires only on a SUCCESSFUL write** — sending the row would invite a list assembled from frames this bus drops by design; announcing a failed write would send the page to refetch an unchanged list on every attempt. Both sabotage-verified. ⚠ **A defect the green suite did not catch, found by READING the baseline image:** "14m ago" wrapped onto two lines in a `w-11` column, so rows rendered at uneven heights — every test passed throughout. A screenshot is only evidence if someone looks at it. ⚠ **Two guards earned their keep:** the architecture test refused `internal/activity` until §14.2 documented it, and the events-provider test caught that the provider never fanned out `onActivity` — its own message explains why TypeScript could not ("every EventHandlers key is optional"), which would have left the feed live in `core` and silently inert in the app. **Also corrected a §5 claim that was never true:** the janitor section described jobs and proposals being purged after `JOBS_RETENTION`/`PROPOSALS_RETENTION`; no purge exists for either and both keys are read by NOTHING — the same declared-but-dead shape V12 found in `backup.retain`. Recorded as open work; `activity.retention` is consumed in the same PR that declares it so it does not become a third promise. |

| V8 · SSO as a credential path (`D-F`) | **done** | `f10667f` (§11 doc) + `7bd4f1e` (OIDC service) + `5ee6492` (routes) + `862d772` (login UI) + `8084a24` (Security block) + `d817fb1` (3 more sabotage checks) + `8da3d55` (Authelia userinfo fix) + `22e8205` (Authentik issuer fix) + `664c1bd` (Authentik flow walk) + `24f25fb`/`83d4007`/`00f37aa` (security-review fixes) — `make check` GREEN + `make test-pg` + **`make test-sso` GREEN (Authelia 4.39 + Authentik 2025.10, 33s)** + `make fe` (**824 app / 43 core**) + **574 visual** (4 new baselines, 0 modified) + `make e2e` (7) + openapi/config-docs/retired no drift. | ⚠ **Touches §11, which CLAUDE.md lists as non-negotiable — so the amendment landed first and is an ADDITION, not a change:** every invariant holds verbatim. An SSO identity with **no allowlist row is REJECTED** even with a valid provider token (the direct analogue of the un-imported media-server case), there is **no `auth.sso.auto_create`** and no code path that creates a row, and **no `auth.sso.admin_group`** — a provider claiming `loomarr-admins` is describing its own world. All three sabotage-verified, including by adding the classic auto-provision-on-first-sign-in the mock draws. ⚠ **OIDC only, not the mock's forward-auth mode** (maintainer's call): header trust is only as strong as the operator's network wiring, and a Loomarr reachable beside its proxy would accept `Remote-User: anyone` as identity — a total auth bypass with no signal. Forward-auth recorded as §20 open work. ⚠ **SABOTAGE FOUND A SECURITY CHECK NOTHING COVERED:** removing the nonce replay check broke NONE of eight passing tests, because each carried the correct nonce. A token minted for one login could be replayed into another's callback; now pinned by a test that starts two logins and completes the second with the first's token. ⚠ **And it corrected a claim I had written.** My no-session-on-refusal test asserted only the cookie's NAME, and passed against a sabotage removing the early return (the cookie is emitted with an empty token either way). Tightened to require no cookie with a VALUE — which STILL passed against a forged token, and that was informative rather than a gap: `http.Redirect` writes headers immediately, so a later `SetCookie` is discarded by net/http. **The guarantee is structural, not the `return`** — recorded above `redirectToLogin` with a warning not to tidy the refusals into a single exit that redirects last. ⚠ **Refusals carry a reason CODE, never a message**, and the vocabulary lives in the frontend: sabotaging the fallback to echo the code renders *"Your session expired, call 555-0100"* on our own login page. ⚠ `next` is validated as a same-app path — the naive `HasPrefix("/")` lets `//evil.test` through, since browsers treat protocol-relative URLs as off-site. ⚠ The test IdP **really signs** (discovery + JWKS + RSA), so `go-oidc` does production verification; stubbing the verifier would let a test pass while the real path accepted an unsigned token. §14 gained three modules (`go-oidc/v3` + `x/oauth2` + `go-jose/v4`), none previously present even indirectly. Also corrected two stale §11 claims: it opened with "two credential paths" (now three), and dev-login called itself "the only sanctioned third credential path" — it is a bypass of the credential CHECK. ⚠ **THREE MORE UNGUARDED CHECKS, same systematic sabotage** (`d817fb1`): **audience** (`SkipClientIDCheck` changed nothing — a token minted for a DIFFERENT client on the same homelab IdP was accepted here), **expiry** (`SkipExpiryCheck` changed nothing — one captured id_token was a permanent credential), and **state single-use** (removing the `delete` changed nothing — a callback URL sits in browser history and every proxy log between). **Four of the five holes in this file came from sabotage, not foresight** — worth recording, because it is the argument for enumerating branches rather than testing what occurs to you. ⚠ **TWO REAL PROVIDERS FOUND TWO REAL BUGS, NEITHER VISIBLE TO THE OTHER.** `make test-sso` stands up **Authelia** and **Authentik** and drives the whole flow; a hand-written stub is one reading of the spec on BOTH sides of the wire, so a misunderstanding agrees with itself. **Authelia** (`8da3d55`): profile claims live at **userinfo**, not the id_token, so `MatchName()` fell to an opaque `sub` UUID and **every login against a default Authelia was refused**; the userinfo `sub` is now cross-checked against the token's, and the severity is ESCALATION — the guarding test signs in as someone whose id_token has no name while userinfo describes a different subject it calls `admin`, and with the check removed the login succeeds AS THE ADMIN. **Authentik** (`22e8205`): its issuer is path-based WITH a trailing slash and OIDC requires an exact match, so our `TrimRight` broke discovery outright — an operator pasting the value Authentik displays could never connect. They also **DISAGREE about HTTPS** (Authelia refuses plain HTTP, Authentik serves it), so neither assumption is baked in. Two operator-facing harness findings kept: Authentik needs a **signing key assigned** or it signs HS256 (an error that reads like OUR verification bug), and Authelia's **cookie domain must match the dialed URL**. ⚠ **`/security-review` FOUND TWO MORE** (`24f25fb`+`83d4007`+`00f37aa`). **Login CSRF:** `/start` minted a state and threw it away (`authURL, _, err :=`), so nothing was written to the browser — **a server-side state map answers "did SOME login start here?", never "did THIS browser start it?"**, and only the second is CSRF protection (two code comments claimed otherwise). Someone the provider authenticates could capture their own `?state=&code=` redirect and get a victim to follow it, landing the victim in the app **signed in as the attacker**, with every subsequent action attributed to the attacker's account. Now a state cookie set at `/start` and required at `/callback` before the exchange; sabotage mints a session for an unbound callback, i.e. the attack reproduced. ⚠ **`SameSite=Lax`, and `Strict` is the trap:** the callback is a cross-site TOP-LEVEL NAVIGATION from the provider, so Strict would not harden it — it would break every SSO login, and **no stub IdP performs a real cross-site redirect, so only a real provider can catch it**. **Open redirect:** `safeReturnPath` read as thorough and let `/\evil.test` straight through (starts with `/`, not `//`, no `://`) — browsers treat `\` as `/` per WHATWG, and Go emits it verbatim since `path.Clean` treats `\` as ordinary. Now PARSED with the backslash rejected before parsing (`url.Parse` reports an empty Host for it), and **re-validated at the callback** rather than trusting a gate three hops upstream. **PKCE** (S256) added alongside, with the honest scope recorded on `pendingLogin.verifier`: ⚠ **it does NOT close the CSRF hole** — an attacker holds their own verifier — it binds the CODE to a flow while the cookie binds the FLOW to a browser. ⚠ Note for the threshold-watchers: the CSRF finding scored **7/10 and was filtered out of the review report** (the bar is 8); fixing it was the more valuable half. |

### ⚠ The Filler UI is behind its own backend — audited 2026-08-01

Filler's phases all read **done**, and the track still does not look like the v2 mock. The
audit that found it was comparing the running page (`/filler`, real dev stack) against
`design/loomarr-prototype-desktop-v2.dc.html`, not reading the phase rows — the rows are
accurate about what each phase built and silent about what the mock asked for.

⚠ **The recurring shape: the API sends the field and the UI never renders it.** `ClipDTO`
already carries `playCount`, `playsCounted`, `lastPlayedAt` and `quality`; the clip card
shows none of them. V30 shipped `GET /v1/filler/thumb/{path...}`. So most of the gap is
*rendering*, not systems — which is why every phase gate passed.

**Tier 1 — data already sent, nothing rendering it**
- Play count / last played on the clip card (the mock's `usedLine`)
- Quality badge on the clip card
- ~~An `AI-TAGGED` marker distinct from the V34 era-suggestion badge~~ — **retired, see below**
- Header status line (`N sources · N clips · last scan …`)

**Tier 2 — UI only, no new backend**
- ~~Inline tag cycling (click era/audience/category to retag)~~ — **retired, see below**
- Block-per-channel on the card (pin exists, block does not) — **reshaped, see below**
- ~~Discover filter chips (era/audience) over the existing query~~ — **retired with the tab**
- ~~Discover empty state: *"Widen the era, or browse a curated source"*~~ — **retired with the tab**

**Tier 3 — needs backend, decide individually**
- **Catalog-wide coverage panel.** Coverage is per-CHANNEL
  (`/v1/channels/{id}/filler/coverage`); the mock's Filler-page version is catalog-wide with
  a diagnosis line. New endpoint. — **reshaped into a Pool health strip, see below**
- **Richer discover rows** (thumbnail, description, duration, quality, licence).
  `DiscoveredClip` is `{id,title,url,year}`. ⚠ **Spike archive.org first** — V33 already
  measured that ~92% of items declare no licence, so promising these fields before checking
  would repeat the licence-badge mistake §6.3 records. — **the redesign already thinned these
  rows to `date · dur · quality` with no licence; the spike's answer was applied for us**
- ~~**Curated source registry** (the mock's "Pull era matches…")~~ — **retired; the registry
  collapses into the toggleable Sources list**

⚠ **Two mock elements were never authored and cannot be "ported":** the 4-step pipeline
explainer (`hint-placeholder-count="4"`, no data) and the starter pack's own rows
(`hasSeedPack` is hardcoded false). Building them means INVENTING content — the same
situation the plan already records for `tcOptions`/`poPresets`. Do not treat their markup
as a specification. **Both are absent from the redesigned screen** — the question resolved
itself by deletion, which is the outcome "do not invent content" was protecting.

### ⚠ …and the audit was overtaken the same day — the mock changed (2026-08-01)

The list above was written against `design/loomarr-prototype-desktop-v2.dc.html`. Hours later the
design project's copy was re-fetched and **the Filler screen had been rewritten**: 125 lines of
markup became 302, and the tabs went `Coverage · Catalog · Sources · Discover` → **`Catalog ·
Incoming · Sources`**. The strikethroughs above are that reconciliation — **five items are retired
outright and three reshaped**, so working the list as first written would have built things the
design has since dropped.

Recording it rather than silently re-writing the list, because the failure mode is the interesting
part: an audit is a claim about a moving reference, and this one had a shelf life of one day. Two
of its items (`usedLine`, the quality badge) survive untouched, which is the honest measure of how
much of it was durable.

⚠ **The deleted interactions were SHIPPED, not merely planned.** Inline click-to-cycle retagging
(`cycleFor`, `filler-page.tsx:130`) landed on this very branch two commits ago and the redesign
replaces it with a select-based **Edit tags** editor; pin/block becomes a single include-set
override with a per-channel fit note and a *Back to automatic* reset. Deleting working code on the
strength of a mock deserves the scrutiny — the argument for it is that both were built *toward*
this screen, and the screen changed.

⚠ **The largest change is not on the Filler page at all:** filler acquisition gains an **approval
gate**. "Find clips" is renamed *"Propose a pull"* in three places and Approvals grows a `FILLER
PULL` card. §10 already stated the principle without an object to hang it on — *"the machine
proposes, a human commits"* — so this is the gate arriving where the doc said it belonged.

~~⚠ **The prototype file in `design/` could NOT be updated** and is stale for this screen.~~
✅ **RESOLVED 2026-08-02 — the export landed** (top entry). The cap was real: `DesignSync.get_file`
stops at 262,144 bytes, before the file's ~192 KB of JS, and a markup-only splice renders nothing.
⚠ The `support.js` "byte-identical, so no runtime change" claim in this block was **wrong** — it
was verified against the truncated fetch.

**Both new phases are specced in plan §6.5: V35** (the Filler page as redesigned) and **V36** (the
Watch player, which arrived in the same import and is blocked on playout emitting raw MPEG-TS —
nothing a `<video>` element can play).

~~**Next up: V35's remainder, then V36.**~~ **SUPERSEDED 2026-08-02 — see the top entry.** Of the
three items this line named: **1.7 was already built** (the record was stale, not the code); the
cycle→Edit-tags swap was **ratified as not-doing**; and the Sources search/add UI grew into **V37**
once YouTube and community packs became real source kinds. The current answer is at the top of this
file — ⚠ **read that, not this line.** This is exactly the drift CLAUDE.md's worktree section warns
about: a "Next up" that outlives the work it points at still reads as current.
The V34 note below is history; its own row records completion.

*Historical, kept for its findings:* **V34 — compilation splitting + per-clip metadata** (plan §6.4).
The whole `### Filler` table has now shipped, and V33's discovery surfaced what V34 exists for:
much of what an operator finds is a COMPILATION — one file holding twenty adverts — which
ingests as a single 15-minute "clip" the pod assembler can never place.

⚠ §6.4 was written from MEASUREMENT on 6 real compilations, not from reasoning, and two of its
findings are load-bearing: one 149s block defeated every A/V detector while its TRANSCRIPT showed
three complete adverts (so the LLM step is the only signal that sees that boundary class), and
running the real `tagSystemPrompt` over real transcripts **invented an era on 2 of 10 clips** —
a §8 grounding hole `validateTags` cannot currently detect.

### ✅ V34 unblocked — both maintainer calls made (2026-07-31), phase active

**Where it stands (2026-07-31, branch `feat/v34-compilation-splitting`):** doc-first pass
(`21b5d1b`), the **era grounding rule** (`8642506` — ungrounded AI eras are operator-confirmable
suggestions on every tagging path, sabotage-verified, migration `00024`), and the **backend
pipeline** (`012d6d4` — whisper settings keys, split-proposal store `00025` conformant on both
backends, chapter triage → blackdetect/silencedetect → whisper+LLM rescue → classify → dHash
dedup → Confirm, with Unsplittable as a first-class never-guess outcome). `make check` +
`make test-pg` green. The **endpoints** (`e6f3abf` — the three §7 split routes wired through the
real composition root, `filler_split` SSE frame, openapi/orval regen) and the **review UI**
(`016fa51` — cut-list editor, era-suggestion confirm, dedup flags, unsplittable rendering, plus
the era badge on clip cards) followed. All FE gates green at `016fa51`: `make fe` (959 app / 49
core), **624 visual (12 new baselines, 0 modified)**, `make e2e` (7), no openapi/config-docs/
retired drift.

⚠ **The review route is a SIBLING of `/filler`, not a child** — the catalog page renders no
`<Outlet/>`, so nesting would have made the whole surface unreachable while every unit test
still passed. That is the V1/V17a/V23 failure mode exactly, so the reachability suite now
derives `/filler/splits/$proposalId` from the router AND requires a clip card to offer the
entry point. A component test could not have caught it.

The **vendoring** closed item (3): whisper.cpp v1.9.1 + `ggml-small.en.bin` (pinned by revision
AND sha256), both platforms building and transcribing. ⚠ Note the numbered list below predates
this and calls the binary "self-contained" — it is not; see the correction two paragraphs down.

⚠ **The model verification was a real gate, not a formality — it changed the answer twice.**
Measured against the *vendored* binary on a real 244s 1990 commercial break, scoring gaps against
**loudness** rather than counting them (a gap over silence is whisper being correct): `tiny.en`
dropped a complete 20s advert at the file's average loudness, **`base.en` dropped 7s of equally
audible speech**, and only `small.en` had no gap over audible content. `base.en` is newly ruled
out — the intuitive compromise, a size §6.4 never measured. Maintainer's call to bake the 466MB
model in rather than fetch on first use, keeping §16's "no variant omits the tooling" promise.

⚠ **`--help` cannot prove a vendored binary works.** whisper-cli is NOT self-contained like
yt-dlp (§14 said it was; corrected): it links libwhisper+libggml, and ggml `dlopen()`s its compute
backend from the **executable's own directory**, not `ld.so.conf`. The obvious layout passed
`--help` on arm64 and aborted on the first real transcription with `GGML_ASSERT(device) failed`.
The build-time proof therefore checks that ggml **loads its compute backend** through the
`/usr/local/bin` symlink — sabotage-verified: putting the binary back in `/usr/local/bin` fails
the build at that line. ⚠ It deliberately does **not** transcribe on both arches: doing so cost
~3s natively and **~341s under QEMU** (whisper is dense matrix math, the worst case for
emulation). The full transcribe-real-audio proof is gated to the **native amd64 leg**, where it
is cheap and still catches a bad model download.

⚠ **It also surfaced a release-blocking arm64 bug that predates V34:** `intel-media-va-driver`
has no arm64 candidate, so `apt` exited 100 and the arm64 half of `release.yml`'s
`linux/amd64,linux/arm64` matrix could never have built — nor could any local build on an
Apple-Silicon Mac. Now amd64-only. **Nothing in the gates would have caught it, because CI did
not build the image at all** — `docker build` ran only at release time, on a `v*` tag. Closed in
the same PR: CI now has an **Image job** gated on `Dockerfile`/`.dockerignore`, building both
release platforms.

**Remaining:** (4) live verification on the maintainer's stack — a real compilation ingested,
split, reviewed, and its segments playing in a pod.

1. **whisper-cli approved as the fifth vendored binary** (the §14 conversation is resolved):
   whisper.cpp's self-contained binary, exec'd like yt-dlp, no cgo, no service, shipping in the
   single image with its model file. ⚠ The shipped model is the smallest verified gap-free against
   the *vendored binary* — §6.4's measurements used the Python package, which is not what ships, so
   model verification against the real binary is part of the gate, not a tuning preference.
2. **The invented-era hole closes with BOTH halves:** the validator accepts an era only when the
   year appears literally in the source text, AND an ungrounded era is recorded as a suggestion the
   operator confirms. Applied to every tagging path — the sidecar path has always been able to hit
   it; transcripts merely made it frequent enough to measure.

Doc-first, same PR: design §14 (the binary), §16 (image contents), §10 (splitting pipeline + era
rule), §7 (the split-proposal routes + era-confirm on `PATCH /v1/filler/{id}`), §5 (persisted split
proposals), §15 (`INGEST_WHISPER_PATH`/`INGEST_WHISPER_MODEL`), §20 (transcript bullet), plan §6.4.

### Where the exploration lives

The V34 measurements came from a scratchpad that is NOT in the repo (compilations downloaded from
YouTube, a Python venv with whisper/PySceneDetect, split clips). It is reproducible from §6.4 —
every number there names its method — but the artefacts are gone. ⚠ Do not treat §6.4's figures
as re-runnable without redoing the captures; treat them as a recorded experiment.

⚠ **Worktrees and merged branches were cleaned up when V33 landed** — one tree on `main`, no
leftovers. Worth checking with `git worktree list` before starting V34 anyway: three dead trees
had accumulated across the Filler track, because a merged PR does not remove the tree that built
it and nothing in the gates notices.

| Phase | Status | Gate evidence | Notes |
| --- | --- | --- | --- |
| V29a · export coverage over the real ladder | **done** | `make check`; agreement test sabotage-verified (reimplementing the rung choice fails 3 of 7 cases) | `filler.Coverage` calls the same `candidatePools` `Assemble` calls. V29's original gate — "consumes `ladder.go`" — was **unbuildable**: every symbol there is unexported. Split into V29a (export) + V29b (meter). ⚠ **This commit shipped BROKEN first:** `coverage.go` was swallowed by a `coverage.*` line in `.gitignore` (meant for Go profile output), so it committed a test with no implementation — green locally, unbuildable from a clean clone. Found only when cherry-picking to the other lane. Ignore rule now names artifacts instead of globbing; fix verified by cloning fresh. |
| V30 · serve clip thumbnail bytes | **done** | `make check` + `openapi-verify`; containment sabotage-verified | `GET /v1/filler/thumb/{path...}`, member-visible. Named `thumb` NOT `preview` — that word already means the pod pool, twice over. ⚠ **A security test that could not fail:** the traversal test passed with `safeThumbPath` DELETED, because net/http rewrites the URL and Go 1.22 ServeMux refuses to decode `%2f` for a `{path...}` match. The guarantee moved to an in-package test of the function, which does fail on all four escapes. |
| V29b-api · `GET /v1/channels/{id}/filler/coverage` | **done** | `make check` + `openapi-verify`; 5 route tests | Per-channel, resolved through the same `SelectionForChannel` the previews use. `rungs: []` never null; `total` is the widest rung, not a sum (they nest). |
| V17b · ClipCard renders the extracted frame | **done** | `make fe` (903) + `make fe-visual` (594), 4 baselines | The last link in a chain complete since V28 — `thumbnail` crossed the wire into a directory nothing served. Frameless clips render NO placeholder: on a Tunarr-backed install that is the whole catalog. 2 of the 4 baselines are the StatusDot `error` tone from #146, whose CI run was cancelled when #147 merged 21s later. |
| V17c · opt-in minimum-quality floor | **done** | `make check` + `openapi-verify` + `config-docs` + `retired-verify`; byte-identical test sabotage-verified | ⚠ **Amends a client-visible contract.** `00014_clips_quality` and `openapi.yaml` both said quality NEVER affects selection; `clip.go` said "by default". Amended doc-first, both dialects. Default 0 = off, and the migration's warning stays written down verbatim — it is why. ⚠ **My headline test did not work:** it compared `Policy{}` to `Policy{MinQualityHeight:0}`, so a floor leaking in as a DEFAULT applied to both sides and stayed green through the exact failure it existed to catch. Now asserts a fixed expectation, and the same sabotage fails it. |
| V29b · the coverage meter | **done** | `make fe` (911) + `make fe-visual` (600); reachability sabotage-verified | Mounted in ChannelFiller above the break preview. Does NOT render the mock's "Breaks resolve exactly N% of the time" — that number is not computable from a catalog (§6). ⚠ Shipped a crash for ~10 minutes: an unknown `level` made `LEVEL_COPY[...]` undefined and reading `.tone` took down the whole Filler section, reddening five unrelated tests. Unknown levels and rungs now degrade, with regression tests. |
| V17d · "Find clips" when the ladder widened (F4) | **partial — the routing half** | `make fe` (913) + `make fe-visual` (600), 4 baselines | "Thin" is `level !== "exact"`, the ladder's own answer, NOT a clip count — a count threshold gets both interesting cases backwards. **The starter pack is deliberately not built:** its load-bearing question is where seeded clips come from, and both answers are the empty drop-folder or V33's discovery. ⚠ **Answered 2026-08-01** — a curated archive.org collection, via the `collection` mode now on `GET /v1/filler/discover`. That gave `clipfetch.DiscoverCollection` its first non-test caller: it shipped with V33 complete, conformant and reachable by nothing. **The UI half is still open** — see the Filler UI audit above, which found this row is not the only thing outstanding on this track. |
| V33 · discovery + sources registry | **done** | `make check` + `openapi-verify` + `test-pg`; `make fe` (121 files, **935 tests**) + `make fe-visual` (**612**, 10 new baselines, no churn); reachability sabotage-verified | **Unblocked by a supervised live capture** (maintainer-approved): `metadata_item_licensed.json` + `collection_search.json` + `keyword_search.json` from archive.org, and `ytdlp/ytsearch.jsonl` from a real `yt-dlp ytsearch` run. Three findings changed the design — the field is `licenseurl` (one word; `license`/`rights` checked and absent), **~92% of items declare none** (667 of 8362), and a licence routed through `SidecarText` would have been silently eaten by `isBoilerplate`, which drops any `http(s)://` line. ⚠ **The licence-badge gate was AMENDED, not met** (§6.3): yt-dlp returns `license: null` for EVERY YouTube result — verified on both flat and full extraction — so a badge could only ever read "unknown", implying a per-item check that never happened. The caveat is stated once, about the search. ⚠ **The pinned fixture caught a type I had guessed**: `year` is an integer, not a string. ⚠ The `filler_sources` table shipped in #152 with a full store layer, conformance on both backends, and **no reader** — the eighth-plus instance of this repo's founding bug, in my own work; V33 closes it at both ends. Search is keyword-across-archive.org (maintainer decision), parenthesised NOT quoted (quoting forces an exact phrase and finds nothing) and scoped to `mediatype:movies`. |
| V34 · compilation splitting + per-clip metadata | **done** (code; live verify outstanding) | `21b5d1b` (doc) + `8642506` (era grounding, migration `00024`) + `012d6d4` (pipeline, `00025`) + `e6f3abf` (endpoints) + `016fa51` (review UI) + `93f6c62` (vendoring) — `make check` + `make test-pg` + `make fe` (**959 app / 49 core**) + **624 visual** (12 new, 0 modified) + `make e2e` (7); no openapi/config-docs/retired drift. | ⚠ **The model gate changed the answer twice.** Verified against the *vendored* v1.9.1 binary on a real 244s 1990 commercial break, scoring gaps by **loudness** rather than counting them (a gap over silence is whisper being correct): `tiny.en` dropped a complete 20s advert at the file's average loudness, **`base.en` dropped 7s of equally audible speech** — a size §6.4 never measured and the intuitive compromise — and only `small.en` had no gap over audible content. ⚠ **`--help` cannot prove a vendored binary works**: it returns 0 without initialising a compute backend, and did so on arm64 in a layout where the first real transcription aborted with `GGML_ASSERT(device) failed`. whisper-cli is NOT self-contained like yt-dlp (§14 said it was; corrected) — it links libwhisper+libggml and ggml `dlopen()`s its backend from the **executable's own directory**. ⚠ **The review route is a SIBLING of `/filler`**: the catalog renders no `<Outlet/>`, so nesting would have made the whole surface unreachable while every unit test passed — the V1/V17a/V23 mode again, now pinned by a reachability assertion that requires a clip card to OFFER the entry point. ⚠ **It also surfaced a release-blocking arm64 bug that predates V34** (`intel-media-va-driver` has no arm64 candidate, so `apt` exited 100 and the arm64 half of `release.yml` could never have built) — invisible because **CI did not build the image at all**; closed by a new Dockerfile-gated Image job. **Remaining: maintainer live verification only.** |
| V22 · wizard reconciliation (`bk.summary`) | **partial — `bk.summary` done; see notes** | `make check` + `make fe` (**964 app / 49 core**) + **628 visual** (4 new, 4 modified — the shared header) + `make e2e` (7); no openapi/config-docs/retired drift. | The row lists THREE things and only one was still open. ⚠ **S12 was already closed by V2b** (`3b0bca2`): the defect was the §20 bullet *"§13's wizard deliberately validates rather than stores"*, and it is struck. A DB-backed-settings-UI bullet still reads as live at a glance but is marked **Resolved and superseded** in place — re-striking it would have been re-litigating a settled question, which is the exact failure `CLAUDE.md`'s worktree section warns about. ⚠ **`bk.summary` is DERIVED, not a new API field.** The v2 mock computes it client-side (`pass→'OK' | fail→'needs attention' | running→'testing…'`), so adding `Summary` to `SetupCheck` would put display copy in the API for no new information — the contract-1:1 rule. The real gap was narrower than the row implies: `ConnectionBlock` ALREADY rendered the verdict line, but only inside the open body, so a **collapsed** block showed a status dot and nothing else. A dot cannot separate "failed" from "never tested" for anyone who does not read colour. ⚠ **`skipped` is NOT done and should not be done blind**: `routes/wizard.tsx:33` deliberately keeps it as component state ("a record of THIS operator's choices, not a location"), and `/v1/setup/state` returns only derived `{bootstrapped}` — persisting it needs a new per-operator record and a maintainer decision, not a patch. Only `library` and `users` are skippable, both optional, so the loss on refresh is cosmetic. |

The lane split and the corrected dependency graph are `docs/engineering/v2-build-plan.md` §6.2.

⚠ **The plan table was wrong in five places and was corrected doc-first before any code.** Four
dependency declarations did not match the code (V17b needs V30 for a byte route; V17d needs V29b
or it ships a coverage UI with nothing behind it; V33's V17c/V30 deps are unjustified and its real
dep on `internal/clipfetch` was unrecorded), and **V29's gate was unbuildable** — "consumes
`internal/filler/ladder.go`" when every symbol in that file is unexported. V29 is now V29a (export)
+ V29b (meter). This is the same staleness §6.1 records for V28; the lesson holds that a phase row
written months before the phase describes the plan's memory of the code, not the code.

`scope.collections` shipped end to end (engine filter → list endpoint → picker), which closes
**all three** of the surface audit's leftovers. The one outstanding item there is a live check,
not a build: the `ParentId` membership query below.

⚠ **`main` carries frontend changes that never got a full CI pass** (2026-07-31). PR #146's run was
cancelled when #147 merged 21s later, and #147 was Go-only so the path filter correctly skipped the
FE jobs. `make fe` was green locally, but **`fe-visual` has never seen** #146's deliberate
`StatusDot` shade change (Running dots `bg-signal` → `bg-signal-400`, plus a pulse). The first FE PR
after this — V17b — will surface it as a baseline diff. That is expected, not a regression.

⚠ **Two things about that row a future reader should not have to re-derive.** First, this was
recorded three times as *"blocked on a missing endpoint"*, and that framing was wrong in the
way this file keeps predicting: the endpoint was the easy half, and the load-bearing half was
that `filterEntries` is a pure no-I/O function, so membership had to be **stamped on the entry
and healed at reconcile** rather than looked up in the filter. Second, `LineupEntry` already had
a `CollectionID` — the TMDB **franchise** id — so the new field is `BoxSetIDs`; the two share an
English word and nothing else, and comparing them type-checks while matching nothing.

✅ **The `ParentId` assumption HELD — verified live against the maintainer's Emby 4.10**
(2026-07-30), which also retired `scripts/capture-collections.sh`: running the app answered
every question the script existed to ask, so keeping it would leave a second, staler source of
truth. `ParentId=<boxset>` enumerates members (109 for a 109-title collection) and each carries
full `ProviderIds`, so membership maps onto a `provision.Key` with no second lookup — the good
case the design assumed. ⚠ **`ChildCount` is never returned by Emby**, even when requested via
`Fields=`, so it is optional on the DTO and the UI renders around its absence.

⚠ **The live run also disproved the premise the UI was built on**, which no test could:
collections are NOT "a small closed set the operator curates by hand". A real library returns
**125**, ~90% auto-generated one-per-franchise ("Alien Collection") because Kometa writes them
in bulk. The checkbox list that shipped was taller than every other control on the Programming
page combined — and its `sr-only` legend broke the page layout outright. Both fixed in
`99ab305`; the lesson is in the row below, since "run it against real data" is the only thing
that catches a premise error.

*(V10 shipped — it was still listed here as "next" for several phases after its own row said
`done`, which is the kind of drift a session-start ritual reads as truth. Anything named here
must not have a `done` row above.)*

| — · Tasks page: River engine + descriptions, pause, open errors, run feedback | **done** | `230b0cd` + `d2c375c` + `2ee30a3` + `7cba4d7` — `make check` GREEN (0 lint, `-race`) + `make test-pg` (migration `00022` conformant on both) + `openapi-verify` / `config-docs` / `retired-verify` no drift + `make fe` (**876 app / 43 core**) + **fe-visual 592** (10 new baselines, **0 modified**, zero `Error: axe`) over an update run then a clean PLAIN run + `make e2e` (7); integration suite 4/4 clean under `-race`. **Every feature verified in the browser against the real install.** | Maintainer request, five parts. **River (§14, doc-first) replaces the hand-rolled scheduler**; `scheduled_jobs` stays the page's read model, because River's tables are keyed by job EXECUTION and rebuilding "this named task's state" from a growing attempt history on every page load is work already done once per run. ⚠ **`cron.ParseStandard` would have rejected EVERY schedule this install has** — River's docs point at it, it is 5-field, and Loomarr's are 6-field seconds-leading; those values are operator-editable settings already in the database, so the documented example fails saved schedules at boot on installs that were working. Caught in a spike before any code (`docs/engineering/FINDINGS-river-spike-2026-07-30.md`). ⚠ **River's schema is applied by `rivermigrate` at boot, never its CLI** — two migration LIBRARIES is a stated cost; two migration SYSTEMS an operator must run would not be shippable. ⚠ **THE BUG THAT COST MOST WAS MINE:** `Trigger` inserted with `ScheduledAt: now+10ms`, and River's two paths are 5s apart (AVAILABLE → fetch poll ~100ms; SCHEDULED → `JobSchedulerIntervalDefault = 5s`). Every Run-now took **~4.97s** and it made `TestScanAvailability_NoWebhook` — the proof that authorized deleting the inbound webhook — flaky. ⚠ **I diagnosed it WRONG first**, blaming SQLite connection contention and "fixing" the fetch poll, which changed nothing: the consistent ~4.97s should have said *fixed interval*, not contention. Variable latency is contention; clockwork is a ticker. 4.97s → 100ms, pinned by a regression test. ⚠ **A REAL GOROUTINE LEAK, caught by V13's goleak test:** River's shutdown issues queries and `defer st.Close()` closed the pool underneath it, stranding 4 goroutines per generation across every §9.2 in-process restart. Fixed with `store.OnClose` so ordering is enforced by the thing being protected. **Pause is a COLUMN on `scheduled_jobs`, not a settings key** — the registry needs every key declared, so 12 static entries would duplicate the job registry and a later job would 404 on pause; ⚠ `paused` is deliberately ABSENT from the upsert's `DO UPDATE` list, or every run would clear it (the `env_override` shape from §3.1). **Run now still works on a paused job**: pause stops the schedule, not the task. ⚠ **"I clicked Run and nothing happened" was REAL** — 202 in ms, server `running` true for ~250ms, both closing before the refetch; sampled 120× over 6s and saw exactly ONE state. ⚠ **The sticky header took THREE failed attempts**, each measured and each wrong (`border-collapse` paints cell backgrounds over the section's; moving it to `th` leaves the gaps), before moving the header out of the scroller entirely. Extracted `ErrorDetails` / `RunButton` / `useRunFeedback` for the next surface of this shape. |

| — · The picker was the wrong control, and it broke the page | **done** | `99ab305` + `2fae5af` — `make fe` GREEN (**860 app / 43 core**) + **fe-visual 582** (2 new, 4 modified, zero churn elsewhere) over an update run then a clean PLAIN run + `make e2e` (7); every claim re-checked in the live browser | **Three defects, none of which any test could have found — all three came from running the app against a real Emby.** ⚠ **(1) An `sr-only` `<legend>` made the WHOLE WINDOW scroll.** `sr-only` is `position:absolute`, and an absolutely-positioned `<legend>` escapes its fieldset's containing block: it laid out at y=1253, dragged `documentElement.scrollHeight` to 1254 against a 900px viewport, and content slid past the end of the FIXED sidebar — the nav ending mid-screen over dead space. The shell is `h-screen overflow-hidden` and **every element in the chain measured exactly 900**; one escaping absolute child defeated all of it. Diagnosed by walking the DOM in Playwright rather than by reading CSS, and confirmed by deleting the node live (1254 → 900). ⚠ **(2) 125 checkboxes is not a control.** The design note said "a small closed set the operator curates by hand" and the stories used three; Kometa generates a franchise collection per movie series, so a real library returns **125** — ~112 auto-generated, ~13 that carry a decision. Bounding it in a scroll box only contained the mess (the maintainer's call: *"regardless if there's a scrollbar, [it] is just not a good UI experience"*). Now the SAME type-ahead + chips as `ChannelSeriesScope`: collapsed field **32px, from ~1400**, curated ranked above franchise, franchises labelled but never hidden (filtering them server-side would drop a shelf someone deliberately named that way on a NAME heuristic). ⚠ **(3) Escape did nothing**, in **four** consumers — `SearchCommand`'s own comment listed three and omitted `proposal-edit`. It says the consumers "each own a Cancel affordance", which is exactly the reasoning that let this ship: a Cancel BUTTON is a POINTER affordance, and **every test clicked Cancel**. Now an opt-in `onEscape`, handled BEFORE the empty-list guard (a picker showing nothing is when you most want out), with a control case pinning that Escape is NOT swallowed when the prop is absent — the ⌘K palette binds it window-level and would close twice. Sabotage-verified: an unconditional binding fails that one test and only that one. Also made `placeholder` a prop; the hardcoded "Search titles, channels, help…" described the palette and this searches none of it. |

| — · `scope.collections` — endpoint + picker | **done** | `c730ab6` (API) + `8463a55` (UI) — `make check` GREEN (0 lint) + `openapi-verify` (1 new route, register-parity green) + `make fe` GREEN (**853 app tests**) + **fe-visual 580** (6 new baselines, **0 modified**, zero `Error: axe`) over an update run then a clean PLAIN run + `make e2e` (7) | The door onto the engine filter below. `GET /v1/library/collections` is **a LIST, not a search** — collections are a small closed set the operator curates by hand, so there is nothing to narrow; that is also why it is not a new `scope` on `/v1/search` (a Candidate models a provisionable title and a collection is not one, the same distinction that keeps clips off that route). Member-readable, matching search: read-only, shows no more than the media server already shows that user. ⚠ **An empty result serializes as `[]`, never `null`** — "you have no collections yet" is a real answer the picker renders as an empty state. ⚠ **The list deliberately does NOT share the membership index**, which is keyed for lookups and TTL-stale by design; an operator who just made a collection and opened the picker expects to see it. **FE is a CHECKBOX LIST, not a search box like its neighbour** — the shape of the data, not a style choice, and it is what lets the empty state say something true. ⚠ **A real bug the test caught and review did not:** the 501 check read `query.data?.status`, but a 501 is an error response and never populates `data`, so the component fell through to its EMPTY state and told an operator with **no media server** to "make a collection in your media server". Fixed to read the thrown `ApiError` (the detection `ChannelIconField` already used); sabotage-verified. A load FAILURE is also split from "you have none", for the same reason. ⚠ **One test of mine proved nothing**: the anonymous-refusal case passed against a sabotage setting the route to `RoleAnonymous`, because middleware 401s a token-less request *before* the per-operation role is consulted — it was asserting the middleware. Kept, but the marker is now pinned by a member-vs-403 test that DOES fail when flipped to `RoleAdmin`. Both API parity guards verified failing too. Every new baseline was READ, not just green. |

| — · `scope.collections` binds (backend) | **done** | `8e9eced` (+ `d04a5e1`/`3af14c5`) — `make check` GREEN (0 lint) + `make test-pg` GREEN + `openapi-verify` / `config-docs` / `retired-verify` no drift; three load-bearing claims each verified failing by sabotage | The last audit leftover, at the engine level. Restricts a channel to titles the operator curated into a media-server collection — the one scope field whose membership is a human judgment rather than derived metadata. ⚠ **"Blocked on a missing endpoint" was the wrong frame**, recorded three times: the endpoint is the easy half. `filterEntries` is a pure function over `[]LineupEntry` with **no per-reconcile library I/O**, and that is load-bearing for scheduling latency (the guide fight was 1910ms → 103ms and the first fix removed exactly this N+1 shape) — so membership is **stamped on the entry and healed at reconcile**, like `OfficialRating` and `CollectionID` before it. ⚠ **`LineupEntry.CollectionID` was already taken by the TMDB FRANCHISE id**, so the new field is `BoxSetIDs` in the media server's own vocabulary; a franchise is a TMDB integer, a BoxSet is an opaque server-local string, and a filter comparing them type-checks while matching nothing. ⚠ **The tri-state does not transfer, and that is the trap.** `-1` works as "resolved standalone" for an int; for a slice, "not looked up" and "in no collection" are BOTH empty, so a heal guarded on emptiness re-fetches the **most common case** every pass forever — the N+1 back through the door. Hence an explicit `BoxSetsResolved bool` (nil-vs-empty does not survive a JSON round trip). Sabotage: the plausible `len(BoxSetIDs) == 0` guard fails the counting test with a message naming the fix. ⚠ **Fails OPEN, unlike audience** — an unresolved entry still airs, because a media server that is down must not silently empty a channel to dead air; scope is a taste filter and §4 owns the fail-closed rule, pointing the other way. ⚠ **The reverse index is built once, not queried per key** — the per-key question does not exist on the wire (the server answers "what is in collection X", never "what collections is item Y in"), and membership is indexed under EVERY key form via `reconcile.ScanItemKeys`, **exported rather than copied**, since a member with both provider ids must be findable under whichever key the entry was born with (the same parity problem the availability scan solved). Both inert-field tests from `3af14c5` were rewritten as the positive assertions their own failure messages instructed — the pins did their job and were replaced, not deleted. **Still assumed:** `ParentId` as the membership query (see the ⚠ above). Doc-first: programming-design §2.2 (new) + §2 + §10; design §6. |

⚠ **The other two leftovers were narrower than this list claimed, and both shrank on being
traced rather than scanned.** Worth recording, because the note was planned against three times
before anyone checked:

- **The draft preview** was never "built and called by nothing". Three of the four preview
  endpoints were wired; the unused one is the *generalized* whole-definition preview, built for
  a review-before-apply flow the Programming page had not adopted. Adopted now (§12's third
  review-before-apply surface). ⚠ `programming-design.md` §336's claim that it is "the one call
  the Programming surface uses while editing" described a **whole-page** draft §12 never
  sanctioned — a companion contradicting the design doc, corrected in that PR. That one sentence
  is what made the endpoint read as unwired.
- **`policy.window`** is not unreachable. It is in `api/openapi.yaml`, on the generated
  `ChannelPolicy`, settable through `PATCH /v1/channels/{id}` (which takes the policy directly),
  and exposed **per rule** in the rules editor (`how.window`). The engine resolves
  `rule.Window → policy.Window → ch.DefaultWindow`. What is missing is only the *channel-level
  default* control — arguably deliberate, since the design puts the knob on the rule. ⚠ An
  earlier note here called it "backend-only, not on the DTO"; that came from grepping `"window"`
  with quotes, which misses the YAML key form.

⚠ **V16 discharged the gate's `restartFacts[0]` clause as DOCUMENTATION, not UI** — the restart
dialog is V13 and does not exist yet. §9.1 now records the honest version, which is
**per-backend rather than a flat reversal**: ffmpeg is a child of Loomarr (`Setpgid`, killed by
process group), so a restart drops every internally-played stream, while Tunarr-backed channels
genuinely do keep playing as the old copy said. Since `playout.backend` is per channel, an
install can have both — and V13's dialog can now read the live session count from
`GET /v1/playout/sessions` to say which.

⚠ **Read the v2 mocks before any IA work**, not §12's tables — §12 records the *shipped* IA and
the v2 nav differs (see the ⚠ under §12's nav table). This cost a wrong turn in V27.

**A `fe-visual` "flaky" line is the harness working, not a defect — don't chase it.** One run
reported **3 flaky** on `People/UserRow` (Imported/Local/Self, mobile); it passed on retry, passed
on a clean re-run, and rewrote no baseline. Investigated rather than left as a breadcrumb:

- It does **not** reproduce in isolation — 36/36 across three `--grep UserRow` runs. The component
  has no images, no fetches, and no time-dependent rendering, so its inputs are not the cause.
- `playwright.shared.ts:14-17` already documents exactly this: *"Residual sub-pixel text-AA jitter
  can nudge a rare shot past the strict ratio. Retries de-flake that WITHOUT masking real
  regressions — a genuine diff reproduces and still fails every attempt."* With
  `maxDiffPixelRatio: 0.001` and `fullyParallel: true`, a rare shot under parallel load losing that
  bet is expected.

So **`flaky` (passed on retry) is green**; a real regression fails all three attempts. What still
warrants attention is a spec that flakes *repeatedly across runs*, or any baseline that changes
without an intended visual edit. Distinct from the `help.test.tsx` **unit** flake, which is a real
open issue (~1 in 4, documented above the test, filed for V15).
**The spine is COMPLETE:** V3 → V4 → V5 → V6 → V6b → V13b → V14a → V14b.

**V14 was split, revisiting D-D.** The plan bundled the IA rename with the grid; the plan's own
sequencing note names splitting as the escape hatch. Bundling would have put the rename's mechanical
churn and the grid's new pixels in ONE visual-baseline diff, where neither can be reviewed
independently. Both halves have now landed (V14a, V14b).

**Two pieces of the v2 nav are deliberately NOT done**, and are additive whenever they are wanted:

1. **`Dashboard`** — belongs to V16. A nav entry pointing at a placeholder is worse than no entry.
2. **Folding `Channels` into `Guide`** — the mock's intent and the right end state, but origination
   ("Add a channel") still lives on the list and the grid has no affordance for it. Removing the list
   today would strand the everyday way a channel is made. Both doors are real; neither is a redirect.
   `Suggest` stays in the admin nav for the same reason. Recorded in §12 so the remainder is visible.

⚠ **Two mock-reading lessons, recorded because both cost real work.** The v2 prototypes are
`design/loomarr-prototype-desktop-v2.dc.html` (502KB, 2026-07-24) — NOT the 146KB July-13 file. I
read the old one, concluded no grid design existed, and built one from scratch; the v2 mock had a
complete TV Guide, and `design/SYNC-LOG-2026-07-24.md` says so outright. And the mock's amber is the
`signal` token (`#FFB020`); `onair` is the RED live-dot (`#E5484D`). Mapping them backwards made the
now-line and every channel number the wrong colour, which no assertion catches — only a screenshot.

### Playout: five traps that each cost a live channel

Every one was found by RUNNING it, and four had green tests over them the whole time. A test can
only assert what you already believe.

1. **Probe the machine you will RUN on.** Verified `h264_vulkan` on the host, set
   `PLAYOUT_ENCODER` for the *container* — which had no `/dev/dri` and no driver libs. Every
   encode died instantly and the log said **nothing**, because a dying encoder closes stdout,
   which reads as a clean EOF. Fixed the guard (`n == 0` regardless of error) and the image
   (one vendor-neutral driver package set, ~120MB, serving Intel/AMD/Vulkan; NVENC needs nothing
   in-image — `libcuda` is injected by the container toolkit).
2. **`format=yuv420p` for every CPU-frame encoder, not just software.** A 10-bit HEVC source
   reaches nvenc as `yuv420p10le` and it refuses with *"No capable devices found"* — a message
   that names the DEVICE, so it reads as a missing GPU.
3. **DECODE dominates on 4K, not encode.** CPU decode + GPU encode measured 341% CPU, *higher*
   than all-software (260%), because the decode had merely been throttled by the slow encoder.
   Adding `-hwaccel cuda` → 42%.
4. **Never `-hwaccel_output_format`, and keep scale/pad on the CPU.** `scale_cuda` has **no pad
   option** (verified: a 4:3 source emits 1440x1080, not letterboxed 1920x1080), which breaks the
   parent's `-c copy` on any channel mixing aspect ratios.
5. **Hardware CAN do quality-targeted rate control.** Capped CBR crushed hard scenes; a comment
   in our own code claimed hardware could not use a quality target. Wrong about NVENC —
   `-rc vbr -cq 21 -b:v 0` with a 2× ceiling took SSIM 0.98262 → 0.98581. `-b:v 0` is
   load-bearing: a non-zero bitrate degenerates cq back to CBR.

**Guide display, learned by two wrong answers against the real Emby.** XMLTV says `<title>` is the
series and `<sub-title>` the episode. Emby parses both correctly but its guide GRID renders
`Name` alone — so episode-in-title showed *"Bart the Mother"* with no show, and series-in-title
showed *"The Simpsons"* with no episode. Now combined (`"The Simpsons: Bart the Mother"`), with
`<sub-title>` still emitted for clients that use it. Tunarr can afford the strict split precisely
because it emits `<desc>`; once ours landed, the constraint changed — revisiting is reasonable.

**Pattern worth noting across V1/V17a/V23:** each was a *complete* feature with one missing link —
a built component nobody imported, a sidecar nobody parsed, a persisted field nobody captured. All
three passed their own unit tests. The gates that catch this class are reachability assertions and
prompt capture, not more component tests.

**Not scheduled, need a design decision first:** C5, C6 — the v2 mock shows **no UI for either**,
so there is nothing to port. Each violates §12's surface-map rule; the honest options are *add the
UI*, *remove the capability*, or *document it as API-only*.

⚠ **This line said "C5, C6, C8" until 2026-07-31, and C8 was answered long before that** — recorded
as API-ONLY BY DECISION in the fold entry above. A stale "needs a decision" note is worse than no
note: it sends the next session to re-litigate a settled question, which is the same failure the
worktree section of `CLAUDE.md` warns about in its own text.

## Environment (recorded Phase 1; verify with `docker info`, `go version`)

| Prereq | State (2026-07-13) | Note |
| --- | --- | --- |
| Go | `go1.26.5` ✓ | Design requires 1.22+; sibling `nexus-open` uses `go 1.26.0`. |
| Node | `v26.4.0` ✓ | Design requires 20+. |
| Docker daemon | **active** ✓ (Server 29.6.1) | Started + `enable`d 2026-07-13 (`systemctl is-enabled` → enabled). Compose v5.3.1. **Hard requirement from Phase 4** — now satisfied. |
| make | GNU Make 4.4.1 ✓ | |
| goose | not on PATH | Installed as a Go tool dep in Phase 1/3 (`go run`/tool), not required on PATH. |

## Project facts (design doc §20 — resolved)

- **Module path:** `github.com/mantonx/loomarr` — matches sibling convention `github.com/mantonx/nexus-open`.
- **License:** MIT (matches `nexus-next`).
- **House conventions to mirror:** `nexus-next` Makefile verb style, `.golangci.yml`, `modernc.org/sqlite` driver.

## Phase-0 findings (fill during contract spikes — this is the pinned evidence)

> Per §21 phase 0 and CLAUDE.md "Ask the maintainer": if any contract deviates from
> §6/§9, **stop and update `loomarr-design.md` first**, then proceed.

- [x] **Tunarr** (local `chrisbenincasa/tunarr:latest` → **v1.3.8**, ffmpeg 7.1.1, node 22.20.0). Spec vendored to `api/vendor/tunarr-openapi.json` (OpenAPI 3.0.3, 117 paths). Throwaway channel CRUD exercised: create 201 → GET 200 → DELETE 200 → gone (404). **API-key question SETTLED: no key required** — spec has empty `securitySchemes`/`security`, unauth reads+writes succeed; `TUNARR_API_KEY` confirmed optional for 1.3.8. Findings: `internal/testkit/fixtures/tunarr/FINDINGS.md`. **Contract surprise: server assigns channel `id`; client-supplied id is ignored** (Phase-10 adapter must use the create-response id).
- [x] **Sonarr** (v4.0.19.2979): `Test` webhook captured verbatim → `internal/testkit/fixtures/sonarr/test_webhook.json` (minimal placeholder shape per §6 — has junk `series`/`episodes`, handler must not resolve a real title from Test).
- [x] **Radarr** (v6.2.1.10461): **full lifecycle captured live** — `Test`, `Grab`, `Download`(import) → `radarr/{test,grab,import}_webhook.json`. Findings: `internal/testkit/fixtures/FINDINGS-arr-webhooks.md`. Confirmed §6 quirks: import event `eventType` is the string **`"Download"`**; `downloadId` correlates Grab↔Download; identity key is `remoteMovie.tmdbId`; upgrade import has `isUpgrade:true`+`deletedFiles`. Method: forced re-grab of *In Flames* via `POST /release {guid,indexerId,movieId}` → real SABnzbd download → real import webhook. Temp webhook conn (id 3) **removed** (verified: only Emby+Mail remain).
- [~] **Sonarr** `Grab`/`Download` not yet captured (only `Test`). Structure mirrors Radarr with `series`/`remoteSeries.tvdbId`; capture at Phase-6 start via same method or Sonarr history. Not blocking.
- [x] **Seerr codebase cross-check** (maintainer lead, `github.com/seerr-team/seerr@develop`): reviewed `server/api/servarr/{base,radarr}.ts`. Reference note: `internal/testkit/fixtures/REFERENCE-seerr.md`. Key takeaways: Seerr add-movie is check-then-act idempotent (`getMovieByTmdbId`); Seerr passes arr key as `?apikey=` query param — **we deliberately diverge, keeping `X-Api-Key` header** per §6. Seerr's connection-leak issue (#2297/#2303) is live evidence for §6's mandatory per-service timeouts.
- [x] **Media server — Emby** (v4.10.0.17): full authed round-trip captured → `internal/testkit/fixtures/emby/{system_info_authed,users_list,lookup_present,lookup_absent,auth_badpw_response}.json` + `FINDINGS.md`. `X-Emby-Token` header auth 200; `AnyProviderIdEquals=tmdb.<id>` presence check works (present→Items[1], absent→Items[], both 200); casing case-insensitive on 4.10 (lowercase per §6 anyway); `/Users` gives `Policy.IsAdministrator/IsDisabled` for §11; `AuthenticateByName` bad-pw→401. **Seerr cross-check (maintainer lead, `jellyfin.ts`):** Seerr uses ONE unified `Authorization: MediaBrowser … Token=…` header for both flavors — verified it also returns 200 on Emby. Recorded as a §6 *option* (single code path), not taken without a doc-first change.
- [x] **Seerr** (v3.2.0) requester → `internal/testkit/fixtures/seerr/{request_available_201,request_repeat}.json` + `FINDINGS.md`. `POST /api/v1/request` with `X-Api-Key`. **Refinement (not a deviation):** re-requesting an available/duplicate movie returns **201 with existing media** (not 409), `downloadStatus:[]` (nothing new queued). §6's "201/409=success" holds, but Phase-6 tests must assert "2xx or 409", never *require* 409 on an available title.
- [ ] Deviations from §6/§9 (if any): **none so far** — Tunarr id-assignment + lineup-needs-programming are details the doc deferred to Phase 10, not contradictions. No `loomarr-design.md` edit required yet.

### Dev infrastructure (persistent — keep running through Phases 10–12)

- **`tunarr-dev`** — persistent local Tunarr for developing the Programmer adapter against.
  Compose: `docker/compose.dev.yaml` (image `chrisbenincasa/tunarr:1.3.8@sha256:88122a21…`,
  volume `tunarr-dev-config`, `restart: unless-stopped`). Up at `localhost:8000`. Setup +
  Emby-wiring steps: `docker/tunarr-dev-setup.md`.
- **Emby media source wired** into `tunarr-dev` (id `fa31064d-50e1-405d-9edc-3364e0754ffd`,
  `{"healthy":true}`) — sees 7 Emby libraries incl. **Movies** (`childType:movie`) and
  **TV shows** (`childType:show`). Phase-10 note: `POST /api/emby/login` only returns a token;
  the source is a *separate* `POST /api/media-sources` carrying it.

### Temp homelab state to revert after Phase 0

- Radarr webhook connection `loomarr-phase0-capture` (id **3**) — DELETE via `/api/v3/notification/3` once import captured.
- Local Tunarr spike container `tunarr-spike` (+ volume `tunarr-spike-data`) — `docker rm -f` when done.
- Forced re-grab of *In Flames* landed a fresh file on the media server (expected; a re-download of an owned title).
