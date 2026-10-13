package releaseverify

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	appleTestFlightScript = "./web/scripts/build-apple-testflight.sh"
	appleTestFlightMain   = "github.ref == 'refs/heads/main'"
	// The plan job reads the event payload itself, so no input reaches its shell.
	appleTestFlightPlanCommand = appleTestFlightScript + ` plan "$EVENT_NAME" "$GITHUB_EVENT_PATH" >> "$GITHUB_OUTPUT"`
	// A run releases only the apps the plan names: the dispatched app, or each app whose CI impact
	// gate a push's diff selects.
	appleTestFlightReleaseCondition = appleTestFlightMain + " && needs.plan.outputs.apps != '[]'"
	// The release job's matrix is exactly the plan's list of apps; the script accepts only the
	// apps it knows.
	appleTestFlightAppsMatrix = "${{ fromJSON(needs.plan.outputs.apps) }}"
	// Each step names its app through the job environment, never through an expression in a shell.
	appleTestFlightAppArgument = appleTestFlightScript + ` "$LOOMARR_APPLE_APP"`
)

// The apps a dispatch may choose, in the script's order.
var appleTestFlightApps = []string{"iphone", "apple-tv"}

var appleTestFlightPlanOutputs = map[string]string{
	"apps":    "${{ steps.plan.outputs.apps }}",
	"upload":  "${{ steps.plan.outputs.upload }}",
	"version": "${{ steps.plan.outputs.version }}",
}

// appleTestFlightRunStep is the audited shape of one script step: its exact command, its exact
// environment (nil when it has none), and its exact condition ("" when it is unconditional).
type appleTestFlightRunStep struct {
	run       string
	env       map[string]string
	condition string
}

// The two maps below index every step of the release job. Action steps are fixed here by name
// only; ci_container_workflow_actions.go pins their commit and inputs.
var appleTestFlightActionSteps = map[int]string{
	0: "actions/checkout",
	1: "actions/setup-node",
	7: "actions/upload-artifact",
}

var appleTestFlightRunSteps = map[int]appleTestFlightRunStep{
	2: {run: "corepack enable"},
	3: {run: "make fe-install"},
	4: {run: "make fe-api-codegen"},
	5: {
		run: appleTestFlightAppArgument + " prepare-credentials",
		env: map[string]string{
			"ASC_API_KEY_P8_BASE64": "${{ secrets.ASC_API_KEY_P8_BASE64 }}",
			"ASC_API_KEY_ID":        "${{ secrets.ASC_API_KEY_ID }}",
			"ASC_API_ISSUER_ID":     "${{ secrets.ASC_API_ISSUER_ID }}",
			"APPLE_TEAM_ID":         "${{ vars.APPLE_TEAM_ID }}",
			"ASC_APPLE_APP_ID":      "${{ vars.ASC_APPLE_APP_ID }}",
			"ASC_APPLE_TV_APP_ID":   "${{ vars.ASC_APPLE_TV_APP_ID }}",
		},
	},
	6: {
		run: appleTestFlightAppArgument + ` build "$RELEASE_VERSION"`,
		env: map[string]string{
			"LOOMARR_APPLE_TEAM_ID": "${{ vars.APPLE_TEAM_ID }}",
			"LOOMARR_ASC_KEY_ID":    "${{ secrets.ASC_API_KEY_ID }}",
			"LOOMARR_ASC_ISSUER_ID": "${{ secrets.ASC_API_ISSUER_ID }}",
			"RELEASE_VERSION":       "${{ needs.plan.outputs.version }}",
		},
	},
	8: {
		run: appleTestFlightAppArgument + " validate",
		env: appleTestFlightSubmitEnvironment(),
	},
	9: {
		run:       appleTestFlightAppArgument + " upload",
		env:       appleTestFlightSubmitEnvironment(),
		condition: "needs.plan.outputs.upload == 'true'",
	},
	10: {run: `rm -rf -- "$RUNNER_TEMP/asc"`, condition: "always()"},
}

