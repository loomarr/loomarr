package releaseverify

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// shardOutput runs go-shard.sh in the repository and returns its stdout words.
func shardOutput(t *testing.T, root string, args ...string) []string {
	t.Helper()
	cmd := exec.Command("bash", append([]string{filepath.Join("scripts", "go-shard.sh")}, args...)...)
	cmd.Dir = root
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("go-shard.sh %v: %v\n%s", args, err, stderr.String())
	}
	return strings.Fields(string(output))
}

// Every package is scheduled in each phase the race policy gives it: by an ordinary or serial lane,
// which runs both, or by split jobs, which must hold every race slice and the plain run. Every split
// job is also in the workflow matrix.
func TestGoShardSchedulesEveryPackageInEveryRacePolicyPhase(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", "..", ".."))
	list := exec.Command("go", "list", "./...")
	list.Dir = root
	listed, err := list.Output()
	if err != nil {
		t.Fatal(err)
	}
	policy := func(flag string) []string {
		cmd := exec.Command("bash", filepath.Join("scripts", "go-race-policy.sh"), flag)
		cmd.Dir = root
		cmd.Stdin = strings.NewReader(string(listed))
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("go-race-policy.sh %s: %v", flag, err)
		}
		return strings.Fields(string(output))
	}

	var bothPhases []string // packages in a lane that runs whatever phases the policy gives them
	for shard := range goRaceShardCount {
		bothPhases = append(bothPhases, shardOutput(t, root, []string{"1/2", "2/2"}[shard])...)
	}
	for _, spec := range []string{"1/4", "2/4", "3/4", "4/4"} {
		bothPhases = append(bothPhases, shardOutput(t, root, "--certification", spec)...)
	}
	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci-go.yml"))
	if err != nil {
		t.Fatal(err)
	}
	scheduled := map[string]map[string]bool{"race": {}, "plain": {}}
	for _, pkg := range bothPhases {
		scheduled["race"][pkg], scheduled["plain"][pkg] = true, true
	}
	slicesOf := map[string][]string{}
	for _, lane := range shardOutput(t, root, "--split-lanes") {
		if !strings.Contains(string(workflow), `"`+lane+`"`) {
			t.Errorf("split lane %s is not in ci-go.yml's matrix", lane)
		}
		job := shardOutput(t, root, "--split", lane)
		if len(job) != 3 {
			t.Fatalf("split lane %s = %q, want mode, package and slice", lane, job)
		}
		mode, pkg, slice := job[0], job[1], job[2]
		if mode == "plain" {
			scheduled["plain"][pkg] = true
			continue
		}
		slicesOf[pkg] = append(slicesOf[pkg], slice)
	}
	for pkg, got := range slicesOf {
		slices.Sort(got)
		_, count, _ := strings.Cut(got[0], "/")
		complete := len(got) > 0
		for index, slice := range got {
			complete = complete && slice == string(rune('1'+index))+"/"+count
		}
		if complete && count == string(rune('0'+len(got))) {
			scheduled["race"][pkg] = true
		} else {
			t.Errorf("%s race slices = %v, want every k/%s once", pkg, got, count)
		}
	}
	for phase, flag := range map[string]string{"race": "--race", "plain": "--no-race"} {
		for _, pkg := range policy(flag) {
			if !scheduled[phase][pkg] {
				t.Errorf("%s is not scheduled in its %s phase", pkg, phase)
			}
		}
	}
}

