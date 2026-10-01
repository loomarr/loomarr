# Beta.9 Guide preview correction

The approved [#1814](https://github.com/loomarr/loomarr/issues/1814) direction restores the
full desktop timeline and replaces its permanent detail column with programme and filler
previews anchored to the initiating block. The behaviour contract is in
[Web UI](../../docs/design/web-ui.md). Home/phone redesign #1815, unresolved mocks #1817,
and iPhone Simulator/TestFlight preparation #1816 remain separate work.

## Implementation and review

The implementation checkpoint is `c7e80da4aaded563c42ff92fa82542bdbb540b65` over
`924cbe122b76a85ce75c0b6848dfc7279766fee8`. A subsequent visual correction,
`dae2752a0`, retains the existing selected-detail outline in touch/TV adapters while
contextual desktop previews leave focus on their initiating blocks. The already-merged
planning documentation from #1818 is integrated into this branch; this does not advance main.

The preview group owns 250 ms entry intent, a 180 ms pointer-transfer grace period, one
active preview, Escape dismissal, viewport clamping and stale-anchor/scroll cleanup.
Keyboard focus uses the same preview without moving focus. Existing Guide movement,
channel-number selection and Enter navigation remain in place. Supplied filler pod
names and kinds appear without inventing clip identity or requiring a new API.

Codex workers ran in visible Orca panes with bounded assignments. Per-terminal
`codex --no-daemon` initialization and an explicit local-network-capable sandbox resolved
the readiness and coordinator-connectivity failures. No global permission settings changed.
Effective Terra/Medium sessions and their native goals were checked independently. Because
hard in-flight budget enforcement was not established, workers performed read-only planning
and review; the supervisor was the sole product-code writer.

| Assignment | Native budget / final usage | Outcome |
| --- | --- | --- |
| Read-only readiness probe | 100,000 / 18,983 | Paused and stopped after sandbox connectivity failed; no product edits |
| Read-only design/seam analysis | 100,000 / 62,044 | Complete; keyboard-focus previews retained as required |
| Fresh Standards and Spec review of `c7e80da4a` | 150,000 / 79,828 | Complete; no reproducible P1/P2 findings |

Each worker's cessation and final meter were checked before its exact pane was closed.
The review is code evidence, not physical-device or production-playback certification.

## Reproduction and evidence

The new `guide-preview.spec.ts` uses 100 invented channels and public-safe programme/filler
names, with the existing mock API harness. It covers 900, 1280 and 1920 px desktop frames,
right/bottom-edge placement, pointer transfer, Escape without reopening, keyboard movement,
a two-minute filler block narrower than 20 px, channel-100 typeahead and Enter navigation.
Its four browser checks passed against this worktree's Vite server. Screenshots are emitted
through Playwright's output path, including `guide-1280.png` and `guide-short-filler.png`.

Six preview lifecycle unit tests and 17 Guide route tests passed. The existing phone Guide
journey at 390 px passed for compact-grid filters, tapped selection, clipping and Watch
navigation. Package type checks passed for design-system, UI and Web. The pinned Linux
Guide visual suite covers 34 desktop/mobile cases; only the two Guide Grid Keyboard
baselines change to remove the permanent side column. Touch and TV adapter baselines remain
unchanged. The final affected verification gate and protected CI results belong to the PR.

For interactive fixture review, build Storybook and open
`/?path=/story/loomarr-components-guide-grid--hundred-channels` on this worktree's Storybook
server. Hover a programme or focus its button to inspect the contextual preview. This is a
fixture preview, not an authenticated backend or playback environment.

## Delivery boundary

The beta.8 merge hold remains in force. This branch must not auto-merge while that hold is
active. No hardware claims, devices, release tags, deployments, performance thresholds or
acceptance baselines were changed. The Guide worktree is retained for maintainer review;
its next retirement review is after #1814 is accepted/merged or explicitly abandoned.
