# AI in this project

Two separate things share the word, and conflating them causes confusion:

1. **AI as a feature** — Loomarr uses an LLM to turn your sentence into a lineup. See
   [how that works](#ai-as-a-feature) below.
2. **AI as a build practice** — Loomarr is largely written with coding agents, and the repo is
   arranged for it. That's most of this page.

---

## AI as a build practice

Much of this codebase was written by AI coding agents working from `docs/design.md`, one phase
per pull request. That isn't a disclaimer; it's the reason several conventions here look
stricter than a project this size would normally justify.

### The entry points

| File | Read by | Holds |
| --- | --- | --- |
| `CLAUDE.md` | Claude Code | The full build guide: prime directives, session ritual, stop points |
| `AGENTS.md` | other harnesses | A shorter ramp pointing at the same rules |
| `docs/agents/*.md` | the installed skills | Config: domain vocabulary, issue tracker, triage labels |
| `.claude/commands/` | on demand | Repo-specific audits — `doc-drift`, `gate-review`, `surface-audit`, `register-check` |
| `CONTEXT.md` | everyone | The glossary — what each word *means*, and nothing about behaviour |

`docs/design.md` wins on behaviour; `CONTEXT.md` wins on vocabulary; `PROGRESS.md` is the phase
record. A `CONTEXT.md` that grows into a spec becomes the second authority the doc-first rule
exists to prevent.

### Why the rules are shaped this way

Every convention below exists because something plausible turned out to be wrong.

**Doc-first, in the same PR.** An agent given a stale spec writes code that is *correct against
the spec* and wrong in production. Updating the design doc before the code keeps the input
honest. This is prime directive #1 for a reason: it is the cheapest defect to prevent and the
most expensive to find later.

**Gates are hard, and never weakened to pass.** The characteristic AI failure is not broken code
— it's code that looks right, reads well, and doesn't do what its comment says. A gate is the
only thing that distinguishes them. If a gate can't pass, either the design or the code is
wrong; weakening the test destroys the one signal that noticed.

**A test that has never failed may not be connected to anything.** Sabotage the code and watch
it go red before you trust a new test. This repo has shipped several gates that exited 0 while
proving nothing:

- A capability probe that never encoded anything — the temp-file and missing-`-y` combination
  made ffmpeg refuse to overwrite and exit 0, so every encoder reported "works". The tell was
  arithmetic: nine trials in 1.49 seconds.
- `go vet ./...` passing for months while `go vet -tags '…' ./...` failed, because build-tagged
  files are invisible to the build system that vet asks.
- A Storybook story on an unregistered route, snapshotting "Not Found" as its baseline.

**Comments lag the code by about one architecture.** An agent reads a comment as truth and
builds on it. One whole subsystem was designed around a discontinuity marker that had never been
implemented — the comment describing it outlived the code by two rewrites. When a comment and
the code disagree, the comment is the bug.

**Retire identifiers explicitly.** When a PR removes a capability, its name goes into
`scripts/check-retired.sh` in the same PR. `docs/help/` ships inside the binary and is read as
instructions, so a deleted webhook that stayed documented kept telling operators to configure a
secret that was never minted. A prose rule wouldn't have caught that; a grep catches it forever.

**Claims about behaviour belong next to a test.** `docs/claims_test.go` asserts that the
embedded help pages don't contradict the code. It was added after three of them spent months
telling every operator that Tunarr did the streaming, when Loomarr had become the default
backend — the design doc was right the whole time, and nothing checked the pages against it.

### Contributing with AI assistance

**It's welcome, and it's held to the same bar as everything else.** There is no separate
process, but a few expectations:

- **You own the diff.** Understand what you're submitting well enough to defend it in review.
  "The agent wrote it" is not an answer to "why does this work?"
- **Run the gates yourself** before opening the PR — see [testing](testing.md). Agents are good
  at producing code that compiles and bad at noticing they've weakened an assertion.
- **Check the comments describe the code you actually shipped**, not the approach you started
  with. This is the single most common defect in agent-written PRs here.
- **Say so in the PR body** if a change was largely agent-generated. It's useful review context,
  not a mark against it.
- **Don't let an agent update a generated file by hand.** They will try. See
  [codegen](codegen.md).

### Parallel agent sessions

Use a **sibling** worktree (`../loomarr-<topic>`), never one inside the repo root — the
Playwright targets bind-mount the root. Setup is in [setup](setup.md).

Two sessions are only safe in parallel if they touch **different generated output**. Anything
adding an endpoint edits `api/openapi.yaml` and regenerates the orval client; two branches doing
that produce a conflict in files nobody wrote, which is the worst kind to resolve.

---

## AI as a feature

Loomarr sends your channel intent to an LLM and gets back a proposed lineup. Two properties
matter more than the model choice:

**It is grounded.** Every pick must resolve to a real title in your library or in TMDB. The
model chooses *among* candidates; it cannot invent one. If nothing grounds, the run fails
clearly rather than producing a channel full of films that don't exist.

**It only ever proposes.** The model extracts intent — an audience ceiling, an era, an ordering
— and deterministic code enforces it. Nothing is downloaded and no channel is created until an
admin approves. See [the programming guide](../help/programming.md).

### What leaves your network

| Provider | What is sent |
| --- | --- |
| **Ollama, local** (default) | Nothing. Runs on your hardware. |
| **Hosted** (OpenAI, Gemini, Groq, OpenRouter, …) | Your intent text, and titles/metadata from your library as grounding candidates |
| **TMDB** | Title searches — needed to ground picks you don't already own |

Optional filler features raise this: vision tagging sends **video keyframes** to the provider,
and hosted transcription sends **audio**. Both are off by default and gated separately, for
exactly this reason. A fully local install — Ollama plus local whisper — sends nothing but TMDB
lookups.

Choose the provider in Settings → AI. Switching takes effect immediately, no restart.
