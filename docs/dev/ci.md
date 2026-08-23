# CI

```mermaid
graph LR
  C["<b>changes</b><br/><i>diffs the merge base</i>"]
  GC["<b>go-contracts</b><br/>static · drift · repository contracts"]
  RC["<b>image-certification</b><br/>release-worker runtime contract"]
  G["<b>go</b> ×3<br/>race-policy tests only"]
  X["<b>windows-playout</b><br/>native process-tree contract"]
  P["<b>store-postgres</b>"]
  F["<b>frontend</b> ×2"]
  W["<b>playwright</b> ×4"]
  T["<b>tuner</b><br/>three browser engines"]
  D["<b>docs</b><br/>links · structure · prose"]
  A["<b>agent-harness-macos</b>"]
  N["<b>android</b><br/>lint · unit · assemble"]
  I["<b>image</b><br/>amd64 + arm64"]
  OK(["<b>ci-ok</b><br/><i>the required check</i>"])

  C -->|"*.go, migrations, docs/help/, scripts/, Makefile"| GC
  C -->|"same Go inputs"| G
  C -->|"same Go inputs"| RC
  C --> X
  C --> P
  C -->|"web/, Makefile"| F
  C --> W
  C --> T
  C -->|"docs/, README, docs-site/"| D
  C --> A
  C --> N
  C -->|"all Docker build inputs"| I
  GC --> OK
  RC --> OK
  G --> OK
  X --> OK
  P --> OK
  F --> OK
  W --> OK
  T --> OK
  D --> OK
  A --> OK
  N --> OK
  I --> OK

  classDef gate fill:#1f6f4a,stroke:#134a31,color:#fff
  classDef job fill:#2b3b52,stroke:#1b2736,color:#dbe4ef
  class OK gate
  class C,GC,RC,G,X,P,F,W,T,D,A,N,I job
```

## Jobs run only when their inputs changed

A `changes` job diffs against the merge base and each job gates on its output. It fails safe: no
usable merge base — first push, force-push, new branch — runs everything.

**Adding a new build input means adding it to the filter in the same PR.**

Two non-obvious entries: `docs/help/` is in the Go filter because those pages are embedded and
the doc-claims test reads them, and `scripts/` is there because the job executes them.

### Specialized gate classifier is shadow-only

The `changes` job also runs `scripts/ci-impact.sh` and publishes `impact_*` outputs for the
specialized contract, Go, full-Go, Rust, Postgres, Windows, web, visual, e2e, tuner, image, docs,
agent, and Android gates. Its run summary places those proposed decisions beside the current broad
Go and web families.

Those outputs are observational: no required job consumes them yet. The existing `go`, `web`,
`image`, `docs`, `agent`, and `android` outputs remain authoritative while shadow results are
compared with complete CI outcomes. A missing base, classifier failure, or unknown path selects
every proposed gate.

`scripts/testdata/ci-impact.tsv` records the exact ordered gate set for representative paths and
multi-path changes across every specialized gate. The classifier contract test compares complete
sets, so both a missed gate and an unexplained extra gate require an explicit fixture decision.

Non-code files consumed by Go tests are Go inputs too. In particular, design/configuration/command
docs, install docs and README, the committed OpenAPI document, production Compose, and embedded
help select the complete Go test set. Dockerfile and packaged licence/notices select repository
contracts as well as the image build. These mappings are fixtures because file extensions cannot
reveal those dependencies.

## The image job is the exception

The image filter follows every source family copied by the Dockerfile: Docker metadata, packaged
LICENSE/notices, Cargo and Rust sources, Go sources/modules/embedded migrations, embedded help, the frontend,
OpenAPI, and the bundle guard. `Makefile` and workflow-only changes do not change image bytes and
therefore do not trigger it.

It builds each release platform on a native runner, loads the resulting image without pushing it,
and inspects the packaged LICENSE/notices and OCI labels. The Dockerfile's build-time commands prove
the bundled tools; the post-build inspection proves the final runtime filesystem rather than a
comment or an intermediate stage.

