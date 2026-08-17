# First beta readiness

- Status: active
- Target: `v0.1.0-beta.1`
- Artifact: one signed OCI image for `linux/amd64` and `linux/arm64`, run directly on Linux or through Docker Desktop on macOS

## Release decision

The initial audit baseline (`c1f24dc5`) was not ready to tag. The repository has strong automated gates, a nonroot multi-architecture image, forward-only migrations, centralized authorization, generated-contract drift checks, and real integration seams. The remaining problems are not a reason to restart or redesign the product, but several are release blockers because they make the shortest documented path fail or make installation and recovery claims untrue.

The beta ships only when every blocker below is either closed on `main` or deliberately removed from the beta contract. “An active branch contains a fix” is not closed.

## Ship contract

| Area | Beta contract |
| --- | --- |
| Core journey | A new admin can install, finish setup, create a grounded proposal, approve it, reach the correct backend-aware channel surface, and tune an internal-playout channel without discovering a required hidden setting. |
| Job durability | Suggestion work survives reload and restart; the UI polls an authoritative persisted job and renders actionable terminal failures. |
| Approval safety | The approval gate remains atomic and unattended acquisition quotas cannot be exceeded by concurrent proposals. |
| Playback | Internal playback has decoded-frame evidence through the real composition root; the supported browser/controller matrix and restart behavior are recorded. Tunarr channels never land on the unsupported in-app HLS player. |
| Distribution | A pinned `ghcr.io/mantonx/loomarr:<beta>` image installs and upgrades through the documented Compose path. Both release architectures build before tagging. Prereleases do not move `latest`. |
| Docker edge | The compiled app is reached through a pinned Traefik service on Linux and Docker Desktop macOS. The supported beta topology starts one Loomarr replica; Traefik is not evidence of horizontal safety by itself. |
| Scale evidence | A Postgres-backed multi-replica investigation exercises request distribution, auth/session consistency, migrations/bootstrap, jobs and recurring schedulers, approval quotas, playout ownership, local media state, and graceful shutdown. Any unsafe subsystem remains an explicit single-replica constraint. The executable matrix is recorded in [`multi-replica-readiness.md`](multi-replica-readiness.md). |
| Data safety | Backup scope is stated accurately, off-volume/off-host copies are recommended, and a restore drill covers permissions, startup, and validation. |
| Security | Public endpoints and bootstrap/throttling behavior are intentional, documented, and covered by negative tests. Distributed binaries and models have a complete license/source/integrity inventory. |
| Documentation | README, install, quickstart, upgrade, configuration, changelog, security policy, Compose, and the rendered app tell the same story. macOS is described as Docker Desktop running the Linux arm64/amd64 image, not as a native macOS container. |
| Gates | Required Go, Postgres, frontend, visual/a11y/e2e, docs, release-image, migration, generated-drift, and runtime-certification gates are green on the release commit. |

## Blocker ledger

