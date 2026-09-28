#!/usr/bin/env bash
# Classify changed repository paths into the smallest trustworthy CI gate set.
#
# Interface: pass paths as arguments, or one path per line on stdin. Pass --all
# when the caller cannot establish a trustworthy diff base. The command prints
# stable key=true|false records suitable for GitHub outputs. An unknown path
# fails closed by selecting every gate.
set -euo pipefail

readonly GATES=(
  contracts go go_full rust postgres web clients apple_mobile apple_tv
  expo_android_mobile expo_android_tv visual e2e tuner image docs agent android policy playout_bench
)

selected=()
strict=false
unknown=false
force_all=false
for ((i = 0; i < ${#GATES[@]}; i++)); do
  selected[i]=false
done

select_gate() {
  local gate="$1" i
  for ((i = 0; i < ${#GATES[@]}; i++)); do
    if [[ "${GATES[$i]}" == "$gate" ]]; then
      selected[i]=true
      return
    fi
  done
  printf 'ci-impact: internal error: unknown gate %q\n' "$gate" >&2
  exit 2
}

select_all() {
  local i
  for ((i = 0; i < ${#GATES[@]}; i++)); do
    selected[i]=true
  done
}

select_all_native_clients() {
  select_gate apple_mobile
  select_gate apple_tv
  select_gate expo_android_mobile
  select_gate expo_android_tv
}

classify() {
  local path="$1"
  local known=false

  # The public repo's real-titles guard (make demo-titles-verify, run by privacy-verify in the
  # Go contracts job) scans these seeds, fixtures and the demo catalogue. It adds the gate
  # without claiming the path, so each path's own classification below still applies. Keep in
  # step with isFixture and guarded in internal/demolibrary/guard_test.go; a test there fails
  # when a scanned path would skip this gate (#1587).
  case "$path" in
    cmd/seed/*|cmd/demo-library/*|internal/demolibrary/*|web/*.stories.tsx|web/apps/web/tests/e2e/mock-backend.ts|web/scripts/tv-emulator-fixture-server.mjs)
      select_gate contracts
      ;;
    web/packages/fixtures/src/*)
      [[ "$path" == *.test.* ]] || select_gate contracts
      ;;
  esac

  # Product Go. Release images compile and embed these source families.
  if [[ "$path" == cmd/releaseverify/*.go || "$path" == internal/releaseverify/* ]]; then
    known=true
    select_gate policy
  elif [[ "$path" == *.go || "$path" == go.mod || "$path" == go.sum ]]; then
    known=true
    select_gate contracts
    select_gate go
    # The first Postgres activation is deliberately conservative. The integration
    # target compiles store, backend-transition, and app tests plus their broad Go
    # dependency closure; until a dependency-aware Postgres selector is proven in
    # shadow, every Go source change retains this gate.
    select_gate postgres
    select_gate image
    case "$path" in
      cmd/loomarr/*|internal/app/*|internal/testkit/*|go.mod|go.sum)
        select_gate go_full
        ;;
    esac
  fi


  # The playout bench runs the real pipeline builder, so a change to the builder, the bench itself
  # or its corpus scripts selects it. Independent of the Go gates above: a builder edit still runs
  # those too.
  case "$path" in
    internal/playout/*|internal/playoutbench/*|cmd/playout-bench/*)
      select_gate playout_bench
      ;;
  esac
  case "$path" in
    scripts/playout-bench-*|docs/engineering/playout-bench/*)
      known=true
      select_gate playout_bench
      select_gate docs
      ;;
    docs/contributing/playout-bench.md)
      known=true
      select_gate docs
      ;;
    .github/workflows/ci-playout-bench.yml)
      known=true
      select_gate playout_bench
      select_gate policy
      ;;
    .github/workflows/playout-bench.yml)
      # Dispatch- and schedule-only; no pull request executes it, so only policy verifies it.
      known=true
      select_gate policy
      ;;
    Cargo.toml|Cargo.lock|rust-toolchain.toml|deny.toml|rust/*)
      known=true
      select_gate rust
      select_gate image
      ;;
    internal/store/migrations/*)
      known=true
      select_gate contracts
      select_gate go
      select_gate go_full
      select_gate postgres
      select_gate image
      ;;
    web/apps/mobile/*)
      known=true
      select_gate clients
      select_gate apple_mobile
      select_gate expo_android_mobile
      ;;
    web/apps/tv/*)
      known=true
      select_gate clients
      select_gate apple_tv
      select_gate expo_android_tv
      select_gate android
      ;;
    web/packages/lan-discovery-native/*)
      known=true
      select_gate clients
      select_gate apple_tv
      select_gate expo_android_tv
      select_gate android
      ;;
    web/packages/design-system/*|web/packages/ui/*|web/packages/ui-tv/*)
      known=true
      select_gate clients
      select_all_native_clients
      # Browser Storybook stories render these universal presentation packages
      # directly. Their output is therefore part of the committed visual contract.
      select_gate visual
      select_gate android
      ;;
    web/packages/fixtures/src/testcard/*)
      # Test-card filler/guide data is imported only by browser stories and tests. It
      # is not part of either native app graph, so fixture-only UI work must not spend
      # Apple or Android runners.
      known=true
      select_gate web
      select_gate visual
      select_gate image
      ;;
    web/packages/api/*|web/packages/core/*|web/packages/fixtures/*)
      known=true
      select_gate web
      select_gate clients
      select_all_native_clients
      select_gate visual
      select_gate e2e
      select_gate tuner
      select_gate image
      select_gate android
      ;;
    web/packages/player/*)
      known=true
      select_gate web
      select_gate clients
      select_all_native_clients
      # The browser and native adapters share one transport contract. Browser playback
      # reaches the tuner matrix and production image; the native adapters reach every
      # supported client build and Android TV bundle.
      select_gate tuner
      select_gate image
      select_gate android
      ;;
    web/package.json|web/pnpm-lock.yaml|web/pnpm-workspace.yaml|web/.gitignore|web/biome.json|web/tsconfig.base.json)
      known=true
      select_gate web
      select_gate clients
      select_all_native_clients
      select_gate image
      select_gate visual
      select_gate e2e
      select_gate tuner
      select_gate android
      ;;
    web/knip.ts)
      # knip runs inside `make fe` (the web gate) and nowhere else.
      known=true
      select_gate web
      ;;
    web/.dependency-cruiser.cjs|web/turbo.json|web/.rnstorybook/*|web/apps/web/client-platform-proof.html|web/apps/web/src/client-platform-proof/*|web/apps/web/tests/client-platform-proof.*|web/apps/web/vite.client-platform.config.ts)
      known=true
      select_gate clients
      ;;
    web/scripts/test-apple-client.sh|web/scripts/test-apple-client.test.mjs|web/scripts/test-apple-client-cache-test.sh|web/scripts/apple-simulator.xcconfig|web/scripts/apple-compilation-cache.xcconfig|web/scripts/validate-apple-compilation-cache.sh|web/scripts/validate-apple-compilation-cache-test.sh|web/scripts/filter-react-native-pods-notice.awk|web/scripts/filter-react-native-pods-notice.test.mjs)
      known=true
      select_gate contracts
      select_gate apple_mobile
      select_gate apple_tv
      ;;
    web/scripts/build-android-client.sh|web/scripts/with-memory-safe-android-build.cjs|web/scripts/with-memory-safe-android-build.test.cjs)
      known=true
      select_gate contracts
      select_gate expo_android_mobile
      select_gate expo_android_tv
      select_gate android
      ;;
    web/scripts/build-shield-play.sh|web/scripts/build-shield-play.test.sh|web/scripts/install-android-ccache.sh|web/scripts/install-android-ccache.test.sh|web/scripts/verify-android-ccache-evidence.sh)
      known=true
      select_gate contracts
      select_gate android
      ;;
    web/scripts/check-imports.mjs|web/scripts/check-imports.test.mjs)
      known=true
      select_gate clients
      ;;
    web/scripts/*)
      known=true
      select_gate contracts
      select_gate web
      select_gate clients
      select_all_native_clients
      select_gate image
      select_gate visual
      select_gate e2e
      select_gate tuner
      select_gate android
      ;;
    web/*)
      known=true
      select_gate web
      select_gate image
      case "$path" in
        web/apps/web/src/*.test.ts|web/apps/web/src/*.test.tsx|web/apps/web/src/*.spec.ts|web/apps/web/src/*.spec.tsx)
          ;;
        web/apps/web/src/*)
          # Storybook can reach any shipping runtime module through an alias import. Until a
          # committed, generator-verified dependency closure exists, runtime source is the safe
          # visual boundary; unit-test-only modules above are not part of that graph.
          select_gate visual
          ;;
      esac
      case "$path" in
        web/apps/web/src/*.test.ts|web/apps/web/src/*.test.tsx|web/apps/web/src/*.spec.ts|web/apps/web/src/*.spec.tsx|web/apps/web/src/*.stories.ts|web/apps/web/src/*.stories.tsx)
          ;;
        web/apps/web/src/*)
          # The tuner matrix imports the shipping SPA and exercises its real HLS controller.
          # Until a committed dependency closure proves a narrower boundary, every runtime
          # source can change that path; unit/spec/story-only modules are not shipped.
          select_gate tuner
          ;;
      esac
      case "$path" in
        web/packages/tokens/*|web/apps/web/public/*|web/apps/web/tests/visual/*|*.stories.ts|*.stories.tsx|*.snap|*.css)
          select_gate visual
          ;;
      esac
      case "$path" in
        web/packages/tokens/*)
          # Tokens feed the production stylesheet consumed by the real tuner SPA build.
          select_gate tuner
          ;;
      esac
      case "$path" in
        web/apps/web/tests/e2e/*|web/apps/web/tests/smoke/*|web/apps/web/src/auth/*|web/apps/web/src/routes/*|web/apps/web/src/wizard/*)
          select_gate e2e
          ;;
      esac
      case "$path" in
        web/apps/web/tests/e2e/tuner-*|web/apps/web/src/channels/channel-watch/*|web/apps/web/src/channels/guide-*|web/apps/web/src/channels/tuner-*|web/apps/web/src/channels/use-channel-tuner/*|web/apps/web/src/channels/use-hls-player/*)
          select_gate tuner
          ;;
      esac
      case "$path" in
        web/package.json|web/pnpm-lock.yaml|web/apps/web/package.json|web/apps/web/playwright*|web/apps/web/.storybook/*)
          select_gate visual
          select_gate e2e
          select_gate tuner
          ;;
        web/apps/web/components.json|web/apps/web/index.html|web/apps/web/tsconfig.json|web/apps/web/tsr.config.json|web/apps/web/vite.config.ts)
          select_gate visual
          select_gate e2e
          select_gate tuner
          ;;
      esac
      ;;
    api/openapi.yaml)
      known=true
      select_gate contracts
      select_gate go
      select_gate go_full
      select_gate web
      select_gate clients
      select_gate visual
      select_gate e2e
      select_gate tuner
      select_gate image
      ;;
    api/vendor/*)
      known=true
      select_gate contracts
      select_gate go
      ;;
    android/*)
      known=true
      select_gate android
      ;;
    store-listing/android-tv/*)
      known=true
      select_gate android
      ;;
    Dockerfile)
      known=true
      select_gate contracts
      select_gate go
      select_gate go_full
      select_gate image
      select_gate policy
      ;;
    LICENSE|THIRD_PARTY_NOTICES.md)
      known=true
      select_gate contracts
      select_gate image
      ;;
    observability/*)
      known=true
      select_gate contracts
      ;;
    .dockerignore)
      known=true
      select_gate image
      ;;
    docs/get-started.md|docs/guides/*|docs/explanation/*)
      known=true
      select_gate contracts
      select_gate go
      select_gate go_full
      select_gate image
      select_gate docs
      ;;
    docs/design.md|docs/reference/*|docs/contributing/ci.md|README.md)
      known=true
      select_gate docs
      select_gate policy
      ;;
    # The shared Claude settings run a session-start hook; agent-assets-verify guards what they hold.
    .claude/settings.json)
      known=true
      select_gate docs
      select_gate agent
      ;;
    docs/*|project/*|CHANGELOG.md|CODE_OF_CONDUCT.md|CONTRIBUTING.md|SECURITY.md|CLAUDE.md|AGENTS.md|CONTEXT.md|PROGRESS.md|docs-site/*|.agents/*|.claude/*|.vale|.vale/*|.vale.ini|lychee.toml|.markdownlint*|.github/CODEOWNERS|.github/ISSUE_TEMPLATE/*|.github/PULL_REQUEST_TEMPLATE.md)
      known=true
      select_gate docs
      ;;
    .graphifyignore|graphify-out/*)
      # The committed graph is developer documentation and query data. It cannot
      # affect a product binary, native client, database, or runtime contract.
      known=true
      select_gate docs
      ;;
    design/*)
      known=true
      select_gate docs
      ;;
    spike/*)
      # Throwaway measurement scripts and results behind project/SPIKE-*.md. Nothing builds,
      # ships or imports them (a spike's Go code is its own module, outside ./...).
      known=true
      select_gate docs
      ;;
	internal/testkit/postgresimage/image.go|internal/testkit/postgresimage/image.txt)
		known=true
		select_gate contracts
		select_gate go
		select_gate go_full
		select_gate postgres
		select_gate image
		select_gate policy
		;;
	internal/testkit/ryukimage/image.txt)
		known=true
		select_gate postgres
      select_gate policy
      ;;
    internal/testkit/fixtures/*)
      known=true
      select_gate go
      select_gate go_full
      ;;
    internal/fillereval/corpus/*)
      known=true
      select_gate contracts
      select_gate go
      ;;
    internal/eval/testdata/*)
      known=true
      select_gate contracts
      select_gate go
      ;;
    internal/recommend/testdata/*)
      known=true
      select_gate contracts
      select_gate go
      ;;
    internal/plannerreference/testdata/*)
      known=true
      select_gate contracts
      select_gate go
      ;;
    internal/fillercorpus/corpus/*)
      known=true
      select_gate contracts
      select_gate go
      ;;
    internal/installationlocation/*.gz|internal/installationlocation/*.json)
      # Generated runtime indexes are embedded by data.go. They can change location
      # behavior and the production image, but they are not inputs to any native client.
      known=true
      select_gate contracts
      select_gate go
      select_gate image
      ;;
    internal/watermark/fonts/*)
      # The Plate bug's typeface and its licence, embedded by render.go. They change the rendered
      # watermark and the production image, but no native client reads them.
      known=true
      select_gate contracts
      select_gate go
      select_gate image
      ;;
    internal/playout/*.cl)
      # OpenCL kernels embedded by the pipeline builder (watermark.go) and run by ffmpeg's
      # program_opencl in the playout graph. The playout bench is selected above.
      known=true
      select_gate contracts
      select_gate go
      select_gate image
      ;;
    internal/fillereval/*.md)
      known=true
      select_gate docs
      ;;
    internal/releaseverify/testdata/*)
      # Release verification is policy-only above; its golden inputs must retain that boundary
      # instead of falling through to the generic Go-package fixture rule.
      known=true
      select_gate policy
      ;;
    internal/*/testdata/*)
      # Go package fixtures belong to their owning package's tests. Treating a new fixture
      # directory as unknown used to fan ordinary backend work out to every Apple/Android and
      # browser gate even though no native client consumes the file.
      known=true
      select_gate contracts
      select_gate go
      ;;
    internal/web/dist/*)
      known=true
      select_gate contracts
      select_gate go
      select_gate go_full
      select_gate image
      select_gate web
      ;;
    brand-assets.lock.json)
      known=true
      select_gate clients
      select_gate android
      ;;
    scripts/*)
      known=true
      case "$path" in
        scripts/agent*|scripts/changed-paths*|scripts/go-direct-impact*|scripts/test-affected*) select_gate agent; select_gate policy ;;
        scripts/apple-compilation-cache*) select_gate contracts; select_gate apple_mobile; select_gate apple_tv; select_gate policy ;;
        scripts/ci-ffmpeg.sh) select_gate contracts; select_gate go; select_gate go_full; select_gate policy ;;
        scripts/go-certification-lanes.tsv|scripts/go-race-policy.sh|scripts/go-race-weights.tsv|scripts/go-shard.sh|scripts/go-test-lane.sh|scripts/go-test-packages.sh) select_gate contracts; select_gate go; select_gate go_full; select_gate policy ;;
        scripts/ensure-container-image.sh) select_gate contracts; select_gate postgres; select_gate visual; select_gate e2e; select_gate tuner; select_gate policy ;;
        scripts/tuner-args*) select_gate tuner; select_gate policy ;;
        scripts/run-playwright-container.sh) select_gate contracts; select_gate visual; select_gate e2e; select_gate tuner; select_gate policy ;;
        # Selection policy is control-plane code. Product gates validate files they consume;
        # the always-on policy job independently validates this classifier and its fixtures.
        scripts/ci-impact.sh) select_gate policy ;;
        scripts/ci-impact*|scripts/ci-diff-base*|scripts/ci-dispatch-scope*|scripts/ci-run-metrics*|scripts/ci-merge-queue-policy*|scripts/testdata/ci-*) select_gate policy ;;
        scripts/dev-*) select_gate contracts; select_gate agent ;;
        # Local-only dev watchers; the agent gate runs their tests and shellcheck.
        scripts/dev/*) select_gate agent ;;
        # Release admission and artifact download inspect existing evidence; changing them does
        # not change the bundle. The signer is exercised by the Android build and remains an input.
        scripts/validate-android-release-source*|scripts/download-android-ci-artifact.sh) select_gate policy ;;
		scripts/android-ccache-promotion*) select_gate android; select_gate policy ;;
        scripts/sign-android-ci-artifact.sh) select_gate android; select_gate policy ;;
        scripts/android-*.sh|scripts/build-android-beta.sh|scripts/check-android-release-env.sh|scripts/generate-android-tv-brand.sh|scripts/publish-android-beta.sh|scripts/test-android-release.sh|scripts/test-android-release-emulator.sh) select_gate android ;;
        scripts/generate-brand-assets.mjs|scripts/check-brand-assets.mjs) select_gate clients; select_gate android ;;
        scripts/check-fe-bundle.mjs) select_gate web; select_gate image ;;
        # Repository contracts, release tooling, and operator helpers the contracts job lints or
        # invokes. Each is named: a new script is not contracts-only until someone says so.
        scripts/center-android-tv-emulator.js|scripts/check-agent-assets.sh|scripts/check-private-fixtures.sh|scripts/check-release-tag.sh|scripts/check-tags.sh) select_gate contracts ;;
        scripts/ci-lane-test.sh|scripts/ci-lane.sh|scripts/codeql-impact-test.sh|scripts/codeql-impact.sh|scripts/deadcode.sh|scripts/generate-release-notes.sh|scripts/go-impact-test.sh) select_gate contracts ;;
        scripts/image-parallelism-bench.sh|scripts/latency-sweep.sh|scripts/playout-diag.sh|scripts/run-android-tv-emulator.sh|scripts/test-android-release-emulator-contract-test.sh) select_gate contracts ;;
        scripts/validate-release-source-test.sh|scripts/validate-release-source.sh) select_gate contracts ;;
        # ⚠ One pattern per line below, never before a `|`. release-verify's container audit reads
        # this file as shell, so a script name followed by `|` looks like a pipeline command. If the
        # audit counts that script as acquiring containers (an engine call, or a variable
        # executable such as "$gh_bin"), this classifier, run by the agent harness, would count too.
        # Scripts with container tooling are listed here conservatively.
        scripts/go-impact.sh) select_gate contracts ;;
        scripts/go-race-weights-refresh.sh) select_gate contracts ;;
        scripts/check-compose.sh) select_gate contracts ;;
        scripts/check-release-image-absence.sh) select_gate contracts ;;
        scripts/check-retired.sh) select_gate contracts ;;
        # The docs gate's inline-path check (ci-docs.yml, `make docs-lint-paths`) and its allowlist.
        scripts/check-doc-paths*) select_gate docs ;;
        # The docs screenshot capture (`make docs-capture`) is a manual operator helper; no gate
        # runs it, and its output is committed images the docs gates already check.
        scripts/docs-capture/*) select_gate contracts ;;
        scripts/generate-diagrams.sh) select_gate contracts ;;
        scripts/merge-release-digests.sh) select_gate contracts ;;
        scripts/observability-dev-runtime-test.sh) select_gate contracts ;;
        scripts/observability-dev-test.sh) select_gate contracts ;;
        scripts/observability-dev.sh) select_gate contracts ;;
        scripts/publish-release-image.sh) select_gate contracts ;;
        scripts/smoke.sh) select_gate contracts ;;
        scripts/verify-observability.sh) select_gate contracts ;;
        scripts/watermark-overlay-matrix.sh) select_gate contracts ;;
        # Fail closed: a script no rule names may be invoked by any gate, so it selects every
        # gate until it is classified here (#1570).
        *) known=false ;;
      esac
      ;;
    # orca.yaml is Orca's adapter over scripts/agent.sh, like the Make module that wraps it.
    mk/agent.mk|orca.yaml)
      known=true
      select_gate agent
      select_gate policy
      ;;
    mk/check.mk)
      known=true
      select_gate contracts
      select_gate go
      select_gate go_full
      select_gate rust
      select_gate postgres
      select_gate policy
      ;;
    mk/eval.mk)
      known=true
      select_gate contracts
      select_gate go
      select_gate policy
      ;;
    mk/build.mk)
      known=true
      select_gate contracts
      select_gate go
      select_gate rust
      select_gate image
      select_gate agent
      select_gate policy
      ;;
    mk/store.mk)
      known=true
      select_gate contracts
      select_gate go
      select_gate postgres
      select_gate policy
      ;;
    # Cache warming is not a gate: it compiles test binaries on main pushes and runs none. The
    # policy job verifies its workflow registration and that no gate can reach compile-only mode.
    mk/cache-warm.mk)
      known=true
      select_gate policy
      ;;
    mk/contracts.mk)
      known=true
      select_gate contracts
      select_gate go
      select_gate docs
      select_gate policy
      ;;
    mk/docs.mk)
      known=true
      select_gate docs
      select_gate policy
      ;;
    mk/frontend.mk)
      known=true
      select_gate web
      select_gate clients
      select_all_native_clients
      select_gate visual
      select_gate e2e
      select_gate tuner
      select_gate image
      select_gate android
      select_gate policy
      ;;
    mk/smoke.mk)
      known=true
      select_gate contracts
      select_gate policy
      ;;
    mk/android.mk)
      known=true
      select_gate android
      select_gate policy
      ;;
    Makefile)
      known=true
      select_all
      ;;
    .github/workflows/ci.yml)
      known=true
      select_gate policy
      ;;
    .github/workflows/ci-agent.yml)
      known=true
      select_gate agent
      select_gate policy
      ;;
    .github/workflows/ci-rust-contracts.yml|.github/workflows/ci-image-certification.yml)
      known=true
      select_gate rust
      select_gate policy
      ;;
    .github/workflows/ci-go-contracts.yml)
      known=true
      select_gate contracts
      select_gate policy
      ;;
    .github/workflows/ci-go.yml)
      known=true
      select_gate go
      select_gate go_full
      select_gate policy
      ;;
    .github/workflows/ci-postgres.yml)
      known=true
      select_gate postgres
      select_gate policy
      ;;
    .github/workflows/ci-frontend.yml)
      known=true
      select_gate web
      select_gate policy
      ;;
    .github/workflows/ci-clients.yml)
      known=true
      select_gate clients
      select_gate policy
      ;;
    .github/workflows/ci-apple-mobile.yml)
      known=true
      select_gate apple_mobile
      select_gate policy
      ;;
    .github/workflows/ci-expo-android-mobile.yml)
      known=true
      select_gate expo_android_mobile
      select_gate policy
      ;;
    .github/workflows/ci-apple-tv.yml)
      known=true
      select_gate apple_tv
      select_gate policy
      ;;
    .github/workflows/ci-apple-cache-validation.yml)
      known=true
      select_gate apple_mobile
      select_gate apple_tv
      select_gate policy
      ;;
    .github/workflows/apple-compilation-cache.yml)
      known=true
      select_gate apple_mobile
      select_gate apple_tv
      select_gate policy
      ;;
    .github/workflows/ci-playwright.yml)
      known=true
      select_gate visual
      select_gate e2e
      select_gate policy
      ;;
    .github/workflows/ci-tuner.yml)
      known=true
      select_gate tuner
      select_gate policy
      ;;
    .github/workflows/ci-image.yml)
      known=true
      select_gate image
      select_gate policy
      ;;
    .github/workflows/ci-docs.yml)
      known=true
      select_gate docs
      select_gate policy
      ;;
    .github/workflows/ci-android.yml)
      known=true
      select_gate android
      select_gate policy
      ;;
    # Workflows whose product checks run elsewhere (release, maintenance, reporting, cache
    # housekeeping). There is no .github/workflows/* catch-all: a workflow no rule names is
    # unknown and selects every gate until it is classified (#1570).
    .github/workflows/android-beta.yml|.github/workflows/android-ccache-promotion.yml|.github/workflows/cache-cleanup.yml|.github/workflows/ci-go-cache-warm.yml|.github/workflows/codeql.yml|.github/workflows/deadcode.yml|.github/workflows/image-benchmark.yml|.github/workflows/pages.yml|.github/workflows/release-notes.yml|.github/workflows/release.yml|.github/workflows/rust-maintenance.yml)
      known=true
      select_gate policy
      ;;
    renovate.json|.github/dependabot.yml) # retired-ok: deleting the legacy config must still select policy gates
	  known=true
	  select_gate contracts
	  select_gate policy
	  ;;
    docker/*|.air.toml|.env.example|.golangci.yml|.node-version|.editorconfig|.gitignore|.vscode/*|.github/actionlint.yaml|.github/actionlint.yml|skills-lock.json)
      known=true
      select_gate contracts
      case "$path" in
        docker/compose.dev*|.node-version) select_gate agent ;;
        docker/compose.yaml) select_gate go; select_gate go_full ;;
      esac
      ;;
  esac

  if [[ "$known" == false ]]; then
    printf 'ci-impact: unknown path %q; selecting every gate\n' "$path" >&2
    unknown=true
    select_all
  fi
}

while (($#)); do
  case "$1" in
    --check-known)
      strict=true
      shift
      ;;
    --all)
      force_all=true
      shift
      ;;
    --)
      shift
      break
      ;;
    *) break ;;
  esac
done

if [[ "$force_all" == true ]]; then
  if (($#)); then
    printf 'ci-impact: --all does not accept paths\n' >&2
    exit 2
  fi
  select_all
elif (($#)); then
  for path in "$@"; do
    [[ -n "$path" ]] && classify "$path"
  done
else
  while IFS= read -r path; do
    [[ -n "$path" ]] && classify "$path"
  done
fi

if [[ "$strict" == true && "$unknown" == true ]]; then
  exit 1
fi

for ((i = 0; i < ${#GATES[@]}; i++)); do
  printf '%s=%s\n' "${GATES[$i]}" "${selected[$i]}"
done
