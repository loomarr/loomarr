package releaseverify

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func verifyRepositoryMakeExecutionEnvironment(root string) error {
	rootMakefile := filepath.Join(root, "Makefile")
	modules, err := filepath.Glob(filepath.Join(root, "mk", "*.mk"))
	if err != nil {
		return err
	}
	sort.Strings(modules)
	paths := append([]string{rootMakefile}, modules...)
	wantIncludes := make(map[string]int, len(modules))
	for _, module := range modules {
		relative, err := filepath.Rel(root, module)
		if err != nil {
			return err
		}
		wantIncludes[filepath.ToSlash(relative)] = 1
	}
	gotIncludes := make(map[string]int, len(modules))
	gotTools := make(map[string]int, 2)
	playwrightUnexports := 0
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect Make source %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("make source must be a regular non-symlink file: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parsed, err := readActiveMakefile(path)
		if err != nil {
			return err
		}
		for targetName, target := range parsed.targets {
			for _, recipe := range target.recipes {
				if err := inspectMakeRecipeReferences(recipe); err != nil {
					return fmt.Errorf("make source %s target %s recipe: %w", path, targetName, err)
				}
			}
		}
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			active := activeMakeSourceLine(strings.TrimSpace(line))
			if active == "" {
				continue
			}
			if strings.HasPrefix(line, "\t") {
				continue
			}
			if err := verifyMakeExpansionPolicy(relative, active); err != nil {
				return fmt.Errorf("make source %s: %w", path, err)
			}
			if strings.HasSuffix(active, `\`) {
				return fmt.Errorf("make source %s uses an unaudited logical continuation: %s", path, active)
			}
			if relative == "mk/frontend.mk" && active == requiredPlaywrightUnexport {
				playwrightUnexports++
				continue
			}
			targetVariable, hasTargetVariable := parseMakeTargetVariable(active)
			if hasTargetVariable && targetVariable.operator == "!=" {
				return fmt.Errorf("make source %s uses a parse-time shell assignment: %s", path, active)
			}
			if makeDirectiveModifier.MatchString(active) && !hasTargetVariable {
				return fmt.Errorf("make source %s uses an unaudited directive: %s", path, active)
			}
			if match := makeAssignmentPattern.FindStringSubmatch(active); match != nil {
				if _, forbidden := forbiddenPlaywrightMakeVariableAuthorities()[match[1]]; forbidden {
					return fmt.Errorf("make source %s assigns removed acquisition-affecting variable %s", path, match[1])
				}
			}
			if hasTargetVariable {
				if _, forbidden := forbiddenPlaywrightMakeVariableAuthorities()[targetVariable.variable]; forbidden {
					return fmt.Errorf("make source %s assigns removed acquisition-affecting variable %s", path, targetVariable.variable)
				}
			}
			if match := makeToolAssignment.FindStringSubmatch(active); match != nil {
				want := strings.ToLower(match[1])
				if path != rootMakefile || match[2] != "?=" || match[3] != want {
					return fmt.Errorf("make source %s changes the closed %s executable: %s", path, match[1], active)
				}
				gotTools[match[1]]++
				continue
			}
			if makeExecutionControl.MatchString(active) || makeControlDefinition.MatchString(active) || makeSpecialControl.MatchString(active) {
				return fmt.Errorf("make source %s changes the closed execution environment: %s", path, active)
			}
			fields := strings.Fields(active)
			if len(fields) > 0 && (fields[0] == "include" || fields[0] == "-include" || fields[0] == "sinclude") {
				if path != rootMakefile || len(fields) != 2 || fields[0] != "include" || wantIncludes[fields[1]] != 1 {
					return fmt.Errorf("make source %s uses an unapproved include: %s", path, active)
				}
				gotIncludes[fields[1]]++
				continue
			}
			if makeAssignmentPattern.MatchString(active) || makeTargetPattern.MatchString(active) || hasTargetVariable {
				continue
			}
			return fmt.Errorf("make source %s uses syntax outside the audited assignment/target grammar: %s", path, active)
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("scan Make source %s: %w", path, err)
		}
	}
	for module, count := range wantIncludes {
		if gotIncludes[module] != count {
			return fmt.Errorf("root Makefile must include %s exactly once", module)
		}
	}
	for _, tool := range []string{"GO", "CARGO"} {
		if gotTools[tool] != 1 {
			return fmt.Errorf("root Makefile must define %s ?= %s exactly once", tool, strings.ToLower(tool))
		}
	}
	if playwrightUnexports != 1 {
		return fmt.Errorf("mk/frontend.mk must unexport removed Playwright container variables exactly once")
	}
	return nil
}

func verifyMakeExpansionPolicy(relative, source string) error {
	_ = relative
	return inspectMakeReferences(source)
}

func inspectMakeReferences(source string) error {
	return inspectMakeReferencesWithEscapedDollar(source, false)
}

func inspectMakeRecipeReferences(source string) error {
	return inspectMakeReferencesWithEscapedDollar(source, true)
}

func inspectMakeReferencesWithEscapedDollar(source string, escapedDollarIsShell bool) error {
	for offset := 0; offset < len(source)-1; offset++ {
		if source[offset] != '$' {
			continue
		}
		if escapedDollarIsShell && source[offset+1] == '$' {
			offset++
			continue
		}
		if source[offset+1] != '(' && source[offset+1] != '{' {
			continue
		}
		content, end, err := makeReferenceContent(source, offset)
		if err != nil {
			return err
		}
		if err := inspectMakeReference(content, escapedDollarIsShell); err != nil {
			return err
		}
		offset = end
	}
	return nil
}

func makeReferenceContent(source string, start int) (string, int, error) {
	closing := []byte{')'}
	if source[start+1] == '{' {
		closing[0] = '}'
	}
	for offset := start + 2; offset < len(source); offset++ {
		if source[offset] == '$' && offset+1 < len(source) && (source[offset+1] == '(' || source[offset+1] == '{') {
			if source[offset+1] == '(' {
				closing = append(closing, ')')
			} else {
				closing = append(closing, '}')
			}
			offset++
			continue
		}
		if source[offset] != closing[len(closing)-1] {
			continue
		}
		closing = closing[:len(closing)-1]
		if len(closing) == 0 {
			return source[start+2 : offset], offset, nil
		}
	}
	return "", 0, fmt.Errorf("contains an unterminated Make reference")
}

func inspectMakeReference(content string, escapedDollarIsShell bool) error {
	if err := inspectMakeReferencesWithEscapedDollar(content, escapedDollarIsShell); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(content)
	if makeVariableName.MatchString(trimmed) {
		return nil
	}
	nameEnd := strings.IndexAny(trimmed, " \t,")
	if nameEnd <= 0 {
		return fmt.Errorf("uses a dynamically named or unaudited Make reference: %s", trimmed)
	}
	name := trimmed[:nameEnd]
	if !makeVariableName.MatchString(name) {
		return fmt.Errorf("uses a dynamically named Make function: %s", trimmed)
	}
	if _, forbidden := sideEffectingMakeFunctionAuthorities()[name]; forbidden {
		return fmt.Errorf("uses side-effecting Make function %s", name)
	}
	if _, allowed := pureMakeFunctionAuthorities()[name]; !allowed {
		return fmt.Errorf("uses side-effecting or unaudited Make function %s", name)
	}
	return nil
}
