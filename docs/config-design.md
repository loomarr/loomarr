# Loomarr Configuration — Subsystem Design

**Status:** Implemented (settings subsystem PR, 2026-07-15) · companion to `design.md`
**Precedence:** the main doc (§13, §15) is authoritative for the *decision* — settings live in the app, the environment pins, `env > database > default`. This doc is authoritative for the configuration subsystem's *design*: the registry, resolution semantics, secrets lifecycle, Settings UI, and onboarding integration. Conflicts → main doc wins on policy, this doc on mechanics; fix the loser in the same PR.

---

## 1. Lineage & the split

Loomarr's model is the *arr convention with one addition:

| App | Where settings live | What we take |
| --- | --- | --- |
| **Sonarr/Radarr** | UI settings + generated API key shown in-app + per-page "Show advanced" toggle | The IA: grouped pages, advanced toggle, explicit save bar, API key in the UI |
| **Seerr** | Wizard-driven; settings persisted by the app | Onboarding *is* configuration: wizard steps are settings forms with live tests |
| **Loomarr** | Both of the above, **plus deterministic env pinning** | GitOps users pin any key via env; it wins and locks the field with provenance |

**The classification rule (so future keys self-classify):** a setting is **bootstrap** iff it is needed *before the database opens* or describes *process topology* — `DATABASE_URL`, `AUTO_MIGRATE`, `LISTEN_ADDR`, `LOG_LEVEL`, `TZ`. Everything else is app-managed and env-pinnable.

**The bootstrap FILE tier (revised — V5).** Bootstrap keys were env-**only**, because the registry lives in the database they are needed to open. That reasoning is unchanged; what it did not anticipate is the wizard needing to **write** one. The Database step (§13) asks "SQLite or PostgreSQL?" and must persist the answer — it cannot write into the database it is choosing, and it cannot set an env var that survives a restart. A file beside the database is the only writable store that exists before the database does. So:

`env > file > default` — **bootstrap keys only**
`env > database > default` — every app-managed setting, unchanged

- **Env still wins.** A GitOps pin is never overridden by something the wizard wrote; the wizard reports the key as pinned instead, the same contract the registry's `pinned` provenance already gives the Settings UI.
- **The file holds bootstrap keys ONLY.** It is not a second settings store. An app-managed key there is a *category* error, not a typo — it would create two places to look for one answer — so it is rejected by name at both read and write, and the error points at Settings.
- **Absent file = today's behaviour exactly.** This tier adds a lookup, never a requirement.
- `bootstrap.json` lives in the data directory (beside the SQLite file), written atomically at `0600`: it decides where the database *is*, so a half-written one would leave the next boot unable to find its own data, and `DATABASE_URL` routinely carries a password. Secrets the app can mint itself (`SESSION_SECRET`, `API_TOKEN`, `PLAYOUT_TOKEN`) are **generated**, never demanded.
  - ⚠ **The file is SEARCHED across two directories, and that is load-bearing** (V11). `DataDirFor` is scheme-dependent — the SQLite file's own directory for `sqlite://`, the conventional `/data` for `postgres://` — so a SQLite→PostgreSQL migration *moved where the next boot looked for the file*. With the database anywhere other than `/data`, the file recording the switch was written beside the SQLite database and then never read again: the app booted back onto SQLite, having apparently migrated successfully. The switch silently undid itself. Reads now try the database's own directory first (an operator who pinned a SQLite path meant that one), then `/data`; writes are unchanged, so an existing install's file keeps being found and nothing has to move. A malformed file still fails the boot rather than falling through to the next directory — a file that exists and is wrong is an operator error to surface, not a reason to quietly use a different one.

**The per-channel tier (added with `programming-design.md`):** programming heuristics introduce settings that vary *per channel* — the ChannelPolicy (scope, audience ceiling, separation windows, ordering, seasonal mode). These are **not registry settings**: a policy instance is channel *data*, stored on the channel row, edited in proposal review / the channel editor, and never env-addressable. What the registry holds is their **global defaults**. Full precedence, per key:

`channel policy > registry default (env-pinnable per the normal rule) > built-in`

The test for which tier a new knob belongs to: *would two channels sensibly want different values?* Yes → policy field with a registry default. No → plain registry setting.

**Self-updating channels (`programming-design.md` §8.2) follow this tier exactly.** A channel opts into scheduled re-curation via a per-channel `policy.autoCurate` field (rides `policy_json`, no schema change — like `rules`/`filler`/`window` before it), and the two thresholds it's bounded by have the classic split: the global defaults `recurate.min_score_pct` (the quality bar a net-new title must clear) and `recurate.max_titles` (the growth cap) are **registry settings** (env-pinnable, hot-applied per call), while `policy.autoCurate` may carry a per-channel **override** of either. `job.recurate.schedule` is a plain global `KindCron` job knob (all channels re-curate on one clock). The registry values are read live inside the re-curation grant, so raising the fleet-wide bar takes effect on the next run with no restart.

