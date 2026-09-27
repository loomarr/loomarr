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

const goRaceShardCount = 2

func TestGoShardUsesMeasuredLongestProcessingTime(t *testing.T) {
	t.Parallel()

	bin := t.TempDir()
	fakeGo := filepath.Join(bin, "go")
	// Deliberately scramble `go list` order. The emitted lane must retain the measured LPT work
	// order; otherwise the Go scheduler starts cheap packages first and creates a critical tail.
	const packages = `example.invalid/i
example.invalid/a
example.invalid/h
example.invalid/b
example.invalid/g
example.invalid/c
example.invalid/f
example.invalid/d
example.invalid/e`
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
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GO_SHARD_WEIGHTS="+weights, "GO_SHARD_CERTIFICATION="+isolated)
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

	cmd := exec.Command("bash", filepath.Join("scripts", "go-shard.sh"), "--worker-plan", "1")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GO_SHARD_WEIGHTS="+weights, "GO_SHARD_CERTIFICATION="+isolated)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("model bounded workers: %v\n%s", err, stderr.String())
	}
	if got := strings.TrimSpace(string(output)); got != "1 12" {
		t.Fatalf("bounded-worker plan = %q, want four-worker LPT makespan %q", got, "1 12")
	}
}

func TestGoRacePolicyPreservesSharderWorkOrder(t *testing.T) {
	t.Parallel()

	bin := t.TempDir()
	fakeGo := filepath.Join(bin, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env bash\nset -euo pipefail\n[[ \"$*\" == \"list -m\" ]]\necho example.invalid\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	run := func(mode string) string {
		t.Helper()
		cmd := exec.Command("bash", filepath.Join("scripts", "go-race-policy.sh"), mode)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		cmd.Stdin = strings.NewReader("example.invalid/z\nexample.invalid/internal/config\nexample.invalid/a\nexample.invalid/z\nexample.invalid/internal/setup\n")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("race policy %s: %v\n%s", mode, err, output)
		}
		return strings.TrimSpace(string(output))
	}

	if got, want := run("--race"), "example.invalid/z\nexample.invalid/a"; got != want {
		t.Fatalf("race work order =\n%s\nwant\n%s", got, want)
	}
	if got, want := run("--no-race"), "example.invalid/internal/config\nexample.invalid/internal/setup"; got != want {
		t.Fatalf("plain work order =\n%s\nwant\n%s", got, want)
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
	if err := os.WriteFile(weights, []byte("a 8\nmedia 7\nb 6\ncert 6\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	isolated := filepath.Join(t.TempDir(), "isolated.txt")
	if err := os.WriteFile(isolated, []byte("# lane reviewed media certification package\n1 media\n2 cert\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := filepath.Clean(filepath.Join("..", ".."))
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("bash", append([]string{filepath.Join("scripts", "go-shard.sh")}, args...)...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GO_SHARD_WEIGHTS="+weights, "GO_SHARD_CERTIFICATION="+isolated)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("go shard %v: %v\n%s", args, err, stderr.String())
		}
		return strings.TrimSpace(string(output))
	}

	if got := run("--certification", "1/2"); got != "example.invalid/media" {
		t.Fatalf("certification lane 1/2 =\n%s\nwant reviewed package", got)
	}
	if got := run("--certification", "2/2"); got != "example.invalid/cert" {
		t.Fatalf("certification lane 2/2 =\n%s\nwant reviewed package", got)
	}
	ordinary := run("1/1")
	if ordinary != "example.invalid/a\nexample.invalid/b" {
		t.Fatalf("ordinary lane =\n%s\nwant certification packages excluded", ordinary)
	}
	verification := run("--verify", "1")
	if !strings.Contains(verification, "1 ordinary shards plus 2 certification lanes cover all 4 packages, no duplicates") {
		t.Fatalf("verification did not prove exact three-lane coverage:\n%s", verification)
	}
}

func TestGoTestLanePinsBoundedParallelismAndIsolation(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "runner.log")
	makeLogPath := filepath.Join(t.TempDir(), "make.log")
	sharder := filepath.Join(bin, "sharder")
	policy := filepath.Join(bin, "policy")
	runner := filepath.Join(bin, "runner")
	makeBin := filepath.Join(bin, "make")
	if err := os.WriteFile(sharder, []byte("#!/usr/bin/env bash\nset -euo pipefail\nprintf 'example.invalid/race\\nexample.invalid/plain\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte("#!/usr/bin/env bash\nset -euo pipefail\ncase \"$1\" in --race) grep '/race$' ;; --no-race) grep '/plain$' ;; *) exit 2 ;; esac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runner, []byte("#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s|%s|%s\\n' \"${GO_TEST_LANE:-}\" \"${GOFLAGS:-}\" \"$*\" >> \"$GO_TEST_LOG\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(makeBin, []byte("#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\n' \"$*\" >> \"$GO_TEST_MAKE_LOG\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	baseEnv := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GOFLAGS=") {
			baseEnv = append(baseEnv, value)
		}
	}

	run := func(lane string, extra ...string) error {
		t.Helper()
		cmd := exec.Command("bash", filepath.Join("scripts", "go-test-lane.sh"))
		cmd.Dir = root
		cmd.Env = append(baseEnv,
			"GO_TEST_LANE="+lane,
			"GO_TEST_SHARDER="+sharder,
			"GO_TEST_RACE_POLICY="+policy,
			"GO_TEST_PACKAGE_RUNNER="+runner,
			"GO_TEST_MAKE_BIN="+makeBin,
			"GO_TEST_LOG="+logPath,
			"GO_TEST_MAKE_LOG="+makeLogPath,
		)
		cmd.Env = append(cmd.Env, extra...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, output)
		}
		return nil
	}

	if err := run("2/2"); err != nil {
		t.Fatalf("ordinary lane: %v", err)
	}
	if err := run("certification-1/2"); err != nil {
		t.Fatalf("certification lane 1/2: %v", err)
	}
	if err := run("certification-2/2"); err != nil {
		t.Fatalf("certification lane 2/2: %v", err)
	}
	if err := run("", "GOFLAGS=-count=1"); err != nil {
		t.Fatalf("unsharded local suite: %v", err)
	}
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "2/2|-p=4|race 25m example.invalid/race\n" +
		"2/2|-p=4|plain 25m example.invalid/plain\n" +
		"certification-1/2|-p=1|race 25m example.invalid/race\n" +
		"certification-1/2|-p=1|plain 25m example.invalid/plain\n" +
		"certification-2/2|-p=1|race 25m example.invalid/race\n" +
		"certification-2/2|-p=1|plain 25m example.invalid/plain\n" +
		"unsharded|-count=1|race 25m example.invalid/race\n" +
		"unsharded|-count=1|plain 25m example.invalid/plain\n"
	if got := string(contents); got != want {
		t.Fatalf("lane runner log =\n%s\nwant\n%s", got, want)
	}
	makeContents, err := os.ReadFile(makeLogPath)
	if err != nil {
		t.Fatal(err)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	wantMake := "-C " + absoluteRoot + " rust-test-worker eval-contract\n"
	if got := string(makeContents); got != wantMake {
		t.Fatalf("shared prerequisite log = %q, want exactly one unsharded invocation %q", got, wantMake)
	}
	if err := run("2/2", "GOFLAGS=-p=99"); err == nil {
		t.Fatal("ordinary lane accepted caller-controlled GOFLAGS")
	}
	if err := run("2/4"); err == nil {
		t.Fatal("ordinary lane accepted retired four-lane identity")
	}
}

func TestGoShardVerificationRejectsAggregateAndWorkerLatencyDrift(t *testing.T) {
	t.Parallel()

	bin := t.TempDir()
	fakeGo := filepath.Join(bin, "go")
	const packages = `example.invalid/a
example.invalid/b
example.invalid/cert-one
example.invalid/cert-two`
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"$*\" == \"list -m\" ]]; then echo example.invalid; exit; fi\n[[ \"$*\" == \"list ./...\" ]]\nprintf '%s\\n' '"+strings.ReplaceAll(packages, "\n", "' '")+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	weights := filepath.Join(t.TempDir(), "weights.tsv")
	certification := filepath.Join(t.TempDir(), "certification.tsv")
	if err := os.WriteFile(certification, []byte("1 cert-one\n2 cert-two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	run := func(weightRows, want string) {
		t.Helper()
		if err := os.WriteFile(weights, []byte(weightRows), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", filepath.Join("scripts", "go-shard.sh"), "--verify", "1")
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"GO_SHARD_WEIGHTS="+weights,
			"GO_SHARD_CERTIFICATION="+certification,
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("go shard verification accepted %s drift:\n%s", want, output)
		}
		if !strings.Contains(string(output), want) {
			t.Fatalf("go shard verification failure =\n%s\nwant %q", output, want)
		}
	}

	run("a 601\nb 600\ncert-one 1\ncert-two 1\n", "modeled aggregate split exceeds")
	run("a 550\nb 1\ncert-one 1\ncert-two 1\n", "modeled bounded-worker split exceeds")
}

// A dropped package is the failure every lane reports as green, so --verify must name it, and must
// name a package scheduled twice. Both are injected from outside the script: a manifest that lists
// one package in both certification lanes, and a `go list` that reports a package the sharder never
// sees (its first call is the verifier's own inventory; later calls feed the ordinary slices).
func TestGoShardVerificationRejectsPackagesInNoLaneOrTwoLanes(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	weights := filepath.Join(t.TempDir(), "weights.tsv")
	if err := os.WriteFile(weights, []byte("a 1\nb 1\ncert 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	verify := func(t *testing.T, fakeGo, certificationRows string) string {
		t.Helper()
		bin := t.TempDir()
		if err := os.WriteFile(filepath.Join(bin, "go"), []byte(fakeGo), 0o700); err != nil {
			t.Fatal(err)
		}
		certification := filepath.Join(t.TempDir(), "certification.tsv")
		if err := os.WriteFile(certification, []byte(certificationRows), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", filepath.Join("scripts", "go-shard.sh"), "--verify", "2")
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"GO_SHARD_WEIGHTS="+weights,
			"GO_SHARD_CERTIFICATION="+certification,
			"GO_SHARD_TEST_STATE="+filepath.Join(t.TempDir(), "listed"),
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("go shard verification accepted a broken partition:\n%s", output)
		}
		return string(output)
	}
	// section returns the package lines under one --verify report header.
	section := func(output, header string) string {
		_, after, found := strings.Cut(output, header)
		if !found {
			t.Fatalf("verification output lacks %q:\n%s", header, output)
		}
		before, _, _ := strings.Cut(after, "\n---")
		return strings.TrimSpace(before)
	}
	const missingHeader = "packages missing from every shard (these would go UNTESTED, green) ---"
	const duplicateHeader = "packages appearing more than once across shards (wasted, not unsafe) ---"

	t.Run("package in two lanes", func(t *testing.T) {
		t.Parallel()
		output := verify(t,
			"#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"$*\" == \"list -m\" ]]; then echo example.invalid; exit; fi\n[[ \"$*\" == \"list ./...\" ]]\nprintf '%s\\n' example.invalid/a example.invalid/b example.invalid/cert\n",
			"1 cert\n2 cert\n")
		if got := section(output, duplicateHeader); got != "example.invalid/cert" {
			t.Fatalf("duplicated packages = %q, want example.invalid/cert:\n%s", got, output)
		}
		if got := section(output, missingHeader); got != "" {
			t.Fatalf("missing packages = %q, want none:\n%s", got, output)
		}
	})
	t.Run("package in no lane", func(t *testing.T) {
		t.Parallel()
		output := verify(t,
			"#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"$*\" == \"list -m\" ]]; then echo example.invalid; exit; fi\n[[ \"$*\" == \"list ./...\" ]]\nprintf '%s\\n' example.invalid/a example.invalid/b example.invalid/cert\nif [[ ! -e \"$GO_SHARD_TEST_STATE\" ]]; then : > \"$GO_SHARD_TEST_STATE\"; echo example.invalid/unplaced; fi\n",
			"1 cert\n")
		if got := section(output, missingHeader); got != "example.invalid/unplaced" {
			t.Fatalf("missing packages = %q, want example.invalid/unplaced:\n%s", got, output)
		}
		if got := section(output, duplicateHeader); got != "" {
			t.Fatalf("duplicated packages = %q, want none:\n%s", got, output)
		}
	})
}

func TestGoCertificationLanePackageSetIsReviewed(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	contents, err := os.ReadFile(filepath.Join(root, "scripts", "go-certification-lanes.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	var packages []string
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if line != "" {
			packages = append(packages, strings.Join(strings.Fields(line), " "))
		}
	}
	want := []string{
		"1 internal/app",
		"2 internal/api",
		"2 internal/playout",
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
	certificationLoads := make([]int, 2)
	for lane := 1; lane <= 2; lane++ {
		cmd := exec.Command("bash", filepath.Join("scripts", "go-shard.sh"), "--certification", strconv.Itoa(lane)+"/2")
		cmd.Dir = root
		var stderr strings.Builder
		cmd.Stderr = &stderr
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("go certification lane %d/2: %v\n%s", lane, err, stderr.String())
		}
		for _, pkg := range strings.Fields(string(output)) {
			relative := strings.TrimPrefix(pkg, "github.com/loomarr/loomarr/")
			seenPackages[relative] = true
			certificationLoads[lane-1] += max(weights[relative], 1)
		}
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
	if maxLoad > 1200 {
		t.Fatalf("modeled ordinary shard exceeds aggregate package-work budget: loads=%v", loads)
	}
	if maxLoad*100 > minLoad*125 {
		t.Fatalf("modeled race shards differ by more than 25%%: loads=%v", loads)
	}
	cmd := exec.Command("bash", filepath.Join("scripts", "go-shard.sh"), "--worker-plan", strconv.Itoa(goRaceShardCount))
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("model bounded workers: %v", err)
	}
	workerLoads := make([]int, goRaceShardCount)
	workerFields := strings.Fields(string(output))
	if len(workerFields) != goRaceShardCount*2 {
		t.Fatalf("bounded-worker plan = %q, want %d lane/load rows", output, goRaceShardCount)
	}
	for field := 0; field < len(workerFields); field += 2 {
		lane, laneErr := strconv.Atoi(workerFields[field])
		load, loadErr := strconv.Atoi(workerFields[field+1])
		if laneErr != nil || loadErr != nil || lane < 1 || lane > goRaceShardCount {
			t.Fatalf("invalid bounded-worker plan row %q %q", workerFields[field], workerFields[field+1])
		}
		workerLoads[lane-1] = load
	}
	minWorkerLoad, maxWorkerLoad := workerLoads[0], workerLoads[0]
	for _, load := range workerLoads[1:] {
		minWorkerLoad = min(minWorkerLoad, load)
		maxWorkerLoad = max(maxWorkerLoad, load)
	}
	if maxWorkerLoad > 540 {
		t.Fatalf("modeled bounded-worker shard exceeds nine test minutes: loads=%v", workerLoads)
	}
	if maxWorkerLoad*100 > minWorkerLoad*125 {
		t.Fatalf("modeled bounded-worker shards differ by more than 25%%: loads=%v", workerLoads)
	}
	if max(certificationLoads[0], certificationLoads[1]) > 540 {
		t.Fatalf("modeled certification lane exceeds nine test minutes: loads=%v", certificationLoads)
	}
	if max(certificationLoads[0], certificationLoads[1])*100 > min(certificationLoads[0], certificationLoads[1])*125 {
		t.Fatalf("modeled certification lanes differ by more than 25%%: loads=%v", certificationLoads)
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
		cmd.Env = append(os.Environ(), "GO_BIN="+fakeGo, "GO_TEST_LANE=2/2", "GITHUB_STEP_SUMMARY="+summary, "FAKE_GO_EXIT="+exitCode)
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
	for _, want := range []string{"Go race package timings (2/2)", "`example.invalid/slow` | 12.500", "`example.invalid/fast` | 1.250", "Reported package total: 13.750s"} {
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

// TestGoTestPackagesCompileOnlyBuildsWithTheLaneFlags covers the cache-warming mode (#1570): it
// must compile the same packages with the same race flag the lane tests with, and execute nothing.
func TestGoTestPackagesCompileOnlyBuildsWithTheLaneFlags(t *testing.T) {
	t.Parallel()

	bin := t.TempDir()
	argsFile := filepath.Join(bin, "args")
	fakeGo := filepath.Join(bin, "go")
	if err := os.WriteFile(fakeGo, []byte(`#!/usr/bin/env bash
printf '%s\n' "$*" > "$FAKE_GO_ARGS"
echo 'ok  example.invalid/fast  0.001s'
`), 0o700); err != nil {
		t.Fatal(err)
	}

	root := filepath.Clean(filepath.Join("..", ".."))
	run := func(mode string, extraEnv ...string) string {
		t.Helper()
		summary := filepath.Join(t.TempDir(), "summary.md")
		cmd := exec.Command("bash", filepath.Join("scripts", "go-test-packages.sh"), mode, "25m", "example.invalid/fast")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), append([]string{"GO_BIN=" + fakeGo, "FAKE_GO_ARGS=" + argsFile, "GITHUB_STEP_SUMMARY=" + summary}, extraEnv...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go-test-packages.sh %s: %v\n%s", mode, err, output)
		}
		args, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(args))
	}

	if got, want := run("race", "GO_TEST_COMPILE_ONLY=1"), "test -timeout 25m -race -exec true example.invalid/fast"; got != want {
		t.Fatalf("compile-only race args = %q, want %q", got, want)
	}
	if got, want := run("plain", "GO_TEST_COMPILE_ONLY=1"), "test -timeout 25m -exec true example.invalid/fast"; got != want {
		t.Fatalf("compile-only plain args = %q, want %q", got, want)
	}
	if got := run("race"); strings.Contains(got, "-exec") {
		t.Fatalf("a real test run must execute its tests: args = %q", got)
	}
}

// TestOnlyTheCacheWarmerCompilesWithoutRunning keeps compile-only mode out of every gate: a gate
// that set it would build every test binary, execute none, and report green.
func TestOnlyTheCacheWarmerCompilesWithoutRunning(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	var sources []string
	for _, pattern := range []string{".github/workflows/*.yml", "Makefile", "mk/*.mk", "scripts/*.sh"} {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, matches...)
	}
	warmer := filepath.Join(root, ".github", "workflows", "ci-go-cache-warm.yml")
	compileOnlyOwners := map[string]bool{
		filepath.Join(root, "mk", "cache-warm.mk"):            true,
		filepath.Join(root, "scripts", "go-test-packages.sh"): true,
	}
	for _, path := range sources {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "GO_TEST_COMPILE_ONLY") && !compileOnlyOwners[path] {
			t.Errorf("%s sets GO_TEST_COMPILE_ONLY; only mk/cache-warm.mk may compile tests without running them", path)
		}
		if strings.HasSuffix(path, ".yml") && strings.Contains(string(data), "go-cache-warm") != (path == warmer) {
			t.Errorf("%s: only ci-go-cache-warm.yml may run make go-cache-warm, and it must", path)
		}
		if strings.Contains(string(data), "-exec true") && path != filepath.Join(root, "scripts", "go-test-packages.sh") {
			t.Errorf("%s runs go test with -exec true; compile-only belongs to go-test-packages.sh behind GO_TEST_COMPILE_ONLY", path)
		}
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
