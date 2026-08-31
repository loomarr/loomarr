package releaseverify

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsRejectsTargetAndPatternSpecificShellAssignments(t *testing.T) {
	tests := map[string]struct {
		assignment string
		target     string
		rule       string
	}{
		"target-specific":          {assignment: "parse-hook: SIDE_EFFECT != %s", target: "parse-hook", rule: ".PHONY: parse-hook\nparse-hook:\n\t@:\n"},
		"target-specific override": {assignment: "parse-hook: override SIDE_EFFECT != %s", target: "parse-hook", rule: ".PHONY: parse-hook\nparse-hook:\n\t@:\n"},
		"target-specific export":   {assignment: "parse-hook: export SIDE_EFFECT != %s", target: "parse-hook", rule: ".PHONY: parse-hook\nparse-hook:\n\t@:\n"},
		"pattern-specific":         {assignment: "%%.probe: SIDE_EFFECT != %s", target: "sample.probe", rule: "%.probe:\n\t@:\n"},
		"pattern-specific override": {
			assignment: "%%.probe: override SIDE_EFFECT != %s", target: "sample.probe", rule: "%.probe:\n\t@:\n",
		},
		"pattern-specific export": {
			assignment: "%%.probe: export SIDE_EFFECT != %s", target: "sample.probe", rule: "%.probe:\n\t@:\n",
		},
		"multi-target-specific": {
			assignment: "parse-hook sample.probe: SIDE_EFFECT != %s", target: "parse-hook", rule: ".PHONY: parse-hook\nparse-hook:\n\t@:\n",
		},
		"multi-target-specific override export": {
			assignment: "parse-hook sample.probe: override export SIDE_EFFECT != %s", target: "parse-hook", rule: ".PHONY: parse-hook\nparse-hook:\n\t@:\n",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "shell-assignment-executed")
			makefile := filepath.Join(t.TempDir(), "GNUmakefile")
			writeFixtureFile(t, makefile, fmt.Sprintf(test.assignment, "touch "+marker)+"\n"+test.rule)
			if output, err := runGNUmake(t, makefile, test.target); err != nil {
				t.Fatalf("GNU Make shell-assignment probe: %v\n%s", err, output)
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatalf("GNU Make did not execute the shell assignment: %v", err)
			}

			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, "Makefile")
			writeFixtureFile(t, path, fmt.Sprintf(test.assignment, "printf harmless")+"\n"+readFixtureFile(t, path))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted a target/pattern-specific parse-time shell assignment")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsAllowsHarmlessTargetAndPatternSpecificAssignments(t *testing.T) {
	tests := []struct {
		name       string
		assignment string
		targets    []string
		rule       string
	}{
		{name: "target-specific recursive", assignment: "parse-hook: SIDE_EFFECT = harmless", targets: []string{"parse-hook"}, rule: ".PHONY: parse-hook\nparse-hook:\n\t@:\n"},
		{name: "target-specific simple", assignment: "parse-hook: SIDE_EFFECT := harmless", targets: []string{"parse-hook"}, rule: ".PHONY: parse-hook\nparse-hook:\n\t@:\n"},
		{name: "target-specific conditional", assignment: "parse-hook: SIDE_EFFECT ?= harmless", targets: []string{"parse-hook"}, rule: ".PHONY: parse-hook\nparse-hook:\n\t@:\n"},
		{name: "target-specific append", assignment: "parse-hook: SIDE_EFFECT += harmless", targets: []string{"parse-hook"}, rule: ".PHONY: parse-hook\nparse-hook:\n\t@:\n"},
		{name: "target-specific override", assignment: "parse-hook: override SIDE_EFFECT := harmless", targets: []string{"parse-hook"}, rule: ".PHONY: parse-hook\nparse-hook:\n\t@:\n"},
		{name: "target-specific export", assignment: "parse-hook: export SIDE_EFFECT := harmless", targets: []string{"parse-hook"}, rule: ".PHONY: parse-hook\nparse-hook:\n\t@:\n"},
		{name: "pattern-specific", assignment: "%.probe: SIDE_EFFECT := harmless", targets: []string{"sample.probe"}, rule: "%.probe:\n\t@:\n"},
		{name: "multi-target", assignment: "parse-hook sample.probe: SIDE_EFFECT := harmless", targets: []string{"parse-hook", "sample.probe"}, rule: ".PHONY: parse-hook sample.probe\nparse-hook sample.probe:\n\t@:\n"},
		{name: "multi-target override export", assignment: "parse-hook sample.probe: override export SIDE_EFFECT := harmless", targets: []string{"parse-hook", "sample.probe"}, rule: ".PHONY: parse-hook sample.probe\nparse-hook sample.probe:\n\t@:\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			makefile := filepath.Join(t.TempDir(), "GNUmakefile")
			writeFixtureFile(t, makefile, test.assignment+"\n"+test.rule)
			if output, err := runGNUmake(t, makefile, test.targets...); err != nil {
				t.Fatalf("GNU Make rejected harmless target/pattern-specific assignment: %v\n%s", err, output)
			}

			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, "Makefile")
			writeFixtureFile(t, path, test.assignment+"\n"+readFixtureFile(t, path))
			if err := VerifyCIContainerDownloads(root); err != nil {
				t.Fatalf("VerifyCIContainerDownloads rejected harmless target/pattern-specific assignment: %v", err)
			}
		})
	}
}

func runGNUmake(t *testing.T, makefile string, targets ...string) ([]byte, error) {
	t.Helper()
	args := append([]string{"--no-print-directory", "-f", makefile}, targets...)
	return exec.CommandContext(t.Context(), "make", args...).CombinedOutput()
}

func TestVerifyCIContainerDownloadsRejectsTargetSpecificExpansionAndIndirection(t *testing.T) {
	mutations := map[string]string{
		"parenthesized shell": `parse-hook: SIDE_EFFECT := $(shell printf harmless)`,
		"braced shell":        `%.probe: SIDE_EFFECT := ${shell printf harmless}`,
		"indirect shell":      "FUNCTION := shell\nparse-hook: SIDE_EFFECT := $(call $(FUNCTION),printf harmless)",
		"multi-target shell":  `parse-hook sample.probe: SIDE_EFFECT := $(shell printf harmless)`,
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, "Makefile")
			writeFixtureFile(t, path, mutation+"\n"+readFixtureFile(t, path))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted target/pattern-specific parse-time expansion")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsMultiTargetSpecificRemovedAuthority(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	path := filepath.Join(root, "Makefile")
	writeFixtureFile(t, path, "parse-hook sample.probe: PW_IMAGE := attacker.invalid/image:pinned\n"+readFixtureFile(t, path))
	if err := VerifyCIContainerDownloads(root); err == nil {
		t.Fatal("VerifyCIContainerDownloads accepted a multi-target-specific removed authority")
	}
}

func TestTargetSpecificAssignmentsDoNotMasqueradeAsPrerequisites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "GNUmakefile")
	writeFixtureFile(t, path, "protected companion: HARMLESS := value\nprotected: actual-prerequisite\ncompanion: companion-prerequisite\n")
	parsed, err := readActiveMakefile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.targets["protected"].dependencies; !slices.Equal(got, []string{"actual-prerequisite"}) {
		t.Fatalf("protected target prerequisites = %s, want only actual-prerequisite", strings.Join(got, ", "))
	}
	if got := parsed.targets["companion"].dependencies; !slices.Equal(got, []string{"companion-prerequisite"}) {
		t.Fatalf("companion target prerequisites = %s, want only companion-prerequisite", strings.Join(got, ", "))
	}
}
