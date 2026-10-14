#!/usr/bin/env bash
# Merge the per-architecture image digests (built natively by the release build matrix and
# downloaded as artifacts) into ONE multi-arch index and emit that index's digest as a GitHub step
# output (`digest`) for the publish helper to sign.
#
# The publish helper (publish-release-image.sh) asserts the index holds exactly linux/amd64 +
# linux/arm64 before it signs, so a missing or malformed arch fails closed there. This script also
# refuses to proceed unless BOTH expected arch digests are present and well-formed, so a dropped
# build leg is caught before a one-arch index is ever pushed.
#
# NO PUBLIC TAG is created here. imagetools needs a reference to push an index, so the index is
# pushed under a build-metadata tag derived from the immutable digest (`sha256-<hex>.index`) — an
# internal, non-SemVer, non-`latest` name that the publish helper never promotes. The helper works
# from the returned digest, and re-tags the public SemVer/latest names from it only after signing.
set -eu

: "${IMAGE:?merge requires IMAGE}"
: "${GITHUB_OUTPUT:?merge requires GITHUB_OUTPUT}"
digests_dir="${DIGESTS_DIR:-/tmp/digests}"

case "$IMAGE" in
	*@*|*:*|*/*/*/*) echo "IMAGE must be an untagged registry/repository reference: $IMAGE" >&2; exit 2 ;;
esac

validate_digest() {
	d=$1
	arch=$2
	case "$d" in
		sha256:[0-9a-f]*) ;;
		*) echo "digest for ${arch} is not a sha256 digest: ${d}" >&2; exit 1 ;;
	esac
	if [ "${#d}" -ne 71 ]; then
		echo "digest for ${arch} must be sha256 + 64 hex chars: ${d}" >&2
		exit 1
	fi
	case "${d#sha256:}" in
		*[!0-9a-f]*) echo "digest for ${arch} must be 64 lowercase hex chars: ${d}" >&2; exit 1 ;;
	esac
}

refs=""
first_digest=""
for arch in amd64 arm64; do
	file="${digests_dir}/${arch}"
	if [ ! -f "$file" ]; then
		echo "missing digest for ${arch}: ${file} (a build leg did not upload its digest)" >&2
		exit 1
	fi
	digest=$(cat "$file")
	validate_digest "$digest" "$arch"
	[ -n "$first_digest" ] || first_digest="$digest"
	refs="${refs} ${IMAGE}@${digest}"
done

# An internal build-metadata tag: content-addressed, so it is stable and collision-free, and it is
# neither a SemVer version nor `latest`, so the publish helper's absence check and promotion never
# touch it. It exists only so imagetools has a name to push the assembled index under; we then read
# the index's own digest back off it.
index_tag="${IMAGE}:${first_digest#sha256:}.index"

# shellcheck disable=SC2086 # refs is an intentional word list of two fully-validated digest refs.
docker buildx imagetools create --tag "$index_tag" $refs

# Read the digest the registry assigned to the pushed index.
inspect=$(docker buildx imagetools inspect "$index_tag")
index_digest=$(printf '%s\n' "$inspect" | awk '/^Digest:[[:space:]]+sha256:/ { print $2; exit }')
validate_digest "$index_digest" "index"

echo "merged index digest: $index_digest" >&2
echo "digest=$index_digest" >> "$GITHUB_OUTPUT"
