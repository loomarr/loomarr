#!/usr/bin/env bash
# Fast, non-authoritative unit feedback for the edit loop. Publication evidence remains make verify.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="${LOOMARR_REPO_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"
BASE="${1:-${BASE:-origin/main}}"

changed="$("$SCRIPT_DIR/changed-paths.sh" "$BASE")"
if [[ -z "$changed" ]]; then
	echo 'test-affected: no changes'
	exit 0
fi

echo 'test-affected: development feedback only'
echo "test-affected: run make verify BASE=$BASE before handoff"
echo 'test-affected: changed files:'
printf '%s\n' "$changed"

ran=()
go_packages="$(printf '%s\n' "$changed" | "$SCRIPT_DIR/go-direct-impact.sh")"
if [[ -n "$go_packages" ]]; then
	package_count="$(printf '%s\n' "$go_packages" | wc -l | tr -d ' ')"
	echo "test-affected: direct Go packages: $package_count"
	traced="$(printf '%s\n' "$go_packages" | "$SCRIPT_DIR/go-race-policy.sh" --race)"
	untraced="$(printf '%s\n' "$go_packages" | "$SCRIPT_DIR/go-race-policy.sh" --no-race)"
	if [[ -n "$traced" ]]; then
		# shellcheck disable=SC2086 # newline-delimited package list intentionally becomes argv.
		(cd "$ROOT" && go test -race -timeout 25m $traced)
	fi
	if [[ -n "$untraced" ]]; then
		# shellcheck disable=SC2086 # newline-delimited package list intentionally becomes argv.
		(cd "$ROOT" && go test -timeout 25m $untraced)
	fi
	ran+=(go)
fi

web_paths=()
script_tests=()
while IFS= read -r path; do
	case "$path" in
		web/apps/web/* | web/packages/*) web_paths+=("$ROOT/$path") ;;
	esac
	case "$path" in
		scripts/agent.sh | scripts/agent-harness-test.sh)
			script_tests+=(scripts/agent-harness-test.sh)
			;;
		scripts/*-test.sh)
			script_tests+=("$path")
			;;
		scripts/*.sh)
			candidate="${path%.sh}-test.sh"
			[[ -f "$ROOT/$candidate" ]] && script_tests+=("$candidate")
			;;
		web/scripts/*.test.mjs | web/scripts/*.test.cjs)
			script_tests+=("$path")
			;;
		web/scripts/*.mjs | web/scripts/*.cjs)
			candidate="${path%.*}.test.${path##*.}"
			[[ -f "$ROOT/$candidate" ]] && script_tests+=("$candidate")
			;;
	esac
done <<<"$changed"
if ((${#web_paths[@]})); then
	echo "test-affected: frontend files through Vitest related: ${#web_paths[@]}"
	(
		cd "$ROOT/web"
		pnpm --filter @loomarr/web --filter './packages/*' -r --parallel exec \
			vitest related --run --passWithNoTests "${web_paths[@]}"
	)
	ran+=(frontend)
fi

if ((${#script_tests[@]})); then
	script_test_list="$(printf '%s\n' "${script_tests[@]}" | sort -u)"
	script_test_count="$(wc -l <<<"$script_test_list" | tr -d ' ')"
	echo "test-affected: directly owned script tests: $script_test_count"
	while IFS= read -r test_path; do
		case "$test_path" in
			*.sh) "$ROOT/$test_path" ;;
			*.mjs | *.cjs) (cd "$ROOT" && node --test "$test_path") ;;
		esac
	done <<<"$script_test_list"
	ran+=(scripts)
fi

scope="$(printf '%s\n' "$changed" | "$SCRIPT_DIR/ci-impact.sh")"
if grep -qx 'apple_mobile=true' <<<"$scope"; then
	echo 'test-affected: mobile app unit tests'
	(cd "$ROOT/web" && pnpm --filter @loomarr/mobile test)
	ran+=(mobile)
fi
if grep -qx 'apple_tv=true' <<<"$scope"; then
	echo 'test-affected: TV app unit tests'
	(cd "$ROOT/web" && pnpm --filter @loomarr/tv test)
	ran+=(tv)
fi
if grep -qE '^(Cargo\.(toml|lock)|rust-toolchain\.toml|deny\.toml|rust/)' <<<"$changed"; then
	echo 'test-affected: Rust workspace unit tests'
	(cd "$ROOT" && cargo test --workspace --all-features --locked)
	ran+=(rust)
fi

if ((${#ran[@]} == 0)); then
	echo 'test-affected: no directly related unit tests'
else
	printf 'test-affected: completed %s\n' "$(IFS=,; echo "${ran[*]}")"
fi
