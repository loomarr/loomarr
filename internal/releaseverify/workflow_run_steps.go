package releaseverify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// goModuleDownloadHelperContract is the one reviewed bounded-retry program for the module graph,
// in the same shape as the container-image helper. A proxy.golang.org HTTP/2 stream reset failed a
// whole required job on its single `go mod download` (run 38418263494); the module cache keeps every
// zip that already landed, so a retry fetches only what the reset interrupted.
const goModuleDownloadHelperContract = `#!/usr/bin/env bash
set -u

if [[ $# -ne 0 ]]; then
  echo "usage: $0" >&2
  exit 2
fi

download_status=1
for attempt in 1 2 3 4 5; do
  if go mod download; then
    exit 0
  else
    download_status=$?
  fi
  if [[ $attempt -lt 5 ]]; then
    sleep 2 || exit $?
  fi
done

exit "$download_status"
`

const goModuleDownloadStep = "./scripts/go-mod-download.sh"

var goModuleDownloadCommand = regexp.MustCompile(`\bgo\s+mod\s+download\b`)

// repositoryRunStep is one `run:` step with the directory GitHub Actions starts its shell in,
// relative to the repository root.
type repositoryRunStep struct {
	file             string
	job              string
	index            int
	run              string
	workingDirectory string
}

func (s repositoryRunStep) String() string {
	return fmt.Sprintf("%s job %s step %d", s.file, s.job, s.index)
}

// repositoryWorkflowRunSteps lists every run step in every workflow, resolving step, job, and
// workflow `defaults.run.working-directory` in GitHub's precedence order.
func repositoryWorkflowRunSteps(root string) ([]repositoryRunStep, error) {
	workflowsDir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		return nil, err
	}
	var steps []repositoryRunStep
	for _, entry := range entries {
		if entry.IsDir() || (filepath.Ext(entry.Name()) != ".yml" && filepath.Ext(entry.Name()) != ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(workflowsDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		workflow, err := parseYAML(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		workflowDirectory := defaultRunWorkingDirectory(workflow)
		jobs, ok := mappingValue(workflow, "jobs")
		if !ok {
			continue
		}
		for i := 0; i+1 < len(jobs.Content); i += 2 {
			jobName, job := jobs.Content[i].Value, jobs.Content[i+1]
			jobDirectory := defaultRunWorkingDirectory(job)
			if jobDirectory == "" {
				jobDirectory = workflowDirectory
			}
			jobSteps, ok := mappingValue(job, "steps")
			if !ok {
				continue
			}
			for index, step := range jobSteps.Content {
				run := scalarValue(step, "run")
				if run == "" {
					continue
				}
				directory := scalarValue(step, "working-directory")
				if directory == "" {
					directory = jobDirectory
				}
				steps = append(steps, repositoryRunStep{
					file: entry.Name(), job: jobName, index: index, run: run, workingDirectory: directory,
				})
			}
		}
	}
	return steps, nil
}

func defaultRunWorkingDirectory(scope *yaml.Node) string {
	defaults, ok := mappingValue(scope, "defaults")
	if !ok {
		return ""
	}
	run, ok := mappingValue(defaults, "run")
	if !ok {
		return ""
	}
	return scalarValue(run, "working-directory")
}

// VerifyGoModuleDownloads keeps every workflow's module-graph download behind the bounded helper.
func VerifyGoModuleDownloads(root string) error {
	helperPath := filepath.Join(root, "scripts", "go-mod-download.sh")
	helper, err := os.Stat(helperPath)
	if err != nil {
		return fmt.Errorf("inspect Go module download helper: %w", err)
	}
	if !helper.Mode().IsRegular() || helper.Mode().Perm()&0o111 == 0 {
		return errors.New("go module download helper must be an executable regular file")
	}
	source, err := os.ReadFile(helperPath)
	if err != nil {
		return err
	}
	if string(source) != goModuleDownloadHelperContract {
		return errors.New("go module download helper must match the reviewed bounded-retry program")
	}
	steps, err := repositoryWorkflowRunSteps(root)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if goModuleDownloadCommand.MatchString(stripShellComments(normalizeShellContinuations(step.run))) {
			return fmt.Errorf("%s runs go mod download directly; use %s", step, goModuleDownloadStep)
		}
	}
	return nil
}

// VerifyWorkflowPnpmPackageManager rejects a workflow step that runs pnpm where Corepack finds no
// pinned packageManager. Corepack resolves pnpm from the shell's directory BEFORE `pnpm -C` applies;
// the repository root has no package.json, so a root-level call silently downloads the newest pnpm,
// which refuses web's pin (ERR_PNPM_BAD_PM_VERSION). That broke a macOS-only step that pull
// requests never run, and cost a merge-queue round trip (run 38415258740). This check runs on every
// pull request, so the next such step fails on Linux before the queue.
func VerifyWorkflowPnpmPackageManager(root string) error {
	steps, err := repositoryWorkflowRunSteps(root)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if err := verifyPnpmRunStep(root, step); err != nil {
			return fmt.Errorf("%s: %w", step, err)
		}
	}
	return nil
}

func verifyPnpmRunStep(root string, step repositoryRunStep) error {
	directory := path.Clean(step.workingDirectory)
	known := !strings.Contains(directory, "$") && !path.IsAbs(directory)
	for _, fields := range shellCommandSegments(stripShellComments(normalizeShellContinuations(step.run))) {
		fields[0] = strings.TrimLeft(fields[0], "(")
		if fields[0] == "cd" {
			if len(fields) != 2 || strings.ContainsAny(fields[1], "$`~") || path.IsAbs(fields[1]) {
				known = false
				continue
			}
			directory = path.Clean(path.Join(directory, fields[1]))
			continue
		}
		if !invokesPnpm(fields) {
			continue
		}
		if !known {
			return errors.New("runs pnpm from a directory the verifier cannot resolve; cd into a literal pnpm workspace first")
		}
		pinned, err := pnpmPackageManagerPinned(root, directory)
		if err != nil {
			return err
		}
		if !pinned {
			return fmt.Errorf("runs pnpm in %q, where Corepack finds no pnpm packageManager pin (it would download the newest pnpm); cd into the workspace first, `pnpm -C` applies too late", directory)
		}
	}
	return nil
}

func invokesPnpm(fields []string) bool {
	for _, field := range fields {
		if field == "pnpm" {
			return true
		}
	}
	return false
}

// pnpmPackageManagerPinned walks from directory toward the repository root the way Corepack does:
// the first package.json that declares packageManager decides.
func pnpmPackageManagerPinned(root, directory string) (bool, error) {
	if directory == ".." || strings.HasPrefix(directory, "../") {
		return false, fmt.Errorf("working directory %q leaves the repository", directory)
	}
	for {
		manifestPath := filepath.Join(root, filepath.FromSlash(directory), "package.json")
		data, err := os.ReadFile(manifestPath)
		switch {
		case err == nil:
			var manifest struct {
				PackageManager string `json:"packageManager"`
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				return false, fmt.Errorf("%s: %w", manifestPath, err)
			}
			if manifest.PackageManager != "" {
				return strings.HasPrefix(manifest.PackageManager, "pnpm@"), nil
			}
		case !errors.Is(err, os.ErrNotExist):
			return false, err
		}
		if directory == "." {
			return false, nil
		}
		directory = path.Dir(directory)
	}
}
