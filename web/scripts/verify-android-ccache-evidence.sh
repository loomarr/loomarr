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

primary_rules=()
while IFS= read -r rule; do
  primary_rules+=("$rule")
done < <(
  find "$web_root" -type f -path '*/.cxx/*/CMakeFiles/rules.ninja' \
    ! -path '*/CMakeFiles/*/CMakeFiles/rules.ninja' -print 2>/dev/null | sort
)
if ((${#primary_rules[@]} == 0)); then
  echo 'CMake generated no primary Ninja rules for ccache proof' >&2
  exit 1
fi
for rule in "${primary_rules[@]}"; do
  if ! grep -Fq -- "$launcher" "$rule"; then
    printf 'primary CMake Ninja rule lacks the reviewed ccache launcher: %s\n' "$rule" >&2
    exit 1
  fi
done

cacheable_calls=$(
  "$launcher" --print-stats --format=json |
    jq -er '(.cache_miss // 0) + (.direct_cache_hit // 0) + (.preprocessed_cache_hit // 0)'
)
if ((cacheable_calls == 0)); then
  echo 'ccache observed no cacheable compiler calls' >&2
  exit 1
fi

if [[ -n "$output" ]]; then
  printf '%s\n' "${primary_rules[@]}" > "$output"
else
  printf '%s\n' "${primary_rules[@]}"
fi
