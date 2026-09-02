package releaseverify

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCIFFmpegInstallRecoversFromStalePackageMetadata(t *testing.T) {
	command := ffmpegInstallWorkflowCommand(t)

	tests := []struct {
		name     string
		scenario string
		wantLog  string
	}{
		{name: "complete cache avoids the network", scenario: "complete", wantLog: "install:no-download\n"},
		{name: "stale cache refreshes metadata once", scenario: "stale", wantLog: "install:no-download\nupdate\ninstall:network\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			home := filepath.Join(root, "home")
			if err := os.MkdirAll(filepath.Join(home, "ffmpeg-debs"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "ffmpeg-debs", "fixture.deb"), nil, 0o600); err != nil {
				t.Fatal(err)
			}

			writeFixtureExecutable(t, filepath.Join(bin, "sudo"), `#!/bin/sh
set -eu
if [ "$1" = cp ]; then
  exit 0
fi
exec "$@"
`)
			writeFixtureExecutable(t, filepath.Join(bin, "apt-get"), `#!/bin/sh
set -eu
case "$1" in
  update)
    printf 'update\n' >> "$APT_LOG"
    : > "$APT_UPDATED"
    ;;
  install)
    no_download=0
    for argument in "$@"; do
      if [ "$argument" = --no-download ]; then
        no_download=1
      fi
    done
    if [ "$no_download" = 1 ]; then
      printf 'install:no-download\n' >> "$APT_LOG"
      if [ "$APT_SCENARIO" = complete ]; then
        exit 0
      fi
      exit 1
    fi
    printf 'install:network\n' >> "$APT_LOG"
    [ -f "$APT_UPDATED" ] || exit 100
    ;;
  *)
    exit 2
    ;;
esac
`)

			logPath := filepath.Join(root, "apt.log")
			cmd := exec.Command("/bin/bash", "-e", "-o", "pipefail", "-c", command)
			cmd.Env = append(os.Environ(),
				"APT_LOG="+logPath,
				"APT_SCENARIO="+test.scenario,
				"APT_UPDATED="+filepath.Join(root, "updated"),
				"HOME="+home,
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("ffmpeg install step failed: %v\n%s", err, output)
			}
			log, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(log) != test.wantLog {
				t.Fatalf("apt calls = %q, want %q", log, test.wantLog)
			}
		})
	}
}

func ffmpegInstallWorkflowCommand(t *testing.T) string {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(readRepositoryWorkflow(t, "ci-go.yml")), &document); err != nil {
		t.Fatal(err)
	}
	steps := mappingValueMust(t, workflowJobNode(t, document.Content[0], "run"), "steps")
	var command string
	for _, step := range steps.Content {
		name, ok := mappingValue(step, "name")
		if !ok || name.Value != "Install ffmpeg (several filler tests skip without it)" {
			continue
		}
		if command != "" {
			t.Fatal("workflow has more than one ffmpeg install step")
		}
		run, ok := mappingValue(step, "run")
		if !ok || run.Kind != yaml.ScalarNode {
			t.Fatal("ffmpeg install step has no shell command")
		}
		command = run.Value
	}
	if command == "" {
		t.Fatal("workflow has no ffmpeg install step")
	}
	return command
}
