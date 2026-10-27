#!/usr/bin/env bash
# Pin the bundled FFmpeg to BtbN's NEWEST release-series GPL build, mirrored with its source.
#
#   ffmpeg-pin-refresh.sh check            key=value facts (GITHUB_OUTPUT-ready); changes nothing
#   ffmpeg-pin-refresh.sh dry-run WORK     download + verify + stage the source release; no publish, no write
#   ffmpeg-pin-refresh.sh publish WORK     dry-run, then publish third-party-sources-<date> and rewrite the pins
#   ffmpeg-pin-refresh.sh automate WORK    the weekly workflow: when BtbN is newer and no pin branch exists,
#                                          publish (or, with DRY_RUN=true, dry-run), verify, push
#                                          deps/ffmpeg-<build>, open the PR and dispatch full CI for it
#
# "Newest release series" is the highest nX.Y GPL build in BtbN's newest autobuild (never master's
# N- builds). The source release carries both binary archives (our mirror: BtbN prunes dailies, so
# the Dockerfile downloads from us), the exact FFmpeg source, BtbN's recipe at the commit the run
# built, and that run's original `download-cache` artifact, which BtbN deletes after 14 days. A
# missing or expired cache FAILS: without it this build has no complete corresponding source.
# WORK is reused across runs: every file already there is re-verified by digest, never trusted.
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
btbn="${BTBN_REPO:-BtbN/FFmpeg-Builds}"
loomarr="${LOOMARR_REPO:-loomarr/loomarr}"
pin_date="${FFMPEG_PIN_DATE:-$(date -u +%Y-%m-%d)}"
# GitHub refuses release assets of 2 GiB or more; larger files are published as numbered parts.
part_bytes=1900000000

