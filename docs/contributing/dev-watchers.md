# Dev watchers

**For:** anyone running long sessions or supervising agent lanes on a dev machine (Linux or macOS).
**You'll get:** how to arm the watchers, what each one reports, how to tune it, and how to retire a
worktree without leaving processes behind.

Parallel lanes slow a machine down gradually. Orphaned `air` watchers keep polling deleted worktrees,
scratch fills a RAM-backed `/tmp`, a PR drops out of the merge queue, or a lane waits on a question
nobody sees. The watchers catch these. Each one prints a line when a condition starts and another
when it clears. When nothing is wrong, they print nothing.

The watchers only observe. None of them stops a process, types into a terminal, or changes a PR. You
decide what to do about each alert.

## Arm them

```sh
make dev-watch
```

This runs every watcher that applies to your machine, in the foreground, until you press Ctrl-C. Each
line starts with the time and the watcher's name. Every Claude Code session in this repo prints a
one-line snapshot of the machine when it starts, as a reminder. The snapshot comes from the shared
`.claude/settings.json` hook.

Supervisors and long-running agents arm the watchers for the whole session (see `AGENTS.md`).

| Watcher | Runs when | Reports |
| --- | --- | --- |
| `scripts/dev/watch-resources.sh` | always | high load, low memory, a full RAM-backed `/tmp`, processes in deleted worktrees, a CPU hog, a busy GPU, an idle `sleep` holding a lock |
| `scripts/dev/watch-prs.sh` | `gh` is signed in | your open PRs against `main` that are conflicted, failing checks, `BEHIND` with auto-merge on, dropped from the merge queue, or unarmed and not queued |
| `scripts/dev/watch-lanes.sh` | the `orca` CLI is installed | agent lanes crossing a token budget level, finishing a turn, waiting on a question menu, or showing API errors |

Each script also runs on its own, and `--once` makes a single check and exits.

## Resource thresholds

The defaults scale to the machine: load is measured against the core count and memory against the
total. You can override every threshold with an environment variable.

| Condition | Default | Variables |
| --- | --- | --- |
| 1-minute load above 70% of cores for 3 minutes | 0.7, 3 | `WATCH_LOAD_RATIO`, `WATCH_LOAD_MINUTES` |
| Available memory below 10% of total | 10 | `WATCH_MEM_MIN_PCT` |
| `/tmp` above 60% full, only when it's tmpfs | 60 | `WATCH_TMP_PCT`, `WATCH_TMP_DIR`, `WATCH_TMP_ANY_FS=1` |
| A process whose working directory is in a deleted or trashed worktree | — | `WATCH_WORKTREE_ROOTS` (colon-separated) |
| One process above 1.5 cores for 5 minutes | 1.5, 5 | `WATCH_HOG_CORES`, `WATCH_HOG_MINUTES`, `WATCH_HOG_IGNORE` |
| NVIDIA GPU above 90% for 5 minutes | 90, 5 | `WATCH_GPU_PCT`, `WATCH_GPU_MINUTES` |
| The GPU or heavy-gate lock held by a `sleep` for 2 polls | the two `/tmp/loomarr-*.lock` files | `WATCH_LOCKS`, `WATCH_LOCK_POLLS` |

The watcher checks every `WATCH_INTERVAL` seconds (default 60). `WATCH_HOG_IGNORE` is a
space-separated list of process-name globs for desktop apps that are allowed to be busy. It covers
common browsers and desktop shells by default. Worktree roots are found automatically: the primary
checkout, its `<primary>-<topic>` siblings, and the directory of every other worktree, which is how
Orca's workspace directory is found.

CPU use is measured from each process's cumulative CPU time between polls. `ps -o pcpu` isn't
used because on Linux it's an average over the process's whole life, so a long-running process that
has only just started spinning wouldn't show up.

## PR and lane alerts

A PR alert fires once for each combination of PR, condition and head commit. The watcher stores
this in `~/.local/state/loomarr-watch` (or under `$XDG_STATE_HOME`), so restarting the watcher
doesn't repeat old alerts. If the same problem comes back after a new push, it alerts again. Drafts
and stacked PRs belong to their owners and are skipped. A PR with auto-merge off is only reported
after `WATCH_PRS_AUTO_GRACE` seconds (default 1200), because lanes arm auto-merge once CI is green.

The lane watcher finds agent terminals through `orca terminal list` in this repo's secondary
worktrees. It counts a lane's budget in output tokens across its current Claude session and that
session's subagents, counting each API message once. It reports at `WATCH_LANE_WARN`,
`WATCH_LANE_CUTOFF` and `WATCH_LANE_LIMIT` (150k, 190k and 240k by default). If a supervisor's
checkpoint spans several sessions, write its start time (epoch seconds) to
`$WATCH_LANES_CHECKPOINTS/<lane>`. The watcher then counts every transcript created since that time.

## Retire a worktree cleanly

Removing a worktree doesn't stop the processes that were started in it. Air, vite, storybook and
headless browsers keep running against the deleted tree and keep using memory. Before and after you
retire a lane, check for them:

```sh
make agent-reap WORKTREE=../loomarr-my-topic          # list processes working in it
make agent-reap WORKTREE=../loomarr-my-topic APPLY=1  # stop them: TERM, then KILL after 5 s
make agent-reap ORPHANS=1                             # anything left in deleted worktrees
```

`make agent-reap` matches processes by their working directory, subdirectories included. It stops
them by PID and never matches itself or the shell that called it. `make agent-gc` keeps any worktree
that still has such a process, and lists the processes so you can stop them first. Leave the
primary checkout's dev backend alone unless you started it.

## Check the watchers on a machine

```sh
scripts/dev/watch-verify.sh
```

This takes about two minutes. It runs the unit tests (`scripts/dev/watch-test.sh`, also part of
`make agent-harness-test`), then creates each condition for real and confirms the watcher reports it
once and then reports it clear. The conditions are an orphan in a deleted sibling directory, a
`yes > /dev/null` hog, a `sleep` holding a private lock file, and on Linux an 8 MiB tmpfs. It
cleans up everything it starts. On macOS the tmpfs check is skipped because `/tmp` is on disk.

## Portability

The scripts run on macOS's stock bash 3.2. They keep state in small files instead of associative
arrays, use `cksum` for hashing, and handle both GNU and BSD `stat`, `ps` and `find`. Linux data
comes from `/proc`. On macOS it comes from `sysctl`, `vm_stat` and `lsof`. The GPU check needs
`nvidia-smi` and is skipped on macOS.
