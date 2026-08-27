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

.PHONY: brand-assets
brand-assets: ## regenerate favicon, launcher, TV, and store artwork from the shared brand contract
	node scripts/generate-brand-assets.mjs

.PHONY: brand-assets-verify
brand-assets-verify: ## verify every platform brand derivative matches the shared brand contract
	node --check scripts/generate-brand-assets.mjs
	node scripts/check-brand-assets.mjs

.PHONY: fe-codegen
fe-codegen: ## regenerate tokens + orval api client from api/openapi.yaml
	cd $(WEB) && pnpm codegen

.PHONY: fe-api-codegen
fe-api-codegen: ## regenerate only the orval api client from api/openapi.yaml
	cd $(WEB) && pnpm api

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
clients: brand-assets-verify ## lint, test, typecheck, and bundle the shared browser, mobile, and TV scaffold
	cd $(WEB) && pnpm exec biome check apps/mobile apps/tv apps/web/client-platform-proof.html \
	  apps/web/src/client-platform-proof apps/web/tests/client-platform-proof.ssr.test.tsx \
	  apps/web/vite.client-platform.config.ts \
	  .rnstorybook native-stories packages/design-system packages/ui turbo.json \
	  && pnpm imports:check && pnpm lint:boundaries && pnpm native-storybook:check && pnpm clients:check

CLIENT_APP ?= mobile
.PHONY: client-android-debug
client-android-debug: fe-api-codegen ## memory-bounded arm64 debug build (CLIENT_APP=mobile|tv)
	cd $(WEB) && ./scripts/build-android-client.sh $(CLIENT_APP)

.PHONY: client-apple-simulator
client-apple-simulator: fe-api-codegen ## build and launch an Apple simulator proof (CLIENT_APP=mobile|tv; macOS)
	cd $(WEB) && ./scripts/test-apple-client.sh $(CLIENT_APP)

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

.PHONY: e2e-update
e2e-update: fe-build ## regenerate the committed e2e page snapshots (sanctioned update path)
	docker run --rm --ipc=host $(PW_DOCKER_USER) -e CI=$(PW_CI) -e GITHUB_ACTIONS=$(PW_REAL_CI) -v "$(PWD):/work" -w /work/web/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test --config=playwright.e2e.config.ts --update-snapshots

# Just the SPA build the e2e suite serves (a subset of `make fe`, so the gate doesn't
# rebuild Storybook or re-run the unit suite to check a flow).
.PHONY: fe-build
fe-build:
	cd $(WEB) && pnpm codegen && pnpm --filter @loomarr/web build
