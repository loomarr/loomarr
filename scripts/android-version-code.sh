#!/usr/bin/env sh

set -eu

decimal_component() {
	case "$1" in
		0 | [1-9] | [1-9][0-9]) return 0 ;;
		*) return 1 ;;
	esac
}

release_number() {
	case "$1" in
		0 | [1-9] | [1-9][0-9] | [1-9][0-9][0-9] | [1-7][0-9][0-9][0-9]) return 0 ;;
		*) return 1 ;;
	esac
}

rc_number() {
	case "$1" in
		[1-9] | [1-9][0-9] | [1-9][0-9][0-9]) return 0 ;;
		*) return 1 ;;
	esac
}

version_code() {
	version=$1
	case "$version" in
		*-*) base=${version%%-*}; suffix=${version#*-} ;;
		*) base=$version; suffix=stable ;;
	esac

	old_ifs=$IFS
	IFS=.
	# shellcheck disable=SC2086 # Splitting the three numeric SemVer components is intentional.
	set -- $base
	IFS=$old_ifs
	if [ "$#" -ne 3 ]; then
		return 1
	fi
	major=$1
	minor=$2
	patch=$3
	decimal_component "$major" || return 1
	decimal_component "$minor" || return 1
	decimal_component "$patch" || return 1
	[ "$major" -le 20 ] || return 1

	case "$suffix" in
		stable)
			channel=9999
			;;
		beta.*)
			number=${suffix#beta.}
			release_number "$number" || return 1
			[ "$number" -ge 1 ] || return 1
			channel=$number
			;;
		rc.*)
			number=${suffix#rc.}
			rc_number "$number" || return 1
			channel=$((8000 + number))
			;;
		*) return 1 ;;
	esac

	code=$((major * 100000000 + minor * 1000000 + patch * 10000 + channel))
	[ "$code" -gt 0 ] && [ "$code" -lt 2100000000 ] || return 1
	printf '%s\n' "$code"
}

self_test() {
	for pair in \
		'0.1.0-beta.1=1000001' \
		'0.1.0-beta.7999=1007999' \
		'0.1.0-rc.1=1008001' \
		'0.1.0-rc.999=1008999' \
		'0.1.0=1009999' \
		'20.99.99=2099999999'; do
		version=${pair%%=*}
		expected=${pair#*=}
		actual=$(version_code "$version")
		if [ "$actual" != "$expected" ]; then
			echo "android release: $version produced $actual, expected $expected" >&2
			exit 1
		fi
	done

	for version in \
		0.1 0.1.0-alpha.1 0.1.0-beta.0 0.1.0-beta.8000 0.1.0-rc.0 \
		0.1.0-rc.1000 00.1.0 0.01.0 0.1.00 21.0.0 1.100.0 1.0.100 1.0.0+build; do
		if version_code "$version" >/dev/null 2>&1; then
			echo "android release: accepted unsupported version $version" >&2
			exit 1
		fi
	done

	beta=$(version_code 0.1.0-beta.7999)
	rc=$(version_code 0.1.0-rc.1)
	stable=$(version_code 0.1.0)
	next_patch=$(version_code 0.1.1-beta.1)
	if ! [ "$beta" -lt "$rc" ] || ! [ "$rc" -lt "$stable" ] || ! [ "$stable" -lt "$next_patch" ]; then
		echo 'android release: version channels are not monotonic' >&2
		exit 1
	fi

	echo 'android release: SemVer names map to bounded monotonic Play version codes'
}

if [ "${1:-}" = "--self-test" ]; then
	self_test
	exit 0
fi

if [ "$#" -ne 1 ] || ! version_code "$1"; then
	echo 'usage: android-version-code.sh <major.minor.patch[-beta.N|-rc.N]>' >&2
	exit 1
fi
