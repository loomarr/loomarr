#!/usr/bin/env bash
# No model calls: capture argv at the CLI boundary, including literal prompt characters.
set -euo pipefail
SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/codex" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@"
EOF
cp "$tmp/codex" "$tmp/claude"
chmod +x "$tmp/codex" "$tmp/claude"
export PATH="$tmp:$PATH"
check() {
	local agent="$1" role="$2" model="$3" effort="$4" out
	# shellcheck disable=SC2016 # Deliberately test literal shell syntax in a prompt.
	out="$("$SCRIPT_DIR/agent-launch.sh" "$agent" "$role" 'literal $(false) `false` prompt' 2>/dev/null)"
	[ "$(printf '%s\n' "$out" | sed -n 2p)" = "$model" ]
	if [ "$agent" = codex ]; then
		printf '%s\n' "$out" | grep -Fxq "model_reasoning_effort=\"$effort\""
	elif [ -n "$effort" ]; then
		[ "$(printf '%s\n' "$out" | sed -n 4p)" = "$effort" ]
	else
		[ "$(printf '%s\n' "$out" | wc -l | tr -d ' ')" = 3 ]
	fi
	# shellcheck disable=SC2016 # Expansion here would defeat the argv preservation check.
	[ "$(printf '%s\n' "$out" | tail -1)" = 'literal $(false) `false` prompt' ]
}
check codex evidence gpt-6-luna low
check codex implementation gpt-5.6-terra medium
check codex complex gpt-6.1-sol high
check codex architecture gpt-6-astra high
check claude evidence haiku ''
check claude implementation sonnet medium
check claude complex opus high
check claude architecture opus high
if "$SCRIPT_DIR/agent-launch.sh" codex unknown >/dev/null 2>&1; then
	echo 'unknown role was accepted' >&2; exit 1
fi
"$SCRIPT_DIR/agent-launch.sh" --dry-run codex evidence | grep -q '^codex --model gpt-6-luna '
echo 'agent-launch-test: ok'
