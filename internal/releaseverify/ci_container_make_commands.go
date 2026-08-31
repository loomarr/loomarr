package releaseverify

import (
	"path/filepath"
	"strconv"
	"strings"
)

func commandInvokesProtectedMakeTarget(command string, protectedTargets map[string]struct{}) bool {
	return commandSegmentsInvokeProtectedRoute(command, protectedTargets, true, false)
}

// commandSegmentsInvokeProtectedRoute classifies only the repository's closed
// command grammar. Literal assignment-only segments affect later segments in
// source order. Prefix assignments, constructed values, and unresolved
// executable or target aliases are not evaluated; they fail closed instead.
func commandSegmentsInvokeProtectedRoute(command string, protectedTargets map[string]struct{}, allowRecursiveMake, failUnresolvedExecutable bool) bool {
	command = githubExpression.ReplaceAllString(command, "GITHUB_EXPRESSION")
	command = normalizeMakeCommandVariables(command)
	return shellCommandMatchesWithFactory(command, func() shellSegmentClassifier {
		assignments := make(map[string]string)
		return func(segment []string) (bool, []string) {
			commandIndex, updates := leadingShellAssignments(segment)
			if commandIndex == len(segment) {
				applyShellAssignments(assignments, updates)
				return false, nil
			}
			executable, resolved := resolveLiteralShellToken(segment[commandIndex], assignments)
			if !resolved {
				if allowRecursiveMake && (segment[commandIndex] == "$MAKE" || segment[commandIndex] == "$$MAKE") {
					executable = "make"
				} else {
					return failUnresolvedExecutable, nil
				}
			}
			var noExecution, wrapperAssignment bool
			commandIndex, executable, resolved, noExecution, wrapperAssignment = unwrapShellCommand(segment, commandIndex, executable, assignments)
			if !resolved {
				return true, nil
			}
			if noExecution {
				return false, nil
			}
			if isContainerScriptToken(executable) {
				return true, nil
			}
			if allowRecursiveMake && isMakeExecutable(executable) {
				if len(updates) != 0 || wrapperAssignment {
					return true, nil
				}
				return makeArgumentsReachProtectedTarget(segment[commandIndex+1:], assignments, protectedTargets), nil
			}
			if isShellInterpreter(executable) {
				return interpreterProtectedRoute(segment[commandIndex+1:], assignments)
			}
			return false, nestedInterpreterCommands(segment, commandIndex)
		}
	})
}

func makeArgumentsReachProtectedTarget(arguments []string, assignments map[string]string, protectedTargets map[string]struct{}) bool {
	foundTarget := false
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if shellAssignment.MatchString(argument) {
			// A recursive assignment can retarget any command-position Make
			// variable used by the selected graph. The repository's recursive
			// routes need no assignments, so reject rather than trying to prove
			// which variables are harmless under every reachable recipe.
			return true
		}
		if argument == "--" {
			continue
		}
		if strings.HasPrefix(argument, "-") {
			consume, safe := auditedRecursiveMakeOption(argument, arguments[index+1:])
			if !safe {
				return true
			}
			index += consume
			continue
		}
		candidate, resolved := resolveLiteralShellToken(argument, assignments)
		if !resolved || candidate == "" {
			return true
		}
		foundTarget = true
		if _, protected := protectedTargets[candidate]; protected {
			return true
		}
	}
	return !foundTarget
}

// auditedRecursiveMakeOption admits only parallelism, which cannot select a
// different Makefile or change whether recipes execute. Every graph-, source-,
// goal-, or execution-affecting option fails closed instead of being partially
// interpreted. The returned count is the number of following operands consumed.
func auditedRecursiveMakeOption(option string, remaining []string) (int, bool) {
	if option == "-j" || option == "--jobs" {
		if len(remaining) == 0 {
			return 0, false
		}
		return 1, positiveDecimal(remaining[0])
	}
	if strings.HasPrefix(option, "-j") && len(option) > 2 {
		return 0, positiveDecimal(option[2:])
	}
	if value, found := strings.CutPrefix(option, "--jobs="); found {
		return 0, positiveDecimal(value)
	}
	return 0, false
}

func positiveDecimal(value string) bool {
	number, err := strconv.Atoi(value)
	return err == nil && number > 0
}

func interpreterProtectedRoute(arguments []string, assignments map[string]string) (bool, []string) {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "-c" {
			if index+1 >= len(arguments) {
				return true, nil
			}
			return false, []string{arguments[index+1]}
		}
		if strings.HasPrefix(argument, "-") {
			continue
		}
		script, resolved := resolveLiteralShellToken(argument, assignments)
		return !resolved || isContainerScriptToken(script), nil
	}
	return true, nil
}

func normalizeMakeCommandVariables(command string) string {
	return makeVariableToken.ReplaceAllStringFunc(command, func(reference string) string {
		match := makeVariableToken.FindStringSubmatch(reference)
		name := match[1]
		if name == "" {
			name = match[2]
		}
		return "$" + name
	})
}

func isMakeExecutable(token string) bool {
	base := filepath.Base(token)
	return base == "make" || base == "gmake"
}
