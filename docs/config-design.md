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

**The classification rule (so future keys self-classify):** a setting is **env-only** iff it is needed *before the database opens* or describes *process topology* — `DATABASE_URL`, `AUTO_MIGRATE`, `LISTEN_ADDR`, `LOG_LEVEL`, `TZ`. Everything else is app-managed and env-pinnable. Secrets the app can mint itself (`SESSION_SECRET`, `API_TOKEN`, `WEBHOOK_SECRET`) are **generated**, never demanded.

**The per-channel tier (added with `programming-design.md`):** programming heuristics introduce settings that vary *per channel* — the ChannelPolicy (scope, audience ceiling, separation windows, ordering, seasonal mode). These are **not registry settings**: a policy instance is channel *data*, stored on the channel row, edited in proposal review / the channel editor, and never env-addressable. What the registry holds is their **global defaults**. Full precedence, per key:

`channel policy > registry default (env-pinnable per the normal rule) > built-in`

The test for which tier a new knob belongs to: *would two channels sensibly want different values?* Yes → policy field with a registry default. No → plain registry setting.

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

**Secrets via files:** every secret env var also accepts the Docker-secrets idiom — `LIBRARY_TOKEN_FILE=/run/secrets/emby` loads the value from the file (trailing newline stripped). `<VAR>` and `<VAR>_FILE` both set → boot error (ambiguous).

**Hot-apply (no restart to reconfigure):**
- The settings service holds an in-memory snapshot (RWMutex) refreshed on local write and, for Postgres replicas, on a ~30s read-through interval (main doc §17).
- **Connections read through per use:** the shared HTTP client factory (§6 main doc) fetches URL/token from the snapshot at call time — saving a new Emby token means the *next* lookup uses it.
- **Intervals re-read per tick:** tickers ask the snapshot each cycle; changing `CHANNEL_RECONCILE_EVERY` takes effect next tick.
- **Long-lived constructions rebuild on change:** the LLM client subscribes via `Watch(keys...) <-chan Change` and reconstructs. (This is the same seam the §8.1 model-selection hot-swap uses — an atomic-pointer provider that rebuilds on a persisted `llm.*` change.)
- `RestartRequired` exists as a flag for honesty but applies only to the bootstrap set, which the UI never edits.

**Audit:** the settings table carries `updated_at` + `updated_by` (nullable — env/migration writes have none). The UI shows "changed by Matt · 2d ago" per field; same spirit as `approved_by`.

---

## 4. Secrets lifecycle

**Generation:** 256-bit random, base64url, created idempotently inside the first-migration transaction (alongside the instance id).

**Display policy (Sonarr-model, differentiated by purpose):**
- `API_TOKEN` and `WEBHOOK_SECRET` are *operational values you must paste elsewhere* — viewable on demand by admins (eye toggle + copy button), exactly like Sonarr's API key.
- `SESSION_SECRET` has nothing to paste anywhere — **never displayed**; the only affordance is Regenerate.
- Integration secrets you *entered* (Emby token, Seerr key, TMDB, LLM key) — masked after save (`set · …a1b2` preview), replace-only. The API returns `{set: true, preview, provenance}` — never the value. (The §8.1 hosted `llm.api_key.<provider>` keys follow this exact rule: stored, previewed, never echoed by any GET.)

**Regeneration side-effects (typed-confirmation dialogs, effects stated up front):**

| Secret | Immediate effect | UX contract |
| --- | --- | --- |
| `SESSION_SECRET` | **All sessions revoked — including yours** | Confirm → regen → redirect to login. `API_TOKEN` remains as break-glass, so you cannot lock yourself out. |
| `API_TOKEN` | Old token dead instantly | Show the new token once prominently; remind that machine clients/scripts must update. |
| `WEBHOOK_SECRET` | Sonarr/Radarr webhooks start failing | Show the new `/hooks/arr?token=…` URL + "re-run the handshake"; the §13 handshake check goes red until a fresh `Test` is received. |

