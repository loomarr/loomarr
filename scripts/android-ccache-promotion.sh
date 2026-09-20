#!/usr/bin/env bash
# Transfer verified merge-queue ccache contents into the trusted main cache scope.

set -euo pipefail

readonly schema=1

sha256_stream() {
  shasum -a 256 | awk '{print $1}'
}

validate_cache_tree() {
  local cache_dir="$1"
  if [[ ! -d "${cache_dir}" || -z "$(find "${cache_dir}" -type f -print -quit)" ]]; then
    printf 'android ccache promotion: cache tree is missing or empty: %s\n' "${cache_dir}" >&2
    exit 1
  fi
  if [[ -n "$(find "${cache_dir}" ! -type d ! -type f -print -quit)" ]]; then
    printf 'android ccache promotion: cache tree contains a non-file entry\n' >&2
    exit 1
  fi
}

tree_digest() {
  local cache_dir="$1"
  validate_cache_tree "${cache_dir}"
  (
    cd "${cache_dir}"
    while IFS= read -r path; do
      printf '%s  %s\n' "$(sha256_stream <"${path}")" "${path#./}"
    done < <(find . -type f -print | LC_ALL=C sort)
  ) | sha256_stream
}

pack() {
  local cache_dir="$1" output_dir="$2" cache_key="$3" digest
  : "${GITHUB_EVENT_NAME:?GITHUB_EVENT_NAME is required}"
  : "${GITHUB_SHA:?GITHUB_SHA is required}"
  : "${GITHUB_RUN_ID:?GITHUB_RUN_ID is required}"
  : "${GITHUB_WORKFLOW_REF:?GITHUB_WORKFLOW_REF is required}"
  if [[ "${GITHUB_EVENT_NAME}" != merge_group ]]; then
    printf 'android ccache promotion: only merge-group producers may create transfers\n' >&2
    exit 1
  fi
  if [[ ! "${GITHUB_SHA}" =~ ^[0-9a-f]{40}$ || ! "${GITHUB_RUN_ID}" =~ ^[1-9][0-9]*$ ]]; then
    printf 'android ccache promotion: producer identity is malformed\n' >&2
    exit 1
  fi
  if [[ ! "${cache_key}" =~ ^android-tv-ccache-v2-Linux-ccache-4\.14-[0-9a-f]{64}-[1-9][0-9]*$ ]]; then
    printf 'android ccache promotion: cache key is malformed\n' >&2
    exit 1
  fi
  validate_cache_tree "${cache_dir}"
  if [[ -e "${output_dir}" ]]; then
    printf 'android ccache promotion: refusing to overwrite transfer: %s\n' "${output_dir}" >&2
    exit 1
  fi
  mkdir -p "${output_dir}/cache"
  cp -R "${cache_dir}/." "${output_dir}/cache/"
  validate_cache_tree "${output_dir}/cache"
  digest=$(tree_digest "${output_dir}/cache")
  jq -n \
    --argjson schema "${schema}" \
    --arg commit "${GITHUB_SHA}" \
    --arg run_id "${GITHUB_RUN_ID}" \
    --arg event "${GITHUB_EVENT_NAME}" \
    --arg workflow_ref "${GITHUB_WORKFLOW_REF}" \
    --arg cache_key "${cache_key}" \
    --arg tree_sha256 "${digest}" \
    '{schema: $schema, commit: $commit, producerRunId: $run_id,
      producerEvent: $event, producerWorkflowRef: $workflow_ref,
      cacheKey: $cache_key, treeSha256: $tree_sha256}' \
    >"${output_dir}/manifest.json"
  printf 'android ccache promotion: packed %s\n' "${cache_key}"
}

