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
	// internal/config is on the race policy's opt-out list, so it runs in the plain group after the
	// race group and adds to the lane's makespan.
	const packages = `example.invalid/a
example.invalid/b
example.invalid/c
example.invalid/d
example.invalid/internal/config
example.invalid/cert-one
example.invalid/cert-three
example.invalid/cert-two`
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"$*\" == \"list -m\" ]]; then echo example.invalid; exit; fi\n[[ \"$*\" == \"list ./...\" ]]\nprintf '%s\\n' '"+strings.ReplaceAll(packages, "\n", "' '")+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	certification := filepath.Join(t.TempDir(), "certification.tsv")
	if err := os.WriteFile(certification, []byte("1 cert-one\n1 cert-three\n2 cert-two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	budgets := goShardBudgets(t)
	lane, aggregate := budgets["lane_test"], budgets["ordinary_aggregate"]
	verify := func(t *testing.T, weightRows string) (string, error) {
		t.Helper()
		weights := filepath.Join(t.TempDir(), "weights.tsv")
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
		return string(output), err
	}
	certs := "cert-one 1\ncert-three 1\ncert-two 2\n"

	rejected := []struct{ name, weights, want string }{
		// A package is the sharder's indivisible unit: none may outlast a lane's whole test step.
		{"package over a lane's test step", fmt.Sprintf("a %d\n%s", lane+1, certs),
			fmt.Sprintf("exceeds the %ds per-package cap", lane)},
		{"aggregate over four workers' lane budget", fmt.Sprintf("a %[1]d\nb %[1]d\nc %[1]d\nd %[1]d\ninternal/config 5\n%[2]s", lane, certs),
			"modeled aggregate split exceeds"},
		{"plain group pushes a lane past the lane budget", fmt.Sprintf("a %d\ninternal/config 20\n%s", lane-10, certs),
			fmt.Sprintf("shard 1: %ds (budget %ds)", lane+10, lane)},
		{"serial certification lane past the lane budget", fmt.Sprintf("cert-one %[1]d\ncert-three %[1]d\ncert-two %[2]d\n", lane/2+10, lane-1),
			fmt.Sprintf("certification lane 1/2: %ds (budget %ds)", 2*(lane/2+10), lane)},
	}
	if aggregate != 4*lane {
		t.Fatalf("aggregate fixture assumes ordinary_aggregate = 4 x lane_test: %v", budgets)
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			output, err := verify(t, tc.weights)
			if err == nil {
				t.Fatalf("go shard verification accepted %s:\n%s", tc.name, output)
			}
			if !strings.Contains(output, tc.want) {
				t.Fatalf("go shard verification failure =\n%s\nwant %q", output, tc.want)
			}
		})
	}
	t.Run("lane exactly at its test step", func(t *testing.T) {
		t.Parallel()
		output, err := verify(t, fmt.Sprintf("a %d\n%s", lane-1, certs))
		if err != nil {
			t.Fatalf("go shard verification rejected a lane at its budget: %v\n%s", err, output)
		}
		// +1: internal/config's one-second floor runs in the plain group after the race group.
		if want := fmt.Sprintf("bounded-worker makespan = %ds (budget %ds)", lane, lane); !strings.Contains(output, want) {
			t.Fatalf("verification output =\n%s\nwant %q", output, want)
		}
	})
}

// The budgets are derived from #1570's ten-minute Go-only queue target, not free constants:
// the lane test step is the target minus the measured queue overhead, and four workers share it.
func TestGoShardBudgetsDeriveFromTheQueueTarget(t *testing.T) {
	t.Parallel()

	b := goShardBudgets(t)
	if b["target_queue"] != 600 {
		t.Fatalf("target_queue = %d, want #1570's 600s Go-only merge-queue target", b["target_queue"])
	}
	if b["queue_overhead"] <= 0 || b["lane_test"] != b["target_queue"]-b["queue_overhead"] {
		t.Fatalf("lane_test = %d, want target_queue - queue_overhead (%d - %d)", b["lane_test"], b["target_queue"], b["queue_overhead"])
	}
	if b["ordinary_aggregate"] != 4*b["lane_test"] {
		t.Fatalf("ordinary_aggregate = %d, want four -p=4 workers x lane_test %d", b["ordinary_aggregate"], b["lane_test"])
	}
	// No package gets more than a lane's test step: an outgrown package is split, not budgeted.
	if b["max_package"] != b["lane_test"] {
		t.Fatalf("max_package = %d, want the lane test step %d", b["max_package"], b["lane_test"])
	}
}

