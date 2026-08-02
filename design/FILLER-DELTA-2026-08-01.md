# Filler screen — prototype delta, 2026-08-01

The maintainer updated `Loomarr Prototype v2.dc.html` in the Claude Design project
(`dc543738-1b55-41c0-a295-a693b65aef05`, "Shared file archive"). This file records the **Filler
screen's** new structure, because the committed prototype in this directory still shows the old
one and the new markup could not be merged into it — see *Why the .dc.html is stale* below.

Read it alongside `docs/engineering/v2-build-plan.md` §6.5 (V35), which is what gets built from it.

> Precedence is unchanged (`design/README.md`): this describes **look and structure**.
> `docs/design.md` wins on behaviour, and the illustrative strings below are mock sample data, not
> a contract.

## Why the .dc.html is stale

`DesignSync.get_file` hard-caps at **262,144 bytes**. The prototype is ≥310 KB of markup before its
`<script data-dc-script>` even begins, followed by ~192 KB of JS. The fetch on 2026-08-01 returned
`truncated: true` at exactly the cap — enough for the whole Filler screen (it lives at byte
172k–260k) and nothing of the JS.

⚠ **A partial splice does not work, and the reason is worth keeping.** The committed file's JS
defines `covBars` / `registry` / `dscFilters`, which the new markup no longer references, while the
new markup binds `poolStats` / `asks` / `reels` / `services`, which its JS does not define. Pasting
the markup in yields a file that parses and renders nothing — strictly worse than a stale file that
renders the old screen honestly.

**Resolution: a maintainer export**, the same route taken 2026-07-28 (`design/README.md` records
that the desktop v2 file was obtained that way for exactly this reason). Until it lands, the values
behind `poolStats`, `asks`, `reels`, `services`, `autoConfChips` and `planRows` are **unknown — do
not invent them.** That is the `tcOptions`/`poPresets` situation the build plan already names.

**Verified unchanged in the same fetch:** `support.js` and `image-slot.js` are byte-identical to the
copies here, so no runtime change accompanies this and the export will render as-is.

## Tabs

`Coverage · Catalog · Sources · Discover` → **`Catalog · Incoming · Sources`**
(`fillerTabs`, `hint-placeholder-count="3"`). Each tab carries an **optional** count badge —
`ft.hasCount` is new; the old markup always rendered one.

The header is otherwise unchanged: title, the "Commercials, bumpers and station IDs" subtitle, and
the pulsing `{{ watchLine }}` status pill.

## Pool health — a strip, not a tab

`showPool`, rendered above the tab content and shared by all three tabs.

Four `poolStats` entries (`label` / `value` / `note`, each with its own colour) and a right-aligned
primary **Propose a pull** (`goDiscover`).

⚠ The old **Coverage tab** and its `covBars` / `covDiagnosis` / *"from the same ladder reconcile
uses"* attribution are gone from this page. Per-channel coverage remains on the channel-detail
Filler tab, where **"Find clips" is renamed "Propose a pull"** (both occurrences).

## Catalog tab

**Toolbar** — search (`catQ`, *"Search clips, brands, categories…"*), 5 filter chips (`catChips`),
and a 2-button **view toggle** (`catViews`, grid ⇄ list).

**Selection** — a count line (`catCount`) + **Select all**. When anything is selected, a bulk bar
appears: `catSelLabel`, three `<select>`s (`catBulk`), **Remove from catalog** (destructive), and
**Clear**.

**Empty states** — `noClips` (nothing in the catalog at all, with its own *Propose a pull*) and
`catNone` (*"No clip matches those filters."*).

**List view** (`catListView`) — a dense 8-column grid: checkbox · thumbnail · name · kind · era ·
audience · duration · `usedLine`.

**Grid view** (`catGrid`) — the card:

| Region | Contents |
| --- | --- |
| Thumbnail | duration (bottom-right), **quality badge** (top-right), **select checkbox** (top-left), full-area preview button (`cl.preview`) |
| Body | name; optional `cl.sourceText` line (`hasSourceText`) |
| Tags | kind · era · audience · category as **read-only spans**, then an **Edit tags** toggle (`cl.toggleEdit`) revealing three `<select>`s (`cl.tagFields`) |
| Footer | `usedLine` + a picker button (`cl.openPicker` / `pickerLabel`) |
| Picker | `modeNote`, then a channel list — each row a **checkbox** (`cp.toggle` / `cp.mark`), number, name, and a **`cp.fitNote`**. When overridden: `overrideLine` + **Back to automatic** |
| Untagged | `NO METADATA` + `suggestLabel` + **Accept** |

### Interactions this reverses

| Shipped today | New |
| --- | --- |
| Click-to-cycle era/audience/category on the chips | Read-only chips + the **Edit tags** select editor — ⚠ **NOT ADOPTED**, see below |
| **Pin** and **block** as two per-channel buttons | One checkbox set per channel + `fitNote` + **Back to automatic** — an include-set override, not two flags |
| `AI-TAGGED` badge | **absent** |
| `NO SIDECAR` | `NO METADATA` |
| `THUMBNAIL · SIDECAR` | `THUMBNAIL` |

⚠ **The tag-editing swap is a ratified divergence from the mock** (maintainer, 2026-08-01).
Click-to-cycle **stays**; the select editor is not being built.

