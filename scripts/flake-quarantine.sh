#!/usr/bin/env bash
# Resolve the flake quarantine (scripts/flake-quarantine.tsv) for one Playwright suite (#1570).
#
#   flake-quarantine.sh check           validate the manifest; no network
#   flake-quarantine.sh active SUITE    print "project<TAB>spec<TAB>title<TAB>issue" for each entry
#                                       of SUITE whose issue is OPEN
#
# Quarantine is the exception, so every doubt resolves toward the required job: an entry whose
# issue is closed, or whose state cannot be read, is not printed and its test runs as required.
# The manifest itself fails closed: a malformed line is an error, never a skipped line.
#
# FLAKE_QUARANTINE_FILE and FLAKE_QUARANTINE_GH override the manifest and the gh binary (tests).

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="${FLAKE_QUARANTINE_FILE:-$root/scripts/flake-quarantine.tsv}"
gh_bin="${FLAKE_QUARANTINE_GH:-gh}"

fail() {
	printf 'flake-quarantine: %s\n' "$*" >&2
	exit 2
}

# entries prints the manifest's validated entries as "suite<TAB>project<TAB>spec<TAB>title<TAB>issue".
entries() {
	[[ -f "$manifest" ]] || fail "manifest $manifest does not exist"
	local line number=0 suite project spec title issue extra
	declare -A seen=()
	while IFS= read -r line || [[ -n "$line" ]]; do
		number=$((number + 1))
		[[ -z "$line" || "$line" == \#* ]] && continue
		IFS=$'\t' read -r suite project spec title issue extra <<<"$line"
		[[ -z "${extra:-}" && -n "${issue:-}" ]] || fail "line $number: want 5 tab-separated fields: suite, project, spec, title, issue"
		[[ "$suite" == tuner ]] || fail "line $number: unknown suite '$suite' (want tuner)"
		case "$project" in
		chromium | firefox | webkit) ;;
		*) fail "line $number: unknown project '$project' (want chromium, firefox, or webkit)" ;;
		esac
		[[ "$spec" =~ ^[A-Za-z0-9._/-]+\.spec\.ts$ ]] || fail "line $number: '$spec' is not a spec file"
		[[ "$title" =~ ^[^[:space:]](.*[^[:space:]])?$ ]] || fail "line $number: the test title is empty or padded"
		[[ "$issue" =~ ^[1-9][0-9]*$ ]] || fail "line $number: issue '$issue' is not an issue number"
		[[ -z "${seen[$suite/$project/$spec/$title]:-}" ]] || fail "line $number: '$title' is already quarantined for $suite/$project"
		seen[$suite/$project/$spec/$title]=1
		printf '%s\t%s\t%s\t%s\t%s\n' "$suite" "$project" "$spec" "$title" "$issue"
	done <"$manifest"
}

case "${1:-}" in
check)
	[[ $# -eq 1 ]] || fail "usage: flake-quarantine.sh check"
	entries >/dev/null
	;;
active)
	[[ $# -eq 2 && -n "$2" ]] || fail "usage: flake-quarantine.sh active SUITE"
	want="$2"
	listed="$(entries)"
	while IFS=$'\t' read -r suite project spec title issue; do
		[[ -n "$suite" && "$suite" == "$want" ]] || continue
		if ! state="$("$gh_bin" issue view "$issue" --json state --jq .state 2>/dev/null)"; then
			printf '::warning::flake quarantine: could not read #%s, so %s "%s" runs in the required job\n' "$issue" "$project" "$title" >&2
			continue
		fi
		case "$state" in
		OPEN)
			printf '::notice::flake quarantine: %s "%s" runs non-blocking while #%s is open\n' "$project" "$title" "$issue" >&2
			printf '%s\t%s\t%s\t%s\n' "$project" "$spec" "$title" "$issue"
			;;
		*)
			printf '::notice::flake quarantine: #%s is %s, so %s "%s" is back in the required job; remove its line\n' "$issue" "${state:-unreadable}" "$project" "$title" >&2
			;;
		esac
	done <<<"$listed"
	;;
*)
	fail "usage: flake-quarantine.sh check | active SUITE"
	;;
esac
