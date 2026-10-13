package testkit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// FakeGoConfig controls the system-boundary double used by tests for code that
// downloads the Go module graph. DownloadStatuses is consumed once per
// `go mod download`; an extra attempt fails distinctly so a retry bound cannot
// pass silently.
type FakeGoConfig struct {
	DownloadStatuses []int
	SleepStatuses    []int
}

// FakeGoResult records only caller-visible process behavior. The double never
// reaches a module proxy or the real toolchain.
type FakeGoResult struct {
	Err              error
	DownloadAttempts int
	SleepCalls       []int
	GoCommands       [][]string
}

// RunWithFakeGo executes one command with go and sleep replaced at PATH by
// stateful doubles.
func RunWithFakeGo(t *testing.T, executable string, args []string, config FakeGoConfig) FakeGoResult {
	t.Helper()
	// Same deadlock bound as RunWithFakeDocker: generous enough for a loaded race suite.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tmp := t.TempDir()
	downloadCount := filepath.Join(tmp, "download-count")
	commandLog := filepath.Join(tmp, "go-commands.log")
	sleepLog := filepath.Join(tmp, "sleep.log")
	if err := writeProbeExecutable(filepath.Join(tmp, "go"), `#!/usr/bin/env bash
set -u
printf '%s\0' "$@" >>"$FAKE_GO_COMMAND_LOG"
printf '\0' >>"$FAKE_GO_COMMAND_LOG"
if [[ $# -eq 2 && "$1" == "mod" && "$2" == "download" ]]; then
  count=0
  if [[ -f "$FAKE_GO_DOWNLOAD_COUNT" ]]; then
    count=$(<"$FAKE_GO_DOWNLOAD_COUNT")
  fi
  count=$((count + 1))
  printf '%s\n' "$count" >"$FAKE_GO_DOWNLOAD_COUNT"
  IFS=',' read -r -a statuses <<<"$FAKE_GO_DOWNLOAD_STATUSES"
  index=$((count - 1))
  if [[ $index -ge ${#statuses[@]} ]]; then
    exit 96
  fi
  exit "${statuses[$index]}"
fi
exit 97
`); err != nil {
		t.Fatal(err)
	}
	if err := writeProbeExecutable(filepath.Join(tmp, "sleep"), fakeSleepScript); err != nil {
		t.Fatal(err)
	}

	command := exec.CommandContext(ctx, executable, args...) //nolint:gosec // caller-controlled repository helper behind a fake PATH
	command.Env = append(os.Environ(),
		"PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GO_DOWNLOAD_STATUSES="+joinStatuses(config.DownloadStatuses),
		"FAKE_GO_DOWNLOAD_COUNT="+downloadCount,
		"FAKE_GO_COMMAND_LOG="+commandLog,
		"FAKE_SLEEP_LOG="+sleepLog,
		"FAKE_SLEEP_STATUSES="+joinStatuses(config.SleepStatuses),
	)
	runErr := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("fake go process exceeded its deadline: %v", ctx.Err())
	}
	downloads, err := readFakeDockerCount(downloadCount)
	if err != nil {
		t.Fatal(err)
	}
	sleeps, err := readFakeDockerSleeps(sleepLog)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := readFakeDockerCommands(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	return FakeGoResult{Err: runErr, DownloadAttempts: downloads, SleepCalls: sleeps, GoCommands: commands}
}
