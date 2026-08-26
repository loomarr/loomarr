# Claude Code adapter

Read `AGENTS.md` first. It is the complete, canonical Loomarr agent contract and wins if this adapter
ever disagrees with it.

Claude Code conveniences are optional adapters:

- `/list-agents` can show Claude sessions, but `make agent-status` is the required cross-harness roster.
- Register work with `make agent-start TASK=... CLAIMS=...`; do not rely on `SendMessage` as the durable
  collision guard.
- Create worktrees with `make agent-worktree TOPIC=...`; do not rely on `--worktree`, `EnterWorktree`,
  `.worktreeinclude`, or implicit secret copying.
- `.claude/skills/` points to the curated `.agents/skills/` bodies; the catalog and invocation
  guidance live in [`docs/dev/skills.md`](docs/dev/skills.md).
- `.claude/commands/` contains thin adapters to the canonical `.agents/workflows/` instructions.
- Frontend packages are deep modules — see [`web/packages/README.md`](web/packages/README.md) before
  adding one or importing from one.

No project rule belongs only in this file. Add shared rules to `AGENTS.md` and shared workflows to
`.agents/workflows/`.
