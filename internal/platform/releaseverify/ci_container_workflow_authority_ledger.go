package releaseverify

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type workflowAuthorityKey struct {
	workflow string
	job      string
	command  string
}

type workflowAuthorityLedger struct {
	required map[workflowAuthorityKey]int
}

func newWorkflowAuthorityLedger() *workflowAuthorityLedger {
	return &workflowAuthorityLedger{required: make(map[workflowAuthorityKey]int)}

}

func (ledger *workflowAuthorityLedger) expectWorkflow(workflowName string) {
	workflow, ok := workflowAuthorityCatalog().runs[workflowName]
	if !ok {
		return
	}
	for jobName, job := range workflow.jobs {
		for command := range job.steps {
			ledger.required[workflowAuthorityKey{workflow: workflowName, job: jobName, command: command}] = 0
		}
	}
}

func (ledger *workflowAuthorityLedger) authorize(workflowName, jobName string, step, run *yaml.Node, stepIndex int, protectedTargets map[string]struct{}, scripts *repositoryScriptAudit) (bool, error) {
	authority, ok := workflowAuthorityCatalog().runs[workflowName]
	if !ok {
		return false, nil
	}
	jobAuthority, ok := authority.jobs[jobName]
	if !ok {
		return false, nil
	}
	stepAuthority, ok := jobAuthority.steps[run.Value]
	if !ok {
		return false, nil
	}
	if !stepAuthority.allowsAcquisition && !targetsRemainNonAcquiring(stepAuthority.targets, protectedTargets) {
		return false, nil
	}
	if !stepAuthority.allowsAcquisition && scripts != nil && scripts.commandAcquires(run.Value) {
		return false, nil
	}
	if stepAuthority.exactStepIndex && stepIndex != stepAuthority.stepIndex {
		return false, fmt.Errorf("workflow %s job %s source-bound step %q moved from absolute step %d to %d", workflowName, jobName, strings.TrimSpace(run.Value), stepAuthority.stepIndex+1, stepIndex+1)
	}
	if _, hasActionKind := mappingValue(step, "uses"); hasActionKind {
		return false, fmt.Errorf("workflow %s job %s source-bound step %q must remain a run step", workflowName, jobName, strings.TrimSpace(run.Value))
	}
	if stepAuthority.exactName {
		gotName, present := mappingValue(step, "name")
		if stepAuthority.name == "" {
			if present {
				return false, fmt.Errorf("workflow %s job %s source-bound step %q must remain unnamed", workflowName, jobName, strings.TrimSpace(run.Value))
			}
		} else if !present || gotName.Kind != yaml.ScalarNode || gotName.Value != stepAuthority.name {
			return false, fmt.Errorf("workflow %s job %s source-bound step %q name differs from its authority", workflowName, jobName, strings.TrimSpace(run.Value))
		}
	}
	label := fmt.Sprintf("workflow %s job %s source-bound step %q", workflowName, jobName, strings.TrimSpace(run.Value))
	continueOnError := ""
	if stepAuthority.nonBlocking {
		continueOnError = "true"
	}
	if err := verifyExactExecutionContext(step, label+" step", stepAuthority.environment, nil, stepAuthority.condition, stepAuthority.shell, continueOnError); err != nil {
		return false, err
	}
	key := workflowAuthorityKey{workflow: workflowName, job: jobName, command: run.Value}
	ledger.required[key]++
	if ledger.required[key] != 1 {
		return false, fmt.Errorf("workflow %s job %s must contain source-bound run authority %q exactly once", workflowName, jobName, strings.TrimSpace(run.Value))
	}
	return true, nil
}

func verifySourceBoundWorkflowContext(workflowName, jobName string, workflow, job *yaml.Node) error {
	authority, ok := workflowAuthorityCatalog().runs[workflowName]
	if !ok {
		return nil
	}
	jobAuthority, ok := authority.jobs[jobName]
	if !ok {
		return nil
	}
	label := fmt.Sprintf("workflow %s job %s source-bound context", workflowName, jobName)
	if err := verifyExactExecutionContext(workflow, label+" workflow", authority.environment, authority.permissions, "", "", ""); err != nil {
		return err
	}
	if err := verifyExactExecutionContext(job, label+" job", jobAuthority.environment, jobAuthority.permissions, jobAuthority.condition, "", ""); err != nil {
		return err
	}
	return verifyWorkflowJobContextAuthority(workflowName, jobName, job)
}

