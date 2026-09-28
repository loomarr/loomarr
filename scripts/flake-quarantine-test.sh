#!/usr/bin/env bash
# Contract for scripts/flake-quarantine.sh: only an OPEN issue quarantines a test; a closed or
# unreadable issue puts it back in the required job; a malformed manifest is an error.

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
quarantine="$root/scripts/flake-quarantine.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

fail() {
	printf 'flake-quarantine-test: %s\n' "$*" >&2
	exit 1
}

# A fake gh: issue 1 is open, 2 is closed, 3 cannot be read.
cat >"$work/gh" <<'EOF'
#!/usr/bin/env bash
[[ "$1 $2 $4 $5 $6" == "issue view --json state --jq" ]] || exit 9
case "$3" in
1) echo OPEN ;;
2) echo CLOSED ;;
*) echo "GraphQL: Could not resolve to an issue" >&2; exit 1 ;;
esac
EOF
chmod +x "$work/gh"
export FLAKE_QUARANTINE_GH="$work/gh"

tab=$'\t'
manifest() {
	printf '%s\n' "$@" >"$work/manifest.tsv"
	export FLAKE_QUARANTINE_FILE="$work/manifest.tsv"
}

# The tuner job runs the script on macOS, whose /bin/bash is 3.2, while this contract runs on Linux's
# bash 5; a bash-4-only construct passes here and fails there (#1735's first merge-group run).
if grep -nE '(declare|local|typeset|readonly)[[:space:]]+-[[:alpha:]]*A|mapfile|readarray|coproc|\$\{[[:alnum:]_]+(,,?|\^\^?)[}]|\|&|&>>' "$quarantine"; then
	fail "the script uses a construct bash 3.2 lacks"
fi

# The committed manifest is valid.
env -u FLAKE_QUARANTINE_FILE "$quarantine" check || fail "the committed manifest does not validate"

manifest '# comment' '' \
	"tuner${tab}webkit${tab}tuner-surf.spec.ts${tab}open test${tab}1" \
	"tuner${tab}firefox${tab}tuner-surf.spec.ts${tab}closed test${tab}2" \
	"tuner${tab}chromium${tab}tuner-surf.spec.ts${tab}unreadable test${tab}3"
got="$("$quarantine" active tuner 2>"$work/log")"
[[ "$got" == "webkit${tab}tuner-surf.spec.ts${tab}open test${tab}1" ]] ||
	fail "active tuner = $(printf %q "$got"), want only the open issue's test"
grep -q '^::notice::.*#2 is CLOSED' "$work/log" || fail "a closed issue's rejoin is not announced"
grep -q '^::warning::.*could not read #3' "$work/log" || fail "an unreadable issue is not a warning"
[[ -z "$("$quarantine" active other 2>/dev/null)" ]] || fail "another suite's entries leaked"

reject() {
	local name="$1"
	shift
	manifest "$@"
	if "$quarantine" check >/dev/null 2>&1 || "$quarantine" active tuner >/dev/null 2>&1; then
		fail "$name was accepted but must fail closed"
	fi
}
reject 'a missing issue' "tuner${tab}webkit${tab}tuner-surf.spec.ts${tab}t"
reject 'an extra field' "tuner${tab}webkit${tab}tuner-surf.spec.ts${tab}t${tab}1${tab}x"
reject 'a non-numeric issue' "tuner${tab}webkit${tab}tuner-surf.spec.ts${tab}t${tab}#1"
reject 'an unknown project' "tuner${tab}safari${tab}tuner-surf.spec.ts${tab}t${tab}1"
reject 'an unknown suite' "e2e${tab}webkit${tab}tuner-surf.spec.ts${tab}t${tab}1"
reject 'a non-spec file' "tuner${tab}webkit${tab}tuner-backend.ts${tab}t${tab}1"
reject 'a padded title' "tuner${tab}webkit${tab}tuner-surf.spec.ts${tab} t${tab}1"
reject 'a space-separated line' "tuner webkit tuner-surf.spec.ts t 1"
reject 'a duplicate' "tuner${tab}webkit${tab}tuner-surf.spec.ts${tab}t${tab}1" "tuner${tab}webkit${tab}tuner-surf.spec.ts${tab}t${tab}2"
FLAKE_QUARANTINE_FILE="$work/missing.tsv" "$quarantine" check >/dev/null 2>&1 && fail "a missing manifest was accepted"
"$quarantine" bogus >/dev/null 2>&1 && fail "an unknown command was accepted"

echo 'flake-quarantine-test: ok'