The reasoning, recorded because a future reader will otherwise see the mock and "fix" the code
back: cycling is faster for the common case, which is one wrong tag on one clip. The select
editor's real advantage is setting several fields at once across several clips — and V35's **bulk
bar** now does exactly that, from a selection, which the mock's per-card editor never could. So
the redesign's own new affordance is what makes its per-card one unnecessary.

The general rule this is an instance of: **a mock is authoritative for what a screen SHOWS, not
for deleting a working interaction it happens not to draw.** `design/README.md` already says
`docs/design.md` wins on behaviour; this is the same precedence applied to an interaction.

## Incoming tab — new

`ftIncoming`. The ingest conveyor, in three parts.

**1. Work header + auto-file policy.** A pulsing dot, `workLine`, `workNote`, and a **Tune** toggle
(`toggleAuto`) opening: *"File automatically above ‹confidence›"* (`autoConfChips`, 3), *"Drop
unidentifiable clips under ‹…›"* (`autoMinChips`, 3), two checkboxes (`autoToggles`), and an
`autoSummary` line.

**2. Asks** — clips that need a human (`asks`, 3; heading `askHeading`; **File all as suggested**).
Each row: a ▶ preview thumb with duration, name, `ak.from`, *"Loomarr's best guess:"* + meta chips +
a coloured confidence, `ak.why`, and **Looks right** / a not-right toggle that opens inline selects
plus **Don't use it**.

`noAsks`: *"Nothing needs you. Everything downloaded so far has been filed or dropped."*

**3. Reels** — compilations (`reels`, 4). Collapsed: thumbnail + quality, title, meta, a coloured
status line, a progress bar when `busy`, a working pulse, and a **Review** toggle.

Expanded: a clickable **filmstrip** (`rl.strip`, *"every block is one detected clip — click to
preview"*), bulk actions (`rl.bulk`), a **Fix cuts** toggle (`rl.toggleFix`), then segment rows —
▶ preview, timecode, name, meta chips, `sg.why`, inline edit selects, a confidence meter + a `pill`,
and the actions **File** / not-right / **Undo split** / **Merge ↓** / **Drop**, with a **+ split
here** divider between rows.

`reelsEmpty`: *"Nothing at that stage right now."*

⚠ This is **V34's split review relocated into a tab** and merged with a per-clip review queue. The
shipped `/filler/splits/$proposalId` route is a *sibling* of `/filler` on purpose — `PROGRESS.md`
records that nesting it would have made the whole surface unreachable while every unit test stayed
green. The tab is an additional door, not a replacement.

## Sources tab — rewritten

The curated **registry** (`regFilters` / `registry` / *"Pull era matches…"*) and the separate
*"Where clips come from"* list collapse into **one list of toggleable services** (`services`, 5).

Each row: an on/off switch, a kind badge, name + description, a stat, an optional licence badge,
an optional **Search** expander, an optional remove ✕. A `svcOnLine` counts the enabled ones.

The switch's stated meaning — and it is a behaviour claim, not decoration:

> *"Switch a source off and Loomarr stops scanning, searching and downloading from it. Clips already
> in the catalog stay put."*

**Search expander** (archive.org) — `dscQ` input → `archResults` rows: thumbnail, title,
`date · dur · quality`, a `queued ✓` state, and **Queue download**. Footnote: *"previews stream from
archive.org — nothing downloads until you queue it"*.

⚠ These rows are deliberately **thinner** than the old Discover cards: no description, **no licence**.
That agrees with plan §6.3, where ~92% of archive.org items were measured to declare no licence at all.

**URL expander** (YouTube) — paste a playlist/channel URL (`ytUrl`) + **Save playlist**.

**Add a source** — a kind `<select>` (`sourceKinds`), an input, **+ Add source**, `sourceFootnote`.

⚠ The **Discover tab is gone.** Finding clips is now something you do *to a source*.

## Approvals — the FILLER PULL card

New on the Queue's *Needs approval* tab (`pullPending`), above the existing proposal cards.

- `FILLER PULL` badge + `pullTitle`, with `pullBy` beneath
- **Not now** (`dismissPull`) / **Approve pull** (`approvePull`)
- `pullWhy` in an inset panel
- `pullEmpty` → *"Every source this pull needs is switched off — turn one on in Filler → Sources."*
- `planRows` (3): a coloured `tag`, name, `why`, an `est`, and a ✕ (*"Leave this collection out"*)
- `pullMeta` + `pullRepeatLine`
- A note input: *"Anything to add or avoid? — 'no local dealers, no PSAs'"*

⚠ **This is the redesign's largest change.** Filler acquisition gains the approval gate that title
acquisition already has, which `docs/design.md` §10 anticipated in prose — *"the machine proposes, a
human commits"* — without an object to hang it on. Note the mock keeps a **direct** per-result
*Queue download* in Sources, so the gate lands on composed multi-source pulls rather than on an admin
queueing one clip. That split is the ratified reading (V35).

## Also in this import, outside Filler

A **Watch** entry on the guide row menu opening a player: LIVE badge, channel number, encoder line,
progress, transport, mute + volume, elapsed/total/remaining, quality pickers, **full frame**,
**Copy stream URL**, *Open in ‹media server›*, and a stats overlay.

⚠ Tracked as **V36**, and it is blocked on something the markup cannot show: playout emits raw
MPEG-TS (`internal/playout/args.go:266`), which no browser plays in a `<video>` element. See the
build plan.
