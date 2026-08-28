package releaseverify

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

type dependabotConfig struct {
	Updates []struct {
		PackageEcosystem string                     `yaml:"package-ecosystem"`
		Directory        string                     `yaml:"directory"`
		Groups           map[string]dependabotGroup `yaml:"groups"`
		Ignore           []dependabotIgnore         `yaml:"ignore"`
	} `yaml:"updates"`
}

type dependabotGroup struct {
	Patterns        []string `yaml:"patterns"`
	ExcludePatterns []string `yaml:"exclude-patterns"`
	UpdateTypes     []string `yaml:"update-types"`
}

type dependabotIgnore struct {
	DependencyName string   `yaml:"dependency-name"`
	UpdateTypes    []string `yaml:"update-types"`
}

func TestDependabotFrontendGroupsRespectCompatibilityBoundaries(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var config dependabotConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}

	var frontend map[string]dependabotGroup
	var ignores []dependabotIgnore
	frontendUpdateCount := 0
	for _, update := range config.Updates {
		if update.PackageEcosystem == "npm" && update.Directory == "/web" {
			frontendUpdateCount++
			frontend = update.Groups
			ignores = update.Ignore
		}
	}
	if frontendUpdateCount != 1 {
		t.Fatalf("expected exactly one /web npm Dependabot update, found %d", frontendUpdateCount)
	}
	if frontend == nil {
		t.Fatal("missing /web npm Dependabot update")
	}

	native := frontend["expo-sdk-57-native"]
	nativePatterns := []string{
		"expo", "expo-*", "@expo/config-plugins", "expo-doctor",
		"react", "react-dom", "react-native", "react-native-reanimated",
		"react-native-worklets", "react-native-safe-area-context", "react-native-screens",
		"react-native-svg", "react-native-web",
	}
	requireExactSet(t, "group names", keys(frontend), []string{"expo-sdk-57-native", "routine-web", "typescript-toolchain", "vite-toolchain"})
	requireExactSet(t, "expo-sdk-57-native patterns", native.Patterns, nativePatterns)
	requireExactSet(t, "expo-sdk-57-native exclusions", native.ExcludePatterns, nil)
	requireExactSet(t, "expo-sdk-57-native update types", native.UpdateTypes, []string{"minor", "patch"})
	requireExactSet(t, "typescript-toolchain patterns", frontend["typescript-toolchain"].Patterns, []string{"typescript"})
	requireExactSet(t, "typescript-toolchain exclusions", frontend["typescript-toolchain"].ExcludePatterns, nil)
	requireExactSet(t, "typescript-toolchain update types", frontend["typescript-toolchain"].UpdateTypes, []string{"major", "minor", "patch"})
	requireExactSet(t, "vite-toolchain patterns", frontend["vite-toolchain"].Patterns, []string{"vite", "@vitejs/plugin-react"})
	requireExactSet(t, "vite-toolchain exclusions", frontend["vite-toolchain"].ExcludePatterns, nil)
	requireExactSet(t, "vite-toolchain update types", frontend["vite-toolchain"].UpdateTypes, []string{"minor", "patch"})

	routine, ok := frontend["routine-web"]
	if !ok || len(routine.Patterns) != 1 || routine.Patterns[0] != "*" {
		t.Fatalf("routine-web must retain the wildcard pattern: %#v", routine.Patterns)
	}
	routineExclusions := append(append([]string{}, nativePatterns...), "typescript", "vite", "@vitejs/plugin-react")
	requireExactSet(t, "routine-web exclusions", routine.ExcludePatterns, routineExclusions)
	requireExactSet(t, "routine-web update types", routine.UpdateTypes, []string{"minor", "patch"})
	for name, group := range frontend {
		for _, updateType := range group.UpdateTypes {
			if updateType != "major" && updateType != "minor" && updateType != "patch" {
				t.Errorf("%s uses schema-invalid update type %q", name, updateType)
			}
		}
	}
	requireExactSet(t, "major-update ignores", majorIgnores(ignores), append(nativePatterns, "vite", "@vitejs/plugin-react"))
	assertNoGroupOverlap(t, frontend)
}

func TestDependabotDockerNodeMajorHold(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var config dependabotConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}

	var dockerGroups map[string]dependabotGroup
	var dockerIgnores []dependabotIgnore
	dockerUpdateCount := 0
	for _, update := range config.Updates {
		if update.PackageEcosystem == "docker" && update.Directory == "/" {
			dockerUpdateCount++
			dockerGroups = update.Groups
			dockerIgnores = update.Ignore
		}
	}
	if dockerUpdateCount != 1 {
		t.Fatalf("expected exactly one root docker Dependabot update, found %d", dockerUpdateCount)
	}
	requireExactSet(t, "docker group names", keys(dockerGroups), []string{"docker-node"})
	requireExactSet(t, "docker-node patterns", dockerGroups["docker-node"].Patterns, []string{"node"})
	requireExactSet(t, "docker-node exclusions", dockerGroups["docker-node"].ExcludePatterns, nil)
	requireExactSet(t, "docker-node update types", dockerGroups["docker-node"].UpdateTypes, []string{"minor", "patch"})
	requireExactSet(t, "docker major-update ignores", majorIgnores(dockerIgnores), []string{"node"})
	assertNoGroupOverlap(t, dockerGroups)
}

func keys(groups map[string]dependabotGroup) []string {
	result := make([]string, 0, len(groups))
	for name := range groups {
		result = append(result, name)
	}
	return result
}

func requireExactSet(t *testing.T, name string, got, want []string) {
	t.Helper()
	gotSet := make(map[string]bool, len(got))
	for _, value := range got {
		if gotSet[value] {
			t.Errorf("%s contains duplicate %q", name, value)
		}
		gotSet[value] = true
	}
	wantSet := make(map[string]bool, len(want))
	for _, value := range want {
		wantSet[value] = true
	}
	if len(gotSet) != len(wantSet) {
		t.Errorf("%s has %v, want exactly %v", name, got, want)
	}
	for value := range wantSet {
		if !gotSet[value] {
			t.Errorf("%s missing %q", name, value)
		}
	}
	for value := range gotSet {
		if !wantSet[value] {
			t.Errorf("%s has unexpected %q", name, value)
		}
	}
}

func majorIgnores(ignores []dependabotIgnore) []string {
	result := make([]string, 0, len(ignores))
	for _, ignore := range ignores {
		if len(ignore.UpdateTypes) == 1 && ignore.UpdateTypes[0] == "version-update:semver-major" {
			result = append(result, ignore.DependencyName)
		}
	}
	return result
}

func assertNoGroupOverlap(t *testing.T, groups map[string]dependabotGroup) {
	t.Helper()
	owners := make(map[string]string)
	for name, group := range groups {
		seen := make(map[string]bool, len(group.Patterns))
		for _, pattern := range group.Patterns {
			if seen[pattern] {
				t.Errorf("%s contains duplicate pattern %q", name, pattern)
			}
			seen[pattern] = true
			if previous, ok := owners[pattern]; ok {
				t.Errorf("dependency pattern %q crosses group boundaries: %s and %s", pattern, previous, name)
			}
			owners[pattern] = name
		}
	}
}