func appleTestFlightSubmitEnvironment() map[string]string {
	return map[string]string{
		"LOOMARR_ASC_KEY_ID":          "${{ secrets.ASC_API_KEY_ID }}",
		"LOOMARR_ASC_ISSUER_ID":       "${{ secrets.ASC_API_ISSUER_ID }}",
		"LOOMARR_ASC_APPLE_APP_ID":    "${{ vars.ASC_APPLE_APP_ID }}",
		"LOOMARR_ASC_APPLE_TV_APP_ID": "${{ vars.ASC_APPLE_TV_APP_ID }}",
	}
}

// VerifyAppleTestFlightWorkflow keeps the App Store Connect key and the TestFlight uploads of the
// iPhone and Apple TV apps behind one main-only environment, serialized per app. A secretless plan
// job decides each run: a dispatch carries its own app, version, and upload choice, and a push to
// main releases only the apps it changes. Every script step is pinned to its exact command and
// environment, so no expression reaches a shell and no step can read the key or skip its cleanup.
func VerifyAppleTestFlightWorkflow(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parseYAML(data)
	if err != nil {
		return err
	}
	if err := verifyUses(root); err != nil {
		return err
	}
	// No workflow-level concurrency: a plan-only run would replace a pending release.
	if err := verifyOnlyKeys(root, "Apple TestFlight workflow", "name", "on", "permissions", "jobs"); err != nil {
		return err
	}
	if scalarValue(root, "name") != "Apple TestFlight betas" {
		return errors.New("Apple TestFlight workflow name must be Apple TestFlight betas")
	}
	if err := verifyAppleTestFlightTrigger(root); err != nil {
		return err
	}
	permissions, err := requiredMap(root, "permissions")
	if err != nil {
		return err
	}
	if err := verifyExactScalarMap(permissions, "Apple TestFlight permissions", map[string]string{"contents": "read"}); err != nil {
		return err
	}
	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		return err
	}
	if len(jobs.Content) != 4 || jobs.Content[0].Value != "plan" || jobs.Content[2].Value != "release" {
		return errors.New("Apple TestFlight workflow must contain only the audited plan and release jobs")
	}
	if err := verifyAppleTestFlightPlanJob(jobs); err != nil {
		return err
	}
	job, err := requiredMap(jobs, "release")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(job, "Apple TestFlight release job", "name", "needs", "if", "strategy", "concurrency", "runs-on", "timeout-minutes", "environment", "env", "steps"); err != nil {
		return err
	}
	if scalarValue(job, "needs") != "plan" || scalarValue(job, "if") != appleTestFlightReleaseCondition {
		return fmt.Errorf("Apple TestFlight release job must need plan and run only if %s", appleTestFlightReleaseCondition)
	}
	if err := verifyAppleTestFlightMatrix(job); err != nil {
		return err
	}
	concurrency, err := requiredMap(job, "concurrency")
	if err != nil {
		return err
	}
	if err := verifyExactScalarMap(concurrency, "Apple TestFlight concurrency", map[string]string{
		"group":              "apple-testflight-release-${{ matrix.app }}",
		"cancel-in-progress": "false",
	}); err != nil {
		return err
	}
	if scalarValue(job, "runs-on") != "xcode-27" || scalarValue(job, "environment") != "ios-testflight" {
		return errors.New("Apple TestFlight job must run on xcode-27 in the ios-testflight environment")
	}
	if scalarValue(job, "timeout-minutes") != "90" {
		return errors.New("Apple TestFlight job must keep its 90-minute timeout")
	}
	env, err := requiredMap(job, "env")
	if err != nil {
		return err
	}
	if err := verifyExactScalarMap(env, "Apple TestFlight job environment", map[string]string{
		"LOOMARR_APPLE_APP":          "${{ matrix.app }}",
		"LOOMARR_APPLE_BUILD_NUMBER": "${{ github.run_number }}",
		"LOOMARR_APPLE_OUTPUT_DIR":   "${{ github.workspace }}/.artifacts/testflight",
	}); err != nil {
		return err
	}
	return verifyAppleTestFlightSteps(job)
}

// verifyAppleTestFlightMatrix pins the release job to one job per planned app, none of which can
// cancel another's upload.
func verifyAppleTestFlightMatrix(job *yaml.Node) error {
	strategy, err := requiredMap(job, "strategy")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(strategy, "Apple TestFlight strategy", "fail-fast", "matrix"); err != nil {
		return err
	}
	if scalarValue(strategy, "fail-fast") != "false" {
		return errors.New("Apple TestFlight strategy must not fail fast")
	}
	matrix, err := requiredMap(strategy, "matrix")
	if err != nil {
		return err
	}
	return verifyExactScalarMap(matrix, "Apple TestFlight matrix", map[string]string{"app": appleTestFlightAppsMatrix})
}

