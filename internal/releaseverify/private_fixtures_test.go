package releaseverify

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyPrivateFixturesRejectsTrackedCaseVariantWithoutDisclosingIt(t *testing.T) {
	t.Parallel()

	candidate := strings.ToUpper(privateFixtureDomainSentinel())
	root := trackedFixtureRepository(t, map[string]string{
		"notes.txt": candidate + "\n",
	})

	err := VerifyPrivateFixtures(root)
	if err == nil {
		t.Fatal("VerifyPrivateFixtures accepted a tracked captured-fixture regression sentinel")
	}
	if !strings.Contains(err.Error(), "private-fixture regression sentinel") {
		t.Fatalf("error %q does not identify the matched fingerprint label", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(candidate)) {
		t.Fatalf("error %q disclosed the matched candidate", err)
	}
}

func TestVerifyPrivateFixturesIgnoresUntrackedCandidates(t *testing.T) {
	t.Parallel()

	root := trackedFixtureRepository(t, map[string]string{"tracked.txt": "safe\n"})
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte(privateFixtureDomainSentinel()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := VerifyPrivateFixtures(root); err != nil {
		t.Fatalf("VerifyPrivateFixtures scanned outside the tracked tree: %v", err)
	}
}

func TestVerifyPrivateFixturesIgnoresTrackedFilesRemovedFromTheWorkingTree(t *testing.T) {
	t.Parallel()

	root := trackedFixtureRepository(t, map[string]string{
		"removed.txt": privateFixtureDomainSentinel() + "\n",
	})
	if err := os.Remove(filepath.Join(root, "removed.txt")); err != nil {
		t.Fatal(err)
	}

	if err := VerifyPrivateFixtures(root); err != nil {
		t.Fatalf("VerifyPrivateFixtures scanned a removed working-tree file: %v", err)
	}
}

func TestVerifyPrivateFixturesScopesCapturedProfileFieldsToAuditedFixtures(t *testing.T) {
	t.Parallel()

	field := "\"Name\"" + ": " + "\"Private Fixture Guard\""
	auditedRoot := trackedFixtureRepository(t, map[string]string{
		"internal/testkit/fixtures/emby/users_list.json": field + "\n",
	})
	if err := VerifyPrivateFixtures(auditedRoot); err == nil || !strings.Contains(err.Error(), "private-fixture field regression sentinel") {
		t.Fatalf("VerifyPrivateFixtures did not reject the audited fixture field: %v", err)
	}

	ordinaryRoot := trackedFixtureRepository(t, map[string]string{"docs/example.json": field + "\n"})
	if err := VerifyPrivateFixtures(ordinaryRoot); err != nil {
		t.Fatalf("VerifyPrivateFixtures widened fixture-only fields to ordinary tracked prose: %v", err)
	}
}

func privateFixtureDomainSentinel() string {
	return "captured-fixture" + ".guard" + ".invalid"
}

func trackedFixtureRepository(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for name, contents := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "."}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	return root
}
