package releaseverify

import (
	"path/filepath"
	"strings"
)

func leadingShellAssignments(tokens []string) (int, map[string]string) {
	updates := make(map[string]string)
	index := 0
	for index < len(tokens) {
		match := shellAssignment.FindStringSubmatch(tokens[index])
		if match == nil {
			break
		}
		updates[match[1]] = match[2]
		index++
	}
	return index, updates
}

func applyShellAssignments(assignments, updates map[string]string) {
	for name, value := range updates {
		if value == "" || strings.Contains(value, "$") {
			delete(assignments, name)
			continue
		}
		assignments[name] = value
	}
}

func unwrapShellCommand(tokens []string, index int, executable string, assignments map[string]string) (int, string, bool, bool, bool) {
	hasEnvironmentAssignment := false
	for traversed := 0; traversed <= len(tokens); traversed++ {
		base := filepath.Base(executable)
		argumentsStart := index + 1
		next, noExecution, wrapper, ok := wrapperExecutableIndex(base, tokens, argumentsStart)
		if !wrapper {
			return index, executable, true, false, hasEnvironmentAssignment
		}
		if !ok {
			return index, "", false, false, hasEnvironmentAssignment
		}
		if base == "env" && shellArgumentsContainAssignment(tokens[argumentsStart:next]) {
			hasEnvironmentAssignment = true
		}
		if noExecution {
			return index, "", true, true, hasEnvironmentAssignment
		}
		if next >= len(tokens) {
			if base == "xargs" {
				return index, "echo", true, false, hasEnvironmentAssignment
			}
			return index, "", false, false, hasEnvironmentAssignment
		}
		index = next
		var resolved bool
		executable, resolved = resolveLiteralShellToken(tokens[index], assignments)
		if !resolved {
			return index, "", false, false, hasEnvironmentAssignment
		}
	}
	return index, "", false, false, hasEnvironmentAssignment
}

func shellArgumentsContainAssignment(arguments []string) bool {
	for _, argument := range arguments {
		if shellAssignment.MatchString(argument) {
			return true
		}
	}
	return false
}

// wrapperExecutableIndex owns the closed grammar for transparent command
// wrappers used by both Make and workflow classification. Unknown options fail
// closed rather than letting an operand be mistaken for the executable.
func wrapperExecutableIndex(wrapper string, tokens []string, index int) (int, bool, bool, bool) {
	switch wrapper {
	case "exec":
		for index < len(tokens) {
			switch tokens[index] {
			case "--":
				return index + 1, false, true, true
			case "-c", "-l":
				index++
			case "-a":
				if index+1 >= len(tokens) {
					return index, false, true, false
				}
				index += 2
			default:
				if strings.HasPrefix(tokens[index], "-") {
					return index, false, true, false
				}
				return index, false, true, true
			}
		}
		return index, false, true, false
	case "command":
		for index < len(tokens) {
			switch tokens[index] {
			case "--":
				return index + 1, false, true, true
			case "-p":
				index++
			case "-v", "-V":
				return index, true, true, true
			default:
				if strings.HasPrefix(tokens[index], "-") {
					return index, false, true, false
				}
				return index, false, true, true
			}
		}
		return index, false, true, false
	case "env":
		for index < len(tokens) {
			token := tokens[index]
			if shellAssignment.MatchString(token) {
				index++
				continue
			}
			switch token {
			case "--":
				return index + 1, false, true, true
			case "-i", "--ignore-environment", "-0", "--null", "--debug":
				index++
			case "-u", "--unset", "-C", "--chdir", "-S", "--split-string":
				if index+1 >= len(tokens) {
					return index, false, true, false
				}
				index += 2
			default:
				if strings.HasPrefix(token, "--unset=") || strings.HasPrefix(token, "--chdir=") || strings.HasPrefix(token, "--split-string=") ||
					(strings.HasPrefix(token, "-u") && len(token) > 2) || (strings.HasPrefix(token, "-C") && len(token) > 2) || (strings.HasPrefix(token, "-S") && len(token) > 2) {
					index++
					continue
				}
				if strings.HasPrefix(token, "-") {
					return index, false, true, false
				}
				return index, false, true, true
			}
		}
		return index, false, true, false
	case "timeout":
		for index < len(tokens) {
			token := tokens[index]
			switch token {
			case "--":
				index++
				goto timeoutDuration
			case "--foreground", "--preserve-status", "--verbose":
				index++
			case "-s", "--signal", "-k", "--kill-after":
				if index+1 >= len(tokens) {
					return index, false, true, false
				}
				index += 2
			default:
				if strings.HasPrefix(token, "--signal=") || strings.HasPrefix(token, "--kill-after=") {
					index++
					continue
				}
				if strings.HasPrefix(token, "-") {
					return index, false, true, false
				}
				goto timeoutDuration
			}
		}
		return index, false, true, false
	timeoutDuration:
		if index >= len(tokens) {
			return index, false, true, false
		}
		return index + 1, false, true, true
	case "nice":
		for index < len(tokens) {
			token := tokens[index]
			switch token {
			case "--":
				return index + 1, false, true, true
			case "-n", "--adjustment":
				if index+1 >= len(tokens) {
					return index, false, true, false
				}
				index += 2
			default:
				if strings.HasPrefix(token, "--adjustment=") || (strings.HasPrefix(token, "-") && positiveDecimal(strings.TrimPrefix(token, "-"))) {
					index++
					continue
				}
				if strings.HasPrefix(token, "-") {
					return index, false, true, false
				}
				return index, false, true, true
			}
		}
		return index, false, true, false
	case "xargs":
		for index < len(tokens) {
			token := tokens[index]
			switch token {
			case "--":
				return index + 1, false, true, true
			case "-0", "--null", "-r", "--no-run-if-empty", "-t", "--verbose", "-x", "--exit":
				index++
			case "-a", "--arg-file", "-d", "--delimiter", "-E", "--eof", "-I", "--replace", "-L", "--max-lines", "-n", "--max-args", "-P", "--max-procs", "-s", "--max-chars":
				if index+1 >= len(tokens) {
					return index, false, true, false
				}
				index += 2
			default:
				if strings.HasPrefix(token, "--arg-file=") || strings.HasPrefix(token, "--delimiter=") || strings.HasPrefix(token, "--eof=") ||
					strings.HasPrefix(token, "--replace=") || strings.HasPrefix(token, "--max-lines=") || strings.HasPrefix(token, "--max-args=") ||
					strings.HasPrefix(token, "--max-procs=") || strings.HasPrefix(token, "--max-chars=") ||
					(len(token) > 2 && strings.Contains("adEILnPs", token[1:2])) {
					index++
					continue
				}
				if strings.HasPrefix(token, "-") {
					return index, false, true, false
				}
				return index, false, true, true
			}
		}
		return index, false, true, true
	case "source", ".":
		return index, false, true, index < len(tokens)
	default:
		return index - 1, false, false, true
	}
}

func resolveLiteralShellToken(token string, assignments map[string]string) (string, bool) {
	if shellVariableToken.MatchString(token) {
		name := strings.TrimPrefix(token, "$")
		name = strings.TrimPrefix(name, "$")
		name = strings.Trim(name, "{}")
		value, ok := assignments[name]
		return value, ok
	}
	return token, !strings.Contains(token, "$")
}