restore() {
  local transfer_dir="$1" output_dir="$2" expected_sha="$3" expected_run_id="$4" key_prefix="$5"
  local manifest="${transfer_dir}/manifest.json" cache_dir="${transfer_dir}/cache" cache_key expected_digest actual_digest
  if [[ ! -f "${manifest}" || ! "${expected_sha}" =~ ^[0-9a-f]{40}$ || ! "${expected_run_id}" =~ ^[1-9][0-9]*$ ]]; then
    printf 'android ccache promotion: restore identity is malformed\n' >&2
    exit 1
  fi
  if [[ "${key_prefix}" != android-tv-ccache-v2-Linux-ccache-4.14- ]]; then
    printf 'android ccache promotion: refusing unsafe cache prefix: %s\n' "${key_prefix}" >&2
    exit 2
  fi
  jq -e \
    --argjson schema "${schema}" \
    --arg sha "${expected_sha}" \
    --arg run_id "${expected_run_id}" \
    --arg prefix "${key_prefix}" \
    --arg run_id_suffix "-${expected_run_id}" '
      .schema == $schema and
      .commit == $sha and
      .producerRunId == $run_id and
      .producerEvent == "merge_group" and
      (.producerWorkflowRef | type == "string" and
        test("/\\.github/workflows/ci\\.yml@refs/heads/gh-readonly-queue/main/")) and
      (.cacheKey | type == "string" and startswith($prefix) and
        test("^android-tv-ccache-v2-Linux-ccache-4\\.14-[0-9a-f]{64}-[1-9][0-9]*$") and
        endswith($run_id_suffix)) and
      (.treeSha256 | type == "string" and test("^[0-9a-f]{64}$"))
    ' "${manifest}" >/dev/null || {
    printf 'android ccache promotion: transfer manifest does not bind the exact trusted producer\n' >&2
    exit 1
  }
  cache_key=$(jq -er '.cacheKey' "${manifest}")
  expected_digest=$(jq -er '.treeSha256' "${manifest}")
  actual_digest=$(tree_digest "${cache_dir}")
  if [[ "${actual_digest}" != "${expected_digest}" ]]; then
    printf 'android ccache promotion: transferred cache tree digest differs from its manifest\n' >&2
    exit 1
  fi
  if [[ -e "${output_dir}" ]]; then
    printf 'android ccache promotion: refusing to overwrite cache output: %s\n' "${output_dir}" >&2
    exit 1
  fi
  mkdir -p "${output_dir}"
  cp -R "${cache_dir}/." "${output_dir}/"
  printf 'cache-key=%s\n' "${cache_key}"
}