func verifyAppleTestFlightTrigger(root *yaml.Node) error {
	on, err := requiredMap(root, "on")
	if err != nil {
		return err
	}
	// Never pull_request or merge_group: those run code that has not reached main.
	if err := verifyOnlyKeys(on, "Apple TestFlight trigger", "push", "workflow_dispatch"); err != nil {
		return err
	}
	push, err := requiredMap(on, "push")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(push, "Apple TestFlight push trigger", "branches"); err != nil {
		return err
	}
	branches, err := requiredSequence(push, "branches")
	if err != nil {
		return err
	}
	if len(branches.Content) != 1 || branches.Content[0].Kind != yaml.ScalarNode || branches.Content[0].Value != "main" {
		return errors.New("the Apple TestFlight push trigger must name only the main branch")
	}
	dispatch, err := requiredMap(on, "workflow_dispatch")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(dispatch, "Apple TestFlight dispatch", "inputs"); err != nil {
		return err
	}
	inputs, err := requiredMap(dispatch, "inputs")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(inputs, "Apple TestFlight inputs", "app", "version", "upload"); err != nil {
		return err
	}
	if err := verifyAppleTestFlightAppInput(inputs); err != nil {
		return err
	}
	version, err := requiredMap(inputs, "version")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(version, "Apple TestFlight version input", "description", "required", "type"); err != nil {
		return err
	}
	if scalarValue(version, "type") != "string" || scalarValue(version, "required") != "true" {
		return errors.New("the Apple TestFlight version must be a required string input")
	}
	upload, err := requiredMap(inputs, "upload")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(upload, "Apple TestFlight upload input", "description", "required", "default", "type"); err != nil {
		return err
	}
	if scalarValue(upload, "type") != "boolean" || scalarValue(upload, "default") != "false" || scalarValue(upload, "required") != "true" {
		return errors.New("the TestFlight upload must be an explicit, default-false boolean input")
	}
	return nil
}

// verifyAppleTestFlightAppInput requires a dispatch to name one known app, with no default, so a
// re-upload never builds an app the caller did not choose.
func verifyAppleTestFlightAppInput(inputs *yaml.Node) error {
	app, err := requiredMap(inputs, "app")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(app, "Apple TestFlight app input", "description", "required", "type", "options"); err != nil {
		return err
	}
	if scalarValue(app, "type") != "choice" || scalarValue(app, "required") != "true" {
		return errors.New("the Apple TestFlight app must be a required choice input")
	}
	options, err := requiredSequence(app, "options")
	if err != nil {
		return err
	}
	got := make([]string, 0, len(options.Content))
	for _, option := range options.Content {
		if option.Kind != yaml.ScalarNode {
			return errors.New("the Apple TestFlight app options must be names")
		}
		got = append(got, option.Value)
	}
	if !slices.Equal(got, appleTestFlightApps) {
		return fmt.Errorf("the Apple TestFlight app options must be exactly %v", appleTestFlightApps)
	}
	return nil
}

