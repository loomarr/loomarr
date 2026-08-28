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

- inherit the supervisor's defaults unless the task shape justifies an override;
- use stronger capability or higher reasoning for ambiguous multi-step work, safety or authorization
  review, architectural integration, and final acceptance;
- use faster or lower-cost execution for bounded read-heavy scans, issue triage, and repetitive
  mechanical work whose acceptance is objective; and
- compare speed or cost only among runs that satisfy the same acceptance criteria.

Record the selection and rationale in the brief. Model choice never changes scope, authority, claims,
tools, stop points, or acceptance. Change it only when issuing a new assignment; preserve the active
worker's model and context during recovery unless the worker has returned control. For an external
session whose model cannot be controlled or verified, record `uncontrolled` instead of guessing.

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
owner: <task and worktree>
base: <commit or branch>
scope: <paths, subsystem, or question>
do-not-touch: <explicit exclusions>
acceptance: <verbatim clauses or exact commands>
return: <required evidence and format>
stop: <completion or escalation condition>
```

Use the current harness's delegation facility. When it supports agent trees, spawn workers under the
supervisor so it can inspect, steer, wait, and collect their results. For an independent external
session, provide the brief through the available channel and treat its registry, branch, worktree,
commits, and reports as observable evidence rather than assuming live steering.

## Supervision loop

Keep the main context on decisions and evidence. Do not copy raw exploration logs into it.

1. Inspect native agent state and `make agent-status` before assigning follow-up work.
2. At a meaningful boundary, collect the worker report below.
3. Check cited files, diffs, commands, exit status, and artifacts. A worker's conclusion is a claim,
   not integration evidence.
4. Send a bounded correction when evidence is missing or scope drifted. Reassign only the unfinished
   portion; do not restart accepted work.
5. Escalate a blocked dependency, contract deviation, authorization change, or overlapping claim.
6. Wait when no supervisor decision is needed; avoid polling agents merely to produce activity.

Workers return this schema:

```text
WORKER REPORT
task: <id>
role: <temporary mission or review lens>
state: <complete | blocked | needs-review>
execution: <actual model/capability and reasoning effort, inherited, or uncontrolled>
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
