#!/usr/bin/env bash
# Task-based model selection for an interactive session in the current worktree.
# This selects model/effort only; the supervisor owns checkpoint budgets and permissions.
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: scripts/dev/agent-launch.sh [--dry-run] codex|claude evidence|implementation|complex|architecture [agent arguments...]

evidence        Mechanical collection against an explicit checklist.
implementation Ordinary implementation or multi-file analysis.
complex         Difficult integration/contract reasoning; justify in the brief.
architecture    Hardest cross-system decisions; justify benefit and risk in the brief.

Run from the registered task worktree after setup. This launcher neither creates a worker
nor enforces a token budget. Follow .agents/workflows/supervise.md for supervised workers.
EOF
}

dry_run=false
if [ "${1:-}" = --dry-run ]; then
	dry_run=true
	shift
fi
if [ "${1:-}" = --help ]; then
	usage
	exit 0
fi
[ "$#" -ge 2 ] || { usage >&2; exit 2; }
agent="$1"
role="$2"
shift 2
effort=medium
case "$agent:$role" in
	codex:evidence) model=gpt-6-luna; effort=low ;;
	codex:implementation) model=gpt-5.6-terra ;;
	codex:complex) model=gpt-6.1-sol; effort=high ;;
	codex:architecture) model=gpt-6-astra; effort=high ;;
	claude:evidence) model=haiku; effort='' ;;
	claude:implementation) model=sonnet ;;
	claude:complex|claude:architecture) model=opus; effort=high ;;
	*) usage >&2; exit 2 ;;
esac

if [ "$agent" = codex ]; then
	set -- codex --model "$model" -c "model_reasoning_effort=\"$effort\"" "$@"
elif [ -n "$effort" ]; then
	set -- claude --model "$model" --effort "$effort" "$@"
else
	# Haiku does not expose the same effort control as Sonnet/Opus.
	set -- claude --model "$model" "$@"
fi
if "$dry_run"; then
	printf '%q ' "$@"
	printf '\n'
	exit 0
fi
printf 'agent-launch: %s %s; requested model=%s effort=%s; verify effective settings in the session\n' \
	"$agent" "$role" "$model" "${effort:-uncontrolled}" >&2
exec "$@"