---

## 2. The registry — every setting declared exactly once

```go
type Setting struct {
    Key      string        // "library.url"
    EnvVar   string        // "LIBRARY_URL" — the pin
    Group    Group         // connections.media_server | connections.requester |
                           // connections.tunarr | ai | channels | filler |
                           // users_security | advanced
    Kind     Kind          // string | int | bool | duration | url | enum | secret | string_list
    Default  any
    Enum     []string      // Kind == enum
    Secret   bool
    Advanced bool          // hidden behind the per-page "Show advanced" toggle
    RequiredFor Feature    // "" | acquisition | suggestions | filler  — feature gating (§7)
    Validate func(any) error      // shape validation (URL normalization lives here)
    Test     *ConnectionTest      // optional live test — powers wizard, checklist, Test buttons
    Doc      string               // one-liner: UI help text + generated docs
}
```

**The registry is the single source of truth**, with the same committed-artifact discipline as OpenAPI: `make config-docs` generates `docs/configuration.md` (grouped tables, env names, defaults, docs) and CI fails on drift (`git diff --exit-code`). The main doc's §15 table is a human mirror; the generated file is the contract. A setting that isn't in the registry does not exist — validation, the API, the UI, the wizard, and the docs all derive from it.

---

## 3. Resolution & runtime semantics

`resolve(key)` per read: **env → db → default**, with asymmetric error handling that matches who made the mistake:

- **Invalid env value → fail the boot**, loudly, naming the variable and the expected shape. An operator typed it; silence would hide their error behind mystery behavior.
- **Invalid db value → log a warning, fall through to default, surface a caution chip on the field.** The app wrote it (or a migration drifted); self-heal and tell the human, don't crash a running install.
- Env **unset after previously pinning**: next boot, provenance flips to `db` (or `default`) and the field unlocks. Pinning is a live property of the environment, not a ratchet.
- Env **set but empty** (`LLM_MODEL=`) counts as **unset**, and resolution falls through to db → default. A blank assignment is what an unfilled `.env` template looks like, not a decision to pin a setting to the empty string — and the two are indistinguishable to us, so we side with the reading that cannot silently destroy an operator's saved value. Treating it as a pin produced exactly that: the §8.1 picker wrote `llm.model` to the db and hot-swapped the live suggester, while every *read* still resolved to the empty env pin, so the checklist reported "no model selected" immediately after one was, and the choice vanished on the next restart. Consequently **there is no way to pin a setting to the empty string via env** — deliberate: "unset" and "set to nothing" mean the same thing to an operator, and clearing a value is what the db layer (and the Settings UI) is for.
- **Boot logs every empty pin it ignored**, at WARN, naming the variable: an operator who *meant* to blank something must not have to infer that we disregarded it.

### 3.1 Taking a key back from the environment (the unlock)

An env pin is normally the one thing an admin cannot change from the UI, and §2's tagline —
*"it wins and locks the field"* — is why. That is right for GitOps and wrong for the case that
motivated this: an operator who put a value in `.env` to get the app booting, then reached the
wizard and found the field they needed to correct was read-only, with the only documented way
out being *edit a file on the host and restart*. On an appliance-style install that is not a
workflow, it is a dead end.

So a key can be **explicitly taken back**: an admin unlocks it, and from that moment the stored
value wins **for that key** until it is handed back.

**This is a durable claim, not a session toggle — and that is the whole design.** Env is
re-read every boot, so an unlock that lived in memory would resolve back to env on restart and
silently discard what the operator saved. That is precisely the `LLM_MODEL` failure recorded
above (write succeeds, every read still returns env, the value vanishes on restart), and
shipping a button that reproduces it would be worse than the locked field, which at least tells
the truth. The claim therefore persists in the database (migration `00020`, `env_override`),
rides §16 backups like any other durable state, and survives restarts and redeploys.

**The rules:**

- **Unlock is per key.** Never global, and never implied by writing a value — an ordinary PATCH
  to a pinned key still returns `pinned` (§7). Taking a key from the deploy config is a
  separate, deliberate act.
- **Admin only, and audited.** It reuses `updated_by`/`updated_at`, so the field's existing
  *"changed by … · when"* line reports who overrode the environment. An operator inspecting a
  box that is not behaving like its `.env` must be able to find out why from the app.
- **Provenance stays the three-value enum** `env | db | default` (§5 already rejected a fourth
  chip). An unlocked key resolving to its stored value is honestly `db`. The *lock state* is a
  separate boolean on the entry — `envOverride` — so the UI can say **"overriding `SEERR_URL`"**
  rather than implying the environment is unset. Conflating the two would make an overridden
  key indistinguishable from one the environment never mentioned.
- **Re-locking is one click and loses nothing.** Clearing the flag returns the key to `env`
  precedence with the stored value still in the database, so handing control back to GitOps is
  reversible in both directions.
