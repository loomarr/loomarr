#!/usr/bin/env bash

set -euo pipefail

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
promotion="${repo_root}/scripts/android-ccache-promotion.sh"
test_root=$(mktemp -d /tmp/android-ccache-promotion-test.XXXXXX)
trap 'rm -rf -- "${test_root}"' EXIT

cache_dir="${test_root}/cache"
transfer_dir="${test_root}/transfer"
restored_dir="${test_root}/restored"
mkdir -p "${cache_dir}/a/1"
printf 'cache payload\n' >"${cache_dir}/a/1/result"
printf 'max_size = 2G\n' >"${cache_dir}/ccache.conf"

GITHUB_EVENT_NAME=merge_group \
GITHUB_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
GITHUB_RUN_ID=12345 \
GITHUB_WORKFLOW_REF='loomarr/loomarr/.github/workflows/ci.yml@refs/heads/gh-readonly-queue/main/pr-1-deadbeef' \
  "${promotion}" pack "${cache_dir}" "${transfer_dir}" \
  android-tv-ccache-v2-Linux-ccache-4.14-ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff-12345

jq -e '
  .schema == 1 and
  .commit == "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" and
  .producerRunId == "12345" and
  .producerEvent == "merge_group" and
  .cacheKey == "android-tv-ccache-v2-Linux-ccache-4.14-ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff-12345" and
  (.treeSha256 | test("^[0-9a-f]{64}$"))
' "${transfer_dir}/manifest.json" >/dev/null

"${promotion}" restore \
  "${transfer_dir}" "${restored_dir}" \
  aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa 12345 \
  android-tv-ccache-v2-Linux-ccache-4.14-
cmp "${cache_dir}/a/1/result" "${restored_dir}/a/1/result"

jq '.commit = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"' \
  "${transfer_dir}/manifest.json" >"${transfer_dir}/manifest.bad.json"
mv "${transfer_dir}/manifest.json" "${transfer_dir}/manifest.good.json"
mv "${transfer_dir}/manifest.bad.json" "${transfer_dir}/manifest.json"
if "${promotion}" restore \
  "${transfer_dir}" "${test_root}/wrong-commit" \
  aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa 12345 \
  android-tv-ccache-v2-Linux-ccache-4.14- >/dev/null 2>&1; then
  echo 'android-ccache-promotion-test: restore accepted the wrong commit' >&2
  exit 1
fi
mv "${transfer_dir}/manifest.good.json" "${transfer_dir}/manifest.json"

jq '.cacheKey |= sub("-12345$"; "-99999")' \
  "${transfer_dir}/manifest.json" >"${transfer_dir}/manifest.bad.json"
mv "${transfer_dir}/manifest.json" "${transfer_dir}/manifest.good.json"
mv "${transfer_dir}/manifest.bad.json" "${transfer_dir}/manifest.json"
if "${promotion}" restore \
  "${transfer_dir}" "${test_root}/wrong-run-key" \
  aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa 12345 \
  android-tv-ccache-v2-Linux-ccache-4.14- >/dev/null 2>&1; then
  echo 'android-ccache-promotion-test: restore accepted a cache key from another run' >&2
  exit 1
fi
mv "${transfer_dir}/manifest.good.json" "${transfer_dir}/manifest.json"

printf 'tampered\n' >>"${transfer_dir}/cache/a/1/result"
if "${promotion}" restore \
  "${transfer_dir}" "${test_root}/tampered" \
  aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa 12345 \
  android-tv-ccache-v2-Linux-ccache-4.14- >/dev/null 2>&1; then
  echo 'android-ccache-promotion-test: restore accepted a changed cache tree' >&2
  exit 1
fi
printf 'cache payload\n' >"${transfer_dir}/cache/a/1/result"

mkdir -p "${test_root}/mock-bin"
cat >"${test_root}/mock-bin/gh" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *'/actions/runs?'*) cat "${MOCK_RUNS}" ;;
  *'/actions/runs/12345/jobs'*) cat "${MOCK_JOBS}" ;;
  *'/actions/runs/12345/artifacts'*) cat "${MOCK_ARTIFACTS}" ;;
  *) printf 'unexpected gh invocation: %s\n' "$*" >&2; exit 1 ;;
