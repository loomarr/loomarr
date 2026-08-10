# Loomarr harness contract (CLAUDE.md "Commands"). Targets are created as their
# phase needs them; unimplemented ones fail loudly rather than pretend to pass.
# CI mirrors: check + openapi-verify + test-pg + fe + e2e.

GO      ?= go
PKG     := ./...
BIN_DIR := bin

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
# ⚠ These tags are NOT run as tests by the gate. They guard work needing real ffmpeg
# (`ffmpeg`), a real LLM (`eval`) or Docker (`integration`); `make check` stays hermetic
# (§19). The gate is only that they still COMPILE — which is free: measured 0.4s warm, and
# 3.2s for a never-before-seen tag set, because tags only recompile packages whose file
# selection actually changed.
#
# ⚠ HAND-MAINTAINED LIST, CURRENTLY UNGUARDED. A new `//go:build` tag is covered only if it is
# added here — the same drift class as `scripts/check-retired.sh`. `make tags-verify` is the
# INTENDED guard and does not guard anything yet: it extracts both lists and prints them, but
# its comparison policy is an unfilled TODO, which is why it is not in `check`. Until that
# lands, adding a tag here is a manual step nothing enforces.
TAGS      := ffmpeg eval integration
comma     := ,
space     := $(subst ,, )
TAGS_CSV  := $(subst $(space),$(comma),$(TAGS))

.DEFAULT_GOAL := help

.PHONY: help
help: ## List targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## ---- the default gate ----------------------------------------------------

.PHONY: check
check: fmt vet vet-tags lint test ## fmt + vet (incl. tagged) + lint + unit tests (the default gate)

.PHONY: fmt
fmt: ## gofmt -l (fails if any file needs formatting)
	@out=$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*')); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

.PHONY: vet
vet: ## go vet
	$(GO) vet $(PKG)

.PHONY: vet-tags
vet-tags: ## go vet over the build-tagged sources (invisible to plain `go vet` — see TAGS)
	$(GO) vet -tags '$(TAGS)' $(PKG)

.PHONY: tags-verify
# ⚠ NOT in `check` yet — the comparison policy is an unfilled TODO in the script, so wiring it
# into the gate today would add a step that exits 0 while proving nothing. Add it to `check`
# in the same change that fills it in.
tags-verify: ## the Makefile's TAGS list still covers every //go:build tag in the tree
	@TAGS='$(TAGS)' ./scripts/check-tags.sh

.PHONY: lint
# ⚠ `--build-tags` WIDENS the file set, it never narrows it: files with no `//go:build` line
# compile under every tag set. Verified there is no negated constraint (`!ffmpeg`) anywhere in
# the tree — one of those WOULD be dropped by this flag, silently creating the blind spot this
# change exists to close. Re-check before adding a negated tag.
lint: ## golangci-lint v2 (run via `go run` so no global install needed)
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --build-tags '$(TAGS_CSV)'

.PHONY: test
test: ## unit tests only (never touch the network — §19)
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
	@pkgs="$$(./scripts/go-shard.sh $(GO_SHARD))" && $(GO) test -race -timeout 25m $$pkgs

.PHONY: go-shard-verify
go-shard-verify: ## the GO_SHARD split must be a PARTITION of go list ./... (CI red on drift)
# ⚠ THIS IS A REAL GATE, not a sanity check. Sharding is the one optimization here that can
# QUIETLY SHRINK the suite: a split that drops a package does not fail — those tests simply never
# run, every shard reports success, and CI is green over code it never executed. Nothing else in
# the pipeline would notice. SHARDS must match ci.yml's `matrix.shard` count; CI passes it from
# `strategy.job-total` so the two cannot drift apart.
	@./scripts/go-shard.sh --verify $(or $(SHARDS),2)

.PHONY: test-ffmpeg
test-ffmpeg: ## playout tests that EXECUTE ffmpeg (needs ffmpeg+ffprobe; not in `make check`)
	$(GO) test -tags ffmpeg -run 'TestLive' ./internal/playout/ ./internal/api/ -v

.PHONY: eval
eval: ## semantic eval: real intents → real LLM → scored (needs LLM_*/LIBRARY_*/TMDB_API_KEY; NOT in the hermetic gate)
	$(GO) test -tags=eval -v -timeout 20m ./internal/eval/

## ---- build / run ---------------------------------------------------------

.PHONY: build
build: ## build the loomarr binary (static, cgo-free — §16)
	CGO_ENABLED=0 $(GO) build -o $(BIN_DIR)/loomarr ./cmd/loomarr

