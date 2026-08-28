---
description: Coordinate bounded agents under one delivery owner and integrate their evidence
---

# Supervise agents

Coordinate independent agents without splitting ownership of one deliverable. The supervisor owns
the goal, acceptance criteria, integration, gates, and delivery. Workers own bounded investigations
or clearly separated implementation seams and return evidence to the supervisor.

**Input:** one goal with acceptance criteria. Optional: named workstreams or a maximum worker count.

Do not use this workflow merely because multiple agents are available. Use it when independent work
can reduce elapsed time, isolate noisy context, or provide a genuinely fresh review. Keep sequential
reasoning and edits to the same mutable seam with one agent.

## Establish the owner

One registered task worktree is the delivery owner. From that worktree:

1. Read `PROGRESS.md`'s **Active work** table and the governing design text.
2. Run `make agent-status`; reconcile active tasks, claims, dependencies, and worktrees.
3. Confirm that the owner holds every scarce-output claim needed for integration.
4. State the acceptance evidence and stop points before delegating.

A product's native agent panel shows only its own agent tree. `make agent-status` is the
cross-harness roster, but it does not expose another session's conversation or reasoning. Report
that visibility limit instead of implying direct control over an independent session.

## Track work in GitHub

GitHub issues track questions, bugs, and work; `PROGRESS.md` alone owns phase status and gate
evidence; claims and worktrees lock mutable seams. An issue is required for every supervised
research, work, implementation, or review assignment. A `PROGRESS.md` row may add a phase-evidence
pointer, but never substitutes for the issue or duplicates its phase state.

Every worker brief and report carries its required issue URL/number and issue actions, including an
open and closed-issue search, a linked issue, a created issue, a comment, or `none` with its reason.
Search open and closed issues before creating one.

File only a confirmed current-`main` bug: include its viewer-visible symptom, reproduction,
evidence, and acceptance criteria. Do not file speculation. Link research findings and conclusions to
the tracking issue and, when the result needs to outlive the report, a durable Markdown record. File
or link an out-of-scope confirmed defect instead of silently fixing it. Implementation, PR, and
cross-host handoff reports retain their tracking issue pointers.

## Choose the execution shape

Match concurrency to independent seams, not to the number of available sessions. Keep one agent for
a small task, a sequential reasoning chain, or edits to shared mutable state. Use a supervisor and
bounded workers when investigation, review, or implementation can proceed independently and the
owner can verify each returned result before integration.

Treat roles as temporary missions or review lenses, not permanent agent identities. A worker returns
control when its brief is complete and does not choose its own next task. The supervisor may then
stop it or issue a new brief with a different role. This rotation keeps specialized context bounded
without creating long-lived ownership silos.

Choose model capability and reasoning effort for the task when the harness exposes those controls:

- use the balanced capability tier at Medium reasoning as the default;
- use a lower-cost tier for bounded read-heavy scans, issue triage, and repetitive mechanical work
  whose acceptance is objective;
- use frontier capability or High reasoning only for measured ambiguity, security, authorization,
  migration, integration, or final-acceptance work; and
- compare speed or cost only among runs that satisfy the same acceptance criteria.

Record the selection and rationale in the brief. Model choice never changes scope, authority, claims,
tools, stop points, or acceptance. Do not switch a model or reasoning setting during an active
checkpoint; change it only with the next bounded assignment after the worker has returned control.
For an external session whose model cannot be controlled or verified, record `uncontrolled` instead
of guessing.

## Build the task graph

Split the goal only at real seams. For every worker, record:

- a unique task id and one-sentence outcome;
- read-only or editing mode;
- exact scope and explicit exclusions;
- inputs and governing acceptance clauses;
- required evidence and return format;
- dependencies, claims, file/interface seam, and merge order when it edits; and
- the condition for stopping and returning control.

Prefer read-only workers for codebase exploration, diagnosis, research, test analysis, competing
designs, and fresh-context review. They may share the owner's checkout because they do not mutate
it. An editing worker needs its own registered worktree only when its file seam, interface seam,
claims, and merge order are all explicit. If any are unclear, keep one editing owner.

Do not ask two workers to edit the same DTO, migration sequence, generated output, visual baseline,
or application interface. Claims reveal known collisions; they do not make overlapping edits safe.

## Brief each worker

Give each worker the smallest complete context, not the supervisor's accumulated reasoning:

```text
WORKER BRIEF
task: <unique id>
role: <temporary mission or review lens>
outcome: <one sentence>
mode: <read-only | editing>
execution: <model/capability and reasoning effort, inherited, or uncontrolled; rationale>
usage: <source; start; end; delta, or unavailable/uncontrolled>
tracking: <required issue URL/#>
phase-evidence: <PROGRESS.md row or none>
issue-actions: <searched open/closed; linked/created/commented, or none with reason>
owner: <task and worktree>
base: <commit or branch>
scope: <paths, subsystem, or question>
do-not-touch: <explicit exclusions>
acceptance: <verbatim clauses or exact commands>
return: <required evidence and format>
stop: <completion or escalation condition>
```

Record usage only when the harness exposes a worker-scoped measurement. `source` says where the
number came from; `start`, `end`, and `delta` are that source's values at the assignment boundaries.
Use `unavailable` when it cannot be observed and `uncontrolled` when an external session cannot be
attributed reliably. Never assign an aggregate goal or session total to an individual worker.

Use the current harness's delegation facility. When it supports agent trees, spawn workers under the
supervisor so it can inspect, steer, wait, and collect their results. For an independent external
session, provide the brief through the available channel and treat its registry, branch, worktree,
commits, and reports as observable evidence rather than assuming live steering.

## Supervision loop

Keep the main context on decisions and evidence. Do not copy raw exploration logs into it.

1. Inspect native agent state and `make agent-status` before assigning follow-up work.
2. At a meaningful evidence checkpoint, collect the worker report below. Interpret progress by
   accepted evidence for that checkpoint, never by tokens alone.
3. Check cited files, diffs, commands, exit status, and artifacts. A worker's conclusion is a claim,
   not integration evidence.
4. Send a bounded correction when evidence is missing or scope drifted. On repeated no-progress,
   duplicated work, or scope drift, rescope or interrupt the worker; a large usage count alone is
   never a reason to do so. Reassign only the unfinished portion; do not restart accepted work.
5. Escalate a blocked dependency, contract deviation, authorization change, or overlapping claim.
6. Wait when no supervisor decision is needed; avoid polling agents merely to produce activity.

Workers return this schema:

```text
WORKER REPORT
task: <id>
role: <temporary mission or review lens>
state: <complete | blocked | needs-review>
execution: <actual model/capability and reasoning effort, inherited, or uncontrolled>
usage: <source; start; end; delta, or unavailable/uncontrolled>
tracking: <required issue URL/#>
phase-evidence: <PROGRESS.md row or none>
issue-actions: <searched open/closed; linked/created/commented, or none with reason>
branch/worktree: <branch> @ <absolute worktree, or read-only>
base/head: <commit> / <commit>
claims: <comma-separated or none>
changed: <paths or none>
evidence: <commands with pass/fail and artifact paths>
findings: <distilled conclusions with file:line citations>
blockers: <specific dependency or none>
next: <recommended supervisor action>
```

`complete` means the assigned outcome and evidence are complete, not that the initiative is merged,
released, or accepted. Only the supervisor may make the initiative-level completion claim.

## Hand off between Linux and Mac

A cross-host handoff is a stop-the-world barrier, not another orchestration channel. Linux and Mac
must never write the same deliverable simultaneously. Before Linux releases the work, stop or wait
for every worker, collect its final `WORKER REPORT`, reconcile `make agent-status`, and add the
handoff record to the tracking issue. Do not hand off merely because a pane is idle. Local registries
cannot enforce cross-host exclusion; the barrier and durable issue handoff record do.

The tracking issue contains this required record. Keep it current through the Linux stop and Mac
acceptance; `issue-actions` preserves searched, linked, created, and commented outcomes.

```text
HANDOFF RECORD
tracking: <required issue URL/#>
issue-actions: <searched open/closed; linked/created/commented, or none with reason>
topic: <exact branch name>
task: <exact task name>
claims: <exact comma-separated values, or empty explicitly>
linux-head: <full commit OID>
transfer: <authorized push/PR URL, or bundle filename + SHA-256 digest>
dirty-paths: <resolved/retained path-by-path disposition, or none>
stopped-linux-writers: <worker ids and owner after make agent-stop>
mac-verified-head: <full commit OID>
mac-owner: <task and worktree>
```

In every fresh Linux or Mac shell, copy the exact `topic`, `task`, and `claims` values from this
record; never assume inherited environment values. `claims` may be empty, but it must be set
explicitly. Before any ref, push, fetch, or worktree command, validate the required values:

```sh
topic='<exact topic from handoff record>'
task='<exact task from handoff record>'
claims='<exact claims from handoff record; empty is explicit>'
[ -n "$topic" ] && [ -n "$task" ] || exit 1
git check-ref-format --branch "$topic" >/dev/null || exit 1
```