func verifySourceBoundWorkflowIdentity(workflowName string, workflow *yaml.Node) error {
	authority, ok := workflowAuthorityCatalog().runs[workflowName]
	if !ok || (authority.name == "" && authority.pullRequestTypes == nil && authority.pushBranches == nil && authority.workflowRun == nil) {
		return nil
	}
	name, present := mappingValue(workflow, "name")
	if authority.name == "" {
		if present {
			return fmt.Errorf("workflow %s must remain unnamed", workflowName)
		}
	} else if !present || name.Kind != yaml.ScalarNode || name.Value != authority.name {
		return fmt.Errorf("workflow %s name differs from its source-bound authority", workflowName)
	}
	trigger, present := mappingValue(workflow, "on")
	if !present || trigger.Kind != yaml.MappingNode || len(trigger.Content) != 2 {
		return fmt.Errorf("workflow %s trigger differs from its source-bound authority", workflowName)
	}
	declared := 0
	for _, set := range []bool{authority.pullRequestTypes != nil, authority.pushBranches != nil, authority.workflowRun != nil} {
		if set {
			declared++
		}
	}
	if declared > 1 {
		return fmt.Errorf("workflow %s authority declares conflicting triggers", workflowName)
	}
	if authority.workflowRun != nil {
		return verifyWorkflowRunTrigger(workflowName, trigger, *authority.workflowRun)
	}
	if authority.pullRequestTypes != nil {
		if trigger.Content[0].Value != "pull_request" {
			return fmt.Errorf("workflow %s trigger must remain exactly pull_request", workflowName)
		}
		pullRequest := trigger.Content[1]
		if pullRequest.Kind != yaml.MappingNode || len(pullRequest.Content) != 2 || pullRequest.Content[0].Value != "types" {
			return fmt.Errorf("workflow %s pull_request trigger differs from its source-bound authority", workflowName)
		}
		types := pullRequest.Content[1]
		if types.Kind != yaml.SequenceNode || len(types.Content) != len(authority.pullRequestTypes) {
			return fmt.Errorf("workflow %s pull_request types differ from its source-bound authority", workflowName)
		}
		for index, want := range authority.pullRequestTypes {
			if types.Content[index].Kind != yaml.ScalarNode || types.Content[index].Value != want {
				return fmt.Errorf("workflow %s pull_request types differ from its source-bound authority", workflowName)
			}
		}
		return nil
	}
	if trigger.Content[0].Value != "push" {
		return fmt.Errorf("workflow %s trigger must remain exactly push", workflowName)
	}
	push := trigger.Content[1]
	if push.Kind != yaml.MappingNode || len(push.Content) != 2 || push.Content[0].Value != "branches" {
		return fmt.Errorf("workflow %s push trigger differs from its source-bound authority", workflowName)
	}
	branches := push.Content[1]
	if branches.Kind != yaml.SequenceNode || len(branches.Content) != len(authority.pushBranches) {
		return fmt.Errorf("workflow %s push branches differ from its source-bound authority", workflowName)
	}
	for index, want := range authority.pushBranches {
		if branches.Content[index].Kind != yaml.ScalarNode || branches.Content[index].Value != want {
			return fmt.Errorf("workflow %s push branches differ from its source-bound authority", workflowName)
		}
	}
	return nil
}

func verifyWorkflowRunTrigger(workflowName string, trigger *yaml.Node, want workflowRunTrigger) error {
	if trigger.Content[0].Value != "workflow_run" {
		return fmt.Errorf("workflow %s trigger must remain exactly workflow_run", workflowName)
	}
	run := trigger.Content[1]
	if run.Kind != yaml.MappingNode || len(run.Content) != 4 {
		return fmt.Errorf("workflow %s workflow_run trigger differs from its source-bound authority", workflowName)
	}
	for key, values := range map[string][]string{"workflows": want.workflows, "types": want.types} {
		got, ok := mappingValue(run, key)
		if !ok || got.Kind != yaml.SequenceNode || len(got.Content) != len(values) {
			return fmt.Errorf("workflow %s workflow_run %s differ from its source-bound authority", workflowName, key)
		}
		for index, value := range values {
			if got.Content[index].Kind != yaml.ScalarNode || got.Content[index].Value != value {
				return fmt.Errorf("workflow %s workflow_run %s differ from its source-bound authority", workflowName, key)
			}
		}
	}
	return nil
}

func (ledger *workflowAuthorityLedger) verifyComplete() error {
	keys := make([]workflowAuthorityKey, 0, len(ledger.required))
	for key := range ledger.required {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].workflow != keys[right].workflow {
			return keys[left].workflow < keys[right].workflow
		}
		if keys[left].job != keys[right].job {
			return keys[left].job < keys[right].job
		}
		return keys[left].command < keys[right].command
	})
	for _, key := range keys {
		count := ledger.required[key]
		if count != 1 {
			return fmt.Errorf("workflow %s job %s must contain source-bound run authority %q exactly once (found %d)", key.workflow, key.job, strings.TrimSpace(key.command), count)
		}
	}
	return nil
}

func verifyExactExecutionContext(scope *yaml.Node, label string, wantEnvironment, wantPermissions map[string]string, wantCondition, wantShell, wantContinueOnError string) error {
	gotEnvironment, err := scalarEnvironment(scope)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if !equalStringMaps(gotEnvironment, wantEnvironment) {
		return fmt.Errorf("%s environment differs from its source-bound authority", label)
	}
	gotPermissions, err := scalarMapping(scope, "permissions", "workflow permissions")
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if !equalStringMaps(gotPermissions, wantPermissions) {
		return fmt.Errorf("%s permissions differ from its source-bound authority", label)
	}
	if _, ok := mappingValue(scope, "defaults"); ok {
		return fmt.Errorf("%s must not override run defaults", label)
	}
	for key, want := range map[string]string{
		"if":                wantCondition,
		"shell":             wantShell,
		"working-directory": "",
		"continue-on-error": wantContinueOnError,
	} {
		got, present := mappingValue(scope, key)
		if want == "" {
			if present {
				return fmt.Errorf("%s must not set %s", label, key)
			}
			continue
		}
		if !present || got.Kind != yaml.ScalarNode || got.Value != want {
			return fmt.Errorf("%s %s differs from its source-bound authority", label, key)
		}
	}
	return nil
}

func equalStringMaps(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
