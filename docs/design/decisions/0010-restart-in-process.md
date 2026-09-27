# 0010. Restart rebuilds in process, never exits

- **Date:** 2026-07-29
- **Status:** accepted

## Context

Applying some settings needs a restart (V13). Three mechanisms were possible: `execve` into a new
image of the binary, rebuilding the application inside the same process, or exiting and letting a
supervisor start it again. An operator must never be left with a dead service and no way back.

## Decision

Rebuild in process: `main` loops `Build → Run → Shutdown`, and a restart ends one iteration so the next
builds fresh. Same PID, no re-exec, no supervisor required. Jellyfin does the same.

## Consequences

- Exit-and-restart is avoided because it assumes a supervisor exists (false for `make dev-be` or a bare
  binary), and exit-code contracts differ between Docker and systemd, with exponential backoff.
- Package-level mutable state becomes a correctness constraint: global registries and package-level
  `sync.Once` are banned, and a repeated-iteration goroutine-leak test is the gate.
- Shutdown must quiesce endless responses before draining HTTP, or the two wait on each other.
