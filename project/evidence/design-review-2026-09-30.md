# September design review

Review snapshot: `924cbe122b76a85ce75c0b6848dfc7279766fee8`, 2026-09-30.

The broadcast identity is a strong foundation. The main weaknesses are viewing hierarchy,
responsive composition, supported empty states and disagreement between references and current
behaviour. Keep the charcoal surfaces, amber accent and channel identities while correcting those
interactions.

This records findings, not a new layout authority. The delivery sequence and acceptance criteria
live in [the beta.9 tracking issue](https://github.com/loomarr/loomarr/issues/1659#issuecomment-5921802651).
The existing [redesign map](../redesign-map-2026-09.md) and subsequent maintainer decisions identify
approved references. Private exports and their screenshots remain outside the public repository.

## Scope and limits

The review inspected the imported Web, Home alternatives, TV/iOS and Watch console references.
Browser captures covered 16 Web mock surfaces at desktop and phone widths, eight Home role/state
combinations, six channel/filler tab states, six Home alternatives and eight native frames. Current
Web review exercised the 23-route shell inventory at desktop and phone widths, with follow-up
captures, populated Home, creation, channel Watch/info/filler/danger and 100-channel Guide fixtures
at 900, 1280 and 1920 pixels.

Current renders used Vite, repository mock APIs and synthetic HLS. They establish frontend behaviour,
not production playback or native-device parity. Current Programming rendering was incomplete
because the generic fixture omitted preview fields; its source and mock were reviewed, and fixture
failures were excluded from product findings. Full authentication/onboarding journeys, physical
devices, light theme, complete screen-reader operation and zoom/reflow coverage were not certified.

Axe reported no selected WCAG A/AA rule violations on six sampled current desktop routes: Home,
Guide, Requests, Settings, People and Account. Mock Guide and Requests produced contrast findings;
export wrappers also produced document and nested-control findings that require isolation before
being treated as product defects. Guide keyboard movement was sampled. Automated results do not
establish complete accessibility.

## Findings and ownership

| Surface | Finding and recommendation | Tracking |
| --- | --- | --- |
| Desktop Guide | The 360px permanent preview compresses the timeline. Its ProgrammeCard is always marked focused. Restore full-width browsing and anchored programme/filler previews on hover and keyboard focus, with pointer transfer, Escape and viewport-edge handling. The maintainer approved this direction. | [#1814](https://github.com/loomarr/loomarr/issues/1814) |
| Home | With channels available but no viewing activity, highlights or additions, the composition can become almost empty. Design dependable viewing choices for a quiet household and a distinct first-channel state. Review ready/playing/viewer-count wording against actual linear playback. | [#1815](https://github.com/loomarr/loomarr/issues/1815) |
| Phone Home | At 390px, New this week keeps a minimum 200px channel card beside five poster columns inside 286px. Posters collapse into slivers without horizontal overflow. Use readable card geometry and explicit item actions. | [#1815](https://github.com/loomarr/loomarr/issues/1815), [#1785](https://github.com/loomarr/loomarr/issues/1785) |
| Watch | Editable identity fields, management tabs and framing compete with the player. Review a viewer-first header with a clear management entry. Preserve approved phone Watching and the decision to remove the mini-player. | [#1817](https://github.com/loomarr/loomarr/issues/1817), [#1785](https://github.com/loomarr/loomarr/issues/1785) |
| Visual system | Keep the identity; clarify primary action, actual focus, selection and warning semantics. Prioritise readable titles, reserve small mono text for secondary information, and choose width and containers by task. | [#1817](https://github.com/loomarr/loomarr/issues/1817), [#970](https://github.com/loomarr/loomarr/issues/970) |
| Artwork | Repeated missing-art panels make valid states look unfinished. Design an intentional shared fallback and sensible aspect ratios. Blank slots in exported mocks do not prove the intended photography was absent. | [#1815](https://github.com/loomarr/loomarr/issues/1815), [#1817](https://github.com/loomarr/loomarr/issues/1817) |
| Requests and creation | Keep Needs you / In progress / Done. Show meaningful approval consequences and bulk scope before optional detail. Complete the manual/AI-disabled creation entry without weakening grounding or approval. | [#1817](https://github.com/loomarr/loomarr/issues/1817) |
| Programming | Keep What plays / How it is ordered / When it changes. Clarify effective defaults, overrides, unsaved changes and when schedule changes take effect. Full current interaction evidence remains outstanding. | [#1817](https://github.com/loomarr/loomarr/issues/1817) |
| Filler | Keep Overview / Library / Manage. Make healthy operation quiet, exceptions actionable, coverage understandable and subordinate-page return paths explicit. Review clip-shaped library imagery. Some current colours already improve on the exported mock. | [#1817](https://github.com/loomarr/loomarr/issues/1817) |
| Administration | Keep searchable Settings. Clarify inheritance and Save/autosave feedback; make Invite primary in People; use consequence/action language in Account and contextual links in Help. | [#1817](https://github.com/loomarr/loomarr/issues/1817) |
| Pairing and onboarding | Complete pending, expiry, invalid-code, success and recovery frames. Distinguish necessary setup from optional integrations and provide a coherent manual route. | [#1817](https://github.com/loomarr/loomarr/issues/1817) |
| TV, phone and tablet | TV Watching/Guide/Surf has the clearest hierarchy. Validate remote focus and real playback separately. Phone selection must identify the channel and distinguish tuning live from a future programme. Tablet split remains a future candidate, not an approved scope change. | [#1817](https://github.com/loomarr/loomarr/issues/1817), [#1500](https://github.com/loomarr/loomarr/issues/1500) |
| Accessibility | Complete keyboard/focus, screen-reader, target geometry, 200% reflow, long-label and reduced-motion evidence. A tall row does not make a narrow filler block an adequate target. | [#1814](https://github.com/loomarr/loomarr/issues/1814), [#1817](https://github.com/loomarr/loomarr/issues/1817) |
| Consolidation | Canonical Web prose still describes an admin-only machine-state Home. Align behaviour docs, reference overrides and implementations. Shared primitives should support distinct pointer/touch/remote layouts. Legacy ledger allowances are not a fresh usage measurement. | [#970](https://github.com/loomarr/loomarr/issues/970), [#1817](https://github.com/loomarr/loomarr/issues/1817) |

The strongest concrete reproductions are in `guide-page.tsx`, `guide-detail.tsx`,
`home-page.tsx`, `home-strip.tsx` and `new-this-week.tsx`. The local evidence bundle contains the
reference hashes, browser scripts, fixture geometry, screenshots and raw accessibility results.
Public implementation evidence must use invented media rather than private reference content.

## Maintainer direction

The maintainer requested beta.9 planning and parallel agents after reviewing these findings, then
clarified the priority: **redesign first, iPhone app second, with independent work in parallel**.
The iPhone delivery order is **Simulator first, then TestFlight**.

[The early iPhone preview](https://github.com/loomarr/loomarr/issues/1816) is a bounded beta.9
checkpoint. It does not move the entire beta.10 device programme into beta.9. Reuse and review the
existing native Guide and Watching PRs before adding overlapping implementation. A runnable
simulator artifact and install/launch evidence are separate from signing, TestFlight and real-device
acceptance. Read-only preparation may run alongside the redesign; device rollout does not lead it.

Novel layouts and unresolved reference conflicts still require reviewable mocks and recorded
decisions under #1659 before implementation. Existing approval, authorisation, release and hardware
gates remain intact. The active beta.8 hardware checkpoint retains its resources and release scope.
