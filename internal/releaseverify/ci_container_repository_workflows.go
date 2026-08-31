package releaseverify

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	postgresWorkflowName         = "ci-postgres.yml"
	playwrightWorkflowName       = "ci-playwright.yml"
	playwrightVisualWorkflowStep = `LOOMARR_PLAYWRIGHT_SHARD=--shard=${{ matrix.shard }}/${{ strategy.job-total }} make fe-visual`
)

const packagedImageMetadataInspection = `set -euo pipefail
image=loomarr-ci:verify
test "$(docker image inspect "$image" --format '{{ index .Config.Labels "org.opencontainers.image.documentation" }}')" = \
  "https://github.com/loomarr/loomarr/blob/main/THIRD_PARTY_NOTICES.md"
test "$(docker image inspect "$image" --format '{{ index .Config.Labels "org.opencontainers.image.licenses" }}')" = \
  "MIT AND GPL-3.0-or-later"
container=$(docker create "$image")
trap 'docker rm -f "$container" >/dev/null' EXIT
docker cp "$container:/usr/share/doc/loomarr/LICENSE" "$RUNNER_TEMP/LICENSE"
docker cp "$container:/usr/share/doc/loomarr/THIRD_PARTY_NOTICES.md" "$RUNNER_TEMP/THIRD_PARTY_NOTICES.md"
cmp LICENSE "$RUNNER_TEMP/LICENSE"
cmp THIRD_PARTY_NOTICES.md "$RUNNER_TEMP/THIRD_PARTY_NOTICES.md"
`

var (
	shellVariableToken = regexp.MustCompile(`^\$\$?(?:[A-Za-z_][A-Za-z0-9_]*|\{[A-Za-z_][A-Za-z0-9_]*\})$`)
)

// verifyRepositoryWorkflowContainerAcquisition parses every workflow and
// reserves container acquisition for the exact audited Make route. The one
// ci-image exception operates only on the image built by its preceding action
// and is source-exact so it cannot become a general pull path.
func verifyRepositoryWorkflowContainerAcquisition(workflowsDir string, protectedTargets map[string]struct{}, scripts *repositoryScriptAudit) error {
	catalog := workflowAuthorityCatalog()
	if err := verifyWorkflowAuthorityCatalog(catalog); err != nil {
		return err
	}
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		return fmt.Errorf("read CI workflows: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("CI workflow path must not be a symlink: %s", entry.Name())
		}
		if entry.IsDir() {
			continue
		}
		extension := filepath.Ext(entry.Name())
		if extension == ".yml" || extension == ".yaml" {
			paths = append(paths, filepath.Join(workflowsDir, entry.Name()))
		}
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return fmt.Errorf("no CI workflow YAML files found in %s", workflowsDir)
	}
	present := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		present[filepath.Base(path)] = struct{}{}
	}
	for workflowName := range registeredWorkflowAuthorities() {
		if _, ok := present[workflowName]; !ok {
			return fmt.Errorf("registered CI workflow authority is missing: %s", workflowName)
		}
	}
	if len(present) != len(catalog.topology) {
		return fmt.Errorf("CI workflow file topology differs from its registered authority")
	}
	for workflowName := range present {
		if _, registered := catalog.topology[workflowName]; !registered {
			return fmt.Errorf("unregistered CI workflow file: %s", workflowName)
		}
	}
	authorities := newWorkflowAuthorityLedger()
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read CI workflow %s: %w", path, readErr)
		}
		workflow, parseErr := parseYAML(data)
		if parseErr != nil {
			return fmt.Errorf("parse CI workflow %s: %w", path, parseErr)
		}
		workflowName := filepath.Base(path)
		if topologyErr := verifyWorkflowTopology(workflowName, workflow); topologyErr != nil {
			return fmt.Errorf("CI workflow %s: %w", path, topologyErr)
		}
		authorizedRuns := make(map[*yaml.Node]struct{})
		if scanErr := verifyWorkflowSourceBoundSteps(workflow, workflowName, protectedTargets, scripts, authorizedRuns, authorities); scanErr != nil {
			return fmt.Errorf("CI workflow %s: %w", path, scanErr)
		}
		if scanErr := verifyWorkflowContainerAcquisition(workflow, workflowName, scripts, authorizedRuns); scanErr != nil {
			return fmt.Errorf("CI workflow %s: %w", path, scanErr)
		}
	}
	return authorities.verifyComplete()
}

