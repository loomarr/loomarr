package releaseverify

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var imageFilterCommand = regexp.MustCompile(`(?m)^[ \t]*if echo "\$changed" \| grep -qE '([^'\r\n]+)'; then\r?\n[ \t]+echo "image=true"`)

var requiredImageInputs = []string{
	"Dockerfile",
	".dockerignore",
	"LICENSE",
	"THIRD_PARTY_NOTICES.md",
	"Cargo.toml",
	"Cargo.lock",
	"rust-toolchain.toml",
	"rust/loomarr-image/src/main.rs",
	"internal/store/migrations/sqlite/99999_release_probe.sql",
	"internal/app/app.go",
	"go.mod",
	"go.sum",
	"docs/help/release-probe.md",
	"web/apps/web/src/main.tsx",
	"api/openapi.yaml",
	"scripts/check-fe-bundle.mjs",
}

// VerifyCIImageInputs executes the active image-filter regex against one path
// from every source family copied into the release image.
func VerifyCIImageInputs(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parseYAML(data)
	if err != nil {
		return err
	}
	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		return err
	}
	changes, err := requiredMap(jobs, "changes")
	if err != nil {
		return err
	}
	steps, err := requiredSequence(changes, "steps")
	if err != nil {
		return err
	}
	var filterRun string
	for _, step := range steps.Content {
		if step.Kind == yaml.MappingNode && scalarValue(step, "id") == "filter" {
			if filterRun != "" {
				return errors.New("CI changes job contains duplicate filter steps")
			}
			filterRun = scalarValue(step, "run")
		}
	}
	if filterRun == "" {
		return errors.New("CI changes job has no active filter step")
	}
	matches := imageFilterCommand.FindAllStringSubmatch(filterRun, -1)
	if len(matches) != 1 {
		return fmt.Errorf("CI filter step must contain exactly one active image filter, found %d", len(matches))
	}
	filter, err := regexp.Compile(matches[0][1])
	if err != nil {
		return fmt.Errorf("compile CI image filter: %w", err)
	}
	for _, input := range requiredImageInputs {
		if !filter.MatchString(input) {
			return fmt.Errorf("CI image filter misses Docker build input %s", input)
		}
	}
	return nil
}

// VerifyCIAggregate ensures the one branch-protected CI result cannot omit a
// top-level job. A job that is allowed to skip still reports "skipped" through
// needs; leaving it out entirely makes its failure invisible to the aggregate.
func VerifyCIAggregate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parseYAML(data)
	if err != nil {
		return err
	}
	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		return err
	}
	ciOK, err := requiredMap(jobs, "ci-ok")
	if err != nil {
		return errors.New("CI workflow must define the ci-ok aggregate job")
	}
	needs, err := requiredSequence(ciOK, "needs")
	if err != nil {
		return errors.New("CI ci-ok job must declare needs as a sequence")
	}

	included := make(map[string]bool, len(needs.Content))
	for _, need := range needs.Content {
		if need.Kind != yaml.ScalarNode || need.Value == "" {
			return errors.New("CI ci-ok needs must contain only job identifiers")
		}
		if included[need.Value] {
			return fmt.Errorf("CI ci-ok needs contains duplicate job %s", need.Value)
		}
		included[need.Value] = true
	}

	knownJobs := make(map[string]bool, len(jobs.Content)/2)
	for i := 0; i < len(jobs.Content); i += 2 {
		job := jobs.Content[i].Value
		knownJobs[job] = true
		if job != "ci-ok" && !included[job] {
			return fmt.Errorf("CI ci-ok aggregate omits job %s", job)
		}
	}
	for need := range included {
		if !knownJobs[need] {
			return fmt.Errorf("CI ci-ok aggregate references unknown job %s", need)
		}
	}
	return nil
}