esac
MOCK
chmod +x "${test_root}/mock-bin/gh"

cat >"${test_root}/runs.json" <<'JSON'
{"workflow_runs":[{"id":12345,"head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","event":"merge_group","status":"completed","conclusion":"success","path":".github/workflows/ci.yml"}]}
JSON
cat >"${test_root}/jobs.json" <<'JSON'
{"jobs":[{"name":"Android TV — React Native Play bundle / Android TV — React Native Play bundle","conclusion":"success"}]}
JSON
cat >"${test_root}/artifacts.json" <<'JSON'
{"artifacts":[{"id":987,"name":"loomarr-android-ccache-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-12345","expired":false}]}
JSON

got=$(
  PATH="${test_root}/mock-bin:${PATH}" \
  MOCK_RUNS="${test_root}/runs.json" \
  MOCK_JOBS="${test_root}/jobs.json" \
  MOCK_ARTIFACTS="${test_root}/artifacts.json" \
  GH_TOKEN=test REPO=loomarr/loomarr GITHUB_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
    "${promotion}" locate
)
want=$'found=true\nrun-id=12345\nartifact-id=987\nartifact-name=loomarr-android-ccache-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-12345'
if [[ "${got}" != "${want}" ]]; then
  printf 'android-ccache-promotion-test: locate got:\n%s\n' "${got}" >&2
  exit 1
fi

jq '.jobs[0].conclusion = "skipped"' "${test_root}/jobs.json" >"${test_root}/jobs-skipped.json"
got=$(
  PATH="${test_root}/mock-bin:${PATH}" \
  MOCK_RUNS="${test_root}/runs.json" \
  MOCK_JOBS="${test_root}/jobs-skipped.json" \
  MOCK_ARTIFACTS="${test_root}/artifacts.json" \
  GH_TOKEN=test REPO=loomarr/loomarr GITHUB_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
    "${promotion}" locate
)
[[ "${got}" == 'found=false' ]] || {
  printf 'android-ccache-promotion-test: skipped producer got %s\n' "${got}" >&2
  exit 1
}

jq '.workflow_runs += [.workflow_runs[0] | .id = 67890]' \
  "${test_root}/runs.json" >"${test_root}/runs-ambiguous.json"
if PATH="${test_root}/mock-bin:${PATH}" \
  MOCK_RUNS="${test_root}/runs-ambiguous.json" \
  MOCK_JOBS="${test_root}/jobs.json" \
  MOCK_ARTIFACTS="${test_root}/artifacts.json" \
  GH_TOKEN=test REPO=loomarr/loomarr GITHUB_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
    "${promotion}" locate >/dev/null 2>&1; then
  echo 'android-ccache-promotion-test: locate accepted ambiguous merge-group runs' >&2
  exit 1
fi

cat >"${test_root}/caches.json" <<'JSON'
{"actions_caches":[
  {"id":10,"ref":"refs/heads/main","key":"android-tv-ccache-v2-Linux-ccache-4.14-old-100"},
  {"id":11,"ref":"refs/heads/main","key":"android-tv-ccache-v2-Linux-ccache-4.14-current-101"},
  {"id":12,"ref":"refs/pull/1/merge","key":"android-tv-ccache-v2-Linux-ccache-4.14-old-99"},
  {"id":13,"ref":"refs/heads/main","key":"unrelated"}
]}
JSON
got=$("${promotion}" retention-plan "${test_root}/caches.json" \
  refs/heads/main android-tv-ccache-v2-Linux-ccache-4.14- \
  android-tv-ccache-v2-Linux-ccache-4.14-current-101)
[[ "${got}" == 10 ]] || {
  printf 'android-ccache-promotion-test: retention plan got %s, want 10\n' "${got}" >&2
  exit 1
}

echo 'android-ccache-promotion-test: ok'
