package releaseverify

import (
	"fmt"
	"path/filepath"
)

// workflowAuthorityCatalog is the one typed interface to the reviewed workflow
// topology, commands, actions, and execution contexts. The constructor returns
// fresh maps so package callers cannot mutate authority for later verification.
type workflowAuthorityRegistry struct {
	topology        map[string]workflowTopologyAuthority
	runs            map[string]workflowAuthority
	actions         map[workflowActionKey]string
	jobContexts     map[workflowJobContextKey]workflowJobContextAuthority
	reusableCallers map[string]reusableWorkflowCallerAuthority
	familyWorkflows map[string]string
}

func workflowAuthorityCatalog() workflowAuthorityRegistry {
	return workflowAuthorityRegistry{
		topology:        workflowTopologyAuthorityEntries(),
		runs:            workflowRunAuthorityEntries(),
		actions:         workflowActionAuthorityEntries(),
		jobContexts:     workflowJobContextAuthorityEntries(),
		reusableCallers: reusableWorkflowCallerAuthorityEntries(),
		familyWorkflows: ciFamilyWorkflowAuthorities(),
	}
}

func verifyWorkflowAuthorityCatalog(catalog workflowAuthorityRegistry) error {
	for workflow, topology := range catalog.topology {
		runs, ok := catalog.runs[workflow]
		if !ok {
			return fmt.Errorf("workflow authority catalog has topology without command/context authority: %s", workflow)
		}
		for job, stepCount := range topology.jobs {
			if _, reusable := catalog.reusableCallers[job]; reusable {
				if workflow != "ci.yml" || stepCount != 0 {
					return fmt.Errorf("workflow authority catalog has misplaced reusable caller %s in %s", job, workflow)
				}
				continue
			}
			if _, registeredJob := runs.jobs[job]; !registeredJob {
				continue
			}
			if _, ok := catalog.jobContexts[workflowJobContextKey{workflow: workflow, job: job}]; !ok {
				return fmt.Errorf("workflow authority catalog lacks job context for %s job %s", workflow, job)
			}
		}
	}
	for workflow, authority := range catalog.runs {
		topology, ok := catalog.topology[workflow]
		if !ok {
			return fmt.Errorf("workflow authority catalog has command authority without topology: %s", workflow)
		}
		for job, jobAuthority := range authority.jobs {
			stepCount, ok := topology.jobs[job]
			if !ok {
				return fmt.Errorf("workflow authority catalog has command authority for unknown %s job %s", workflow, job)
			}
			occupied := make(map[int]string)
			for command, step := range jobAuthority.steps {
				if !step.exactStepIndex || step.stepIndex < 0 || step.stepIndex >= stepCount {
					return fmt.Errorf("workflow authority catalog command %q has invalid step identity in %s job %s", command, workflow, job)
				}
				if previous := occupied[step.stepIndex]; previous != "" {
					return fmt.Errorf("workflow authority catalog commands %q and %q share %s job %s step %d", previous, command, workflow, job, step.stepIndex+1)
				}
				occupied[step.stepIndex] = command
			}
		}
	}
	for key := range catalog.actions {
		topology, ok := catalog.topology[key.workflow]
		if !ok || topology.jobs[key.job] <= key.step || key.step < 0 {
			return fmt.Errorf("workflow authority catalog action has invalid step identity in %s job %s", key.workflow, key.job)
		}
		if run := catalog.runs[key.workflow].jobs[key.job].steps; run != nil {
			for command, authority := range run {
				if authority.stepIndex == key.step {
					return fmt.Errorf("workflow authority catalog action and command %q share %s job %s step %d", command, key.workflow, key.job, key.step+1)
				}
			}
		}
	}
	for key := range catalog.jobContexts {
		topology, ok := catalog.topology[key.workflow]
		if !ok {
			return fmt.Errorf("workflow authority catalog context has unknown workflow %s", key.workflow)
		}
		if _, ok := topology.jobs[key.job]; !ok {
			return fmt.Errorf("workflow authority catalog context has unknown %s job %s", key.workflow, key.job)
		}
	}
	for job, workflow := range catalog.familyWorkflows {
		if _, ok := catalog.reusableCallers[job]; !ok {
			return fmt.Errorf("workflow authority catalog family %s lacks reusable caller authority", job)
		}
		if _, ok := catalog.topology[filepath.Base(workflow)]; !ok {
			return fmt.Errorf("workflow authority catalog family %s references unknown workflow %s", job, workflow)
		}
	}
	if len(catalog.familyWorkflows) != len(catalog.reusableCallers) {
		return fmt.Errorf("workflow authority catalog reusable caller and family topology cardinality differs")
	}
	return nil
}
