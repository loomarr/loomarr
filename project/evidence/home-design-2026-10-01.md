# Home design review candidate

Review checkpoint for [#1815](https://github.com/loomarr/loomarr/issues/1815),
[#1817](https://github.com/loomarr/loomarr/issues/1817) and parent
[#1659](https://github.com/loomarr/loomarr/issues/1659). This is a proposal, not an approved
behaviour change. Production components, routes and API contracts are unchanged.

Open [the interactive prototype](../prototypes/home-review.html) directly in a browser.
Its controls switch role, household state, arrival count, new-channel presence and a 390px frame.
All fixtures and abstract artwork are invented. Links report their proposed action without
calling a server. The prototype illustrates content within the existing navigation; it does not
settle the separate phone-navigation decision.

## Problem and proposed composition

The current Home can hide every viewing section when a household has channels but no active
sessions, highlights or recent additions. On a 390px phone, a minimum-width new-channel card
competes with five poster columns, leaving poster slivers. Current lifecycle and last-tune copy
also implies playing or resumable content beyond the supplied evidence.

Keep the web-mock Home and make its order:

1. Neutral channel inventory count.
2. Optional **Recently tuned**, using the last settled channel and its current programme.
3. **On now**, providing immediate choices even in a quiet household.
4. Actual **Watching now** sessions, followed by typed **Tonight** highlights.
5. **New this week**, with new channels separate from the poster grid.
6. Existing role-specific request/idea sections and the admin settings footer.

Show up to six current scheduled live channels in channel-number order, using existing Guide
facts. A **Full guide** link provides the complete timetable. Never duplicate choices to fill a
row. If channels exist without a current playable block, show **Your channels**, actual lifecycle
or schedule-gap wording, and **View channel**, rather than a first-channel prompt.

The return card says **Recently tuned**, **Join what’s on now.**, and **Watch live**. It does not
promise saved episode progress. Hide it if its channel no longer exists, or the same channel is
already represented by an active own-viewing card. **Watching now** means observed sessions;
member fixtures show only their own identity. A viewing count labels devices unless actual
unique-user aggregation supports a people count. Media-server Live TV attribution remains outside
this contract. Future highlights retain their schedule time and explain that watching joins the
channel live, not the future title.

## Geometry and artwork

Use current semantic charcoal and amber tokens. Amber marks the primary action and selected
navigation; keyboard focus has a separate 2px ring. Programme identity sits below artwork.

| Surface | Desktop | Phone within the existing 56px rail |
| --- | --- | --- |
| Content | Maximum 1120px outer width, 24px padding | 16px padding; about 302px available at 390px |
| Current channels | Three columns; two at intermediate widths | One column; artwork remains 16:9 |
| Recent posters | Up to five columns, 136–180px each | Two columns about 145px each; one below 284px content width |
| New channel | Separate row above posters | Separate row, wrapping text and action |
| Spacing | 32px between sections; 12px within grids | 24px between sections; 12px within grids |

Poster images retain a 2:3 ratio. A title with channel membership offers **Watch [channel] live**;
without membership it remains a static **In your library** card. Arrival identity does not grant
on-demand playback. Membership choice and item actions are proposed decisions for review.

Missing or failed artwork keeps its intended geometry, neutral surface and a restrained line
motif with a channel monogram where available. Metadata remains visible. No repeated large
missing-art messages, broken-image glyphs or invented photography. Supplied image-service
placeholders may represent pending images; missing metadata cannot manufacture them.

## State contract for review

- Quiet household: inventory plus actual **On now** choices; omit successfully empty optional sections.
- Successful zero channels: **Your first channel starts here**, with an admin creation action or
  member request action using the existing workflow. Do not infer missing server setup from zero channels.
- Loading: **Loading your channels…**, stable skeleton geometry, and no premature first-run action.
- Initial error: **Couldn’t load your channels.** and **Try again**.
- Partial error: retain known channel choices and give the failed section its own retry notice.
- Cached refresh failure, for eventual implementation: retain valid data and state that it is the
  last update; do not label an expired programme as current.

The prototype covers populated, quiet, first-run, no-current-programme, missing-art, long-title,
loading, initial-error and partial-error fixtures. It illustrates role-specific requests but does
not implement live data, full idea generation, all cached-data transitions or server authorization.

## Alternative and authority

A dense all-channel console was considered and is not recommended: it makes quiet households
usable, but makes larger households a scanning task, gives phone layouts another table to collapse,
and mixes scheduling, lifecycle and observed viewership. The proposed bounded card selection leaves
exhaustive browsing to Guide. This recommendation is not a recorded maintainer rejection.

The September 30 review requires reviewed mocks before novel behaviour changes. The canonical
Home passage in `docs/design/web-ui.md` still describes an admin machine-state dashboard, while
current Home and shell source differ. Reconcile that conflict, the proposed section order,
truthful copy, item actions and fallback treatment in the implementation PR after the design decision.
Preserve the separate phone shell, Watching, Guide, TV, release and hardware work.

Implementation seams are the existing `web/apps/web/src/home/` modules, shared public UI and
design-system interfaces, and `docs/design/web-ui.md`; no new package or data authority is needed.

## Evidence

Frozen source reviewed: `e7fe39e1ac419155f8f72e029ae8d31815275d88`. Existing issues #1815,
#1817, #1785, #1659 and delivered viewing/arrival/highlight issues #1662–#1664 were read.
The read-only design worker returned a concrete specification; the parent authored this prototype.

Seven local Playwright checks passed against the standalone HTML: admin/member at 1440, 390 and
320px, each across all nine states; arrival counts 0/1/2/5/14; new-channel removal; actual poster
widths; and ancestor clipping via the repository's `findClippedContent` helper. A separate check
covered 640px reflow, reduced-motion preference and visible keyboard focus. The 390px poster
artwork measured 144px wide inside the prototype's bordered frame. These are prototype layout
checks, not production integration, device or accessibility certification.

Local reproduction: serve `project/prototypes/` with a static server and open `home-review.html`.
The development capture/check scripts and screenshots are retained under `.artifacts/home-review/`.
No private production screenshots or assets are included.
