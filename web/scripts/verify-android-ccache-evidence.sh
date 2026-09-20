#!/usr/bin/env bash
# Prove that a completed Android native build actually routed compiler calls through ccache.
set -euo pipefail

launcher=${1:-}
web_root=${2:-}
output=${3:-}

if [[ -z "$launcher" || -z "$web_root" ]]; then
  echo "usage: $0 <ccache-launcher> <web-root> [launcher-evidence-output]" >&2
  exit 2
fi

launcher_rules=()
while IFS= read -r rule; do
  launcher_rules+=("$rule")
done < <(
  find "$web_root" -type f -path '*/.cxx/*/CMakeFiles/rules.ninja' -exec grep -Fl -- "$launcher" {} + 2>/dev/null | sort
)
if ((${#launcher_rules[@]} == 0)); then
  echo 'CMake generated no launcher-bearing Ninja rules for ccache proof' >&2
  exit 1
fi

cacheable_calls=$(
  "$launcher" --print-stats --format=json |
    jq -er '(.cache_miss // 0) + (.direct_cache_hit // 0) + (.preprocessed_cache_hit // 0)'
)
if ((cacheable_calls == 0)); then
  echo 'ccache observed no cacheable compiler calls' >&2
  exit 1
fi

if [[ -n "$output" ]]; then
  printf '%s\n' "${launcher_rules[@]}" > "$output"
else
  printf '%s\n' "${launcher_rules[@]}"
fi
