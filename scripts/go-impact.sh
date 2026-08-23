#!/usr/bin/env bash
# Print the repository Go packages affected by changed paths, including reverse
# dependants. Paths may be arguments or newline-delimited stdin.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULE="$(cd "$ROOT" && go list -m)"

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

all_packages() {
  (cd "$ROOT" && go list ./...)
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
    | awk -F '\t' -v module="$MODULE" '
        BEGIN {
          count = split(ENVIRON["CHANGED_IMPORTS"], changed, "\n")
          for (i = 1; i <= count; i++) wanted[changed[i]] = 1
        }
        {
          haystack = " " $1 " " $2 " " $3 " " $4 " "
          for (dependency in wanted) {
            if (index(haystack, " " dependency " ")) {
              path = $1
              sub("^" module, ".", path)
              print path
              break
            }
          }
        }
      ' \
    | sort -u
)