die() { echo "ffmpeg-pin-refresh: $*" >&2; exit 1; }
usage() { echo "usage: $0 check | dry-run WORK_DIR | publish WORK_DIR | automate WORK_DIR" >&2; exit 2; }
mode="${1:-}"
case "$mode" in
  check) (($# == 1)) || usage ;;
  dry-run|publish|automate) (($# == 2)) || usage; work="$2" ;;
  *) usage ;;
esac
[[ "$pin_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || die "FFMPEG_PIN_DATE must be YYYY-MM-DD"
for tool in gh jq curl sha256sum; do command -v "$tool" >/dev/null || die "$tool is required"; done

read_pin() {
  local value
  value="$(sed -n "s/^ARG $1=//p" "$repo_root/Dockerfile")"
  [[ -n "$value" && "$value" != *$'\n'* ]] || die "missing or duplicate Dockerfile pin: $1"
  printf '%s' "$value"
}
sha() { local s; s="$(sha256sum "$1")"; printf '%s' "${s%% *}"; }
series_of() { [[ "$1" =~ ^n([0-9]+)\.([0-9]+) ]] && printf '%s.%s' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}"; }

# --- Resolve BtbN's newest autobuild and its newest release-series GPL build ---------------------
release_json="$(gh api "repos/$btbn/releases?per_page=30" \
  | jq -c '[.[] | select(.tag_name | test("^autobuild-[0-9]{4}(-[0-9]{2}){4}$"))] | sort_by(.created_at) | last // empty')"
[[ -n "$release_json" ]] || die "no autobuild release found in $btbn"
release="$(jq -r .tag_name <<<"$release_json")"
build_id="$(jq -r '
  [.assets[].name | capture("^ffmpeg-(?<id>n(?<maj>[0-9]+)\\.(?<min>[0-9]+)(\\.[0-9]+)?-[0-9]+-g[0-9a-f]+)-linux64-gpl-(?<s>[0-9]+\\.[0-9]+)\\.tar\\.xz$")
   | select(.s == "\(.maj).\(.min)")]
  | sort_by([(.maj | tonumber), (.min | tonumber)]) | last | .id // empty' <<<"$release_json")"
[[ -n "$build_id" ]] || die "$release carries no release-series linux64 GPL build"
series="$(series_of "$build_id")"
current_build="$(read_pin FFMPEG_BUILD_ID)"
current_series="$(series_of "$current_build")"
changed=false
[[ "$build_id" == "$current_build" && "$release" == "$(read_pin FFMPEG_RELEASE)" ]] || changed=true
series_change=false
[[ "$series" == "$current_series" ]] || series_change=true
if [[ "$mode" == check ]]; then
  printf '%s\n' "release=$release" "build_id=$build_id" "series=$series" "current_build=$current_build" \
    "current_series=$current_series" "changed=$changed" "series_change=$series_change" "mirror=third-party-sources-$pin_date"
  exit 0
fi
automate=false
if [[ "$mode" == automate ]]; then
  automate=true
  branch="deps/ffmpeg-$build_id"
  [[ "$changed" == true ]] || { echo "the pin $current_build is BtbN's newest build; nothing to do" >&2; exit 0; }
  if git -C "$repo_root" ls-remote --exit-code --heads origin "$branch" >/dev/null; then
    echo "$branch already exists; nothing to do" >&2
    exit 0
  fi
  [[ "${DRY_RUN:-false}" == true ]] && mode=dry-run || mode=publish
fi

# --- Provenance: the run that published the release, its recipe, the FFmpeg source commit -------
# A release's created_at is its tag commit's date; published_at is when the Publish job made it.
created="$(jq -r .published_at <<<"$release_json")"
run_id=""
for candidate in $(gh api "repos/$btbn/actions/workflows/build.yml/runs?status=success&per_page=30" \
  --jq ".workflow_runs[] | select(.created_at < \"$created\") | .id"); do
  jobs="$(gh api --paginate "repos/$btbn/actions/runs/$candidate/jobs?per_page=100" --jq '.jobs[] | [.name, .conclusion, (.completed_at // "")] | @tsv')"
  published="$(awk -F'\t' -v t="$created" '$1 == "Publish release" && $2 == "success" && $3 >= t' <<<"$jobs")"
  built="$(awk -F'\t' -v s="$series" '($1 == "Build ffmpeg (linux64, gpl " s ")" || $1 == "Build ffmpeg (linuxarm64, gpl " s ")") && $2 == "success"' <<<"$jobs" | wc -l)"
  if [[ -n "$published" && "$built" == 2 ]]; then run_id="$candidate"; break; fi
done
[[ -n "$run_id" ]] || die "no successful BtbN run published $release with both $series GPL builds"
recipe="$(gh api "repos/$btbn/actions/runs/$run_id" --jq .head_sha)"
tag_commit="$(gh api "repos/$btbn/commits/$release" --jq .sha)"
cache_json="$(gh api "repos/$btbn/actions/runs/$run_id/artifacts?name=download-cache" \
  --jq '[.artifacts[] | select(.expired == false)] | last // empty')"
[[ -n "$cache_json" ]] || die "run $run_id's download-cache artifact is EXPIRED or missing (BtbN keeps it 14 days); this build's corresponding source cannot be captured, so it must not be pinned"
cache_id="$(jq -r .id <<<"$cache_json")"
cache_digest="$(jq -r '.digest | ltrimstr("sha256:")' <<<"$cache_json")"
[[ "$cache_digest" =~ ^[0-9a-f]{64}$ ]] || die "artifact $cache_id has no sha256 digest"
abbrev="${build_id##*-g}"
source_commit="$(gh api "repos/FFmpeg/FFmpeg/commits/$abbrev" --jq .sha)"
[[ "$source_commit" == "$abbrev"* ]] || die "FFmpeg commit $abbrev did not resolve"
base_tag="${build_id%-*-g*}"
distance="${build_id#"$base_tag"-}"; distance="${distance%%-*}"
gh api "repos/FFmpeg/FFmpeg/compare/$base_tag...$source_commit" --jq '"\(.ahead_by) \(.behind_by)"' \
  | grep -qx "$distance 0" || die "$source_commit is not $distance commits after $base_tag"

# --- Download and verify --------------------------------------------------------------------------
mkdir -p "$work/release"
fetch() { # fetch URL DEST DIGEST — reuse DEST only when it still matches DIGEST
  local url="$1" dest="$2" digest="$3"
  if [[ -f "$dest" && "$(sha "$dest")" == "$digest" ]]; then return; fi
  curl --fail --location --retry 5 --retry-all-errors --retry-max-time 300 --connect-timeout 20 -sS -o "$dest.partial" "$url"
  [[ "$(sha "$dest.partial")" == "$digest" ]] || die "$(basename "$dest") does not match $digest"
  mv -- "$dest.partial" "$dest"
}
curl --fail --location -sS -o "$work/checksums.sha256" "https://github.com/$btbn/releases/download/$release/checksums.sha256"
declare -A asset_sha
for arch in amd64 arm64; do
  [[ "$arch" == amd64 ]] && btbn_arch=linux64 || btbn_arch=linuxarm64
  asset="ffmpeg-$build_id-$btbn_arch-gpl-$series.tar.xz"
  digest="$(jq -r --arg a "$asset" '.assets[] | select(.name == $a) | .digest | ltrimstr("sha256:")' <<<"$release_json")"
  [[ "$digest" =~ ^[0-9a-f]{64}$ ]] || die "$asset is missing from $release"
  grep -qx "$digest  $asset" "$work/checksums.sha256" || die "$asset digest differs from BtbN's checksums.sha256"
  fetch "https://github.com/$btbn/releases/download/$release/$asset" "$work/release/$asset" "$digest"
  asset_sha[$arch]="$digest"
done
source_archive="ffmpeg-$source_commit.tar.gz"
recipe_archive="ffmpeg-builds-${recipe:0:8}.tar.gz"
for pair in "FFmpeg/FFmpeg/archive/$source_commit.tar.gz $source_archive" "$btbn/archive/$recipe.tar.gz $recipe_archive"; do
  read -r path name <<<"$pair"
  # GitHub generates these archives on demand; two downloads must agree before one is published.
  curl --fail --location --retry 5 --retry-all-errors -sS -o "$work/$name.a" "https://github.com/$path"
  curl --fail --location --retry 5 --retry-all-errors -sS -o "$work/$name.b" "https://github.com/$path"
  [[ "$(sha "$work/$name.a")" == "$(sha "$work/$name.b")" ]] || die "$name is not stable across downloads"
  mv -- "$work/$name.a" "$work/release/$name"; rm -f -- "$work/$name.b"
done
cache="$work/ffmpeg-original-download-cache.zip"
if [[ ! -f "$cache" || "$(sha "$cache")" != "$cache_digest" ]]; then
  gh api "repos/$btbn/actions/artifacts/$cache_id/zip" >"$cache.partial"
  [[ "$(sha "$cache.partial")" == "$cache_digest" ]] || die "download-cache artifact $cache_id does not match its digest"
  mv -- "$cache.partial" "$cache"
fi
unzip -l "$cache" | grep -q ' cache\.tar\.gz$' || die "download-cache artifact has no cache.tar.gz"
cache_bytes="$(stat -c %s "$cache")"
rm -f "$work/release/ffmpeg-original-download-cache.zip"*
if ((cache_bytes >= part_bytes)); then
  split -b "$part_bytes" -d -a 2 "$cache" "$work/release/ffmpeg-original-download-cache.zip.part"
else
  ln -f "$cache" "$work/release/ffmpeg-original-download-cache.zip"
fi

# --- Stage the source release ---------------------------------------------------------------------
tag="third-party-sources-$pin_date"
release_url="https://github.com/$loomarr/releases/tag/$tag"
cat >"$work/release/README.txt" <<EOF
Loomarr — third-party source materials (FFmpeg $build_id)

Binaries and corresponding source for the FFmpeg pin: BtbN autobuild $release, build $build_id
(linux64-gpl-$series and linuxarm64-gpl-$series), published by BtbN run $run_id. This release
contains no Loomarr application binaries and does not certify any application release.

ffmpeg-$build_id-*-gpl-$series.tar.xz are BtbN's archives, byte-identical to the release assets
(SHA256 checked against BtbN's checksums.sha256 and GitHub's asset digests). Loomarr's image
downloads them from here because BtbN prunes daily releases.

$source_archive is the exact FFmpeg source at $source_commit ($distance commits after $base_tag).
$recipe_archive is the BtbN build recipe at $recipe, the commit run $run_id built (the release tag
itself points at $tag_commit). Both are GitHub source archives of those commits.

ffmpeg-original-download-cache.zip is BtbN run $run_id's download-cache artifact $cache_id
($cache_bytes bytes, SHA256 $cache_digest): cache.tar.gz holds the original dependency source
archives the build restored. GitHub limits release assets to 2 GiB, so a larger cache is published
as numbered .partNN files; join them in order (cat ffmpeg-original-download-cache.zip.part* >
ffmpeg-original-download-cache.zip) and check the SHA256 above.

Each upstream archive retains its own license and copyright materials. This package does not assert
bit-for-bit reproduction of upstream outputs. Verify SHA256SUMS before use. Source distribution:
$release_url
EOF
jq -n --arg release "$release" --arg build "$build_id" --arg series "$series" --arg source "$source_commit" \
  --arg recipe "$recipe" --arg tagCommit "$tag_commit" --argjson run "$run_id" --argjson artifact "$cache_id" \
  --argjson bytes "$cache_bytes" --arg digest "$cache_digest" '{
    schemaVersion: 1, ffmpegRelease: $release, ffmpegReleaseCommit: $tagCommit, ffmpegBuild: $build, ffmpegSeries: $series,
    ffmpegSourceCommit: $source, buildRecipeCommit: $recipe, upstreamRun: $run,
    originalFFmpegCache: {upstreamRun: $run, upstreamArtifact: $artifact, bytes: $bytes, sha256: $digest}}' \
  >"$work/release/source-package-inventory.json"
rm -f "$work/release/SHA256SUMS"
(cd "$work/release" && find . -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort | xargs sha256sum) >"$work/SHA256SUMS"
mv -- "$work/SHA256SUMS" "$work/release/SHA256SUMS"
cat >"$work/notes.md" <<EOF
Binaries and corresponding source for Loomarr's FFmpeg pin: BtbN \`$release\`, build \`$build_id\` (linux64/linuxarm64 gpl-$series). Source-only release; it contains no Loomarr application binaries and does not certify an application release.

- FFmpeg source at \`$source_commit\`, BtbN recipe at \`$recipe\` (the commit run [$run_id](https://github.com/$btbn/actions/runs/$run_id) built).
- \`ffmpeg-original-download-cache.zip\`: that run's original dependency download cache (artifact \`$cache_id\`, SHA256 \`$cache_digest\`); split into \`.partNN\` files when over GitHub's 2 GiB asset limit.

Verify \`SHA256SUMS\` before use; \`README.txt\` explains the contents.
EOF
echo "staged $tag in $work/release:" >&2
(cd "$work/release" && ls -l) >&2
[[ "$mode" == publish ]] || { echo "dry-run: nothing published or written" >&2; exit 0; }

# --- Publish, then verify what GitHub serves -----------------------------------------------------
if gh release view "$tag" --repo "$loomarr" >/dev/null 2>&1; then die "$tag already exists"; fi
target="$(gh api "repos/$loomarr/commits/main" --jq .sha)"
gh release create "$tag" --repo "$loomarr" --prerelease --latest=false --target "$target" \
  --title "Third-party source materials — FFmpeg $build_id" --notes-file "$work/notes.md" "$work"/release/*
published_json="$(gh api "repos/$loomarr/releases/tags/$tag")"
while read -r name; do
  want="$(sha "$work/release/$name")"
  got="$(jq -r --arg n "$name" '.assets[] | select(.name == $n) | .digest | ltrimstr("sha256:")' <<<"$published_json")"
  [[ "$got" == "$want" ]] || die "published $name digest $got differs from local $want"
done < <(cd "$work/release" && ls)

# --- Rewrite the pins and evidence ---------------------------------------------------------------
set_arg() { # set_arg NAME VALUE — rewrite exactly one `ARG NAME=` line, then prove it
  local tmp="$repo_root/Dockerfile.pin.$$"
  awk -v k="ARG $1=" -v v="$2" 'index($0, k) == 1 { print k v; n++; next } { print } END { exit n != 1 }' \
    "$repo_root/Dockerfile" >"$tmp" || { rm -f "$tmp"; die "Dockerfile must have exactly one ARG $1"; }
  mv -- "$tmp" "$repo_root/Dockerfile"
  [[ "$(read_pin "$1")" == "$2" ]] || die "ARG $1 did not take $2"
}
set_arg FFMPEG_RELEASE "$release"
set_arg FFMPEG_MIRROR_RELEASE "$tag"
set_arg FFMPEG_BUILD_ID "$build_id"
set_arg FFMPEG_AMD64_SHA256 "${asset_sha[amd64]}"
set_arg FFMPEG_ARM64_SHA256 "${asset_sha[arm64]}"

manifest="$repo_root/project/evidence/redistribution-manifest-v1.json"
mirror_dl="https://github.com/$loomarr/releases/download/$tag"
jq --arg release "$release" --arg build "$build_id" --arg series "$series" --arg source "$source_commit" \
  --arg recipe "$recipe" --arg tagCommit "$tag_commit" --arg tag "$tag" --arg dl "$mirror_dl" --arg url "$release_url" \
  --arg a "${asset_sha[amd64]}" --arg r "${asset_sha[arm64]}" '
  (.artifacts[] | select(.name == "ffmpeg")) |= {
    name: "ffmpeg",
    assets: [
      {architecture: "amd64", asset: "ffmpeg-\($build)-linux64-gpl-\($series).tar.xz", asset_url: "\($dl)/ffmpeg-\($build)-linux64-gpl-\($series).tar.xz", sha256: $a},
      {architecture: "arm64", asset: "ffmpeg-\($build)-linuxarm64-gpl-\($series).tar.xz", asset_url: "\($dl)/ffmpeg-\($build)-linuxarm64-gpl-\($series).tar.xz", sha256: $r}],
    upstream: {
      release_url: "https://github.com/BtbN/FFmpeg-Builds/releases/tag/\($release)", release_revision: $release,
      release_commit: $tagCommit,
      source_url: "https://github.com/FFmpeg/FFmpeg/commit/\($source)", source_revision: $source,
      build_url: "https://github.com/BtbN/FFmpeg-Builds/tree/\($recipe)", build_revision: $recipe, build_id: $build,
      retention_class: "mirrored", retention_policy_url: $url, retention_policy_revision: $tag,
      license_urls: ["https://github.com/BtbN/FFmpeg-Builds/blob/\($recipe)/LICENSE", "https://github.com/FFmpeg/FFmpeg/blob/\($source)/COPYING.GPLv3"]}}' \
  "$manifest" >"$manifest.tmp"
mv -- "$manifest.tmp" "$manifest"

distribution="$repo_root/project/evidence/source-distribution-$pin_date.json"
jq --arg url "$release_url" --arg tag "$tag" --arg target "$target" --arg build "$build_id" --arg dl "$mirror_dl" '{
    schemaVersion: 1, sourceRelease: $url, sourceTag: $tag, releaseTarget: $target, publishedAt: .published_at,
    scope: "FFmpeg \($build): both binary archives (the image downloads them from here), exact source, BtbN recipe and the original dependency download cache",
    assets: [.assets[] | {name, bytes: .size, sha256: (.digest | ltrimstr("sha256:")), url: "\($dl)/\(.name)"}] | sort_by(.name)}' \
  <<<"$published_json" >"$distribution"

record="$repo_root/project/evidence/ffmpeg-pin-$pin_date.md"
cat >"$record" <<EOF
# FFmpeg pin $build_id

Written by \`scripts/ffmpeg-pin-refresh.sh\` when it published the source release. Release acceptance
is separate: the pin's PR runs \`make test-ffmpeg\` and the image build.

| Artifact | Identity | SHA256 |
| --- | --- | --- |
| Linux amd64 binary archive | BtbN \`$release\`, \`$build_id\` | \`${asset_sha[amd64]}\` |
| Linux arm64 binary archive | Same release/build | \`${asset_sha[arm64]}\` |
| FFmpeg source archive | \`$source_commit\` ($distance commits after \`$base_tag\`) | \`$(sha "$work/release/$source_archive")\` |
| BtbN build recipe archive | \`$recipe\` (built by run \`$run_id\`) | \`$(sha "$work/release/$recipe_archive")\` |
| Original dependency download cache | Run \`$run_id\`, artifact \`$cache_id\`, $cache_bytes bytes | \`$cache_digest\` |

- **Build selection.** \`$release\` was BtbN's newest autobuild, and \`$build_id\` its newest release-series
  (\`$series\`) GPL build. Both binary digests match BtbN's \`checksums.sha256\` and GitHub's asset digests.
- **Provenance.** Run [\`$run_id\`](https://github.com/$btbn/actions/runs/$run_id) built
  \`Build ffmpeg (linux64, gpl $series)\` and \`Build ffmpeg (linuxarm64, gpl $series)\` from recipe
  \`$recipe\`, and its \`Publish release\` job created the release. The release tag points at \`$tag_commit\`,
  BtbN's master when the release was published.
- **Source archives.** These are GitHub's archives of the two commits; two downloads of each agreed byte
  for byte before publication. The cache is the run's own \`download-cache\` artifact, matched against
  GitHub's artifact digest.
- **Publication.** Everything is published in [$tag]($release_url), together with both binary archives.
  The Dockerfile downloads the binaries from that release, and
  [source-distribution-$pin_date.json](source-distribution-$pin_date.json) binds every asset's URL, size
  and digest to it.
EOF

notices="$repo_root/THIRD_PARTY_NOTICES.md"
block="$work/notices-block.md"
cat >"$block" <<EOF
The \`ffmpeg\` binary is a **GPL-3.0-or-later** BtbN build. The [\`Dockerfile\`](Dockerfile) pins BtbN
autobuild \`$release\`, build \`$build_id\`, by per-architecture SHA256, and downloads both archives from
Loomarr's own source release [\`$tag\`]($release_url). That release also carries the exact FFmpeg source
(\`$source_commit\`), BtbN's build recipe (\`$recipe\`) and the build's original dependency download
cache. [\`source-distribution-$pin_date.json\`](project/evidence/source-distribution-$pin_date.json) binds
each asset's URL, size and SHA256, and the [pin record](project/evidence/ffmpeg-pin-$pin_date.md) gives the
provenance.
EOF
awk -v f="$block" '
  /^<!-- ffmpeg-pin:begin -->$/ { print; while ((getline line < f) > 0) print line; skip = 1; n++; next }
  /^<!-- ffmpeg-pin:end -->$/ { skip = 0 }
  !skip { print } END { exit n != 1 }' "$notices" >"$notices.tmp" || { rm -f "$notices.tmp"; die "THIRD_PARTY_NOTICES.md needs one ffmpeg-pin block"; }
mv -- "$notices.tmp" "$notices"
echo "pinned $build_id from $release; published $release_url" >&2
[[ "$automate" == true ]] || exit 0

# --- Automate: verify, push the pin branch, open the PR, dispatch full CI -------------------------
(cd "$repo_root" && go test ./internal/releaseverify/)
git -C "$repo_root" switch --create "$branch"
git -C "$repo_root" add Dockerfile THIRD_PARTY_NOTICES.md project/evidence
git -C "$repo_root" -c user.name='github-actions[bot]' \
  -c user.email='41898282+github-actions[bot]@users.noreply.github.com' \
  commit --quiet --no-verify -m "build(deps): ffmpeg $build_id" \
  -m "BtbN $release, mirrored with its corresponding source in $tag by scripts/ffmpeg-pin-refresh.sh."
git -C "$repo_root" push origin "HEAD:refs/heads/$branch"
head_sha="$(git -C "$repo_root" rev-parse HEAD)"
cat >"$work/pr-body.md" <<EOF
Automated FFmpeg pin refresh: \`$current_build\` → \`$build_id\` (BtbN \`$release\`).

The binaries and their corresponding source are published in [$tag]($release_url) and recorded in
\`project/evidence/\`. CI was dispatched with \`scope=full\`: it builds the image on amd64 and arm64 and
runs \`make test-ffmpeg\` against the image's own ffmpeg, including the software picture checks.

CI has no GPU: before merging, run the GPU picture checks in docs/contributing/releasing.md.
EOF
title="build(deps): ffmpeg $build_id"
flags=(--base main --head "$branch" --label dependencies --body-file "$work/pr-body.md")
if [[ "$series_change" == true ]]; then
  # renovate.json holds every major for explicit approval; a new FFmpeg series waits the same way.
  title="$title (new series $current_series → $series: needs approval)"
  flags+=(--draft --label ready-for-human)
fi
gh pr create --repo "$loomarr" --title "$title" "${flags[@]}"
# A push made with GITHUB_TOKEN triggers no workflow, so CI is dispatched at the branch.
gh workflow run ci.yml --repo "$loomarr" --ref "$branch" -f scope=full
for _ in $(seq 1 24); do
  run_url="$(gh run list --repo "$loomarr" --workflow ci.yml --branch "$branch" --event workflow_dispatch \
    --limit 20 --json headSha,url | jq -r --arg sha "$head_sha" 'map(select(.headSha == $sha)) | .[0].url // ""')"
  [[ -z "$run_url" ]] || { echo "CI for $head_sha: $run_url" >&2; exit 0; }
  sleep 5
done
die "CI was dispatched for $branch but no run for $head_sha appeared"