.PHONY: dev
dev: ## dev compose stack (external deps: tunarr-dev; portable Mac/Linux, CPU transcode)
	docker compose -f docker/compose.dev.yaml up -d

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
dev-be: ## backend with live reload (Air) — rebuilds + restarts on any Go change
	@# Air is a dev tool, not a dependency (§14): run via `go run` so it is never added to
	@# go.mod and needs no manual install step. A committed .air.toml with no way to run it
	@# is how this box spent a session serving a stale binary.
	@#
	@# ⚠ SINGLE-INSTANCE GUARD (scripts/dev-be-guard.sh). Air itself has no "am I already
	@# running?" check, so a SECOND `make dev-be` used to start a second Air + binary that lost
	@# the :8080 bind and exited — while the stale one kept serving OLD code. That zombie cost
	@# DAYS of "my fix didn't take". The guard refuses to start a duplicate (or, with
	@# DEV_BE_REPLACE=1, cleanly replaces ONLY the loomarr dev binary — never a blanket kill).
	@sh scripts/dev-be-guard.sh
	@# ⚠ STALE-BINARY WATCHDOG (scripts/dev-be-watchdog.sh). Even with `.air.toml`'s
	@# stop_on_error=false + poll=true, Air can still end up ALIVE but not rebuilding (poll loop
	@# stalled) — serving a frozen binary while your saves do nothing. Config can't detect its own
	@# watcher dying; this out-of-band watchdog does. It runs beside Air, notices when the running
	@# binary stays older than the newest .go source, and self-heals (nudge Air, then restart the
	@# binary via Air's own path — never a competing process). Backgrounded here; the `trap` reaps
	@# it when Air exits so `make dev-be` leaves nothing behind. Opt out with DEV_BE_NO_WATCHDOG=1.
	@sh -c 'if [ "$${DEV_BE_NO_WATCHDOG:-0}" != "1" ]; then \
	    sh scripts/dev-be-watchdog.sh & wd=$$!; trap "kill $$wd 2>/dev/null" EXIT INT TERM; fi; \
	  exec $(GO) run github.com/air-verse/air@v1.67.3'

.PHONY: dev-gpu
dev-gpu: ## dev compose stack with NVIDIA transcode overlay (Linux + nvidia-container-toolkit)
	docker compose -f docker/compose.dev.yaml -f docker/compose.dev.gpu.yaml up -d

## ---- store conformance (Phase 3/4) --------------------------------------

.PHONY: test-pg
test-pg: ## store conformance vs Postgres (testcontainers; requires Docker)
	$(GO) test -race -tags=integration -run TestPostgresConformance ./internal/store/

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
	cd $(WEB) && pnpm biome check

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
	cd $(WEB) && pnpm biome check && pnpm codegen && pnpm -r --parallel typecheck \
	  && pnpm --filter '!@loomarr/web' -r --parallel test \
	  && pnpm --filter @loomarr/web test $(FE_SHARD_ARG) \
	  && pnpm --filter @loomarr/web build && pnpm --filter @loomarr/web build-storybook
	@touch internal/web/dist/.gitkeep

.PHONY: storybook
storybook: ## Storybook dev workshop (the component gallery/contract) on :6006
	cd $(WEB) && pnpm --filter @loomarr/web storybook

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
	docker run --rm --ipc=host $(PW_DOCKER_USER) -e CI=$(PW_CI) -v "$(PWD)/web:/work" -w /work/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test $(PW_SHARD)

.PHONY: fe-visual-update
fe-visual-update: storybook-build ## regenerate the committed Linux baselines in the Docker image (sanctioned update path)
	docker run --rm --ipc=host $(PW_DOCKER_USER) -e CI=$(PW_CI) -v "$(PWD)/web:/work" -w /work/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test --update-snapshots

# The e2e suite drives the REAL embedded SPA build, which Vite writes to
# internal/web/dist — OUTSIDE web/. So unlike fe-visual it mounts the repo ROOT, and
# runs from /work/web/apps/web (node_modules still resolves up to /work/web).
.PHONY: e2e
e2e: fe-build ## wizard e2e smoke vs a mocked backend, in the pinned Docker image (13.3 gate)
	docker run --rm --ipc=host $(PW_DOCKER_USER) -e CI=$(PW_CI) -v "$(PWD):/work" -w /work/web/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test --config=playwright.e2e.config.ts

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
	docker run --rm --ipc=host $(PW_DOCKER_USER) -e CI=$(PW_CI) -v "$(PWD):/work" -w /work/web/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test --config=playwright.e2e.config.ts --update-snapshots

# Just the SPA build the e2e suite serves (a subset of `make fe`, so the gate doesn't
# rebuild Storybook or re-run the unit suite to check a flow).
.PHONY: fe-build
fe-build:
	cd $(WEB) && pnpm codegen && pnpm --filter @loomarr/web build

.PHONY: seed
seed: ## populate a dev store via the real domain paths (approval gate honored — CLAUDE.md)
	DATABASE_URL=$${DATABASE_URL:-sqlite://./loomarr-dev.db} go run ./cmd/seed
