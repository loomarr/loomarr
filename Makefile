# Loomarr harness contract. Targets are created as their phase needs them;
# unimplemented ones fail loudly rather than pretend to pass.
#
# ⚠ DO NOT restate the target list or the CI set in prose — here or anywhere else.
# This header used to end "CI mirrors: check + openapi-verify + test-pg + fe + e2e",
# which omitted six gates CI actually runs, and four other files carried their own
# disagreeing copies. `docs/dev/commands.md` is GENERATED from the `## ` comments below
# plus the `make` invocations in .github/workflows, and `make dev-docs-verify` fails the
# build on drift. Describe a target once, in its `## ` comment, and let the page follow.

GO      ?= go
CARGO   ?= cargo
RUST_FUZZ_TOOLCHAIN ?= nightly-2026-08-14
PKG     := ./...
BIN_DIR := bin

# STYLE is checked only on what ships to a reader. design.md, PROGRESS.md and
# docs/engineering/ are long-form internal records whose house style predates any linter, and
# whose volume would bury every real finding.
DOC_GLOBS := README.md CONTRIBUTING.md docs/help docs/install docs/dev

# LINKS are checked far wider, and the difference is deliberate: a broken link is objectively
# wrong everywhere, while a style rule is a preference that only earns its place in prose
# someone reads cover to cover.
#
# ⚠ This list is wider because the narrow one MISSED A REAL BREAK. Moving six superseded plans
# into docs/engineering/archive/ left nine dangling references — in PROGRESS.md, in two
# agent workflows, design/, and two Go doc comments — none of which the style set
# covers. Anything that can hold a relative link to a doc belongs here.
LINK_GLOBS := README.md CONTRIBUTING.md CLAUDE.md AGENTS.md CONTEXT.md PROGRESS.md docs 'design/*.md' .agents .claude

# CI-only shard passthrough for `make test` / `make check` (e.g. GO_SHARD=1/2). Empty by
# default — see the note on the `test` target, and PW_SHARD for the same contract on the
# visual suite. Never set this in a local gate run.
GO_SHARD ?=

