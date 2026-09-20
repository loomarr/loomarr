package releaseverify

import (
	"bufio"
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
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"$*\" == \"list -m\" ]]; then echo example.invalid; exit; fi\n[[ \"$*\" == \"list ./...\" ]]\nprintf '%s\\n' '"+strings.ReplaceAll(packages, "\n", "' '")+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	weights := filepath.Join(t.TempDir(), "weights.tsv")
	if err := os.WriteFile(weights, []byte("a 9\nb 8\nc 7\nd 6\ne 5\nf 4\ng 3\nh 2\ni 1\n"), 0o600); err != nil {
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
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GO_SHARD_WEIGHTS="+weights)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go shard %s: %v\n%s", shard, err, output)
			}
			if got := strings.TrimSpace(string(output)); got != expected {
				t.Fatalf("go shard %s =\n%s\nwant measured longest-processing-time assignment\n%s", shard, got, expected)
			}
		})
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
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go shard %d/%d: %v\n%s", shard, goRaceShardCount, err, output)
		}
		for _, pkg := range strings.Fields(string(output)) {
			relative := strings.TrimPrefix(pkg, "github.com/loomarr/loomarr/")
			seenPackages[relative] = true
			loads[shard-1] += max(weights[relative], 1)
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
	if maxLoad > 540 {
		t.Fatalf("modeled race shard exceeds nine test minutes: loads=%v", loads)
	}
	if maxLoad*100 > minLoad*125 {
		t.Fatalf("modeled race shards differ by more than 25%%: loads=%v", loads)
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
		cmd.Env = append(os.Environ(), "GO_BIN="+fakeGo, "GO_SHARD=2/6", "GITHUB_STEP_SUMMARY="+summary, "FAKE_GO_EXIT="+exitCode)
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
