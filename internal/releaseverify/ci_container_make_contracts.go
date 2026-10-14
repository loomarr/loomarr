package releaseverify

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

func verifyPostgresImageAcquisition(makefile *activeMakefile) error {
	implementation := makefile.targets["ensure-postgres-test-image"]
	if implementation == nil {
		return fmt.Errorf("make target ensure-postgres-test-image is missing")
	}
	if !makefile.targetDependsOn(".PHONY", "ensure-postgres-test-image") {
		return fmt.Errorf("make target ensure-postgres-test-image must remain phony")
	}
	want := []string{
		`./scripts/ensure-container-image.sh "$$(cat internal/testkit/postgresimage/image.txt)"`,
		`./scripts/ensure-container-image.sh "$$(cat internal/testkit/ryukimage/image.txt)"`,
	}
	if len(implementation.dependencies) != 0 || !slices.Equal(implementation.recipes, want) {
		return fmt.Errorf("make target ensure-postgres-test-image must read the image authority at recipe time and run only its bounded helpers")
	}
	return nil
}

func (makefile *activeMakefile) targetDependsOn(target, dependency string) bool {
	implementation, ok := makefile.targets[target]
	if !ok {
		return false
	}
	for _, candidate := range implementation.dependencies {
		if candidate == dependency {
			return true
		}
	}
	return false
}

type makePrerequisiteContract struct {
	dependencies []string
	recipes      []string
}

func playwrightMakePrerequisites(target string) map[string]makePrerequisiteContract {
	support := "fe-build"
	supportRecipe := `cd $(WEB) && pnpm codegen && pnpm --filter @loomarr/web build`
	if target == "fe-visual" || target == "fe-visual-update" {
		support = "storybook-build"
		supportRecipe = `cd $(WEB) && pnpm --filter @loomarr/web build-storybook`
	}
	return map[string]makePrerequisiteContract{
		"ensure-playwright-image": {recipes: []string{`./scripts/run-playwright-container.sh ensure`}},
		support:                   {recipes: []string{supportRecipe}},
	}
}

func playwrightTargetRecipes(target string) []string {
	mode := target
	switch target {
	case "fe-visual":
		mode = "visual"
	case "fe-visual-update":
		mode = "visual-update"
	}
	return []string{`./scripts/run-playwright-container.sh ` + mode}
}

func protectedContainerMakeTargets(makefile *activeMakefile, scripts *repositoryScriptAudit) map[string]struct{} {
	protected := make(map[string]struct{})
	for name, target := range makefile.targets {
		if name == ".PHONY" {
			continue
		}
		for _, recipe := range target.recipes {
			if makeRecipeAcquiresContainerWithScripts(makefile, recipe, scripts) {
				protected[name] = struct{}{}
				break
			}
		}
	}

	for changed := true; changed; {
		changed = false
		for name, target := range makefile.targets {
			if name == ".PHONY" {
				continue
			}
			if _, alreadyProtected := protected[name]; alreadyProtected {
				continue
			}
			for _, dependency := range target.dependencies {
				if _, reachesProtected := protected[dependency]; reachesProtected {
					protected[name] = struct{}{}
					changed = true
					break
				}
			}
			if _, nowProtected := protected[name]; nowProtected {
				continue
			}
			for _, recipe := range target.recipes {
				if commandInvokesProtectedMakeTarget(normalizedMakeRecipe(recipe), protected) {
					protected[name] = struct{}{}
					changed = true
					break
				}
			}
		}
	}
	return protected
}

func makeRecipeAcquiresContainer(makefile *activeMakefile, recipe string) bool {
	return makeRecipeAcquiresContainerWithScripts(makefile, recipe, nil)
}

func makeRecipeAcquiresContainerWithScripts(makefile *activeMakefile, recipe string, scripts *repositoryScriptAudit) bool {
	active := resolveMakeCommandVariables(makefile, normalizedMakeRecipe(recipe))
	return (scripts != nil && scripts.commandAcquires(active)) ||
		commandContainsProtectedScriptToken(active) ||
		containerCommandInvokesEngine(active) ||
		commandSegmentsInvokeProtectedRoute(active, nil, true, true)
}

func isShellInterpreter(token string) bool {
	base := filepath.Base(token)
	return base == "bash" || base == "sh"
}

func resolveMakeCommandVariables(makefile *activeMakefile, command string) string {
	resolved := command
	for range len(makefile.variables) + 1 {
		next := makeVariableToken.ReplaceAllStringFunc(resolved, func(reference string) string {
			match := makeVariableToken.FindStringSubmatch(reference)
			name := match[1]
			if name == "" {
				name = match[2]
			}
			values := makefile.variables[name]
			if len(values) == 1 && values[0] != "" {
				return values[0]
			}
			return reference
		})
		if next == resolved {
			break
		}
		resolved = next
	}
	return normalizeMakeCommandVariables(resolved)
}

func normalizedMakeRecipe(recipe string) string {
	return strings.TrimSpace(strings.TrimLeft(stripShellComments(recipe), "@+-"))
}

func postgresMakePrerequisites() map[string]makePrerequisiteContract {
	return map[string]makePrerequisiteContract{
		"ensure-postgres-test-image": {recipes: []string{
			`./scripts/ensure-container-image.sh "$$(cat internal/testkit/postgresimage/image.txt)"`,
			`./scripts/ensure-container-image.sh "$$(cat internal/testkit/ryukimage/image.txt)"`,
		}},
		"rust-dev-build": {recipes: []string{`LOOMARR_RELEASE=dev $(CARGO) build --locked -p loomarr-image`}},
	}
}

func verifyProtectedMakeTarget(makefile *activeMakefile, target string, recipes []string) error {
	implementation := makefile.targets[target]
	if implementation == nil {
		return fmt.Errorf("protected target is missing")
	}
	if !makefile.targetDependsOn(".PHONY", target) {
		return fmt.Errorf("protected target must remain phony")
	}
	if !slices.Equal(implementation.recipes, recipes) {
		return fmt.Errorf("protected target recipes differ from the audited product command")
	}
	return nil
}

func verifyMakePrerequisiteClosure(makefile *activeMakefile, target string, allowed map[string]makePrerequisiteContract) error {
	implementation := makefile.targets[target]
	if implementation == nil {
		return fmt.Errorf("make target is missing")
	}
	visited := make(map[string]bool)
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		visited[name] = true
		candidate := makefile.targets[name]
		if candidate == nil {
			return fmt.Errorf("prerequisite %s is missing", name)
		}
		contract, approved := allowed[name]
		if !approved {
			return fmt.Errorf("prerequisite %s is outside the audited closure", name)
		}
		if !slices.Equal(candidate.dependencies, contract.dependencies) || !slices.Equal(candidate.recipes, contract.recipes) {
			return fmt.Errorf("prerequisite %s differs from its audited dependency and recipe contract", name)
		}
		for _, dependency := range candidate.dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		return nil
	}
	for _, dependency := range implementation.dependencies {
		if err := visit(dependency); err != nil {
			return err
		}
	}
	for name := range allowed {
		if !visited[name] {
			return fmt.Errorf("does not reach approved prerequisite %s", name)
		}
	}
	return nil
}
