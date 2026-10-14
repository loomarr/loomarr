# Manual / AI-off channel creation — draft for review

Draft checkpoint for [#1817](https://github.com/loomarr/loomarr/issues/1817) and parent
[#1659](https://github.com/loomarr/loomarr/issues/1659): "manual / AI-disabled channel creation entry
and recovery, without changing grounding or approval." This is a proposal, not an approved design —
built interactively the way [#1822](https://github.com/loomarr/loomarr/pull/1822),
[#1839](https://github.com/loomarr/loomarr/pull/1839) and
[#1840](https://github.com/loomarr/loomarr/pull/1840) were, following the same mock-draft pattern.

Prototype: [`project/prototypes/manual-channel.html`](../prototypes/manual-channel.html) — standalone,
offline, invented fixtures (no real titles, no remote images), a state switcher for Surface
(Guide / Home), Role (Admin / Member) and Stage, plus a desktop/phone frame toggle.

## What this does not change

Decision G2 and this draft only add an AI-off **path to the existing Proposal**, never a second
creation route. Every stage in the prototype that "creates" or "requests" a channel ends at the same
gate the AI path already uses:

- Admin's own channel: `ChannelSuggestPanel` → `ProposalReview` → `useApproveProposal` (admin-only,
  `web/apps/web/src/suggest/channel-suggest-panel/channel-suggest-panel.tsx:52-65`).
- Member's request: the same panel, `proposal.status === "submitted"`, reviewed later by an admin in
  the approval queue (`web/apps/web/src/suggest/approval-queue/approval-queue.tsx`), same
  `ProposalReview` component, same approve/deny mutations.
- The member channel-ideas request path already does this for its narrower case:
  `POST /v1/discovery/ideas/{ideaId}/request` "puts the idea in the approval queue as the caller's own
  channel request… the idea's name, its pitch and its library titles become the proposal an admin
  approves" (`api/openapi.yaml:12789-12791`).

The mock's "Review" stage is deliberately rendered to look like the real `ProposalReview` component
(same header, status badge, lineup rows, footer buttons, "How these titles were chosen" disclosure) —
reusing it almost verbatim is the recommendation, not a new review surface. See **Open questions**
below for the one piece of that component's copy that doesn't quite fit a lineup with no AI
rationale.

## Sources, row by row

| Choice in the prototype | Source |
| --- | --- |
| LLM-off path exists for Guide, Programming and the wizard's last step | Decision **G2**, redesign map `project/redesign-map-2026-09.md:773`: "With the LLM off: starter templates plus a manual lineup, with a one-line reason" |
| Manual lineup is grounded in library facets and the seasonal calendar first, LLM only enriches | Decision **H5** (`project/redesign-map-2026-09.md:769`), as already implemented in `GET /v1/discovery/ideas` (#1665/#1720): "Built from the library's genres and decades that no channel plays yet, and from holidays on now or within six weeks… No LLM is involved" (`api/openapi.yaml:12770`) |
| Starter template labels/descriptions (4 templates) | `web/packages/core/src/templates/templates.ts` verbatim — the same list the wizard's first-channel step and this panel must share (§13) |
| Picking a template fills name/description but leaves the lineup empty (Option A, not a facet guess) | Matches the wizard's own comment: a template "still hands off an intent" and "picking one still saves a draft" (`web/apps/web/src/wizard/first-channel-step/first-channel-step.tsx:9-25`) — it has never mapped a template to specific titles. Mapping tone/era to a facet would be invented; see **Open questions** |
| Facet chips (genre / decade / holiday), their reason text, counts and availability wording | `ChannelIdeaDTO`/`ChannelIdeaReasonDTO` (`api/openapi.yaml:1273-1370`) and the shipped `ideaReason`/`ideaCount`/`ideaAvailability` functions in `web/apps/web/src/home/channel-ideas/channel-ideas.tsx:49-76` — reused almost word for word ("14 comedy movies, none on a channel yet" pattern) |
| "Search your library or request new titles" box, add/remove rows, In your library / Will be added badges | `ProposalEdit`'s existing add-title flow: `SearchCommand`, `useSearch`, `PickRow` (`web/apps/web/src/components/loomarr/ai/proposal-edit/proposal-edit.tsx`) — same search endpoint, same badges, same row shape |
| Header labels ("Add a channel" admin / "Request a channel" member) and the "same panel, same call, server enforces the gate either way" model | `web/apps/web/src/channels/guide-page/guide-page.tsx:359-384` (existing comment block) |
| Entry points: Guide header button, Guide's "Dead air" empty state, the wizard's handoff via `?intent=` | `project/redesign-map-2026-09.md` Guide section (`:243-291`) + `guide-page.tsx` (`adding` state, `initialIntent`) |
| "Edit first" and "Describe your own channel" on Home's Channel ideas | Explicitly named as **not yet built, waiting on this mock** in the shipped component's own comment: `web/apps/web/src/home/channel-ideas/channel-ideas.tsx:145` ("'Edit first' and 'describe your own' wait on the LLM-off mock, so they aren't here") and the PR #1766 commit message |
| Channel ideas are member-only; an admin creates directly (no ideas grid for admins) | Decision **H4** + `POST /v1/discovery/ideas/{ideaId}/request` description: "Members only (maintainer H4); an admin makes channels directly" (`api/openapi.yaml:12790`) |
| "AI unconfigured" wording ("No AI service is connected…", draft preserved) | `channel-suggest-panel.tsx:135-163`'s existing `aiUnconfigured` branch — the new copy replaces/augments that dead-end rather than inventing new error language |
| Generation-failure recovery actions (Edit description / Try again / Check AI settings) | `channel-suggest-panel.tsx:180-218`, driven by the server's `recoveryAction` enum (`internal/api/proposaljourneys.go:81`) |
| Programming tab's AI-off state is **not** redesigned here | Roll-up #16 and the Programming section both only ask for an LLM-off state on "Refine with AI" (`project/redesign-map-2026-09.md:355`); an existing channel already has a non-approval lineup editor (`channel-lineup-editor.tsx`, "one lineup writer" — binder-owned, no approval round-trip). The recommended fix there is a one-line pointer ("AI isn't set up — edit titles directly below") to that existing editor, not a new flow. Not mocked here because it's a copy change, not a new surface |
| Desktop (1440) + phone (390) frames, review-bar state switcher, invented posters, token colors | Copied structurally from the approved `home-review.html` (shell/nav/frame-toggle pattern) and `native-requests.html` (review bar + fixtures + intercepted clicks pattern) |
| Tokens: canvas `#0B0C0E`, raised `#131519`, elevated `#1B1E24`, hairline `#2A2E37`, signal `#FFB020`, suggest `#D6409F`/`#DC5BAC`, lock `#3DD68C`, tune `#4CC9E8`, onair `#E5484D`/`#E85A5F` | `project/redesign-map-2026-09.md` Tokens section (`:112-120`), reused unchanged — no new tokens needed |

## New pieces this draft proposes (not in any existing mock or shipped code)

1. **The manual lineup builder itself** — starter templates + facet chips + search, feeding one
   working lineup list — is new UI. It is assembled entirely from existing, shipped building blocks
   (table above); nothing in it is a new interaction pattern.
2. **"Build it manually instead"** as a recovery action on a failed AI generation. Today's failure
   actions come straight from the server's Journey (`run.actions`, sourced from
   `recoveryAction`), so this is flagged, not silently added — see open questions.
3. **A resume/continuity moment when AI becomes available** while a manual draft is unsent. No
   existing code models this; two alternatives are shown side by side in the prototype (toggle on the
   "recovered" stage) rather than picking one.
4. **Copy for "How these titles were chosen" on a fully manual proposal.** The existing component's
   fallback ("Loomarr couldn't check this saved draft…") was written for a *stale* AI proposal, not a
   proposal with no AI involvement at all. Two alternatives are shown side by side on the "Review"
   stage.

## Open questions (none decided by the inputs)

- **Recovery-action wiring** (point 2 above): should "Build it manually instead" be a server-provided
  `Journey` action (new value alongside `edit_reference`/`retry_later`/…), gated per failure reason —
  or always offered client-side regardless of the failure? The prototype defaults to **always
  client-side** (no backend change needed) and names the alternative in its own review note.
- **AI-recovers-while-a-manual-draft-is-open** (point 3 above): Alt A shows a dismissible banner
  ("AI is connected now — try AI suggestions, or keep building manually"); Alt B makes no interruption
  and instead treats the manual lineup as a starting point an AI run would add to later. Neither is
  implied by G2/H5; both are shown on the prototype's "recovered" stage via a labelled toggle.
- **Manual-proposal copy** (point 4 above): Alt A keeps today's fallback string verbatim (lowest
  risk, no behaviour change); Alt B proposes manual-specific copy ("You picked every title in this
  channel yourself — there's nothing to check."). Shown on the "Review" stage via a labelled toggle.
- **Template → facet mapping**: this draft deliberately does *not* let a starter template
  pre-populate the lineup from a guessed genre (e.g. "Cozy Mystery Nights" → the Mystery facet),
  because nothing in the inputs defines that mapping and templates only carry `era`/`tone` text, not
  a genre value. The template just fills the name/description, same as the wizard does today. If the
  maintainer wants that link, it needs an explicit template→facet table — out of scope for this
  draft.
- **Channel Programming's "Refine with AI" AI-off state** is intentionally left as a one-line
  pointer to the existing direct lineup editor rather than mocked as its own flow (see table above) —
  flagging in case the maintainer wants it mocked explicitly in a follow-up.

## What's covered

- Entry points when AI is off: Guide header button, Guide's "Dead air" empty state, Home's Channel
  ideas "Describe your own channel" row, and "Edit first" on an idea card.
- Picking a starter template (admin and member wording).
- Building/editing a manual lineup from the library: facet chips (genre/decade/holiday, grounded in
  `/v1/discovery/ideas`) and direct search/add, with remove.
- Recovery when a generation fails (existing failure UI + the new manual fallback) and when AI comes
  back later while a manual draft is open (two alternatives).
- Admin vs. member: header/button copy, self-review vs. "sent for approval," and the H4 role gate on
  Channel ideas.
- Desktop (1440px) and phone (390px) frames for both surfaces.

## Verification

- `node --check` on the extracted `<script>` block (syntax only — the gate this repo's docs checks
  run; there's no Vitest/TS here, it's a standalone file).
- A short local Playwright pass against the pinned Chromium already cached at
  `~/.cache/ms-playwright` (no `chrome` binary at `/opt/google/chrome` on this machine, so the
  `playwright` MCP tool itself couldn't launch; drove `playwright-core` directly instead, matching
  #1822's "short local Playwright check of the standalone prototype only"): loaded the file, stepped
  through all ten Surface/Stage combinations and confirmed each renders non-empty content with zero
  page/console errors, exercised a facet-click → add-title → review handoff, and took screenshots of
  the builder, review and Channel-ideas (desktop + phone) stages for visual sanity. Full fe-visual/e2e
  suites were not run (standing rule: those are CI's, not local).