// verifyAppleTestFlightPlanJob pins the job that decides each run. It has no environment, so no
// secret reaches it, and its only command is the script's plan mode.
func verifyAppleTestFlightPlanJob(jobs *yaml.Node) error {
	job, err := requiredMap(jobs, "plan")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(job, "Apple TestFlight plan job", "name", "if", "runs-on", "outputs", "steps"); err != nil {
		return err
	}
	if scalarValue(job, "if") != appleTestFlightMain || scalarValue(job, "runs-on") != "ubuntu-latest" {
		return errors.New("Apple TestFlight plan job must run on ubuntu-latest only from refs/heads/main")
	}
	outputs, err := requiredMap(job, "outputs")
	if err != nil {
		return err
	}
	if err := verifyExactScalarMap(outputs, "Apple TestFlight plan outputs", appleTestFlightPlanOutputs); err != nil {
		return err
	}
	steps, err := requiredSequence(job, "steps")
	if err != nil {
		return err
	}
	if len(steps.Content) != 2 || steps.Content[0].Kind != yaml.MappingNode || steps.Content[1].Kind != yaml.MappingNode {
		return errors.New("Apple TestFlight plan job must contain exactly the checkout and plan steps")
	}
	checkout, plan := steps.Content[0], steps.Content[1]
	if err := verifyOnlyKeys(checkout, "Apple TestFlight plan checkout", "uses", "with"); err != nil {
		return err
	}
	if strings.SplitN(scalarValue(checkout, "uses"), "@", 2)[0] != "actions/checkout" {
		return errors.New("Apple TestFlight plan job must start with actions/checkout")
	}
	if err := verifyOnlyKeys(plan, "Apple TestFlight plan step", "name", "id", "env", "run"); err != nil {
		return err
	}
	if scalarValue(plan, "id") != "plan" || strings.TrimSpace(scalarValue(plan, "run")) != appleTestFlightPlanCommand {
		return fmt.Errorf("Apple TestFlight plan step must be id plan running %q", appleTestFlightPlanCommand)
	}
	env, err := requiredMap(plan, "env")
	if err != nil {
		return err
	}
	return verifyExactScalarMap(env, "Apple TestFlight plan step environment", map[string]string{
		"EVENT_NAME": "${{ github.event_name }}",
	})
}

func verifyAppleTestFlightSteps(job *yaml.Node) error {
	steps, err := requiredSequence(job, "steps")
	if err != nil {
		return err
	}
	if len(steps.Content) != len(appleTestFlightActionSteps)+len(appleTestFlightRunSteps) {
		return fmt.Errorf("Apple TestFlight job must contain exactly %d audited steps, found %d",
			len(appleTestFlightActionSteps)+len(appleTestFlightRunSteps), len(steps.Content))
	}
	for index, step := range steps.Content {
		if step.Kind != yaml.MappingNode {
			return fmt.Errorf("Apple TestFlight step %d must be a mapping", index+1)
		}
		if action, ok := appleTestFlightActionSteps[index]; ok {
			if err := verifyOnlyKeys(step, fmt.Sprintf("Apple TestFlight step %d", index+1), "name", "uses", "with"); err != nil {
				return err
			}
			if strings.SplitN(scalarValue(step, "uses"), "@", 2)[0] != action {
				return fmt.Errorf("Apple TestFlight step %d must use %s", index+1, action)
			}
			continue
		}
		want := appleTestFlightRunSteps[index]
		if err := verifyOnlyKeys(step, fmt.Sprintf("Apple TestFlight step %d", index+1), "name", "if", "env", "run"); err != nil {
			return err
		}
		if strings.TrimSpace(scalarValue(step, "run")) != want.run {
			return fmt.Errorf("Apple TestFlight step %d is not the audited command %q", index+1, want.run)
		}
		if got := scalarValue(step, "if"); got != want.condition {
			return fmt.Errorf("Apple TestFlight step %d condition is %q, want %q", index+1, got, want.condition)
		}
		if err := verifyAppleTestFlightStepEnvironment(step, index, want.env); err != nil {
			return err
		}
	}
	return nil
}

func verifyAppleTestFlightStepEnvironment(step *yaml.Node, index int, want map[string]string) error {
	env, ok := mappingValue(step, "env")
	if want == nil {
		if ok {
			return fmt.Errorf("Apple TestFlight step %d must not declare an environment", index+1)
		}
		return nil
	}
	if !ok {
		return fmt.Errorf("Apple TestFlight step %d must declare its audited environment", index+1)
	}
	return verifyExactScalarMap(env, fmt.Sprintf("Apple TestFlight step %d environment", index+1), want)
}

// verifyExactScalarMap requires node to be a mapping with exactly the keys of want, each a scalar
// with the wanted value.
func verifyExactScalarMap(node *yaml.Node, context string, want map[string]string) error {
	if node == nil || node.Kind != yaml.MappingNode || len(node.Content) != len(want)*2 {
		return fmt.Errorf("%s must contain exactly %d audited keys", context, len(want))
	}
	for index := 0; index < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		expected, ok := want[key.Value]
		if !ok || value.Kind != yaml.ScalarNode || value.Value != expected {
			return fmt.Errorf("%s %s = %q is not audited", context, key.Value, value.Value)
		}
	}
	return nil
}
