#!/usr/bin/env sh

set -eu

SCRIPT_DIR="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd -P)"
ROOT="$(CDPATH='' cd -- "$SCRIPT_DIR/.." && pwd -P)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

fail() {
	echo "agent-assets-verify: $*" >&2
	exit 1
}

find "$ROOT/.agents/skills" -mindepth 1 -maxdepth 1 -type d -exec basename {} \; | sort > "$WORK/directories"
sed -n 's/^    "\([^"]*\)": {$/\1/p' "$ROOT/skills-lock.json" | sort > "$WORK/locked"
find "$ROOT/.claude/skills" -mindepth 1 -maxdepth 1 -type l -exec basename {} \; | sort > "$WORK/claude"

cmp -s "$WORK/directories" "$WORK/locked" || {
	diff -u "$WORK/directories" "$WORK/locked" >&2 || true
	fail 'skill directories and skills-lock.json differ'
}
cmp -s "$WORK/directories" "$WORK/claude" || {
	diff -u "$WORK/directories" "$WORK/claude" >&2 || true
	fail 'skill directories and Claude adapters differ'
}

node -e 'JSON.parse(require("fs").readFileSync(process.argv[1], "utf8"))' "$ROOT/skills-lock.json"

while IFS= read -r name; do
	skill="$ROOT/.agents/skills/$name/SKILL.md"
	metadata="$ROOT/.agents/skills/$name/agents/openai.yaml"
	adapter="$ROOT/.claude/skills/$name"
	[ -f "$skill" ] || fail "$name has no SKILL.md"
	[ -f "$metadata" ] || fail "$name has no agents/openai.yaml"
	grep -Fqx "name: $name" "$skill" || fail "$name frontmatter has the wrong name"
	grep -q '^description: .' "$skill" || fail "$name has no description"
	[ "$(readlink "$adapter")" = "../../.agents/skills/$name" ] || fail "$name Claude adapter points somewhere else"
	grep -Fq "| \`$name\` |" "$ROOT/docs/dev/skills.md" || fail "$name is absent from docs/dev/skills.md"
done < "$WORK/directories"

find "$ROOT/.agents/workflows" -maxdepth 1 -type f -name '*.md' -exec basename {} \; | sort > "$WORK/workflows"
find "$ROOT/.claude/commands" -maxdepth 1 -type f -name '*.md' -exec basename {} \; | sort > "$WORK/commands"
cmp -s "$WORK/workflows" "$WORK/commands" || {
	diff -u "$WORK/workflows" "$WORK/commands" >&2 || true
	fail 'durable workflows and Claude command adapters differ'
}

while IFS= read -r workflow; do
	grep -Fq ".agents/workflows/$workflow" "$ROOT/.claude/commands/$workflow" ||
		fail "$workflow adapter does not point to its durable workflow"
done < "$WORK/workflows"

echo 'agent-assets-verify: ok'