On Linux, from the delivery worktree, verify the intended transfer and make every intended tracked
change a commit. Resolve or explicitly retain dirty and untracked paths; neither is silently handed
to Mac. Validate the topic before using it as a ref, and derive transfer filenames only from a safe
HEAD-derived handoff id. Use one authorized transfer route:

```sh
make agent-status || exit 1
git status --short || exit 1
git diff --check || exit 1
git log --oneline origin/main..HEAD || exit 1
git add <intended-paths> || exit 1
git commit -m "<handoff-ready change>" || exit 1
git check-ref-format --branch "$topic" >/dev/null || exit 1
test "$(git branch --show-current)" = "$topic" || exit 1
handoff_id=$(git rev-parse --verify --short=12 HEAD) || exit 1

# Only with authorization to publish this branch or PR:
git push origin "refs/heads/$topic:refs/heads/$topic" || exit 1

# Or, when a push/PR is not authorized, create a named bundle for secure copying:
bundle="loomarr-handoff-$handoff_id.bundle"
git bundle create "$bundle" "$topic" ^origin/main || exit 1
sha256sum "$bundle" > "$bundle.sha256" || exit 1
```

Transfer the bundle and its checksum through an authorized secure channel, then verify the checksum
on Mac before fetching it. Name and checksum build/test artifacts separately; transfer only the
artifacts explicitly needed for acceptance. Private evidence, `.env` files, and credentials are
never copied by default. tmux panes, local registries, claims, caches, ports, and ignored generated
outputs are host-local and must be recreated or re-established on Mac. Once the transfer is verified,
Mac tells Linux to stop. Every Linux writer, including the owner, then runs `make agent-stop` before
Mac edits begin.

On Mac, start from a fresh clone and re-establish the task before any edit. For a pushed branch:

```sh
git clone <authorized-repository-url> loomarr || exit 1
cd loomarr || exit 1
git check-ref-format --branch "$topic" >/dev/null || exit 1
git fetch origin "refs/heads/$topic:refs/remotes/origin/$topic" || exit 1
make doctor || exit 1
make agent-worktree TOPIC="$topic" BASE="origin/$topic" TASK="$task" CLAIMS="$claims" || exit 1
# cd to the worktree path printed by the harness; do not derive a path from $topic.
cd <harness-printed-worktree> || exit 1
make agent-status || exit 1
test "$(git rev-parse HEAD)" = "$(git rev-parse "origin/$topic")" || exit 1
git diff --exit-code "origin/$topic...HEAD" || exit 1
make agent-baseline || exit 1
```

For a securely copied bundle, verify and fetch it before the same `make doctor` sequence:

```sh
bundle=<securely-copied-bundle-path>
git check-ref-format --branch "$topic" >/dev/null || exit 1
expected=$(awk '{print $1}' "$bundle.sha256") || exit 1
actual=$(shasum -a 256 "$bundle" | awk '{print $1}') || exit 1
test "$actual" = "$expected" || exit 1
git bundle verify "$bundle" || exit 1
git fetch "$bundle" "refs/heads/$topic:refs/remotes/origin/$topic" || exit 1
```

`make agent-worktree` registers `TASK` and the same claims before bootstrap; do not add a second
`make agent-start`. Confirm the registration and claims with `make agent-status`, resolve any
conflict, and update the tracking issue with Mac's verified head before editing. Preserve the Linux
branch, bundle (if used), and all worker reports until Mac has completed acceptance. The Mac owner
alone may resume writing after this barrier.

## Integrate and deliver

1. Preserve read-only findings separately from the supervisor's judgment.
2. Review every editing worker's diff against its brief before integrating it in the recorded order.
3. Re-run affected checks after integration; worker-local green checks do not prove the combined tree.
4. Use `gate-review.md` for a fresh-context acceptance review when the change has a written gate.
5. Run the complete required gates for the touched areas, publish the owning PR, and follow its CI.
6. After merge, release claims with `make agent-stop`; audit retirement with `make agent-gc` before
   any explicit `APPLY=1` cleanup.

## Supervisor output

```text
SUPERVISION: <goal>
owner: <task>  branch: <branch>  worktree: <path>

WORKERS
<task>  <state>  <role>  <mode>  <execution>  <outcome>  <evidence or blocker>

INTEGRATION
accepted: <worker outputs incorporated>
pending: <remaining work or evidence>
conflicts: <claims, dependencies, or none>
next: <single next action>
```
