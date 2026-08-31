package releaseverify

import (
	"path/filepath"
	"strings"
)

// normalizeShellContinuations applies the shell's lexical backslash-newline
// splice without executing or expanding any command text. Both workflow run
// scalars and logical Make recipes cross this boundary before classification.
func normalizeShellContinuations(source string) string {
	source = strings.ReplaceAll(source, "\\\r\n", "")
	return strings.ReplaceAll(source, "\\\n", "")
}

// shellCommandSubstitutions returns the literal bodies of command and process
// substitutions without evaluating them. The boolean is false when the lexical
// shape is ambiguous, which callers reject rather than attempting partial shell
// parsing.
func shellCommandSubstitutions(command string) ([]string, bool) {
	var substitutions []string
	characters := []rune(command)
	singleQuoted, doubleQuoted, escaped := false, false, false
	for index := 0; index < len(characters); index++ {
		character := characters[index]
		if escaped {
			escaped = false
			continue
		}
		if character == '\\' && !singleQuoted {
			escaped = true
			continue
		}
		if character == '\'' && !doubleQuoted {
			singleQuoted = !singleQuoted
			continue
		}
		if character == '"' && !singleQuoted {
			doubleQuoted = !doubleQuoted
			continue
		}
		if singleQuoted {
			continue
		}
		if character == '`' {
			// Legacy backticks require escape-sensitive recursive shell parsing to
			// distinguish nested execution. No checked-in route needs that grammar,
			// so fail closed and let only an earlier whole-step authority admit it.
			return nil, false
		}
		if character == '$' && index+1 < len(characters) && characters[index+1] == '(' {
			end, body, ok := parenthesizedCommandSubstitution(characters, index+1)
			if !ok {
				return nil, false
			}
			substitutions = append(substitutions, body)
			index = end
			continue
		}
		if (character == '<' || character == '>') && index+1 < len(characters) && characters[index+1] == '(' {
			end, body, ok := parenthesizedCommandSubstitution(characters, index+1)
			if !ok {
				return nil, false
			}
			substitutions = append(substitutions, body)
			index = end
		}
	}
	return substitutions, !singleQuoted && !doubleQuoted && !escaped
}

func parenthesizedCommandSubstitution(characters []rune, start int) (int, string, bool) {
	depth := 1
	singleQuoted, doubleQuoted, escaped := false, false, false
	for index := start + 1; index < len(characters); index++ {
		character := characters[index]
		if escaped {
			escaped = false
			continue
		}
		if character == '\\' && !singleQuoted {
			escaped = true
			continue
		}
		if character == '\'' && !doubleQuoted {
			singleQuoted = !singleQuoted
			continue
		}
		if character == '"' && !singleQuoted {
			doubleQuoted = !doubleQuoted
			continue
		}
		if singleQuoted {
			continue
		}
		switch character {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index, string(characters[start+1 : index]), true
			}
		}
	}
	return 0, "", false
}

func commandContainsProtectedScriptToken(command string) bool {
	for _, token := range shellCommandTokens(command) {
		if isContainerScriptToken(token) {
			return true
		}
	}
	return false
}

func isContainerScriptToken(token string) bool {
	token = strings.TrimPrefix(filepath.ToSlash(token), "./")
	base := filepath.Base(token)
	return base == "ensure-container-image.sh" || base == "run-playwright-container.sh"
}

func containerCommandInvokesEngine(command string) bool {
	return shellCommandMatches(command, containerEngineSegment)
}

func containerEngineSegment(segment []string) (bool, []string) {
	// Shell control words and punctuation can precede an executable in a valid
	// compound command. Conservatively recognize acquisition grammar at every
	// word position instead of pretending the segment is always a simple command.
	for index, token := range segment {
		if isContainerEngineExecutable(filepath.Base(strings.Trim(token, `"'{}()`))) && variableEngineGrammar(segment, index+1) {
			return true, nil
		}
	}
	index, _ := leadingShellAssignments(segment)
	if index == len(segment) {
		return false, nil
	}
	executable := strings.Trim(segment[index], `"'`)
	if shellVariableToken.MatchString(executable) && variableEngineGrammar(segment, index+1) {
		return true, nil
	}
	var resolved, noExecution bool
	index, executable, resolved, noExecution, _ = unwrapShellCommand(segment, index, executable, nil)
	if !resolved {
		return true, nil
	}
	if noExecution {
		return false, nil
	}
	base := filepath.Base(executable)
	if isContainerEngineExecutable(base) {
		return true, nil
	}
	return false, nestedInterpreterCommands(segment, index)
}

func isContainerEngineExecutable(base string) bool {
	switch base {
	case "docker", "podman", "nerdctl", "buildah", "docker-compose":
		return true
	default:
		return false
	}
}

type shellSegmentClassifier func([]string) (bool, []string)

// shellCommandMatches owns the shared fail-closed recursion over logical
// command segments, interpreter -c bodies, and command substitutions. Callers
// retain separate segment grammars for dynamic executables and engines.
func shellCommandMatches(command string, classify shellSegmentClassifier) bool {
	return shellCommandMatchesWithFactory(command, func() shellSegmentClassifier { return classify })
}

func shellCommandMatchesWithFactory(command string, factory func() shellSegmentClassifier) bool {
	return shellCommandMatchesDepth(normalizeShellContinuations(command), 0, factory)
}