// VerifyCIImpactActivation pins each specialized decision that has graduated
// from shadow mode. An activated job must consume its dedicated classifier
// output, preserve the explicit manual release scope, and stop consulting the
// legacy broad family that the specialized decision replaces.
func VerifyCIImpactActivation(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parseYAML(data)
	if err != nil {
		return err
	}
	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		return err
	}
	changes, err := requiredMap(jobs, "changes")
	if err != nil {
		return errors.New("CI workflow must define the changes job")
	}
	outputs, err := requiredMap(changes, "outputs")
	if err != nil {
		return errors.New("CI changes job must publish classifier outputs")
	}

	type classifierOutput struct {
		name   string
		source string
	}
	activated := map[string]struct {
		outputs   []classifierOutput
		condition string
	}{
		"ci-policy": {
			outputs:   []classifierOutput{{name: "impact_policy", source: "policy"}},
			condition: "needs.changes.outputs.impact_policy == 'true'",
		},
		"rust-contracts": {
			outputs:   []classifierOutput{{name: "impact_rust", source: "rust"}},
			condition: "needs.changes.outputs.impact_rust == 'true' || needs.changes.outputs.release_candidate == 'true'",
		},
		"go-contracts": {
			outputs:   []classifierOutput{{name: "impact_contracts", source: "contracts"}},
			condition: "needs.changes.outputs.impact_contracts == 'true' || needs.changes.outputs.release_candidate == 'true'",
		},
		"image-certification": {
			outputs:   []classifierOutput{{name: "impact_rust", source: "rust"}},
			condition: "needs.changes.outputs.impact_rust == 'true' || needs.changes.outputs.release_candidate == 'true'",
		},
		"go": {
			outputs:   []classifierOutput{{name: "impact_go", source: "go"}},
			condition: "needs.changes.outputs.impact_go == 'true'",
		},
		"store-postgres": {
			outputs:   []classifierOutput{{name: "impact_postgres", source: "postgres"}},
			condition: "needs.changes.outputs.impact_postgres == 'true'",
		},
		"playwright": {
			outputs: []classifierOutput{
				{name: "impact_visual", source: "visual"},
				{name: "impact_e2e", source: "e2e"},
			},
			condition: "needs.changes.outputs.impact_visual == 'true' || needs.changes.outputs.impact_e2e == 'true'",
		},
		"frontend": {
			outputs:   []classifierOutput{{name: "impact_web", source: "web"}},
			condition: "needs.changes.outputs.impact_web == 'true'",
		},
		"clients": {
			outputs:   []classifierOutput{{name: "impact_clients", source: "clients"}},
			condition: "needs.changes.outputs.impact_clients == 'true'",
		},
		"image": {
			outputs:   []classifierOutput{{name: "impact_image", source: "image"}},
			condition: "needs.changes.outputs.impact_image == 'true'",
		},
		"docs": {
			outputs:   []classifierOutput{{name: "impact_docs", source: "docs"}},
			condition: "needs.changes.outputs.impact_docs == 'true'",
		},
		"android": {
			outputs:   []classifierOutput{{name: "impact_android", source: "android"}},
			condition: "needs.changes.outputs.impact_android == 'true'",
		},
		"tuner": {
			outputs:   []classifierOutput{{name: "impact_tuner", source: "tuner"}},
			condition: "github.event_name != 'pull_request' && needs.changes.outputs.impact_tuner == 'true'",
		},
		"apple-mobile": {
			outputs:   []classifierOutput{{name: "impact_apple_mobile", source: "apple_mobile"}},
			condition: "github.event_name != 'pull_request' && needs.changes.outputs.impact_apple_mobile == 'true'",
		},
		"apple-tv": {
			outputs:   []classifierOutput{{name: "impact_apple_tv", source: "apple_tv"}},
			condition: "github.event_name != 'pull_request' && needs.changes.outputs.impact_apple_tv == 'true'",
		},
	}
	for jobName, gate := range activated {
		for _, expected := range gate.outputs {
			output, ok := mappingValue(outputs, expected.name)
			if !ok || output.Kind != yaml.ScalarNode ||
				output.Value != "${{ steps.impact.outputs."+expected.source+" }}" {
				return fmt.Errorf("CI changes job does not publish %s from the specialized classifier", expected.name)
			}
		}
		job, err := requiredMap(jobs, jobName)
		if err != nil {
			return fmt.Errorf("CI workflow must define activated job %s", jobName)
		}
		needs, ok := mappingValue(job, "needs")
		if !ok || !yamlNodeContainsScalar(needs, "changes") {
			return fmt.Errorf("CI job %s must depend on changes", jobName)
		}
		condition := scalarValue(job, "if")
		if condition != gate.condition {
			return fmt.Errorf("CI job %s condition %q does not match activated contract %q", jobName, condition, gate.condition)
		}
	}
	appleCommands := map[string]string{
		"apple-mobile": "make client-apple-simulator CLIENT_APP=mobile",
		"apple-tv":     "make client-apple-simulator CLIENT_APP=tv",
	}
	for jobName, command := range appleCommands {
		job, err := requiredMap(jobs, jobName)
		if err != nil {
			return fmt.Errorf("CI workflow must define split Apple job %s", jobName)
		}
		if _, ok := mappingValue(job, "strategy"); ok {
			return fmt.Errorf("CI Apple job %s must be a single app-specific job", jobName)
		}
		if !yamlNodeContainsScalar(job, command) {
			return fmt.Errorf("CI Apple job %s must run %q", jobName, command)
		}
	}
	return nil
}

