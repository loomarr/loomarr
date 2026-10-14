#!/usr/bin/env sh

set -eu

missing_signature() {
	ref=$1
	error=$2
	[ "$error" = "ERROR: $ref: not found" ]
}

self_test() {
	ref=ghcr.io/loomarr/loomarr:0.1.0-beta.1
	if ! missing_signature "$ref" "ERROR: $ref: not found"; then
		echo 'release-verify: rejected the exact missing-manifest signature' >&2
		exit 1
	fi

	for error in \
		"ERROR: unexpected status from HEAD request to https://ghcr.io/v2/loomarr/loomarr/manifests/0.1.0-beta.1: 404 Not Found" \
		'ERROR: failed to authorize: 403 Forbidden' \
		'ERROR: unexpected status: 429 Too Many Requests' \
		'ERROR: failed to do request: dial tcp: network is unreachable'; do
		if missing_signature "$ref" "$error"; then
			echo "release-verify: accepted a non-manifest failure: $error" >&2
			exit 1
		fi
	done

	echo 'release-verify: image absence detection fails closed'
}

if [ "${1:-}" = "--self-test" ]; then
	self_test
	exit 0
fi

if [ "$#" -ne 1 ]; then
	echo 'usage: check-release-image-absence.sh <registry/image:tag>' >&2
	exit 2
fi

ref=$1
if inspect_error="$(docker buildx imagetools inspect "$ref" 2>&1)"; then
	echo "refusing to overwrite published image tag $ref" >&2
	exit 1
fi

# Buildx renders GHCR's MANIFEST_UNKNOWN response as exactly this ref-qualified
# line. Do not accept a generic 404: it could come from a proxy or registry
# service route rather than prove this manifest is absent.
if ! missing_signature "$ref" "$inspect_error"; then
	echo "could not prove image tag $ref is absent" >&2
	printf '%s\n' "$inspect_error" >&2
	exit 1
fi