func shellCommandMatchesDepth(command string, depth int, factory func() shellSegmentClassifier) bool {
	if depth > 16 {
		return true
	}
	classify := factory()
	for _, segment := range shellCommandSegments(command) {
		matched, nested := classify(segment)
		if matched {
			return true
		}
		for _, nestedCommand := range nested {
			if shellCommandMatchesDepth(nestedCommand, depth+1, factory) {
				return true
			}
		}
	}
	substitutions, unambiguous := shellCommandSubstitutions(command)
	if !unambiguous {
		return true
	}
	for _, substitution := range substitutions {
		if shellCommandMatchesDepth(substitution, depth+1, factory) {
			return true
		}
	}
	return false
}

func nestedInterpreterCommands(segment []string, executableIndex int) []string {
	if executableIndex >= len(segment) {
		return nil
	}
	if filepath.Base(segment[executableIndex]) == "eval" {
		if executableIndex+1 >= len(segment) {
			return nil
		}
		command := strings.Join(segment[executableIndex+1:], " ")
		// The checked-in image certification targets import only the exact
		// worktree environment emitted by this repository helper. Every other
		// eval body remains recursively classified or fails closed on expansion.
		if command == `$$(./scripts/dev-env.sh export)` {
			return nil
		}
		return []string{command}
	}
	if !isShellInterpreter(segment[executableIndex]) {
		return nil
	}
	var commands []string
	for argument := executableIndex + 1; argument+1 < len(segment); argument++ {
		if segment[argument] == "-c" {
			commands = append(commands, segment[argument+1])
		}
	}
	return commands
}

func variableEngineGrammar(tokens []string, start int) bool {
	limit := len(tokens)
	for index := start; index < limit; index++ {
		switch tokens[index] {
		case "pull", "run", "create":
			return true
		case "image":
			if commandWindowContains(tokens, index+1, limit, "pull") {
				return true
			}
		case "container":
			if commandWindowContains(tokens, index+1, limit, "run") || commandWindowContains(tokens, index+1, limit, "create") {
				return true
			}
		case "compose":
			if commandWindowContains(tokens, index+1, limit, "up") {
				return true
			}
		}
	}
	return false
}

func commandWindowContains(tokens []string, start, limit int, want string) bool {
	for index := start; index < limit; index++ {
		if tokens[index] == want {
			return true
		}
	}
	return false
}

// shellCommandTokens is deliberately a classifier, not a shell evaluator. It
// preserves words while splitting every command-list and subshell boundary, so
// acquisition grammar is recognized under quotes, logical operators, and $(...).
func shellCommandTokens(command string) []string {
	var tokens []string
	var word strings.Builder
	singleQuoted, doubleQuoted, escaped := false, false, false
	flush := func() {
		if word.Len() == 0 {
			return
		}
		tokens = append(tokens, word.String())
		word.Reset()
	}
	for _, character := range command {
		if escaped {
			word.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' && !singleQuoted {
			escaped = true
			continue
		}
		if character == '\'' && !doubleQuoted {
			singleQuoted = !singleQuoted
			continue
		}
		if character == '"' && !singleQuoted {
			doubleQuoted = !doubleQuoted
			continue
		}
		if !singleQuoted && !doubleQuoted && (strings.ContainsRune(" \t\r\n;&|()`", character)) {
			flush()
			continue
		}
		word.WriteRune(character)
	}
	flush()
	return tokens
}

func stripShellComments(command string) string {
	lines := strings.Split(command, "\n")
	for lineIndex, line := range lines {
		singleQuoted, doubleQuoted, escaped := false, false, false
		for index, character := range line {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' && !singleQuoted {
				escaped = true
				continue
			}
			if character == '\'' && !doubleQuoted {
				singleQuoted = !singleQuoted
				continue
			}
			if character == '"' && !singleQuoted {
				doubleQuoted = !doubleQuoted
				continue
			}
			if character == '#' && !singleQuoted && !doubleQuoted &&
				(index == 0 || strings.ContainsRune(" \t;&|()", rune(line[index-1]))) {
				lines[lineIndex] = line[:index]
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

func shellCommandSegments(command string) [][]string {
	var segments [][]string
	var tokens []string
	var word strings.Builder
	singleQuoted, doubleQuoted, escaped := false, false, false
	substitutionDepth := 0
	flushWord := func() {
		if word.Len() == 0 {
			return
		}
		tokens = append(tokens, word.String())
		word.Reset()
	}
	flushSegment := func() {
		flushWord()
		if len(tokens) == 0 {
			return
		}
		segments = append(segments, tokens)
		tokens = nil
	}
	characters := []rune(command)
	for index, character := range characters {
		if escaped {
			word.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' && !singleQuoted {
			escaped = true
			continue
		}
		if character == '\'' && !doubleQuoted {
			singleQuoted = !singleQuoted
			continue
		}
		if character == '"' && !singleQuoted {
			doubleQuoted = !doubleQuoted
			continue
		}
		if !singleQuoted && character == '(' && index > 0 && characters[index-1] == '$' {
			substitutionDepth++
			word.WriteRune(character)
			continue
		}
		if !singleQuoted && substitutionDepth > 0 {
			word.WriteRune(character)
			if character == ')' {
				substitutionDepth--
			}
			continue
		}
		if !singleQuoted && !doubleQuoted {
			if strings.ContainsRune(";&|\n", character) {
				flushSegment()
				continue
			}
			if character == ' ' || character == '\t' || character == '\r' {
				flushWord()
				continue
			}
		}
		word.WriteRune(character)
	}
	flushSegment()
	return segments
}
