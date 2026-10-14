#!/usr/bin/env bash
# Fast, non-authoritative unit feedback for the edit loop. Publication evidence remains make verify.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="${LOOMARR_REPO_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"
BASE="${1:-${BASE:-origin/main}}"
WARN_SECONDS="${AFFECTED_TEST_WARN_SECONDS:-30}"

if [[ ! "$WARN_SECONDS" =~ ^[0-9]+$ ]]; then
	echo "test-affected: AFFECTED_TEST_WARN_SECONDS must be a non-negative integer" >&2
	exit 2
fi

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
slow_owners=()
total_started="$(date +%s)"

record_timing() {
	local owner="$1" elapsed="$2" warn="${3:-true}"
	echo "test-affected: timing $owner ${elapsed}s"
	if [[ "$warn" == true ]] && ((elapsed > WARN_SECONDS)); then
		echo "test-affected: slow owner $owner ${elapsed}s (advisory threshold ${WARN_SECONDS}s)" >&2
		slow_owners+=("$owner")
	fi
}

run_timed() {
	local owner="$1"
	shift
	local started status elapsed warn=true
	started="$(date +%s)"
	if "$@"; then
		status=0
	else
		status=$?
	fi
	elapsed=$(($(date +%s) - started))
	case "$owner" in
	go:race | go:no-race) warn=false ;;
	esac
	record_timing "$owner" "$elapsed" "$warn"
	return "$status"
}

report_go_timings() {
	local log="$1" package duration seconds
	while read -r package duration; do
		[[ -n "$package" && -n "$duration" ]] || continue
		seconds="${duration%s}"
		echo "test-affected: timing go:$package $duration"
		if awk -v elapsed="$seconds" -v threshold="$WARN_SECONDS" 'BEGIN { exit !(elapsed > threshold) }'; then
			echo "test-affected: slow owner go:$package $duration (advisory threshold ${WARN_SECONDS}s)" >&2
			slow_owners+=("go:$package")
		fi
	done < <(awk '$1 == "ok" && $3 ~ /^[0-9]+([.][0-9]+)?s$/ { print $2, $3 }' "$log")
}

run_go_tests() {
	local mode="$1" packages="$2" log="$3"
	if [[ "$mode" == race ]]; then
		# shellcheck disable=SC2086 # newline-delimited package list intentionally becomes argv.
		(cd "$ROOT" && go test -race -timeout 25m $packages) | tee "$log"
	else
		# shellcheck disable=SC2086 # newline-delimited package list intentionally becomes argv.
		(cd "$ROOT" && go test -timeout 25m $packages) | tee "$log"
	fi
}

run_web_related() {
	(
		cd "$ROOT/web"
		pnpm --filter @loomarr/web --filter './packages/*' -r --parallel exec \
			vitest related --run --passWithNoTests "${web_paths[@]}"
	)
}

run_node_test() {
	(cd "$ROOT" && node --test "$1")
}

run_web_package_test() {
	(cd "$ROOT/web" && pnpm --filter "$1" test)
}

run_rust_tests() {
	(cd "$ROOT" && cargo test --workspace --all-features --locked)
}

go_packages="$(printf '%s\n' "$changed" | "$SCRIPT_DIR/go-direct-impact.sh")"
if [[ -n "$go_packages" ]]; then
	timing_dir="$(mktemp -d)"
	trap 'rm -rf "$timing_dir"' EXIT
	package_count="$(printf '%s\n' "$go_packages" | wc -l | tr -d ' ')"
	echo "test-affected: direct Go packages: $package_count"
	traced="$(printf '%s\n' "$go_packages" | "$SCRIPT_DIR/go-race-policy.sh" --race)"
	untraced="$(printf '%s\n' "$go_packages" | "$SCRIPT_DIR/go-race-policy.sh" --no-race)"
	if [[ -n "$traced" ]]; then
		run_timed go:race run_go_tests race "$traced" "$timing_dir/go-race.log"
		report_go_timings "$timing_dir/go-race.log"
	fi
	if [[ -n "$untraced" ]]; then
		run_timed go:no-race run_go_tests no-race "$untraced" "$timing_dir/go-no-race.log"
		report_go_timings "$timing_dir/go-no-race.log"
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
	run_timed frontend:related run_web_related
	ran+=(frontend)
fi

if ((${#script_tests[@]})); then
	script_test_list="$(printf '%s\n' "${script_tests[@]}" | sort -u)"
	script_test_count="$(wc -l <<<"$script_test_list" | tr -d ' ')"
	echo "test-affected: directly owned script tests: $script_test_count"
	while IFS= read -r test_path; do
		case "$test_path" in
		*.sh) run_timed "script:$test_path" "$ROOT/$test_path" ;;
		*.mjs | *.cjs) run_timed "script:$test_path" run_node_test "$test_path" ;;
		esac
	done <<<"$script_test_list"
	ran+=(scripts)
fi

scope="$(printf '%s\n' "$changed" | "$SCRIPT_DIR/ci-impact.sh")"
if grep -qx 'apple_mobile=true' <<<"$scope"; then
	echo 'test-affected: mobile app unit tests'
	run_timed native:mobile run_web_package_test @loomarr/mobile
	ran+=(mobile)
fi
if grep -qx 'apple_tv=true' <<<"$scope"; then
	echo 'test-affected: TV app unit tests'
	run_timed native:tv run_web_package_test @loomarr/tv
	ran+=(tv)
fi
if grep -qE '^(Cargo\.(toml|lock)|rust-toolchain\.toml|deny\.toml|rust/)' <<<"$changed"; then
	echo 'test-affected: Rust workspace unit tests'
	run_timed rust:workspace run_rust_tests
	ran+=(rust)
fi

if ((${#ran[@]} == 0)); then
	echo 'test-affected: no directly related unit tests'
else
	printf 'test-affected: completed %s\n' "$(IFS=,; echo "${ran[*]}")"
fi
record_timing total "$(($(date +%s) - total_started))" false
if ((${#slow_owners[@]})); then
	printf 'test-affected: advisory slow owners: %s\n' "$(IFS=,; echo "${slow_owners[*]}")"
else
	echo "test-affected: no owners exceeded the ${WARN_SECONDS}s advisory threshold"
fi
