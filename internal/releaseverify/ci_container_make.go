package releaseverify

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	makeAssignmentPattern  = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*(?:::?[:?]?|\?)?=\s*(.*)$`)
	makeTargetPattern      = regexp.MustCompile(`^((?:[A-Za-z0-9_.-]|\\#)+):\s*(.*)$`)
	makeTargetName         = regexp.MustCompile(`^(?:[A-Za-z0-9_.%-]|\\#)+$`)
	makeTargetVariableTail = regexp.MustCompile(`^\s*((?:(?:override|export|unexport|private)\s+)*)([A-Za-z0-9_.-]+)\s*([:+?!]?=)\s*(.*)$`)
	makeExecutionControl   = regexp.MustCompile(`(?:^|:\s+)(?:(?:override|export|unexport|private)\s+)*(?:SHELL|\.SHELLFLAGS|MAKE|MAKEFLAGS|GNUMAKEFLAGS|MFLAGS|MAKEOVERRIDES|MAKEFILES|MAKELEVEL|GO|GOFLAGS|GOTOOLCHAIN|CARGO|PATH|\.RECIPEPREFIX|\.EXTRA_PREREQS)\s*(?:[:+?!]?=|$)`)
	makeControlDefinition  = regexp.MustCompile(`^(?:(?:override|export|unexport|private)\s+)*(?:define|undefine)\s+(?:SHELL|\.SHELLFLAGS|MAKE|MAKEFLAGS|GNUMAKEFLAGS|MFLAGS|MAKEOVERRIDES|MAKEFILES|MAKELEVEL|GO|GOFLAGS|GOTOOLCHAIN|CARGO|PATH|\.RECIPEPREFIX|\.EXTRA_PREREQS)(?:\s|$)`)
	makeSpecialControl     = regexp.MustCompile(`^(?:\.ONESHELL|\.IGNORE|\.SILENT|\.POSIX|\.SECONDEXPANSION|\.NOTPARALLEL|\.EXPORT_ALL_VARIABLES)\s*:`)
	makeToolAssignment     = regexp.MustCompile(`^(GO|CARGO)\s*([:+?!]?=)\s*(.*)$`)
	makeDirectiveModifier  = regexp.MustCompile(`^(?:ifeq|ifneq|ifdef|ifndef|else|endif|define|endef|override|export|unexport|private|undefine|vpath|load)(?:\s|$)|:\s+(?:override|export|unexport|private)(?:\s|$)`)
	makeVariableName       = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	makeVariableToken      = regexp.MustCompile(`\$\(([A-Za-z_][A-Za-z0-9_]*)\)|\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	shellAssignment        = regexp.MustCompile(`(?s)^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	githubExpression       = regexp.MustCompile(`\$\{\{.*?\}\}`)
)

const requiredPlaywrightUnexport = `unexport PW_DOCKER_USER PW_CI PW_REAL_CI PW_IMAGE PW_SHARD`

func pureMakeFunctionAuthorities() map[string]struct{} {
	return map[string]struct{}{
		"if":       {},
		"or":       {},
		"sort":     {},
		"subst":    {},
		"wildcard": {},
	}
}

func sideEffectingMakeFunctionAuthorities() map[string]struct{} {
	return map[string]struct{}{"eval": {}, "file": {}, "guile": {}, "shell": {}}
}

func forbiddenPlaywrightMakeVariableAuthorities() map[string]struct{} {
	return map[string]struct{}{
		"PW_CI":          {},
		"PW_DOCKER_USER": {},
		"PW_IMAGE":       {},
		"PW_REAL_CI":     {},
		"PW_SHARD":       {},
	}
}

type activeMakefile struct {
	variables map[string][]string
	targets   map[string]*activeMakeTarget
}

type activeMakeTarget struct {
	dependencies []string
	recipes      []string
}

type makeTargetVariableAssignment struct {
	targets   []string
	modifiers string
	variable  string
	operator  string
	value     string
}

// parseMakeTargetVariable structurally separates a target list from its
// target-specific variable assignment. A target list cannot contain a colon
// in the audited Make grammar, so the first colon is the rule separator.
func parseMakeTargetVariable(source string) (makeTargetVariableAssignment, bool) {
	separator := strings.IndexByte(source, ':')
	if separator <= 0 {
		return makeTargetVariableAssignment{}, false
	}
	targets := strings.Fields(source[:separator])
	if len(targets) == 0 {
		return makeTargetVariableAssignment{}, false
	}
	for _, target := range targets {
		if !makeTargetName.MatchString(target) {
			return makeTargetVariableAssignment{}, false
		}
	}
	match := makeTargetVariableTail.FindStringSubmatch(source[separator+1:])
	if match == nil {
		return makeTargetVariableAssignment{}, false
	}
	return makeTargetVariableAssignment{
		targets:   targets,
		modifiers: match[1],
		variable:  match[2],
		operator:  match[3],
		value:     match[4],
	}, true
}

func newActiveMakefile() *activeMakefile {
	return &activeMakefile{variables: make(map[string][]string), targets: make(map[string]*activeMakeTarget)}
}

func readActiveMakefile(path string) (*activeMakefile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	parsed := newActiveMakefile()
	var current *activeMakeTarget
	var continuedRecipe strings.Builder
	flushRecipe := func() {
		command := shellRecipeText(normalizeShellContinuations(continuedRecipe.String()))
		if current != nil && command != "" {
			current.recipes = append(current.recipes, command)
		}
		continuedRecipe.Reset()
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if continuedRecipe.Len() > 0 || strings.HasPrefix(line, "\t") {
			if current != nil {
				if continuedRecipe.Len() > 0 {
					continuedRecipe.WriteByte('\n')
				}
				continuedRecipe.WriteString(trimmed)
				if hasShellLineContinuation(line) {
					continue
				}
				flushRecipe()
			}
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		current = nil
		if assignment, ok := parseMakeTargetVariable(trimmed); ok {
			for _, name := range assignment.targets {
				targetName := activeMakeSyntaxText(name)
				target := parsed.targets[targetName]
				if target == nil {
					target = &activeMakeTarget{}
					parsed.targets[targetName] = target
				}
				current = target
			}
			continue
		}
		if match := makeAssignmentPattern.FindStringSubmatch(trimmed); match != nil {
			parsed.variables[match[1]] = append(parsed.variables[match[1]], activeMakeSyntaxText(match[2]))
			continue
		}
		match := makeTargetPattern.FindStringSubmatch(trimmed)
		if match == nil {
			continue
		}
		targetName := activeMakeSyntaxText(match[1])
		target := parsed.targets[targetName]
		if target == nil {
			target = &activeMakeTarget{}
			parsed.targets[targetName] = target
		}
		prerequisites, inlineRecipe, hasInlineRecipe := splitActiveMakeTarget(match[2])
		target.dependencies = append(target.dependencies, strings.Fields(prerequisites)...)
		if hasInlineRecipe {
			command := shellRecipeText(inlineRecipe)
			if command != "" {
				target.recipes = append(target.recipes, command)
			}
		}
		current = target
	}
	if continuedRecipe.Len() > 0 {
		return nil, fmt.Errorf("scan %s: unterminated continued recipe", path)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return parsed, nil
}

func hasShellLineContinuation(line string) bool {
	backslashes := 0
	for index := len(line) - 1; index >= 0 && line[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func readRepositoryMakefiles(root string) (*activeMakefile, error) {
	paths := []string{filepath.Join(root, "Makefile")}
	modules, err := filepath.Glob(filepath.Join(root, "mk", "*.mk"))
	if err != nil {
		return nil, err
	}
	sort.Strings(modules)
	paths = append(paths, modules...)
	merged := newActiveMakefile()
	for _, path := range paths {
		parsed, err := readActiveMakefile(path)
		if err != nil {
			return nil, err
		}
		for name, values := range parsed.variables {
			merged.variables[name] = append(merged.variables[name], values...)
		}
		for name, source := range parsed.targets {
			target := merged.targets[name]
			if target == nil {
				target = &activeMakeTarget{}
				merged.targets[name] = target
			}
			target.dependencies = append(target.dependencies, source.dependencies...)
			target.recipes = append(target.recipes, source.recipes...)
		}
	}
	return merged, nil
}

func activeMakeSourceLine(source string) string {
	if assignment, ok := parseMakeTargetVariable(source); ok {
		var active strings.Builder
		for index, target := range assignment.targets {
			if index > 0 {
				active.WriteByte(' ')
			}
			active.WriteString(activeMakeSyntaxText(target))
		}
		active.WriteString(": ")
		if modifiers := strings.Join(strings.Fields(assignment.modifiers), " "); modifiers != "" {
			active.WriteString(modifiers)
			active.WriteByte(' ')
		}
		active.WriteString(assignment.variable)
		active.WriteByte(' ')
		active.WriteString(assignment.operator)
		if value := activeMakeSyntaxText(assignment.value); value != "" {
			active.WriteByte(' ')
			active.WriteString(value)
		}
		return strings.TrimSpace(active.String())
	}
	if match := makeTargetPattern.FindStringSubmatch(source); match != nil {
		prerequisites, inlineRecipe, hasInlineRecipe := splitActiveMakeTarget(match[2])
		active := activeMakeSyntaxText(match[1]) + ": " + prerequisites
		if hasInlineRecipe {
			active += "; " + shellRecipeText(inlineRecipe)
		}
		return strings.TrimSpace(active)
	}
	if match := makeAssignmentPattern.FindStringSubmatch(source); match != nil {
		assignment := source[:len(source)-len(match[2])] + activeMakeSyntaxText(match[2])
		return strings.TrimSpace(assignment)
	}
	return activeMakeSyntaxText(source)
}

// activeMakeSyntaxText preserves the Make syntax facts needed by container
// policy: the first unescaped # outside a recipe starts a comment, an escaped
// hash remains data, and recipe text retains the shell's separate rules. It
// intentionally canonicalizes insignificant surrounding whitespace; it is not
// a byte-for-byte GNU Make value evaluator.
func activeMakeSyntaxText(source string) string {
	var active strings.Builder
	for index := 0; index < len(source); index++ {
		if source[index] != '#' {
			active.WriteByte(source[index])
			continue
		}
		backslashes := 0
		for offset := index - 1; offset >= 0 && source[offset] == '\\'; offset-- {
			backslashes++
		}
		if backslashes%2 == 0 {
			break
		}
		text := active.String()
		active.Reset()
		active.WriteString(strings.TrimSuffix(text, `\`))
		active.WriteByte('#')
	}
	return strings.TrimSpace(active.String())
}

func splitActiveMakeTarget(source string) (string, string, bool) {
	for index := 0; index < len(source); index++ {
		switch source[index] {
		case '#':
			backslashes := 0
			for offset := index - 1; offset >= 0 && source[offset] == '\\'; offset-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				return activeMakeSyntaxText(source[:index]), "", false
			}
		case ';':
			backslashes := 0
			for offset := index - 1; offset >= 0 && source[offset] == '\\'; offset-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				return activeMakeSyntaxText(source[:index]), source[index+1:], true
			}
		}
	}
	return activeMakeSyntaxText(source), "", false
}

func shellRecipeText(source string) string {
	return strings.TrimSpace(source)
}
