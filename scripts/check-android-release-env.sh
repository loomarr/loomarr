#!/usr/bin/env sh

set -eu

missing=
for name in \
	LOOMARR_ANDROID_VERSION_NAME \
	LOOMARR_ANDROID_VERSION_CODE \
	LOOMARR_ANDROID_KEYSTORE_PATH \
	LOOMARR_ANDROID_KEYSTORE_PASSWORD \
	LOOMARR_ANDROID_KEY_ALIAS \
	LOOMARR_ANDROID_KEY_PASSWORD; do
	eval "value=\${$name:-}"
	if [ -z "$value" ]; then
		missing="${missing}${missing:+, }$name"
	fi
done

if [ -n "$missing" ]; then
	echo "android release: missing required environment variables: $missing" >&2
	exit 1
fi

case "$LOOMARR_ANDROID_VERSION_CODE" in
	*[!0-9]* | '')
		echo 'android release: LOOMARR_ANDROID_VERSION_CODE must be a positive decimal integer' >&2
		exit 1
		;;
esac
if [ "$LOOMARR_ANDROID_VERSION_CODE" -lt 1 ] || [ "$LOOMARR_ANDROID_VERSION_CODE" -ge 2100000000 ]; then
	echo 'android release: LOOMARR_ANDROID_VERSION_CODE must be between 1 and 2099999999' >&2
	exit 1
fi

expected_code=$("$(dirname "$0")/android-version-code.sh" "$LOOMARR_ANDROID_VERSION_NAME") || {
	echo 'android release: LOOMARR_ANDROID_VERSION_NAME is not a supported release name' >&2
	exit 1
}
if [ "$LOOMARR_ANDROID_VERSION_CODE" != "$expected_code" ]; then
	echo "android release: version code must be $expected_code for $LOOMARR_ANDROID_VERSION_NAME" >&2
	exit 1
fi

if [ ! -f "$LOOMARR_ANDROID_KEYSTORE_PATH" ]; then
	echo 'android release: LOOMARR_ANDROID_KEYSTORE_PATH must name an existing file' >&2
	exit 1
fi

# The :env forms keep both passwords out of argv and process listings.
keytool \
	-list \
	-keystore "$LOOMARR_ANDROID_KEYSTORE_PATH" \
	-storepass:env LOOMARR_ANDROID_KEYSTORE_PASSWORD \
	-keypass:env LOOMARR_ANDROID_KEY_PASSWORD \
	-alias "$LOOMARR_ANDROID_KEY_ALIAS" >/dev/null

echo "android release: version $LOOMARR_ANDROID_VERSION_NAME ($LOOMARR_ANDROID_VERSION_CODE) and upload key are valid"
