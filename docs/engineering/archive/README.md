# Archive — superseded plans and dated records

**Nothing in this directory is an instruction.** These are records of work that has shipped, or
of research that informed it. They are kept because they answer *why* a decision was made — the
question git history answers badly — and moved here because they were answering it in the
present tense, which made them read as work still to do.

That was a real cost, not a tidiness concern. `playout-prior-art.md` opened with "Read before
building V6" long after V6 shipped. `v2-build-plan.md` said "ready to execute" with roughly forty
of its phases already recorded done in `PROGRESS.md`. `frontend-build-plan.md` planned, in future
tense, the pnpm monorepo you can go and read today.

**For current truth, go to `docs/design.md`.** For phase status, `PROGRESS.md`. If an archived
document disagrees with either, the archived one is wrong — that is what being archived means.

| Document | What it recorded | Superseded by |
| --- | --- | --- |
| `v2-build-plan.md` | The 39-phase v2 programme, decided 2026-07-24 | `PROGRESS.md` — the phase rows carry real status; most are done |
| `frontend-build-plan.md` | Phase 13's sequencing and locked decisions | The frontend it planned; `docs/frontend-design.md` for the design system |
| `playout-prior-art.md` | Tunarr / ErsatzTV research read before building internal playout | `docs/design.md` §9.1 — the design that came out of it |
| `playout-prior-art-viewra.md` | Lessons from mantonx/viewra, mostly warnings | Same |
| `design-mock-review-2026-07-20.md` | Screen-by-screen build-vs-prototype review | The v2 mocks; `design/README.md` |
| `surface-audit-2026-07-26.md` | Capabilities reachable in the API but not the UI | Closed by the PRs it prompted; re-run `/surface-audit` for a current answer |
| `first-beta-readiness.md` | The first-public-beta ship contract and blocker ledger | Published beta releases and `PROGRESS.md` |
| `release-native-arm64-2026-08-18.md` | Native per-architecture release image plan | `.github/workflows/release.yml` and its verifier |
| `durable-first-channel-workflow.md` | Durable proposal-workflow delivery plan | The shipped workflow and `PROGRESS.md` evidence |
| `v54-filler-refresh-2.md` | Filler refresh programme | The shipped filler system and `PROGRESS.md` evidence |
| `v59a-image-runtime-certification.md` | Rust image worker certification plan | The retained certification gates and `PROGRESS.md` evidence |
| `v59b-image-runtime-optimization.md` | Rust image worker optimization plan | The retained benchmarks and `PROGRESS.md` evidence |
| `shield-client-2026-08-17.md` | Initial Android TV client plan | `docs/native-client-design.md` and the shared-client plan |

## What is NOT archived, and why

`docs/engineering/` still holds documents that are dated evidence rather than superseded plans —
`phase-0-findings.md`, `v2-mock-delta-2026-07-24.md`, `FINDINGS-river-spike-2026-07-30.md`,
`channels-refinement-2026-07-24.md`. They describe what was measured or decided on a date and
make no claim about what to do next, so they age without going wrong.

The distinction worth keeping: **a finding stays true; a plan expires.**