**Redaction is systemic, not per-callsite:** the settings service exposes a `Redactor` (the current set of secret values) wired into the `slog` handler — a secret value appearing in any log line is replaced before write. Secrets are excluded from `/v1/setup/status`, from validation error strings (validators must never echo the value), and from RFC 7807 bodies. There is a test that greps captured logs for a known secret and demands zero hits.

---

## 5. Settings UI — information architecture

Sonarr's shape, Test Card's skin (FE doc §6 provenance rules apply):

| Page | Contents | Live tests |
| --- | --- | --- |
| **Connections** | Media server (flavor · URL · token) · Requester (Seerr *or* direct Sonarr+Radarr) · Tunarr · TMDB · **the two one-click wiring actions** (Connect Tunarr to the guide; Wire Tunarr to your library — main doc §6) | one **Test** button per connection block → runs the same `ConnectionTest` the wizard/checklist uses; the wiring actions run `POST /v1/setup/{livetv,tunarr}-connect` and report through their `livetv` / `tunarr_library` checks |
| **AI** | Provider (ollama/openai) · URL · model · key · auto-approve + quota · **in-app model picker** (probe + catalog + hot-swap, main doc §8.1) | the tool-call **probe** (main doc §8) + `GET /v1/system/llm` (probe/catalog), `POST /v1/system/llm/test` (key validation) |
| **Channels & playback** | Default strategy · backfill mode · reconcile interval · season precision · **policy defaults** (episode/movie no-repeat windows, series min-gap, block max, default ordering, seasonal mode, holiday calendar toggles — `programming-design.md` §2) | — |
| **Filler** | Drop-folder path (registered with Tunarr as a `local` source — *not* a media-server library, design §10) · sync interval · AI tagging · pod density · ingest tool paths (`loomarr:filler` only) | drop-folder readable + Tunarr local-source check |
| **Users & security** | Session TTL · cookie mode · user-sync interval · **Generated secrets panel** (view/copy/regenerate per §4) | — |
| **Advanced** | TTLs · retention · job workers · event webhook — plus every `Advanced: true` key surfaces here *and* behind its home page's toggle | — |

**Field anatomy:** label · control · provenance chip (`set via environment` = locked; caution chip on self-healed values) · one-line doc · Test button where testable · "changed by … · when".

