# Loomarr harness contract. Targets are created as their phase needs them;
# unimplemented ones fail loudly rather than pretend to pass.
#
# ⚠ DO NOT restate the target list or the CI set in prose — here or anywhere else.
# This header used to end "CI mirrors: check + openapi-verify + test-pg + fe + e2e",
# which omitted six gates CI actually runs, and four other files carried their own
# disagreeing copies. `docs/dev/commands.md` is GENERATED from the `## ` comments in this
# interface and its ordered `mk/*.mk` modules plus the `make` invocations in .github/workflows;
# `make dev-docs-verify` fails the
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

# CI-only shard passthrough for `make test` / comprehensive verification (e.g. GO_SHARD=1/2). Empty by
# default — see the note on the `test` target. The visual suite's separate environment-only
# shard input is documented in mk/frontend.mk. Never set this in a local gate run.
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
# (`ffmpeg`), a real LLM (`eval`) or Docker (`integration`); comprehensive verification stays hermetic
# (§19). The gate is only that they still COMPILE — which is free: measured 0.4s warm, and
# 3.2s for a never-before-seen tag set, because tags only recompile packages whose file
# selection actually changed.
#
# Platform constraints are retained in the inventory so a new platform-specific source file
# cannot masquerade as a custom-tagged one. They are not included in TAGS_CSV: the supported
# release artifact is Linux, and an unsupported GOOS is not a CI assurance target.
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


# Ordered ownership modules. cmd/dev-docs follows this list as the public command surface.
include mk/agent.mk
include mk/check.mk
include mk/eval.mk
include mk/build.mk
include mk/store.mk
include mk/contracts.mk
include mk/docs.mk
include mk/frontend.mk
include mk/smoke.mk
include mk/android.mk

.PHONY: prototype-spoken-safety-cascade
prototype-spoken-safety-cascade: ## PROTOTYPE: run the private native-audio spoken-safety pilot
	@set -a; \
	if [ -f ../loomarr/.env ]; then . ../loomarr/.env; fi; \
	set +a; \
	$(GO) run ./cmd/prototype-spoken-safety-cascade \
		--auto \
		--root "$(abspath ../../LoomarrData/filler-development-2026-08-30)" \
		$(PROTOTYPE_ARGS)
