#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLASSIFIER="$SCRIPT_DIR/codeql-impact.sh"

assert_language() {
	local output="$1" language="$2" want="$3"
	grep -qx "$language=$want" <<<"$output" || {
		echo "codeql-impact-test: $language = $want not found in [$output]" >&2
		exit 1
	}
}

docs="$(printf '%s\n' docs/dev/ci.md scripts/test-affected.sh | "$CLASSIFIER")"
for language in actions go javascript-typescript python ruby rust; do
	assert_language "$docs" "$language" false
done

mixed="$(printf '%s\n' internal/api/server.go web/apps/web/src/App.tsx web/scripts/apple-launch-diagnostics.py web/scripts/hermes-download-test.rb rust/loomarr-image/src/lib.rs .github/workflows/ci.yml | "$CLASSIFIER")"
for language in actions go javascript-typescript python ruby rust; do
	assert_language "$mixed" "$language" true
done

go_only="$($CLASSIFIER go.mod)"
assert_language "$go_only" go true
assert_language "$go_only" rust false

control="$($CLASSIFIER scripts/codeql-impact.sh)"
for language in actions go javascript-typescript python ruby rust; do
	assert_language "$control" "$language" true
done

unknown="$($CLASSIFIER future-language/source.quux 2>/dev/null)"
for language in actions go javascript-typescript python ruby rust; do
	assert_language "$unknown" "$language" true
done

full="$($CLASSIFIER --all)"
for language in actions go javascript-typescript python ruby rust; do
	assert_language "$full" "$language" true
done

echo 'codeql-impact-test: ok'
