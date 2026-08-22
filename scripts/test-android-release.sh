#!/usr/bin/env bash

set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
temp_dir=$(mktemp -d)
trap 'rm -r -- "$temp_dir"' EXIT

password=loomarr-ephemeral-release-test
keystore="$temp_dir/upload.p12"
keytool \
	-genkeypair -noprompt \
	-storetype PKCS12 \
	-keystore "$keystore" \
	-storepass "$password" \
	-keypass "$password" \
	-alias loomarr-upload \
	-keyalg RSA \
	-keysize 4096 \
	-validity 1 \
	-dname 'CN=Loomarr Ephemeral Release Test,O=Loomarr Test,C=US' >/dev/null 2>&1
fingerprint=$(
	LC_ALL=C keytool -list -v -keystore "$keystore" -storepass "$password" -alias loomarr-upload |
		awk -F': ' '/SHA256:/{print $2; exit}'
)
version_name=0.1.0-beta.1
version_code=$("$script_dir/android-version-code.sh" "$version_name")

LOOMARR_ANDROID_VERSION_NAME=$version_name \
	LOOMARR_ANDROID_VERSION_CODE=$version_code \
	LOOMARR_ANDROID_KEYSTORE_PATH=$keystore \
	LOOMARR_ANDROID_KEYSTORE_PASSWORD=$password \
	LOOMARR_ANDROID_KEY_ALIAS=loomarr-upload \
	LOOMARR_ANDROID_KEY_PASSWORD=$password \
	LOOMARR_ANDROID_UPLOAD_CERT_SHA256=$fingerprint \
	ANDROID_RELEASE_OUTPUT_DIR="$temp_dir/output" \
	"$script_dir/build-android-beta.sh"

jq -e \
	--argjson code "$version_code" \
	'.package == "loomarr.media" and .versionName == "0.1.0-beta.1" and
	 .versionCode == $code and .nativeLibraries > 0 and
	 .abis == ["arm64-v8a", "armeabi-v7a", "x86", "x86_64"] and
	 .elfLoadAlignmentBytes == 16384' \
	"$temp_dir/output/loomarr-tv-$version_name-$version_code.json" >/dev/null

echo 'android release: ephemeral signed AAB passed identity, signature, ABI, and 16 KiB checks'
