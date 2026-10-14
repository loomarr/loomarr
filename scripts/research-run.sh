#!/usr/bin/env bash
# Run one archived filler research tool from research/, its own Go module (#1560 D1).
#
#   scripts/research-run.sh <tool> [args...]    e.g. scripts/research-run.sh filler-corpus-review --help
#
# `go run` cannot reach a nested module from the repository root, and `go -C research run` would
# move the tool's working directory, so every relative path the eval targets pass would resolve
# under research/. This builds research/cmd/<tool> into .artifacts/research-bin and runs it from
# the caller's directory instead. GO overrides the go binary, as in the Makefile.
set -euo pipefail

tool=${1:?usage: scripts/research-run.sh <tool> [args...]}
shift
bin_dir="$PWD/.artifacts/research-bin"
"${GO:-go}" -C research build -o "$bin_dir/$tool" "./cmd/$tool"
exec "$bin_dir/$tool" "$@"
