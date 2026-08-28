package testkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// FakeDockerConfig controls the system-boundary double used by tests for code
// that inspects and acquires container images. PullStatuses is consumed once per
// pull; an extra attempt fails distinctly so a retry bound cannot pass silently.
type FakeDockerConfig struct {
	InspectStatus int
	PullStatuses  []int
	SleepStatuses []int
	RunStatus     int
	Environment   []string
	Dir           string
}

// FakeDockerResult records only caller-visible process behavior. The double
// never invokes a container runtime or network service.
type FakeDockerResult struct {
	Err            error
	PullAttempts   int
	SleepCalls     []int
	InspectImages  []string
	PullImages     []string
	DockerCommands [][]string
}

// RunWithFakeDocker executes one command with Docker and sleep replaced at PATH
// by shared stateful doubles. Tests remain responsible for assertions about the
// command's public behavior rather than the doubles' implementation.
func RunWithFakeDocker(t *testing.T, executable string, args []string, config FakeDockerConfig) FakeDockerResult {
	t.Helper()
	// The full race-enabled suite can leave these short-lived bash probes waiting for CPU while
	// many packages compile and test in parallel. Keep a hard deadlock bound, but do not confuse
	// scheduler contention with a hung helper.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := ProbeWithFakeDocker(ctx, executable, args, config)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// ProbeWithFakeDocker executes a process behind the same fake Docker/sleep
// boundary as RunWithFakeDocker. Production code must not import testkit; this
// non-testing.T form lets tests share the process seam where needed.
func ProbeWithFakeDocker(ctx context.Context, executable string, args []string, config FakeDockerConfig) (FakeDockerResult, error) {
	tmp, err := os.MkdirTemp("", "loomarr-fake-docker-*")
	if err != nil {
		return FakeDockerResult{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	pullCount := filepath.Join(tmp, "pull-count")
	inspectLog := filepath.Join(tmp, "inspect-images.log")
	pullLog := filepath.Join(tmp, "pull-images.log")
	commandLog := filepath.Join(tmp, "docker-commands.log")
	sleepLog := filepath.Join(tmp, "sleep.log")
	docker := filepath.Join(tmp, "docker")
	if err := writeProbeExecutable(docker, `#!/usr/bin/env bash
set -u
printf '%s\0' "$@" >>"$FAKE_DOCKER_COMMAND_LOG"
printf '\0' >>"$FAKE_DOCKER_COMMAND_LOG"
if [[ "$1" == "image" && "$2" == "inspect" ]]; then
  printf '%s\n' "$3" >>"$FAKE_DOCKER_INSPECT_LOG"
  exit "$FAKE_DOCKER_INSPECT_STATUS"
fi
if [[ "$1" == "pull" ]]; then
	printf '%s\n' "$2" >>"$FAKE_DOCKER_PULL_LOG"
  count=0
  if [[ -f "$FAKE_DOCKER_PULL_COUNT" ]]; then
    count=$(<"$FAKE_DOCKER_PULL_COUNT")
  fi
  count=$((count + 1))
  printf '%s\n' "$count" >"$FAKE_DOCKER_PULL_COUNT"
  IFS=',' read -r -a statuses <<<"$FAKE_DOCKER_PULL_STATUSES"
  index=$((count - 1))
  if [[ $index -ge ${#statuses[@]} ]]; then
    exit 96
  fi
  exit "${statuses[$index]}"
fi
if [[ "$1" == "run" ]]; then
  exit "$FAKE_DOCKER_RUN_STATUS"
fi
exit 97
`); err != nil {
		return FakeDockerResult{}, err
	}
	sleep := filepath.Join(tmp, "sleep")
	if err := writeProbeExecutable(sleep, `#!/usr/bin/env bash
set -u
printf '%s\n' "$1" >>"$FAKE_SLEEP_LOG"
if [[ -z "$FAKE_SLEEP_STATUSES" ]]; then
  exit 0
fi
count=$(wc -l <"$FAKE_SLEEP_LOG")
IFS=',' read -r -a statuses <<<"$FAKE_SLEEP_STATUSES"
index=$((count - 1))
if [[ $index -ge ${#statuses[@]} ]]; then
  exit 95
fi
exit "${statuses[$index]}"
`); err != nil {
		return FakeDockerResult{}, err
	}

	statuses := make([]string, len(config.PullStatuses))
	for i, status := range config.PullStatuses {
		statuses[i] = strconv.Itoa(status)
	}
	sleepStatuses := make([]string, len(config.SleepStatuses))
	for i, status := range config.SleepStatuses {
		sleepStatuses[i] = strconv.Itoa(status)
	}
	command := exec.CommandContext(ctx, executable, args...) //nolint:gosec // caller-controlled repository helper behind a fake PATH
	command.Dir = config.Dir
	command.Env = append(os.Environ(),
		"PATH="+filepath.Dir(docker)+string(os.PathListSeparator)+filepath.Dir(sleep)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_DOCKER_INSPECT_STATUS="+strconv.Itoa(config.InspectStatus),
		"FAKE_DOCKER_PULL_STATUSES="+strings.Join(statuses, ","),
		"FAKE_DOCKER_PULL_COUNT="+pullCount,
		"FAKE_DOCKER_INSPECT_LOG="+inspectLog,
		"FAKE_DOCKER_PULL_LOG="+pullLog,
		"FAKE_DOCKER_COMMAND_LOG="+commandLog,
		"FAKE_DOCKER_RUN_STATUS="+strconv.Itoa(config.RunStatus),
		"FAKE_SLEEP_LOG="+sleepLog,
		"FAKE_SLEEP_STATUSES="+strings.Join(sleepStatuses, ","),
	)
	command.Env = append(command.Env, config.Environment...)
	runErr := command.Run()
	if ctx.Err() != nil {
		return FakeDockerResult{}, fmt.Errorf("fake Docker process exceeded its deadline: %w", ctx.Err())
	}
	pulls, err := readFakeDockerCount(pullCount)
	if err != nil {
		return FakeDockerResult{}, err
	}
	sleeps, err := readFakeDockerSleeps(sleepLog)
	if err != nil {
		return FakeDockerResult{}, err
	}

	inspectImages, err := readFakeDockerStrings(inspectLog)
	if err != nil {
		return FakeDockerResult{}, err
	}
	pullImages, err := readFakeDockerStrings(pullLog)
	if err != nil {
		return FakeDockerResult{}, err
	}
	dockerCommands, err := readFakeDockerCommands(commandLog)
	if err != nil {
		return FakeDockerResult{}, err
	}

	return FakeDockerResult{
		Err: runErr, PullAttempts: pulls, SleepCalls: sleeps,
		InspectImages: inspectImages, PullImages: pullImages, DockerCommands: dockerCommands,
	}, nil
}

func readFakeDockerCommands(path string) ([][]string, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fields := strings.Split(string(contents), "\x00")
	commands := make([][]string, 0)
	current := make([]string, 0)
	for _, field := range fields {
		if field == "" {
			if len(current) > 0 {
				commands = append(commands, current)
				current = nil
			}
			continue
		}
		current = append(current, field)
	}
	return commands, nil
}

func writeProbeExecutable(path, script string) error {
	return os.WriteFile(path, []byte(script), 0o700)
}

func readFakeDockerCount(path string) (int, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil {
		return 0, err
	}
	return count, nil
}

func readFakeDockerSleeps(path string) ([]int, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(string(contents))
	out := make([]int, len(lines))
	for i, line := range lines {
		out[i], err = strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func readFakeDockerStrings(path string) ([]string, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(contents)), nil
}
