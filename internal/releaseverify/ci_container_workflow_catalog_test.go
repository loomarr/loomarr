package releaseverify

import "testing"

func TestWorkflowAuthorityCatalogIsFreshAndRejectsRegistryDrift(t *testing.T) {
	if err := verifyWorkflowAuthorityCatalog(workflowAuthorityCatalog()); err != nil {
		t.Fatalf("checked-in workflow authority catalog: %v", err)
	}

	tests := map[string]func(workflowAuthorityRegistry){
		"missing workflow topology": func(catalog workflowAuthorityRegistry) {
			delete(catalog.topology, "ci-docs.yml")
		},
		"missing command authority": func(catalog workflowAuthorityRegistry) {
			delete(catalog.runs, "ci-docs.yml")
		},
		"action outside topology": func(catalog workflowAuthorityRegistry) {
			catalog.actions[workflowActionKey{workflow: "ci-docs.yml", job: "run", step: 99}] = `{"uses":"actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"}`
		},
		"context outside topology": func(catalog workflowAuthorityRegistry) {
			catalog.jobContexts[workflowJobContextKey{workflow: "ci-docs.yml", job: "attacker"}] = workflowJobContextAuthority{}
		},
		"missing reusable family": func(catalog workflowAuthorityRegistry) {
			delete(catalog.familyWorkflows, "docs")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			mutated := workflowAuthorityCatalog()
			mutate(mutated)
			if err := verifyWorkflowAuthorityCatalog(mutated); err == nil {
				t.Fatal("workflow authority catalog accepted registry drift")
			}
			if err := verifyWorkflowAuthorityCatalog(workflowAuthorityCatalog()); err != nil {
				t.Fatalf("mutating one catalog instance changed later verification: %v", err)
			}
		})
	}
}
