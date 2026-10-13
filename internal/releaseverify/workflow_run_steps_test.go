package releaseverify

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit"
)

func runGoModuleDownloadHelper(t *testing.T, config testkit.FakeGoConfig) testkit.FakeGoResult {
	t.Helper()
	return testkit.RunWithFakeGo(t, filepath.Join(repositoryRoot(t), "scripts", "go-mod-download.sh"), nil, config)
}

func TestGoModuleDownloadHelperDownloadsOnce(t *testing.T) {
	result := runGoModuleDownloadHelper(t, testkit.FakeGoConfig{DownloadStatuses: []int{0}})
	if result.Err != nil {
		t.Fatalf("clean download: %v", result.Err)
	}
	if want := [][]string{{"mod", "download"}}; !slices.EqualFunc(result.GoCommands, want, slices.Equal) {
		t.Fatalf("go commands = %v, want %v", result.GoCommands, want)
	}
	if len(result.SleepCalls) != 0 {
		t.Fatalf("clean download slept: %v", result.SleepCalls)
	}
}

func TestGoModuleDownloadHelperRetriesProxyStreamReset(t *testing.T) {
	result := runGoModuleDownloadHelper(t, testkit.FakeGoConfig{DownloadStatuses: []int{1, 1, 1, 1, 0}})
	if result.Err != nil {
		t.Fatalf("transient download: %v", result.Err)
	}
	if result.DownloadAttempts != 5 {
		t.Fatalf("transient download made %d attempts, want 5", result.DownloadAttempts)
	}
	if want := []int{2, 2, 2, 2}; !slices.Equal(result.SleepCalls, want) {
		t.Fatalf("transient download sleeps = %v, want %v", result.SleepCalls, want)
	}
}

func TestGoModuleDownloadHelperStopsAfterFiveFailures(t *testing.T) {
	result := runGoModuleDownloadHelper(t, testkit.FakeGoConfig{DownloadStatuses: []int{11, 12, 13, 14, 15}})
	var exitError *exec.ExitError
	if !errors.As(result.Err, &exitError) || exitError.ExitCode() != 15 {
		t.Fatalf("permanent failure = %v, want the fifth go status 15", result.Err)
	}
	if result.DownloadAttempts != 5 {
		t.Fatalf("permanent failure made %d attempts, want 5", result.DownloadAttempts)
	}
}

func TestGoModuleDownloadHelperFailsClosedWhenSleepFails(t *testing.T) {
	result := runGoModuleDownloadHelper(t, testkit.FakeGoConfig{DownloadStatuses: []int{1, 0}, SleepStatuses: []int{41}})
	var exitError *exec.ExitError
	if !errors.As(result.Err, &exitError) || exitError.ExitCode() != 41 {
		t.Fatalf("sleep failure = %v, want status 41", result.Err)
	}
	if result.DownloadAttempts != 1 {
		t.Fatalf("sleep failure made %d downloads, want 1", result.DownloadAttempts)
	}
}

func TestRepositoryWorkflowDownloadsAndPnpmCallsFollowPolicy(t *testing.T) {
	root := repositoryRoot(t)
	if err := VerifyGoModuleDownloads(root); err != nil {
		t.Fatal(err)
	}
	if err := VerifyWorkflowPnpmPackageManager(root); err != nil {
		t.Fatal(err)
	}
}

// policyFixture is a minimal repository: the helper, a pinned web workspace, and one workflow.
func policyFixture(t *testing.T, workflow string) string {
	t.Helper()
	root := t.TempDir()
	writeFixtureExecutable(t, filepath.Join(root, "scripts", "go-mod-download.sh"), goModuleDownloadHelperContract)
	writeFixtureFile(t, filepath.Join(root, "web", "package.json"), `{"packageManager": "pnpm@11.13.1"}`)
	writeFixtureFile(t, filepath.Join(root, "web", "packages", "planner", "package.json"), `{"name": "planner"}`)
	writeFixtureFile(t, filepath.Join(root, "tools", "package.json"), `{"packageManager": "yarn@4.0.0"}`)
	writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-example.yml"), workflow)
	return root
}

func runStepWorkflow(run string, extra string) string {
	return "on: workflow_call\njobs:\n  run:\n    runs-on: ubuntu-latest\n" + extra +
		"    steps:\n      - run: |\n" + indent(run, "          ") + "\n"
}

func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func TestVerifyGoModuleDownloadsRejectsDirectDownloads(t *testing.T) {
	for name, run := range map[string]string{
		"direct":       "go mod download",
		"with flags":   "go mod download -x",
		"in a chain":   "set -eu\ncd web && go  mod download",
		"continuation": "go mod \\\ndownload",
	} {
		t.Run(name, func(t *testing.T) {
			err := VerifyGoModuleDownloads(policyFixture(t, runStepWorkflow(run, "")))
			if err == nil || !strings.Contains(err.Error(), "runs go mod download directly") {
				t.Fatalf("%q: got %v, want a direct-download rejection", run, err)
			}
		})
	}
	for name, run := range map[string]string{
		"helper":  "./scripts/go-mod-download.sh",
		"comment": "# a bare go mod download fails the job on one proxy reset\n./scripts/go-mod-download.sh",
	} {
		t.Run(name, func(t *testing.T) {
			if err := VerifyGoModuleDownloads(policyFixture(t, runStepWorkflow(run, ""))); err != nil {
				t.Fatalf("rejected %q: %v", run, err)
			}
		})
	}
}

