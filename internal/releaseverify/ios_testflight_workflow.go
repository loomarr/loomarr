package releaseverify

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const iosTestFlightScript = "./web/scripts/build-ios-testflight.sh"

// iosTestFlightRunStep is the audited shape of one script step: its exact command, its exact
// environment (nil when it has none), and its exact condition ("" when it is unconditional).
type iosTestFlightRunStep struct {
	run       string
	env       map[string]string
	condition string
}

// The two maps below index every step of the release job. Action steps are fixed here by name
// only; ci_container_workflow_actions.go pins their commit and inputs.
var iosTestFlightActionSteps = map[int]string{
	0: "actions/checkout",
	1: "actions/setup-node",
	7: "actions/upload-artifact",
}

var iosTestFlightRunSteps = map[int]iosTestFlightRunStep{
	2: {run: "corepack enable"},
	3: {run: "make fe-install"},
	4: {run: "make fe-api-codegen"},
	5: {
		run: iosTestFlightScript + " prepare-credentials",
		env: map[string]string{
			"ASC_API_KEY_P8_BASE64": "${{ secrets.ASC_API_KEY_P8_BASE64 }}",
			"ASC_API_KEY_ID":        "${{ secrets.ASC_API_KEY_ID }}",
			"ASC_API_ISSUER_ID":     "${{ secrets.ASC_API_ISSUER_ID }}",
			"APPLE_TEAM_ID":         "${{ vars.APPLE_TEAM_ID }}",
			"ASC_APPLE_APP_ID":      "${{ vars.ASC_APPLE_APP_ID }}",
		},
	},
	6: {
		run: iosTestFlightScript + ` build "$RELEASE_VERSION"`,
		env: map[string]string{
			"LOOMARR_APPLE_TEAM_ID": "${{ vars.APPLE_TEAM_ID }}",
			"LOOMARR_ASC_KEY_ID":    "${{ secrets.ASC_API_KEY_ID }}",
			"LOOMARR_ASC_ISSUER_ID": "${{ secrets.ASC_API_ISSUER_ID }}",
			"RELEASE_VERSION":       "${{ inputs.version }}",
		},
	},
	8: {
		run: iosTestFlightScript + " validate",
		env: iosTestFlightSubmitEnvironment(),
	},
	9: {
		run:       iosTestFlightScript + " upload",
		env:       iosTestFlightSubmitEnvironment(),
		condition: "inputs.upload",
	},
	10: {run: `rm -rf -- "$RUNNER_TEMP/asc"`, condition: "always()"},
}

func iosTestFlightSubmitEnvironment() map[string]string {
	return map[string]string{
		"LOOMARR_ASC_KEY_ID":       "${{ secrets.ASC_API_KEY_ID }}",
		"LOOMARR_ASC_ISSUER_ID":    "${{ secrets.ASC_API_ISSUER_ID }}",
		"LOOMARR_ASC_APPLE_APP_ID": "${{ vars.ASC_APPLE_APP_ID }}",
	}
}

// VerifyIOSTestFlightWorkflow keeps the App Store Connect key and the TestFlight upload behind one
// manual, serialized, main-only environment. Every script step is pinned to its exact command and
// environment, so no expression reaches a shell and no step can read the key or skip its cleanup.
func VerifyIOSTestFlightWorkflow(path string) error {
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
	if err := verifyOnlyKeys(root, "iPhone TestFlight workflow", "name", "on", "concurrency", "permissions", "jobs"); err != nil {
		return err
	}
	if scalarValue(root, "name") != "iPhone TestFlight beta" {
		return errors.New("iPhone TestFlight workflow name must be iPhone TestFlight beta")
	}
	if err := verifyIOSTestFlightTrigger(root); err != nil {
		return err
	}
	permissions, err := requiredMap(root, "permissions")
	if err != nil {
		return err
	}
	if err := verifyExactScalarMap(permissions, "iPhone TestFlight permissions", map[string]string{"contents": "read"}); err != nil {
		return err
	}
	concurrency, err := requiredMap(root, "concurrency")
	if err != nil {
		return err
	}
	if err := verifyExactScalarMap(concurrency, "iPhone TestFlight concurrency", map[string]string{
		"group":              "ios-testflight-release",
		"cancel-in-progress": "false",
	}); err != nil {
		return err
	}

	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		return err
	}
	if len(jobs.Content) != 2 || jobs.Content[0].Value != "release" {
		return errors.New("iPhone TestFlight workflow must contain only the audited release job")
	}
	job, err := requiredMap(jobs, "release")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(job, "iPhone TestFlight release job", "name", "if", "runs-on", "timeout-minutes", "environment", "env", "steps"); err != nil {
		return err
	}
	if scalarValue(job, "if") != "github.ref == 'refs/heads/main'" {
		return errors.New("iPhone TestFlight job must run only from refs/heads/main")
	}
	if scalarValue(job, "runs-on") != "xcode-27" || scalarValue(job, "environment") != "ios-testflight" {
		return errors.New("iPhone TestFlight job must run on xcode-27 in the ios-testflight environment")
	}
	if scalarValue(job, "timeout-minutes") != "90" {
		return errors.New("iPhone TestFlight job must keep its 90-minute timeout")
	}
	env, err := requiredMap(job, "env")
	if err != nil {
		return err
	}
	if err := verifyExactScalarMap(env, "iPhone TestFlight job environment", map[string]string{
		"LOOMARR_IOS_BUILD_NUMBER": "${{ github.run_number }}",
		"LOOMARR_IOS_OUTPUT_DIR":   "${{ github.workspace }}/.artifacts/ios-testflight",
	}); err != nil {
		return err
	}
	return verifyIOSTestFlightSteps(job)
}

