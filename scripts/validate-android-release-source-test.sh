#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
temp_dir=$(mktemp -d)
trap 'rm -r -- "$temp_dir"' EXIT

mkdir -p "$temp_dir/bin"
ln -s "$script_dir/testdata/fake-android-release-git.sh" "$temp_dir/bin/git"
ln -s "$script_dir/testdata/fake-android-release-gh.sh" "$temp_dir/bin/gh"

head=0123456789abcdef0123456789abcdef01234567
android_job=$'Android TV — lint + unit + assemble\tsuccess\n'
clients_job=$'Shared clients — lint + test + browser/iOS/Android/TV bundles\tsuccess\n'

validate() {
	PATH="$temp_dir/bin:$PATH" \
		GITHUB_REPOSITORY=loomarr/loomarr \
		GITHUB_SHA="$head" \
		GITHUB_REF="${TEST_GITHUB_REF:-refs/heads/main}" \
		FAKE_GIT_HEAD="$head" \
		FAKE_GITHUB_JOBS="$TEST_GITHUB_JOBS" \
		LOOMARR_ANDROID_RENDERER="${TEST_RENDERER:-compose}" \
		"$script_dir/validate-android-release-source.sh"
}

TEST_GITHUB_JOBS=$android_job validate >/dev/null
TEST_RENDERER=react-native TEST_GITHUB_JOBS="$android_job$clients_job" validate >/dev/null

if TEST_RENDERER=react-native TEST_GITHUB_JOBS=$android_job validate 2>"$temp_dir/error"; then
	echo 'android release source test: React Native accepted CI without the shared-client gate' >&2
	exit 1
fi
grep -Fq 'React Native source did not execute the shared-client gate' "$temp_dir/error"

if TEST_GITHUB_JOBS=$clients_job validate 2>"$temp_dir/error"; then
	echo 'android release source test: accepted CI without the Android gate' >&2
	exit 1
fi
grep -Fq 'successful CI run did not execute the Android gate' "$temp_dir/error"

if TEST_GITHUB_REF=refs/heads/feature TEST_GITHUB_JOBS=$android_job validate 2>"$temp_dir/error"; then
	echo 'android release source test: accepted a non-main workflow ref' >&2
	exit 1
fi
grep -Fq 'workflow must run from the main branch' "$temp_dir/error"

echo 'android-release-source-test: ok'
