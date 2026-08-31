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
