#!/usr/bin/env bash
# Select CodeQL languages from a trustworthy changed-path set. Unknown paths fail closed.
set -euo pipefail

readonly LANGUAGES=(actions go javascript-typescript python ruby rust)
selected=(false false false false false false)
force_all=false
unknown=false

select_language() {
	local wanted="$1" i
	for ((i = 0; i < ${#LANGUAGES[@]}; i++)); do
		if [[ "${LANGUAGES[$i]}" == "$wanted" ]]; then
			selected[i]=true
			return
		fi
	done
	echo "codeql-impact: internal error: unknown language $wanted" >&2
	exit 2
}

select_all() {
	local i
	for ((i = 0; i < ${#LANGUAGES[@]}; i++)); do
		selected[i]=true
	done
}

classify() {
	local path="$1"
	case "$path" in
	.github/workflows/codeql.yml | scripts/codeql-impact.sh | scripts/codeql-impact-test.sh)
		select_all
		;;
	.github/workflows/*.yml | .github/workflows/*.yaml | .github/actions/*)
		select_language actions
		;;
	*.go | go.mod | go.sum)
		select_language go
		;;
	*.js | *.jsx | *.ts | *.tsx | *.mjs | *.cjs | package.json | package-lock.json | yarn.lock | \
		pnpm-lock.yaml | pnpm-workspace.yaml | web/package.json | web/pnpm-lock.yaml | web/pnpm-workspace.yaml | \
		web/tsconfig*.json | web/*/tsconfig*.json)
		select_language javascript-typescript
		;;
	*.py | requirements.txt | requirements/*.txt | pyproject.toml | poetry.lock | Pipfile | Pipfile.lock)
		select_language python
		;;
	*.rb | Gemfile | Gemfile.lock | *.gemspec)
		select_language ruby
		;;
	*.rs | Cargo.toml | Cargo.lock | rust-toolchain.toml | rust-toolchain | deny.toml)
		select_language rust
		;;
	*.md | *.sh | *.css | *.html | *.svg | *.png | *.jpg | *.jpeg | *.webp | *.ico | *.woff | *.woff2 | \
		*.sql | *.xml | *.plist | *.xcconfig | *.pbxproj | *.storyboard | *.strings | *.gradle | *.properties | \
		*.json | *.toml | *.yml | *.yaml | *.lock | .gitignore | .gitattributes | .editorconfig | .dockerignore | \
		Dockerfile | Makefile | *.mk | LICENSE)
		;;
	*)
		echo "codeql-impact: unknown path $path; selecting every language" >&2
		unknown=true
		;;
	esac
}

paths=()
for arg in "$@"; do
	if [[ "$arg" == --all ]]; then
		force_all=true
	else
		paths+=("$arg")
	fi
done
if [[ "$force_all" == false ]] && ((${#paths[@]} == 0)) && [[ ! -t 0 ]]; then
	while IFS= read -r path; do
		[[ -n "$path" ]] && paths+=("$path")
	done
fi

if [[ "$force_all" == true ]]; then
	select_all
else
	for path in "${paths[@]}"; do
		classify "$path"
	done
	[[ "$unknown" == true ]] && select_all
fi

for ((i = 0; i < ${#LANGUAGES[@]}; i++)); do
	printf '%s=%s\n' "${LANGUAGES[$i]}" "${selected[$i]}"
done
