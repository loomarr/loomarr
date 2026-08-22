#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "$script_dir/.." && pwd)
android_root="$repo_root/android"
output_dir=${ANDROID_RELEASE_OUTPUT_DIR:-"$repo_root/.artifacts/android-release"}
package_name=loomarr.media

"$script_dir/check-android-release-env.sh"

for command in jq unzip readelf keytool jarsigner sha256sum; do
	if ! command -v "$command" >/dev/null 2>&1; then
		echo "android release: required command is unavailable: $command" >&2
		exit 1
	fi
done

normalize_fingerprint() {
	tr -d ':[:space:]' | tr '[:lower:]' '[:upper:]'
}

expected_fingerprint=$(printf '%s' "${LOOMARR_ANDROID_UPLOAD_CERT_SHA256:-}" | normalize_fingerprint)
if [[ ! "$expected_fingerprint" =~ ^[0-9A-F]{64}$ ]]; then
	echo 'android release: LOOMARR_ANDROID_UPLOAD_CERT_SHA256 must be a SHA-256 certificate fingerprint' >&2
	exit 1
fi

keystore_fingerprint=$(
	LC_ALL=C keytool \
		-list -v \
		-keystore "$LOOMARR_ANDROID_KEYSTORE_PATH" \
		-storepass:env LOOMARR_ANDROID_KEYSTORE_PASSWORD \
		-keypass:env LOOMARR_ANDROID_KEY_PASSWORD \
		-alias "$LOOMARR_ANDROID_KEY_ALIAS" |
		awk -F': ' '/SHA256:/{print $2; exit}' |
		normalize_fingerprint
)
if [[ "$keystore_fingerprint" != "$expected_fingerprint" ]]; then
	echo 'android release: upload keystore certificate does not match the protected fingerprint' >&2
	exit 1
fi

(
	cd "$android_root"
	./gradlew --no-configuration-cache clean :app:bundleRelease
)

source_aab="$android_root/app/build/outputs/bundle/release/app-release.aab"
metadata="$android_root/app/build/intermediates/merged_manifests/release/processReleaseManifest/output-metadata.json"
if [[ ! -f "$source_aab" || ! -f "$metadata" ]]; then
	echo 'android release: Gradle did not produce the expected bundle and manifest metadata' >&2
	exit 1
fi

jq -e \
	--arg package "$package_name" \
	--arg name "$LOOMARR_ANDROID_VERSION_NAME" \
	--argjson code "$LOOMARR_ANDROID_VERSION_CODE" \
	'.applicationId == $package and .variantName == "release" and
	 (.elements | length) == 1 and .elements[0].versionName == $name and
	 .elements[0].versionCode == $code' \
	"$metadata" >/dev/null || {
	echo 'android release: generated manifest identity does not match the requested release' >&2
	exit 1
}

LC_ALL=C jarsigner -verify "$source_aab" >/dev/null
bundle_fingerprint=$(
	LC_ALL=C keytool -printcert -jarfile "$source_aab" |
		awk -F': ' '/SHA256:/{print $2; exit}' |
		normalize_fingerprint
)
if [[ "$bundle_fingerprint" != "$expected_fingerprint" ]]; then
	echo 'android release: signed bundle certificate does not match the protected fingerprint' >&2
	exit 1
fi

inspection_dir=$(mktemp -d)
trap 'rm -r -- "$inspection_dir"' EXIT
unzip -q "$source_aab" 'base/lib/*/*.so' -d "$inspection_dir"

mapfile -t libraries < <(find "$inspection_dir/base/lib" -type f -name '*.so' -print | sort)
if ((${#libraries[@]} == 0)); then
	echo 'android release: bundle inspection found no native libraries' >&2
	exit 1
fi
for abi in arm64-v8a armeabi-v7a x86 x86_64; do
	if [[ ! -d "$inspection_dir/base/lib/$abi" ]]; then
		echo "android release: bundle is missing required ABI $abi" >&2
		exit 1
	fi
done
for library in "${libraries[@]}"; do
	mapfile -t alignments < <(readelf -lW "$library" | awk '$1 == "LOAD" {print $NF}')
	if ((${#alignments[@]} == 0)); then
		echo "android release: no ELF load segments found in ${library#"$inspection_dir/"}" >&2
		exit 1
	fi
	for alignment in "${alignments[@]}"; do
		if ((alignment < 0x4000)); then
			echo "android release: ${library#"$inspection_dir/"} has LOAD alignment $alignment below 16 KiB" >&2
			exit 1
		fi
	done
done

mkdir -p "$output_dir"
artifact_stem="loomarr-tv-${LOOMARR_ANDROID_VERSION_NAME}-${LOOMARR_ANDROID_VERSION_CODE}"
output_aab="$output_dir/$artifact_stem.aab"
output_manifest="$output_dir/$artifact_stem.json"
install -m 0644 "$source_aab" "$output_aab"
bundle_sha256=$(sha256sum "$output_aab" | awk '{print $1}')

jq -n \
	--arg package "$package_name" \
	--arg versionName "$LOOMARR_ANDROID_VERSION_NAME" \
	--argjson versionCode "$LOOMARR_ANDROID_VERSION_CODE" \
	--arg commit "$(git -C "$repo_root" rev-parse HEAD)" \
	--arg uploadCertificateSha256 "$bundle_fingerprint" \
	--arg aabSha256 "$bundle_sha256" \
	--argjson nativeLibraries "${#libraries[@]}" \
	'{
	  package: $package,
	  versionName: $versionName,
	  versionCode: $versionCode,
	  commit: $commit,
	  uploadCertificateSha256: $uploadCertificateSha256,
	  aabSha256: $aabSha256,
	  nativeLibraries: $nativeLibraries,
	  abis: ["arm64-v8a", "armeabi-v7a", "x86", "x86_64"],
	  elfLoadAlignmentBytes: 16384
	}' >"$output_manifest"

echo "android release: built and verified $output_aab"
echo "android release: evidence $output_manifest"
