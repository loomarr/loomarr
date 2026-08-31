# Client design-system reconciliation

**Date:** 2026-08-30
**Snapshot:** `origin/main` at `f7ed19fbb60f507b552bc2f30b13e6cb8315b07c`; PR #581 at
`5c903446f65fc049d99083a5d333a360a92200f3`; PR #607 at
`1e3a7edabdd47cf023cf2747dc9435284a553df7`
**Scope:** shared design system, production adoption, branch reconciliation, acceptance evidence,
and existing GitHub work. This is a read-only audit; it does not amend the plan or open issues.

## Executive conclusion

The shared-client effort is not lost, but its three layers are at different maturity levels:

1. **P1–P3 are on `main`.** The shared visual foundations, platform-neutral Guide rules, and paired
   mobile/TV shells merged through [#534](https://github.com/loomarr/loomarr/pull/534),
   [#535](https://github.com/loomarr/loomarr/pull/535), and
   [#538](https://github.com/loomarr/loomarr/pull/538).
2. **P3.5 exists only in draft PR #581.** Git patch comparison finds no #581 feature commit already
   applied to `main`; the branch is 40 first-parent commits behind, GitHub reports it conflicting,
   and its own ledger still marks 8 capabilities partial, 2 legacy, and 1 claimed despite saying
   the implementation gate is complete. [PR #581](https://github.com/loomarr/loomarr/pull/581),
   [coverage ledger on the branch](https://github.com/loomarr/loomarr/blob/client-design-system-completion/docs/engineering/client-design-system-coverage.md)
3. **The production native vertical slice is implemented on top of #581, not beside it.** Draft
   [#607](https://github.com/loomarr/loomarr/pull/607) targets `client-design-system-completion` and
   adds the player package plus real Watching, Guide, and Surf journeys. It therefore cannot be
   used to bypass or replace the #581 reconciliation without deliberately restacking its work.

The correct recovery path is to salvage and rebase #581, narrow or explicitly exclude its partial
rows, collect the required iPhone workshop proof, then rebase #607 and finish the adoption evidence.
Starting a third implementation branch would create a second source of truth over the same package,
lockfile, Storybook, and baseline seams.

## What landed independently

No P3.5 feature commit is patch-equivalent to a commit on current `main`. `git cherry
origin/main origin/client-design-system-completion` reports every non-merge #581 commit as `+`.
Source inspection agrees: `main` has no `ui-tv` package and no shared layout, interaction,
selection, viewport, StatePanel, identity, overlay, Guide surface, or Surf rail modules. The
production web source imports the shared visual packages only from the isolated client-platform
proof and Storybook stories, not from a shipping route.

Related work did land after #581's last merge base, but it is maintenance around the stack rather
than an independent P3.5 implementation:

- [#650](https://github.com/loomarr/loomarr/pull/650) aligned Expo SDK 57 patch versions and changed
  app manifests and the pnpm lockfile.
- [#569](https://github.com/loomarr/loomarr/pull/569) and
  [#571](https://github.com/loomarr/loomarr/pull/571) activated separate Apple mobile and Apple TV
  CI impact decisions.
- [#630](https://github.com/loomarr/loomarr/pull/630) added bounded image-download retries, and
  [#700](https://github.com/loomarr/loomarr/pull/700) changed the retained FFmpeg artifact. Both
  postdate the image-download failure currently shown on #581.

The shipping Compose TV implementation also continued to exist on `main`; it was not displaced by
these branches. Its current code still owns Guide, playback, pairing, navigation, and design under
[`android/app/src/main/java/loomarr/media`](https://github.com/loomarr/loomarr/tree/f7ed19fbb60f507b552bc2f30b13e6cb8315b07c/android/app/src/main/java/loomarr/media).

## Divergence and merge risk

The numerical divergence is 40 commits reachable only from `main` and 31 reachable only from #581,
with merge base `114ecbb3ca33d6e9fb66968daafe782b2535ca7b`. The risk is manageable but not merely a textual
`PROGRESS.md` conflict:

| Seam | Current conflict or semantic choice | Required resolution |
| --- | --- | --- |
| `PROGRESS.md` | Both sides rewrote active-workstream truth. | Keep current `main` rows, then add a current, evidence-bounded client row. Do not resurrect stale filler/discovery claims from #581. |
| `docs/design.md` | #581 adds `@storybook/addon-themes`; `main` has since refreshed the surrounding dependency contract. | Retain the theme decision only after aligning it with current versions and the current §14 text. |
| package manifests and `pnpm-lock.yaml` | #581 was authored around Storybook 10.5.8, TypeScript 6, and earlier Expo patches; `main` pins Storybook 10.5.10, TypeScript 7, Vite 8.2.2, and Expo 57.0.18-era packages. | Resolve from current manifests, regenerate the lockfile, and re-run package/public-interface gates. Do not accept the branch's older lock entries. |
| Storybook configuration and baselines | #581 changes the global provider/theme decorator and many existing baselines. `main` has since changed People and other web stories. | Rebuild against current `main`; review regenerated diffs as new evidence. Old #581 images are not proof for newly changed stories. |
| fixtures/UI package | Both lines changed `testcard` fixtures and `@loomarr/ui` package metadata. | Preserve current generated/API-shaped fixtures and add only the shared UI test dependencies still required. |
| client build scripts | #581 serializes native JS bundles; later CI and Expo work altered affected lanes and package versions. | Preserve current fail-closed CI classification while keeping serialization where resource evidence still requires it. |

The branch has been merged from `main` repeatedly rather than rebased, and #607 contains still more
merges from #581. Reconciliation should therefore be judged by the resulting tree and gates, not by
trying to preserve every historical merge commit.

## Production adoption matrix

| Shipping surface | On current `main` | In #581 | In stacked #607 | Remaining adoption work |
| --- | --- | --- | --- | --- |
| Embedded administrative web app | Still Tailwind v4, local shadcn-style/CVA primitives, and Base UI. The real Guide delegates geometry and navigation rules to `@loomarr/core`, but no production route renders `@loomarr/design-system` or `@loomarr/ui`. [web package](https://github.com/loomarr/loomarr/blob/f7ed19fbb60f507b552bc2f30b13e6cb8315b07c/web/apps/web/package.json), [Guide route](https://github.com/loomarr/loomarr/blob/f7ed19fbb60f507b552bc2f30b13e6cb8315b07c/web/apps/web/src/routes/_authed/guide.tsx) | Adds browser adapters and exhaustive workshop stories, not route migration. | Moves the web HLS transport behind `@loomarr/player/browser`, but leaves web presentation and administrative routes legacy. | Route-by-route migration of viewer and administrative presentation; forms, menus, accessibility, responsive parity, and removal of the legacy styling stack remain later adoption work. |
| Mobile Expo app | Production entry point pairs securely and renders shared `PairingShell` and `ClientShell`; Watching, Guide, and Surf are labels over a placeholder body. [mobile entry point](https://github.com/loomarr/loomarr/blob/f7ed19fbb60f507b552bc2f30b13e6cb8315b07c/web/apps/mobile/app/index.tsx), [placeholder shell](https://github.com/loomarr/loomarr/blob/f7ed19fbb60f507b552bc2f30b13e6cb8315b07c/web/packages/ui/src/client-shell/client-shell.tsx) | Improves navigation, overlays, safe areas, and shared viewer presentation modules; destinations remain placeholders. | Connects real generated API data, player transport, Guide, Surf, artwork, SSE reconciliation, and lifecycle handling. | Real-iPhone portrait/landscape workshop and production journey proof, including rotation, safe areas, touch traversal, time-shift, and recovery. |
| Expo TV app | Production prototype pairs securely and renders the same placeholder shell. [TV entry point](https://github.com/loomarr/loomarr/blob/f7ed19fbb60f507b552bc2f30b13e6cb8315b07c/web/apps/tv/src/app.tsx) | Adds Guide/Surf controllers and workshops; it does not connect the production destinations. | Connects real Watching/Guide/Surf, player, focus registry, number entry, artwork, and SSE; emulator journey is recorded. | Corrected physical-Shield visual acceptance, populated focus/time-shift soak, background/foreground, Back and number-key proof, and the physical Macrobenchmark. |
| Shipping Compose Android TV app | Remains the released implementation with permanent package identity, pairing, Guide, Surf/Watching, Media3 playback, screenshots, and release/update pipeline. | Intentionally untouched. | Intentionally retained as rollback; adds a read-only credential migration adapter and a guarded React Native production renderer. | Retire only after an adopted vertical slice plus Play in-place update, pairing preservation, rollback, listing/review, protected signing, and final release evidence. |

This distinguishes **shared module availability** from **production adoption**. #581 is primarily a
library/workshop PR. #607 is the first substantial production consumer of those modules. The broad
web migration has not started.

## The completeness-ledger contradiction

The #581 ledger says P3.5 is complete only when every row is proven or carries a
maintainer-approved exclusion with a named later owner. Its table currently contains:

- 9 `proven` rows;
- 8 `partial` rows;
- 2 `legacy` rows; and
- 1 `claimed` row.

The unresolved rows are not cosmetic. They include production navigation, Guide and TV mechanics,
native modal behavior, Surf, touch mechanics, browser application semantics, player transport, and
player chrome. Several exit cells explicitly ask for P4/P6/P7-style production and hardware proof.
At the same time, the document header says “implementation and automated/rendered gates complete,”
and the PR title says “complete shared component library.”

There are two coherent ways to repair this:

1. **Recommended:** define P3.5 as library/interface completeness, record explicit maintainer
   exclusions for production integration and hardware behavior, and name #607/P4, viewer migration,
   web migration, and retirement as their owners. Under this reading, `partial` is allowed only for
   the excluded acceptance dimension, not for a missing shared interface promised by P3.5.
2. Expand #581 until every row is `proven`. This would absorb much of #607 and the later migration
   phases, making the present branch/phase split misleading and producing an unnecessarily large
   merge.

The second option conflicts with the migration plan's deliberate P3.5 → P4 → P5 adoption gate and
with #607's existing dependency structure. The first should be recorded explicitly before calling
PR #581 complete. [Migration delivery sequence](https://github.com/loomarr/loomarr/blob/client-design-system-completion/docs/engineering/plans/shared-client-platform.md#delivery-sequence)

## Remaining acceptance and CI blockers

### PR #581

- GitHub currently reports the PR as draft, `DIRTY`, and conflicting with `main`.
- Its declared draft blocker is real-iPhone portrait and landscape workshop evidence. The ledger
  requires safe areas, targets, scrolling, gestures, keyboard, and focus-transfer evidence, not
  merely a successful iOS bundle.
- The most recent run passed frontend, shared clients, all four Playwright shards, docs, Go, and
  arm64 image. amd64 failed while downloading an image dependency with `curl` connection-reset/
  HTTP2-cancel errors, and the aggregate consequently failed. [CI run 33130264916](https://github.com/loomarr/loomarr/actions/runs/33130264916)
- Because `main` has since merged download retries, an FFmpeg pin, dependency refreshes, new web
  stories, and CI orchestration changes, the old green sub-jobs cannot certify the reconciled head.
  A current protected run is required after rebase and baseline review.
- The ledger's completion semantics must be fixed as described above.

### PR #607

- #607 is draft and depends on #581; it is 40 `main` commits behind through the same stack.
- Its recorded emulator journey is useful P4 evidence, but its own checklist still requires:
  corrected physical-Shield presentation; populated focus/time-shift soak; Shield background/
  foreground behavior; physical Macrobenchmark; protected upload-key build; Play in-place update,
  pairing preservation, rollback and adoption; and final serialized gates/CI.
- Its last CI similarly failed only the amd64 image download while the client/frontend, emulator,
  Android TV, and other selected jobs passed. [CI run 33130942049](https://github.com/loomarr/loomarr/actions/runs/33130942049)
- It must not be called complete merely because it contains the P4 implementation. Its branch ledger
  correctly describes player, Guide, Surf, TV, overlay, navigation, and touch rows as partial until
  real-device evidence closes them.

## Existing work and overlap

No open GitHub **issue** specifically tracks the shared design-system reconciliation, native
vertical-slice adoption decision, broad web migration, or Compose retirement. The work is currently
represented by draft PRs and the plan:

- [#581](https://github.com/loomarr/loomarr/pull/581): P3.5 library/interfaces/workshop;
- [#607](https://github.com/loomarr/loomarr/pull/607): production native player and
  Watching/Guide/Surf vertical slice, stacked on #581;
- [#622](https://github.com/loomarr/loomarr/pull/622): Expo Android mobile CI activation, stacked on
  #607;
- [#625](https://github.com/loomarr/loomarr/pull/625): Expo Android TV CI activation, stacked on #622.

Do not create new issues that independently reimplement player transport, native Guide/Surf, or
Android client CI activation: those would duplicate #607/#622/#625. Also keep general CI redesign
out of this effort because [#680](https://github.com/loomarr/loomarr/issues/680) already owns the
fast-PR/merge-queue/publish lane refactor, and keep Apple compilation caching under
[#718](https://github.com/loomarr/loomarr/issues/718).

## Suggested issue boundaries

These are outcome-oriented tracking boundaries, not duplicate implementation plans:

1. **Reconcile and publish the shared client UI contract.** Own #581's rebase, current dependency
   resolution, baseline regeneration/review, truthful ledger exclusions, iPhone workshop evidence,
   and current protected CI. It closes when #581 merges; it must not absorb production playback.
2. **Record the Guide-to-playback adoption decision.** Own the remaining #607 real-iPhone and
   physical-Shield journey evidence, performance report, source-sharing/deletion test, and explicit
   `adopt`, `revise and repeat`, or `reject` decision. Link #607 rather than restating its already
   implemented player/native work.
3. **Migrate production web surfaces in reviewable cohorts.** Start only after `adopt`; inventory
   routes by shared target and migrate cohorts with authorization, form, keyboard, screen-reader,
   responsive, visual, and rollback evidence. The viewer route should be separated from
   administrative forms because they exercise different adapters and acceptance gates.
4. **Close remaining viewer parity and retire the legacy presentation.** Own viewer surfaces beyond
   the first slice and, only after parity, Compose/Tailwind/shadcn/Base UI retirement, protected Play
   signing, in-place update, pairing preservation, rollback, store review, and retired-identifier
   checks. Keep retirement blocked on the recorded adoption decision.

The first two issues are immediately actionable. The latter two should be marked blocked by the
adoption decision so issue state does not imply authorization to begin a full rewrite.

## Recommended next sequence

1. Rebase #581 onto current `main` and resolve from current package manifests, lockfile, docs, CI,
   fixtures, and baselines.
2. Amend the ledger so every non-proven row has an explicit, maintainer-approved later owner and a
   precise excluded acceptance dimension.
3. Capture real-iPhone portrait/landscape workshop evidence and run the current full publication
   gate; merge #581.
4. Restack #607, #622, and #625 in order, preserving current CI authority.
5. Finish #607's real-device/performance/distribution evidence and record the P5 decision.
6. Only after `adopt`, schedule production web/viewer migration and final legacy retirement.
