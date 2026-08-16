#!/usr/bin/env sh

set -eu

# SemVer 2.0.0, with the repository's required leading v. Numeric prerelease
# identifiers may not have leading zeroes. Build metadata is valid SemVer but is
# rejected separately because '+' is not representable in an OCI tag.
SEMVER_RE='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*)|([0-9]*[A-Za-z-][0-9A-Za-z-]*))(\.((0|[1-9][0-9]*)|([0-9]*[A-Za-z-][0-9A-Za-z-]*)))*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'

valid_tag() {
	printf '%s\n' "$1" | grep -Eq "$SEMVER_RE" || return 1
	case "$1" in
		*+*) return 1 ;;
	esac
}

self_test() {
	for tag in v0.1.0 v1.2.3 v1.2.3-beta.1 v1.0.0-0.3.7 v1.0.0-x.7.z.92; do
		if ! valid_tag "$tag"; then
			echo "release-verify: rejected valid tag $tag" >&2
			exit 1
		fi
	done

	for tag in 1.2.3 v01.2.3 v1.02.3 v1.2.03 v1.2 v1.2.3-01 v1.2.3-alpha..1 v1.2.3-alpha. v1.2.3+build.1; do
		if valid_tag "$tag"; then
			echo "release-verify: accepted unsupported tag $tag" >&2
			exit 1
		fi
	done

	echo 'release-verify: SemVer and OCI tag policy are enforced'
}

if [ "${1:-}" = "--self-test" ]; then
	self_test
	exit 0
fi

if [ "$#" -ne 1 ] || ! valid_tag "$1"; then
	echo "release tag must be strict SemVer with a leading v and no build metadata: ${1:-<missing>}" >&2
	exit 1
fi
