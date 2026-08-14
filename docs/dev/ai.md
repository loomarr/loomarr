# AI in this project

Two different things share the word:

1. **AI as a feature** — Loomarr uses an LLM to turn your sentence into a lineup.
2. **AI as a build practice** — much of this codebase is written by coding agents, and the repo
   is arranged for it.

## As a build practice

### Entry points

| File | Read by | Holds |
| --- | --- | --- |
| `AGENTS.md` | every harness | Canonical directives, session lifecycle, stop points |
| `CLAUDE.md` | Claude Code | Thin adapter to `AGENTS.md` |
| `docs/agents/*.md` | the installed skills | Domain vocabulary, issue tracker, triage labels |
| `.agents/workflows/` | every harness | `doc-drift`, `gate-review`, `surface-audit`, `register-check` |
| `.claude/commands/` | Claude Code | Thin adapters to `.agents/workflows/` |
| `CONTEXT.md` | everyone | The glossary — what each word means |

`docs/design.md` wins on behaviour, `CONTEXT.md` on vocabulary, `PROGRESS.md` on phase status.

### Why the rules are strict

**Doc-first, same PR.** An agent given a stale spec writes code that's correct against the spec
and wrong in production.

**Gates are never weakened to pass.** The characteristic failure isn't broken code — it's code
that looks right, reads well, and doesn't do what its comment says. A gate is the only thing that
distinguishes them.

**A test that has never failed may not be connected to anything.** Sabotage the code and watch it
go red. Gates that exited 0 while proving nothing, here:

- A capability probe that never encoded anything — a missing `-y` made ffmpeg refuse to overwrite
  and exit 0, so every encoder reported "works". The tell was arithmetic: nine trials in 1.49s.
- `go vet ./...` passing for months while the tagged run failed.
- A Storybook story on an unregistered route, snapshotting "Not Found" as its baseline.

**Comments lag the code by about one architecture.** One subsystem was designed around a marker
that had never been implemented. When a comment and the code disagree, the comment is the bug.

**Retire identifiers explicitly.** When a PR removes a capability, its name goes into
`scripts/check-retired.sh` in the same PR — `docs/help/` ships in the binary and is read as
instructions.

**Claims about behaviour belong next to a test.** `docs/claims_test.go` asserts the help pages
don't contradict the code, after three of them spent months saying Tunarr did the streaming.

### Contributing with AI assistance

Welcome, same bar as anything else. A few expectations:

- **You own the diff.** Understand it well enough to defend it in review.
- **Run the gates yourself** — agents are good at producing code that compiles and bad at
  noticing they've weakened an assertion.
- **Check the comments describe what you shipped**, not the approach you started with. This is
  the most common defect in agent-written PRs here.
- **Say so in the PR body.** It's useful review context, not a mark against it.

For parallel sessions, use the shared registry and worktree harness — see [Agent development](agents.md).

## As a feature

Loomarr sends your intent to an LLM and gets back a proposed lineup. Two properties matter more
than the model:

**It's grounded.** Every pick resolves to a real title in your library or TMDB. The model chooses
among candidates; it can't invent one.

**It only proposes.** The model extracts intent — a rating cap, an era, an ordering — and
deterministic code enforces it. Nothing downloads until an admin approves.

### What leaves your network

| Provider | Sends |
| --- | --- |
| Ollama, local (default) | Nothing |
| Hosted (OpenAI, Gemini, Groq, OpenRouter) | Your intent, plus titles and metadata from your library |
| TMDB | Title searches |

Two optional filler features send more and are off by default: vision tagging sends video
keyframes, hosted transcription sends audio. A fully local install sends only TMDB lookups.

Choose the provider in Settings → AI. Switching takes effect immediately.
