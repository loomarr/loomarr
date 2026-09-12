#!/usr/bin/env bash
set -euo pipefail

SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUNNER="$SOURCE_ROOT/scripts/test-affected.sh"
TEST_REPO="$(mktemp -d)"
TEST_BIN="$(mktemp -d)"
RUN_LOG="$TEST_BIN/runs.log"
trap 'rm -rf "$TEST_REPO" "$TEST_BIN"' EXIT

git -C "$TEST_REPO" init -q
git -C "$TEST_REPO" config user.email test@example.invalid
git -C "$TEST_REPO" config user.name 'Affected Test Runner'
mkdir -p "$TEST_REPO/internal/leaf" "$TEST_REPO/internal/consumer" \
	"$TEST_REPO/web/apps/web/src" "$TEST_REPO/web/apps/mobile/tests" \
	"$TEST_REPO/web/apps/tv/tests" "$TEST_REPO/web/packages/core/src" \
	"$TEST_REPO/rust/fixture/src" "$TEST_REPO/scripts"
printf 'module example.test/affected\n\ngo 1.27\n' >"$TEST_REPO/go.mod"
printf 'package leaf\n' >"$TEST_REPO/internal/leaf/leaf.go"
printf 'package leaf\n' >"$TEST_REPO/internal/leaf/leaf_test.go"
printf 'package consumer\n\nimport _ "example.test/affected/internal/leaf"\n' >"$TEST_REPO/internal/consumer/consumer.go"
printf 'export const value = 1;\n' >"$TEST_REPO/web/apps/web/src/value.ts"
printf 'import "../src";\n' >"$TEST_REPO/web/apps/mobile/tests/app.test.mjs"
printf 'import "../src";\n' >"$TEST_REPO/web/apps/tv/tests/app.test.mjs"
printf 'export const shared = 1;\n' >"$TEST_REPO/web/packages/core/src/shared.ts"
printf 'pub const VALUE: u8 = 1;\n' >"$TEST_REPO/rust/fixture/src/lib.rs"
printf '#!/usr/bin/env bash\n' >"$TEST_REPO/scripts/tool.sh"
cat >"$TEST_REPO/scripts/tool-test.sh" <<'EOF'
#!/usr/bin/env bash
printf 'owned script test\n' >>"$RUN_LOG"
EOF
chmod +x "$TEST_REPO/scripts/tool.sh" "$TEST_REPO/scripts/tool-test.sh"
git -C "$TEST_REPO" add .
git -C "$TEST_REPO" commit -qm base

REAL_GO="$(command -v go)"
cat >"$TEST_BIN/go" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == test ]]; then
	printf 'go %s\n' "$*" >>"$RUN_LOG"
	for arg in "$@"; do
		if [[ "$arg" == example.test/affected/* ]]; then
			printf 'ok  %s  35.125s\n' "$arg"
		fi
	done
	exit 0
fi
exec "$REAL_GO" "$@"
EOF
cat >"$TEST_BIN/pnpm" <<'EOF'
#!/usr/bin/env bash
printf 'pnpm %s\n' "$*" >>"$RUN_LOG"
EOF
cat >"$TEST_BIN/cargo" <<'EOF'
#!/usr/bin/env bash
printf 'cargo %s\n' "$*" >>"$RUN_LOG"
EOF
chmod +x "$TEST_BIN/go" "$TEST_BIN/pnpm" "$TEST_BIN/cargo"

printf 'package leaf\n\nconst Changed = true\n' >"$TEST_REPO/internal/leaf/leaf.go"
printf 'export const value = 2;\n' >"$TEST_REPO/web/apps/web/src/value.ts"
printf 'export const shared = 2;\n' >"$TEST_REPO/web/packages/core/src/shared.ts"
printf 'pub const VALUE: u8 = 2;\n' >"$TEST_REPO/rust/fixture/src/lib.rs"
printf '#!/usr/bin/env bash\n# changed\n' >"$TEST_REPO/scripts/tool.sh"
output="$(PATH="$TEST_BIN:$PATH" REAL_GO="$REAL_GO" RUN_LOG="$RUN_LOG" \
	LOOMARR_REPO_ROOT="$TEST_REPO" "$RUNNER" HEAD 2>&1)"
grep -q 'development feedback only' <<<"$output"
grep -q 'make verify BASE=HEAD before handoff' <<<"$output"
grep -q 'timing go:example.test/affected/internal/leaf 35.125s' <<<"$output"
grep -q 'slow owner go:example.test/affected/internal/leaf 35.125s (advisory threshold 30s)' <<<"$output"
grep -q 'advisory slow owners: go:example.test/affected/internal/leaf' <<<"$output"
grep -q 'timing total ' <<<"$output"
grep -q 'go test -race .*example.test/affected/internal/leaf' "$RUN_LOG"
if grep -q 'internal/consumer' "$RUN_LOG"; then
	echo 'test-affected-test: direct edit loop selected a reverse dependant' >&2
	exit 1
fi
grep -q 'pnpm .*vitest related.*web/apps/web/src/value.ts' "$RUN_LOG"
grep -q 'pnpm --filter @loomarr/mobile test' "$RUN_LOG"
grep -q 'pnpm --filter @loomarr/tv test' "$RUN_LOG"
grep -q 'cargo test --workspace --all-features --locked' "$RUN_LOG"
grep -q 'owned script test' "$RUN_LOG"
if grep -qE 'rust-test-worker|eval-contract|golangci|lint|playwright' "$RUN_LOG"; then
	echo 'test-affected-test: development loop ran publication evidence' >&2
	exit 1
fi

git -C "$TEST_REPO" reset --hard -q HEAD
: >"$RUN_LOG"
clean_output="$(PATH="$TEST_BIN:$PATH" REAL_GO="$REAL_GO" RUN_LOG="$RUN_LOG" \
	LOOMARR_REPO_ROOT="$TEST_REPO" "$RUNNER" HEAD)"
grep -q 'no changes' <<<"$clean_output"
[[ ! -s "$RUN_LOG" ]] || {
	echo 'test-affected-test: clean tree executed tests' >&2
	exit 1
}

printf 'package leaf\n\nconst Staged = true\n' >"$TEST_REPO/internal/leaf/leaf.go"
git -C "$TEST_REPO" add internal/leaf/leaf.go
: >"$RUN_LOG"
staged_output="$(PATH="$TEST_BIN:$PATH" REAL_GO="$REAL_GO" RUN_LOG="$RUN_LOG" \
	LOOMARR_REPO_ROOT="$TEST_REPO" "$RUNNER" HEAD 2>&1)"
grep -q 'internal/leaf/leaf.go' <<<"$staged_output"
grep -q 'go test -race .*example.test/affected/internal/leaf' "$RUN_LOG"

if PATH="$TEST_BIN:$PATH" REAL_GO="$REAL_GO" RUN_LOG="$RUN_LOG" \
	LOOMARR_REPO_ROOT="$TEST_REPO" AFFECTED_TEST_WARN_SECONDS=fast "$RUNNER" HEAD >/dev/null 2>&1; then
	echo 'test-affected-test: invalid advisory threshold was accepted' >&2
	exit 1
fi

echo 'test-affected-test: ok'