It's also the only job with a `timeout-minutes`; GitHub's default is six hours.

It exists because a Dockerfile that could never build for arm64 sat undetected. Build both
platforms or it can't catch that.

## `ci-ok` is the only required check

It always runs and inspects `needs.*.result` explicitly. That has to be explicit: a skipped job
doesn't fail an aggregate by default, and neither does a failed one under `if: always()`.

The `main` branch requires the GitHub Actions-owned `CI` check in strict mode: a pull request must
be tested against the current base before it can merge. Preserve both the check name and its app
binding when editing branch protection.

`make release-verify` parses the workflow and requires every top-level job to appear in
`ci-ok.needs`. This prevents a newly added or accidentally removed dependency from producing a
green required check while its real job is red.

Never add a workflow-level `paths:`. A run that doesn't trigger reports no checks, so a required
check sits "expected" forever and the PR can't merge. Filter per job.

The workflow already handles `merge_group`, but GitHub offers merge queues only to public
repositories owned by organizations (or qualifying organization-owned private repositories).
`mantonx/loomarr` is personally owned, so queue activation is deferred until an organization owns
the repository. Strict required checks provide the protected current-base boundary in the meantime.

## Sharding

Go tests, frontend and Playwright split across runners for wall-clock only. Repository-wide Go and
Rust contracts run once in `go-contracts`, in parallel with three test-only Go shards and the
independent release-worker certification. Their union is the same assurance as local `make check`
plus the existing CI-only certification. The `ci-ok` aggregate requires every job, so moving a
contract out of the test shards cannot make it optional.

`make go-shard-verify` runs in `go-contracts` and asserts the Go shards are a true partition of
`go list ./...` — a split that drops a package would otherwise pass by not running it.

Sharding is free on a public repo. Check the bill before copying it into a private one.

## Caching

- **`actions/cache` never overwrites an existing key.** A cache whose contents track something
  its key doesn't gets written once and frozen. Use a rolling key with `restore-keys`.
- **The 10GB cap evicts LRU across all refs**, so closed PRs' caches push out live ones.
  `cache-cleanup.yml` deletes them on close.

## Hand-maintained lists, and what guards them

Three lists in this repo are written by hand and would rot silently. Each has an executable guard
that fails when it drifts, and all three run in CI:

| List | Guard | Runs via |
| --- | --- | --- |
| `TAGS` in the Makefile | `scripts/check-tags.sh` | `make tags-verify`, part of `make check` |
| Retired identifiers | `scripts/check-retired.sh` | `make retired-verify`, its own CI step |
| Release-image source-family probes | `releaseverify.VerifyCIImageInputs` | `make release-verify`, part of `make check` |

`tags-verify` compares the tags in `//go:build` lines against `TAGS` and fails **both ways**:

- **In the tree, not in `TAGS`** — those files are invisible to `vet-tags` and `lint`. Nothing
  compiles them, which is how a live ffmpeg test sat uncompiled for months.
- **In `TAGS`, not in the tree** — the list claims coverage it doesn't have. Drop the tag in the
  PR that removed its last file.

Downgrading the second direction to a warning is the obvious-looking fix when it's inconvenient.
Don't — a warning printed by a job that exits 0 is one nobody reads.

The CI path filter includes `scripts/`, so a PR editing only a guard still runs the job that
executes it.

## Scheduled Rust maintenance

`rust-maintenance.yml` is intentionally outside the required PR gate. Every Monday, and on manual
dispatch, it installs pinned cargo-deny and cargo-fuzz versions, checks both Cargo lockfiles for
RustSec advisories, approved SPDX licences, and untrusted sources, then fuzzes the worker's bounded
JSON-to-decoder boundary under nightly libFuzzer. A crash retains its reproducer for 30 days.

This job is allowed to be expensive and network-sensitive. The fast deterministic protections stay
in `make check`: Cargo lock enforcement, clippy/tests, and `#![forbid(unsafe_code)]` on Loomarr-owned
shipping crates.
