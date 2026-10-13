#!/usr/bin/env bash
set -euo pipefail

# Fails a release whose rendered notes contain a user-facing change with no matching docs
# change in the same release range (#1682, maintainer decision D3, step 5 of #1572).
#
# "User-facing" is NOT a new taxonomy: it reuses the exact seven-category classification
# scripts/generate-release-notes.sh already assigns every merged PR to (internal/releasenotes).
# Four of those categories ship visible behaviour; the other three do not:
#
#   user-facing:     New Features, Improvements, Bug Fixes, Security Fixes
#   not user-facing: Documentation, Dependencies, Maintenance
#
# This script never re-classifies anything itself — it reads the headings already rendered by
# `make release-notes-preview` (cmd/release-notes) so the two can never disagree about what
# counts as user-facing.

USER_FACING_HEADINGS=(
	"## 🆕 New Features"
	"## ✨ Improvements"
	"## 🐞 Bug Fixes"
	"## 🔐 Security Fixes"
)

usage() {
	echo "usage: check-release-docs-gate.sh --self-test" >&2
	echo "       check-release-docs-gate.sh <rendered-notes-file> <changed-paths-file>" >&2
}

has_user_facing_change() {
	local notes_file=$1 heading
	for heading in "${USER_FACING_HEADINGS[@]}"; do
		grep -qF "$heading" "$notes_file" && return 0
	done
	return 1
}

has_docs_change() {
	# A release counts as documented if it touches the published docs site, the help set it
	# embeds, or the README a new user reads first. One match anywhere in the range is enough —
	# this gate does not try to match a specific PR to a specific doc.
	local paths_file=$1
	grep -Eq '^(README\.md$|docs/|docs-site/)' "$paths_file"
}

check() {
	local notes_file=$1 paths_file=$2
	[[ -f "$notes_file" ]] || { echo "check-release-docs-gate: no such notes file: $notes_file" >&2; return 2; }
	[[ -f "$paths_file" ]] || { echo "check-release-docs-gate: no such changed-paths file: $paths_file" >&2; return 2; }
	if has_user_facing_change "$notes_file" && ! has_docs_change "$paths_file"; then
		cat >&2 <<'EOF'
check-release-docs-gate: this release has user-facing changes (New Features, Improvements,
Bug Fixes, or Security Fixes in the release notes) but no change under docs/, docs-site/, or
README.md anywhere in the release range.

Either add the missing docs change, or if every user-facing PR title was miscategorized,
re-run `make release-notes-preview` after fixing the PR titles/labels that misled the
classifier — this gate trusts the same classification the release notes already show.
EOF
		return 1
	fi
	echo "check-release-docs-gate: ok"
}

self_test() {
	local tmp
	tmp=$(mktemp -d)
	trap 'rm -rf -- "$tmp"' RETURN

	# 1. User-facing change, with a docs change in range: passes.
	printf '## 🆕 New Features\n\n* Add channel priority https://github.com/loomarr/loomarr/pull/1\n' >"$tmp/notes-with-docs.md"
	printf 'internal/api/channels.go\ndocs/guides/add-a-channel.md\n' >"$tmp/paths-with-docs.txt"
	if ! check "$tmp/notes-with-docs.md" "$tmp/paths-with-docs.txt" >/dev/null; then
		echo "check-release-docs-gate: self-test: expected a docs change to pass" >&2
		exit 1
	fi

	# 2. User-facing change, with NO docs change in range: fails.
	printf 'internal/api/channels.go\ninternal/store/migrations/0099.sql\n' >"$tmp/paths-without-docs.txt"
	if check "$tmp/notes-with-docs.md" "$tmp/paths-without-docs.txt" >/dev/null 2>&1; then
		echo "check-release-docs-gate: self-test: expected a missing docs change to fail" >&2
		exit 1
	fi

	# 3. Only non-user-facing categories, no docs change: passes — nothing to document.
	printf '## 📦 Dependencies\n\n* Bump go.mod deps https://github.com/loomarr/loomarr/pull/2\n\n## 🧰 Maintenance\n\n* Refactor internals https://github.com/loomarr/loomarr/pull/3\n' >"$tmp/notes-maintenance-only.md"
	if ! check "$tmp/notes-maintenance-only.md" "$tmp/paths-without-docs.txt" >/dev/null; then
		echo "check-release-docs-gate: self-test: expected maintenance-only notes to pass without docs" >&2
		exit 1
	fi

	# 4. README-only docs change still counts.
	printf 'README.md\n' >"$tmp/paths-readme-only.txt"
	if ! check "$tmp/notes-with-docs.md" "$tmp/paths-readme-only.txt" >/dev/null; then
		echo "check-release-docs-gate: self-test: expected a README-only change to count as docs" >&2
		exit 1
	fi

	# 5. Missing input files are a usage error (exit 2), not a silent pass.
	local status
	set +e
	check "$tmp/does-not-exist.md" "$tmp/paths-with-docs.txt" >/dev/null 2>&1
	status=$?
	set -e
	if [[ "$status" -ne 2 ]]; then
		echo "check-release-docs-gate: self-test: expected a missing notes file to exit 2, got $status" >&2
		exit 1
	fi

	echo "check-release-docs-gate: self-test ok"
}

case "${1:-}" in
--self-test)
	self_test
	;;
"" | --help | -h)
	usage
	exit 2
	;;
*)
	[[ $# -eq 2 ]] || { usage; exit 2; }
	check "$1" "$2"
	exit $?
	;;
esac
