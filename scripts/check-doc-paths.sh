#!/usr/bin/env bash
# check-doc-paths.sh — every repo path a live doc names in inline code must exist.
#
# lychee checks links, but most of our docs name files as inline code (`internal/api/help.go`),
# and nothing checked those. That gap is how 13 references to moved or deleted paths built up
# before #1729 fixed them (#1718). This check reads the same live docs, finds each inline-code
# span that starts with a top-level repo directory, and fails for one that names nothing.
#
# A span resolves when its path exists from the repo root, from the doc's own directory, or from
# docs/ (docs/design.md writes `design/overview.md` for docs/design/overview.md). Spans that are
# patterns (`*`, `{}`, `<...>`, `$VAR`), gitignored build output (`storybook-static`) and Go
# symbols (`internal/metrics.Recorder`, checked as the package directory) are not paths to
# check. A `:12` or `:12-34` line suffix and a `#fragment` are stripped first.
#
# History is out of scope: PROGRESS.md, project/ and design/ record what paths WERE. A live doc
# that must name a missing path on purpose (a retirement note, a path a future change creates)
# lists it in scripts/check-doc-paths.allow with the reason.
#
# Usage: scripts/check-doc-paths.sh   (from anywhere in the repo; exit 1 on a dead path)
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"
allow_file="scripts/check-doc-paths.allow"

# Top-level directories, so a span like `web/apps/web` is recognized as a path and prose like
# `on/off` is not.
tops="$(git ls-tree -d --name-only HEAD | paste -sd'|' -)"

allowed() {
  local doc="$1" path="$2"
  [[ -f "$allow_file" ]] || return 1
  awk -v doc="$doc" -v path="$path" '
    /^[[:space:]]*(#|$)/ { next }
    $1 == doc && $2 == path { found = 1 }
    END { exit found ? 0 : 1 }
  ' "$allow_file"
}

resolves() {
  local doc_dir="$1" path="$2"
  [[ -e "$path" || -e "$doc_dir/$path" || -e "docs/$path" ]] && return 0
  # Build output. The trailing-slash form matches directory-only patterns (`storybook-static/`).
  git check-ignore -q "$path" 2>/dev/null && return 0
  git check-ignore -q "$path/" 2>/dev/null && return 0
  # A Go symbol: `internal/metrics.Recorder`, `internal/api.Server.Handler()`.
  if [[ "$path" =~ ^(.*/[A-Za-z0-9_-]+)\.[A-Z][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)?(\(\))?$ ]]; then
    [[ -d "${BASH_REMATCH[1]}" ]] && return 0
  fi
  return 1
}

failures=0
while IFS= read -r doc; do
  doc_dir="$(dirname "$doc")"
  while IFS=: read -r line span; do
    path="${span//\`/}"
    path="${path%%#*}"
    path="$(sed -E 's/:[0-9]+(-[0-9]+)?$//; s#/\.\.\.$##; s#/$##' <<<"$path")"
    [[ "$path" =~ [][*?{}\<\>\$] ]] && continue
    resolves "$doc_dir" "$path" && continue
    allowed "$doc" "$path" && continue
    printf '%s:%s: "%s" names no file or directory in the repo\n' "$doc" "$line" "$path"
    failures=$((failures + 1))
  done < <(grep -noE "\`($tops)/[^\` ]*\`" "$doc" || true)
done < <(
  git ls-files -- 'README.md' 'CONTRIBUTING.md' 'CLAUDE.md' 'AGENTS.md' 'CONTEXT.md' \
    'docs/**.md' '.agents/**.md' '.claude/**.md' |
    grep -v '^\.agents/skills/' || true
)

if ((failures > 0)); then
  printf '\ncheck-doc-paths: %d dead path(s). Fix the reference, or, if the doc must name a missing\n' "$failures" >&2
  printf 'path on purpose, add "<doc> <path>  # reason" to %s.\n' "$allow_file" >&2
  exit 1
fi
printf 'check-doc-paths: every inline repo path in the live docs resolves\n'