func goShardBudgets(t *testing.T) map[string]int {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join("scripts", "go-shard.sh"), "--budgets")
	cmd.Dir = filepath.Clean(filepath.Join("..", ".."))
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go shard budgets: %v", err)
	}
	budgets := make(map[string]int)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		seconds, convErr := strconv.Atoi(fields[len(fields)-1])
		if len(fields) != 2 || convErr != nil {
			t.Fatalf("invalid budget row %q", line)
		}
		budgets[fields[0]] = seconds
	}
	return budgets
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
		"1 internal/api",
		"1 internal/playout",
		"2 internal/store",
		"2 internal/integration",
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

	budgets := goShardBudgets(t)
	minLoad, maxLoad := loads[0], loads[0]
	for _, load := range loads[1:] {
		minLoad = min(minLoad, load)
		maxLoad = max(maxLoad, load)
	}
	if maxLoad > budgets["ordinary_aggregate"] {
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
	if maxWorkerLoad > budgets["lane_test"] {
		t.Fatalf("modeled bounded-worker shard exceeds the %ds lane budget: loads=%v", budgets["lane_test"], workerLoads)
	}
	if maxWorkerLoad*100 > minWorkerLoad*125 {
		t.Fatalf("modeled bounded-worker shards differ by more than 25%%: loads=%v", workerLoads)
	}
	if max(certificationLoads[0], certificationLoads[1]) > budgets["lane_test"] {
		t.Fatalf("modeled certification lane exceeds the %ds lane budget: loads=%v", budgets["lane_test"], certificationLoads)
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

// The weight file is regenerated from hosted runs, never hand-kept: the median of each package's
// `ok` time over successful race-lane jobs, rounded up, with sub-five-second packages left to the
// sharder's floor. Failed lanes and other Go jobs must not contribute samples.
func TestGoRaceWeightsRefreshTakesMedianOfSuccessfulLaneJobs(t *testing.T) {
	t.Parallel()

	fixtures := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(fixtures, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lane := func(name string) string { return "Go — race-policy tests / Go — race-policy tests (" + name + ")" }
	ok := func(job, pkg, seconds string) string {
		return job + "\tUNKNOWN STEP\t2026-09-27T16:36:20.2344716Z ok  \tgithub.com/loomarr/loomarr/" + pkg + "\t" + seconds + "\n"
	}
	write("11.jobs", "101\tsuccess\t"+lane("1/2")+"\n102\tfailure\t"+lane("2/2")+"\n103\tsuccess\tGo — repository contracts / Go — repository contracts\n")
	write("22.jobs", "201\tsuccess\t"+lane("certification-1/2")+"\n202\tsuccess\t"+lane("2/2")+"\n")
	write("33.jobs", "301\tsuccess\t"+lane("1/2")+"\n")
	write("101.log", ok("j", "internal/store", "100.1s")+ok("j", "internal/small", "3.0s")+ok("j", "internal/alpha", "10.000s")+
		"j\tUNKNOWN STEP\t2026-09-27T16:36:20Z ok  \tgithub.com/loomarr/loomarr/internal/cachedpkg\t(cached)\n")
	write("102.log", ok("j", "internal/store", "999s"))
	write("103.log", ok("j", "internal/store", "999s"))
	write("201.log", ok("j", "internal/app", "50.2s"))
	write("202.log", ok("j", "internal/store", "90s"))
	write("301.log", ok("j", "internal/store", "120.5s")+ok("j", "internal/alpha", "11s"))

	bin := t.TempDir()
	fakeGh := "#!/usr/bin/env bash\nset -euo pipefail\n[[ \"$1 $2\" == \"run view\" ]]\n" +
		"if [[ \"$4\" == --json ]]; then cat '" + fixtures + "'/\"$3\".jobs; exit; fi\n" +
		"[[ \"$4 $6\" == \"--job --log\" ]]\ncat '" + fixtures + "'/\"$5\".log\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(fakeGh), 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	refresh := func(runs ...string) (string, error) {
		cmd := exec.Command("bash", append([]string{filepath.Join("scripts", "go-race-weights-refresh.sh")}, runs...)...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GO_RACE_WEIGHTS_GH="+filepath.Join(bin, "gh"))
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	output, err := refresh("11", "22", "33")
	if err != nil {
		t.Fatalf("refresh: %v\n%s", err, output)
	}
	var rows []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if !strings.HasPrefix(line, "#") {
			rows = append(rows, line)
		}
	}
	// store: median(100.1, 90, 120.5) = 100.1 -> 101; alpha: median(10, 11) = 10.5 -> 11.
	want := []string{"internal/store\t101", "internal/app\t51", "internal/alpha\t11"}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("weights =\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(output, "merge-group runs 11 22 33") {
		t.Fatalf("header does not name its source runs:\n%s", output)
	}

	if output, err := refresh("33x"); err == nil {
		t.Fatalf("refresh accepted an invalid run id:\n%s", output)
	}
	write("44.jobs", "401\tfailure\t"+lane("1/2")+"\n")
	if output, err := refresh("44"); err == nil || !strings.Contains(output, "no successful race-policy lane timings") {
		t.Fatalf("refresh with no successful lane = %v, want a loud failure:\n%s", err, output)
	}
}
