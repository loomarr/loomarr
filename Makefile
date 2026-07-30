# Loomarr harness contract (CLAUDE.md "Commands"). Targets are created as their
# phase needs them; unimplemented ones fail loudly rather than pretend to pass.
# CI mirrors: check + openapi-verify + test-pg + fe + e2e.

GO      ?= go
PKG     := ./...
BIN_DIR := bin

.DEFAULT_GOAL := help

.PHONY: help
help: ## List targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## ---- the default gate ----------------------------------------------------

.PHONY: check
check: fmt vet lint test ## fmt + vet + lint + unit tests (the default gate)

.PHONY: fmt
fmt: ## gofmt -l (fails if any file needs formatting)
	@out=$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*')); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

.PHONY: vet
vet: ## go vet
	$(GO) vet $(PKG)

.PHONY: lint
lint: ## golangci-lint v2 (run via `go run` so no global install needed)
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run

.PHONY: test
test: ## unit tests only (never touch the network — §19)
	$(GO) test -race $(PKG)

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
	$(GO) run github.com/air-verse/air@v1.67.3

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

.PHONY: fe
fe: ## biome + codegen + typecheck + unit tests + embedded SPA + storybook gallery
	cd $(WEB) && pnpm biome check && pnpm codegen && pnpm -r --parallel typecheck && pnpm -r --parallel test && pnpm --filter @loomarr/web build && pnpm --filter @loomarr/web build-storybook
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
# ⚠ `docker run` inherits NOTHING from the host environment, so `-e CI` (bare, forwarding the
# host value only when set) is what lets the container know it is in CI. Without it
# `process.env.CI` was undefined inside the image: the worker count fell back to Playwright's
# half-the-cores default (2 on a 4-core runner, for 504 tests), AND `forbidOnly` never applied
# — so a stray `test.only` would have silently narrowed the suite in CI while passing.
PW_IMAGE := mcr.microsoft.com/playwright:v1.61.1-noble

.PHONY: fe-visual
fe-visual: storybook-build ## Playwright visual + a11y over storybook-static, in the pinned Docker image (§5.2)
	docker run --rm --ipc=host -e CI -v "$(PWD)/web:/work" -w /work/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test

.PHONY: fe-visual-update
fe-visual-update: storybook-build ## regenerate the committed Linux baselines in the Docker image (sanctioned update path)
	docker run --rm --ipc=host -e CI -v "$(PWD)/web:/work" -w /work/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test --update-snapshots

# The e2e suite drives the REAL embedded SPA build, which Vite writes to
# internal/web/dist — OUTSIDE web/. So unlike fe-visual it mounts the repo ROOT, and
# runs from /work/web/apps/web (node_modules still resolves up to /work/web).
.PHONY: e2e
e2e: fe-build ## wizard e2e smoke vs a mocked backend, in the pinned Docker image (13.3 gate)
	docker run --rm --ipc=host -e CI -v "$(PWD):/work" -w /work/web/apps/web $(PW_IMAGE) \
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
	docker run --rm --ipc=host -e CI -v "$(PWD):/work" -w /work/web/apps/web $(PW_IMAGE) \
		node_modules/.bin/playwright test --config=playwright.e2e.config.ts --update-snapshots

# Just the SPA build the e2e suite serves (a subset of `make fe`, so the gate doesn't
# rebuild Storybook or re-run the unit suite to check a flow).
.PHONY: fe-build
fe-build:
	cd $(WEB) && pnpm codegen && pnpm --filter @loomarr/web build

.PHONY: seed
seed: ## populate a dev store via the real domain paths (approval gate honored — CLAUDE.md)
	DATABASE_URL=$${DATABASE_URL:-sqlite://./loomarr-dev.db} go run ./cmd/seed
