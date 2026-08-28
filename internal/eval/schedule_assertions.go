//go:build eval

package eval

import (
	"fmt"
	"slices"
)

func scheduledChecks(c Case, materialized []MaterializedProgram) []string {
	if len(c.RequireScheduledPrograms) == 0 && len(c.ForbidScheduledPrograms) == 0 && len(c.RequireScheduledSequence) == 0 &&
		!(c.TitleEvidence == TitleEvidenceScheduled && (len(c.RequireTitles) > 0 || len(c.ForbidTitles) > 0)) {
		return nil
	}
	programs := materializedProgramIdentities(materialized)
	seen := make(map[string]bool, len(programs))
	for _, program := range programs {
		seen[program] = true
	}
	var failures []string
	for _, required := range c.RequireScheduledPrograms {
		if !seen[required] {
			failures = append(failures, fmt.Sprintf("required scheduled program %q is missing", required))
		}
	}
	for _, forbidden := range c.ForbidScheduledPrograms {
		if seen[forbidden] {
			failures = append(failures, fmt.Sprintf("forbidden scheduled program %q is present", forbidden))
		}
	}
	if len(c.RequireScheduledSequence) > 0 && !containsSequence(programs, c.RequireScheduledSequence) {
		failures = append(failures, fmt.Sprintf("required scheduled sequence %v is missing from %v", c.RequireScheduledSequence, programs))
	}
	if c.TitleEvidence == TitleEvidenceScheduled {
		titles := make(map[string]bool, len(materialized))
		for _, program := range materialized {
			titles[normalizeExactTitle(program.Title)] = true
		}
		for _, required := range c.RequireTitles {
			normalized := normalizeExactTitle(required)
			if normalized == "" {
				failures = append(failures, "required scheduled title must not be blank")
			} else if !titles[normalized] {
				failures = append(failures, fmt.Sprintf("required scheduled title %q is missing", required))
			}
		}
		for _, forbidden := range c.ForbidTitles {
			normalized := normalizeExactTitle(forbidden)
			if normalized == "" {
				failures = append(failures, "forbidden scheduled title must not be blank")
			} else if titles[normalized] {
				failures = append(failures, fmt.Sprintf("forbidden scheduled title %q is present", forbidden))
			}
		}
	}
	return failures
}

func materializedProgramIdentities(programs []MaterializedProgram) []string {
	out := make([]string, len(programs))
	for i := range programs {
		out[i] = programs[i].Identity
	}
	return out
}

func containsSequence(programs, sequence []string) bool {
	if len(sequence) > len(programs) {
		return false
	}
	for start := 0; start+len(sequence) <= len(programs); start++ {
		if slices.Equal(programs[start:start+len(sequence)], sequence) {
			return true
		}
	}
	return false
}
