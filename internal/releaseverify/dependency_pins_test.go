package releaseverify

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var exactNPMVersion = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$`)

func TestDirectDependenciesUseExactVersions(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, directory := range []string{"web", "docs-site"} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() && (entry.Name() == "node_modules" || entry.Name() == "dist") {
				return filepath.SkipDir
			}
			if entry.Name() != "package.json" {
				return nil
			}
			checkPackagePins(t, path)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	for _, path := range []string{
		filepath.Join(root, "Cargo.toml"),
		filepath.Join(root, "rust", "loomarr-image", "Cargo.toml"),
		filepath.Join(root, "rust", "loomarr-image", "fuzz", "Cargo.toml"),
	} {
		checkCargoPins(t, path)
	}

	composeFiles, err := filepath.Glob(filepath.Join(root, "docker", "compose*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range composeFiles {
		checkComposeImagePins(t, path)
	}
}

func checkPackagePins(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	for section, dependencies := range map[string]map[string]string{
		"dependencies":         manifest.Dependencies,
		"devDependencies":      manifest.DevDependencies,
		"optionalDependencies": manifest.OptionalDependencies,
	} {
		for name, version := range dependencies {
			if strings.HasPrefix(version, "workspace:") {
				continue
			}
			if strings.HasPrefix(version, "npm:") {
				_, version, _ = strings.Cut(version, "@")
			}
			if !exactNPMVersion.MatchString(version) {
				t.Errorf("%s %s.%s = %q, want one exact version", path, section, name, version)
			}
		}
	}
}

func checkCargoPins(t *testing.T, path string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	}()

	section := ""
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		if !strings.Contains(section, "dependencies]") || line == "" || !strings.Contains(line, "=") {
			continue
		}
		if strings.Contains(line, "workspace = true") || strings.Contains(line, "path =") {
			continue
		}
		version := cargoDependencyVersion(line)
		if !strings.HasPrefix(version, "=") {
			t.Errorf("%s:%d dependency %s uses %q, want an exact =version pin", path, lineNumber, line, version)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func cargoDependencyVersion(line string) string {
	right := strings.TrimSpace(strings.SplitN(line, "=", 2)[1])
	if strings.HasPrefix(right, "\"") {
		return strings.Trim(right, "\"")
	}
	match := regexp.MustCompile(`version\s*=\s*"([^"]+)"`).FindStringSubmatch(right)
	if len(match) == 2 {
		return match[1]
	}
	return fmt.Sprintf("unrecognized declaration %q", right)
}

func checkComposeImagePins(t *testing.T, path string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	}()

	imageLine := regexp.MustCompile(`^\s*image:\s*([^\s#]+)`)
	immutableDigest := regexp.MustCompile(`@sha256:[a-f0-9]{64}$`)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		match := imageLine.FindStringSubmatch(scanner.Text())
		if len(match) != 2 {
			continue
		}
		image := match[1]
		if strings.HasPrefix(image, "ghcr.io/loomarr/loomarr:${LOOMARR_VERSION:") {
			continue
		}
		if !immutableDigest.MatchString(image) {
			t.Errorf("%s:%d image %q must use an immutable sha256 digest", path, lineNumber, image)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}
