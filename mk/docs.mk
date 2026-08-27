## ---- documentation lint --------------------------------------------------

# Structure, links, prose, and generated diagrams. Nothing needs a global install:
# markdownlint comes from npx, and the remaining tools run from pinned Docker images — the
# same approach as PW_IMAGE, and the reason a contributor can run the doc gate without a
# toolchain of its own.
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
D2_IMAGE     ?= terrastruct/d2:v0.7.1@sha256:810f69aff66636a75ab0b14afaca7d9055698ebe3a54274b477b351dffef9b58
DOCKER_DOC   := docker run --rm -v "$(CURDIR)":/work -w /work

.PHONY: diagrams
diagrams: ## format D2 sources and regenerate the committed SVG diagrams
	@D2_IMAGE="$(D2_IMAGE)" ./scripts/generate-diagrams.sh

.PHONY: diagrams-verify
diagrams-verify: diagrams ## regenerated D2 sources and SVG diagrams must match committed
	@git diff --exit-code -- docs/diagrams

.PHONY: docs-lint
docs-lint: diagrams-verify docs-lint-md docs-lint-links docs-lint-prose ## D2 + markdownlint + lychee (offline) + Vale

.PHONY: docs-lint-md
docs-lint-md: ## markdown structure (globs + rules live in .markdownlint-cli2.jsonc)
	npx --yes markdownlint-cli2@0.20.0

.PHONY: docs-lint-links
docs-lint-links: ## relative-link check, offline, over the WIDE set (see lychee.toml for why)
	$(DOCKER_DOC) $(LYCHEE_IMAGE) --offline --no-progress $(LINK_GLOBS)

.PHONY: docs-lint-prose
docs-lint-prose: ## repo vocabulary + proper-noun casing (.vale.ini — no stock style package)
	$(DOCKER_DOC) $(VALE_IMAGE) $(DOC_GLOBS)

