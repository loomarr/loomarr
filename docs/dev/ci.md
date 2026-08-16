# CI

```mermaid
graph LR
  C["<b>changes</b><br/><i>diffs the merge base</i>"]
  G["<b>go</b> ×2<br/>check · drift gates"]
  P["<b>store-postgres</b>"]
  F["<b>frontend</b> ×2"]
  W["<b>playwright</b> ×4"]
  D["<b>docs</b><br/>links · structure · prose"]
  I["<b>image</b><br/>amd64 + arm64"]
  OK(["<b>ci-ok</b><br/><i>the required check</i>"])

  C -->|"*.go, migrations, docs/help/, scripts/, Makefile"| G
  C --> P
  C -->|"web/, Makefile"| F
  C --> W
  C -->|"docs/, README, docs-site/"| D
  C -->|"Dockerfile only"| I
  G --> OK
  P --> OK
  F --> OK
  W --> OK
  D --> OK
  I --> OK

  classDef gate fill:#1f6f4a,stroke:#134a31,color:#fff
  classDef job fill:#2b3b52,stroke:#1b2736,color:#dbe4ef
  class OK gate
  class C,G,P,F,W,D,I job
```

## Jobs run only when their inputs changed

A `changes` job diffs against the merge base and each job gates on its output. It fails safe: no
usable merge base — first push, force-push, new branch — runs everything.

**Adding a new build input means adding it to the filter in the same PR.**

Two non-obvious entries: `docs/help/` is in the Go filter because those pages are embedded and
the doc-claims test reads them, and `scripts/` is there because the job executes them.

## The image job is the exception

`Makefile` and workflow changes trigger Go and Frontend, but not the image, which gates on
`Dockerfile` and `.dockerignore` only. It builds both release platforms under QEMU — about half
an hour of billed CI — and neither of those files changes what `docker build` produces.

It's also the only job with a `timeout-minutes`; GitHub's default is six hours.

It exists because a Dockerfile that could never build for arm64 sat undetected. Build both
platforms or it can't catch that.

## `ci-ok` is the only required check

It always runs and inspects `needs.*.result` explicitly. That has to be explicit: a skipped job
doesn't fail an aggregate by default, and neither does a failed one under `if: always()`.

Never add a workflow-level `paths:`. A run that doesn't trigger reports no checks, so a required
check sits "expected" forever and the PR can't merge. Filter per job.

## Sharding

Go, frontend and Playwright split across runners for wall-clock only. `make go-shard-verify`
asserts the Go shards are a true partition of `go list ./...` — a split that drops a package
would pass by not running.

Sharding is free on a public repo. Check the bill before copying it into a private one.

## Caching

- **`actions/cache` never overwrites an existing key.** A cache whose contents track something
  its key doesn't gets written once and frozen. Use a rolling key with `restore-keys`.
- **The 10GB cap evicts LRU across all refs**, so closed PRs' caches push out live ones.
  `cache-cleanup.yml` deletes them on close.

## Hand-maintained lists, and what guards them

Two lists in this repo are written by hand and would rot silently. Each has a script that fails
when it drifts, and both run in CI:

| List | Guard | Runs via |
| --- | --- | --- |
| `TAGS` in the Makefile | `scripts/check-tags.sh` | `make tags-verify`, part of `make check` |
| Retired identifiers | `scripts/check-retired.sh` | `make retired-verify`, its own CI step |

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