// A split package's rows are keyed by the lane that timed them, its samples from other lanes are
// dropped, and GO_RACE_WEIGHTS_ONLY leaves every other package's row as it was.
func TestGoRaceWeightsRefreshKeysSplitSamplesByLane(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", "..", ".."))
	dir := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	job := func(id, lane string) string { return id + "\tsuccess\tGo — race-policy tests (" + lane + ")\n" }
	ok := func(pkg, seconds string) string {
		return "j\tstep\t2026-01-01T00:00:00Z ok  \tgithub.com/loomarr/loomarr/" + pkg + "\t" + seconds + "s\n"
	}
	write("1.jobs", job("11", "1/2")+job("12", "big-race-1/2")+job("13", "big-race-2/2")+job("14", "big-plain"))
	write("11.log", ok("internal/big", "900")+ok("internal/other", "50"))
	write("12.log", ok("internal/big", "300"))
	write("13.log", ok("internal/big", "280"))
	write("14.log", ok("internal/big", "90"))
	gh := write("gh", "#!/usr/bin/env bash\nif [[ $4 == --json ]]; then cat '"+dir+"'/\"$3\".jobs; else cat '"+dir+"'/\"$5\".log; fi\n")
	manifest := write("split.tsv", "big internal/big 2\n")
	weights := write("weights.tsv", "# kept header\ninternal/big\t339\ninternal/other\t7\n")
	refresh := func(only string) string {
		t.Helper()
		cmd := exec.Command("bash", filepath.Join("scripts", "go-race-weights-refresh.sh"), "1")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GO_RACE_WEIGHTS_GH="+gh, "GO_SHARD_SPLIT="+manifest,
			"GO_SHARD_WEIGHTS="+weights, "GO_RACE_WEIGHTS_ONLY="+only)
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("refresh: %v", err)
		}
		var rows []string
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if !strings.HasPrefix(line, "#") {
				rows = append(rows, line)
			}
		}
		return strings.Join(rows, "\n")
	}
	const split = "internal/big@race-1/2\t300\ninternal/big@race-2/2\t280\ninternal/big@plain\t90"
	if got, want := refresh(""), split+"\ninternal/other\t50"; got != want {
		t.Errorf("full refresh = %q, want %q", got, want)
	}
	if got, want := refresh("internal/big"), "internal/other\t7\n"+split; got != want {
		t.Errorf("one-package refresh = %q, want %q", got, want)
	}
}

// The race slices of a split package are disjoint and together run every top-level test that `go
// test -list` names, and a slice that would hold none fails instead of passing over nothing.
func TestGoTestPackagesSlicesEveryListedTestExactlyOnce(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", "..", ".."))
	run := func(t *testing.T, part, listed string) (string, error) {
		t.Helper()
		bin := t.TempDir()
		fakeGo := filepath.Join(bin, "go")
		runLog := filepath.Join(bin, "run.log")
		script := "#!/usr/bin/env bash\nfor arg in \"$@\"; do if [[ $arg == -list ]]; then printf '" + listed +
			"'; exit; fi; done\nprintf '%s\\n' \"$@\" > '" + runLog + "'\n"
		if err := os.WriteFile(fakeGo, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", filepath.Join("scripts", "go-test-packages.sh"), "race", "25m", "example.invalid/p")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GO_BIN="+fakeGo, "GO_TEST_PART="+part, "GITHUB_STEP_SUMMARY=", "GO_TEST_COMPILE_ONLY=")
		if output, err := cmd.CombinedOutput(); err != nil {
			return string(output), err
		}
		args, err := os.ReadFile(runLog)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(args), "\n")
		at := slices.Index(lines, "-run")
		if at < 0 {
			t.Fatalf("slice %s ran without -run: %q", part, lines)
		}
		return lines[at+1], nil
	}

	const listed = `TestB\nFuzzC\nExampleD\nTestA\nTestAB\nBenchmarkE\nok  \texample.invalid/p\t0.1s\n`
	ran := map[string]int{}
	for _, part := range []string{"1/2", "2/2"} {
		pattern, err := run(t, part, listed)
		if err != nil {
			t.Fatalf("slice %s: %v\n%s", part, err, pattern)
		}
		selector := regexp.MustCompile(pattern)
		count := 0
		for _, name := range []string{"TestA", "TestAB", "TestB", "FuzzC", "ExampleD"} {
			if selector.MatchString(name) {
				ran[name]++
				count++
			}
		}
		if count == 0 {
			t.Fatalf("slice %s selects no listed test: %s", part, pattern)
		}
	}
	for _, name := range []string{"TestA", "TestAB", "TestB", "FuzzC", "ExampleD"} {
		if ran[name] != 1 {
			t.Errorf("%s ran in %d slices, want exactly one", name, ran[name])
		}
	}
	if output, err := run(t, "2/2", `TestOnly\n`); err == nil || !strings.Contains(output, "holds no tests") {
		t.Fatalf("an empty slice = %v %q, want a failure naming it", err, output)
	}
}
