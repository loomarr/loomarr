# Programming defaults, overrides &amp; save/review timing — design review candidate

Review checkpoint for [#1817](https://github.com/loomarr/loomarr/issues/1817) item 4 and parent
[#1659](https://github.com/loomarr/loomarr/issues/1659). This is a proposal, not an approved
behaviour change. No production routes, components or API contracts change.

Open [the interactive prototype](../prototypes/programming-defaults.html) directly in a browser.
Its controls switch the channel's customization state (fresh / lightly customized / heavily
customized with a conflict), the rules editor's saved/dirty state, the two defaults-summary
alternatives, and a 390px frame. All fixtures are invented; nothing calls a server; the lineup,
rules and preview rows are decorative stubs, not the real player or scheduler.

## What this covers and why

[#1817](https://github.com/loomarr/loomarr/issues/1817) item 4 asks to design "effective defaults,
overrides and save/review timing" for channel Programming, while preserving the three sections
the mock organizes it into: **What plays**, **How it's ordered**, **When it changes**. The
Programming tab itself — block layout, field set, the rules-draft save model and the cycle
preview — is **already built** in [#1739](https://github.com/loomarr/loomarr/pull/1739)
(`web/apps/web/src/routes/_authed/channels/$id/-channel-programming.tsx`). What is **not** built,
and what this draft designs, is the layer the brief asks for on top of it: an **effective-value +
source** indicator per field, a **reset-to-default** affordance, and a **conflicting-override
warning** — none of which exist in the merged component today.

## What the prototype shows

### Each field's effective value and source (SOURCE: `channel-policy-fields.tsx`, `channel-auto-curate.tsx`, `channel-seasonal.tsx`, `internal/schedule/policy.go`)

Every field in **What plays** and **How it's ordered**, plus Seasonal behavior and the auto-curate
opt-in and its two overrides in **When it changes**, now carries a **Channel override** or
**Default** pill and, when overridden, a **Reset to default** action. The reset target per field
is not a placeholder — it is the exact sentinel the merged code already treats as "inherit,"
confirmed field-by-field in the Go struct comments and the existing editor:

- `audience.ceiling`, `ordering`: empty string (`policy.go:48`, `:527`; the merged select already
  renders these as "No limit" / "Use channel strategy").
- `scope.era` / `scope.dates`: `nil` (`policy.go:243`, `:382`).
- `scope.runtimeMax`: `0` — a cleared box must send `0`, not `undefined`, documented at
  `channel-policy-fields.tsx:515-534`.
- `separation.movieNoRepeat`/`episodeNoRepeat`/`seriesMinGap`: `undefined` ("no restriction" is
  the real zero state, 0 would mean something else — same file, lines 538-630).
- `separation.blockMax`: `undefined`, same trap in the opposite direction.
- `policy.autoCurate`: presence IS the opt-in (a `*AutoCurate`); Reset removes the key entirely,
  exactly like unticking the checkbox (`channel-auto-curate.tsx:17-57`).
- `seasonal.mode`/`offSeason`: empty string resolves identically to the explicit `"auto"`/`"loop"`
  wire values (`channel-seasonal.tsx:17-24`).

The lineup, rules list and cycle preview are not "defaultable" fields — they are authored content,
not a policy knob — so they carry no source badge, matching the merged component's own
distinction between the lineup editor and `ChannelPolicyFields`.

### Resetting to default

Reset buttons are live in the prototype (not decorative): clicking one sets that field back to
its sentinel value and the pill flips from **Channel override** to **Default**, exercising the
same code path the field's own onBlur/onChange handler already uses for "clear the box" — this
draft only adds the explicit, labelled button in front of it. Auto-curate's Reset also clears its
two threshold overrides, since they have no meaning once the opt-in itself is off.

### Edit, review, then save — and what's actually staged (SOURCE: `-channel-programming.tsx`, `use-channel-rules-draft.ts`, `internal/api/channels.go`)

The merged Programming surface already has **two different save rhythms**, and this draft keeps
both rather than inventing a third:

- **Seamless (immediate, no review step):** the lineup, Era/Dates, ceiling, ordering, separation,
  seasonal and auto-curate all save on blur/change through `onPolicyChange`, which PATCHes
  `/v1/channels/{id}`. "Every edit auto-reconciles — there is no manual rebuild" is the operation's
  own description (`internal/api/channels.go:134`), so there is no staged/applied distinction for
  these fields: what you type is what airs, as soon as you leave the field.
- **Staged draft + Apply/Discard (the review step):** only the **rules** block works this way,
  because rules resolve first-match-by-priority — an intermediate state during authoring is a
  *different* schedule, and inline-saving each step could put something half-finished on air
  (`use-channel-rules-draft.ts:32-47`). The "Rules editor" control switches between **Saved** and
  **Editing (unsaved)**; dirty, the Apply/Discard bar appears exactly as built, and the Preview
  schedule panel switches its banner and rows to show the **draft**, not the saved policy —
  reproducing `draftPolicy={rules.isDirty ? rules.draft : undefined}` (`-channel-programming.tsx:208`)
  and the 300 ms debounce that drives the live re-preview (`use-channel-rules-draft.ts:13`,
  `:73-82`).
- **The "preview of what will change on air and when" the brief asks for already exists** as this
  one mechanism — the cycle preview is a time-travel query ("what airs at `at`, and which rule is
  active") that runs the *same* pure lineup builder reconcile uses
  (`internal/api/channels.go:114`), so preview and reality cannot disagree. It is a single-point
  query, not a diff of upcoming changes; see Open question 3 below.

### Conflicting-override warning (SOURCE: `internal/schedule/lineup.go:739,749`; `channel-policy-fields.tsx:413-502`)

This is the one genuinely new affordance: a warning banner, shown when both **Era** and
**Programming dates** are set at once. Both are independent `ChannelPolicy` fields, and the
engine **ANDs** them — `lineup.go:739` checks `Scope.Era.Contains(year)` and `:749` separately
checks `seriesAiringDateOK(year)`, so a title must clear both at once, which can silently narrow
the eligible lineup to nothing. The built field editor already treats them as exclusive by
clearing one whenever you edit the other (`channel-policy-fields.tsx:434`, `:455`, `:493`) — so
this can't be produced by hand through today's UI. It *can* happen through a refine (the LLM
writes `ChannelPolicy` directly) or a direct API write that sets both independently, and nothing
today tells the operator it happened. The "Heavily customized" preset reproduces that state; the
warning's two resolve buttons ("Keep Era, clear dates" / "Keep dates, clear Era") just call the
same clear-the-other-field logic the built editor already has, surfaced explicitly instead of
silently.

### The three preserved sections (SOURCE: `-channel-programming.tsx:118-196`, redesign-map Programming §)

**What plays**, **How it's ordered** and **When it changes** keep the merged component's exact
block titles, hints and `CollapsibleSection` grouping — including "What plays" opening by default
and the other two closed. Preview schedule remains a fourth, separate `CollapsibleSection` at the
bottom, as built, not folded into "When it changes."

### Desktop and phone

The channel header and tab bar reproduce the merged route's layout (`web/apps/web/src/routes/_authed/channels/$id/route.tsx`,
confirmed in the [#1870 Watch-header draft](watch-header-2026-10-03.md)). At 390px the nav rail
collapses to the existing 56px icon floor (X1 is still undecided, out of scope here — see
[phone-web-nav.html](../prototypes/phone-web-nav.html)); the content column drops its max-width
and the two-column field grid collapses to one column via the existing container query.

## Open questions (not decided by any input)

1. **The defaults-summary wording.** The mock's own text ("1 setting is custom · the rest follow
   Channel defaults") implies every inherited field falls back to a real, household-configurable
   default. In the shipped settings surface, only two Programming-adjacent keys are actually
   configurable per household — `sched.window_hours` and `filler.breaks_per_hour`
   (`web/apps/web/src/routes/_authed/settings/defaults.tsx:13-28`) — and **neither is a field on
   this page**. Every field this draft covers (Era, ceiling, ordering, separation, seasonal,
   auto-curate) falls back to a hardcoded Go zero-value default, not a Settings page value. Alt A
   keeps the mock's literal one-count wording, which reads naturally but overstates how
   configurable "default" is. Alt B splits the count and says so plainly, which is accurate but
   denser. Toggle "Defaults summary wording" to compare; neither the redesign map nor any merged
   code picks one.
2. **Whether the Era/dates conflict should be a warning or a write-time rejection.** This draft
   shows a dismissible warning banner with one-click resolution. An alternative not modelled here:
   reject the write outright (422) when a refine or API call would leave both set, forcing the
   caller to choose instead of letting an operator discover it later on the channel page. Nothing
   in `programming-design.md` or the redesign map settles which; a warning matches the page's
   existing seamless-save philosophy (nothing here blocks a save today), a rejection matches how
   `ScopePolicy`'s *other* mutual-exclusion (`Era`/`EraWindows` on the filler side,
   `policy.go:341`) is already enforced as a validation error.
3. **Whether the cycle preview should stay single-point or grow into a change list.** The brief
   asks for "a preview of what will change on air and when." The only preview that exists
   (`/programming/preview`) answers "what airs at one instant," which this draft reproduces
   faithfully. A multi-row "here are the N upcoming slots this edit changes" view would need new
   backend support (the preview endpoint is a time → one answer, not a diff) and isn't decided by
   the redesign map or `programming-design.md` §8.1. This draft takes the single-point preview as
   given and does not propose the richer surface.

## Not covered here

- The lineup editor's search/add-title picker, `ChannelSeriesScope`, `ChannelCollectionsScope` and
  the rules-authoring token editor itself — all built and unchanged, shown here as static stand-ins
  so the page reads complete.
- "Refine with AI" having no LLM-off state (redesign map's Major critique on Programming) — a
  separate, already-flagged gap in `RefinePanel`, not part of this item's scope.
- The Filler tab's own sandbox draft (`useChannelFillerDraft`) — same debounce and shape as the
  rules draft by design, but a different tab and a different PR.
- Member-facing Programming — the tab is admin-only today (`SECTIONS` filter in `route.tsx`); this
  draft does not add a member view.

## Evidence

Frozen source reviewed: `61d3bd93a4ada210a43893954b48ae170bd26b1a`. Read: issue #1817 item 4 and
the redesign map's Programming (admin) section and Decisions table
([redesign-map-2026-09.md](../redesign-map-2026-09.md#channel-detail)); the merged PR #1739's
`-channel-programming.tsx`, `channel-policy-fields.tsx`, `channel-auto-curate.tsx`,
`channel-seasonal.tsx` and `use-channel-rules-draft.ts`; `internal/schedule/policy.go`'s
`ChannelPolicy`/`ScopePolicy`/`AudiencePolicy` struct comments for sentinel semantics; the Era/dates
AND confirmed at `internal/schedule/lineup.go:739,749`, `internal/schedule/rule.go:227,237` and
`internal/schedule/slotting.go:81,98`; `internal/api/channels.go`'s `UpdateChannel` description
("every edit auto-reconciles"); and `web/apps/web/src/routes/_authed/settings/defaults.tsx` for
which keys are actually household-configurable.

A short local Playwright check drove the standalone HTML headlessly (Chromium, via
`web/node_modules/.pnpm/playwright@1.62.1`): every preset/rules-state/defaults-wording combination
rendered with zero console or page errors; the Era/dates conflict banner appeared on the heavy
preset and cleared after a resolve click; the phone frame toggled; and reset buttons were present
and enabled exactly on overridden fields. This is a prototype layout check, not production
integration, device or accessibility certification.

Local reproduction: open `project/prototypes/programming-defaults.html` directly in a browser
(file://, no server needed).
