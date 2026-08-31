package releaseverify

import (
	"fmt"
	"os"
	"path/filepath"
)

func playwrightContainerTargets() []string {
	return []string{
		"fe-visual",
		"fe-visual-update",
		"e2e",
		"tuner-e2e",
		"e2e-update",
	}
}

// VerifyCIContainerDownloads ensures CI-controlled container acquisition uses
// the bounded helper before product tests start, while one reviewed source owns
// each pinned image name.
func VerifyCIContainerDownloads(root string) error {
	if err := verifyRepositoryMakeExecutionEnvironment(root); err != nil {
		return fmt.Errorf("ci Make execution environment: %w", err)
	}
	helperPath := filepath.Join(root, "scripts", "ensure-container-image.sh")
	helper, err := os.Stat(helperPath)
	if err != nil {
		return fmt.Errorf("inspect CI container acquisition helper: %w", err)
	}
	if !helper.Mode().IsRegular() || helper.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("ci container acquisition helper must be an executable regular file")
	}
	if err := verifyContainerImageHelper(helperPath); err != nil {
		return fmt.Errorf("ci container acquisition helper behavior: %w", err)
	}
	runnerPath := filepath.Join(root, "scripts", "run-playwright-container.sh")
	runner, err := os.Stat(runnerPath)
	if err != nil {
		return fmt.Errorf("inspect Playwright container runner: %w", err)
	}
	if !runner.Mode().IsRegular() || runner.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("playwright container runner must be an executable regular file")
	}
	if err := verifyPlaywrightContainerRunner(runnerPath); err != nil {
		return fmt.Errorf("playwright container runner behavior: %w", err)
	}

	frontend, err := readActiveMakefile(filepath.Join(root, "mk", "frontend.mk"))
	if err != nil {
		return err
	}
	store, err := readActiveMakefile(filepath.Join(root, "mk", "store.mk"))
	if err != nil {
		return err
	}
	if err := verifyPostgresImageAuthority(root, store); err != nil {
		return err
	}
	if err := verifyPostgresImageAcquisition(store); err != nil {
		return fmt.Errorf("postgres container acquisition: %w", err)
	}

	repositoryMake, err := readRepositoryMakefiles(root)
	if err != nil {
		return err
	}
	repositoryScripts, err := newRepositoryScriptAudit(root)
	if err != nil {
		return err
	}
	if !frontend.targetDependsOn(".PHONY", "ensure-playwright-image") {
		return fmt.Errorf("playwright target ensure-playwright-image must remain phony")
	}
	for _, target := range playwrightContainerTargets() {
		if !frontend.targetDependsOn(target, "ensure-playwright-image") {
			return fmt.Errorf("playwright target %s must depend directly on ensure-playwright-image", target)
		}
		if err := verifyMakePrerequisiteClosure(repositoryMake, target, playwrightMakePrerequisites(target)); err != nil {
			return fmt.Errorf("playwright target %s: %w", target, err)
		}
		if err := verifyProtectedMakeTarget(repositoryMake, target, playwrightTargetRecipes(target)); err != nil {
			return fmt.Errorf("playwright target %s: %w", target, err)
		}
	}
	if !store.targetDependsOn("test-pg", "ensure-postgres-test-image") {
		return fmt.Errorf("postgres target test-pg must depend directly on ensure-postgres-test-image")
	}
	if err := verifyMakePrerequisiteClosure(repositoryMake, "test-pg", postgresMakePrerequisites()); err != nil {
		return fmt.Errorf("postgres target test-pg: %w", err)
	}
	if err := verifyProtectedMakeTarget(repositoryMake, "test-pg", []string{
		`TESTCONTAINERS_HUB_IMAGE_NAME_PREFIX=docker.io TESTCONTAINERS_RYUK_DISABLED=false $(GO) test -race -tags=integration -timeout=20m ./internal/store/ ./internal/backendtransition/ ./internal/app/`,
	}); err != nil {
		return fmt.Errorf("postgres target test-pg: %w", err)
	}

	if err := verifyPostgresWorkflow(filepath.Join(root, ".github", "workflows", "ci-postgres.yml")); err != nil {
		return err
	}
	if err := VerifyDirectCIPolicyBootstrap(filepath.Join(root, ".github", "workflows", "ci.yml")); err != nil {
		return err
	}
	if err := VerifyCIFamilyWorkflows(filepath.Join(root, ".github", "workflows", "ci.yml")); err != nil {
		return fmt.Errorf("CI reusable workflow acquisition closure: %w", err)
	}
	if err := verifyRepositoryWorkflowContainerAcquisition(filepath.Join(root, ".github", "workflows"), protectedContainerMakeTargets(repositoryMake, repositoryScripts), repositoryScripts); err != nil {
		return err
	}
	return nil
}
