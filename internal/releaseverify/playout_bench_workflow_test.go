package releaseverify

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The repository is public. A self-hosted runner on the maintainer's GPU machines must never be
// reachable from a pull request, so the hardware bench workflow may only be dispatched or scheduled,
// and only it may name a self-hosted playout runner.
func TestPlayoutBenchWorkflowNeverRunsOnPullRequest(t *testing.T) {
	t.Parallel()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(readRepositoryWorkflow(t, "playout-bench.yml")), &doc); err != nil {
		t.Fatal(err)
	}
	root := doc.Content[0]
	triggers, ok := mappingValue(root, "on")
	if !ok || triggers.Kind != yaml.MappingNode {
		t.Fatal("playout-bench.yml must declare its triggers")
	}
	for i := 0; i < len(triggers.Content); i += 2 {
		switch name := triggers.Content[i].Value; name {
		case "workflow_dispatch", "schedule":
		default:
			t.Errorf("playout-bench.yml trigger %q is not allowed: only workflow_dispatch and schedule", name)
		}
	}
	jobs, _ := mappingValue(root, "jobs")
	for i := 0; i < len(jobs.Content); i += 2 {
		name, job := jobs.Content[i].Value, jobs.Content[i+1]
		runsOn, _ := mappingValue(job, "runs-on")
		if runsOn == nil || !strings.HasPrefix(runsOn.Value, "loomarr-playout-") {
			continue
		}
		condition := scalarValue(job, "if")
		if !strings.Contains(condition, "github.event_name == 'workflow_dispatch'") {
			t.Errorf("self-hosted job %s must check for workflow_dispatch itself, has %q", name, condition)
		}
		if strings.Contains(condition, "schedule") {
			t.Errorf("self-hosted job %s must not run on a schedule: %q", name, condition)
		}
	}
}

// A skipped hardware leg must stay visible (#1566): the coverage job always runs, waits for every
// leg, judges each leg by the same gate as the leg's own `if:`, and every leg records whether it wrote
// a report even when its bench failed. Each mutation hides a skip or a missing report again.
func TestPlayoutBenchCoverageCannotGoGreenByOmission(t *testing.T) {
	t.Parallel()
	baseline := readRepositoryWorkflow(t, "playout-bench.yml")
	mutations := map[string]struct{ old, new string }{
		"coverage only runs when every leg passed": {
			"    if: always()\n    needs: [macos, gpu-t4, arc, geforce]\n",
			"    needs: [macos, gpu-t4, arc, geforce]\n",
		},
		"coverage ignores a leg": {
			"    needs: [macos, gpu-t4, arc, geforce]\n",
			"    needs: [macos, gpu-t4, arc]\n",
		},
		"coverage gate drifts from the T4 leg's gate": {
			"PLAYOUT_BENCH_EXPECT_T4: ${{ vars.PLAYOUT_BENCH_T4_RUNNER != ''",
			"PLAYOUT_BENCH_EXPECT_T4: ${{ vars.PLAYOUT_BENCH_T4_RUNNER == ''",
		},
		"T4 leg gate drifts from the coverage gate": {
			"    if: vars.PLAYOUT_BENCH_T4_RUNNER != '' && (",
			"    if: vars.PLAYOUT_BENCH_T4_RUNNER != 'off' && (",
		},
		"coverage judges a leg as configured unconditionally": {
			"PLAYOUT_BENCH_EXPECT_ARC: ${{ github.event_name == 'workflow_dispatch' && inputs.run_arc }}",
			"PLAYOUT_BENCH_EXPECT_ARC: ${{ false }}",
		},
		"a failed bench skips recording its report": {
			"        if: always()\n        run: ./scripts/playout-bench-coverage.sh record",
			"        run: ./scripts/playout-bench-coverage.sh record",
		},
		"coverage step no longer fails the run": {
			"        run: ./scripts/playout-bench-coverage.sh summarise\n",
			"        run: ./scripts/playout-bench-coverage.sh summarise || true\n",
		},
	}
	root := writeCIContainerDownloadsFixture(t)
	workflowPath := filepath.Join(root, ".github", "workflows", "playout-bench.yml")
	writeFixtureFile(t, workflowPath, baseline)
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("baseline hardware bench workflow must pass: %v", err)
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(baseline, mutation.old) {
				t.Fatalf("mutation anchor %q is missing from playout-bench.yml", mutation.old)
			}
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "playout-bench.yml"), strings.Replace(baseline, mutation.old, mutation.new, 1))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a hardware bench that can go green by omission")
			}
		})
	}
}

func TestPlayoutBenchCoverageScript(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("bash", filepath.Join("scripts", "playout-bench-coverage-test.sh"))
	cmd.Dir = filepath.Clean(filepath.Join("..", ".."))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("playout bench coverage contract: %v\n%s", err, output)
	}
}

func TestOnlyPlayoutBenchWorkflowUsesSelfHostedPlayoutRunners(t *testing.T) {
	t.Parallel()
	_, source, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(source), "..", "..", ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "playout-bench.yml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "loomarr-playout-") {
			t.Errorf("%s names a self-hosted playout runner; only playout-bench.yml may", entry.Name())
		}
	}
}
