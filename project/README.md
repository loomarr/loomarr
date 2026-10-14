# Project records

**For:** maintainers.
**You'll get:** what lives here, and what doesn't.

Material that's still in use but isn't documentation for readers: it's kept in the repo so it's
reviewed in PRs, and off the docs site so it doesn't bury the pages readers need.

| Path | What it is |
| --- | --- |
| [`roadmap.md`](roadmap.md) | Release sequencing for the next betas |
| [`release/`](release/README.md) | Hand-written release-note headers; `scripts/generate-release-notes.sh` reads them |
| [`android-release.md`](android-release.md) | The Android TV signing and Play Console runbook |
| [`design-archive-2026-09/`](design-archive-2026-09/README.md) | Verbatim `design.md` slices with measured evidence or retired protocols, from the #779 split. Not instructions: `docs/design/` wins |
| [`plans/`](plans/shared-client-platform.md) | Plans that live code or docs still cite |
| [`evidence/`](evidence/ffmpeg-august-source-retention.md) | License-compliance evidence for shipped binaries; `internal/releaseverify` reads the manifest |
| [`redesign-map-2026-09.md`](redesign-map-2026-09.md) | The beta.9 redesign map (#1659): screens, gaps, critique, the maintainer's decisions, PR sequence. Delete when #1659 closes |
| [`sweep-2026-09.md`](sweep-2026-09.md) | The measured codebase sweep (#1718): the verdict re-check and the ranked do and decide items. Delete when #1718 closes |
| `SPIKE-*.md`, `FINDINGS-*.md` | Measurements an open issue still depends on. Delete each when its issue closes |

Finished plans, dated certifications and research were deleted in #1572. Git history keeps them:
browse the tree at [`e9d9f9cc`](https://github.com/loomarr/loomarr/tree/e9d9f9cc/docs/engineering).
Open work belongs in an issue, not a new file here.
