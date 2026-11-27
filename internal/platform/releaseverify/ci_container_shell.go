package releaseverify

import (
	"path/filepath"
	"strings"
)

// normalizeShellContinuations applies the shell's lexical backslash-newline
// splice without executing or expanding any command text. Both workflow run
// scalars and logical Make recipes cross this boundary before classification.
// An escaped backslash, a single-quoted one and one in a comment splice nothing.
func normalizeShellContinuations(source string) string {
	runes := scanShell(source).runes
	var spliced strings.Builder
	for index := 0; index < len(runes); index++ {
		if runes[index].value == '\\' && runes[index].class == shellSyntax {
			next := index + 1
			if next < len(runes) && runes[next].value == '\r' {
				next++
			}
			if next < len(runes) && runes[next].value == '\n' {
				index = next
				continue
			}
		}
		spliced.WriteRune(runes[index].value)
	}
	return spliced.String()
}

// shellCommandSubstitutions returns the literal bodies of command and process
// substitutions without evaluating them. The boolean is false when the lexical
// shape is ambiguous, which callers treat as "may acquire" rather than
// attempting partial shell parsing.
func shellCommandSubstitutions(command string) ([]string, bool) {
	scan := scanShell(command)
	// Legacy backticks need escape-sensitive recursive parsing to find nested
	// execution, so a live one is ambiguous.
	if scan.err != nil || scan.backticks {
		return nil, false
	}
	return scan.substitutions, true
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
	flush := func() {
		if word.Len() == 0 {
			return
		}
		tokens = append(tokens, word.String())
		word.Reset()
	}
	for _, character := range scanShell(command).runes {
		switch {
		case character.class == shellSyntax:
		case character.class != shellQuoted && strings.ContainsRune(" \t\r\n;&|()`", character.value):
			flush()
		default:
			word.WriteRune(character.value)
		}
	}
	flush()
	return tokens
}

func stripShellComments(command string) string {
	var active strings.Builder
	for _, character := range scanShell(command).runes {
		if character.class != shellComment {
			active.WriteRune(character.value)
		}
	}
	return active.String()
}

func shellCommandSegments(command string) [][]string {
	var segments [][]string
	var tokens []string
	var word strings.Builder
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
	for _, character := range scanShell(command).runes {
		if character.class == shellSyntax {
			continue
		}
		if character.class != shellQuoted && !character.nested {
			if strings.ContainsRune(";&|\n", character.value) {
				flushSegment()
				continue
			}
			if strings.ContainsRune(" \t\r", character.value) {
				flushWord()
				continue
			}
		}
		word.WriteRune(character.value)
	}
	flushSegment()
	return segments
}