| Blocker | Evidence | Ownership / exit |
| --- | --- | --- |
| Internal playout requires `server.public_url`, but the default wizard never collects it. | `web/apps/web/src/wizard/steps/steps.ts`; `web/apps/web/src/wizard/playout-step/playout-step.tsx`; `web/apps/web/src/test/wizard-router.test.tsx`; `internal/settings/declared.go`; `internal/app/playoutadapter.go` | This slice adds the internal-only registry field, reachable-address guidance, env-lock behavior, persisted completion gate, and routed valid/invalid/error regressions. Close only after it merges to `main`. |
| Suggestion jobs can strand on restart and the UI depends on one SSE terminal frame. | `internal/store/{sqlite,postgres}.go`; `internal/suggest/worker.go`; `web/apps/web/src/suggest/use-suggestion-run/use-suggestion-run.ts` | Active `first-channel-success` / `proposal-jobs` claim. Re-audit after merge; do not duplicate it here. |
| Concurrent auto-approval can exceed a user's unattended-acquisition quota. | `internal/suggest/autoapprove.go`; `internal/store/approval.go`; store conformance | This change orders every approval for a requester, then runs automatic quota reads and commit on one transaction/session; manual approval participates in ordering without being quota-rejected, and post-commit reconciliation runs after ordering is released. Exit: merge after SQLite/Postgres concurrent, cancellation/reacquisition, and independent-Postgres-pool evidence is green. |
| Channel status and post-approval routing assume Tunarr in some places and internal HLS in others. | `web/apps/web/src/routes/_authed/channels/$id/route.tsx`; `channel-watch.tsx`; `use-hls-player.ts`; `internal/api/channelplayurl.go` | Partly active first-channel/playback work. Remaining exit: use canonical `inAppPlayable`; route Tunarr to a real handoff rather than Watch. |
| The product calls AI/TMDB optional while the only normal channel-origination UI requires suggestions. | `README.md`; `docs/install/index.md`; `docs/help/quickstart.md`; `internal/api/proposals.go`; `docs/design.md` §12 | Beta documentation now names TMDB and an LLM as prerequisites for the defining flow. Close after merge; a non-AI UI is not promised for this beta. |
| `--profile postgres` starts Postgres but leaves Loomarr on SQLite. | `docker/compose.yaml`; `docs/install/docker.md`; `docs/help/quickstart.md` | `docker/compose.postgres.yaml` and `compose-verify` now wire and enforce the Postgres DSN. Close after merge and runtime restore proof. |
| Install/upgrade Compose points at `loomarr:latest`, while releases publish GHCR. | `docker/compose.yaml`; `docs/install/upgrading.md`; `.github/workflows/release.yml` | Compose now requires an exact GHCR `LOOMARR_VERSION` and rejects mutable or malformed tags in a preflight using the publisher's validation policy; install, upgrade, and rollback use the same pin. Close after published-image install proof. |
| The release Compose path exposes Loomarr directly and has no Traefik edge or routing verification. | `docker/compose.yaml`; `docs/install/docker.md`; `scripts/check-compose.sh` | Digest-pinned Traefik now owns the only host port. Real amd64 routing/no-bypass passed; arm64 and Docker Desktop evidence remain. |
| The release image is not rebuilt in CI for all Docker build inputs; tag publishing does not require or run the full gate. | `.github/workflows/ci.yml`; `Dockerfile`; `.github/workflows/release.yml` | Image inputs now cover Go, web, OpenAPI, and embedded help; release tags require successful main CI and prereleases cannot move `latest`. Close after the workflow runs on the merged commit. |
| Distributed binary/model notices and integrity evidence are incomplete. | `Dockerfile`; `THIRD_PARTY_NOTICES.md`; `internal/releaseverify/notices_test.go`; `.github/workflows/ci.yml` | The six yt-dlp, Deno, and whisper archives are SHA256-verified per architecture; CI inspects both built architectures for the packaged LICENSE/notices and OCI labels; yt-dlp and ffmpeg GPLv3+ terms are recorded. Mutable ffmpeg/base inputs, exact corresponding source, DejaVu/transitive texts, Prometheus NOTICE, and final legal review remain open. |
| Backup prose overstates scope and restore guidance is not a drill. | `.env.example`; `docs/install/{docker,upgrading}.md`; `internal/store/backup_restore*_test.go`; `docs/design.md` durability section | Database-only scope, off-volume copies, exact archive format, and rollback permissions are explicit. The isolated SQLite drill is locally green; the Postgres drill compiles and is part of `test-pg`, but local execution is blocked by Docker socket permission. Exit: record its green CI run and release-image uid/gid startup proof on the integrated commit. |
| Playout admission normally replaces measured capacity with the default override of four. | `internal/app/app.go`; `internal/settings/declared.go`; `internal/api/playout.go`; `docs/design.md` capacity section | Resolved in the beta capacity slice: measurement is authoritative, `0` is automatic, a positive setting can only lower it, unknown measurement safely means one transcode, and both admission and quality use the same effective budget. Close after merge and full gates. |
| Required decoded playback and browser/runtime certification is incomplete. | `PROGRESS.md` V58; `internal/api/playoutchain_live_test.go`; active tuner-browser plan | Active `tuner-browser-matrix` / `tuner-cert` claim. Beta waits for recorded evidence on `main`. |

## Required cleanup before the tag

Cleanup is release work when it removes ambiguity, false affordances, or a known failure mode. It is not permission for broad aesthetic refactors.

- Remove or correct false UI actions and copy: permanent-delete wording for detach, the toast-only “Open in media server,” missing stream handoff, and backend-blind “Not on air yet.”
- Reconcile stale status/design claims, including contradictory open/fixed entries and the obsolete production-`testing` exemption.
- Make `make doctor` and contributor docs surface the Node 22 contract consistently; do not treat results from the host's unsupported Node 26 as release evidence.
- Decompose `internal/app.BuildHandler` only along its already-approved subsystem-builder seams and only after ship blockers no longer churn those call sites.
- Remove test-only mutable production globals and broaden architecture classification after the composition-root work stabilizes.
- Triage open issues into beta blockers, beta follow-ups, and post-beta backlog. Keep known limitations in release notes rather than silently implying completeness.

## Evidence snapshot

- Main commit `90c4d29` is all green: CI run `31925624018` and Docs site run `31925624025` both completed successfully, including native `linux/amd64` and `linux/arm64` release-image builds, Postgres conformance, frontend shards, docs, visual/a11y shards, wizard e2e, the macOS agent harness, and Rust certification.
- Local `make test` passed under the race detector.
- The agent process-ownership scan now tolerates an unrelated process whose worktree directory was deleted, with a regression in `scripts/agent-harness-test.sh`.
- Local `make doctor` correctly rejected Node 26 because the release toolchain contract is Node 22.x.
- No release tag or published image exists yet. A green component gate is evidence, not proof of the full beta journey.

## Delivery sequence

1. Release/repository truth: harness cleanup, Compose image/Postgres wiring, release tag policy and CI inputs, backup/install/upgrade/license docs.
2. First-channel truth: merge and re-audit active durable-job/first-channel work; close wizard and backend-aware navigation gaps.
3. Safety: atomic auto-approval quota; maintainer decision and implementation for playout capacity; selected auth hardening with required negative tests.
4. Runtime proof: merge browser/controller and decoded-playback certification; run restore drills, the explicit multi-replica investigation, and release-candidate image smoke on both architectures. Treat every scale failure as a documented single-replica constraint or a blocker, never as a successful load-balancer test.
5. Cleanup/freeze: close stale claims and false affordances, classify remaining issues, update changelog/security/support statements, and freeze non-blocking refactors.
6. Release: all gates green on one commit, create `v0.1.0-beta.1`, verify registry manifest/signature/SBOM/provenance, install the pinned image on Linux and Docker Desktop macOS, then publish beta notes with known limitations.