func yamlNodeContainsScalar(node *yaml.Node, value string) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.ScalarNode {
		return node.Value == value
	}
	for _, child := range node.Content {
		if yamlNodeContainsScalar(child, value) {
			return true
		}
	}
	return false
}

// VerifyCINativeAdmission keeps scarce macOS capacity behind the merge queue.
// Pull-request CI remains fast feedback; merge-group, main, and manual runs
// retain the same required native evidence before delivery or release.
func VerifyCINativeAdmission(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parseYAML(data)
	if err != nil {
		return err
	}
	on, err := requiredMap(root, "on")
	if err != nil {
		return errors.New("CI workflow must define triggers")
	}
	if _, ok := mappingValue(on, "merge_group"); !ok {
		return errors.New("CI workflow must trigger for merge groups")
	}
	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		return err
	}
	expected := map[string]string{
		"agent-harness-macos": "github.event_name != 'pull_request' && needs.changes.outputs.impact_agent == 'true'",
		"apple-mobile":        "github.event_name != 'pull_request' && needs.changes.outputs.impact_apple_mobile == 'true'",
		"apple-tv":            "github.event_name != 'pull_request' && needs.changes.outputs.impact_apple_tv == 'true'",
		"tuner":               "github.event_name != 'pull_request' && needs.changes.outputs.impact_tuner == 'true'",
	}
	for jobName, condition := range expected {
		job, err := requiredMap(jobs, jobName)
		if err != nil {
			return fmt.Errorf("CI workflow must define scarce-capacity job %s", jobName)
		}
		if got := scalarValue(job, "if"); got != condition {
			return fmt.Errorf("CI job %s condition %q does not match native admission contract %q", jobName, got, condition)
		}
	}
	return nil
}

// VerifyCIManualScopes pins the manual-dispatch contract used to certify an
// exact main commit before release. The candidate scope is intentionally
// proportional; the full scope remains available for deliberate deep checks.
func VerifyCIManualScopes(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parseYAML(data)
	if err != nil {
		return err
	}
	on, err := requiredMap(root, "on")
	if err != nil {
		return errors.New("CI workflow must define triggers")
	}
	dispatch, err := requiredMap(on, "workflow_dispatch")
	if err != nil {
		return errors.New("CI workflow must define workflow_dispatch")
	}
	inputs, err := requiredMap(dispatch, "inputs")
	if err != nil {
		return errors.New("CI workflow_dispatch must define inputs")
	}
	scope, err := requiredMap(inputs, "scope")
	if err != nil {
		return errors.New("CI workflow_dispatch must define the scope input")
	}
	if scalarValue(scope, "type") != "choice" || scalarValue(scope, "default") != "release-candidate" {
		return errors.New("CI scope input must be a choice defaulting to release-candidate")
	}
	options, err := requiredSequence(scope, "options")
	if err != nil || len(options.Content) != 2 || options.Content[0].Value != "release-candidate" || options.Content[1].Value != "full" {
		return errors.New("CI scope choices must be release-candidate then full")
	}

	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		return err
	}
	changes, err := requiredMap(jobs, "changes")
	if err != nil {
		return errors.New("CI workflow must define the changes job")
	}
	outputs, err := requiredMap(changes, "outputs")
	if err != nil || scalarValue(outputs, "release_candidate") != "${{ steps.filter.outputs.release_candidate }}" {
		return errors.New("CI changes job must expose the release_candidate filter output")
	}
	steps, err := requiredSequence(changes, "steps")
	if err != nil {
		return errors.New("CI changes job must define steps")
	}
	var filterRun string
	for _, step := range steps.Content {
		if step.Kind == yaml.MappingNode && scalarValue(step, "id") == "filter" {
			filterRun = scalarValue(step, "run")
		}
	}
	if !strings.Contains(filterRun, `./scripts/ci-dispatch-scope.sh "$DISPATCH_SCOPE"`) {
		return errors.New("CI filter step must delegate manual scope selection to ci-dispatch-scope.sh")
	}

	markers := map[string]string{
		"release-candidate-scope": "Release candidate — exact main scope",
		"full-manual-scope":       "Manual CI — full scope",
	}
	for jobID, name := range markers {
		job, jobErr := requiredMap(jobs, jobID)
		if jobErr != nil || scalarValue(job, "name") != name {
			return fmt.Errorf("CI workflow must define the %s marker job", jobID)
		}
	}
	for _, jobID := range []string{"go-contracts", "image-certification"} {
		job, jobErr := requiredMap(jobs, jobID)
		if jobErr != nil || !strings.Contains(scalarValue(job, "if"), "needs.changes.outputs.release_candidate == 'true'") {
			return fmt.Errorf("CI %s job must run for release-candidate dispatches", jobID)
		}
	}
	return nil
}