func verifyIOSTestFlightTrigger(root *yaml.Node) error {
	on, err := requiredMap(root, "on")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(on, "iPhone TestFlight trigger", "workflow_dispatch"); err != nil {
		return err
	}
	dispatch, err := requiredMap(on, "workflow_dispatch")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(dispatch, "iPhone TestFlight dispatch", "inputs"); err != nil {
		return err
	}
	inputs, err := requiredMap(dispatch, "inputs")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(inputs, "iPhone TestFlight inputs", "version", "upload"); err != nil {
		return err
	}
	version, err := requiredMap(inputs, "version")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(version, "iPhone TestFlight version input", "description", "required", "type"); err != nil {
		return err
	}
	if scalarValue(version, "type") != "string" || scalarValue(version, "required") != "true" {
		return errors.New("the iPhone TestFlight version must be a required string input")
	}
	upload, err := requiredMap(inputs, "upload")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(upload, "iPhone TestFlight upload input", "description", "required", "default", "type"); err != nil {
		return err
	}
	if scalarValue(upload, "type") != "boolean" || scalarValue(upload, "default") != "false" || scalarValue(upload, "required") != "true" {
		return errors.New("the TestFlight upload must be an explicit, default-false boolean input")
	}
	return nil
}

func verifyIOSTestFlightSteps(job *yaml.Node) error {
	steps, err := requiredSequence(job, "steps")
	if err != nil {
		return err
	}
	if len(steps.Content) != len(iosTestFlightActionSteps)+len(iosTestFlightRunSteps) {
		return fmt.Errorf("iPhone TestFlight job must contain exactly %d audited steps, found %d",
			len(iosTestFlightActionSteps)+len(iosTestFlightRunSteps), len(steps.Content))
	}
	for index, step := range steps.Content {
		if step.Kind != yaml.MappingNode {
			return fmt.Errorf("iPhone TestFlight step %d must be a mapping", index+1)
		}
		if action, ok := iosTestFlightActionSteps[index]; ok {
			if err := verifyOnlyKeys(step, fmt.Sprintf("iPhone TestFlight step %d", index+1), "name", "uses", "with"); err != nil {
				return err
			}
			if strings.SplitN(scalarValue(step, "uses"), "@", 2)[0] != action {
				return fmt.Errorf("iPhone TestFlight step %d must use %s", index+1, action)
			}
			continue
		}
		want := iosTestFlightRunSteps[index]
		if err := verifyOnlyKeys(step, fmt.Sprintf("iPhone TestFlight step %d", index+1), "name", "if", "env", "run"); err != nil {
			return err
		}
		if strings.TrimSpace(scalarValue(step, "run")) != want.run {
			return fmt.Errorf("iPhone TestFlight step %d is not the audited command %q", index+1, want.run)
		}
		if got := scalarValue(step, "if"); got != want.condition {
			return fmt.Errorf("iPhone TestFlight step %d condition is %q, want %q", index+1, got, want.condition)
		}
		if err := verifyIOSTestFlightStepEnvironment(step, index, want.env); err != nil {
			return err
		}
	}
	return nil
}

func verifyIOSTestFlightStepEnvironment(step *yaml.Node, index int, want map[string]string) error {
	env, ok := mappingValue(step, "env")
	if want == nil {
		if ok {
			return fmt.Errorf("iPhone TestFlight step %d must not declare an environment", index+1)
		}
		return nil
	}
	if !ok {
		return fmt.Errorf("iPhone TestFlight step %d must declare its audited environment", index+1)
	}
	return verifyExactScalarMap(env, fmt.Sprintf("iPhone TestFlight step %d environment", index+1), want)
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