func TestVerifyGoModuleDownloadsRequiresTheReviewedHelper(t *testing.T) {
	root := policyFixture(t, runStepWorkflow("./scripts/go-mod-download.sh", ""))
	helper := filepath.Join(root, "scripts", "go-mod-download.sh")
	if err := os.WriteFile(helper, []byte(strings.Replace(goModuleDownloadHelperContract, "1 2 3 4 5", "1 2 3", 1)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyGoModuleDownloads(root); err == nil {
		t.Fatal("accepted a helper with a different retry bound")
	}
	if err := os.WriteFile(helper, []byte(goModuleDownloadHelperContract), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(helper, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyGoModuleDownloads(root); err == nil {
		t.Fatal("accepted a non-executable helper")
	}
}

func TestVerifyWorkflowPnpmPackageManager(t *testing.T) {
	const webDefaults = "    defaults:\n      run:\n        working-directory: web/packages/planner\n"
	for _, tc := range []struct {
		name  string
		run   string
		extra string
		ok    bool
	}{
		{name: "root -C download (run 38415258740)", run: "pnpm -C web/packages/planner exec vitest run"},
		{name: "root --dir", run: "pnpm --dir web install"},
		{name: "after an unrelated cd", run: "cd scripts && pnpm install"},
		{name: "unresolvable cd", run: `cd "$WORKSPACE" && pnpm install`},
		{name: "yarn-pinned workspace", run: "cd tools && pnpm install"},
		{name: "cd out of the workspace", run: "cd web\npnpm install\ncd ..\npnpm build"},
		{name: "enter web first", run: "cd web && pnpm -C packages/planner exec vitest run", ok: true},
		{name: "nested package inherits the pin", run: "cd web/packages/planner && pnpm exec vitest", ok: true},
		{name: "subshell", run: "(cd web && pnpm install)", ok: true},
		{name: "job working directory", run: "pnpm exec vitest", extra: webDefaults, ok: true},
		{name: "comment", run: "# pnpm -C web would download the newest pnpm\ncorepack enable", ok: true},
		{name: "no pnpm", run: "make fe-install", ok: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyWorkflowPnpmPackageManager(policyFixture(t, runStepWorkflow(tc.run, tc.extra)))
			if tc.ok && err != nil {
				t.Fatalf("rejected %q: %v", tc.run, err)
			}
			if !tc.ok && (err == nil || !strings.Contains(err.Error(), "runs pnpm")) {
				t.Fatalf("%q: got %v, want a pnpm policy rejection", tc.run, err)
			}
		})
	}
}

func TestVerifyWorkflowPnpmPackageManagerHonoursStepWorkingDirectory(t *testing.T) {
	workflow := "on: workflow_call\njobs:\n  run:\n    runs-on: ubuntu-latest\n    steps:\n" +
		"      - working-directory: web\n        run: pnpm install --frozen-lockfile\n"
	if err := VerifyWorkflowPnpmPackageManager(policyFixture(t, workflow)); err != nil {
		t.Fatal(err)
	}
}

// The merge-queue-only regression from #1893, replayed against the real Apple mobile workflow.
func TestVerifyWorkflowPnpmPackageManagerCatchesTheAppleMobileRootCall(t *testing.T) {
	source := repositoryRoot(t)
	root := t.TempDir()
	for _, manifest := range []string{"web/package.json", "docs-site/package.json"} {
		data, err := os.ReadFile(filepath.Join(source, manifest))
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, filepath.Join(root, manifest), string(data))
	}
	appleMobile := filepath.Join(root, ".github", "workflows", "ci-apple-mobile.yml")
	workflow, err := os.ReadFile(filepath.Join(source, ".github", "workflows", "ci-apple-mobile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	const fixed = "run: cd web && pnpm -C packages/lan-discovery-native exec"
	const broken = "run: pnpm -C web/packages/lan-discovery-native exec"
	if !strings.Contains(string(workflow), fixed) {
		t.Fatalf("ci-apple-mobile.yml no longer contains %q; update this regression", fixed)
	}
	writeFixtureFile(t, appleMobile, string(workflow))
	if err := VerifyWorkflowPnpmPackageManager(root); err != nil {
		t.Fatalf("fixed workflow rejected: %v", err)
	}
	writeFixtureFile(t, appleMobile, strings.Replace(string(workflow), fixed, broken, 1))
	if err := VerifyWorkflowPnpmPackageManager(root); err == nil || !strings.Contains(err.Error(), "ci-apple-mobile.yml") {
		t.Fatalf("got %v, want the root-level pnpm call that failed merge group run 38415258740 rejected", err)
	}
}
