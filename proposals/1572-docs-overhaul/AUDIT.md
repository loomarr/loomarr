# Docs audit (#1572, phase 1)

Every Markdown file under `docs/`, plus the README and the site. Checked against `origin/main`
at `381e9165` on 2026-09-27. **Q** is quality from 1 (harmful) to 5 (keep as is). **Refs** is
the number of other files in the repo that name the file. Actions: **keep**, **rewrite**,
**merge** (into another page), **move** (off the site), **delete** (git history keeps it).

## User, operator and site pages

| File | Audience | Job | Q | Stale vs main | Action → target |
| --- | --- | --- | --- | --- | --- |
| `README.md` | Everyone | Pitch + install + dev + ops | 3 | **Pins `0.1.0-beta.8`**; overview diagram wrong | **Rewrite**: pitch, one diagram, Get started link. Dev and Operations sections go to their pages |
| `docs/README.md` | Us | Map of the tree + standards | 3 | Diagram standard cites a shared D2 config that doesn't exist | **Merge** standards into `contributing/docs.md`; the map becomes the site nav |
| `docs-site/` (config, landing, CSS) | Readers | The site | 2 | Off-brand green; `/` is a meta-refresh; design doc in the nav | **Rewrite**: token theme, landing page, Diátaxis nav, redirects |
| `docs/help/quickstart.md` | New users | Tutorial | 3 | **Pins `0.1.0-beta.8`**; **missing the Location step**; Postgres and `DATABASE_URL` asides mid-tutorial | **Rewrite** → `get-started.md` (sample in this PR) |
| `docs/install/index.md` | Operators | Prerequisites + backend choice | 3 | Diagram repeats the prose | **Merge** → `get-started.md` (needs) + `explanation/how-loomarr-works.md` (backend choice) |
| `docs/install/docker.md` | Operators | Install + backup + filler mounts | 3 | **Pins `0.1.0-beta.8`**; lists `/data/prepared` as live | **Rewrite**: split into `guides/install-docker.md` (Postgres, ports, discovery) and `guides/backups.md` |
| `docs/install/hardware.md` | Operators with GPUs | HW encode + HDR | 4 | Current (updated 09-26) | **Rewrite** lightly → `guides/hardware-encoding.md` + `reference/hardware.md` (the matrix tables) |
| `docs/install/upgrading.md` | Operators | Upgrade + rollback | 3 | **Pins `0.1.0-beta.8`**; "prepared media can be regenerated" | **Rewrite** → `guides/upgrade.md`; backup steps link to `guides/backups.md` |
| `docs/install/monitoring.md` | Operators | Prometheus + Grafana | 4 | Current | **Keep** → `guides/monitoring.md` |
| `docs/configuration.md` | Operators | Settings reference (generated) | 4 | Current (generated) | **Keep** → `reference/settings.md` (generator path changes) |
| `docs/integrations/media-server-livetv.md` | Written for us | Briefing on Live TV wiring | 2 | Opens "Intended repo location"; Phase 0 talk; dated Firefox anecdote | **Delete**; its 4 troubleshooting lines merge into `guides/troubleshooting.md`, and the design lives in #779's design docs |
| `docs/help/concepts.md` | Users | Mental model | 3 | Quality-ledger section is a privacy policy, not a concept | **Rewrite** → `explanation/how-loomarr-works.md` + `explanation/privacy.md` |
| `docs/help/programming.md` | Admins | Channel policy | 4 | **Real film title** as example | **Rewrite** (genre example) → `explanation/curation.md` + `guides/add-a-channel.md` |
| `docs/help/member-guide.md` | Members | Roles, intents, status | 3 | "Drifted" status: verify it's still a UI state | **Merge** → `guides/add-a-channel.md` + `explanation/how-loomarr-works.md` |
| `docs/help/filler.md` | Admins | Filler pipeline | 3 | Current; long, and reads like a spec | **Rewrite** → `guides/filler.md` (task) + `explanation/curation.md` (why) |
| `docs/help/integrations.md` | Admins | Per-service settings | 3 | Current; repeats the settings reference | **Rewrite** → `guides/connect-services.md`; settings detail links to reference |
| `docs/help/troubleshooting.md` | Anyone stuck | Fixes by symptom | 4 | Current; **headings are an API contract** | **Keep** → `guides/troubleshooting.md` (anchors preserved) |
| `docs/frontend-design.md` | Contributors | Client architecture + tokens | 4 | Current | **Move** under Contributing → Design (#779 owns placement) |
| `docs/config-design.md` | Contributors | Settings subsystem design | 3 | Current | **Move** under Contributing → Design (#779) |
| `docs/programming-design.md` | Contributors | Policy heuristics design | 3 | Current | **Move** under Contributing → Design (#779) |
| `docs/design.md` | Contributors | System design | — | Owned by #779 | **Reference only**: Contributing → Design; out of the user nav |

## Contributor pages (`docs/dev/`)

| File | Audience | Job | Q | Stale vs main | Action → target |
| --- | --- | --- | --- | --- | --- |
| `index.md` | Contributors | Entry + rules + code map | 3 | Says "`README.md`, `README.md`" (duplicate) | **Rewrite** → `contributing/index.md` |
| `setup.md` | Contributors | Toolchain | 4 | Current | **Keep** → `contributing/setup.md` |
| `dev-loop.md` | Contributors | Run BE + FE | 4 | Current; its diagram adds nothing | **Keep** → `contributing/dev-loop.md`; delete the diagram |
| `testing.md` | Contributors | Test layers (553 lines) | 3 | Mixes gates with certification history | **Rewrite** → `contributing/testing.md`; certification history → `project/` |
| `ci.md` | Contributors | CI jobs (773 lines) | 3 | Current; far too long for a reader | **Rewrite** → `contributing/ci.md` (what runs on my PR); detail stays in the workflow comments |
| `codegen.md` | Contributors | What's generated | 4 | Current | **Merge** → `contributing/dev-loop.md` |
| `commands.md` | Contributors | Make targets (generated) | 4 | Generated | **Keep** → `reference/make.md` |
| `releasing.md` | Maintainers | Release process | 4 | Current | **Keep** → `contributing/releasing.md` |
| `android-beta.md` | Maintainers | Play Console + signing | 3 | Current; one maintainer's runbook | **Move** → `project/android-release.md` |
| `playout-bench.md` | Contributors | Bench thresholds + CI | 3 | Current | **Keep** → `contributing/playout-bench.md` |
| `ai.md` | Contributors + users | AI practice + what leaves the network | 3 | Two audiences in one page | **Split**: "what leaves your network" → `explanation/privacy.md`; the rest → `contributing/ai.md` |
| `agents.md` | Agents | Harness, claims, worktrees | 3 | Current | **Move** off the site (next to `AGENTS.md`); agents read the repo, not the site |
| `skills.md` | Agents | Skill catalog | 3 | Current | **Move** off the site, same reason |
| `graphify.md` | Agents | Knowledge-graph tool | 2 | Current | **Move** off the site |

## Internal material (all off the site)

Default: **delete**. Git history keeps every file, and none of these helps a reader today.
**Move** to `project/` only where something live points at the file.

| File | Q | Refs | Action |
| --- | --- | --- | --- |
| `docs/agents/domain.md` | 3 | 0 | **Keep in place, off site**: vendored skills name `docs/agents/*` and are hash-pinned |
| `docs/agents/issue-tracker.md` | 3 | 2 | **Keep in place, off site** (same) |
| `docs/agents/triage-labels.md` | 3 | 1 | **Keep in place, off site** (same) |
| `docs/release/README.md` | 3 | many | **Move** → `project/release/` with `scripts/generate-release-notes.sh` updated |
| `docs/release/v0.1.0-beta.1.md` … `v0.2.0-beta.7.md` (5 files) | 3 | 0–1 | **Move** with it; `v0.2.0-beta.7.md` names a real film (see open questions) |
| `docs/research/issue-1237-provider-discovery-apis.md` | 3 | 0 | **Delete**; summarise on #1237 if it's still open |
| `docs/engineering/README.md` | 3 | many | **Delete** with the tree; a one-line `project/README.md` replaces it |
| `docs/engineering/archive/README.md` | 2 | many | **Delete** |
| `docs/engineering/archive/progress-journal.md` (4,596 lines) | 1 | 1 | **Delete** |
| `docs/engineering/archive/design-mock-review-2026-07-20.md` | 1 | 1 | **Delete** |
| `docs/engineering/archive/durable-first-channel-workflow.md` | 1 | 2 | **Delete** |
| `docs/engineering/archive/first-beta-readiness.md` | 1 | 2 | **Delete** |
| `docs/engineering/archive/frontend-build-plan.md` | 1 | 3 | **Delete** |
| `docs/engineering/archive/playout-prior-art.md` | 2 | 4 | **Delete** (prior art for a playout design that has itself been replaced) |
| `docs/engineering/archive/playout-prior-art-viewra.md` | 1 | 2 | **Delete** |
| `docs/engineering/archive/release-native-arm64-2026-08-18.md` | 1 | 1 | **Delete** |
| `docs/engineering/archive/shield-client-2026-08-17.md` | 1 | 1 | **Delete** |
| `docs/engineering/archive/surface-audit-2026-07-26.md` | 1 | 3 | **Delete** |
| `docs/engineering/archive/v2-build-plan.md` | 1 | 6 | **Delete** (fix the 6 references) |
| `docs/engineering/archive/v54-filler-refresh-2.md` | 1 | 3 | **Delete** |
| `docs/engineering/archive/v59a-image-runtime-certification.md` | 1 | 2 | **Delete** |
| `docs/engineering/archive/v59b-image-runtime-optimization.md` | 1 | 2 | **Delete** |
| `docs/engineering/account-access-certification-2026-08-31.md` | 2 | 1 | **Delete** |
| `docs/engineering/channel-recommendation-certification-2026-09-02.md` | 2 | 0 | **Delete** |
| `docs/engineering/channel-recommendation-certification-v2-2026-09-02.md` | 2 | 0 | **Delete** |
| `docs/engineering/channel-recommendation-certification-v2-contract-2026-09-02.md` | 2 | 0 | **Delete** |
| `docs/engineering/channel-recommendation-protocol-diagnosis-2026-09-02.md` | 2 | 0 | **Delete** |
| `docs/engineering/channels-refinement-2026-07-24.md` | 2 | 3 | **Delete** (a decision record; the decision is in the code and design) |
| `docs/engineering/client-design-system-coverage.md` | 3 | 2 | **Move** → `project/` while beta.9's client design-system work is live |
| `docs/engineering/client-platform-inventory.md` | 3 | 2 | **Move** → `project/` (same) |
| `docs/engineering/evidence/database-lifecycle-certification-2026-08-31.md` | 2 | 0 | **Delete** |
| `docs/engineering/evidence/ffmpeg-august-source-retention.md` | 3 | 1 | **Move** → `project/` (licence-compliance evidence for a shipped binary) |
| `docs/engineering/evidence/filler-temporal-unit-role-diagnostic-2026-09-01.md` | 2 | 0 | **Delete** |
| `docs/engineering/evidence/shield-native-reference-review-2026-09-04.md` | 2 | 0 | **Delete** |
| `docs/engineering/filler-bakeoff-openrouter.md` | 2 | 1 | **Delete** |
| `docs/engineering/filler-model-led-identification-plan-2026-08-31.md` | 2 | 3 | **Delete** |
| `docs/engineering/filler-spoken-cascade-certification.md` | 2 | 0 | **Delete** |
| `docs/engineering/filler-spoken-corpus-assembly.md` | 2 | 0 | **Delete** |
| `docs/engineering/filler-spoken-known-script-preparation.md` | 2 | 0 | **Delete** |
| `docs/engineering/filler-spoken-model-review.md` | 2 | 1 | **Delete** |
| `docs/engineering/filler-spoken-vctk-preparation.md` | 2 | 1 | **Delete** |
| `docs/engineering/FINDINGS-android-tv-beta-distribution-2026-08-22.md` | 2 | 1 | **Delete** |
| `docs/engineering/FINDINGS-beta-binary-redistribution-2026-08-16.md` | 3 | 0 | **Move** → `project/` if `THIRD_PARTY_NOTICES.md` relies on it; otherwise delete |
| `docs/engineering/FINDINGS-river-spike-2026-07-30.md` | 2 | 4 | **Delete** |
| `docs/engineering/go-1.27-follow-up-2026-09-02.md` | 1 | 1 | **Delete** |
| `docs/engineering/phase-0-findings.md` | 2 | 6 | **Delete** (fix references) |
| `docs/engineering/planner-model-recommendation-2026-09-01.md` | 2 | 1 | **Delete** |
| `docs/engineering/planner-tool-finalization-diagnosis-2026-09-02.md` | 2 | 0 | **Delete** |
| `docs/engineering/plans/README.md` | 2 | many | **Delete**; plans live in issues |
| `docs/engineering/plans/next-three-betas.md` | 3 | 3 | **Move** → `project/roadmap.md`, or a pinned issue (maintainer's call) |
| `docs/engineering/plans/backend-consolidation.md` | 2 | 1 | **Delete**; open work → issue |
| `docs/engineering/plans/channel-discovery-next.md` | 2 | 2 | **Delete**; open work → issue |
| `docs/engineering/plans/channel-reference-journeys.md` | 2 | 0 | **Delete** |
| `docs/engineering/plans/client-diagnostics.md` | 2 | 1 | **Delete**; open work → issue |
| `docs/engineering/plans/current-health.md` | 2 | 1 | **Delete** (shipped) |
| `docs/engineering/plans/local-ci-feedback.md` | 2 | 2 | **Delete**; #1570 carries CI work |
| `docs/engineering/plans/multi-replica-readiness.md` | 3 | 4 | **Move** → `project/` (the install guide cites it for the one-replica limit) |
| `docs/engineering/plans/shared-client-platform.md` | 3 | 6 | **Move** → `project/` (beta.9 is live on it) |
| `docs/engineering/plans/suggestion-query-evaluation.md` | 2 | 4 | **Delete**; the evaluation lives in tests |
| `docs/engineering/plans/suggestion-query-family-registry.md` | 2 | 3 | **Delete** (same) |
| `docs/engineering/playout-bench/README.md` | 3 | many | **Move** → next to the bench baselines it describes |
| `docs/engineering/playout-hardware-qualification.md` | 2 | 0 | **Delete**; the declared-profile design is in §9.1 |
| `docs/engineering/SPIKE-4k-tonemap.md` | 3 | 2 | **Move** → `project/` while #1512 is open, then delete |
| `docs/engineering/SPIKE-watermark.md` | 3 | 1 | **Move** → `project/` while #1512 is open, then delete |
| `docs/engineering/v2-mock-delta-2026-07-24.md` (1,116 lines) | 1 | 5 | **Delete** (fix references) |
| `docs/engineering/research/README.md` | 2 | many | **Delete** |
| `docs/engineering/research/android-tv-briefing-2026-08-17.md` | 2 | 2 | **Delete** |
| `docs/engineering/research/apple-native-cache-2026-08-30.md` | 2 | 0 | **Delete** |
| `docs/engineering/research/channel-discovery-quality-2026-08-23.md` | 2 | 2 | **Delete** |
| `docs/engineering/research/client-design-system-reconciliation-2026-08-30.md` | 2 | 0 | **Delete** |
| `docs/engineering/research/federal-av-replacement-candidates-2026-09-05.md` | 2 | 1 | **Delete** |
| `docs/engineering/research/filler-admission-confidence.md` | 3 | 3 | **Move** → `project/` if beta.11 filler work uses it; otherwise delete |
| `docs/engineering/research/filler-authentic-multimodal-bakeoff-2026-08-26.md` | 2 | 0 | **Delete** |
| `docs/engineering/research/filler-authentic-source-lanes-2026-08-27.md` | 2 | 0 | **Delete** |
| `docs/engineering/research/filler-authentic-temporal-model-bakeoff-2026-08-26.md` | 2 | 0 | **Delete** |
| `docs/engineering/research/filler-certification-corpus-2026-08-25.md` | 2 | 1 | **Delete** |
| `docs/engineering/research/filler-certification-source-qualification-2026-08-26.md` | 2 | 1 | **Delete** |
| `docs/engineering/research/filler-certification-sources-openrouter-2026-08-25.md` | 2 | 1 | **Delete** |
| `docs/engineering/research/filler-conditioning-634-findings-2026-08-28.md` | 1 | 0 | **Delete** |
| `docs/engineering/research/filler-evidence-retrieval-2026-09-19.md` | 2 | 0 | **Delete** |
| `docs/engineering/research/filler-local-open-weight-bakeoff-2026-08-27.md` | 2 | 0 | **Delete** |
| `docs/engineering/research/filler-spoken-certification-corpus-authority-2026-09-02.md` | 2 | 1 | **Delete** |
| `docs/engineering/research/filler-spoken-keyword-spotting-options-2026-09-02.md` | 2 | 1 | **Delete** |
| `docs/engineering/research/filler-suitability-local-safety-lanes-2026-09-02.md` | 2 | 1 | **Delete** |
| `docs/engineering/research/filler-visual-clean-source-smithsonian-2026-09-04.md` | 2 | 0 | **Delete** |
| `docs/engineering/research/filler-visual-portable-lane-2026-09-04.md` (823 lines) | 2 | 0 | **Delete** |
| `docs/engineering/research/go-1.27-opportunities-2026-08-28.md` | 1 | 0 | **Delete** |
| `docs/engineering/research/met-open-access-rights-prescreen-2026-09-04.md` | 2 | 0 | **Delete** |
| `docs/engineering/research/nfo-omdb-training-sources-2026-09-03.md` | 2 | 0 | **Delete** |
| `docs/engineering/research/planner-stock-model-bakeoff-2026-09-01.md` | 2 | 1 | **Delete** |
| `docs/engineering/research/prometheus-grafana-observability-2026-08-31.md` | 2 | 0 | **Delete** (shipped; `install/monitoring.md` covers it) |
| `docs/engineering/research/replacement-filler-source-candidates-2026-09-05.md` | 2 | 0 | **Delete** |
| `docs/engineering/research/subjective-movie-query-operational-signals.md` | 2 | 2 | **Delete** |
| `docs/engineering/research/suggestion-query-source-truth.md` (762 lines) | 2 | 3 | **Delete** |

**Totals.** Of 131 files: about 30 published pages after merges, 15 moved off the site, 3 kept
in place off the site (`docs/agents/`), about 75 deleted, and `design.md` + `design/` left to #779.
Every deletion that has references gets those references fixed in the same PR (lychee enforces it).
