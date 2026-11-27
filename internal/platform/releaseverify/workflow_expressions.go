package releaseverify

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// maskWorkflowExpressions prepares a workflow `run:` text for the shell reader.
// Actions replaces each "${{ ... }}" before any shell starts, so the quotes and
// braces inside one are removed and the shell around it reads on. Only workflow
// run text crosses this: to a shell, "${{" in a script ends at the first "}".
// The error names the line within run, counted from 1.
func maskWorkflowExpressions(run string) (string, int, error) {
	var masked strings.Builder
	inside, literal, line, opened := false, false, 1, 0
	for index := 0; index < len(run); index++ {
		character := run[index]
		if character == '\n' {
			line++
		}
		if !inside {
			if strings.HasPrefix(run[index:], "${{") {
				inside, opened = true, line
				masked.WriteString("${{")
				index += 2
				continue
			}
			masked.WriteByte(character)
			continue
		}
		switch {
		case !literal && strings.HasPrefix(run[index:], "}}"):
			inside = false
			masked.WriteString("}}")
			index++
			continue
		case character == '\'':
			literal = !literal
			continue
		case character == '"' || character == '`' || character == '\\' || (literal && character == '$'):
			// Substituted into the script, these could change its quoting.
			return "", line, fmt.Errorf("workflow expression contains %q, which the shell reader cannot follow", character)
		case character == '{' || character == '}':
			continue
		}
		masked.WriteByte(character)
	}
	if inside {
		return "", opened, fmt.Errorf("unterminated workflow expression")
	}
	return masked.String(), 0, nil
}

// verifyWorkflowExpressions reports an unreadable expression by workflow file
// and line.
func verifyWorkflowExpressions(workflowName string, run *yaml.Node) error {
	_, line, err := maskWorkflowExpressions(run.Value)
	if err == nil {
		return nil
	}
	line += run.Line - 1
	if run.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		line++ // a block scalar's text starts below its key
	}
	return fmt.Errorf("%s:%d: %w", workflowName, line, err)
}
