#!/usr/bin/env sh

set -eu

required_env() {
	name=$1
	eval "value=\${$name:-}"
	if [ -z "$value" ]; then
		echo "release publication requires $name" >&2
		exit 2
	fi
}

# GITHUB_SHA and GITHUB_REPOSITORY are set by Actions in every step: the tagged commit and the
# repository the image's provenance must name (#661).
for name in IMAGE DIGEST RELEASE_TAG PUBLISH_LATEST COSIGN_CERTIFICATE_IDENTITY COSIGN_CERTIFICATE_OIDC_ISSUER GITHUB_SHA GITHUB_REPOSITORY; do
	required_env "$name"
done
case "$GITHUB_SHA" in
	*[!0-9a-f]*) echo "GITHUB_SHA must be a full lowercase commit id" >&2; exit 2 ;;
esac
if [ "${#GITHUB_SHA}" -ne 40 ]; then
	echo "GITHUB_SHA must be a full lowercase commit id" >&2
	exit 2
fi

case "$IMAGE" in
	*@*|*:*|*/*/*/*) echo "IMAGE must be an untagged registry/repository reference: $IMAGE" >&2; exit 2 ;;
esac
case "$DIGEST" in
	sha256:[0-9a-f][0-9a-f]*) ;;
	*) echo "DIGEST must be a sha256 digest" >&2; exit 2 ;;
esac
if [ "${#DIGEST}" -ne 71 ]; then
	echo "DIGEST must contain exactly 64 lowercase hex characters" >&2
	exit 2
fi
case "${DIGEST#sha256:}" in
	*[!0-9a-f]*) echo "DIGEST must contain exactly 64 lowercase hex characters" >&2; exit 2 ;;
esac
case "$PUBLISH_LATEST" in
	true|false) ;;
	*) echo "PUBLISH_LATEST must be true or false" >&2; exit 2 ;;
esac

version=${RELEASE_TAG#v}
if [ "$version" = "$RELEASE_TAG" ]; then
	echo "RELEASE_TAG must start with v" >&2
	exit 2
fi
./scripts/check-release-tag.sh "$RELEASE_TAG"

digest_ref="${IMAGE}@${DIGEST}"
version_ref="${IMAGE}:${version}"

inspect_digest() {
	ref=$1
	want_digest=$2

	inspect=$(docker buildx imagetools inspect "$ref")
	got_digest=$(printf '%s\n' "$inspect" | awk '/^Digest:[[:space:]]+sha256:/ { print $2 }')
	if [ "$got_digest" != "$want_digest" ]; then
		echo "release manifest $ref resolved to ${got_digest:-<missing>}, want $want_digest" >&2
		exit 1
	fi

	raw=$(docker buildx imagetools inspect --raw "$ref")
	if ! printf '%s\n' "$raw" | jq -e '
		.schemaVersion == 2 and
		(.mediaType == "application/vnd.oci.image.index.v1+json" or
		 .mediaType == "application/vnd.docker.distribution.manifest.list.v2+json") and
		([.manifests[] |
		  select(.annotations["vnd.docker.reference.type"] != "attestation-manifest") |
		  (.platform.os + "/" + .platform.architecture +
		   (if (.platform.variant // "") == "" then "" else "/" + .platform.variant end))] | sort) ==
		["linux/amd64", "linux/arm64"] and
		all(.manifests[];
		  if .annotations["vnd.docker.reference.type"] == "attestation-manifest"
		  then .platform.os == "unknown" and .platform.architecture == "unknown"
		  else true end)
	' >/dev/null; then
		echo "release manifest $ref does not contain exactly linux/amd64 and linux/arm64 application images" >&2
		exit 1
	fi
}

# verify_release_evidence requires, for BOTH platform images, a non-empty SPDX SBOM and a build
# provenance whose every recorded source revision is the tagged commit of this repository (#661).
# That binding is what ties the published image to the exact release source, and through it to
# the third-party source releases its notices name; an image without it is not published.
verify_release_evidence() {
	ref=$1
	sbom=$(docker buildx imagetools inspect "$ref" --format '{{ json .SBOM }}')
	if ! printf '%s\n' "$sbom" | jq -e '
		(keys == ["linux/amd64", "linux/arm64"]) and
		all(.[]; (.SPDX.spdxVersion // "" | startswith("SPDX-")) and ((.SPDX.packages // []) | length > 0))
	' >/dev/null; then
		echo "release manifest $ref does not carry a non-empty SPDX SBOM for both linux/amd64 and linux/arm64" >&2
		exit 1
	fi
	provenance=$(docker buildx imagetools inspect "$ref" --format '{{ json .Provenance }}')
	if ! printf '%s\n' "$provenance" | jq -e --arg rev "$GITHUB_SHA" --arg src "https://github.com/$GITHUB_REPOSITORY" '
		(keys == ["linux/amd64", "linux/arm64"]) and
		all(.[];
		  ([.. | objects | .["vcs:revision"]? // empty] | unique) == [$rev] and
		  ([.. | objects | .["vcs:source"]? // empty] | unique) == [$src])
	' >/dev/null; then
		echo "release manifest $ref does not carry build provenance naming $GITHUB_REPOSITORY at $GITHUB_SHA for both linux/amd64 and linux/arm64" >&2
		exit 1
	fi
}

# Re-check immediately before any publication. A workflow-level check alone leaves
# a time-of-check/time-of-use window if a tag is raced into GHCR while the image builds.
./scripts/check-release-image-absence.sh "$version_ref"
inspect_digest "$digest_ref" "$DIGEST"
verify_release_evidence "$digest_ref"

cosign sign --yes "$digest_ref"
cosign verify \
	--certificate-identity "$COSIGN_CERTIFICATE_IDENTITY" \
	--certificate-oidc-issuer "$COSIGN_CERTIFICATE_OIDC_ISSUER" \
	"$digest_ref" >/dev/null

# Public names do not exist until both identity-bound operations above succeed.
set -- --tag "$version_ref"
if [ "$PUBLISH_LATEST" = true ]; then
	set -- "$@" --tag "${IMAGE}:latest"
fi
docker buildx imagetools create "$@" "$digest_ref"

# Promotion is not success until every public name resolves back to the signed
# digest and retains the exact application-platform set.
inspect_digest "$version_ref" "$DIGEST"
if [ "$PUBLISH_LATEST" = true ]; then
	inspect_digest "${IMAGE}:latest" "$DIGEST"
fi
