package releaseverify

import (
	"errors"
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

var imageFilterCommand = regexp.MustCompile(`(?m)^[ \t]*if echo "\$changed" \| grep -qE '([^'\r\n]+)'; then\r?\n[ \t]+echo "image=true"`)

var requiredImageInputs = []string{
	"Dockerfile",
	".dockerignore",
	"LICENSE",
	"THIRD_PARTY_NOTICES.md",
	"Cargo.toml",
	"Cargo.lock",
	"rust-toolchain.toml",
	"rust/loomarr-image/src/main.rs",
	"internal/store/migrations/sqlite/99999_release_probe.sql",
	"internal/app/app.go",
	"go.mod",
	"go.sum",
	"docs/help/release-probe.md",
	"web/apps/web/src/main.tsx",
	"api/openapi.yaml",
	"scripts/check-fe-bundle.mjs",
}

// VerifyCIImageInputs executes the active image-filter regex against one path
// from every source family copied into the release image.
func VerifyCIImageInputs(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parseYAML(data)
	if err != nil {
		return err
	}
	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		return err
	}
	changes, err := requiredMap(jobs, "changes")
	if err != nil {
		return err
	}
	steps, err := requiredSequence(changes, "steps")
	if err != nil {
		return err
	}
	var filterRun string
	for _, step := range steps.Content {
		if step.Kind == yaml.MappingNode && scalarValue(step, "id") == "filter" {
			if filterRun != "" {
				return errors.New("CI changes job contains duplicate filter steps")
			}
			filterRun = scalarValue(step, "run")
		}
	}
	if filterRun == "" {
		return errors.New("CI changes job has no active filter step")
	}
	matches := imageFilterCommand.FindAllStringSubmatch(filterRun, -1)
	if len(matches) != 1 {
		return fmt.Errorf("CI filter step must contain exactly one active image filter, found %d", len(matches))
	}
	filter, err := regexp.Compile(matches[0][1])
	if err != nil {
		return fmt.Errorf("compile CI image filter: %w", err)
	}
	for _, input := range requiredImageInputs {
		if !filter.MatchString(input) {
			return fmt.Errorf("CI image filter misses Docker build input %s", input)
		}
	}
	return nil
}