locate() {
  : "${GH_TOKEN:?GH_TOKEN is required}"
  : "${REPO:?REPO is required}"
  : "${GITHUB_SHA:?GITHUB_SHA is required}"
  local runs run_count run_id jobs android_job_count android_conclusion artifact_name artifacts artifact_count artifact_id
  if [[ ! "${GITHUB_SHA}" =~ ^[0-9a-f]{40}$ ]]; then
    printf 'android ccache promotion: current main identity is malformed\n' >&2
    exit 1
  fi
  runs=$(gh api "repos/${REPO}/actions/runs?head_sha=${GITHUB_SHA}&event=merge_group&status=success&per_page=100")
  run_count=$(jq -er --arg sha "${GITHUB_SHA}" '
    [.workflow_runs[] | select(
      .head_sha == $sha and .event == "merge_group" and .status == "completed" and
      .conclusion == "success" and .path == ".github/workflows/ci.yml"
    )] | length
  ' <<<"${runs}")
  if [[ "${run_count}" == 0 ]]; then
    echo 'found=false'
    return
  fi
  if [[ "${run_count}" != 1 ]]; then
    printf 'android ccache promotion: expected one exact successful merge-group CI run, got %s\n' "${run_count}" >&2
    exit 1
  fi
  run_id=$(jq -er --arg sha "${GITHUB_SHA}" '
    [.workflow_runs[] | select(
      .head_sha == $sha and .event == "merge_group" and .status == "completed" and
      .conclusion == "success" and .path == ".github/workflows/ci.yml"
    )][0].id | select(type == "number" and . > 0 and floor == .)
  ' <<<"${runs}")
  jobs=$(gh api "repos/${REPO}/actions/runs/${run_id}/jobs?per_page=100")
  android_job_count=$(jq -er '[.jobs[] | select(
    .name == "Android TV — React Native Play bundle" or
    .name == "Android TV — React Native Play bundle / Android TV — React Native Play bundle"
  )] | length' <<<"${jobs}")
  if [[ "${android_job_count}" != 1 ]]; then
    printf 'android ccache promotion: merge-group CI has %s Android jobs, want one\n' "${android_job_count}" >&2
    exit 1
  fi
  android_conclusion=$(jq -er '[.jobs[] | select(
    .name == "Android TV — React Native Play bundle" or
    .name == "Android TV — React Native Play bundle / Android TV — React Native Play bundle"
  )][0].conclusion | select(type == "string")' <<<"${jobs}")
  if [[ "${android_conclusion}" == skipped ]]; then
    echo 'found=false'
    return
  fi
  if [[ "${android_conclusion}" != success ]]; then
    printf 'android ccache promotion: Android producer did not succeed: %s\n' "${android_conclusion}" >&2
    exit 1
  fi
  artifact_name="loomarr-android-ccache-${GITHUB_SHA}-${run_id}"
  artifacts=$(gh api "repos/${REPO}/actions/runs/${run_id}/artifacts?per_page=100")
  artifact_count=$(jq -er --arg name "${artifact_name}" \
    '[.artifacts[] | select(.name == $name and .expired == false)] | length' <<<"${artifacts}")
  if [[ "${artifact_count}" != 1 ]]; then
    printf 'android ccache promotion: expected one unexpired exact transfer artifact, got %s\n' "${artifact_count}" >&2
    exit 1
  fi
  artifact_id=$(jq -er --arg name "${artifact_name}" '
    [.artifacts[] | select(.name == $name and .expired == false)][0].id
    | select(type == "number" and . > 0 and floor == .)
  ' <<<"${artifacts}")
  printf 'found=true\nrun-id=%s\nartifact-id=%s\nartifact-name=%s\n' \
    "${run_id}" "${artifact_id}" "${artifact_name}"
}

retention_plan() {
  local caches_json="$1" ref="$2" prefix="$3" keep_key="$4"
  if [[ ! -f "${caches_json}" || "${ref}" != refs/heads/main ||
        "${prefix}" != android-tv-ccache-v2-Linux-ccache-4.14- ||
        "${keep_key}" != "${prefix}"* ]]; then
    printf 'android ccache promotion: unsafe retention input\n' >&2
    exit 2
  fi
  jq -er --arg ref "${ref}" --arg prefix "${prefix}" --arg keep "${keep_key}" '
    .actions_caches as $caches
    | if ($caches | type) != "array" then error("actions_caches must be an array") else $caches end
    | [
        .[]
        | select(.ref == $ref)
        | select((.key | type) == "string" and (.key | startswith($prefix)) and .key != $keep)
      ] as $matching
    | if any($matching[]; (.id | type) != "number" or .id < 1 or (.id | floor) != .id)
      then error("matching cache metadata is malformed")
      else $matching[]?.id
      end
  ' "${caches_json}"
}

case "${1:-}" in
  pack)
    [[ $# == 4 ]] || { echo 'usage: android-ccache-promotion.sh pack CACHE_DIR OUTPUT_DIR CACHE_KEY' >&2; exit 2; }
    pack "$2" "$3" "$4"
    ;;
  restore)
    [[ $# == 6 ]] || { echo 'usage: android-ccache-promotion.sh restore TRANSFER_DIR OUTPUT_DIR SHA RUN_ID KEY_PREFIX' >&2; exit 2; }
    restore "$2" "$3" "$4" "$5" "$6"
    ;;
  locate)
    [[ $# == 1 ]] || { echo 'usage: android-ccache-promotion.sh locate' >&2; exit 2; }
    locate
    ;;
  retention-plan)
    [[ $# == 5 ]] || { echo 'usage: android-ccache-promotion.sh retention-plan CACHES_JSON REF PREFIX KEEP_KEY' >&2; exit 2; }
    retention_plan "$2" "$3" "$4" "$5"
    ;;
  *)
    echo 'usage: android-ccache-promotion.sh {pack|restore|locate|retention-plan} ...' >&2
    exit 2
    ;;
esac