func registeredWorkflowAuthorities() map[string]struct{} {
	topology := workflowAuthorityCatalog().topology
	registered := make(map[string]struct{}, len(topology))
	for workflowName := range topology {
		registered[workflowName] = struct{}{}
	}
	return registered
}

func verifyWorkflowContainerAcquisition(
	node *yaml.Node,
	workflowName string,
	scripts *repositoryScriptAudit,
	authorizedRuns map[*yaml.Node]struct{},
) error {
	if node.Kind == yaml.MappingNode {
		for index := 0; index < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			switch key.Value {
			case "services", "container":
				return fmt.Errorf("workflow %s declarations bypass the bounded Make route", key.Value)
			case "uses":
				if value.Kind == yaml.ScalarNode && strings.HasPrefix(strings.TrimSpace(value.Value), "docker://") {
					return fmt.Errorf("docker protocol actions bypass the bounded Make route")
				}
			case "run":
				if value.Kind != yaml.ScalarNode {
					break // A job may itself be named run; executable step shapes are checked by their owner.
				}
				command := value.Value
				if _, authorized := authorizedRuns[value]; authorized {
					break
				}
				if workflowRouteCandidateWithScripts(command, nil, scripts) {
					return fmt.Errorf("workflow shell command bypasses the bounded Make route: %q", strings.TrimSpace(command))
				}
			}
		}
	}
	for _, child := range node.Content {
		if err := verifyWorkflowContainerAcquisition(child, workflowName, scripts, authorizedRuns); err != nil {
			return err
		}
	}
	return nil
}

func verifyWorkflowSourceBoundSteps(workflow *yaml.Node, workflowName string, protectedTargets map[string]struct{}, scripts *repositoryScriptAudit, authorizedRuns map[*yaml.Node]struct{}, authorities *workflowAuthorityLedger) error {
	authorities.expectWorkflow(workflowName)
	if err := verifySourceBoundWorkflowIdentity(workflowName, workflow); err != nil {
		return err
	}
	workflowEnvironment, err := scalarEnvironment(workflow)
	if err != nil {
		return err
	}
	jobs, ok := mappingValue(workflow, "jobs")
	if !ok || jobs.Kind != yaml.MappingNode {
		return nil
	}
	if err := verifyRegisteredWorkflowActionCardinality(workflowName, jobs); err != nil {
		return err
	}
	for index := 0; index < len(jobs.Content); index += 2 {
		jobName, job := jobs.Content[index].Value, jobs.Content[index+1]
		if job.Kind != yaml.MappingNode {
			continue
		}
		if uses, delegated := mappingValue(job, "uses"); delegated {
			familyWorkflows := workflowAuthorityCatalog().familyWorkflows
			if workflowName != "ci.yml" || uses.Kind != yaml.ScalarNode || familyWorkflows[jobName] == "" || uses.Value != "./"+familyWorkflows[jobName] {
				return fmt.Errorf("workflow %s job %s delegates to an unaudited reusable workflow", workflowName, jobName)
			}
			continue
		}
		jobEnvironment, environmentErr := mergedScalarEnvironment(workflowEnvironment, job)
		if environmentErr != nil {
			return environmentErr
		}
		steps, ok := mappingValue(job, "steps")
		if !ok || steps.Kind != yaml.SequenceNode {
			continue
		}
		if contextErr := verifySourceBoundWorkflowContext(workflowName, jobName, workflow, job); contextErr != nil {
			return contextErr
		}
		if actionErr := verifySourceBoundWorkflowActions(workflowName, jobName, steps); actionErr != nil {
			return actionErr
		}
		for stepIndex, step := range steps.Content {
			if step.Kind != yaml.MappingNode {
				continue
			}
			run, ok := mappingValue(step, "run")
			if !ok || run.Kind != yaml.ScalarNode {
				continue
			}
			if authorized, authorityErr := authorities.authorize(workflowName, jobName, step, run, stepIndex, protectedTargets, scripts); authorityErr != nil {
				return authorityErr
			} else if authorized {
				authorizedRuns[run] = struct{}{}
				continue
			}
			command := strings.TrimSpace(run.Value)
			stepEnvironment, environmentErr := mergedScalarEnvironment(jobEnvironment, step)
			if environmentErr != nil {
				return environmentErr
			}
			if workflowRouteCandidateWithScripts(run.Value, stepEnvironment, scripts) {
				return fmt.Errorf("workflow shell command is not an exact source-bound container route: %q", command)
			}
		}
	}
	return nil
}

