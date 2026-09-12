#!/usr/bin/env bash
# Print the Go packages that directly own changed files. Unlike go-impact.sh, this intentionally
# excludes reverse dependants: it is the quick edit-loop boundary, not publication evidence.
set -euo pipefail

ROOT="${LOOMARR_REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"

paths=()
if (($#)); then
	paths=("$@")
else
	while IFS= read -r path; do
		[[ -n "$path" ]] && paths+=("$path")
	done
fi

((${#paths[@]})) || exit 0

owners=()
for path in "${paths[@]}"; do
	path="${path#./}"
	case "$path" in
		*.go)
			dir="${path%/*}"
			[[ "$dir" == "$path" ]] && dir=.
			;;
		internal/* | cmd/*)
			dir="${path%/*}"
			[[ "$dir" == "$path" ]] && dir=.
			while [[ "$dir" != . && "$dir" != / ]]; do
				if find "$ROOT/$dir" -maxdepth 1 -type f -name '*.go' -print -quit 2>/dev/null | grep -q .; then
					break
				fi
				parent="${dir%/*}"
				if [[ "$parent" == "$dir" ]]; then dir=.; else dir="$parent"; fi
			done
			if [[ "$dir" == . ]] && ! find "$ROOT" -maxdepth 1 -type f -name '*.go' -print -quit | grep -q .; then
				continue
			fi
			;;
		*) continue ;;
	esac
	owners+=("$dir")
done

((${#owners[@]})) || exit 0

printf '%s\n' "${owners[@]}" | sort -u | while IFS= read -r dir; do
	package="$(cd "$ROOT" && go list -e -f '{{if .Error}}{{else}}{{.ImportPath}}{{end}}' "./$dir")"
	if [[ -n "$package" ]]; then
		printf '%s\n' "$package"
	else
		printf 'go-direct-impact: cannot resolve %s; no direct package selected\n' "$dir" >&2
	fi
done | sort -u
