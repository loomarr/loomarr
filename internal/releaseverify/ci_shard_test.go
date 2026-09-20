package releaseverify

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const goRaceShardCount = 6

func TestGoShardUsesMeasuredLongestProcessingTime(t *testing.T) {
	t.Parallel()

	bin := t.TempDir()
	fakeGo := filepath.Join(bin, "go")
	const packages = `example.invalid/a
example.invalid/b
example.invalid/c
example.invalid/d
example.invalid/e
example.invalid/f
example.invalid/g
example.invalid/h
example.invalid/i`
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"$*\" == \"list -m\" ]]; then echo example.invalid; exit; fi\n[[ \"$*\" == \"list ./...\" ]]\necho 'go: downloading example.invalid/dependency v1.0.0' >&2\nprintf '%s\\n' '"+strings.ReplaceAll(packages, "\n", "' '")+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	weights := filepath.Join(t.TempDir(), "weights.tsv")
	if err := os.WriteFile(weights, []byte("a 9\nb 8\nc 7\nd 6\ne 5\nf 4\ng 3\nh 2\ni 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	isolated := filepath.Join(t.TempDir(), "isolated.txt")
	if err := os.WriteFile(isolated, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	root := filepath.Clean(filepath.Join("..", ".."))
	want := map[string]string{
		"1/3": "example.invalid/a\nexample.invalid/f\nexample.invalid/g",
		"2/3": "example.invalid/b\nexample.invalid/e\nexample.invalid/h",
		"3/3": "example.invalid/c\nexample.invalid/d\nexample.invalid/i",
	}
	for shard, expected := range want {
		shard, expected := shard, expected
		t.Run(shard, func(t *testing.T) {
			cmd := exec.Command("bash", filepath.Join("scripts", "go-shard.sh"), shard)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GO_SHARD_WEIGHTS="+weights, "GO_SHARD_ISOLATED="+isolated)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("go shard %s: %v\n%s", shard, err, stderr.String())
			}
			if got := strings.TrimSpace(string(output)); got != expected {
				t.Fatalf("go shard %s =\n%s\nwant measured longest-processing-time assignment\n%s", shard, got, expected)
			}
		})
	}
}

func TestGoShardSeparatesLatencySensitiveCertificationPackages(t *testing.T) {
	t.Parallel()

	bin := t.TempDir()
	fakeGo := filepath.Join(bin, "go")
	const packages = `example.invalid/a
example.invalid/media
example.invalid/b
example.invalid/cert`
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"$*\" == \"list -m\" ]]; then echo example.invalid; exit; fi\n[[ \"$*\" == \"list ./...\" ]]\nprintf '%s\\n' '"+strings.ReplaceAll(packages, "\n", "' '")+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	weights := filepath.Join(t.TempDir(), "weights.tsv")
	if err := os.WriteFile(weights, []byte("a 8\nmedia 7\nb 6\ncert 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	isolated := filepath.Join(t.TempDir(), "isolated.txt")
	if err := os.WriteFile(isolated, []byte("# reviewed media certification packages\nmedia\ncert\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := filepath.Clean(filepath.Join("..", ".."))
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("bash", append([]string{filepath.Join("scripts", "go-shard.sh")}, args...)...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GO_SHARD_WEIGHTS="+weights, "GO_SHARD_ISOLATED="+isolated)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("go shard %v: %v\n%s", args, err, stderr.String())
		}
		return strings.TrimSpace(string(output))
	}

	if got := run("--isolated"); got != "example.invalid/media\nexample.invalid/cert" {
		t.Fatalf("isolated lane =\n%s\nwant reviewed package order", got)
	}
	ordinary := run("1/1")
	if ordinary != "example.invalid/a\nexample.invalid/b" {
		t.Fatalf("ordinary lane =\n%s\nwant certification packages excluded", ordinary)
	}
	verification := run("--verify", "1")
	if !strings.Contains(verification, "1 ordinary shards plus certification cover all 4 packages, no duplicates") {
		t.Fatalf("verification did not prove exact seven-lane coverage:\n%s", verification)
	}
}

func TestGoTestLanePinsBoundedParallelismAndIsolation(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "runner.log")
	sharder := filepath.Join(bin, "sharder")
	policy := filepath.Join(bin, "policy")
	runner := filepath.Join(bin, "runner")
	if err := os.WriteFile(sharder, []byte("#!/usr/bin/env bash\nset -euo pipefail\nprintf 'example.invalid/race\\nexample.invalid/plain\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte("#!/usr/bin/env bash\nset -euo pipefail\ncase \"$1\" in --race) grep '/race$' ;; --no-race) grep '/plain$' ;; *) exit 2 ;; esac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runner, []byte("#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s|%s|%s\\n' \"${GO_TEST_LANE:-}\" \"${GOFLAGS:-}\" \"$*\" >> \"$GO_TEST_LOG\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	run := func(lane string, extra ...string) error {
		t.Helper()
		cmd := exec.Command("bash", filepath.Join("scripts", "go-test-lane.sh"))
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GO_TEST_LANE="+lane,
			"GO_TEST_SHARDER="+sharder,
			"GO_TEST_RACE_POLICY="+policy,
			"GO_TEST_PACKAGE_RUNNER="+runner,
			"GO_TEST_LOG="+logPath,
		)
		cmd.Env = append(cmd.Env, extra...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, output)
		}
		return nil
	}

	if err := run("2/6"); err != nil {
		t.Fatalf("ordinary lane: %v", err)
	}
	if err := run("isolated"); err != nil {
		t.Fatalf("certification lane: %v", err)
	}
	if err := run("", "GOFLAGS=-count=1"); err != nil {
		t.Fatalf("unsharded local suite: %v", err)
	}
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "2/6|-p=2|race 25m example.invalid/race\n" +
		"2/6|-p=2|plain 25m example.invalid/plain\n" +
		"isolated|-p=1|race 25m example.invalid/race\n" +
		"isolated|-p=1|plain 25m example.invalid/plain\n" +
		"unsharded|-count=1|race 25m example.invalid/race\n" +
		"unsharded|-count=1|plain 25m example.invalid/plain\n"
	if got := string(contents); got != want {
		t.Fatalf("lane runner log =\n%s\nwant\n%s", got, want)
	}
	if err := run("2/6", "GOFLAGS=-p=99"); err == nil {
		t.Fatal("ordinary lane accepted caller-controlled GOFLAGS")
	}
}

func TestGoCertificationLanePackageSetIsReviewed(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	contents, err := os.ReadFile(filepath.Join(root, "scripts", "go-isolated-packages.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var packages []string
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line != "" {
			packages = append(packages, line)
		}
	}
	want := []string{
		"cmd/playout-load-cert",
		"internal/app",
		"internal/playout",
		"internal/playoutcert",
		"internal/prepared",
		"internal/testkit/playoutcertfixture",
	}
	if strings.Join(packages, "\n") != strings.Join(want, "\n") {
		t.Fatalf("certification lane packages = %v, want reviewed set %v", packages, want)
	}
}

func TestGoShardBalancesMeasuredRaceWork(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	weights := readGoRaceWeights(t, filepath.Join(root, "scripts", "go-race-weights.tsv"))
	loads := make([]int, goRaceShardCount)
	seenPackages := make(map[string]bool)

	for shard := 1; shard <= goRaceShardCount; shard++ {
		cmd := exec.Command("bash", filepath.Join("scripts", "go-shard.sh"), strconv.Itoa(shard)+"/"+strconv.Itoa(goRaceShardCount))
		cmd.Dir = root
		var stderr strings.Builder
		cmd.Stderr = &stderr
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("go shard %d/%d: %v\n%s", shard, goRaceShardCount, err, stderr.String())
		}
		for _, pkg := range strings.Fields(string(output)) {
			relative := strings.TrimPrefix(pkg, "github.com/loomarr/loomarr/")
			seenPackages[relative] = true
			loads[shard-1] += max(weights[relative], 1)
		}
	}
	cmd := exec.Command("bash", filepath.Join("scripts", "go-shard.sh"), "--isolated")
	cmd.Dir = root
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go certification lane: %v\n%s", err, stderr.String())
	}
	isolatedLoad := 0
	for _, pkg := range strings.Fields(string(output)) {
		relative := strings.TrimPrefix(pkg, "github.com/loomarr/loomarr/")
		seenPackages[relative] = true
		isolatedLoad += max(weights[relative], 1)
	}
	for weightedPackage := range weights {
		if !seenPackages[weightedPackage] {
			t.Errorf("measured race weight names missing package %q", weightedPackage)
		}
	}

	minLoad, maxLoad := loads[0], loads[0]
	for _, load := range loads[1:] {
		minLoad = min(minLoad, load)
		maxLoad = max(maxLoad, load)
	}
	if maxLoad > 540 {
		t.Fatalf("modeled race shard exceeds nine test minutes: loads=%v", loads)
	}
	if maxLoad*100 > minLoad*125 {
		t.Fatalf("modeled race shards differ by more than 25%%: loads=%v", loads)
	}
	if isolatedLoad > 540 {
		t.Fatalf("modeled certification lane exceeds nine test minutes: load=%d", isolatedLoad)
	}
}

func TestGoTestPackagesPublishesTimingSummaryAndPreservesFailure(t *testing.T) {
	t.Parallel()

	bin := t.TempDir()
	fakeGo := filepath.Join(bin, "go")
	if err := os.WriteFile(fakeGo, []byte(`#!/usr/bin/env bash
set -euo pipefail
echo 'ok  example.invalid/fast  1.250s'
echo 'ok  example.invalid/slow  12.500s'
exit "${FAKE_GO_EXIT:-0}"
`), 0o700); err != nil {
		t.Fatal(err)
	}

	root := filepath.Clean(filepath.Join("..", ".."))
	summary := filepath.Join(t.TempDir(), "summary.md")
	run := func(exitCode string) error {
		cmd := exec.Command("bash", filepath.Join("scripts", "go-test-packages.sh"), "race", "25m", "example.invalid/fast", "example.invalid/slow")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GO_BIN="+fakeGo, "GO_TEST_LANE=2/6", "GITHUB_STEP_SUMMARY="+summary, "FAKE_GO_EXIT="+exitCode)
		return cmd.Run()
	}
	if err := run("0"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"Go race package timings (2/6)", "`example.invalid/slow` | 12.500", "`example.invalid/fast` | 1.250", "Reported package total: 13.750s"} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary missing %q:\n%s", want, text)
		}
	}
	if err := run("7"); err == nil {
		t.Fatal("go-test-packages.sh hid the go test failure")
	} else if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 7 {
		t.Fatalf("go-test-packages.sh failure = %v, want exit 7", err)
	}
}

func readGoRaceWeights(t *testing.T, path string) map[string]int {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close race weights: %v", err)
		}
	})

	weights := make(map[string]int)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("invalid race weight row %q", line)
		}
		weight, err := strconv.Atoi(fields[1])
		if err != nil || weight < 1 {
			t.Fatalf("invalid race weight row %q", line)
		}
		if _, exists := weights[fields[0]]; exists {
			t.Fatalf("duplicate race weight row %q", line)
		}
		weights[fields[0]] = weight
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return weights
}