func targetsRemainNonAcquiring(targets []string, protectedTargets map[string]struct{}) bool {
	for _, target := range targets {
		if _, protected := protectedTargets[target]; protected {
			return false
		}
	}
	return true
}

func scalarEnvironment(scope *yaml.Node) (map[string]string, error) {
	return scalarMapping(scope, "env", "workflow environment")
}

func scalarMapping(scope *yaml.Node, key, label string) (map[string]string, error) {
	result := make(map[string]string)
	mapping, ok := mappingValue(scope, key)
	if !ok {
		return result, nil
	}
	if mapping.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", label)
	}
	for index := 0; index < len(mapping.Content); index += 2 {
		name, value := mapping.Content[index], mapping.Content[index+1]
		if name.Kind != yaml.ScalarNode || value.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s entries must be scalar", label)
		}
		result[name.Value] = value.Value
	}
	return result, nil
}

func mergedScalarEnvironment(inherited map[string]string, scope *yaml.Node) (map[string]string, error) {
	local, err := scalarEnvironment(scope)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(inherited)+len(local))
	for key, value := range inherited {
		result[key] = value
	}
	for key, value := range local {
		result[key] = value
	}
	return result, nil
}

func workflowRouteCandidateWithScripts(command string, environment map[string]string, scripts *repositoryScriptAudit) bool {
	active := stripShellComments(normalizeShellContinuations(command))
	if scripts != nil && scripts.commandAcquires(active) {
		return true
	}
	if workflowHasDynamicExecutable(active) {
		return true
	}
	if containerCommandInvokesEngine(active) {
		return true
	}
	for _, token := range shellCommandTokens(active) {
		if workflowRouteToken(token) {
			return true
		}
		for _, word := range strings.Fields(token) {
			if workflowRouteToken(word) {
				return true
			}
		}
	}
	for name, value := range environment {
		if workflowRouteToken(value) && workflowReferencesEnvironment(active, name) {
			return true
		}
	}
	return false
}

// workflowHasDynamicExecutable does not resolve variables or predict shell
// control flow. It treats every lexical command-list segment independently and
// rejects a variable in executable position, including one reached through a
// transparent wrapper. Exact unrelated checked-in steps are authorized before
// this classifier and therefore do not need a dynamic-shell escape hatch.

func workflowHasDynamicExecutable(command string) bool {
	return shellCommandMatches(command, dynamicExecutableSegment)
}

func dynamicExecutableSegment(segment []string) (bool, []string) {
	index, _ := leadingShellAssignments(segment)
	if index == len(segment) {
		return false, nil
	}
	executable, resolved := resolveLiteralShellToken(segment[index], nil)
	if !resolved {
		return true, nil
	}
	var noExecution bool
	index, _, resolved, noExecution, _ = unwrapShellCommand(segment, index, executable, nil)
	if !resolved {
		return true, nil
	}
	if noExecution {
		return false, nil
	}
	return false, nestedInterpreterCommands(segment, index)
}

func workflowRouteToken(token string) bool {
	for _, word := range strings.Fields(token) {
		if word != token && workflowRouteToken(word) {
			return true
		}
	}
	if _, value, found := strings.Cut(token, "="); found {
		token = value
	}
	token = strings.Trim(token, `"'`)
	base := filepath.Base(filepath.ToSlash(token))
	switch base {
	case "make", "gmake", "docker", "podman", "nerdctl", "buildah", "docker-compose", "ensure-container-image.sh", "run-playwright-container.sh":
		return true
	default:
		return false
	}
}

func workflowReferencesEnvironment(command, name string) bool {
	return strings.Contains(command, "$"+name) ||
		strings.Contains(command, "${"+name+"}") ||
		strings.Contains(command, "env."+name)
}
