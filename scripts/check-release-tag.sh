#!/usr/bin/env sh

set -eu

# SemVer 2.0.0 without the Git tag's leading v. Numeric prerelease identifiers
# may not have leading zeroes. Build metadata is valid SemVer but is rejected
# separately because '+' is not representable in an OCI tag.
SEMVER_RE='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*)|([0-9]*[A-Za-z-][0-9A-Za-z-]*))(\.((0|[1-9][0-9]*)|([0-9]*[A-Za-z-][0-9A-Za-z-]*)))*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'

valid_image_tag() {
	printf '%s\n' "$1" | grep -Eq "$SEMVER_RE" || return 1
	case "$1" in
		*+*) return 1 ;;
	esac
	# OCI distribution limits a tag to 128 bytes. Force the C locale so this
	# remains a byte count if a future caller passes non-ASCII input.
	[ "$(LC_ALL=C printf %s "$1" | wc -c | tr -d ' ')" -le 128 ]
}

valid_release_tag() {
	case "$1" in
		v*) valid_image_tag "${1#v}" ;;
		*) return 1 ;;
	esac
}

self_test() {
	for tag in v0.1.0 v1.2.3 v1.2.3-beta.1 v1.0.0-0.3.7 v1.0.0-x.7.z.92; do
		if ! valid_release_tag "$tag"; then
			echo "release-verify: rejected valid tag $tag" >&2
			exit 1
		fi
	done

	for tag in 1.2.3 v01.2.3 v1.02.3 v1.2.03 v1.2 v1.2.3-01 v1.2.3-alpha..1 v1.2.3-alpha. v1.2.3+build.1; do
		if valid_release_tag "$tag"; then
			echo "release-verify: accepted unsupported tag $tag" >&2
			exit 1
		fi
	done

	identifier_122="$(awk 'BEGIN { for (i = 0; i < 122; i++) printf "a" }')"
	identifier_123="${identifier_122}a"
	if ! valid_release_tag "v1.2.3-${identifier_122}"; then
		echo 'release-verify: rejected the 128-byte OCI tag boundary' >&2
		exit 1
	fi
	if valid_release_tag "v1.2.3-${identifier_123}"; then
		echo 'release-verify: accepted an OCI tag over 128 bytes' >&2
		exit 1
	fi

	echo 'release-verify: SemVer and OCI tag policy are enforced'
}

if [ "${1:-}" = "--self-test" ]; then
	self_test
	exit 0
fi

if [ "${1:-}" = "--image-tag" ]; then
	if [ "$#" -ne 2 ] || ! valid_image_tag "$2"; then
		echo "image tag must be strict SemVer, no leading v or build metadata, and at most 128 bytes: ${2:-<missing>}" >&2
		exit 1
	fi
	exit 0
fi

if [ "$#" -ne 1 ] || ! valid_release_tag "$1"; then
	echo "release tag must be strict SemVer with a leading v, no build metadata, and an OCI tag of at most 128 bytes: ${1:-<missing>}" >&2
	exit 1
fi
