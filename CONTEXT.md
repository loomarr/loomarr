# Loomarr

Loomarr turns a natural-language channel intent into a live, self-maintaining TV channel:
it suggests a lineup, acquires what is missing, schedules it with commercial breaks, and
converges it on Loomarr's internal playout or an optional Tunarr backend.

This file is a **glossary and nothing else** — what each word *means*. It deliberately holds
no behavior, no endpoints, and no decisions.

⚠ **`docs/design.md` remains the single source of truth** for how the system behaves, and
its numbered sections (`§7`, `§11`, …) are cited from ~2,600 places in the code. Where a
term's *behavior* matters, the § reference next to it is the authority. This file exists
because those definitions are currently spread across 1,700 lines of dense prose, so there
is nowhere to look up what a word means without reading the section that uses it.

## Language

### Content and acquisition

**Title**:
A unit of content the app wants — one movie or one series. Identity is always an external
provider id, never a name string (§3).
_Avoid_: media, item, content (all three are the media server's words, not ours)

**Key**:
The stable identity string derived from a Title, identical whether it came from a suggestion
or a webhook: `series:tvdb:<id>`, else `<mediatype>:tmdb:<id>` (§3).
_Avoid_: id, slug

**Record**:
The persisted provisioning state of one Key — its state, library item id, deadline, attempts
and last error (§3).
_Avoid_: row, entity

**Provisioning state**:
Where a Record sits on its way into the library: `wanted` → `requested` → `downloading` →
`available`, or `unavailable` when it gave up (§4). Only `available` is schedulable.
_Avoid_: status (reserved for Proposals and Jobs), download state

**Library**:
The operator's media server (Emby or Jellyfin) — the authority on what content actually
exists. Loomarr reads it and never writes to it.
_Avoid_: media server (acceptable in prose, but `library` is the term in code), Emby, Jellyfin

**Acquisition**:
A pick that is not in the Library yet and must be requested through Seerr → Sonarr/Radarr.
The opposite of an in-library pick, which is free and instant.
_Avoid_: download, request (a request is the downstream act, not the pick)

### Suggestion and approval

**Intent**:
The operator's natural-language description of a channel — the input to the Suggester.
_Avoid_: prompt, query, description

**Proposal Job**:
One execution of an Intent, distinct from the optional Proposal artifact that may result.
_Avoid_: request, suggestion job, generation job, Proposal

**Proposal**:
The Suggester's grounded answer to an Intent: a lineup of picks plus an extracted policy.
Statuses are `submitted`, `approved`, `denied` (§7, §8).
Lives at `/v1/proposals*`; every operationId is `*-proposal(s)`.
_Avoid_: suggestion, recommendation, plan

⚠ The routes said `/v1/suggestions` until V41 (retired-ok — named here to record the rename),
and one operationId (`submit-suggestion`) sat
among five `*-proposal` siblings in the same file — so one resource was submitted as a
"suggestion" and read, approved and denied as a "proposal". A glossary nothing follows is not a
glossary. `scripts/check-retired.sh` now guards the old path.

⚠ Two survivors are deliberate, and both are the VERB, not the artifact. The Proposal Job's
persisted `kind` is `"suggest"` (renaming it is a data migration, and the job is not the
proposal), and the SSE frame `"suggestion"` reports that job's PHASE — its Go→TS handler pairing
has no drift guard, so churning it is real risk for no glossary gain. The banned noun is the name
for the artifact; `internal/suggest` remains the package that produces one.

**Grounding**:
The rule that the model may only pick from candidates a tool call actually returned this run.
An id the catalog never surfaced cannot enter a Proposal (§8).
_Avoid_: validation, filtering

**The approval gate**:
The rule that nothing spends real resources until an admin approves it. Acquisition happens
on approval, never on suggestion (§7).
_Avoid_: review, confirmation

**Refine**:
Re-running the Suggester against a stored Intent *plus* the channel's current lineup, to
adjust rather than replace it (§7, §8.2).
_Avoid_: regenerate, retry

### Channels and programming

**Channel**:
A Loomarr-owned channel definition — identity, Lineup, and Policy — whose desired state is
materialized locally and, when Tunarr is selected, projected into a Tunarr channel.
_Avoid_: station, feed

**Lineup**:
The ordered set of Titles a Channel draws from. Editing it is a whole-list replace, diffed
server-side (§7).
_Avoid_: playlist, schedule (a schedule is the time-bound result, not the source set)

**Policy** (`ChannelPolicy`):
The per-channel programming rules: scope, audience, ordering, separation, seasonal, filler,
window (`programming-design.md` §2).
_Avoid_: config, settings (both mean the app-wide subsystem), preferences

**Operator-set**:
A Policy field an admin edited by hand. A later Proposal may not overwrite it — the audience
ceiling may still be tightened, never relaxed (`programming-design.md` §2).
_Avoid_: locked, pinned, dirty, overridden

**Provenance**:
Whether a scheduling rule was authored by the model (`llm`) or a person (`operator`). A refine
replaces the `llm` rules and preserves the `operator` ones.
_Avoid_: origin, author

**Pending slot**:
A Lineup entry whose Title is not `available`. It renders as flex to Tunarr and swaps to a
program in place only if that Title independently reaches `available` — so a manual edit can
never make unapproved content play (§7).
_Avoid_: placeholder, gap, empty slot

### Commercials

**Clip**:
One piece of filler content. Identity is its **sparse content hash** — 64 hex characters, not
its path (§10 V38c). A file that moves within `FILLER_DIR` is the same Clip; two copies at
different paths are one Clip.
_Avoid_: commercial, ad, asset

**Composite**:
A Clip that is a *container* of other clips — "KCPQ/Fox commercials, 5/28/1996" — kept as the
parent after splitting. A Composite is **not airable**: it is excluded from selection, and its
Segments are what play (§10 V45).
_Avoid_: compilation, reel, source clip

**Segment**:
A Clip produced by splitting a Composite, carrying lineage back to its parent (§10 V45).
_Avoid_: cut, chunk, part

**Taxon** (and **Tag**):
The clip vocabulary is a **forest of taxa on independent axes** — product, format, seasonal,
audience-cue — where a leaf tag like `beer` rolls up to `alcohol` and `drinks`. A Clip carries
a **set** of tags, not one category, so a curation rule can ask "is `cereal` a kind of food?"
A model's output is resolved against the vocabulary or dropped (§10 V45a).
_Avoid_: category (the flat 12-value string this replaced), genre, label

**Pod**:
An assembled commercial break — an ordered set of Clips inserted between programs (§10).
_Avoid_: break, ad block, interstitial

**Filler**:
Loomarr-owned non-program content as a whole. It lives in a Tunarr-local media source, never
in the operator's Library, so it structurally cannot leak into a programming Lineup (§10).
_Avoid_: bumpers, interstitials (both are *kinds* of filler, not the category)

### Delivery

**Playout**:
Loomarr serving its own video streams, as opposed to delegating to Tunarr (§9.1).
_Avoid_: streaming, transcoding (transcoding is one step within playout)

**Reconcile**:
Bringing owned state to its desired form — a Channel's local schedule and optional Tunarr
projection, or the Library into Records. Always best-effort and repeatable; there is no manual
"rebuild" (§7, §9).
_Avoid_: sync, push, publish

**Image** (and **Rendition**):
Every picture in Loomarr — channel icon, clip still, TMDB poster — is one **Image** travelling
one pipeline; a **Rendition** is a particular size and format of it (§22). Callers hand bytes
or a URL and receive an Image; they ask for a Rendition and receive a file. Nothing outside
that package knows the disk layout, the hash, the format ladder, or which encoder ran.
_Avoid_: thumbnail, asset, poster (all are Renditions of an Image)

**Image worker**:
The required Rust process behind the Image service's one rendering seam (§22). It validates and
interprets pixels and writes unpublished Renditions; Go still owns Image identity, policy, and
publication. It is an implementation detail shipped in the Loomarr container, not a sidecar or an
optional integration.
_Avoid_: image service (that includes the Go-owned domain), daemon, fallback

### People and access

**The allowlist**:
The `users` table. You can sign in iff you have a row — a credential proves *who you are*, the
row decides *whether you may enter* (§11). The central authorization invariant.
_Avoid_: whitelist, user list, ACL

**Credential path**:
A way of proving identity: local password, imported media-server account, or SSO. All three
resolve to the same allowlist and none of them provisions a row (§11).
_Avoid_: auth method, provider, login type

**Imported**:
A media-server account an admin explicitly added to the allowlist. Signing in is never
self-provisioning (§11).
_Avoid_: synced, linked, connected

### Operations

**Job**:
A named unit of recurring or long-running work on the job bus, with a cron default and a
settings key. Reports progress over SSE and is cancellable (§18).
_Avoid_: task, cron, worker

**Phase**:
One numbered step of the build plan (§21). Its status and gate evidence live in `PROGRESS.md`,
never in an issue.
_Avoid_: milestone, sprint, epic

**Gate**:
The set of tests that must pass for a Phase to count as done. Never stubbed, skipped, or
weakened (AGENTS.md prime directive #2).
_Avoid_: check, CI (CI is where gates run, not what they are)
