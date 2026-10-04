#!/usr/bin/env bash
# Print the repository Go packages affected by changed paths, including reverse
# dependants. Paths may be arguments or newline-delimited stdin.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

paths=()
if (($#)); then
  paths=("$@")
else
  while IFS= read -r path; do
    [[ -n "$path" ]] && paths+=("$path")
  done
fi

((${#paths[@]})) || exit 0

scope="$("$ROOT/scripts/ci-impact.sh" "${paths[@]}")"
[[ "$(sed -n 's/^go=//p' <<<"$scope")" == true ]] || exit 0

# Read from the Makefile (`make -s print-tags-csv`) rather than keeping a second,
# hand-maintained copy of CUSTOM_TAGS — see mk/check.mk's `lint` target and `tags-verify`'s
# both-directions guard against exactly this kind of drift.
TAGS_CSV="$(cd "$ROOT" && make -s print-tags-csv)"

# `go list ./...` without `-tags` is blind to packages whose files are ALL guarded by a
# `//go:build` constraint (e.g. internal/eval, entirely `eval`-tagged): the build system
# reports no Go files there at all, so such a package is silently absent from this fallback's
# selection even though `make lint` below is given `--build-tags` and would gladly check it.
# `-tags` must match what `make lint` compiles under, or this "select everything" fallback
# would itself have a blind spot narrower than "everything" (GH #1279).
all_packages() {
  (cd "$ROOT" && go list -tags "$TAGS_CSV" ./...)
}

if [[ "$(sed -n 's/^go_full=//p' <<<"$scope")" == true ]]; then
  all_packages
  exit 0
fi

changed_imports=()
for path in "${paths[@]}"; do
  [[ "$path" == *.go ]] || continue
  dir="${path%/*}"
  [[ "$dir" == "$path" ]] && dir=.
  if ! import_path="$(cd "$ROOT" && go list -e -f '{{if .Error}}{{else}}{{.ImportPath}}{{end}}' "./$dir")" \
    || [[ -z "$import_path" ]]; then
    printf 'go-impact: cannot resolve %q; selecting every Go package\n' "$dir" >&2
    all_packages
    exit 0
  fi
  changed_imports+=("$import_path")
done

((${#changed_imports[@]})) || {
  printf 'go-impact: Go gate selected without a resolvable Go source path; selecting every Go package\n' >&2
  all_packages
  exit 0
}

changed_list="$(printf '%s\n' "${changed_imports[@]}" | sort -u)"
(
  cd "$ROOT"
  export CHANGED_IMPORTS="$changed_list"
  go list \
    -f '{{.ImportPath}}{{"\t"}}{{join .Deps " "}}{{"\t"}}{{join .TestImports " "}}{{"\t"}}{{join .XTestImports " "}}' \
    ./... \
    | awk -F '\t' '
        BEGIN {
          count = split(ENVIRON["CHANGED_IMPORTS"], changed, "\n")
          for (i = 1; i <= count; i++) wanted[changed[i]] = 1
        }
        {
          haystack = " " $1 " " $2 " " $3 " " $4 " "
          for (dependency in wanted) {
            if (index(haystack, " " dependency " ")) {
              print $1
              break
            }
          }
        }
      ' \
    | sort -u
)