# Build-tagged sources are INVISIBLE to `go vet ./...` and to golangci-lint, because both ask
# the Go build system which files exist and the build system honours `//go:build`. That blind
# spot ran for months (GH #227 §1): `go vet ./...` exited 0 while `go vet -tags '…' ./...`
# exited 1, and `TestLiveChain_RealFfmpegAdvancesThroughPrograms` — by its own comment "the
# only test that proves programs actually sequence" — had not compiled since V47 added
# `PlanFor` and left a stub behind in the same commit.
#
# ⚠ `go build -tags` would NOT have caught it, so don't "strengthen" this by adding one.
# `go build ./...` skips `_test.go` files entirely and 9 of the 11 tagged files are tests;
# `go vet` typechecks test files, which is why vet is the load-bearing half. `make fmt` was
# never blind here — it globs with `find`, not the build system.
#
# ⚠ The CUSTOM tags are NOT run as tests by the gate. They guard work needing real ffmpeg
# (`ffmpeg`), a real LLM (`eval`) or Docker (`integration`); `make check` stays hermetic
# (§19). The gate is only that they still COMPILE — which is free: measured 0.4s warm, and
# 3.2s for a never-before-seen tag set, because tags only recompile packages whose file
# selection actually changed.
#
# Platform constraints are guarded by the same inventory but compiled through their real
# GOOS adapter — passing `-tags windows` on Linux does NOT select `_windows.go` files and would
# falsely exclude `!windows` files. `windows-compile` is therefore the platform half of this gate.
#
# ⚠ HAND-MAINTAINED LIST — but guarded. A new `//go:build` tag is covered only if it is added
# here, the same drift class as `scripts/check-retired.sh`. `make tags-verify` enforces it in
# BOTH directions (a tag in the tree but not here, and one here that no build constraint uses)
# and runs as part of `check`, so the list can neither miss coverage nor overstate it.
CUSTOM_TAGS   := ffmpeg eval integration
PLATFORM_TAGS := windows
TAGS          := $(CUSTOM_TAGS) $(PLATFORM_TAGS)
SHELL_SCRIPTS := $(sort $(wildcard scripts/*.sh) $(wildcard web/scripts/*.sh))
comma     := ,
space     := $(subst ,, )
TAGS_CSV  := $(subst $(space),$(comma),$(CUSTOM_TAGS))

.DEFAULT_GOAL := help

.PHONY: help
help: ## List targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## ---- agent / worktree harness --------------------------------------------

.PHONY: agent-start agent-status agent-renew agent-prune agent-stop agent-env agent-baseline agent-verify agent-worktree bootstrap doctor agent-harness-test
agent-start: ## register this worktree and claim shared outputs (TASK=... CLAIMS=a,b)
	@./scripts/agent.sh start "$(TASK)" "$(CLAIMS)"

agent-status: ## list tool-neutral agent sessions across every worktree
	@./scripts/agent.sh status

agent-renew: ## renew this worktree's claim lease (AGENT_LEASE_HOURS=12)
	@./scripts/agent.sh renew

agent-prune: ## remove expired entries from the shared agent registry
	@./scripts/agent.sh prune

agent-stop: ## release this worktree's task and shared-output claims
	@./scripts/agent.sh stop

agent-env: ## show this worktree's isolated ports, database, compose project, and artifact path
	@./scripts/agent.sh env

agent-baseline: ## run make check once per clean commit/toolchain and share the green result across worktrees
	@./scripts/agent.sh baseline

agent-verify: ## run focused changed-file checks (not the final gate; BASE=origin/main)
	@BASE="$(or $(BASE),origin/main)" ./scripts/agent.sh verify

agent-worktree: ## create + bootstrap a ready-to-use sibling worktree off origin/main (TOPIC=branch; BASE=HEAD to stack)
	@COPY_ENV="$(or $(COPY_ENV),0)" BOOTSTRAP_SKIP_FE="$(or $(BOOTSTRAP_SKIP_FE),0)" BASE="$(BASE)" ./scripts/agent.sh worktree "$(TOPIC)"

bootstrap: ## build the Rust worker and prepare frontend, isolated directories, and dev identity
	@./scripts/agent.sh bootstrap

doctor: ## report toolchain drift, worktrees, ports, caches, and misplaced artifacts
	@./scripts/agent.sh doctor

agent-harness-test: ## regression-test worktree isolation and shared-output claims
	@./scripts/agent-harness-test.sh

.PHONY: compose-verify
compose-verify: ## verify Traefik, database wiring, and pinned release images
	@./scripts/check-compose.sh

.PHONY: release-verify release-notes-preview
release-verify: ## verify server, Android, and release-note publication policy
	@./scripts/check-release-tag.sh --self-test
	@./scripts/check-release-image-absence.sh --self-test
	@./scripts/android-version-code.sh --self-test
	@$(GO) test ./internal/releaseverify
	@$(GO) run ./cmd/releaseverify -root .

release-notes-preview: ## generate validated release notes (TAG required; optional PREVIOUS_TAG and OUTPUT)
	@test -n "$(TAG)" || { echo "TAG is required (for example TAG=v0.2.0)" >&2; exit 2; }
	@mkdir -p .artifacts
	@PREVIOUS_TAG="$(PREVIOUS_TAG)" ./scripts/generate-release-notes.sh "$(TAG)" "$(or $(OUTPUT),.artifacts/release-notes-$(TAG).md)"

.PHONY: backup-restore-verify backup-restore-drill
backup-restore-verify: ## isolated SQLite backup, destructive replacement, restore, and state validation
	$(GO) test -race ./internal/store -run '^TestSQLiteBackupRestoreDrill$$' -count=1

backup-restore-drill: backup-restore-verify ## SQLite + Docker-backed Postgres backup/restore drills
	$(GO) test -race -tags=integration ./internal/store -run '^TestPostgresBackupRestoreDrill$$' -count=1

## ---- the default gate ----------------------------------------------------

.PHONY: check check-static
check: check-static test ## complete local gate: repository contracts plus race-policy-aware unit tests

check-static: rust-check fmt shellcheck privacy-verify vet tags-verify vet-tags windows-compile lint agent-harness-test compose-verify release-verify go-race-verify ## repository contracts without the unit-test suite (CI runs this once beside test shards)

.PHONY: rust-check rust-test-worker rust-audit rust-fuzz
rust-check: rust-test-worker ## format, lint, build, and test the required Rust image worker
	$(CARGO) fmt --all -- --check
	$(CARGO) clippy --workspace --all-targets --all-features --locked -- -D warnings
	$(CARGO) test --workspace --all-features --locked

rust-test-worker: ## build the debug Rust image worker required by Go unit tests
	LOOMARR_RELEASE=dev $(CARGO) build --locked -p loomarr-image

rust-audit: ## check Rust advisories, licences, and dependency sources (needs cargo-deny)
	$(CARGO) deny check advisories licenses sources
	$(CARGO) deny --manifest-path rust/loomarr-image/fuzz/Cargo.toml check advisories licenses sources

rust-fuzz: ## fuzz the bounded Rust image protocol/decoder; optional FUZZ_SECONDS (needs nightly + cargo-fuzz)
	@seconds="$${FUZZ_SECONDS:-60}"; \
	  cd rust/loomarr-image; \
	  $(CARGO) +$(RUST_FUZZ_TOOLCHAIN) fuzz run protocol_decoder -- -max_total_time="$$seconds" -max_len=1048576

.PHONY: fmt
fmt: ## gofmt -l (fails if any file needs formatting)
	@out=$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*')); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

.PHONY: shellcheck
shellcheck: ## shellcheck every repository shell script
	shellcheck -S style $(SHELL_SCRIPTS)

.PHONY: privacy-verify
privacy-verify: ## captured private fixture literals must not re-enter the tracked tree
	@./scripts/check-private-fixtures.sh

.PHONY: vet
vet: ## go vet
	$(GO) vet $(PKG)

.PHONY: vet-tags
vet-tags: ## go vet over custom-tagged sources; platform constraints use their cross-compile gate
	$(GO) vet -tags '$(CUSTOM_TAGS)' $(PKG)

.PHONY: windows-compile
windows-compile: ## cross-compile every Go package and test for Windows (does not execute them)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) test -exec=true ./...

.PHONY: tags-verify
# Runs BEFORE vet-tags in `check`: it is ~0.1s and it validates the very list vet-tags consumes,
# so a missing tag is named before anything is compiled with an incomplete one.
tags-verify: ## the Makefile's TAGS list matches every //go:build tag in the tree, both ways
	@TAGS='$(TAGS)' ./scripts/check-tags.sh

.PHONY: lint
# ⚠ `--build-tags` WIDENS the file set, it never narrows it: files with no `//go:build` line
# compile under every tag set. Verified there is no negated constraint (`!ffmpeg`) anywhere in
# the tree — one of those WOULD be dropped by this flag, silently creating the blind spot this
# change exists to close. Re-check before adding a negated tag.
lint: ## golangci-lint v2 (run via `go run` so no global install needed)
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --build-tags '$(TAGS_CSV)'

.PHONY: test
test: rust-test-worker eval-contract ## unit tests with their required Rust worker (never touch the network — §19)
# ⚠ **-timeout is set explicitly because Go's default is 10m PER PACKAGE and `internal/api` grew
# past it.** Measured 2026-08-09: that package alone is 267s locally under `-race`, and a CI runner
# is roughly twice as slow — so it tripped the default and the job died with `panic: test timed out
# after 10m0s`. The dump named one test at `(0s)`, which is the tell that nothing HUNG: tests were
# still starting when the alarm fired, and the package was simply long. A genuine hang shows the
# stuck test with a large duration beside it.
#
# ⚠ This is an infrastructure limit, not the gate — raising it weakens nothing, because the gate is
# the assertions and every one of them still runs. What it must not do is hide growth: `internal/api`
# is ~500 tests each paying a fresh SQLite open plus migrations, and the fix when this bites again is
# to share that setup, NOT to raise the number a second time.
#
# GO_SHARD is a CI-only passthrough (`make check GO_SHARD=1/2`), the same contract as PW_SHARD:
# EMPTY by default, so a local `make test` — and `make check` — runs the whole tree. Sharding must
# never be implicit, or someone runs a fraction of the gate and reads the green as the whole thing.
# The shard COUNT lives in ci.yml's `matrix.shard`; see scripts/go-shard.sh for the split.
#
# ⚠ `&&`, not a `$(shell ...)` expansion. `$(shell)` swallows a non-zero exit and yields the empty
# string, and `go test` with NO packages exits 0 — so a bad GO_SHARD would have produced a silent
# green over zero tests, which is the exact failure this sharding must not be able to cause. Here a
# failing helper fails the recipe: `pkgs=$(...)` carries the substitution's status into the `&&`.
#
# `-race` is scoped, not tree-wide: scripts/go-race-policy.sh splits this shard's packages into the
# ones that run UNDER -race (the default — everything with any concurrency, incl. every httptest
# server) and a short, verified opt-out set of concurrency-free config/table-test packages that run
# without it, to skip the detector's ~2-3x overhead where no race is possible. `make go-race-verify`
# guards the opt-out list; the split is per-shard so it composes with GO_SHARD.
#
# ⚠ Each `go test` runs ONLY IF its list is non-empty. A shard whose opt-out subset is empty is
# normal (data-dependent on the split), but `go test` with no packages exits 0 — the same silent-
# green trap as above — so an empty list must SKIP the invocation, never invoke `go test` with none.
	@set -e; \
	pkgs="$$(./scripts/go-shard.sh $(GO_SHARD))"; \
	race_pkgs="$$(printf '%s\n' "$$pkgs" | ./scripts/go-race-policy.sh --race)"; \
	norace_pkgs="$$(printf '%s\n' "$$pkgs" | ./scripts/go-race-policy.sh --no-race)"; \
	if [ -n "$$race_pkgs" ]; then $(GO) test -race -timeout 25m $$race_pkgs; fi; \
	if [ -n "$$norace_pkgs" ]; then $(GO) test -timeout 25m $$norace_pkgs; fi

.PHONY: go-shard-verify
go-shard-verify: ## the GO_SHARD split must be a PARTITION of go list ./... (CI red on drift)
# ⚠ THIS IS A REAL GATE, not a sanity check. Sharding is the one optimization here that can
# QUIETLY SHRINK the suite: a split that drops a package does not fail — those tests simply never
# run, every shard reports success, and CI is green over code it never executed. Nothing else in
# the pipeline would notice. SHARDS must match ci.yml's `matrix.shard` count; CI passes it from
# `strategy.job-total` so the two cannot drift apart.
	@./scripts/go-shard.sh --verify $(or $(SHARDS),2)

.PHONY: go-race-verify
go-race-verify: ## every -race opt-out (scripts/go-race-policy.sh RACE_OFF) must be a real package
# ⚠ A GUARD, not decoration. The opt-out list is FAIL-SAFE by construction — race stays ON for
# anything not listed, so a forgotten new package is race-checked, never silently skipped. What this
# catches is the other drift: a RACE_OFF entry that no longer names a real package (renamed, moved,
# deleted). That entry would opt nothing out — harmless for coverage, but the list would be lying
# about what it excludes, so CI goes red until it is corrected.
	@./scripts/go-race-policy.sh --verify

.PHONY: test-ffmpeg
test-ffmpeg: ## playout tests that EXECUTE ffmpeg (needs ffmpeg+ffprobe; not in `make check`)
	$(GO) test -tags ffmpeg -run 'TestLive' ./internal/playout/ ./internal/api/ -v

.PHONY: eval-contract eval eval-cert eval-matrix
eval-contract: ## hermetic semantic-evaluation contracts; never contacts a model, Library, or TMDB
	LOOMARR_EVAL_CONTRACT_ONLY=1 $(GO) test -tags=eval ./internal/eval/

eval: ## semantic eval: real intents → real LLM → scored (needs LLM_*/LIBRARY_*/TMDB_API_KEY; NOT in the hermetic gate)
	$(GO) test -tags=eval -v -timeout 20m ./internal/eval/

eval-cert: ## certify exact starter/adversarial intents; fails on missing config and writes a scorecard
	@eval "$$(./scripts/dev-env.sh export)"; \
	  report="$${LOOMARR_EVAL_OUT:-$$LOOMARR_ARTIFACT_DIR/semantic-certification.json}"; \
	  mkdir -p "$$(dirname "$$report")"; \
	  LOOMARR_EVAL_REQUIRED=1 LOOMARR_EVAL_OUT="$$report" \
	    $(GO) test -count=1 -tags=eval -v -timeout 20m ./internal/eval/

eval-matrix: ## explicitly certify local + OpenRouter generation sequentially (manual, resource-heavy)
	@test -n "$$OPENROUTER_API_KEY" || { echo "eval-matrix: OPENROUTER_API_KEY is required" >&2; exit 2; }; \
	  test -n "$$OPENROUTER_MODEL" || { echo "eval-matrix: OPENROUTER_MODEL is required" >&2; exit 2; }; \
	  test "$$LOOMARR_EVAL_ALLOW_LOCAL" = "1" || { echo "eval-matrix: refusing local inference; confirm an idle host with sufficient RAM/VRAM, then set LOOMARR_EVAL_ALLOW_LOCAL=1" >&2; exit 2; }; \
	  eval "$$(./scripts/dev-env.sh export)"; \
	  judge_model="$${OPENROUTER_JUDGE_MODEL:-$$OPENROUTER_MODEL}"; \
	  status=0; \
	  LOOMARR_EVAL_PROFILE=local \
	  LOOMARR_EVAL_OUT="$$LOOMARR_ARTIFACT_DIR/semantic-certification-local.json" \
	  LOOMARR_EVAL_JUDGE="$$judge_model" LOOMARR_EVAL_JUDGE_PROVIDER=openai \
	  LOOMARR_EVAL_JUDGE_URL=https://openrouter.ai/api/v1 \
	  LOOMARR_EVAL_JUDGE_API_KEY="$$OPENROUTER_API_KEY" \
	    $(MAKE) eval-cert || status=$$?; \
	  LLM_PROVIDER=openai LLM_URL=https://openrouter.ai/api/v1 \
	  LLM_MODEL="$$OPENROUTER_MODEL" LLM_API_KEY="$$OPENROUTER_API_KEY" \
	  LOOMARR_EVAL_PROFILE=openrouter \
	  LOOMARR_EVAL_OUT="$$LOOMARR_ARTIFACT_DIR/semantic-certification-openrouter.json" \
	  LOOMARR_EVAL_JUDGE="$$judge_model" LOOMARR_EVAL_JUDGE_PROVIDER=openai \
	  LOOMARR_EVAL_JUDGE_URL=https://openrouter.ai/api/v1 \
	  LOOMARR_EVAL_JUDGE_API_KEY="$$OPENROUTER_API_KEY" \
	    $(MAKE) eval-cert || status=$$?; \
	  exit "$$status"

## ---- build / run ---------------------------------------------------------

.PHONY: build rust-build image-cert image-bench image-parallelism-bench
build: rust-build ## build the cgo-free Go server and required Rust image worker
	release="$${LOOMARR_RELEASE:-dev}"; \
	  CGO_ENABLED=0 $(GO) build \
	    -ldflags="-X github.com/loomarr/loomarr/internal/buildinfo.version=$$release" \
	    -o $(BIN_DIR)/loomarr ./cmd/loomarr; \
	  $(BIN_DIR)/loomarr-image capabilities --protocol 1 --self-test | grep -q "\"release\":\"$$release\""

rust-build: ## build the required Rust image worker
	LOOMARR_RELEASE="$${LOOMARR_RELEASE:-dev}" $(CARGO) build --release --locked -p loomarr-image
	install -d $(BIN_DIR)
	install -m 0755 target/release/loomarr-image $(BIN_DIR)/loomarr-image

image-cert: rust-build ## certify the Rust image worker; optional IMAGE_CERT_CORPUS=/absolute/path
	@eval "$$(./scripts/dev-env.sh export)"; \
	  report="$${IMAGE_CERT_REPORT:-$$LOOMARR_ARTIFACT_DIR/image-certification.json}"; \
	  if [ -n "$${IMAGE_CERT_CORPUS:-}" ]; then \
	    LOOMARR_RELEASE="$${LOOMARR_RELEASE:-dev}" $(GO) run ./cmd/image-cert \
	      --worker "$(BIN_DIR)/loomarr-image" --report "$$report" --corpus "$$IMAGE_CERT_CORPUS"; \
	  else \
	    LOOMARR_RELEASE="$${LOOMARR_RELEASE:-dev}" $(GO) run ./cmd/image-cert \
	      --worker "$(BIN_DIR)/loomarr-image" --report "$$report"; \
	  fi

image-bench: rust-build ## benchmark release-worker AVIF ladders; optional IMAGE_BENCH_RUNS/ROLES/REPORT
	@eval "$$(./scripts/dev-env.sh export)"; \
	  report="$${IMAGE_BENCH_REPORT:-$$LOOMARR_ARTIFACT_DIR/image-benchmark.json}"; \
	  LOOMARR_RELEASE="$${LOOMARR_RELEASE:-dev}" $(GO) run ./cmd/image-bench \
	    --worker "$(BIN_DIR)/loomarr-image" --report "$$report" \
	    --roles "$${IMAGE_BENCH_ROLES:-poster,backdrop,icon}" \
	    --workers "$${IMAGE_BENCH_WORKERS:-1}" \
	    --avif-threads "$${IMAGE_BENCH_AVIF_THREADS:-1}"

image-parallelism-bench: rust-build ## compare AVIF process/thread shapes at 2/4/8 CPUs (opt-in, Linux)
	@eval "$$(./scripts/dev-env.sh export)"; \
	  report_dir="$${IMAGE_BENCH_REPORT_DIR:-$$LOOMARR_ARTIFACT_DIR/image-parallelism}"; \
	  LOOMARR_RELEASE="$${LOOMARR_RELEASE:-dev}" GO="$(GO)" \
	    ./scripts/image-parallelism-bench.sh "$(BIN_DIR)/loomarr-image" "$$report_dir"

.PHONY: dev
dev: ## dev compose stack (external deps: tunarr-dev; portable Mac/Linux, CPU transcode)
	@eval "$$(./scripts/dev-env.sh export)"; \
	  echo "dev: $$COMPOSE_PROJECT_NAME — Tunarr http://localhost:$$TUNARR_DEV_PORT"; \
	  docker compose -p "$$COMPOSE_PROJECT_NAME" -f docker/compose.dev.yaml up -d

.PHONY: test-sso
test-sso: ## SSO against REAL Authelia + Authentik containers (requires Docker)
	@# Not in `make check`: §19 keeps the default suite Docker-free, like test-pg.
	@#
	@# TWO providers, because each found a bug the other could not. Authelia: profile claims
	@# live at userinfo, not in the id_token, so every login against a default install was
	@# refused. Authentik: its issuer is path-based WITH a trailing slash, which our
	@# normalisation stripped — discovery failed outright. A hand-written stub IdP showed
	@# neither, because it was our own reading of the spec on both sides of the wire.
	$(GO) test -count=1 -tags=integration -timeout 20m -run 'TestSSO_AgainstReal' ./internal/auth/

.PHONY: dev-be
dev-be: rust-dev-build ## backend with live reload (Air) — rebuilds + restarts on Go/Rust changes
	@# Air is a dev tool, not a dependency (§14): run via `go run` so it is never added to
	@# go.mod and needs no manual install step. A committed .air.toml with no way to run it
	@# is how this box spent a session serving a stale binary.
	@#
	@# ⚠ SINGLE-INSTANCE GUARD (scripts/dev-be-guard.sh). Air itself has no "am I already
	@# running?" check, so a SECOND `make dev-be` used to start a second Air + binary that lost
	@# the :8080 bind and exited — while the stale one kept serving OLD code. That zombie cost
	@# DAYS of "my fix didn't take". The guard refuses to start a duplicate (or, with
	@# DEV_BE_REPLACE=1, cleanly replaces ONLY the loomarr dev binary — never a blanket kill).
	@eval "$$(./scripts/dev-env.sh export)"; \
	  mkdir -p .agent-data "$$LOOMARR_ARTIFACT_DIR" "$${LOOMARR_AGENT_FILLER_DIR:-.filler-drop}" "$${LOOMARR_AGENT_PREPARED_DIR:-.agent-data/prepared}"; \
	  echo "dev-be: $$LOOMARR_INSTANCE — http://localhost:$$LOOMARR_DEV_PORT"; \
	  sh scripts/dev-be-guard.sh; \
	  sh -c 'if [ "$${DEV_BE_NO_WATCHDOG:-0}" != "1" ]; then \
	      sh scripts/dev-be-watchdog.sh & wd=$$!; trap "kill $$wd 2>/dev/null" EXIT INT TERM; fi; \
	    exec $(GO) run github.com/air-verse/air@v1.67.3'
	@# ⚠ STALE-BINARY WATCHDOG (scripts/dev-be-watchdog.sh). Even with `.air.toml`'s
	@# stop_on_error=false + poll=true, Air can still end up ALIVE but not rebuilding (poll loop
	@# stalled) — serving a frozen binary while your saves do nothing. Config can't detect its own
	@# watcher dying; this out-of-band watchdog does. It runs beside Air, notices when the running
	@# binary stays older than the newest .go source, and self-heals (nudge Air, then restart the
	@# binary via Air's own path — never a competing process). Backgrounded here; the `trap` reaps
	@# it when Air exits so `make dev-be` leaves nothing behind. Opt out with DEV_BE_NO_WATCHDOG=1.

.PHONY: rust-dev-build
rust-dev-build: ## build the required Rust worker for local development
	LOOMARR_RELEASE=dev $(CARGO) build --locked -p loomarr-image
.PHONY: dev-gpu
dev-gpu: ## dev compose stack with NVIDIA transcode overlay (Linux + nvidia-container-toolkit)
	@eval "$$(./scripts/dev-env.sh export)"; \
	  echo "dev-gpu: $$COMPOSE_PROJECT_NAME — Tunarr http://localhost:$$TUNARR_DEV_PORT"; \
	  docker compose -p "$$COMPOSE_PROJECT_NAME" -f docker/compose.dev.yaml -f docker/compose.dev.gpu.yaml up -d

.PHONY: dev-fe
dev-fe: ## frontend with HMR on this worktree's isolated port, proxying its backend
	@eval "$$(./scripts/dev-env.sh export)"; \
	  echo "dev-fe: $$LOOMARR_INSTANCE — http://localhost:$$LOOMARR_FE_PORT -> $$LOOMARR_API"; \
	  cd $(WEB) && pnpm --filter @loomarr/web dev

## ---- store conformance (Phase 3/4) --------------------------------------

.PHONY: test-pg
test-pg: rust-dev-build ## all real-Postgres integration suites (store, backend transition, app; testcontainers; requires Docker)
# ⚠ The `-run TestPostgresConformance` filter this used to carry meant every OTHER integration test
# in the package compiled and never ran — including TestMigrateSQLiteToPostgres, which its own file
# header calls "the V11 gate", plus TestMigrateCoversEveryTable and the three TestPreflight* tests.
# A filter is invisible in the output: the target printed a genuine pass and said nothing about the
# six tests it had not selected. The migrator was broken the whole time (seeded destination rows
# collided on insert) and no gate could have told anyone.
#
# This is the third variant of "green that proves nothing" this repo has hit — after a pipe masking
# an exit code, and a missing -tags=integration printing `ok … [no tests to run]`. A test existing,
# compiling, and EXECUTING are three separate facts.
	$(GO) test -race -tags=integration ./internal/store/ ./internal/backendtransition/ ./internal/app/

## ---- OpenAPI (Phase 8) ---------------------------------------------------

.PHONY: openapi
openapi: ## export api/openapi.yaml from the running definitions
	$(GO) run ./cmd/openapi api/openapi.yaml

.PHONY: openapi-verify
openapi-verify: openapi ## regenerated spec must match committed (CI red on drift)
	@git diff --exit-code api/openapi.yaml

ci-lint: ## actionlint over .github/workflows — catches what YAML parsing cannot
	@$(GO) run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 .github/workflows/*.yml

retired-verify: ## retired identifiers must not appear as live instructions (CI red on drift)
	@./scripts/check-retired.sh

## ---- config docs (settings registry) ------------------------------------

.PHONY: config-docs
config-docs: ## generate docs/configuration.md from the settings registry
	$(GO) run ./cmd/config-docs docs/configuration.md

.PHONY: config-docs-verify
config-docs-verify: config-docs ## regenerated config docs must match committed (CI red on drift)
	@git diff --exit-code docs/configuration.md

## ---- architecture map (design.md §2) -------------------------------------
# The same discipline as config-docs, applied to the one section a newcomer reads first.
# §2's diagram had omitted `filler` (explicitly, "to keep the diagram legible") and `playout`
# (which arrived later as §9.1) — between them two of the five largest packages. A hand-kept
# architecture map is the same shape as scripts/check-retired.sh and the TAGS list: a list
# that drifts. This one is derived from each package's own doc comment and its imports.
#
# ⚠ Writes ONE marker-delimited block inside design.md and refuses to run over a malformed
# marker pair — the rest of the file is hand-written and authoritative (AGENTS.md doc-first).

.PHONY: arch-docs
arch-docs: ## regenerate the §2 package map in docs/design.md from the code
	$(GO) run ./cmd/arch-docs docs/design.md internal

.PHONY: arch-docs-verify
arch-docs-verify: arch-docs ## regenerated package map must match committed (CI red on drift)
	@git diff --exit-code docs/design.md

## ---- dev docs (the command contract) ------------------------------------

.PHONY: dev-docs
dev-docs: ## generate docs/dev/commands.md from this Makefile + the CI workflows
	$(GO) run ./cmd/dev-docs docs/dev/commands.md

.PHONY: dev-docs-verify
dev-docs-verify: dev-docs ## regenerated command reference must match committed (CI red on drift)
	@git diff --exit-code docs/dev/commands.md

## ---- documentation lint --------------------------------------------------

# Three checks over the prose. Nothing needs a global install: markdownlint comes from npx,
# and the two Rust/Go tools run from pinned Docker images — the same approach as PW_IMAGE,
# and the reason a contributor can run the doc gate without a toolchain of its own.
#
# ⚠ lychee runs OFFLINE deliberately. Checking external URLs on every PR imports the whole
# internet's link rot as CI flake, and a red build nobody can fix stops being a gate.
# Relative links are the class that has actually broken here — twice. See lychee.toml.
#
# ⚠ markdownlint reads .markdownlint-cli2.jsonc for BOTH its globs and its rules, so it takes
# no path arguments here. Passing DOC_GLOBS as well would silently override the ignores and
# lint the generated command reference.
# ⚠ lychee publishes no semver tags on Docker Hub — only `sha-*`, `nightly` and `master`. The
# sha tag is the pin; `latest` does not exist for this image and `master` is a moving target.
LYCHEE_IMAGE ?= lycheeverse/lychee:sha-c36b9aa-alpine
VALE_IMAGE   ?= jdkato/vale:v3.17.1
DOCKER_DOC   := docker run --rm -v "$(CURDIR)":/work -w /work

.PHONY: docs-lint
docs-lint: docs-lint-md docs-lint-links docs-lint-prose ## markdownlint + lychee (offline) + Vale over the prose set

.PHONY: docs-lint-md
docs-lint-md: ## markdown structure (globs + rules live in .markdownlint-cli2.jsonc)
	npx --yes markdownlint-cli2@0.20.0

.PHONY: docs-lint-links
docs-lint-links: ## relative-link check, offline, over the WIDE set (see lychee.toml for why)
	$(DOCKER_DOC) $(LYCHEE_IMAGE) --offline --no-progress $(LINK_GLOBS)

.PHONY: docs-lint-prose
docs-lint-prose: ## repo vocabulary + proper-noun casing (.vale.ini — no stock style package)
	$(DOCKER_DOC) $(VALE_IMAGE) $(DOC_GLOBS)

## ---- frontend (Phase 13) -------------------------------------------------

WEB := web

.PHONY: fe-install
fe-install: ## install the web workspace (pnpm)
	cd $(WEB) && pnpm install --frozen-lockfile

.PHONY: fe-tokens
fe-tokens: ## regenerate design-token artifacts from packages/tokens (CI diff must be empty)
	cd $(WEB) && pnpm --filter @loomarr/tokens generate

.PHONY: fe-tokens-verify
fe-tokens-verify: fe-tokens ## regenerated token artifacts must match committed
	@git diff --exit-code web/packages/tokens/generated

.PHONY: fe-codegen
fe-codegen: ## regenerate tokens + orval api client from api/openapi.yaml
	cd $(WEB) && pnpm codegen

.PHONY: fe-lint
fe-lint: ## Biome lint + format check (web/)
	cd $(WEB) && pnpm lint

.PHONY: fe-lint-fix
fe-lint-fix: ## Biome autofix — format + safe lint fixes (web/)
	cd $(WEB) && pnpm biome check --write

# FE_SHARD is a CI-only passthrough (`make fe FE_SHARD=1/2`), same contract as GO_SHARD and
# PW_SHARD: EMPTY by default so a local `make fe` runs the whole suite. The shard COUNT lives
# in ci.yml's `matrix.shard`.
#
# ⚠ ONLY apps/web is sharded, and that is not arbitrary. 166 of the 172 test files live there;
# the other three packages hold 12 between them. More importantly `packages/core` and
# `packages/tokens` run plain `vitest run` WITHOUT --passWithNoTests, so any shard that handed
# them zero files would exit non-zero — a red CI caused purely by the split, appearing only at
# higher shard counts. They stay unsharded, which is both safe and free.
#
# ⚠ NO `--` BEFORE THE FLAG. `pnpm --filter X test -- --shard=1/2` passes `-- --shard=1/2` to
# vitest, which reads it as a FILENAME FILTER, matches nothing, falls back to everything, and
# exits 0 having run all 166 files. Measured while writing this: the `--` form reported "166
# passed" for BOTH shards — a green, doubled, entirely unsharded run that looks exactly like a
# working one. The form below reports 83 and 83.
FE_SHARD ?=
FE_SHARD_ARG := $(if $(FE_SHARD),--shard=$(FE_SHARD),)

.PHONY: fe
fe: ## biome + codegen + typecheck + unit tests + embedded SPA + storybook gallery
	cd $(WEB) && pnpm codegen && pnpm lint && pnpm --filter @loomarr/web... -r --parallel typecheck \
	  && pnpm --filter @loomarr/web... --filter '!@loomarr/web' -r --parallel test \
	  && pnpm --filter @loomarr/web test $(FE_SHARD_ARG) \
	  && pnpm --filter @loomarr/web build && pnpm --filter @loomarr/web build-storybook
	@touch internal/web/dist/.gitkeep

.PHONY: clients
clients: ## lint, test, typecheck, and bundle the shared browser, mobile, and TV scaffold
	cd $(WEB) && pnpm exec biome check apps/mobile apps/tv apps/web/client-platform-proof.html \
	  apps/web/src/client-platform-proof apps/web/tests/client-platform-proof.ssr.test.tsx \
	  apps/web/vite.client-platform.config.ts \
	  packages/design-system packages/ui turbo.json \
	  && pnpm imports:check && pnpm lint:boundaries && pnpm clients:check

CLIENT_APP ?= mobile
.PHONY: client-android-debug
client-android-debug: ## memory-bounded arm64 debug build (CLIENT_APP=mobile|tv)
	cd $(WEB) && ./scripts/build-android-client.sh $(CLIENT_APP)

.PHONY: storybook
storybook: ## Storybook dev workshop on this worktree's isolated port
	@eval "$$(./scripts/dev-env.sh export)"; \
	  echo "storybook: http://localhost:$$LOOMARR_STORYBOOK_PORT"; \
	  cd $(WEB) && pnpm --filter @loomarr/web exec storybook dev -p "$$LOOMARR_STORYBOOK_PORT" --no-open

.PHONY: storybook-build
storybook-build: ## offline storybook-static build (what fe-visual snapshots)
	cd $(WEB) && pnpm --filter @loomarr/web build-storybook

# Playwright Docker image = the reference rasterizer (§5.2): Linux + software rendering,
# deterministic and identical to CI. Baselines are the *-linux.png it writes. Keep the
# tag pinned to the @playwright/test version in web/apps/web/package.json so the image's
# browsers match exactly. The container reuses the host's (JS-only) node_modules read
# through the bind mount and the browsers baked into the image — no in-container install,
# so the host's macOS binaries are never touched.
# ⚠ `docker run` inherits NOTHING from the host environment, so the container has to be TOLD it
# is in CI. Without it `process.env.CI` is undefined inside the image: the worker count falls back
# to Playwright's half-the-cores default, AND `forbidOnly` never applies — so a stray `test.only`
# silently narrows the suite while passing.
#
# ⚠ **This was `-e CI` (bare) and that forwards NOTHING when `CI` is unset on the host** — which
# is every local run. So locally the suite ran 12 workers on a 24-core box and `forbidOnly` was
# OFF, while CI ran with both. `?=` keeps it overridable (`make fe-visual PW_CI=`), and an
# exported `CI=true` still wins.
#
# ⚠ **The win here is CONSISTENCY, not speed** — measured 2026-08-02, and the measurement is worth
# recording because the obvious assumption is wrong: doubling the workers took the visual suite
# from 96s to 86s, about 10%. 706 fast screenshot tests are not worker-bound; something else
# (container I/O, the static server, per-test context setup) is already the floor. Do not reach
# for more parallelism here expecting a large win — and GPU passthrough is a dead end for the same
# reason, since these are static pages with no video, WebGL or canvas to rasterize.
#
# What it DOES buy is a local gate that behaves like CI: all cores, and `test.only` refused in
# both places rather than only one.
PW_CI ?= 1

# ⚠ Forwarded so the CONTAINER can tell a real runner from a developer's desktop. `CI=1` above
# is deliberately set for local runs too (that is the whole point of PW_CI), so inside the
# container `CI` says "behave like CI" and cannot answer "whose hardware is this".
#
# playwright.shared.ts needs that second answer, because worker count is a MEMORY decision:
# a 24-core workstation given cpus() workers boots 24 browsers and swap-thrashes into a hard
# lock, measured going from 16GB free to 2GB in about a minute under fe-visual-update. It is
# empty locally and `true` on the runner, which GitHub sets and this Makefile never fabricates.
#
# Everything else PW_CI buys is unchanged — notably `forbidOnly`, so `test.only` is still
# refused locally exactly as it is in CI.
PW_REAL_CI ?= $(GITHUB_ACTIONS)
PW_IMAGE := mcr.microsoft.com/playwright:v1.62.0-noble

# PW_SHARD is a CI-only passthrough (`make fe-visual PW_SHARD=--shard=1/4`). Empty by
# default, so a local `make fe-visual` still runs the WHOLE suite — sharding must never
# be the default, or someone runs half the gate and reads it as green. CI splits the
# suite across runners purely for wall-clock, and public-repo standard runners are free,
# so N runners cost the same as one and finish sooner.
#
# ⚠ The shard COUNT lives in ci.yml's `matrix.shard` and nowhere else — the denominator is
# derived there from `strategy.job-total`. Do not write a specific N into this file: the
# "1/2" that used to be in the line above outlived the 2-shard config it described.
PW_SHARD ?=

# ⚠ Run the container AS THE HOST USER, or everything it writes into the bind mount is owned by
# root. The Playwright image runs as root by default, so `test-results/` and its per-test artifacts
# land root-owned in a directory the host user cannot delete — and the symptom shows up far from
# the cause: `git worktree remove` half-fails ("Permission denied"), git DEREGISTERS the worktree
# anyway, and ~550MB per worktree is stranded on disk with no git record that it exists. Three
# worktrees had accumulated 1.7GB that way before this flag was added.
#
# ⚠ `HOME=/tmp` rides along and is not optional. As a non-root uid the container's default HOME is
# `/root`, which is not writable, so anything wanting a cache/config dir fails in a way that reads
# like a Playwright bug rather than a permissions one. `/tmp` is writable for any uid.
#
# The browsers themselves are unaffected: they live in /ms-playwright, which is world-readable.
PW_DOCKER_USER ?= --user $(shell id -u):$(shell id -g) -e HOME=/tmp

.PHONY: fe-visual
fe-visual: storybook-build ## Playwright visual + a11y over storybook-static, in the pinned Docker image (§5.2)
	docker run --rm --ipc=host $(PW_DOCKER_USER) -e CI=$(PW_CI) -e GITHUB_ACTIONS=$(PW_REAL_CI) -v "$(PWD)/web:/work" -w /work/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test $(PW_SHARD)

.PHONY: fe-visual-update
fe-visual-update: storybook-build ## regenerate the committed Linux baselines in the Docker image (sanctioned update path)
	docker run --rm --ipc=host $(PW_DOCKER_USER) -e CI=$(PW_CI) -e GITHUB_ACTIONS=$(PW_REAL_CI) -v "$(PWD)/web:/work" -w /work/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test --update-snapshots

# The e2e suite drives the REAL embedded SPA build, which Vite writes to
# internal/web/dist — OUTSIDE web/. So unlike fe-visual it mounts the repo ROOT, and
# runs from /work/web/apps/web (node_modules still resolves up to /work/web).
.PHONY: e2e
e2e: fe-build ## wizard e2e smoke vs a mocked backend, in the pinned Docker image (13.3 gate)
	docker run --rm --ipc=host $(PW_DOCKER_USER) -e CI=$(PW_CI) -e GITHUB_ACTIONS=$(PW_REAL_CI) -v "$(PWD):/work" -w /work/web/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test --config=playwright.e2e.config.ts

.PHONY: tuner-e2e
tuner-e2e: fe-build ## 100-Channel tuner controller matrix in Chromium, Firefox, and WebKit (§9.1)
	docker run --rm --ipc=host $(PW_DOCKER_USER) -e CI=$(PW_CI) -e GITHUB_ACTIONS=$(PW_REAL_CI) -v "$(PWD):/work" -w /work/web/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test --config=playwright.tuner.config.ts

.PHONY: tuner-e2e-host
tuner-e2e-host: fe-build ## 100-Channel tuner controller matrix in host-installed browsers (§9.1)
	cd web/apps/web && node_modules/.bin/playwright test --config=playwright.tuner.config.ts

## ---- Maintainer smoke (NOT CI) -------------------------------------------
# §21's second half: the real-stack run. Deliberately NOT in CI and NOT part of `check` —
# it needs the maintainer's .env and touches their live media server. Uses a throwaway
# database and its own Tunarr container; the requester is omitted so nothing downloads.
# Deliberately does NOT depend on fe-build: the stack is left running between runs so
# iterating on specs costs seconds. Run `make fe-build` yourself when the UI changed.
smoke: ## maintainer smoke: drive the REAL stack (starts it only if it isn't up)
	./scripts/smoke.sh

smoke-reset: ## force a true FIRST RUN (wipes the smoke database + Tunarr), then run
	./scripts/smoke.sh reset

smoke-livetv: ## Live TV wiring vs a DISPOSABLE Jellyfin (destroyed after — never touches your media server)
	./scripts/smoke.sh livetv

smoke-down: ## tear down the smoke stack (container, volume, temp database)
	./scripts/smoke.sh down

.PHONY: e2e-update
e2e-update: fe-build ## regenerate the committed e2e page snapshots (sanctioned update path)
	docker run --rm --ipc=host $(PW_DOCKER_USER) -e CI=$(PW_CI) -e GITHUB_ACTIONS=$(PW_REAL_CI) -v "$(PWD):/work" -w /work/web/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test --config=playwright.e2e.config.ts --update-snapshots

# Just the SPA build the e2e suite serves (a subset of `make fe`, so the gate doesn't
# rebuild Storybook or re-run the unit suite to check a flow).
.PHONY: fe-build
fe-build:
	cd $(WEB) && pnpm codegen && pnpm --filter @loomarr/web build

.PHONY: seed
seed: ## populate a dev store via the real domain paths (approval gate honored — AGENTS.md)
	DATABASE_URL=$${DATABASE_URL:-sqlite://./loomarr-dev.db} go run ./cmd/seed

.PHONY: android-tokens
android-tokens: ## regenerate the Android design tokens from the shared tokens.json
	node scripts/gen-android-tokens.mjs

.PHONY: android-tokens-verify
android-tokens-verify: android-tokens ## regenerated tokens must match committed (CI red on drift)
	@git diff --exit-code android/app/src/main/java/loomarr/media/design/LoomarrTokens.kt

.PHONY: android-load
android-load: ## report heavy local processes before starting a build
	@sh scripts/dev-load-check.sh

# ⚠ NOT --no-daemon. A reused warm daemon is ONE bounded JVM; --no-daemon starts a fresh one per
# invocation, which is worse under the rapid successive builds an agent session produces. The
# ceilings live in android/gradle.properties instead, where they bound every entry point.
.PHONY: android
android: android-tokens-verify ## Android TV client — tokens + ktlint + Android Lint + unit tests + screenshots + debug APK
# ⚠ `verifyRoborazziDebug`, NOT `testDebugUnitTest`. It runs the same unit tests AND compares the
# screenshot tests against their committed baselines. Roborazzi does nothing unless a task puts it
# in record or verify mode, so under plain `testDebugUnitTest` the screenshot tests still execute,
# capture nothing, and pass — a gate that cannot fail.
	cd android && ./gradlew ktlintCheck lintDebug verifyRoborazziDebug assembleDebug

.PHONY: android-release-test
android-release-test: ## build an ephemeral signed AAB and verify release identity, ABIs, and 16 KiB alignment
	@./scripts/test-android-release.sh

.PHONY: android-fmt
android-fmt: ## Android TV client — apply ktlint formatting
	cd android && ./gradlew ktlintFormat

.PHONY: android-screenshots
android-screenshots: ## Android TV client — re-record screenshot baselines (review the diff!)
# ⚠ Recording ACCEPTS whatever the UI currently renders — it is not verification. Run it when a
# visual change is intended, then LOOK at the resulting PNG diff before committing: a baseline
# rewritten without being read turns the gate into a rubber stamp. That is the same trap web's
# visual suite carries, where `--update-snapshots` on a crashing story cheerfully records the crash.
	cd android && ./gradlew recordRoborazziDebug

.PHONY: android-stop
android-stop: ## stop the Gradle/Kotlin daemons this module started
	cd android && ./gradlew --stop
