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

.PHONY: eval
eval: ## semantic eval: real intents → real LLM → scored (needs LLM_*/LIBRARY_*/TMDB_API_KEY; NOT in the hermetic gate)
	$(GO) test -tags=eval -v -timeout 20m ./internal/eval/

## ---- build / run ---------------------------------------------------------

.PHONY: build
build: ## build the loomarr binary (static, cgo-free — §16)
	CGO_ENABLED=0 $(GO) build -o $(BIN_DIR)/loomarr ./cmd/loomarr

.PHONY: dev
dev: ## dev compose stack (external deps: tunarr-dev wired to Emby)
	docker compose -f docker/compose.dev.yaml up -d

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

## ---- config docs (settings registry) ------------------------------------

.PHONY: config-docs
config-docs: ## generate docs/configuration.md from the settings registry
	$(GO) run ./cmd/config-docs docs/configuration.md

.PHONY: config-docs-verify
config-docs-verify: config-docs ## regenerated config docs must match committed (CI red on drift)
	@git diff --exit-code docs/configuration.md

## ---- frontend (Phase 13) -------------------------------------------------

.PHONY: fe
fe: ## orval typegen + tsc + vitest
	@echo "fe: implemented in Phase 13"; exit 1

.PHONY: e2e
e2e: ## Playwright smoke vs mocked backend
	@echo "e2e: implemented in Phase 13"; exit 1

.PHONY: seed
seed: ## populate a dev store via the testkit (admin path only — CLAUDE.md)
	@echo "seed: implemented alongside the store/API"; exit 1
