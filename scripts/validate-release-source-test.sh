#!/usr/bin/env bash

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test_root=$(mktemp -d /tmp/release-source-test.XXXXXX)
trap 'find "$test_root" -mindepth 1 -depth -delete; rmdir "$test_root"' EXIT
mkdir -p "$test_root/bin"

sha=0123456789abcdef0123456789abcdef01234567

cat > "$test_root/bin/git" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  'rev-parse HEAD'|'rev-parse origin/main') printf '%s\n' "$RELEASE_TEST_SHA" ;;
  'fetch --no-tags origin main') ;;
  *) echo "unexpected git invocation: $*" >&2; exit 64 ;;
esac
STUB

cat > "$test_root/bin/gh" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == *'/jobs?per_page=100'* ]]; then
  printf '%b\n' "$RELEASE_TEST_JOBS"
else
  printf '%b\n' "$RELEASE_TEST_RUN"
fi
STUB
chmod +x "$test_root/bin/git" "$test_root/bin/gh"

common_jobs=$'CI\tsuccess\nImage — release build (linux/amd64)\tsuccess\nImage — release build (linux/arm64)\tsuccess'
candidate_jobs=$'Release candidate — exact main scope\tsuccess\nGo + Rust — repository contracts\tsuccess\nRust image — runtime certification\tsuccess'

run_case() {
  local name=$1 event=$2 jobs=$3 expected=$4
  local output status
  set +e
  output=$(cd "$root" && \
    PATH="$test_root/bin:$PATH" \
    RELEASE_TEST_SHA="$sha" \
    RELEASE_TEST_RUN=$'42\tsuccess\t'"$event" \
    RELEASE_TEST_JOBS="$jobs" \
    GITHUB_REF_NAME=v1.2.3 \
    GITHUB_REPOSITORY=loomarr/loomarr \
    GITHUB_SHA="$sha" \
    ./scripts/validate-release-source.sh 2>&1)
  status=$?
  set -e
  if [[ "$expected" == pass && $status -ne 0 ]]; then
    printf 'validate-release-source-test: %s unexpectedly failed:\n%s\n' "$name" "$output" >&2
    exit 1
  fi
  if [[ "$expected" == fail && $status -eq 0 ]]; then
    printf 'validate-release-source-test: %s unexpectedly passed\n' "$name" >&2
    exit 1
  fi
}

run_case push-evidence push "$common_jobs" fail
run_case candidate-evidence workflow_dispatch "$common_jobs"$'\n'"$candidate_jobs" pass
run_case full-evidence workflow_dispatch "$common_jobs"$'\nManual CI — full scope\tsuccess' fail
run_case candidate-missing-contract workflow_dispatch \
  "$common_jobs"$'\nRelease candidate — exact main scope\tsuccess\nRust image — runtime certification\tsuccess' fail
run_case dispatch-missing-marker workflow_dispatch "$common_jobs" fail
run_case unsupported-event pull_request "$common_jobs" fail
run_case missing-aggregate push \
  $'Image — release build (linux/amd64)\tsuccess\nImage — release build (linux/arm64)\tsuccess' fail

echo 'validate-release-source-test: ok'