- **`DELETE /v1/settings/{key}` still 409s on a *locked* key** (§7). The explicit clear drops a
  stored override so a key falls back to env/default; on a pinned key that is a contradiction.
  On an *unlocked* key it succeeds and the key reverts to the env value — which is the correct
  meaning of "clear my override" once the operator has taken the key back.
- **Bootstrap keys are never unlockable.** `DATABASE_URL`, `LISTEN_ADDR`, `LOG_LEVEL`, `TZ`,
  `AUTO_MIGRATE` are read *before the database opens* (§2's classification rule), so a flag
  stored in that database cannot affect them. Offering the control would be a lie; the API
  rejects it rather than accepting a write that does nothing.
- **Unlocking SEEDS the stored value from the env value it is taking over, and does not change
  what the app is doing.** The alternative — unlock to an empty row — would make the act of
  unlocking *blank the setting*, so an operator who wanted to correct one character in a URL
  would instead knock the service offline the moment they clicked. Unlock is a transfer of
  authority, not an edit; the value changes on the next save, by the human, deliberately.
- **⚠ Secrets are the exception: they never seed.** Seeding would copy a credential out of the
  environment into the database and therefore into every §16 backup, quietly widening where that
  secret lives — a security change disguised as a convenience. An unlocked secret is `set:false`
  and the operator must enter one, which is the same replace-only flow §4 already defines and
  the only one that keeps "where did this credential come from" answerable.

**The honest cost, recorded rather than glossed:** a redeployed `.env` no longer fully describes
a running instance. That is a real loss for GitOps, and it is why the act is explicit, per-key,
admin-only, audited, and visible on the field — an operator who never unlocks anything keeps
exactly the old contract.

**Secrets via files:** every secret env var also accepts the Docker-secrets idiom — `LIBRARY_TOKEN_FILE=/run/secrets/emby` loads the value from the file (trailing newline stripped). `<VAR>` and `<VAR>_FILE` both set → boot error (ambiguous).

**Hot-apply (no restart to reconfigure):**
- The settings service holds an in-memory snapshot (RWMutex) refreshed on local write and, for Postgres replicas, on a ~30s read-through interval (main doc §17).
- **Connections read through per use:** the shared HTTP client factory (§6 main doc) fetches URL/token from the snapshot at call time — saving a new Emby token means the *next* lookup uses it.
- **Intervals re-read per tick:** tickers ask the snapshot each cycle; changing `CHANNEL_RECONCILE_EVERY` takes effect next tick.
- **Long-lived constructions rebuild on change:** the LLM client subscribes via `Watch(keys...) <-chan Change` and reconstructs. (This is the same seam the §8.1 model-selection hot-swap uses — an atomic-pointer provider that rebuilds on a persisted `llm.*` change.)
- `RestartRequired` exists as a flag for honesty and applies only to the bootstrap set. **Revised (V5): the UI now edits exactly one of them** — the wizard's Database step writes `DATABASE_URL` to the bootstrap file (above). That is precisely why the flag stops being decorative: the step that writes it is also the step that must say a restart is coming.

  **Revised again (V13): the flag is now computed, and there is a restart control to act on it.**
  It was a hardcoded `true` on the database switchover response — the only place that could set
  it — which made it a property of *one endpoint* rather than of the instance. It is now derived
  by comparing each bootstrap key's **running** value against its **resolved** value, so
  `GET /v1/system/restart` can name the specific key: *"You changed a boot-time setting
  (`DATABASE_URL`). Loomarr is still running the old value until it restarts."*
  ⚠ **Derived, never a sticky boolean someone remembers to set.** A flag written at the moment of
  an edit is wrong the moment the operator edits it back, and would nag about a restart that is
  no longer needed. Comparing running-vs-resolved cannot drift, because it re-reads the same
  resolution the app itself uses.
  The restart mechanism (in-process rebuild, no supervisor, no re-exec) is `design.md` §9.2.

**Audit:** the settings table carries `updated_at` + `updated_by` (nullable — env/migration writes have none). The UI shows "changed by Matt · 2d ago" per field; same spirit as `approved_by`.

---

## 4. Secrets lifecycle

**Generation:** 256-bit random, base64url, created idempotently inside the first-migration transaction (alongside the instance id).

**Display policy (Sonarr-model, differentiated by purpose):**
- `API_TOKEN` and `PLAYOUT_TOKEN` are *operational values you must paste elsewhere* — viewable on demand by admins (eye toggle + copy button), exactly like Sonarr's API key.
- `SESSION_SECRET` has nothing to paste anywhere — **never displayed**; the only affordance is Regenerate.
- Integration secrets you *entered* (Emby token, Seerr key, TMDB, LLM key) — masked after save (`set · …a1b2` preview), replace-only. The API returns `{set: true, preview, provenance}` — never the value. (The §8.1 hosted `llm.api_key.<provider>` keys follow this exact rule: stored, previewed, never echoed by any GET.)

**Regeneration side-effects (typed-confirmation dialogs, effects stated up front):**

| Secret | Immediate effect | UX contract |
| --- | --- | --- |
| `SESSION_SECRET` | **All sessions revoked — including yours** | Confirm → regen → redirect to login. `API_TOKEN` remains as break-glass, so you cannot lock yourself out. |
| `API_TOKEN` | Old token dead instantly | Show the new token once prominently; remind that machine clients/scripts must update. |
| `PLAYOUT_TOKEN` | Every media-server tuner stops — the M3U and XMLTV URLs carry the old token | Show the new `/playout/tuner.m3u?token=…` URL; the media server's tuner entry has to be updated to match, or Live TV goes empty. |

**Redaction is systemic, not per-callsite:** the settings service exposes a `Redactor` (the current set of secret values) wired into the `slog` handler — a secret value appearing in any log line is replaced before write. Secrets are excluded from `/v1/setup/status`, from validation error strings (validators must never echo the value), and from RFC 7807 bodies. There is a test that greps captured logs for a known secret and demands zero hits.

---

## 5. Settings UI — information architecture

Sonarr's shape, Test Card's skin (FE doc §6 provenance rules apply):

| Page | Contents | Live tests |
| --- | --- | --- |
| **Connections** | Media server (flavor · URL · token) · Requester (Seerr *or* direct Sonarr+Radarr) · Tunarr · TMDB. **No manual wiring actions** — connecting Tunarr to the guide and pointing it at the library happen *automatically on save* (see below). | one **Test** button per connection block → runs the same `ConnectionTest` the wizard uses; the `livetv` / `tunarr_library` outcomes surface on the Tunarr + Media-server block verdicts, since a save auto-runs `POST /v1/setup/{livetv,tunarr}-connect` server-side |
| **AI** | Model roles: lineup model/provider, filler vision/language models, suggestion safety limit, and auto-curation limits. The in-app model picker still owns probe/catalog/hot-swap. Approval remains per-person; there is no global auto-approve switch. | the tool-call **probe** (main doc §8) + `GET /v1/system/llm` (probe/catalog), `POST /v1/system/llm/test` (key validation) |
| **Defaults** | Only what a NEW channel inherits: ordering, no-repeat/separation policy, seasonality, rolling window, and filler break density. Filler ingestion/storage/automation live with the Filler workflow, not above these defaults. | — |
| **System** | The machine, not the product. Sub-tabs: **Tasks** · **Playback** (backend, quality, language/subtitles, detected encoder/capacity, guide, advanced paths) · **Database** · **Backup** (schedule, retention, destination, files) · **Storage** (image location, remote-artwork policy, upload/cache bounds) · **About**. “Playback” is the user-facing label for the `playout` domain. | per sub-tab where testable |
| **Security** | Session TTL · cookie mode · user-sync interval · **Generated secrets panel** (view/copy/regenerate per §4) · SSO once V8 lands | — |
| **All settings** | Every key, searchable by key **and** group **and** value, with an `ADV` chip reflecting `Setting.Advanced` (V10). The escape hatch: an operator who knows a key's name should never have to guess which page owns it. Rows are **editable in place** — see below. | — |

⚠ **This table was AMENDED (V9) to the v2 mock's structure**, and the change is a restructure
rather than a rename — worth recording because the previous six pages shipped and are what an
existing install has bookmarked:

| Was | Now |
| --- | --- |
| Channels & playback + Filler | **Defaults** (both answer "what does a new channel inherit?") |
| Tasks (added after this table was written) | **System → Tasks** |
| Users & security | **Security** |
| Advanced | **All settings** (searchable, not a dumping ground) |

The mock wins here despite `design/README.md` making the prototypes non-authoritative for IA:
that rule exists because a prototype's *structure* can contradict a considered decision, and
here the opposite is true — the v2 program's own phases are written against this shape (V12 is
literally "System → Backup UI"; V13's probes are System-shaped), so following the older table
would leave four phases describing pages that do not exist. **`design.md` §12 still wins on
behaviour**; this table owns the config surface's shape.

⚠ **All settings is EDITABLE, and this is the one place the v2 mock is not followed.** The mock
draws it as a read-only lookup. That works only if every key also has a home page — and the
restructure above broke exactly that: `GroupAdvanced` holds 19 keys (job schedules, TTLs, the
reconcile interval) whose only editor was the *Advanced* page that folded into this one. A
read-only table would leave all 19 uneditable with nothing on screen saying so, and the loss
falls in the seam between two phases that each look complete alone. So rows edit in place,
through the same `SettingField` every other page uses (a `compact` mode) and staging into the
same cross-tab buffer — env-pinned locking and the secret replace-flow (§4) behave identically
here. **The lookup half still governs presentation:** keys are monospace and verbatim, never
humanized, because someone arrives holding a literal `job.workers` from a compose file and a row
reading "Job workers" does not match the string they are carrying.

V55 closes the loophole that made this escape hatch the only home of ordinary settings. Every
non-advanced key has an owning workflow page; the raw table links to that page, carries the same
help and environment-takeover affordance as the full field, and exposes the explicit **Clear
override** operation. Editing remains available for genuine advanced keys and for operators who
arrive with a literal key, but the table is never the sole UI for a product decision.

Two consequences worth stating, because both are easy to "fix" wrongly later:

- **Three provenance chips, not the mock's four.** The mock adds `generated`, but generated
  secrets (`playout_token`, the API token) are a separate registry (`internal/settings/secrets.go`)
  and are not settings entries at all — the API's provenance enum is exactly env/db/default.
  A fourth chip would render a state the backend never sends. Those secrets have their own panel
  on **Security**, where view/copy/regenerate belong.
- **The compact control has no `<label>`, so it must carry `aria-labelledby`** pointing at the
  row's visible **Key** cell. Naming it by `title` alone is an axe `label-title-only` violation
  rated *serious*: a sighted mouse user gets a tooltip, a screen-reader user gets an unnamed text
  box. The visible label already exists on the row — it just has to be associated, not duplicated.

**Display semantics are registry-owned.** A setting carries its human label and presentation
(`plain`, `duration`, `bytes`, `cron`, `path`, `language`) beside its validation kind. Validation
still decides what may be stored; presentation decides how the same value is explained and edited.
This keeps `720h` as the stable wire/storage value while every UI says “30 days”, and keeps a byte
ceiling from appearing as an unexplained integer. Enum options remain
`[]EnumOption{Value, Label}` for the same reason. The UI may fall back to humanizing an unknown key
only in the raw escape hatch; workflow forms never derive product copy from identifiers.

**Conditional fields (`ShowWhen`).** A setting may declare `ShowWhen map[string][]string` — it is shown only when the *current* value of a named key is one of the listed values (empty = always shown). `llm.api_key` is hosted-only, while `llm.url` applies to both providers: it is the Ollama host for local AI and the OpenAI-compatible base URL for hosted AI. Hiding the local URL would make a non-default Ollama host impossible to configure. The UI evaluates conditions against live edits; a hidden field's value is untouched.

### V55 surface audit decisions

- Retired declared-but-unconsumed promises: `season.precision`, `playout.transport`,
  `suggest.auto_approve`, `sched.backfill`, `ingest.max_concurrent`, <!-- retired-ok -->
  `filler.starter_collection`, `reconcile.every`, and `event.webhook_url`. Reintroducing one <!-- retired-ok -->
  requires its consumer in the same change.
- Retired the remaining declared-but-unconsumed promises: the `sched.*` policy defaults and
  `seasonal.mode`, the global `playout.subtitles` default, and `user.sync_every`. Per-channel <!-- retired-ok -->
  programming policy remains the place to set ordering, separation, and seasonal behaviour;
  subtitle burn-in is not exposed until the encoder actually honours it; user import remains an
  explicit admin action until there is a scheduled consumer. A control that merely round-trips
  through the registry is not implemented.
- Image formats, remote concurrency, and the remote-artwork retention ceiling are implementation
  policy, not operator preference. AVIF/WebP/JPEG compatibility, the provider concurrency cap, and
  the six-month compliance ceiling are fixed by the image module. Operators control storage,
  outbound fetching, upload size, and derivative cache budget.

**Field anatomy:** label · control · provenance chip (`set via environment` = locked; caution chip on self-healed values) · one-line doc · Test button where testable · "changed by … · when". Two of these are *present but not permanently visible*, so a page of fields reads as controls rather than a wall of prose: the **one-line doc** lives in an `(i)` hover tooltip (kept in the DOM via `aria-describedby` for screen readers), and the **"changed by … · when"** audit line reveals on hover/focus of the field (kept in the DOM, opacity-toggled, so it's keyboard- and reader-reachable). The provenance chip, caution chip, and validation stay always-visible — they change *what the field is or does*, not merely its history.

**One curated home, with progressive disclosure.** `All settings` remains the searchable escape
hatch, but every key has only one task-shaped editor elsewhere. A control does not appear on both
an operational workflow and a generic Settings page: the workflow owns it when its effect is best
understood beside the affected work (for example, auto-filing beside Incoming clips), while service
and model configuration stays in Settings. Workflow pages group controls by the question they answer,
show ordinary choices first, and put tuning limits, executable paths, and pipeline budgets behind the
group's Advanced disclosure. A safe default is not a reason to make its tuning knob part of the
everyday path. Group headings carry a one-line explanation when the distinction would otherwise be
unclear.

**Save model — explicit, spanning the whole Settings surface (Sonarr's sticky save bar):** the
buffer and the bar both live in the Settings *layout*, not on a page (V9/V10) — the tab bar is
navigation, not a commit boundary, and a per-page buffer silently discarded edits on tab switch.
Edits accumulate with dirty tracking; a persistent bar offers Save/Discard; navigation away with dirty state prompts. Chosen over per-field autosave because connection settings often change *together* (URL + token) and half-saved pairs mid-test are a footgun. Save = validate → persist → hot-apply → per-key results (RFC 7807 problems map to inline field errors; `pinned` keys are rejected with the chip explanation).

**The save bar spans TABS, not just a page (V9).** Dirty state survives switching between
Connections, AI, Defaults and so on: an operator who edits a connection, checks a default, and
comes back must not silently lose the first edit. One `edits` map keyed by setting id, one bar,
one Save. The alternative — per-tab state — makes the bar's "3 unsaved" mean something different
depending on where you are standing, which is worse than no count.

**Four INLINE-COMMIT exceptions, and they are all verbs.** These commit immediately and are
deliberately outside the save bar, because each is an *action* rather than a staged edit — you
do not "stage" regenerating a secret, and a Save button next to *Run now* would be nonsense:

| Action | Where | Why it cannot stage |
| --- | --- | --- |
| **Select a model** | AI | Hot-swaps the live suggester (§8.1); the picker's whole point is trying one |
| **Pull a model** | AI | A long streaming download with its own progress, not a value |
| **Regenerate a secret** | Security | Destructive and irreversible — it invalidates the old value on the spot (§4) |
| **Run a job now** | System → Tasks | Triggers work; there is no "unsaved run" |

Everything else on every page goes through the bar. A fifth exception should be argued for
against this list, not added quietly.

This applies to contextual workflow editors too. Filler's Incoming-page auto-filing panel is the
curated home for its enable switch, confidence threshold, and destructive on-file loudness option,
but those values are staged inside the panel and committed with an explicit **Save auto-filing**
action. Being closer to the affected clips is not permission to introduce an uncued autosave model.

**Everything on Connections is self-diagnosing — and quiet once set up.** A connection block (Media server, Requester, Tunarr, TMDB) is a collapsible card carrying its own live status dot + inline Test verdict + `Fix →` link — the same shell the wizard's Connect step uses (config-design §6; the shared `ConnectionBlock` component). **Broken blocks open, healthy ones collapse**, so the page opens focused on what needs attention — and a fully set-up install shows a page of quiet collapsed blocks with nothing to worry about.

**Wiring is an effect of saving, not a manual action.** Registering Tunarr as a tuner/guide source in the media server (`livetv`) and pointing Tunarr at the library (`tunarr_library`) are *idempotent and fully derived from the saved connection values* — there is no decision to make and re-running is a no-op. So a Connections save runs them server-side automatically (`settingsPatch` → both connectors, best-effort and non-fatal: a wiring failure never fails the save and surfaces on the relevant connection's own status). The page therefore has **no wiring buttons and no separate checklist** — both would be scaffolding a set-up operator shouldn't have to see. **This holds in the wizard too: there is no standalone "Live TV" / "TV guide" wizard step.** Saving the Tunarr connection (the Connections step) already auto-wires the guide, so a dedicated step would only re-run the same no-op and add a redundant click — the `livetv` outcome instead surfaces on the Tunarr connection's own verdict. The `webhook` handshake *does* remain a first-run wizard step, because it is genuinely interactive (a paste-and-listen flow that can't be derived from a saved value), not a resting Connections concern. Settings remains the troubleshooting console (main doc §13) because each block is re-testable in place and every failure links to its fix.

---

## 6. Onboarding integration — the wizard *is* the settings system

No parallel form system. Each wizard step renders the relevant **settings group's form** (essentials only — `Advanced` keys hidden), pre-resolved (env-pinned fields render locked), with the group's live test inline. **Configure → validate → save → advance.** The wizard writes through the exact same PATCH path as Settings.

- **Step → group mapping:** claim (auth, pre-settings) → **playout choice** (`playout.backend`; see below) *(picking Tunarr reveals Connections/Tunarr + its media-source check* **on that same step**, *because "who plays my channels" and "where is it" are one decision;* **saving auto-wires Live TV into the guide** — no separate step*)* → Connections/media-server → Connections/requester + **webhook handshake** (displays the generated secret's URL, listens for `Test`) → AI (skippable; includes the §8.1 model picker) → Filler (skippable; drop-folder path + optional starter ingest targets) → guided first channel.
- ⚠ **The step list is DERIVED from `playout.backend`, not constant** (design §13). Internal playout is the default since §9.1, but the wizard hardcoded `tunarr` as a blocking check and gave it a wiring step, so the default path demanded a second server it would never use and refused to continue without it. The blocking set is now `media_server` alone on the internal path, `media_server` + `tunarr` on the Tunarr path; the Tunarr form and the "give Tunarr your library" step exist **only** on the Tunarr path. Removed steps are **hidden, not marked satisfied** — a rail entry reading "not needed" still advertises work that is not part of this install.
- **Being configured elsewhere does not stop a check blocking.** Tunarr's form lives on the Playout step, but on the Tunarr path its check still gates the Connections step. Where a setting is *edited* and where its failure is *reported* are separate questions, and conflating them would either hide a real blocker or move the whole checklist.
- **A blocking check is a property of the chosen path, so nothing that cannot be satisfied can block.** This is the same rule the skippable steps encode, applied one level up: an internal install can never turn `tunarr` green, and a gate on a check the operator cannot satisfy is a dead end the wizard's Back/Continue offers no way around.
- **Skippable steps are neutral, not red:** checklist states are `pass | fail | skipped | pinned`. Skipping AI doesn't shame you with a red X — it shows a neutral "not configured" that links back here.
- **First-run detection:** the `setup.completed` registry key (bool, `SETUP_COMPLETED`, Advanced — dotted to match every other key); until set, `/` routes to the wizard. The wizard's final step sets it through the ordinary `PATCH /v1/settings` path, so it is a setting like any other. "Re-run setup" lives in Settings forever.
- **Defaults philosophy:** every optional key ships a working default, so the shortest honest path to a live channel is **the media server alone** — Loomarr plays the channels itself (§9.1). Choosing Tunarr adds it to that path. Seerr adds acquisitions; AI adds suggestions.

---

## 7. Feature gating derives from settings completeness — with one environment-derived exception

`RequiredFor` on registry keys computes feature availability:

- **AI unconfigured** → the Suggest tab renders an inviting empty state ("Connect an LLM to build channels from a sentence" → deep link to Settings→AI), and `POST /v1/proposals` returns a 409 problem `feature_not_configured` with the same pointer. Never a stack trace, never a dead button.
- **Requester unconfigured** → proposals still generate from the library; acquisitions render disabled with "Connect Seerr to request missing titles."
- **Filler unconfigured** → channels build without pods; the channel card notes "no filler drop-folder configured."

**`ingest` is the exception, and it is deliberate.** Its availability is not a settings question — it depends on whether `yt-dlp` + `ffmpeg` are actually **runnable**, which no setting can assert. It therefore resolves from binary presence at startup rather than from `RequiredFor`.

⚠ **This paragraph used to say the gate meant "you are on `loomarr:latest`, switch to `loomarr:filler`".** That two-tag split no longer exists — the single image always ships the tooling (design §16) — so `ingest` **off** now means a *degraded* install: a custom image built without the vendored binaries, or a configured path that is missing or not executable. The UI copy must say that, not send an operator hunting for an image tag nobody publishes. The underlying rule is unchanged and is why this exception is recorded at all: pointing someone at Settings for a gate no setting can open is the dead end this section exists to prevent — and pointing them at a nonexistent image is the same mistake wearing different clothes.

The rule that survives: **one function computes the set, and every consumer reads it.** The checklist, the tab states, and the API gating never re-derive availability — only the *inputs* differ, never the seam.

**Policy defaults are settings; policy effects are data.** Changing a registry default (e.g. the episode no-repeat window) hot-applies like any setting — but it only affects channels that *don't override* that key. The channel editor shows per-field provenance mirroring the settings UI: `channel override | default | built-in`.

---

## 8. API contract (extends main doc §7)

- `GET /v1/settings` → grouped entries: `{key, group, kind, value | {set, preview}, provenance, advanced, doc, enum, requiredFor, testable, updatedBy, updatedAt}`.
- `PATCH /v1/settings` → per-key results `{saved | invalid(problem) | pinned}`; hot-applies on success. An empty value clears an optional key, **except on a secret, where it is `invalid`** (§9).
- `DELETE /v1/settings/{key}` → the **explicit clear**: drops the stored override so the key reverts to env/default. This is the only way to unset a secret. `204` on success; `404` for an unknown key; `409` when the key is env-pinned (the environment wins — unset the variable to manage it in the app). Hot-applies like any write.
- `POST /v1/setup/test` body `{check}` → run **one** named check (powers per-block Test buttons); `GET /v1/setup/status` runs all.
- `GET /v1/settings/secrets/{name}` → reveal a **displayable** generated secret's value (`{value, displayable}`), the read half of §4's "viewable on demand by admins (eye toggle + copy button)". `SESSION_SECRET` is never displayable — it returns `displayable:false` with no value. Without this, the only way to see `PLAYOUT_TOKEN` would be to *rotate* it, which stops every media-server tuner already configured; the Live TV setup step needs to show the URL, not change it.
- `POST /v1/settings/secrets/{name}/regenerate` → per §4 side-effects.
- The §8.1 model-selection routes (`GET /v1/system/llm`, `POST /v1/system/llm/{select,test,pull}`) are the AI group's live-configuration surface — the same admin-gated, secret-masking discipline applies (keys never returned).
- All admin; secrets masked everywhere per §4.

---

## 9. Failure modes & edges (decided)

- **Concurrent edits:** last-write-wins per key (`updated_at`); the save bar refreshes provenance/values on conflict rather than silently clobbering the whole page.
- **URL normalization** in `Validate` (scheme required, trailing slash stripped) so `http://emby:8096/` and `http://emby:8096` are one value.
- **Unknown keys in the DB** (only reachable if the downgrade guard were bypassed): ignored with a warning, never fatal.
- **Empty-string PATCH** on an optional key clears it (reverts to default); on a required-shape key it's `invalid`. **A secret is the exception: an empty-string PATCH on a `secret` key is `invalid` ("replace-only"), never a clear.** Reason: `GET /v1/settings` deliberately returns no value for a secret (§4), so a client that reads settings and writes them back would submit `""` for every secret — silently destroying the stored Emby token, Seerr/TMDB/LLM keys. Making the round-trip *loud and harmless* rather than quietly destructive is worth more than the convenience of clear-by-empty, and §4 already calls secrets replace-only. Clearing a secret is an explicit act: `DELETE /v1/settings/{key}` (§8).
- **Secret shape sanity-check** (in `parseKind` for `KindSecret`, so it guards *every* secret — Emby/Jellyfin token, Seerr/TMDB/LLM keys, and any future one). Secrets have no universal format, so this is a *sanity* guard, not a format check: the raw value is **trimmed** of surrounding whitespace (a common paste artifact), then rejected as `invalid` if it (a) contains **internal whitespace** (space/tab/newline — no real API key or token does) or (b) is **shorter than 4 characters** after trimming (a 1–3 char fragment). This exists because a `secret` otherwise stores *any* string verbatim, and a connection-test hint (`"set a media server flavor (emby | jellyfin)"`) once ended up persisted as `library.token`, which then made every media-server probe fail with a `401` that looked like a credential problem rather than a corrupt-store problem. **The length floor is deliberately LOW (4)** because this guard also runs on the resolve/read path — a stored value is re-parsed on read and self-heals to the default if it no longer parses (§9 "db drift") — so a higher floor would retroactively invalidate a real short key already in a user's store on upgrade. The whitespace check is what actually catches the error-string corruption; the length floor is a light bonus. The message is secret-safe — it names the *problem* ("a token can't contain spaces"), never echoes the value (§4). The parsed (trimmed) value is what gets **persisted** (see the canonical-value rule below), so the store never keeps the surrounding whitespace. This does **not** replace the live connection Test, which is the real proof a token works.
- **Canonical value on write.** `PATCH` persists the value the *parse* produced, not the raw input — so a URL's stripped trailing slash and a secret's trimmed whitespace are what land in the store, and the stored form matches the resolved form. (Storing raw would keep e.g. `http://emby:8096/` on disk while resolving to `http://emby:8096`, a silent disagreement.)

---

## 10. Testing (extends main doc §19)

- **Resolution matrix** per Kind: env beats db beats default; invalid **env fails boot** with the var named; invalid **db self-heals** to default + warning surfaced.
- **Pin lifecycle:** set env → locked + `pinned` on PATCH; unset env + reboot → unlocked, provenance `db`.
- **Hot-apply:** change library URL via PATCH → next `Lookup` hits the new host (mock); interval change takes effect next tick; an §8.1 model `select` hot-swaps the live suggester provider without restart.
- **Secrets:** generation idempotent across restarts; `<VAR>_FILE` loads (and `<VAR>`+`_FILE` together fails boot); regen side-effects (sessions die incl. caller's; handshake check goes red until fresh `Test`); the **log-grep redaction test** (a known secret never appears in captured logs, error bodies, or setup/status).
- **Feature gating:** AI keys absent → Suggest returns the 409 problem and the computed feature set says so; add the keys → gate opens without restart.
- **`make config-docs` drift check** green.

---

## 11. Build integration

- **Phase 1 (built as a cross-phase retrofit, 2026-07-15):** registry + resolution + env/`_FILE` loading + snapshot/Watch + redactor into slog. `make config-docs` target. The settings table gains its `updated_at`/`updated_by` audit columns (§3) via **forward-only migration `00008`** (an `ALTER TABLE settings ADD COLUMN …`, the second real ALTER after `00007`'s `policy_json`; the bare `(key,value)` KV from `00001` never drops). The proposed registry middle tier for ChannelPolicy was later retired in V55 because no production scheduling path consumed it; channel policy now resolves directly to the scheduler's documented built-ins.
- **Phase 8:** the §8 API surface.
- **Phase 9:** generated secrets + regeneration side-effects (auth interplay).
- **Phase 13:** Settings pages, save bar, provenance chips, wizard-as-settings-forms, feature-gated empty states.
- This doc is a **seed doc**: incorporate as `docs/config-design.md` during phase 14; `docs/configuration.md` is the *generated* reference beside it.