**Save model — explicit, per page (Sonarr's sticky save bar):** edits accumulate with dirty tracking; a persistent bar offers Save/Discard; navigation away with dirty state prompts. Chosen over per-field autosave because connection settings often change *together* (URL + token) and half-saved pairs mid-test are a footgun. Save = validate → persist → hot-apply → per-key results (RFC 7807 problems map to inline field errors; `pinned` keys are rejected with the chip explanation).

The re-runnable **connection checklist** sits at the top of Connections — Settings remains the troubleshooting console (main doc §13).

---

## 6. Onboarding integration — the wizard *is* the settings system

No parallel form system. Each wizard step renders the relevant **settings group's form** (essentials only — `Advanced` keys hidden), pre-resolved (env-pinned fields render locked), with the group's live test inline. **Configure → validate → save → advance.** The wizard writes through the exact same PATCH path as Settings.

- **Step → group mapping:** claim (auth, pre-settings) → Connections/media-server → Connections/Tunarr (+ media-source check + one-click Live TV connect) → Connections/requester + **webhook handshake** (displays the generated secret's URL, listens for `Test`) → AI (skippable; includes the §8.1 model picker) → Filler (skippable; drop-folder path + optional starter ingest targets on `loomarr:filler`) → guided first channel.
- **Skippable steps are neutral, not red:** checklist states are `pass | fail | skipped | pinned`. Skipping AI doesn't shame you with a red X — it shows a neutral "not configured" that links back here.
- **First-run detection:** the `setup.completed` registry key (bool, `SETUP_COMPLETED`, Advanced — dotted to match every other key); until set, `/` routes to the wizard. The wizard's final step sets it through the ordinary `PATCH /v1/settings` path, so it is a setting like any other. "Re-run setup" lives in Settings forever.
- **Defaults philosophy:** every optional key ships a working default, so the shortest honest path to a live channel is media server + Tunarr; Seerr adds acquisitions; AI adds suggestions.

---

## 7. Feature gating derives from settings completeness — with one environment-derived exception

`RequiredFor` on registry keys computes feature availability:

- **AI unconfigured** → the Suggest tab renders an inviting empty state ("Connect an LLM to build channels from a sentence" → deep link to Settings→AI), and `POST /v1/suggestions` returns a 409 problem `feature_not_configured` with the same pointer. Never a stack trace, never a dead button.
- **Requester unconfigured** → proposals still generate from the library; acquisitions render disabled with "Connect Seerr to request missing titles."
- **Filler unconfigured** → channels build without pods; the channel card notes "no filler drop-folder configured."

**`ingest` is the exception, and it is deliberate.** Its availability is not a settings question — it depends on whether the running *image* carries `yt-dlp` + `ffmpeg`, which only `loomarr:filler` does (design §16). No amount of configuring makes it available on `loomarr:latest`, so it resolves from binary presence at startup rather than from `RequiredFor`. This matters for the UI copy: every other gate says "configure this," while `ingest` must say "**run the `loomarr:filler` image**" — pointing an operator at Settings for a gate no setting can open is exactly the dead end this section exists to prevent.

The rule that survives: **one function computes the set, and every consumer reads it.** The checklist, the tab states, and the API gating never re-derive availability — only the *inputs* differ, never the seam.

**Policy defaults are settings; policy effects are data.** Changing a registry default (e.g. the episode no-repeat window) hot-applies like any setting — but it only affects channels that *don't override* that key. The channel editor shows per-field provenance mirroring the settings UI: `channel override | default | built-in`.

---

## 8. API contract (extends main doc §7)

- `GET /v1/settings` → grouped entries: `{key, group, kind, value | {set, preview}, provenance, advanced, doc, enum, requiredFor, testable, updatedBy, updatedAt}`.
- `PATCH /v1/settings` → per-key results `{saved | invalid(problem) | pinned}`; hot-applies on success. An empty value clears an optional key, **except on a secret, where it is `invalid`** (§9).
- `DELETE /v1/settings/{key}` → the **explicit clear**: drops the stored override so the key reverts to env/default. This is the only way to unset a secret. `204` on success; `404` for an unknown key; `409` when the key is env-pinned (the environment wins — unset the variable to manage it in the app). Hot-applies like any write.
- `POST /v1/setup/test` body `{check}` → run **one** named check (powers per-block Test buttons); `GET /v1/setup/status` runs all.
- `GET /v1/settings/secrets/{name}` → reveal a **displayable** generated secret's value (`{value, displayable}`), the read half of §4's "viewable on demand by admins (eye toggle + copy button)". `SESSION_SECRET` is never displayable — it returns `displayable:false` with no value. Without this, the only way to see `WEBHOOK_SECRET` would be to *rotate* it, which breaks every webhook already configured; the §13 handshake step needs to show the URL, not change it.
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

- **Phase 1 (built as a cross-phase retrofit, 2026-07-15):** registry + resolution + env/`_FILE` loading + snapshot/Watch + redactor into slog. `make config-docs` target. The settings table gains its `updated_at`/`updated_by` audit columns (§3) via **forward-only migration `00008`** (an `ALTER TABLE settings ADD COLUMN …`, the second real ALTER after `00007`'s `policy_json`; the bare `(key,value)` KV from `00001` never drops). Registry defaults become the middle tier of the ChannelPolicy precedence (`channel policy > registry default > built-in`) that `programming-design.md` §9 recorded as deferred — the `SCHED_*`/`SEASONAL_MODE` policy-default keys (main doc §15) now resolve through the registry instead of Go constants.
- **Phase 8:** the §8 API surface.
- **Phase 9:** generated secrets + regeneration side-effects (auth interplay).
- **Phase 13:** Settings pages, save bar, provenance chips, wizard-as-settings-forms, feature-gated empty states.
- This doc is a **seed doc**: incorporate as `docs/config-design.md` during phase 14; `docs/configuration.md` is the *generated* reference beside it.
